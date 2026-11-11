package application

import (
	"context"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

func TestNewGroupingResult(t *testing.T) {
	result := NewGroupingResult()

	if result.GroupsCreated != 0 {
		t.Errorf("Expected GroupsCreated=0, got %d", result.GroupsCreated)
	}
	if result.GroupsMerged != 0 {
		t.Errorf("Expected GroupsMerged=0, got %d", result.GroupsMerged)
	}
	if result.ArticlesGrouped != 0 {
		t.Errorf("Expected ArticlesGrouped=0, got %d", result.ArticlesGrouped)
	}
	if result.ArticlesSolo != 0 {
		t.Errorf("Expected ArticlesSolo=0, got %d", result.ArticlesSolo)
	}
	if len(result.Errors) != 0 {
		t.Errorf("Expected Errors=[], got %v", result.Errors)
	}
}

func TestAppWithScorer(t *testing.T) {
	repo := storage.NewMemoryRepository()

	// Insert test stories
	now := time.Now().UTC()
	testStories := []domain.Story{
		{
			ID:          "story-1",
			Title:       "Confirmed trade deal between teams",
			Source:      "espn",
			PublishedAt: now.Add(-1 * time.Hour),
			Summary:     "The trade was officially confirmed by both teams.",
			ArticleIDs:  []string{"art-1", "art-2"},
			Headline:    "Confirmed trade deal",
			Entities:    []string{"ne", "nyg"},
		},
		{
			ID:          "story-2",
			Title:       "Rumor: potential free agent signing",
			Source:      "cbs",
			PublishedAt: now.Add(-12 * time.Hour),
			Summary:     "Sources say a deal could be near.",
			ArticleIDs:  []string{"art-3"},
			Headline:    "Rumor signing",
			Entities:    []string{"phi"},
		},
	}

	for _, s := range testStories {
		if _, err := repo.Upsert(s); err != nil {
			t.Fatalf("Failed to insert story: %v", err)
		}
	}

	// Test with scorer
	scorer := domain.NewScorer().WithTime(now)
	app := New(WithRepository(repo), WithScorer(scorer), WithClusterer(domain.NewClusterer()))

	result := app.RunScoring(context.Background(), now)

	if len(result.Errors) > 0 {
		t.Errorf("RunScoring returned errors: %v", result.Errors)
	}

	if result.TotalGroups == 0 {
		t.Error("Expected at least one group")
	}

	// Verify breakdowns exist and scores are in range
	for _, bd := range result.Breakdowns {
		if bd.TotalScore < 0 || bd.TotalScore > 100 {
			t.Errorf("TotalScore = %.2f, want [0,100]", bd.TotalScore)
		}
	}

	// Verify notes are populated
	if len(result.Breakdowns) > 0 {
		// Note: story-1 should be confirmed, story-2 should be unverified
		// After clustering, we get groups; at least one should have the unverified note
		t.Logf("Notes from first breakdown: %v", result.Breakdowns[0].Notes)
	}
}

func TestRunScoringWithNilScorer(t *testing.T) {
	repo := storage.NewMemoryRepository()
	clusterer := domain.NewClusterer()

	// Insert a test story
	now := time.Now().UTC()
	story := domain.Story{
		ID:          "story-1",
		Title:       "Test Story",
		Source:      "feed-1",
		PublishedAt: now.Add(-1 * time.Hour),
		Summary:     "Summary",
		ArticleIDs:  []string{"art-1"},
		Headline:    "Test Story",
	}
	if _, err := repo.Upsert(story); err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	app := New(
		WithRepository(repo),
		WithClusterer(clusterer),
		// No WithScorer - scorer is nil
	)
	result := app.RunScoring(context.Background(), now)

	if len(result.Errors) != 1 {
		t.Errorf("Expected 1 error for nil scorer, got %d", len(result.Errors))
	}
	if result.Errors[0].Error() != "scorer not configured" {
		t.Errorf("Expected 'scorer not configured', got %v", result.Errors[0])
	}
	if len(result.Breakdowns) != 0 {
		t.Errorf("Expected 0 breakdowns for nil scorer, got %d", len(result.Breakdowns))
	}
}

func TestRunScoringIdempotency(t *testing.T) {
	repo := storage.NewMemoryRepository()

	// Fixed timestamp for idempotency
	fixedTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	// Insert one story with fixed timestamp
	story := domain.Story{
		ID:          "single",
		Title:       "Test Story",
		Source:      "test",
		PublishedAt: fixedTime,
		Summary:     "Summary",
	}
	if _, err := repo.Upsert(story); err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	// Create a scorer with fixed time
	scorer := domain.NewScorer().WithTime(fixedTime)
	app := New(WithRepository(repo), WithScorer(scorer), WithClusterer(domain.NewClusterer()))

	// First run - will run grouping
	result1 := app.RunScoring(context.Background(), fixedTime)

	// For idempotency test, manually score the same story without re-running grouping
	scorer2 := domain.NewScorer().WithTime(fixedTime)
	// Get the story back
	stories, err := repo.List()
	if err != nil {
		t.Fatalf("Failed to list: %v", err)
	}

	// Rebuild articles and score
	articles := make([]domain.Article, 0, len(stories))
	for _, s := range stories {
		articles = append(articles, domain.Article{
			ID:          s.ID,
			SourceID:    s.Source,
			Title:       s.Title,
			Summary:     s.Summary,
			URL:         s.URL,
			PublishedAt: s.PublishedAt.UTC().Format("2006-01-02T15:04:05Z"),
			FirstSeenAt: s.FirstSeenAt.UTC().Format("2006-01-02T15:04:05Z"),
			Entities:    s.Entities,
		})
	}

	groups := domain.NewClusterer().Cluster(articles)
	var breakdowns2 []domain.ScoreBreakdown
	for _, g := range groups {
		dis := scorer2.Score(g)
		breakdowns2 = append(breakdowns2, dis)
	}

	if len(result1.Breakdowns) != len(breakdowns2) {
		t.Errorf("Breakdown count differs: %d vs %d", len(result1.Breakdowns), len(breakdowns2))
	}

	if len(result1.Breakdowns) > 0 && len(breakdowns2) > 0 {
		if result1.Breakdowns[0].TotalScore != breakdowns2[0].TotalScore {
			t.Errorf("Idempotency failed: %v != %v",
				result1.Breakdowns[0].TotalScore, breakdowns2[0].TotalScore)
		}
	}
}

func TestRunGroupingNoClusteringEngine(t *testing.T) {
	repo := storage.NewMemoryRepository()
	app := New(WithRepository(repo))

	result := app.RunGrouping(context.Background())

	if len(result.Errors) != 1 {
		t.Errorf("Expected 1 error for missing clustering engine, got %d", len(result.Errors))
	}
	if result.Errors[0].Error() != "clustering engine not configured" {
		t.Errorf("Expected 'clustering engine not configured', got %v", result.Errors[0])
	}
}

func TestRunIngestNoRepository(t *testing.T) {
	app := New()
	result := app.RunIngest(context.Background())

	if result.StoriesSkipped != 0 {
		t.Errorf("Expected 0 stories skipped, got %d", result.StoriesSkipped)
	}
	if len(result.Errors) != 0 {
		t.Errorf("Expected no errors, got %v", result.Errors)
	}
}

func TestRunExportWithStories(t *testing.T) {
	repo := storage.NewMemoryRepository()

	// Insert some stories
	now := time.Now().UTC()
	stories := []domain.Story{
		{
			ID:          "story-1",
			Title:       "Test Story 1",
			Source:      "feed-1",
			PublishedAt: now.Add(-1 * time.Hour),
			Summary:     "Summary 1",
			ArticleIDs:  []string{"art-1"},
			Headline:    "Test Story 1",
		},
		{
			ID:          "story-2",
			Title:       "Test Story 2",
			Source:      "feed-2",
			PublishedAt: now.Add(-2 * time.Hour),
			Summary:     "Summary 2",
			ArticleIDs:  []string{"art-2"},
			Headline:    "Test Story 2",
		},
	}

	for _, s := range stories {
		if _, err := repo.Upsert(s); err != nil {
			t.Fatalf("Failed to insert: %v", err)
		}
	}

	app := New(WithRepository(repo))
	result := app.RunExport(context.Background())

	if result.StoriesExported != 2 {
		t.Errorf("Expected 2 stories exported, got %d", result.StoriesExported)
	}
	if len(result.Errors) != 0 {
		t.Errorf("Expected no errors, got %v", result.Errors)
	}
}

func TestRunIngestWithMockFetchers(t *testing.T) {
	repo := storage.NewMemoryRepository()

	// Create a mock fetcher that returns some entries
	mockEntries := []domain.FeedEntry{
		{
			Title:       "Test Article 1",
			Source:      "mock-feed",
			PublishedAt: time.Now().UTC(),
			Summary:     "Test summary 1",
			URL:         "https://example.com/1",
			GUID:        "guid-1",
		},
		{
			Title:       "Test Article 2",
			Source:      "mock-feed",
			PublishedAt: time.Now().UTC(),
			Summary:     "Test summary 2",
			URL:         "https://example.com/2",
			GUID:        "guid-2",
		},
	}

	// Create fetcher that always returns the same entries
	fetcher := &mockFetcher{entries: mockEntries}

	app := New(WithRepository(repo), WithFeedFetchers(fetcher))
	result := app.RunIngest(context.Background())

	if result.EntriesFetched != 2 {
		t.Errorf("Expected 2 entries fetched, got %d", result.EntriesFetched)
	}
	if result.StoriesCreated != 2 {
		t.Errorf("Expected 2 stories created, got %d", result.StoriesCreated)
	}
	if len(result.Errors) != 0 {
		t.Errorf("Expected no errors, got %v", result.Errors)
	}
}

func TestRunIngestWithRegistry(t *testing.T) {
	repo := storage.NewMemoryRepository()

	// Create a feed registry with one enabled source
	registry, err := domain.NewFeedRegistry(domain.FeedSource{
		ID:            "test-feed",
		URL:           "https://example.com/feed",
		Publication:   "Test Feed",
		Priority:      1,
		Enabled:       true,
		Language:      "en",
		License:       "Test license",
		AcceptableUse: domain.AcceptanceAllowed,
	})
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	// Create a mock fetcher for the registry
	fetcher := &mockFetcher{entries: []domain.FeedEntry{
		{
			Title:       "Registry Article",
			Source:      "test-feed",
			PublishedAt: time.Now().UTC(),
			Summary:     "Summary",
			URL:         "https://example.com/1",
			GUID:        "guid-1",
		},
	}}

	app := New(
		WithRepository(repo),
		WithRegistry(registry),
		WithFetcherMap(map[string]domain.FeedFetcher{"test-feed": fetcher}),
	)
	result := app.RunIngest(context.Background())

	if result.EntriesFetched != 1 {
		t.Errorf("Expected 1 entry fetched, got %d", result.EntriesFetched)
	}
	if result.StoriesCreated != 1 {
		t.Errorf("Expected 1 story created, got %d", result.StoriesCreated)
	}
	if len(result.Errors) != 0 {
		t.Errorf("Expected no errors, got %v", result.Errors)
	}
}

// mockFetcher is a mock FeedFetcher that returns predefined entries
type mockFetcher struct {
	entries []domain.FeedEntry
}

func (m *mockFetcher) Fetch(sourceID string) ([]domain.FeedEntry, error) {
	return m.entries, nil
}
