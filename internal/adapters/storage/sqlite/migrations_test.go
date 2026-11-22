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

	// Check that we have 6 migrations now (001-006)
	if len(versions) != 6 {
		t.Errorf("expected 6 migrations, got %d: %v", len(versions), versions)
	}

	// Verify that teams and article_teams tables no longer exist
	repo, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	defer func() { _ = repo.Close() }()

	for _, table := range []string{"teams", "article_teams"} {
		var n int
		if err := repo.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n); err != nil {
			t.Fatalf("query sqlite_master for %s: %v", table, err)
		}
		if n != 0 {
			t.Errorf("table %s still exists after migration 005", table)
		}
	}

	// ranking_snapshots survives the rebuild and accepts a conf/teams.yaml slug as team_id
	if _, err := repo.db.Exec(`INSERT INTO ranking_snapshots (snapshot_id, rank_date, team_id, rank, metric, value) VALUES
		('44444444-4444-4444-4444-444444444444', '2026-11-19', 'eagles', 1, 'coverage', 0.9)`); err != nil {
		t.Fatalf("insert into rebuilt ranking_snapshots: %v", err)
	}
	var n int
	if err := repo.db.QueryRow(`SELECT count(*) FROM ranking_snapshots WHERE team_id='eagles'`).Scan(&n); err != nil {
		t.Fatalf("query ranking_snapshots: %v", err)
	}
	if n != 1 {
		t.Errorf("ranking_snapshots rows with team_id=eagles = %d; want 1", n)
	}

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
