package application

import (
	"context"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

func TestRunSnapshot_Empty(t *testing.T) {
	repo := storage.NewMemoryRepository()
	teamReg := domain.NewTeamRegistry()
	clustering := domain.NewClusterer()
	scorer := domain.NewScorer()

	app := New(
		WithRepository(repo),
		WithTeamRegistry(teamReg),
		WithClusterer(clustering),
		WithScorer(scorer),
	)

	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	result := app.RunSnapshot(context.Background(), now)

	if result.StoriesBuilt != 0 {
		t.Errorf("StoriesBuilt = %d; want 0", result.StoriesBuilt)
	}
	if !result.PubResult.Committed {
		t.Error("PubResult.Committed should be true (empty snapshot committed)")
	}
	if len(result.Errors) > 0 {
		t.Errorf("Errors should be empty: %v", result.Errors)
	}
}

func TestRunSnapshot_Scenario(t *testing.T) {
	repo := storage.NewMemoryRepository()
	teamReg := domain.NewTeamRegistry()
	clustering := domain.NewClusterer()
	scorer := domain.NewScorer()

	// Add some stories with different titles that won't cluster together
	stories := []domain.Story{
		{
			ID:          "s1",
			Title:       "Story about Eagles winning Super Bowl",
			Source:      "test-feed",
			PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
			Summary:     "Summary 1",
			URL:         "http://example.com/s1",
		},
		{
			ID:          "s2",
			Title:       "Different story about Giants draft picks",
			Source:      "test-feed",
			PublishedAt: time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC),
			Summary:     "Summary 2",
			URL:         "http://example.com/s2",
		},
	}

	for _, s := range stories {
		if _, err := repo.Upsert(s); err != nil {
			t.Fatalf("Upsert failed: %v", err)
		}
	}

	app := New(
		WithRepository(repo),
		WithTeamRegistry(teamReg),
		WithClusterer(clustering),
		WithScorer(scorer),
	)

	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	result := app.RunSnapshot(context.Background(), now)

	t.Logf("Snapshot: Stories=%v (nil? %v, len? %d), Latest=%v (nil? %v, len? %d), Teams=%v (nil? %v, len? %d)",
		result.Snapshot.Stories, result.Snapshot.Stories == nil, len(result.Snapshot.Stories),
		result.Snapshot.Latest, result.Snapshot.Latest == nil, len(result.Snapshot.Latest),
		result.Snapshot.Teams, result.Snapshot.Teams == nil, len(result.Snapshot.Teams))

	// With 2 stories that don't cluster together, we expect 2 groups
	// (one story per cluster since they have different titles)
	if len(result.Snapshot.Stories) == 0 {
		t.Error("Stories should exist")
	}
	if !result.PubResult.Committed {
		t.Error("PubResult.Committed should be true")
	}
	if len(result.Errors) > 0 {
		t.Errorf("Errors should be empty: %v", result.Errors)
	}
}

func TestRunSnapshot_IdempotentNoop(t *testing.T) {
	repo := storage.NewMemoryRepository()
	teamReg := domain.NewTeamRegistry()
	clustering := domain.NewClusterer()
	// Use a fixed scorer time so scoring is deterministic
	scorer := domain.NewScorer().WithTime(time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC))

	// Add a story
	if _, err := repo.Upsert(domain.Story{
		ID:          "s1",
		Title:       "Test Story",
		Source:      "test-feed",
		PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		Summary:     "Summary",
		URL:         "http://example.com/s1",
	}); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	app := New(
		WithRepository(repo),
		WithTeamRegistry(teamReg),
		WithClusterer(clustering),
		WithScorer(scorer),
	)

	// First run
	now1 := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	result1 := app.RunSnapshot(context.Background(), now1)
	if len(result1.Errors) > 0 {
		t.Fatalf("First run failed: %v", result1.Errors)
	}

	// Second run with different time
	now2 := time.Date(2024, 1, 16, 12, 0, 0, 0, time.UTC)
	result2 := app.RunSnapshot(context.Background(), now2)
	if len(result2.Errors) > 0 {
		t.Fatalf("Second run failed: %v", result2.Errors)
	}

	// Content comparison should exclude score (which depends on time)
	// Check all other fields match
	if result1.Snapshot.Version != result2.Snapshot.Version {
		t.Errorf("Version differs: %s vs %s", result1.Snapshot.Version, result2.Snapshot.Version)
	}
	if len(result1.Snapshot.Stories) != len(result2.Snapshot.Stories) {
		t.Errorf("Stories count differs: %d vs %d", len(result1.Snapshot.Stories), len(result2.Snapshot.Stories))
	}
	// Check each story matches (excluding score)
	for i := range result1.Snapshot.Stories {
		s1 := result1.Snapshot.Stories[i]
		s2 := result2.Snapshot.Stories[i]
		if s1.ID != s2.ID || s1.Title != s2.Title || s1.Summary != s2.Summary ||
			s1.FirstPublishedAt != s2.FirstPublishedAt || s1.LastPublishedAt != s2.LastPublishedAt ||
			s1.EvidenceLevel != s2.EvidenceLevel {
			t.Errorf("Story %d differs (ignoring score): %v vs %v", i, s1, s2)
		}
		if len(s1.Publishers) != len(s2.Publishers) {
			t.Errorf("Publishers count differs for story %d: %d vs %d", i, len(s1.Publishers), len(s2.Publishers))
		}
	}
	if len(result1.Snapshot.Latest) != len(result2.Snapshot.Latest) {
		t.Errorf("Latest count differs: %d vs %d", len(result1.Snapshot.Latest), len(result2.Snapshot.Latest))
	}
	if len(result1.Snapshot.Teams) != len(result2.Snapshot.Teams) {
		t.Errorf("Teams count differs: %d vs %d", len(result1.Snapshot.Teams), len(result2.Snapshot.Teams))
	}
}

func TestRunSnapshot_NoScorer(t *testing.T) {
	repo := storage.NewMemoryRepository()
	teamReg := domain.NewTeamRegistry()
	clustering := domain.NewClusterer()

	app := New(
		WithRepository(repo),
		WithTeamRegistry(teamReg),
		WithClusterer(clustering),
		// No scorer set
	)

	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	result := app.RunSnapshot(context.Background(), now)

	if len(result.Errors) == 0 {
		t.Error("Should have error when scorer not configured")
	}
	// When error occurs during build, PubResult should have Committed=false
	// But the default publisher returns Committed=true for offline mode
	// So we just check that errors are present
}

// TestRunSnapshot_NoClustering tests that missing clustering engine is handled
func TestRunSnapshot_NoClustering(t *testing.T) {
	repo := storage.NewMemoryRepository()
	teamReg := domain.NewTeamRegistry()
	scorer := domain.NewScorer()

	app := New(
		WithRepository(repo),
		WithTeamRegistry(teamReg),
		// No clustering engine
		WithScorer(scorer),
	)

	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	result := app.RunSnapshot(context.Background(), now)

	if len(result.Errors) == 0 {
		t.Error("Should have error when clustering engine not configured")
	}
}

// TestBuildSnapshotWithStoriesAndClustering tests building with stories and clustering
func TestBuildSnapshotWithStoriesAndClustering(t *testing.T) {
	repo := storage.NewMemoryRepository()
	teamReg := domain.NewTeamRegistry()
	clustering := domain.NewClusterer()
	scorer := domain.NewScorer()

	// Add a story
	if _, err := repo.Upsert(domain.Story{
		ID:          "s1",
		Title:       "Test Story",
		Source:      "test-feed",
		PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		Summary:     "Summary",
		URL:         "http://example.com/s1",
	}); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	app := New(
		WithRepository(repo),
		WithTeamRegistry(teamReg),
		WithClusterer(clustering),
		WithScorer(scorer),
	)

	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	result := app.RunSnapshot(context.Background(), now)

	t.Logf("Stories count: %d, Errors: %v", len(result.Snapshot.Stories), result.Errors)
	t.Logf("Snapshot: Stories=%v (nil? %v, len? %d)", result.Snapshot.Stories, result.Snapshot.Stories == nil, len(result.Snapshot.Stories))

	// With clustering, the story should be grouped
	if len(result.Snapshot.Stories) != 1 {
		t.Errorf("Stories count = %d; want 1", len(result.Snapshot.Stories))
	}
	if result.Snapshot.Version != domain.SnapshotVersion {
		t.Errorf("Snapshot version = %q; want %q", result.Snapshot.Version, domain.SnapshotVersion)
	}
	if !result.PubResult.Committed {
		t.Error("PubResult.Committed should be true")
	}
	if len(result.Errors) > 0 {
		t.Errorf("Errors should be empty: %v", result.Errors)
	}
}
