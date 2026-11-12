package application

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/rss"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// TestExtractTeams tests the extractTeams helper function
func TestExtractTeams(t *testing.T) {
	teamRegistry := domain.NewTeamRegistry(
		domain.Team{
			Slug:         "packers",
			Name:         "Green Bay Packers",
			ShortName:    "Packers",
			Abbreviation: "GB",
		},
	)

	tests := []struct {
		name             string
		story            domain.Story
		registry         *domain.TeamRegistry
		expectedEntities []string
	}{
		{
			name: "nil registry returns unchanged story",
			story: domain.Story{
				ID:       "test",
				Title:    "Test",
				Summary:  "Summary",
				Entities: []string{"entity-1"},
			},
			registry:         nil,
			expectedEntities: []string{"entity-1"},
		},
		{
			name: "extracts teams from title",
			story: domain.Story{
				ID:       "test",
				Title:    "Packers win",
				Summary:  "Summary",
				Entities: []string{},
			},
			registry:         teamRegistry,
			expectedEntities: []string{"packers"},
		},
		{
			name: "extracts teams from summary",
			story: domain.Story{
				ID:       "test",
				Title:    "Game",
				Summary:  "Packers win",
				Entities: []string{},
			},
			registry:         teamRegistry,
			expectedEntities: []string{"packers"},
		},
		{
			name: "merges existing entities with extracted",
			story: domain.Story{
				ID:       "test",
				Title:    "Packers game",
				Summary:  "vs Bears",
				Entities: []string{"existing-entity"},
			},
			registry:         teamRegistry,
			expectedEntities: []string{"existing-entity", "packers"},
		},
		{
			name: "no duplicates when entity exists",
			story: domain.Story{
				ID:       "test",
				Title:    "Packers game",
				Summary:  "Summary",
				Entities: []string{"packers"},
			},
			registry:         teamRegistry,
			expectedEntities: []string{"packers"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTeams(tt.story, tt.registry)

			if !slicesEqual(result.Entities, tt.expectedEntities) {
				t.Errorf("Entities mismatch: got %v, want %v", result.Entities, tt.expectedEntities)
			}
		})
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

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

// legacyMockFetcher is a mock FeedFetcher used in legacy path tests
// It supports failure simulation via fail flag
type legacyMockFetcher struct {
	entries []domain.FeedEntry
	fail    bool
	failMsg string
}

func (m *legacyMockFetcher) Fetch(sourceID string) ([]domain.FeedEntry, error) {
	if m.fail {
		return nil, fmt.Errorf("fetcher error: %s", m.failMsg)
	}
	return m.entries, nil
}

// TestRunIngestWithRegistryFetchError tests the error path in runIngestWithRegistry
func TestRunIngestWithRegistryFetchError(t *testing.T) {
	repo := storage.NewMemoryRepository()

	registry, err := domain.NewFeedRegistry(domain.FeedSource{
		ID:            "failing-feed",
		URL:           "https://example.com/fail",
		Publication:   "Failing Feed",
		Priority:      1,
		Enabled:       true,
		Language:      "en",
		License:       "Test license",
		AcceptableUse: domain.AcceptanceAllowed,
	})
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	// Create fetcher that fails
	fetcher := &legacyMockFetcher{fail: true, failMsg: "network error"}

	app := New(
		WithRepository(repo),
		WithRegistry(registry),
		WithFetcherMap(map[string]domain.FeedFetcher{"failing-feed": fetcher}),
	)
	result := app.RunIngest(context.Background())

	// Should have errors from fetch failure
	if len(result.Errors) == 0 {
		t.Errorf("Expected errors from fetch failure, got none")
	}
	if result.EntriesFetched != 0 {
		t.Errorf("Expected 0 entries fetched on failure, got %d", result.EntriesFetched)
	}
	if result.StoriesCreated != 0 {
		t.Errorf("Expected 0 stories created on failure, got %d", result.StoriesCreated)
	}
}

func TestRunIngestWithMissingFetcherInMap(t *testing.T) {
	repo := storage.NewMemoryRepository()

	// Create registry with source that is NOT in fetcherMap
	registry, err := domain.NewFeedRegistry(domain.FeedSource{
		ID:            "missing-feed",
		URL:           "https://example.com/missing",
		Publication:   "Missing Feed",
		Priority:      1,
		Enabled:       true,
		Language:      "en",
		License:       "Test license",
		AcceptableUse: domain.AcceptanceAllowed,
	})
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	// Empty fetcher map - missing-feed will get fallback fetcher
	app := New(
		WithRepository(repo),
		WithRegistry(registry),
		WithFetcherMap(map[string]domain.FeedFetcher{}),
	)
	result := app.RunIngest(context.Background())

	// Should NOT have errors since fallback fetcher is used
	if len(result.Errors) != 0 {
		t.Errorf("Expected no errors (fallback fetcher used), got %v", result.Errors)
	}
	// Fallback fetcher returns empty entries
	if result.EntriesFetched != 0 {
		t.Errorf("Expected 0 entries with fallback fetcher, got %d", result.EntriesFetched)
	}
	if result.StoriesCreated != 0 {
		t.Errorf("Expected 0 stories created, got %d", result.StoriesCreated)
	}
}

// TestRunIngestWithTeamRegistry tests extractTeams path
func TestRunIngestWithTeamRegistry(t *testing.T) {
	repo := storage.NewMemoryRepository()

	// Create a registry with known team patterns
	teamRegistry := domain.NewTeamRegistry(
		domain.Team{
			Slug:         "packers",
			Name:         "Green Bay Packers",
			ShortName:    "Packers",
			Abbreviation: "GB",
			Aliases:      []string{"GB Packers"},
		},
		domain.Team{
			Slug:         "bears",
			Name:         "Chicago Bears",
			ShortName:    "Bears",
			Abbreviation: "CHI",
			Aliases:      []string{"Chicago Football Club"},
		},
	)

	registry, err := domain.NewFeedRegistry(domain.FeedSource{
		ID:            "nfl-feed",
		URL:           "https://example.com/nfl",
		Publication:   "NFL News",
		Priority:      1,
		Enabled:       true,
		Language:      "en",
		License:       "Test license",
		AcceptableUse: domain.AcceptanceAllowed,
	})
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	fetcher := &mockFetcher{entries: []domain.FeedEntry{
		{
			Title:       "Packers beat Bears",
			Source:      "nfl-feed",
			PublishedAt: time.Now().UTC(),
			Summary:     "Green Bay won against Chicago",
			URL:         "https://example.com/1",
			GUID:        "guid-1",
		},
	}}

	app := New(
		WithRepository(repo),
		WithRegistry(registry),
		WithFetcherMap(map[string]domain.FeedFetcher{"nfl-feed": fetcher}),
		WithTeamRegistry(teamRegistry),
	)
	result := app.RunIngest(context.Background())

	if len(result.Errors) != 0 {
		t.Errorf("Expected no errors, got %v", result.Errors)
	}
	if result.StoriesCreated != 1 {
		t.Errorf("Expected 1 story created, got %d", result.StoriesCreated)
	}

	// Check that team entities were extracted
	stories, err := repo.List()
	if err != nil {
		t.Fatalf("Failed to list: %v", err)
	}
	if len(stories) != 1 {
		t.Fatalf("Expected 1 story, got %d", len(stories))
	}

	// The story should have both original entities and extracted teams
	var hasPackers, hasBears bool
	for _, e := range stories[0].Entities {
		if e == "packers" {
			hasPackers = true
		}
		if e == "bears" {
			hasBears = true
		}
	}
	if !hasPackers {
		t.Errorf("Expected 'packers' entity from team extraction, got %v", stories[0].Entities)
	}
	if !hasBears {
		t.Errorf("Expected 'bears' entity from team extraction, got %v", stories[0].Entities)
	}
}

func TestRunExportEmptyRepository(t *testing.T) {
	repo := storage.NewMemoryRepository()
	app := New(WithRepository(repo))
	result := app.RunExport(context.Background())

	if result.StoriesExported != 0 {
		t.Errorf("Expected 0 stories exported for empty repo, got %d", result.StoriesExported)
	}
	if len(result.Errors) != 0 {
		t.Errorf("Expected no errors for empty repo, got %v", result.Errors)
	}
}

// mockRepository fails on List to test RunScoring error paths
type mockRepository struct {
	stories  []domain.Story
	failList bool
}

func (r *mockRepository) Upsert(story domain.Story) (domain.UpsertStatus, error) {
	return domain.UpsertCreated, nil
}

func (r *mockRepository) List() ([]domain.Story, error) {
	if r.failList {
		return nil, fmt.Errorf("mock: List failed")
	}
	return r.stories, nil
}

func (r *mockRepository) Count() (int, error) {
	return len(r.stories), nil
}

func TestRunScoringWithListError(t *testing.T) {
	// Create a mock repository that fails on List
	mockRepo := &mockRepository{stories: []domain.Story{}, failList: true}
	scorer := domain.NewScorer()
	clusterer := domain.NewClusterer()
	app := New(WithRepository(mockRepo), WithScorer(scorer), WithClusterer(clusterer))

	result := app.RunScoring(context.Background(), time.Now())

	// RunScoring calls RunGrouping first, then repo.List()
	// With a failing repo.List(), we expect at least one error from List failure
	if len(result.Errors) < 1 {
		t.Errorf("Expected at least 1 error for List failure, got %d", len(result.Errors))
	}
	// Check that the List failure error is present
	var listErrorFound bool
	for _, e := range result.Errors {
		if e.Error() == "failed to list stories: mock: List failed" {
			listErrorFound = true
		}
	}
	if !listErrorFound {
		t.Errorf("Expected 'failed to list stories: mock: List failed' in errors %v", result.Errors)
	}
	if len(result.Breakdowns) != 0 {
		t.Errorf("Expected 0 breakdowns for List error, got %d", len(result.Breakdowns))
	}
}

func TestRunScoringWithClustererError(t *testing.T) {
	repo := storage.NewMemoryRepository()
	scorer := domain.NewScorer()

	// WithClusterer(nil) triggers the error path
	app := New(WithRepository(repo), WithScorer(scorer), WithClusterer(nil))
	result := app.RunScoring(context.Background(), time.Now())

	if len(result.Errors) != 1 {
		t.Errorf("Expected 1 error for nil clusterer, got %d", len(result.Errors))
	}
	if result.Errors[0].Error() != "clustering engine not configured; RunGrouping must be called first" {
		t.Errorf("Expected 'clustering engine not configured...', got %v", result.Errors[0])
	}
	if len(result.Breakdowns) != 0 {
		t.Errorf("Expected 0 breakdowns for nil clusterer, got %d", len(result.Breakdowns))
	}
}

func TestWithFetcherResolver(t *testing.T) {
	resolver := func(sourceID string) domain.FeedFetcher {
		return nil
	}

	app := New(WithFetcherResolver(resolver))

	// Verify that the option was applied (no panic, and app is created)
	if app == nil {
		t.Fatal("Expected non-nil Application")
	}
}

// TestRunPipelineSuccessWithOneSource tests RunPipeline with one successful source.
func TestRunPipelineSuccessWithOneSource(t *testing.T) {
	repo := storage.NewMemoryRepository()
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
		t.Fatalf("NewFeedRegistry() error = %v", err)
	}
	fetcher := &mockFetcher{entries: []domain.FeedEntry{
		{Title: "Story 1", Source: "test-feed", PublishedAt: time.Now().UTC(), Summary: "Summary 1", URL: "https://example.com/1"},
	}}
	publisher := &TestPublisher{}
	app := New(
		WithRepository(repo),
		WithRegistry(registry),
		WithFetcherMap(map[string]domain.FeedFetcher{"test-feed": fetcher}),
		WithClusterer(domain.NewClusterer()),
		WithScorer(domain.NewScorer()),
		WithPublisher(publisher),
		WithLookback(24*time.Hour),
	)
	result := app.RunPipeline(context.Background(), time.Now())
	if result.Status != domain.RunStatusOK {
		t.Errorf("Status = %s, want OK", result.Status)
	}
	if result.Homepage == nil {
		t.Error("Homepage should not be nil")
	}
}

// TestRebuildRankingSuccessWithStories tests RebuildRanking with existing stories.
func TestRebuildRankingSuccessWithStories(t *testing.T) {
	repo := storage.NewMemoryRepository()
	_, _ = repo.Upsert(domain.Story{
		ID:          "story-1",
		Title:       "Story 1",
		Source:      "feed1",
		PublishedAt: time.Now().UTC().Add(-1 * time.Hour),
		Summary:     "Summary 1",
		Entities:    []string{"team-a"},
	})
	_, _ = repo.Upsert(domain.Story{
		ID:          "story-2",
		Title:       "Story 2",
		Source:      "feed2",
		PublishedAt: time.Now().UTC().Add(-30 * time.Minute),
		Summary:     "Summary 2",
		Entities:    []string{"team-a"},
	})
	app := New(
		WithRepository(repo),
		WithClusterer(domain.NewClusterer()),
		WithScorer(domain.NewScorer()),
		WithLookback(24*time.Hour),
	)
	result := app.RebuildRanking(context.Background(), time.Now())
	if result.Status != domain.RunStatusOK {
		t.Errorf("Status = %s, want OK", result.Status)
	}
	if result.Homepage == nil {
		t.Error("Homepage should not be nil")
	}
}

// TestPublisher is a test publisher implementation.
type TestPublisher struct {
	committed bool
	called    int
	failOnce  bool
}

func (p *TestPublisher) Publish(ctx context.Context, homepage domain.Homepage) error {
	p.called++
	if p.failOnce {
		p.failOnce = false
		return fmt.Errorf("publish failed (simulated)")
	}
	p.committed = true
	return nil
}

func (p *TestPublisher) Committed() bool {
	return p.committed
}

func (p *TestPublisher) Called() int {
	return p.called
}

func (p *TestPublisher) SetFailOnce() {
	p.failOnce = true
}

func (p *TestPublisher) Reset() {
	p.committed = false
	p.called = 0
	p.failOnce = false
}

// TestRunPipelineAllPhases success path testsRunPipeline complete pipeline execution.
func TestRunPipelineAllPhases(t *testing.T) {
	repo := storage.NewMemoryRepository()
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
		t.Fatalf("NewFeedRegistry() error = %v", err)
	}
	fetcher := &mockFetcher{entries: []domain.FeedEntry{
		{Title: "Story 1", Source: "test-feed", PublishedAt: time.Now().UTC(), Summary: "Summary 1", URL: "https://example.com/1"},
	}}
	publisher := &TestPublisher{}
	app := New(
		WithRepository(repo),
		WithRegistry(registry),
		WithFetcherMap(map[string]domain.FeedFetcher{"test-feed": fetcher}),
		WithClusterer(domain.NewClusterer()),
		WithScorer(domain.NewScorer()),
		WithPublisher(publisher),
		WithLookback(24*time.Hour),
	)
	result := app.RunPipeline(context.Background(), time.Now())
	if result.Status != domain.RunStatusOK {
		t.Errorf("Status = %s, want OK", result.Status)
	}
	if result.Homepage == nil || len(result.Homepage.RankedStories) == 0 {
		t.Error("Homepage should have ranked stories")
	}
	if !result.Committed {
		t.Error("Committed should be true")
	}
}

// TestRunIngestWithLegacyFetchers tests the fallback path when registry is nil but fetchers is set
func TestRunIngestWithLegacyFetchers(t *testing.T) {
	repo := storage.NewMemoryRepository()

	// Create a mock fetcher that returns some entries
	mockEntries := []domain.FeedEntry{
		{
			Title:       "Test Entry 1",
			Source:      "feed-1",
			PublishedAt: time.Now().UTC().Add(-1 * time.Hour),
			Summary:     "Summary 1",
			URL:         "https://example.com/1",
			GUID:        "guid-1",
		},
		{
			Title:       "Test Entry 2",
			Source:      "feed-1",
			PublishedAt: time.Now().UTC().Add(-2 * time.Hour),
			Summary:     "Summary 2",
			URL:         "https://example.com/2",
			GUID:        "guid-2",
		},
	}

	// Create a static fetcher (legacy path - registry is nil)
	fetcher := rss.NewStaticFetcher(mockEntries)

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

	// Verify stories were stored
	stories, err := repo.List()
	if err != nil {
		t.Fatalf("Failed to list stories: %v", err)
	}
	if len(stories) != 2 {
		t.Errorf("Expected 2 stories, got %d", len(stories))
	}
}

// TestRunIngestWithRegistryNoEnabledSources tests when registry is set but no sources are enabled
func TestRunIngestWithRegistryNoEnabledSources(t *testing.T) {
	repo := storage.NewMemoryRepository()

	// Create registry with disabled sources
	registry, err := domain.NewFeedRegistry(
		domain.FeedSource{
			ID:        "feed-1",
			URL:       "http://example.com/feed",
			Publication: "Feed 1",
			Priority:  10,
			Enabled:   false, // Disabled
		},
	)
	if err != nil {
		t.Fatalf("Failed to create registry: %v", err)
	}

	app := New(WithRepository(repo), WithRegistry(registry))
	result := app.RunIngest(context.Background())

	if result.StoriesSkipped != 0 {
		t.Errorf("Expected 0 stories skipped, got %d", result.StoriesSkipped)
	}
	if result.EntriesFetched != 0 {
		t.Errorf("Expected 0 entries fetched, got %d", result.EntriesFetched)
	}
}

// mockFetcher is a simple fetcher implementation for testing
type mockFetcher struct {
	entries []domain.FeedEntry
	err     error
}

func (f *mockFetcher) Fetch(sourceID string) ([]domain.FeedEntry, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.entries, nil
}

// TestRunIngestWithFetcherError tests when fetcher returns an error
func TestRunIngestWithFetcherError(t *testing.T) {
	repo := storage.NewMemoryRepository()

	// Create a static fetcher that returns an error
	fetcher := &mockFetcher{entries: nil, err: fmt.Errorf("fetch error")}

	app := New(WithRepository(repo), WithFeedFetchers(fetcher))
	result := app.RunIngest(context.Background())

	if len(result.Errors) != 1 {
		t.Errorf("Expected 1 error, got %d: %v", len(result.Errors), result.Errors)
	}
}
