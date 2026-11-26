package sqlite

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// openManifestTestDB opens a migrated sqlite DB for manifest tests.
func openManifestTestDB(t *testing.T) *sql.DB {
	t.Helper()
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	if _, err := MigrateDB(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestDeployedManifest_TableExists(t *testing.T) {
	db := openManifestTestDB(t)
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='deployed_manifests'`).Scan(&n); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if n != 1 {
		t.Errorf("deployed_manifests table missing")
	}
	// The `commit` column must exist (quoted reserved word).
	if err := db.QueryRow(`SELECT count(*) FROM pragma_table_info('deployed_manifests') WHERE name = 'commit'`).Scan(&n); err != nil {
		t.Fatalf("pragma_table_info: %v", err)
	}
	if n != 1 {
		t.Errorf("deployed_manifests.commit column missing")
	}
}

func TestDeployedManifest_GetEmpty(t *testing.T) {
	db := openManifestTestDB(t)
	m, err := GetDeployedManifest(db)
	if err != nil {
		t.Fatalf("GetDeployedManifest on empty store: %v", err)
	}
	if m != nil {
		t.Errorf("expected nil manifest for empty store, got %+v", m)
	}
}

func TestDeployedManifest_PutGetRoundTrip(t *testing.T) {
	db := openManifestTestDB(t)
	m := domain.DeployedManifest{
		Version:       domain.ManifestVersion,
		DeployedAt:    "2026-01-02T00:00:00Z",
		GeneratedAt:   "2026-01-02T00:00:00Z",
		Commit:        "abc123",
		ContentSHA256: "hash123",
		Stories:       7,
		Latest:        3,
	}
	if err := PutDeployedManifest(db, m); err != nil {
		t.Fatalf("PutDeployedManifest: %v", err)
	}
	got, err := GetDeployedManifest(db)
	if err != nil {
		t.Fatalf("GetDeployedManifest: %v", err)
	}
	if got == nil {
		t.Fatal("expected a manifest, got nil")
	}
	if *got != m {
		t.Errorf("round trip mismatch:\n got  %+v\n want %+v", *got, m)
	}
}

func TestDeployedManifest_PutUpsertSingleRow(t *testing.T) {
	db := openManifestTestDB(t)
	m1 := domain.DeployedManifest{
		Version:       domain.ManifestVersion,
		DeployedAt:    "2026-01-01T00:00:00Z",
		GeneratedAt:   "2026-01-01T00:00:00Z",
		Commit:        "aaa",
		ContentSHA256: "hashA",
		Stories:       1,
	}
	m2 := domain.DeployedManifest{
		Version:       domain.ManifestVersion,
		DeployedAt:    "2026-01-02T00:00:00Z",
		GeneratedAt:   "2026-01-02T00:00:00Z",
		Commit:        "bbb",
		ContentSHA256: "hashB",
		Stories:       2,
	}
	if err := PutDeployedManifest(db, m1); err != nil {
		t.Fatalf("put m1: %v", err)
	}
	if err := PutDeployedManifest(db, m2); err != nil {
		t.Fatalf("put m2: %v", err)
	}
	// Exactly one row must exist (single-row store).
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM deployed_manifests`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("expected exactly 1 manifest row after two puts, got %d", n)
	}
	got, err := GetDeployedManifest(db)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil || got.Commit != "bbb" || got.Stories != 2 {
		t.Errorf("expected m2 to be the stored manifest, got %+v", got)
	}
}

func TestDeployedManifest_PutValidates(t *testing.T) {
	db := openManifestTestDB(t)
	// Missing required fields → error.
	if err := PutDeployedManifest(db, domain.DeployedManifest{Commit: "abc"}); err == nil {
		t.Error("expected error for missing required fields")
	}
	// Version is normalized to ManifestVersion on write.
	m := domain.DeployedManifest{
		Version:       "",
		DeployedAt:    "2026-01-01T00:00:00Z",
		GeneratedAt:   "2026-01-01T00:00:00Z",
		ContentSHA256: "h",
	}
	if err := PutDeployedManifest(db, m); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, err := GetDeployedManifest(db)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil || got.Version != domain.ManifestVersion {
		t.Errorf("expected version normalized to %q, got %+v", domain.ManifestVersion, got)
	}
}
