package turso

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage/sqlite"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// TestTursoDeployedManifestStore_RoundTrip proves the Turso manifest store
// works over any database/sql driver with migrations applied (issue #247 /
// PUB-01). Production uses the libsql driver; this test uses modernc sqlite
// to exercise the Get/Put + lazy-init path without network.
func TestTursoDeployedManifestStore_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	tmpfile := filepath.Join(tmp, "test.db")
	db, err := sql.Open("sqlite", "file:"+tmpfile+"?cache=shared")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	if _, err := sqlite.MigrateDB(db); err != nil {
		t.Fatalf("MigrateDB failed: %v", err)
	}

	store := newDeployedManifestStoreFromDB(db, "")
	defer func() { _ = store.Close() }()

	// Empty store → nil manifest.
	m, err := store.Get()
	if err != nil {
		t.Fatalf("Get on empty store: %v", err)
	}
	if m != nil {
		t.Errorf("expected nil for empty store, got %+v", m)
	}

	// Put then Get round trip.
	want := domain.DeployedManifest{
		Version:       domain.ManifestVersion,
		DeployedAt:    "2026-01-02T00:00:00Z",
		GeneratedAt:   "2026-01-02T00:00:00Z",
		Commit:        "abc123",
		ContentSHA256: "hash123",
		Stories:       5,
		Latest:        2,
	}
	if err := store.Put(want); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := store.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("expected a manifest, got nil")
	}
	if *got != want {
		t.Errorf("round trip mismatch:\n got  %+v\n want %+v", *got, want)
	}
}

// TestTursoDeployedManifestStore_NewStoreEmptyURL proves NewDeployedManifestStore
// fails closed on an empty URL (no network attempt).
func TestTursoDeployedManifestStore_NewStoreEmptyURL(t *testing.T) {
	if _, err := NewDeployedManifestStore("", ""); err == nil {
		t.Fatal("expected error for empty database URL")
	}
}
