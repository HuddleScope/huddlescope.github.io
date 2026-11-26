package storage

import (
	"path/filepath"
	"testing"

	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// TestNewDeployedManifestStore_SqliteFileRoundTrip proves the factory returns a
// working store for the sqlite-file backend (issue #247 / PUB-01).
func TestNewDeployedManifestStore_SqliteFileRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.StorageConfig{Backend: "sqlite-file", Path: filepath.Join(tmp, "test.db")}
	store, err := NewDeployedManifestStore(cfg)
	if err != nil {
		t.Fatalf("NewDeployedManifestStore: %v", err)
	}
	defer func() { _ = store.Close() }()

	if m, err := store.Get(); err != nil || m != nil {
		t.Fatalf("empty store: got %+v, err %v; want nil, nil", m, err)
	}
	want := domain.DeployedManifest{
		Version:       domain.ManifestVersion,
		DeployedAt:    "2026-01-02T00:00:00Z",
		GeneratedAt:   "2026-01-02T00:00:00Z",
		Commit:        "abc",
		ContentSHA256: "hash",
		Stories:       3,
	}
	if err := store.Put(want); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := store.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || *got != want {
		t.Errorf("round trip mismatch: got %+v want %+v", got, want)
	}
}

// TestNewDeployedManifestStore_MemoryFailsClosed proves the memory backend is
// refused (volatile backends cannot durably hold deploy state; fail closed).
func TestNewDeployedManifestStore_MemoryFailsClosed(t *testing.T) {
	if _, err := NewDeployedManifestStore(config.StorageConfig{Backend: "memory"}); err == nil {
		t.Fatal("expected error for memory backend")
	}
}

// TestNewDeployedManifestStore_UnknownBackendFailsClosed proves an unknown
// backend is refused.
func TestNewDeployedManifestStore_UnknownBackendFailsClosed(t *testing.T) {
	if _, err := NewDeployedManifestStore(config.StorageConfig{Backend: "postgres"}); err == nil {
		t.Fatal("expected error for unknown backend")
	}
}

// TestNewDeployedManifestStore_TursoEmptyURLFailsClosed proves the turso
// backend fails closed on a missing URL (no network attempt).
func TestNewDeployedManifestStore_TursoEmptyURLFailsClosed(t *testing.T) {
	cfg := config.StorageConfig{Backend: "turso", TursoURL: "", TursoToken: ""}
	if _, err := NewDeployedManifestStore(cfg); err == nil {
		t.Fatal("expected error for empty turso URL")
	}
}
