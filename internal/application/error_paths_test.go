package application

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// errRepository is a Repository stub with configurable per-method errors.
type errRepository struct {
	stories    []domain.Story
	failUpsert bool
	failList   bool
}

func (r *errRepository) Upsert(story domain.Story) (domain.UpsertStatus, error) {
	if r.failUpsert {
		return 0, fmt.Errorf("upsert boom")
	}
	return domain.UpsertCreated, nil
}

func (r *errRepository) List() ([]domain.Story, error) {
	if r.failList {
		return nil, fmt.Errorf("list boom")
	}
	return r.stories, nil
}

func (r *errRepository) Count() (int, error) {
	return len(r.stories), nil
}

// errSnapshotPublisher is a SnapshotPublisher stub with a failure switch.
type errSnapshotPublisher struct {
	fail bool
}

func (p *errSnapshotPublisher) Publish(ctx context.Context, snapshot domain.Snapshot) (domain.PublicationResult, error) {
	if p.fail {
		return domain.PublicationResult{}, fmt.Errorf("snapshot publish boom")
	}
	return domain.PublicationResult{Committed: true}, nil
}

// errStoryPagePublisher is a StoryPagePublisher stub with a failure switch.
type errStoryPagePublisher struct {
	fail bool
}

func (p *errStoryPagePublisher) Publish(ctx context.Context, page domain.StoryDetailPage) error {
	if p.fail {
		return fmt.Errorf("story page publish boom")
	}
	return nil
}

func hasErrorContaining(errs []error, substr string) bool {
	for _, err := range errs {
		if err != nil && strings.Contains(err.Error(), substr) {
			return true
		}
	}
	return false
}

func newTestRegistry(t *testing.T, id string, enabled bool) *domain.FeedRegistry {
	t.Helper()
	registry, err := domain.NewFeedRegistry(domain.FeedSource{
		ID:            id,
		URL:           "https://example.com/feed",
		Publication:   "Test Feed",
		Priority:      1,
		Enabled:       enabled,
		Language:      "en",
		License:       "Test license",
		AcceptableUse: domain.AcceptanceAllowed,
	})
	if err != nil {
		t.Fatalf("NewFeedRegistry() error = %v", err)
	}
	return registry
}

func testEntry(sourceID string, publishedAt time.Time) domain.FeedEntry {
	return domain.FeedEntry{
		Title:       "Article " + sourceID,
		Source:      sourceID,
		PublishedAt: publishedAt,
		Summary:     "Summary " + sourceID,
		URL:         "https://example.com/" + sourceID,
		GUID:        "guid-" + sourceID,
	}
}

func TestDefaultPublisherPublish(t *testing.T) {
	p := &DefaultPublisher{}
	if err := p.Publish(context.Background(), domain.Homepage{}); err != nil {
		t.Errorf("Publish() error = %v, want nil", err)
	}
}

func TestWithSnapshotPublisherOption(t *testing.T) {
	repo := storage.NewMemoryRepository()
	if _, err := repo.Upsert(domain.Story{
		ID:          "s1",
		Title:       "Story",
		Source:      "feed1",
		PublishedAt: time.Now().UTC().Add(-time.Hour),
		Summary:     "Summary",
		URL:         "https://example.com/s1",
	}); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	snapPub := &errSnapshotPublisher{fail: true}
	app := New(
		WithRepository(repo),
		WithTeamRegistry(domain.NewTeamRegistry()),
		WithClusterer(domain.NewClusterer()),
		WithScorer(domain.NewScorer()),
		WithSnapshotPublisher(snapPub),
	)
	result := app.RunSnapshot(context.Background(), time.Now())
	if !hasErrorContaining(result.Errors, "publish failed") {
		t.Errorf("Errors = %v, want publish failure", result.Errors)
	}
	if result.PubResult.Committed {
		t.Error("PubResult.Committed should be false after publish failure")
	}
}

func TestRunSnapshotErrorPaths(t *testing.T) {
	t.Run("no repository", func(t *testing.T) {
		app := New(
			WithClusterer(domain.NewClusterer()),
			WithScorer(domain.NewScorer()),
		)
		result := app.RunSnapshot(context.Background(), time.Now())
		if !hasErrorContaining(result.Errors, "repository") {
			t.Errorf("Errors = %v, want repository error", result.Errors)
		}
	})

	t.Run("list stories fails during build", func(t *testing.T) {
		app := New(
			WithRepository(&errRepository{failList: true}),
			WithClusterer(domain.NewClusterer()),
			WithScorer(domain.NewScorer()),
		)
		result := app.RunSnapshot(context.Background(), time.Now())
		if !hasErrorContaining(result.Errors, "build snapshot failed") {
			t.Errorf("Errors = %v, want build snapshot failure", result.Errors)
		}
		if result.PubResult.Committed {
			t.Error("PubResult.Committed should be false")
		}
	})
}

func TestRunIngestBranches(t *testing.T) {
	t.Run("no fetchers configured", func(t *testing.T) {
		app := New(WithRepository(storage.NewMemoryRepository()))
		result := app.RunIngest(context.Background())
		if result.EntriesFetched != 0 || len(result.Errors) != 0 {
			t.Errorf("result = %+v, want empty", result)
		}
	})

	t.Run("upsert error", func(t *testing.T) {
		app := New(
			WithRepository(&errRepository{failUpsert: true}),
			WithFeedFetchers(&mockFetcher{entries: []domain.FeedEntry{testEntry("f1", time.Now().UTC())}}),
		)
		result := app.RunIngest(context.Background())
		if !hasErrorContaining(result.Errors, "upsert boom") {
			t.Errorf("Errors = %v, want upsert error", result.Errors)
		}
	})

	t.Run("updated and unchanged statuses", func(t *testing.T) {
		repo := storage.NewMemoryRepository()
		entry := testEntry("f1", time.Now().UTC().Add(-time.Hour))
		fetcher := &mockFetcher{entries: []domain.FeedEntry{entry}}
		app := New(WithRepository(repo), WithFeedFetchers(fetcher))

		first := app.RunIngest(context.Background())
		if first.StoriesCreated != 1 {
			t.Fatalf("first run StoriesCreated = %d, want 1", first.StoriesCreated)
		}

		// Same entry again: memory repo dedupes by GUID/URL, so the second
		// upsert is updated (new summary) then unchanged.
		changed := testEntry("f1", time.Now().UTC().Add(-time.Hour))
		changed.Summary = "changed summary"
		fetcher.entries = []domain.FeedEntry{changed}
		second := app.RunIngest(context.Background())
		if second.StoriesUpdated != 1 {
			t.Errorf("second run StoriesUpdated = %d, want 1", second.StoriesUpdated)
		}

		fetcher.entries = []domain.FeedEntry{changed}
		third := app.RunIngest(context.Background())
		if third.StoriesSkipped != 1 {
			t.Errorf("third run StoriesSkipped = %d, want 1 (unchanged)", third.StoriesSkipped)
		}
	})

	t.Run("registry upsert error", func(t *testing.T) {
		app := New(
			WithRepository(&errRepository{failUpsert: true}),
			WithRegistry(newTestRegistry(t, "f1", true)),
			WithFetcherMap(map[string]domain.FeedFetcher{
				"f1": &mockFetcher{entries: []domain.FeedEntry{testEntry("f1", time.Now().UTC())}},
			}),
		)
		result := app.RunIngest(context.Background())
		if !hasErrorContaining(result.Errors, "upsert boom") {
			t.Errorf("Errors = %v, want upsert error", result.Errors)
		}
	})

	t.Run("registry created updated and skipped", func(t *testing.T) {
		repo := storage.NewMemoryRepository()
		registry := newTestRegistry(t, "f1", true)
		fetcher := &mockFetcher{entries: []domain.FeedEntry{testEntry("f1", time.Now().UTC().Add(-time.Hour))}}
		app := New(
			WithRepository(repo),
			WithRegistry(registry),
			WithFetcherMap(map[string]domain.FeedFetcher{"f1": fetcher}),
		)

		first := app.RunIngest(context.Background())
		if first.StoriesCreated != 1 {
			t.Fatalf("first run StoriesCreated = %d, want 1", first.StoriesCreated)
		}

		changed := testEntry("f1", time.Now().UTC().Add(-time.Hour))
		changed.Summary = "changed summary"
		fetcher.entries = []domain.FeedEntry{changed}
		second := app.RunIngest(context.Background())
		if second.StoriesUpdated != 1 {
			t.Errorf("second run StoriesUpdated = %d, want 1", second.StoriesUpdated)
		}

		third := app.RunIngest(context.Background())
		if third.StoriesSkipped != 1 {
			t.Errorf("third run StoriesSkipped = %d, want 1 (unchanged)", third.StoriesSkipped)
		}
	})
}

func TestRunGroupingBranches(t *testing.T) {
	t.Run("no repository", func(t *testing.T) {
		app := New(WithClusterer(domain.NewClusterer()))
		result := app.RunGrouping(context.Background())
		if !hasErrorContaining(result.Errors, "repository") {
			t.Errorf("Errors = %v, want repository error", result.Errors)
		}
	})

	t.Run("list stories fails", func(t *testing.T) {
		app := New(
			WithClusterer(domain.NewClusterer()),
			WithRepository(&errRepository{failList: true}),
		)
		result := app.RunGrouping(context.Background())
		if !hasErrorContaining(result.Errors, "list stories") {
			t.Errorf("Errors = %v, want list error", result.Errors)
		}
	})

	t.Run("upsert of grouped story fails", func(t *testing.T) {
		repo := &errRepository{
			failUpsert: true,
			stories: []domain.Story{
				{ID: "s1", Title: "Story", Source: "f1", PublishedAt: time.Now().UTC(), URL: "https://example.com/s1"},
			},
		}
		app := New(WithClusterer(domain.NewClusterer()), WithRepository(repo))
		result := app.RunGrouping(context.Background())
		if !hasErrorContaining(result.Errors, "upsert boom") {
			t.Errorf("Errors = %v, want upsert error", result.Errors)
		}
	})

	t.Run("solo and grouped counts", func(t *testing.T) {
		repo := storage.NewMemoryRepository()
		for i, source := range []string{"f1", "f2", "f3"} {
			story := domain.Story{
				ID:          fmt.Sprintf("s%d", i),
				Title:       fmt.Sprintf("Story %d", i),
				Source:      source,
				PublishedAt: time.Now().UTC().Add(-time.Duration(i) * time.Hour),
				Summary:     "Summary",
				URL:         fmt.Sprintf("https://example.com/s%d", i),
				Entities:    []string{"team-a"},
			}
			if _, err := repo.Upsert(story); err != nil {
				t.Fatalf("Upsert() error = %v", err)
			}
		}
		app := New(WithClusterer(domain.NewClusterer()), WithRepository(repo))
		result := app.RunGrouping(context.Background())
		if result.GroupsCreated == 0 {
			t.Error("GroupsCreated should be > 0")
		}
		if result.ArticlesSolo+result.ArticlesGrouped == 0 {
			t.Errorf("expected some articles, got solo=%d grouped=%d", result.ArticlesSolo, result.ArticlesGrouped)
		}
	})
}

func TestRunExportBranches(t *testing.T) {
	t.Run("no repository", func(t *testing.T) {
		app := New()
		result := app.RunExport(context.Background())
		if result.StoriesExported != 0 || len(result.Errors) != 0 {
			t.Errorf("result = %+v, want empty", result)
		}
	})

	t.Run("list stories fails", func(t *testing.T) {
		app := New(WithRepository(&errRepository{failList: true}))
		result := app.RunExport(context.Background())
		if !hasErrorContaining(result.Errors, "list boom") {
			t.Errorf("Errors = %v, want list error", result.Errors)
		}
	})
}

func TestRunScoringNoRepository(t *testing.T) {
	app := New(
		WithClusterer(domain.NewClusterer()),
		WithScorer(domain.NewScorer()),
	)
	result := app.RunScoring(context.Background(), time.Now())
	if !hasErrorContaining(result.Errors, "repository") {
		t.Errorf("Errors = %v, want repository error", result.Errors)
	}
}

func TestRunPipelinePublishBranches(t *testing.T) {
	buildApp := func(publisher Publisher) *Application {
		return New(
			WithRepository(storage.NewMemoryRepository()),
			WithRegistry(newTestRegistry(t, "f1", true)),
			WithFetcherMap(map[string]domain.FeedFetcher{
				"f1": &mockFetcher{entries: []domain.FeedEntry{testEntry("f1", time.Now().UTC().Add(-time.Hour))}},
			}),
			WithClusterer(domain.NewClusterer()),
			WithScorer(domain.NewScorer()),
			WithLookback(24*time.Hour),
			WithPublisher(publisher),
		)
	}

	t.Run("publish failure fails the run", func(t *testing.T) {
		pub := &TestPublisher{}
		pub.SetFailOnce()
		app := buildApp(pub)
		result := app.RunPipeline(context.Background(), time.Now())
		if result.Status != domain.RunStatusFailed {
			t.Errorf("Status = %s, want Failed", result.Status)
		}
		if result.Committed {
			t.Error("Committed should be false after publish failure")
		}
		if !hasErrorContaining(result.Errors, "homepage publish failed") {
			t.Errorf("Errors = %v, want publish error", result.Errors)
		}
	})

	t.Run("no publisher commits offline", func(t *testing.T) {
		app := New(
			WithRepository(storage.NewMemoryRepository()),
			WithRegistry(newTestRegistry(t, "f1", true)),
			WithFetcherMap(map[string]domain.FeedFetcher{
				"f1": &mockFetcher{entries: []domain.FeedEntry{testEntry("f1", time.Now().UTC().Add(-time.Hour))}},
			}),
			WithClusterer(domain.NewClusterer()),
			WithScorer(domain.NewScorer()),
			WithLookback(24*time.Hour),
		)
		result := app.RunPipeline(context.Background(), time.Now())
		if result.Status != domain.RunStatusOK {
			t.Errorf("Status = %s, want OK", result.Status)
		}
		if !result.Committed {
			t.Error("Committed should be true without publisher")
		}
	})
}

func TestRunIngestPipelineBranches(t *testing.T) {
	t.Run("no repository", func(t *testing.T) {
		app := New()
		result := app.RunPipeline(context.Background(), time.Now())
		if result.Status != domain.RunStatusFailed {
			t.Errorf("Status = %s, want Failed", result.Status)
		}
		if !hasErrorContaining(result.Errors, "repository") {
			t.Errorf("Errors = %v, want repository error", result.Errors)
		}
	})

	t.Run("no registry or fetchers", func(t *testing.T) {
		app := New(WithRepository(storage.NewMemoryRepository()))
		result := app.RunPipeline(context.Background(), time.Now())
		if result.Status != domain.RunStatusFailed {
			t.Errorf("Status = %s, want Failed", result.Status)
		}
		if !hasErrorContaining(result.Errors, "no registry or fetchers") {
			t.Errorf("Errors = %v, want configuration error", result.Errors)
		}
	})

	t.Run("legacy fetcher success", func(t *testing.T) {
		app := New(
			WithRepository(storage.NewMemoryRepository()),
			WithFeedFetchers(&mockFetcher{entries: []domain.FeedEntry{testEntry("f1", time.Now().UTC().Add(-time.Hour))}}),
		)
		result := app.RunPipeline(context.Background(), time.Now())
		if result.EntriesFetched != 1 {
			t.Errorf("EntriesFetched = %d, want 1", result.EntriesFetched)
		}
		if result.StoriesCreated != 1 {
			t.Errorf("StoriesCreated = %d, want 1", result.StoriesCreated)
		}
	})

	t.Run("legacy fetcher failure", func(t *testing.T) {
		app := New(
			WithRepository(storage.NewMemoryRepository()),
			WithFeedFetchers(&mockFetcher{err: fmt.Errorf("fetch boom")}),
		)
		result := app.RunPipeline(context.Background(), time.Now())
		if result.Status != domain.RunStatusFailed {
			t.Errorf("Status = %s, want Failed", result.Status)
		}
		if !hasErrorContaining(result.Errors, "fetch boom") {
			t.Errorf("Errors = %v, want fetch error", result.Errors)
		}
	})

	t.Run("registry without enabled sources", func(t *testing.T) {
		app := New(
			WithRepository(storage.NewMemoryRepository()),
			WithRegistry(newTestRegistry(t, "f1", false)),
		)
		result := app.RunPipeline(context.Background(), time.Now())
		if result.Status != domain.RunStatusFailed {
			t.Errorf("Status = %s, want Failed", result.Status)
		}
		if !hasErrorContaining(result.Errors, "no enabled sources") {
			t.Errorf("Errors = %v, want enabled sources error", result.Errors)
		}
	})

	t.Run("fetch failure marks partial", func(t *testing.T) {
		app := New(
			WithRepository(storage.NewMemoryRepository()),
			WithRegistry(newTestRegistry(t, "f1", true)),
			WithFetcherMap(map[string]domain.FeedFetcher{"f1": &mockFetcher{err: fmt.Errorf("fetch boom")}}),
			WithClusterer(domain.NewClusterer()),
			WithScorer(domain.NewScorer()),
		)
		result := app.RunPipeline(context.Background(), time.Now())
		if result.Status != domain.RunStatusPartial {
			t.Errorf("Status = %s, want Partial", result.Status)
		}
		if !hasErrorContaining(result.Errors, "fetch boom") {
			t.Errorf("Errors = %v, want fetch error", result.Errors)
		}
	})

	t.Run("success with upsert error on second entry", func(t *testing.T) {
		repo := storage.NewMemoryRepository()
		app := New(
			WithRepository(repo),
			WithRegistry(newTestRegistry(t, "f1", true)),
			WithFetcherMap(map[string]domain.FeedFetcher{
				"f1": &mockFetcher{entries: []domain.FeedEntry{
					testEntry("f1", time.Now().UTC().Add(-time.Hour)),
					testEntry("f2", time.Now().UTC().Add(-30*time.Minute)),
				}},
			}),
			WithClusterer(domain.NewClusterer()),
			WithScorer(domain.NewScorer()),
			WithLookback(24*time.Hour),
		)
		result := app.RunPipeline(context.Background(), time.Now())
		if result.Status != domain.RunStatusOK {
			t.Errorf("Status = %s, want OK", result.Status)
		}
		if result.StoriesCreated != 2 {
			t.Errorf("StoriesCreated = %d, want 2", result.StoriesCreated)
		}
		if result.LastPublishedAt == nil {
			t.Error("LastPublishedAt should be set")
		}
	})

	t.Run("upsert error recorded per entry", func(t *testing.T) {
		app := New(
			WithRepository(&errRepository{failUpsert: true}),
			WithRegistry(newTestRegistry(t, "f1", true)),
			WithFetcherMap(map[string]domain.FeedFetcher{
				"f1": &mockFetcher{entries: []domain.FeedEntry{testEntry("f1", time.Now().UTC().Add(-time.Hour))}},
			}),
			WithClusterer(domain.NewClusterer()),
			WithScorer(domain.NewScorer()),
			WithLookback(24*time.Hour),
		)
		result := app.RunPipeline(context.Background(), time.Now())
		if !hasErrorContaining(result.Errors, "upsert boom") {
			t.Errorf("Errors = %v, want upsert error", result.Errors)
		}
	})

	t.Run("entry outside lookback is filtered", func(t *testing.T) {
		repo := storage.NewMemoryRepository()
		app := New(
			WithRepository(repo),
			WithRegistry(newTestRegistry(t, "f1", true)),
			WithFetcherMap(map[string]domain.FeedFetcher{
				"f1": &mockFetcher{entries: []domain.FeedEntry{
					testEntry("f1", time.Now().UTC().Add(-72*time.Hour)),
				}},
			}),
			WithClusterer(domain.NewClusterer()),
			WithScorer(domain.NewScorer()),
			WithLookback(24*time.Hour),
		)
		result := app.RunPipeline(context.Background(), time.Now())
		if result.StoriesCreated != 0 {
			t.Errorf("StoriesCreated = %d, want 0 (filtered by lookback)", result.StoriesCreated)
		}
		stories, err := repo.List()
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(stories) != 0 {
			t.Errorf("repo should be empty, got %d stories", len(stories))
		}
	})
}

func TestRunGroupAndScorePipelineBranches(t *testing.T) {
	ingestApp := func(opts ...Option) *Application {
		base := []Option{
			WithRepository(storage.NewMemoryRepository()),
			WithRegistry(newTestRegistry(t, "f1", true)),
			WithFetcherMap(map[string]domain.FeedFetcher{
				"f1": &mockFetcher{entries: []domain.FeedEntry{testEntry("f1", time.Now().UTC().Add(-time.Hour))}},
			}),
			WithLookback(24 * time.Hour),
		}
		base = append(base, opts...)
		return New(base...)
	}

	t.Run("no clustering engine", func(t *testing.T) {
		app := ingestApp(WithScorer(domain.NewScorer()))
		result := app.RunPipeline(context.Background(), time.Now())
		if result.Status != domain.RunStatusFailed {
			t.Errorf("Status = %s, want Failed", result.Status)
		}
		if !hasErrorContaining(result.Errors, "clustering engine") {
			t.Errorf("Errors = %v, want clustering error", result.Errors)
		}
	})

	t.Run("no scorer", func(t *testing.T) {
		app := ingestApp(WithClusterer(domain.NewClusterer()))
		result := app.RunPipeline(context.Background(), time.Now())
		if result.Status != domain.RunStatusFailed {
			t.Errorf("Status = %s, want Failed", result.Status)
		}
		if !hasErrorContaining(result.Errors, "scorer") {
			t.Errorf("Errors = %v, want scorer error", result.Errors)
		}
	})

	t.Run("list stories fails", func(t *testing.T) {
		app := New(
			WithRepository(&errRepository{failList: true}),
			WithClusterer(domain.NewClusterer()),
			WithScorer(domain.NewScorer()),
		)
		result := app.RebuildRanking(context.Background(), time.Now())
		if result.Status != domain.RunStatusFailed {
			t.Errorf("Status = %s, want Failed", result.Status)
		}
		if !hasErrorContaining(result.Errors, "list") {
			t.Errorf("Errors = %v, want list error", result.Errors)
		}
	})
}

func TestRebuildRankingBranches(t *testing.T) {
	t.Run("no repository", func(t *testing.T) {
		app := New(WithClusterer(domain.NewClusterer()), WithScorer(domain.NewScorer()))
		result := app.RebuildRanking(context.Background(), time.Now())
		if result.Status != domain.RunStatusFailed {
			t.Errorf("Status = %s, want Failed", result.Status)
		}
		if !hasErrorContaining(result.Errors, "repository") {
			t.Errorf("Errors = %v, want repository error", result.Errors)
		}
	})

	t.Run("no clustering engine", func(t *testing.T) {
		app := New(
			WithRepository(storage.NewMemoryRepository()),
			WithScorer(domain.NewScorer()),
		)
		result := app.RebuildRanking(context.Background(), time.Now())
		if result.Status != domain.RunStatusFailed {
			t.Errorf("Status = %s, want Failed", result.Status)
		}
		if !hasErrorContaining(result.Errors, "clustering engine") {
			t.Errorf("Errors = %v, want clustering error", result.Errors)
		}
	})

	t.Run("no scorer", func(t *testing.T) {
		app := New(
			WithRepository(storage.NewMemoryRepository()),
			WithClusterer(domain.NewClusterer()),
		)
		result := app.RebuildRanking(context.Background(), time.Now())
		if result.Status != domain.RunStatusFailed {
			t.Errorf("Status = %s, want Failed", result.Status)
		}
		if !hasErrorContaining(result.Errors, "scorer") {
			t.Errorf("Errors = %v, want scorer error", result.Errors)
		}
	})

	t.Run("publish failure fails the run", func(t *testing.T) {
		repo := storage.NewMemoryRepository()
		if _, err := repo.Upsert(domain.Story{
			ID:          "s1",
			Title:       "Story",
			Source:      "f1",
			PublishedAt: time.Now().UTC().Add(-time.Hour),
			Summary:     "Summary",
			URL:         "https://example.com/s1",
		}); err != nil {
			t.Fatalf("Upsert() error = %v", err)
		}
		pub := &TestPublisher{}
		pub.SetFailOnce()
		app := New(
			WithRepository(repo),
			WithClusterer(domain.NewClusterer()),
			WithScorer(domain.NewScorer()),
			WithPublisher(pub),
		)
		result := app.RebuildRanking(context.Background(), time.Now())
		if result.Status != domain.RunStatusFailed {
			t.Errorf("Status = %s, want Failed", result.Status)
		}
		if result.Committed {
			t.Error("Committed should be false after publish failure")
		}
	})

	t.Run("publish success commits", func(t *testing.T) {
		repo := storage.NewMemoryRepository()
		if _, err := repo.Upsert(domain.Story{
			ID:          "s1",
			Title:       "Story",
			Source:      "f1",
			PublishedAt: time.Now().UTC().Add(-time.Hour),
			Summary:     "Summary",
			URL:         "https://example.com/s1",
		}); err != nil {
			t.Fatalf("Upsert() error = %v", err)
		}
		app := New(
			WithRepository(repo),
			WithClusterer(domain.NewClusterer()),
			WithScorer(domain.NewScorer()),
			WithPublisher(&TestPublisher{}),
		)
		result := app.RebuildRanking(context.Background(), time.Now())
		if result.Status != domain.RunStatusOK {
			t.Errorf("Status = %s, want OK", result.Status)
		}
		if !result.Committed {
			t.Error("Committed should be true")
		}
	})
}

func TestRunStoryPagesBranches(t *testing.T) {
	t.Run("list stories fails", func(t *testing.T) {
		app := New(
			WithClusterer(domain.NewClusterer()),
			WithRepository(&errRepository{failList: true}),
		)
		result := app.RunStoryPages(context.Background(), time.Now(), nil)
		if result.FailedStories != 1 {
			t.Errorf("FailedStories = %d, want 1", result.FailedStories)
		}
		if !hasErrorContaining(result.Errors, "list boom") {
			t.Errorf("Errors = %v, want list error", result.Errors)
		}
	})

	t.Run("publish failure counts failed stories", func(t *testing.T) {
		repo := storage.NewMemoryRepository()
		if _, err := repo.Upsert(domain.Story{
			ID:          "s1",
			Title:       "Story",
			Source:      "f1",
			PublishedAt: time.Now().UTC().Add(-time.Hour),
			Summary:     "Summary",
			URL:         "https://example.com/s1",
		}); err != nil {
			t.Fatalf("Upsert() error = %v", err)
		}
		app := New(WithClusterer(domain.NewClusterer()), WithRepository(repo))
		result := app.RunStoryPages(context.Background(), time.Now(), &errStoryPagePublisher{fail: true})
		if result.FailedStories != 1 {
			t.Errorf("FailedStories = %d, want 1", result.FailedStories)
		}
		if !hasErrorContaining(result.Errors, "publish failed") {
			t.Errorf("Errors = %v, want publish error", result.Errors)
		}
	})
}

func TestSourceNameResolverWithRegistry(t *testing.T) {
	repo := storage.NewMemoryRepository()
	if _, err := repo.Upsert(domain.Story{
		ID:          "s1",
		Title:       "Story",
		Source:      "f1",
		PublishedAt: time.Now().UTC().Add(-time.Hour),
		Summary:     "Summary",
		URL:         "https://example.com/s1",
	}); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	registry, err := domain.NewFeedRegistry(
		domain.FeedSource{
			ID:            "f1",
			URL:           "https://example.com/f1",
			Publication:   "Fancy Publication Name",
			Priority:      1,
			Enabled:       true,
			Language:      "en",
			License:       "Test license",
			AcceptableUse: domain.AcceptanceAllowed,
		},
		domain.FeedSource{
			ID:            "f2",
			URL:           "https://example.com/f2",
			Priority:      2,
			Enabled:       true,
			Language:      "en",
			License:       "Test license",
			AcceptableUse: domain.AcceptanceAllowed,
		},
	)
	if err != nil {
		t.Fatalf("NewFeedRegistry() error = %v", err)
	}

	app := New(
		WithRepository(repo),
		WithRegistry(registry),
		WithTeamRegistry(domain.NewTeamRegistry()),
		WithClusterer(domain.NewClusterer()),
		WithScorer(domain.NewScorer()),
	)
	result := app.RunSnapshot(context.Background(), time.Now())
	if len(result.Errors) > 0 {
		t.Errorf("Errors = %v, want none", result.Errors)
	}
	if result.StoriesBuilt != 1 {
		t.Errorf("StoriesBuilt = %d, want 1", result.StoriesBuilt)
	}
}
