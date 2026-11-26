package sqlite

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestMigrate008CreatesIngestRuns (AC-7): a fresh DB applies all migrations,
// ingest_runs exists, and the legacy ingestion_runs table is dropped.
func TestMigrate008CreatesIngestRuns(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")

	versions, err := Migrate(dbPath)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if len(versions) != 12 {
		t.Fatalf("expected 12 migrations, got %d: %v", len(versions), versions)
	}

	repo, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	defer func() { _ = repo.Close() }()

	var n int
	if err := repo.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='ingest_runs'`).Scan(&n); err != nil {
		t.Fatalf("query ingest_runs: %v", err)
	}
	if n != 1 {
		t.Errorf("ingest_runs table missing")
	}
	if err := repo.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='ingestion_runs'`).Scan(&n); err != nil {
		t.Fatalf("query ingestion_runs: %v", err)
	}
	if n != 0 {
		t.Errorf("legacy ingestion_runs table should be dropped")
	}
	// Index present.
	if err := repo.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='index' AND name='idx_ingest_runs_finished'`).Scan(&n); err != nil {
		t.Fatalf("query index: %v", err)
	}
	if n != 1 {
		t.Errorf("idx_ingest_runs_finished missing")
	}
}

// TestMigrate008CarryOver (AC-7): legacy ingestion_runs rows are copied into
// ingest_runs with status mapping completed->ok, then the legacy table is
// dropped.
func TestMigrate008CarryOver(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "legacy.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Simulate a production DB at schema 7: apply migrations 001-007.
	// Migration 001 creates the legacy ingestion_runs table.
	for ver := 1; ver <= 7; ver++ {
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')))`); err != nil {
			t.Fatalf("create schema_migrations: %v", err)
		}
	}
	// Apply 001-007 by name using the embedded FS.
	names, err := ListMigrations()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, name := range names {
		ver, _ := parseMigrationVersion(name)
		if ver > 7 {
			continue // skip 008; it is the migration under test
		}
		raw, err := ReadMigration(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if _, err := db.Exec(string(raw)); err != nil {
			t.Fatalf("exec %s: %v", name, err)
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, ver); err != nil {
			t.Fatalf("mark %d: %v", ver, err)
		}
	}

	// Seed legacy rows in the table 001 created.
	rows := []struct {
		id, started, fin, status string
		total, ok, newA, newS    int
	}{
		{"11111111-1111-1111-1111-111111111111", "2026-01-01T00:00:00Z", "2026-01-01T00:05:00Z", "completed", 3, 3, 10, 4},
		{"22222222-2222-2222-2222-222222222222", "2026-01-02T00:00:00Z", "2026-01-02T00:05:00Z", "failed", 3, 1, 2, 1},
		{"33333333-3333-3333-3333-333333333333", "2026-01-03T00:00:00Z", "", "running", 3, 0, 0, 0},
	}
	for _, r := range rows {
		if _, err := db.Exec(`INSERT INTO ingestion_runs (run_id, started_at, finished_at, sources_total, sources_ok, articles_new, stories_new, status)
			VALUES (?,?,?,?,?,?,?,?)`, r.id, r.started, nullIfEmpty(r.fin), r.total, r.ok, r.newA, r.newS, r.status); err != nil {
			t.Fatalf("seed legacy: %v", err)
		}
	}

	// Now run MigrateDB: 008 and 009 are new (008 carry-over under test, 009
	// lease rebuild), plus 010 (additive display_title) and 011
	// (deployed_manifests). All four apply.
	versions, err := MigrateDB(db)
	if err != nil {
		t.Fatalf("MigrateDB: %v", err)
	}
	wantVersions := map[int]bool{8: true, 9: true, 10: true, 11: true, 12: true}
	if len(versions) != 5 {
		t.Errorf("expected migrations [8 9 10 11 12] applied, got %d: %v", len(versions), versions)
	}
	for _, v := range versions {
		if !wantVersions[v] {
			t.Errorf("unexpected migration version applied: %d", v)
		}
	}

	// Carry-over verification.
	want := map[string]string{
		"11111111-1111-1111-1111-111111111111": "ok",
		"22222222-2222-2222-2222-222222222222": "failed",
		"33333333-3333-3333-3333-333333333333": "running",
	}
	for id, status := range want {
		var got, origin string
		if err := db.QueryRow(`SELECT status, origin FROM ingest_runs WHERE run_id = ?`, id).Scan(&got, &origin); err != nil {
			t.Fatalf("carry-over row %s: %v", id, err)
		}
		if got != status {
			t.Errorf("row %s status = %q, want %q", id, got, status)
		}
		if origin != "legacy" {
			t.Errorf("row %s origin = %q, want legacy", id, origin)
		}
	}

	// Counter mapping: legacy sources_total -> sources_checked.
	var checked int
	if err := db.QueryRow(`SELECT sources_checked FROM ingest_runs WHERE run_id = '11111111-1111-1111-1111-111111111111'`).Scan(&checked); err != nil {
		t.Fatalf("query counter: %v", err)
	}
	if checked != 3 {
		t.Errorf("sources_checked = %d, want 3", checked)
	}

	// Legacy table dropped.
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='ingestion_runs'`).Scan(&n); err != nil {
		t.Fatalf("query legacy: %v", err)
	}
	if n != 0 {
		t.Errorf("legacy ingestion_runs should be dropped")
	}
}

func TestMigrateIdempotentWith008(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "idem.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := MigrateDB(db); err != nil {
		t.Fatalf("first MigrateDB: %v", err)
	}
	second, err := MigrateDB(db)
	if err != nil {
		t.Fatalf("second MigrateDB: %v", err)
	}
	if len(second) != 0 {
		t.Errorf("second MigrateDB applied %d migrations, want 0", len(second))
	}
	// ingest_runs still present and empty.
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM ingest_runs`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("ingest_runs rows = %d, want 0", n)
	}
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
