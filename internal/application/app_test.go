package application

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/rss"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

func TestNewApplication(t *testing.T) {
	app := New()
	if app == nil {
		t.Fatal("New() returned nil")
	}
}

func TestWithRepository(t *testing.T) {
	repo := &mockRepository{}
	_ = New(WithRepository(repo))
	// Can't directly check app.repo since it's unexported, but we can test via RunIngest behavior
}

func TestWithFeedFetchers(t *testing.T) {
	fetcher := rss.NewStaticFetcher([]domain.FeedEntry{
		{Title: "test", Source: "test", PublishedAt: time.Now(), Summary: "test", URL: "https://test.com"},
	})
	app := New(WithFeedFetchers(fetcher))
	if len(app.fetchers) != 1 {
		t.Errorf("fetchers length = %d; want 1", len(app.fetchers))
	}
}

func TestRunIngestNoRepository(t *testing.T) {
	app := New()
	result := app.RunIngest(context.Background())
	if result.EntriesFetched != 0 || result.StoriesCreated != 0 {
		t.Errorf("expected no work, got entries=%d stories=%d", result.EntriesFetched, result.StoriesCreated)
	}
}

func TestRunIngestNoFetchers(t *testing.T) {
	repo := &mockRepository{}
	app := New(WithRepository(repo))
	result := app.RunIngest(context.Background())
	if result.EntriesFetched != 0 || result.StoriesCreated != 0 {
		t.Errorf("expected no work, got entries=%d stories=%d", result.EntriesFetched, result.StoriesCreated)
	}
}

func TestRunIngestWithFetchers(t *testing.T) {
	repo := &mockRepository{}
	fetcher := rss.NewStaticFetcher([]domain.FeedEntry{
		{Title: "test story", Source: "test", PublishedAt: time.Now(), Summary: "test", URL: "https://test.com"},
	})
	app := New(WithRepository(repo), WithFeedFetchers(fetcher))
	result := app.RunIngest(context.Background())
	if result.EntriesFetched != 1 {
		t.Errorf("entries fetched = %d; want 1", result.EntriesFetched)
	}
	if result.StoriesCreated != 1 {
		t.Errorf("stories created = %d; want 1", result.StoriesCreated)
	}
}

func TestRunIngestFailureIsolation(t *testing.T) {
	// Test that one failing source doesn't block others
	repo := &mockRepository{}
	fetcher1 := rss.NewStaticFetcher([]domain.FeedEntry{
		{Title: "story1", Source: "feed1", PublishedAt: time.Now(), Summary: "s1", URL: "https://f1.com"},
	})
	fetcher2 := &failingFetcher{err: fmt.Errorf("fetcher2 failed")}
	fetcher3 := rss.NewStaticFetcher([]domain.FeedEntry{
		{Title: "story3", Source: "feed3", PublishedAt: time.Now(), Summary: "s3", URL: "https://f3.com"},
	})

	app := New(WithRepository(repo), WithFeedFetchers(fetcher1, fetcher2, fetcher3))
	result := app.RunIngest(context.Background())

	// Should have 2 entries (feed1 and feed3 succeeded), feed2 failed
	if result.EntriesFetched != 2 {
		t.Errorf("entries fetched = %d; want 2 (feed1 and feed3)", result.EntriesFetched)
	}
	if result.StoriesCreated != 2 {
		t.Errorf("stories created = %d; want 2", result.StoriesCreated)
	}
	if len(result.Errors) != 1 {
		t.Errorf("errors = %d; want 1 (feed2)", len(result.Errors))
	}
	if result.Errors[0].Error() != "fetcher2 failed" {
		t.Errorf("first error = %v; want \"fetcher2 failed\"", result.Errors[0])
	}
}

// mockRepository implements domain.Repository for testing.
type mockRepository struct{}

func (r *mockRepository) Upsert(story domain.Story) (domain.UpsertStatus, error) {
	return domain.UpsertCreated, nil
}

func (r *mockRepository) List() ([]domain.Story, error) {
	return nil, nil
}

func (r *mockRepository) Count() (int, error) {
	return 0, nil
}

// failingFetcher always returns an error for Fetch.
type failingFetcher struct {
	err error
}

func (f *failingFetcher) Fetch(sourceID string) ([]domain.FeedEntry, error) {
	return nil, f.err
}

func TestRunIngestWithRegistry(t *testing.T) {
	// Create a registry with two sources
	sources := []domain.FeedSource{
		{ID: "feed1", URL: "https://f1.com", Publication: "Feed 1", Priority: 1, Enabled: true},
		{ID: "feed2", URL: "https://f2.com", Publication: "Feed 2", Priority: 2, Enabled: false}, // disabled
	}
	registry, err := domain.NewFeedRegistry(sources...)
	if err != nil {
		t.Fatalf("NewFeedRegistry error = %v", err)
	}

	repo := &mockRepository{}
	// Create fetchers for enabled sources
	fetcherMap := map[string]domain.FeedFetcher{
		"feed1": rss.NewStaticFetcher([]domain.FeedEntry{
			{Title: "story1", Source: "feed1", PublishedAt: time.Now(), Summary: "s1", URL: "https://f1.com"},
		}),
	}

	app := New(WithRepository(repo), WithRegistry(registry), WithFetcherMap(fetcherMap))
	result := app.RunIngest(context.Background())

	// Should only fetch feed1 (feed2 is disabled)
	if result.EntriesFetched != 1 {
		t.Errorf("entries fetched = %d; want 1 (feed1 only)", result.EntriesFetched)
	}
	if result.StoriesCreated != 1 {
		t.Errorf("stories created = %d; want 1", result.StoriesCreated)
	}

	// Verify status was recorded
	status := registry.Status("feed1")
	if !status.LastOK {
		t.Error("feed1 status LastOK = false; want true")
	}
}

func TestRunIngestRegistryNoEnabledSources(t *testing.T) {
	// Create a registry with only disabled sources
	sources := []domain.FeedSource{
		{ID: "feed1", URL: "https://f1.com", Publication: "Feed 1", Priority: 1, Enabled: false},
		{ID: "feed2", URL: "https://f2.com", Publication: "Feed 2", Priority: 2, Enabled: false},
	}
	registry, err := domain.NewFeedRegistry(sources...)
	if err != nil {
		t.Fatalf("NewFeedRegistry error = %v", err)
	}

	repo := &mockRepository{}
	app := New(WithRepository(repo), WithRegistry(registry))
	result := app.RunIngest(context.Background())

	if result.EntriesFetched != 0 || result.StoriesCreated != 0 {
		t.Errorf("expected no work with no enabled sources, got entries=%d stories=%d", result.EntriesFetched, result.StoriesCreated)
	}
}

func TestRunIngestRegistryFetchFailure(t *testing.T) {
	// Create a registry with a source that has a failing fetcher
	sources := []domain.FeedSource{
		{ID: "feed1", URL: "https://f1.com", Publication: "Feed 1", Priority: 1, Enabled: true},
	}
	registry, err := domain.NewFeedRegistry(sources...)
	if err != nil {
		t.Fatalf("NewFeedRegistry error = %v", err)
	}

	repo := &mockRepository{}
	fetcherMap := map[string]domain.FeedFetcher{
		"feed1": &failingFetcher{err: fmt.Errorf("network error")},
	}

	app := New(WithRepository(repo), WithRegistry(registry), WithFetcherMap(fetcherMap))
	result := app.RunIngest(context.Background())

	if result.EntriesFetched != 0 {
		t.Errorf("entries fetched = %d; want 0 (fetch failed)", result.EntriesFetched)
	}
	if result.StoriesCreated != 0 {
		t.Errorf("stories created = %d; want 0", result.StoriesCreated)
	}

	// One error expected
	if len(result.Errors) != 1 {
		t.Errorf("errors = %d; want 1", len(result.Errors))
	}

	// Status should record the failure
	status := registry.Status("feed1")
	if status.LastOK {
		t.Error("status LastOK = true; want false")
	}
	if status.LastError == nil {
		t.Error("status LastError = nil; want non-nil")
	}
}

func TestRunIngestRegistryPerSourceIsolation(t *testing.T) {
	// Create a registry with two enabled sources
	sources := []domain.FeedSource{
		{ID: "feed1", URL: "https://f1.com", Publication: "Feed 1", Priority: 1, Enabled: true},
		{ID: "feed2", URL: "https://f2.com", Publication: "Feed 2", Priority: 2, Enabled: true},
	}
	registry, err := domain.NewFeedRegistry(sources...)
	if err != nil {
		t.Fatalf("NewFeedRegistry error = %v", err)
	}

	repo := &mockRepository{}
	fetcherMap := map[string]domain.FeedFetcher{
		"feed1": rss.NewStaticFetcher([]domain.FeedEntry{
			{Title: "story1", Source: "feed1", PublishedAt: time.Now(), Summary: "s1", URL: "https://f1.com"},
		}),
		"feed2": &failingFetcher{err: fmt.Errorf("feed2 failed")},
	}

	app := New(WithRepository(repo), WithRegistry(registry), WithFetcherMap(fetcherMap))
	result := app.RunIngest(context.Background())

	// feed1 succeeded, feed2 failed
	if result.EntriesFetched != 1 {
		t.Errorf("entries fetched = %d; want 1 (feed1 only)", result.EntriesFetched)
	}
	if result.StoriesCreated != 1 {
		t.Errorf("stories created = %d; want 1", result.StoriesCreated)
	}

	// One error for feed2
	if len(result.Errors) != 1 {
		t.Errorf("errors = %d; want 1 (feed2 only)", len(result.Errors))
	}

	// feed1 status should be success
	status1 := registry.Status("feed1")
	if !status1.LastOK {
		t.Error("feed1 status LastOK = false; want true")
	}

	// feed2 status should be failure
	status2 := registry.Status("feed2")
	if status2.LastOK {
		t.Error("feed2 status LastOK = true; want false")
	}
}

func TestRunIngestLegacyFallback(t *testing.T) {
	// When registry is nil, should fall back to fetchers list
	repo := &mockRepository{}
	fetcher := rss.NewStaticFetcher([]domain.FeedEntry{
		{Title: "legacy", Source: "legacy", PublishedAt: time.Now(), Summary: "legacy", URL: "https://legacy.com"},
	})
	app := New(WithRepository(repo), WithFeedFetchers(fetcher))

	// registry should be nil
	if app.registry != nil {
		t.Errorf("app.registry = %v; want nil (legacy mode)", app.registry)
	}

	result := app.RunIngest(context.Background())
	if result.EntriesFetched != 1 {
		t.Errorf("entries fetched = %d; want 1", result.EntriesFetched)
	}
}

func TestRunExport(t *testing.T) {
	repo := &mockRepository{}
	app := New(WithRepository(repo))

	result := app.RunExport(context.Background())
	if result.StoriesExported != 0 {
		t.Errorf("stories exported = %d; want 0 (no stories)", result.StoriesExported)
	}
	if len(result.Errors) != 0 {
		t.Errorf("errors = %v; want nil", result.Errors)
	}
}

func TestRunExportNoRepository(t *testing.T) {
	app := New()
	result := app.RunExport(context.Background())
	if result.StoriesExported != 0 {
		t.Errorf("stories exported = %d; want 0", result.StoriesExported)
	}
}
