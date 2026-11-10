package application

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/rss"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

type mockRepository struct {
	stories []domain.Story
}

func (r *mockRepository) Upsert(story domain.Story) (domain.UpsertStatus, error) {
	r.stories = append(r.stories, story)
	return domain.UpsertCreated, nil
}

func (r *mockRepository) List() ([]domain.Story, error) {
	return r.stories, nil
}

func (r *mockRepository) Count() (int, error) {
	return len(r.stories), nil
}

type failingFetcher struct {
	err error
}

func (f *failingFetcher) Fetch(sourceID string) ([]domain.FeedEntry, error) {
	return nil, f.err
}

func TestNewApplication(t *testing.T) {
	app := New()
	if app == nil {
		t.Fatal("New() returned nil")
	}
}

func TestWithRepository(t *testing.T) {
	repo := &mockRepository{}
	_ = New(WithRepository(repo))
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

func TestWithClusterer(t *testing.T) {
	clust := domain.NewClusterer()
	app := New(WithClusterer(clust))
	result := app.RunGrouping(context.Background())
	if len(result.Errors) == 0 {
		t.Error("expected error when no repository configured")
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

func TestRunIngestWithRegistry(t *testing.T) {
	registry, err := domain.NewFeedRegistry(domain.FeedSource{
		ID:          "test-feed",
		URL:         "https://example.com/feed.xml",
		Publication: "Test Publication",
		Priority:    1,
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}
	app := New(WithRepository(&mockRepository{}), WithRegistry(registry))
	result := app.RunIngest(context.Background())
	_ = result
}

func TestRunIngestRegistryNoEnabledSources(t *testing.T) {
	registry, err := domain.NewFeedRegistry()
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}
	app := New(WithRepository(&mockRepository{}), WithRegistry(registry))
	result := app.RunIngest(context.Background())
	if result.EntriesFetched != 0 || result.StoriesCreated != 0 {
		t.Errorf("expected no work, got entries=%d stories=%d", result.EntriesFetched, result.StoriesCreated)
	}
}

func TestRunIngestRegistryFetchFailure(t *testing.T) {
	fetcher := &failingFetcher{err: fmt.Errorf("fetch failed")}
	registry, err := domain.NewFeedRegistry(domain.FeedSource{
		ID:          "test-feed",
		URL:         "https://example.com/feed.xml",
		Publication: "Test Publication",
		Priority:    1,
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}
	app := New(WithRepository(&mockRepository{}), WithFetcherMap(map[string]domain.FeedFetcher{"test-feed": fetcher}), WithRegistry(registry))
	result := app.RunIngest(context.Background())
	if result.EntriesFetched != 0 {
		t.Errorf("expected 0 entries fetched, got %d", result.EntriesFetched)
	}
	if result.StoriesCreated != 0 {
		t.Errorf("expected 0 stories created, got %d", result.StoriesCreated)
	}
}

func TestRunIngestRegistryPerSourceIsolation(t *testing.T) {
	fetcher1 := rss.NewStaticFetcher([]domain.FeedEntry{
		{Title: "story1", Source: "feed1", PublishedAt: time.Now(), Summary: "s1", URL: "https://f1.com"},
	})
	fetcher2 := &failingFetcher{err: fmt.Errorf("fetcher2 failed")}
	fetcher3 := rss.NewStaticFetcher([]domain.FeedEntry{
		{Title: "story3", Source: "feed3", PublishedAt: time.Now(), Summary: "s3", URL: "https://f3.com"},
	})

	registry, err := domain.NewFeedRegistry(
		domain.FeedSource{ID: "feed1", URL: "https://f1.com", Enabled: true, Publication: "Feed 1", Priority: 1},
		domain.FeedSource{ID: "feed2", URL: "https://f2.com", Enabled: true, Publication: "Feed 2", Priority: 2},
		domain.FeedSource{ID: "feed3", URL: "https://f3.com", Enabled: true, Publication: "Feed 3", Priority: 3},
	)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	app := New(
		WithRepository(&mockRepository{}),
		WithFetcherMap(map[string]domain.FeedFetcher{
			"feed1": fetcher1,
			"feed2": fetcher2,
			"feed3": fetcher3,
		}),
		WithRegistry(registry),
	)
	result := app.RunIngest(context.Background())

	if result.EntriesFetched != 2 {
		t.Errorf("entries fetched = %d; want 2", result.EntriesFetched)
	}
	if result.StoriesCreated != 2 {
		t.Errorf("stories created = %d; want 2", result.StoriesCreated)
	}
	if len(result.Errors) != 1 {
		t.Errorf("errors = %d; want 1", len(result.Errors))
	}
}

func TestRunIngestLegacyFallback(t *testing.T) {
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

	if result.EntriesFetched != 2 {
		t.Errorf("entries fetched = %d; want 2", result.EntriesFetched)
	}
	if result.StoriesCreated != 2 {
		t.Errorf("stories created = %d; want 2", result.StoriesCreated)
	}
	if len(result.Errors) != 1 {
		t.Errorf("errors = %d; want 1", len(result.Errors))
	}
}

func TestRunExport(t *testing.T) {
	repo := &mockRepository{}
	app := New(WithRepository(repo))
	result := app.RunExport(context.Background())
	if result.StoriesExported != 0 {
		t.Errorf("expected 0 stories exported, got %d", result.StoriesExported)
	}
}

func TestRunExportWithStories(t *testing.T) {
	repo := &mockRepository{
		stories: []domain.Story{
			{ID: "story1", Title: "Story 1", Source: "src1", PublishedAt: time.Now(), Summary: "s1", URL: "https://1.com"},
			{ID: "story2", Title: "Story 2", Source: "src2", PublishedAt: time.Now(), Summary: "s2", URL: "https://2.com"},
		},
	}
	app := New(WithRepository(repo))
	result := app.RunExport(context.Background())
	if result.StoriesExported != 2 {
		t.Errorf("expected 2 stories exported, got %d", result.StoriesExported)
	}
}

func TestRunGroupingWithRepository(t *testing.T) {
	repo := &mockRepository{}
	clust := domain.NewClusterer()
	app := New(WithRepository(repo), WithClusterer(clust))

	result := app.RunGrouping(context.Background())

	if result.GroupsCreated != 0 {
		t.Errorf("expected 0 groups created, got %d", result.GroupsCreated)
	}
}

func TestRunGroupingNoEngine(t *testing.T) {
	repo := &mockRepository{}
	app := New(WithRepository(repo))

	result := app.RunGrouping(context.Background())

	if len(result.Errors) == 0 {
		t.Error("expected error when no clustering engine configured")
	}
}

func TestRunGroupingNoRepository(t *testing.T) {
	clust := domain.NewClusterer()
	app := New(WithClusterer(clust))

	result := app.RunGrouping(context.Background())

	if len(result.Errors) == 0 {
		t.Error("expected error when no repository configured")
	}
}

func TestRunGroupingWithStories(t *testing.T) {
	repo := &mockRepository{
		stories: []domain.Story{
			{ID: "story1", Title: "Story One", Source: "src1", PublishedAt: time.Now(), Summary: "s1", URL: "https://1.com"},
			{ID: "story2", Title: "Story Two", Source: "src2", PublishedAt: time.Now(), Summary: "s2", URL: "https://2.com"},
		},
	}
	clust := domain.NewClusterer()
	app := New(WithRepository(repo), WithClusterer(clust))

	result := app.RunGrouping(context.Background())

	_ = result
}

func TestRunIngestRegistryUpsertCreated(t *testing.T) {
	fetcher := rss.NewStaticFetcher([]domain.FeedEntry{
		{Title: "Story 1", Source: "feed1", PublishedAt: time.Now(), Summary: "s1", URL: "https://f1.com", GUID: "guid1"},
	})
	registry, err := domain.NewFeedRegistry(domain.FeedSource{
		ID:          "test-feed",
		URL:         "https://example.com/feed.xml",
		Publication: "Test Publication",
		Priority:    1,
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}
	app := New(WithRepository(&mockRepository{}), WithFetcherMap(map[string]domain.FeedFetcher{"test-feed": fetcher}), WithRegistry(registry))
	result := app.RunIngest(context.Background())
	if result.StoriesCreated != 1 {
		t.Errorf("expected 1 story created, got %d", result.StoriesCreated)
	}
}

func TestRunIngestRegistryMultipleStories(t *testing.T) {
	fetcher := rss.NewStaticFetcher([]domain.FeedEntry{
		{Title: "Story 1", Source: "feed1", PublishedAt: time.Now(), Summary: "s1", URL: "https://f1.com", GUID: "guid1"},
		{Title: "Story 2", Source: "feed1", PublishedAt: time.Now(), Summary: "s2", URL: "https://f2.com", GUID: "guid2"},
	})
	registry, err := domain.NewFeedRegistry(domain.FeedSource{
		ID:          "test-feed",
		URL:         "https://example.com/feed.xml",
		Publication: "Test Publication",
		Priority:    1,
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}
	app := New(WithRepository(&mockRepository{}), WithFetcherMap(map[string]domain.FeedFetcher{"test-feed": fetcher}), WithRegistry(registry))
	result := app.RunIngest(context.Background())
	if result.StoriesCreated != 2 {
		t.Errorf("expected 2 stories created, got %d", result.StoriesCreated)
	}
}
