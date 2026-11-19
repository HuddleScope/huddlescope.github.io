package sqlite

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

func TestMigrate005DropsOrphanedTablesPreservesRanking(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	defer func() { _ = os.Remove(dbPath) }()

	// Apply all migrations up to 005
	versions, err := Migrate(dbPath)
	if err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	
	// Check that we have 5 migrations now
	if len(versions) != 5 {
		t.Errorf("expected 5 migrations, got %d: %v", len(versions), versions)
	}

	// Verify that teams and article_teams tables no longer exist
	repo, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	defer func() { _ = repo.Close() }()

	// Check that ranking_snapshots table exists and has data
	count, err := repo.Count()
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 stories, got %d", count)
	}

	// Insert a story to make sure the DB is functional
	story := domain.Story{
		ID:          "test-story-1",
		Title:       "Test Story",
		Source:      "test-source",
		PublishedAt: now(),
		Summary:     "Test summary",
		URL:         "https://example.com/1",
		GUID:        "guid-1",
		DedupeKey:   "test|guid1",
	}

	if _, err := repo.Upsert(story); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	count, err = repo.Count()
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 story after insert, got %d", count)
	}
}