package sqlite

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestMigrate009CreatesIngestLease (AC-5): a fresh DB applies all migrations
// and the ingest_lease table exists.
func TestMigrate009CreatesIngestLease(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")

	versions, err := Migrate(dbPath)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if len(versions) != 12 {
		t.Fatalf("expected 12 migrations, got %d: %v", len(versions), versions)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	var n int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='ingest_lease'`).Scan(&n); err != nil {
		t.Fatalf("query ingest_lease: %v", err)
	}
	if n != 1 {
		t.Errorf("ingest_lease table missing")
	}
	// ingest_runs still present (rebuilt, not dropped).
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='ingest_runs'`).Scan(&n); err != nil {
		t.Fatalf("query ingest_runs: %v", err)
	}
	if n != 1 {
		t.Errorf("ingest_runs table missing after rebuild")
	}
}

// TestMigrate009ExtendedCheck (migration): the rebuilt ingest_runs CHECK accepts
// the two new skipped_* statuses and still rejects garbage.
func TestMigrate009ExtendedCheck(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "check.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := MigrateDB(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Both new statuses must be accepted.
	for _, status := range []string{"skipped_locked", "skipped_fresh"} {
		if _, err := db.Exec(`INSERT INTO ingest_runs (run_id, origin, started_at, status)
			VALUES (?, 'test', '2026-01-01T00:00:00Z', ?)`, status+"-1", status); err != nil {
			t.Errorf("insert status %q should be accepted: %v", status, err)
		}
	}
	// Old statuses still accepted.
	for _, status := range []string{"running", "ok", "partial", "failed"} {
		if _, err := db.Exec(`INSERT INTO ingest_runs (run_id, origin, started_at, status)
			VALUES (?, 'test', '2026-01-01T00:00:00Z', ?)`, status+"-1", status); err != nil {
			t.Errorf("insert status %q should be accepted: %v", status, err)
		}
	}
	// Garbage status must be rejected by the CHECK.
	if _, err := db.Exec(`INSERT INTO ingest_runs (run_id, origin, started_at, status)
		VALUES ('bogus-1', 'test', '2026-01-01T00:00:00Z', 'not-a-status')`); err == nil {
		t.Error("insert of an invalid status should be rejected by CHECK")
	}
}

// TestMigrate009CarryOver (migration): rows present in ingest_runs before 009
// are carried over verbatim into the rebuilt table (no data loss), and the
// migration applies idempotently on re-application.
func TestMigrate009CarryOver(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "carry.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Simulate a production DB at schema 8: create schema_migrations and apply
	// migrations 001-008 manually (MigrateDB would apply all 9 at once; we want
	// to isolate 009 as the only migration it discovers).
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
	)`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	names, err := ListMigrations()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, name := range names {
		ver, _ := parseMigrationVersion(name)
		if ver > 8 {
			continue // skip 009; it is the migration under test
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

	// Seed a few ingest_runs rows (as 008 would leave them) at schema 8.
	seed := []struct {
		id, status, started, fin string
		checked, ok              int
	}{
		{"run-ok", "ok", "2026-01-01T00:00:00Z", "2026-01-01T00:05:00Z", 3, 3},
		{"run-failed", "failed", "2026-01-02T00:00:00Z", "2026-01-02T00:05:00Z", 2, 0},
		{"run-running", "running", "2026-01-03T00:00:00Z", "", 1, 0},
	}
	for _, r := range seed {
		fin := sql.NullString{}
		if r.fin != "" {
			fin = sql.NullString{String: r.fin, Valid: true}
		}
		if _, err := db.Exec(`INSERT INTO ingest_runs (run_id, origin, started_at, finished_at, status, sources_checked, sources_succeeded)
			VALUES (?,?,?,?,?,?,?)`, r.id, "vps", r.started, fin, r.status, r.checked, r.ok); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	// Now run MigrateDB: 009 (under test), 010 (additive display_title) and 011
	// (deployed_manifests) are new. Simulating schema 8 leaves all three
	// undiscovered, so all three apply.
	versions, err := MigrateDB(db)
	if err != nil {
		t.Fatalf("MigrateDB: %v", err)
	}
	// We expect migrations 9, 10, 11, 12 applied (009 is under test; 010/011/012 are additive).
	if len(versions) != 4 {
		t.Errorf("expected migrations [9 10 11 12] applied, got %d: %v", len(versions), versions)
	}
	wantVersions := map[int]bool{9: true, 10: true, 11: true, 12: true}
	for _, v := range versions {
		if !wantVersions[v] {
			t.Errorf("unexpected migration version applied: %d", v)
		}
	}

	// Verify carry-over: all 3 rows survive with their status/counters intact.
	want := map[string]struct {
		status   string
		checked  int
		finValid bool
		finVal   string
	}{
		"run-ok":      {"ok", 3, true, "2026-01-01T00:05:00Z"},
		"run-failed":  {"failed", 2, true, "2026-01-02T00:05:00Z"},
		"run-running": {"running", 1, false, ""},
	}
	for id, w := range want {
		var status, started string
		var fin sql.NullString
		var checked int
		if err := db.QueryRow(`SELECT status, started_at, finished_at, sources_checked FROM ingest_runs WHERE run_id = ?`, id).
			Scan(&status, &started, &fin, &checked); err != nil {
			t.Fatalf("carry-over row %s: %v", id, err)
		}
		if status != w.status || checked != w.checked {
			t.Errorf("row %s = status %q checked %d, want %q %d", id, status, checked, w.status, w.checked)
		}
		if fin.Valid != w.finValid {
			t.Errorf("row %s finished_at valid=%v, want %v", id, fin.Valid, w.finValid)
		}
		if w.finValid && fin.String != w.finVal {
			t.Errorf("row %s finished_at = %q, want %q", id, fin.String, w.finVal)
		}
	}

	// Idempotency: a second MigrateDB applies nothing.
	second, err := MigrateDB(db)
	if err != nil {
		t.Fatalf("second MigrateDB: %v", err)
	}
	if len(second) != 0 {
		t.Errorf("second MigrateDB applied %d migrations, want 0", len(second))
	}
	var cnt int
	if err := db.QueryRow(`SELECT count(*) FROM ingest_runs`).Scan(&cnt); err != nil {
		t.Fatalf("count: %v", err)
	}
	if cnt != 3 {
		t.Errorf("ingest_runs rows = %d, want 3 (no loss/duplication)", cnt)
	}
}
