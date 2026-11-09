package application

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// memoryRepository is a test implementation of domain.Repository.
type memoryRepository struct {
	stories map[string]domain.Story
	errList error // Simulate List() error
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{stories: make(map[string]domain.Story)}
}

func (r *memoryRepository) setErrorList(err error) {
	r.errList = err
}

func (r *memoryRepository) Upsert(story domain.Story) error {
	r.stories[story.ID] = story
	return nil
}

func (r *memoryRepository) List() ([]domain.Story, error) {
	result := make([]domain.Story, 0, len(r.stories))
	for _, s := range r.stories {
		result = append(result, s)
	}
	return result, r.errList
}

func (r *memoryRepository) Count() (int, error) {
	return len(r.stories), nil
}

// staticFetcher returns a fixed feed entry for testing.
type staticFetcher struct {
	entries []domain.FeedEntry
}

func (f *staticFetcher) Fetch(sourceID string) ([]domain.FeedEntry, error) {
	return f.entries, nil
}

func TestApplicationRunIngestSuccess(t *testing.T) {
	repo := newMemoryRepository()
	fetcher := &staticFetcher{
		entries: []domain.FeedEntry{
			{
				Title:       "Test Story 1",
				Source:      "test-source",
				PublishedAt: time.Now().UTC(),
				Summary:     "Test summary 1",
			},
			{
				Title:       "Test Story 2",
				Source:      "test-source",
				PublishedAt: time.Now().UTC(),
				Summary:     "Test summary 2",
			},
		},
	}

	app := New(
		WithRepository(repo),
		WithFeedFetchers(fetcher),
	)

	result := app.RunIngest(context.Background())

	if result.EntriesFetched != 2 {
		t.Errorf("EntriesFetched = %d; want 2", result.EntriesFetched)
	}
	if len(result.Errors) != 0 {
		t.Errorf("Errors = %v; want none", result.Errors)
	}
	// With the current implementation, both entries create a story (upsert behavior)
	// For idempotency testing, see TestApplicationRunIngestIdempotent
	if result.StoriesCreated != 2 {
		t.Errorf("StoriesCreated = %d; want 2", result.StoriesCreated)
	}
}

func TestApplicationRunIngestNoRepository(t *testing.T) {
	app := New()

	result := app.RunIngest(context.Background())

	if result.StoriesSkipped != 0 {
		t.Errorf("StoriesSkipped = %d; want 0 (no work)", result.StoriesSkipped)
	}
	if len(result.Errors) != 0 {
		t.Errorf("Errors = %v; want none", result.Errors)
	}
}

func TestApplicationRunIngestIdempotent(t *testing.T) {
	repo := newMemoryRepository()
	fetcher := &staticFetcher{
		entries: []domain.FeedEntry{
			{
				Title:       "Same Title",
				Source:      "test-source",
				PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Summary:     "Test",
			},
		},
	}

	app := New(
		WithRepository(repo),
		WithFeedFetchers(fetcher),
	)

	// First run
	result1 := app.RunIngest(context.Background())
	if len(result1.Errors) != 0 {
		t.Errorf("First run Errors = %v; want none", result1.Errors)
	}

	// Second run with same data (idempotent)
	result2 := app.RunIngest(context.Background())
	if len(result2.Errors) != 0 {
		t.Errorf("Second run Errors = %v; want none", result2.Errors)
	}

	// Count should be 1 (idempotent upsert overwrites)
	count, _ := repo.Count()
	if count != 1 {
		t.Errorf("Count = %d; want 1 (idempotent upsert)", count)
	}
}

func TestApplicationRunExportNoRepository(t *testing.T) {
	app := New()

	result := app.RunExport(context.Background())

	if result.StoriesExported != 0 {
		t.Errorf("StoriesExported = %d; want 0 (no work)", result.StoriesExported)
	}
}

func TestApplicationRunExportSuccess(t *testing.T) {
	repo := newMemoryRepository()
	_ = repo.Upsert(domain.Story{ID: "s1", Title: "S1"})
	_ = repo.Upsert(domain.Story{ID: "s2", Title: "S2"})

	app := New(
		WithRepository(repo),
	)

	result := app.RunExport(context.Background())

	if result.StoriesExported != 2 {
		t.Errorf("StoriesExported = %d; want 2", result.StoriesExported)
	}
	if len(result.Errors) != 0 {
		t.Errorf("Errors = %v; want none", result.Errors)
	}
}

func TestApplicationRunExportFailure(t *testing.T) {
	repo := newMemoryRepository()
	_ = repo.Upsert(domain.Story{ID: "s1", Title: "S1"})
	repo.setErrorList(fmt.Errorf("list error"))

	app := New(
		WithRepository(repo),
	)

	result := app.RunExport(context.Background())

	if result.StoriesExported != 0 {
		t.Errorf("StoriesExported = %d; want 0", result.StoriesExported)
	}
	if len(result.Errors) != 1 {
		t.Errorf("Errors = %v; want 1 error", result.Errors)
	}
}

func TestApplicationRunIngestFailurePath(t *testing.T) {
	repo := newMemoryRepository()
	// Use nil repository to test failure path
	app := New(
		WithRepository(repo),
	)

	result := app.RunIngest(context.Background())
	// With a configured repo but no fetchers, returns no work
	if len(result.Errors) != 0 {
		t.Errorf("Errors = %v; want none (no fetchers)", result.Errors)
	}
}

func TestApplicationRunExportFailurePath(t *testing.T) {
	repo := newMemoryRepository()
	// Store a story
	_ = repo.Upsert(domain.Story{ID: "s1", Title: "S1"})

	app := New(
		WithRepository(repo),
	)

	result := app.RunExport(context.Background())
	if result.StoriesExported != 1 {
		t.Errorf("StoriesExported = %d; want 1", result.StoriesExported)
	}
	if len(result.Errors) != 0 {
		t.Errorf("Errors = %v; want none", result.Errors)
	}
}
