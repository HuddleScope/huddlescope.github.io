package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

func factoryTestStory() domain.Story {
	return domain.Story{
		ID:          "factory-test-id",
		Title:       "Factory Test Story",
		Source:      "factory-test",
		PublishedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Summary:     "Factory test summary",
		URL:         "https://example.com/factory",
		GUID:        "factory-guid",
		DedupeKey:   "factory-test|factory-guid",
	}
}

func TestNewRepository_Memory(t *testing.T) {
	repo, err := NewRepository(config.StorageConfig{Backend: "memory"})
	if err != nil {
		t.Fatalf("NewRepository(memory) error = %v", err)
	}
	if _, ok := repo.(*MemoryRepository); !ok {
		t.Fatalf("NewRepository(memory) returned %T; want *MemoryRepository", repo)
	}
	if _, err := repo.Upsert(factoryTestStory()); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	if _, err := repo.List(); err != nil {
		t.Fatalf("List failed: %v", err)
	}
}

func TestNewRepository_SQLiteFile(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "repo.db")

	repo, err := NewRepository(config.StorageConfig{Backend: "sqlite-file", Path: dbPath})
	if err != nil {
		t.Fatalf("NewRepository(sqlite-file, %q) error = %v", dbPath, err)
	}
	if _, err := repo.Upsert(factoryTestStory()); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	if _, err := repo.List(); err != nil {
		t.Fatalf("List failed: %v", err)
	}
}

// TestNewRepository_SQLiteFileDefaultPath verifies the factory's empty-Path
// fallback to the relative default data/huddlescope.db. The working directory
// is a temp dir so the relative default never writes into the repository tree.
func TestNewRepository_SQLiteFileDefaultPath(t *testing.T) {
	cwd, err := os.MkdirTemp(t.TempDir(), "cwd")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Chdir(cwd)
	if err := os.MkdirAll("data", 0o755); err != nil {
		t.Fatalf("MkdirAll(data): %v", err)
	}

	repo, err := NewRepository(config.StorageConfig{Backend: "sqlite-file"})
	if err != nil {
		t.Fatalf("NewRepository(sqlite-file, empty Path) error = %v", err)
	}
	if _, err := repo.Upsert(factoryTestStory()); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join("data", "huddlescope.db")); err != nil {
		t.Errorf("expected default database at data/huddlescope.db: %v", err)
	}
}

func TestNewRepository_Turso(t *testing.T) {
	// A file: DSN runs the turso adapter against local sqlite without a
	// network connection (same pattern as turso package tests).
	tmpfile := t.TempDir() + "/turso-dispatch.db"

	repo, err := NewRepository(config.StorageConfig{
		Backend:  "turso",
		TursoURL: "file:" + tmpfile,
	})
	if err != nil {
		t.Fatalf("NewRepository(turso, file: DSN) error = %v", err)
	}
	if _, err := repo.Upsert(factoryTestStory()); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	if _, err := repo.List(); err != nil {
		t.Fatalf("List failed: %v", err)
	}
}

func TestNewRepository_UnknownBackend(t *testing.T) {
	_, err := NewRepository(config.StorageConfig{Backend: "redis"})
	if err == nil {
		t.Fatal("NewRepository(redis) error = nil; want unknown backend error")
	}
	want := `unknown backend "redis"; use memory, sqlite-file, or turso`
	if !strings.Contains(err.Error(), want) {
		t.Errorf("NewRepository(redis) error = %q; want %q", err.Error(), want)
	}
}
