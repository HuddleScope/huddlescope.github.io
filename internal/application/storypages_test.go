package application

import (
	"context"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

func TestRunStoryPagesNoRepoErrors(t *testing.T) {
	app := New()

	result := app.RunStoryPages(context.Background(), time.Now(), nil)

	if len(result.Errors) == 0 {
		t.Error("Expected error when repo not configured")
	}
	if result.PagesBuilt != 0 {
		t.Errorf("Expected 0 pages built, got %d", result.PagesBuilt)
	}
	if result.PagesPublished != 0 {
		t.Errorf("Expected 0 pages published, got %d", result.PagesPublished)
	}
}

func TestRunStoryPagesNoClusteringErrors(t *testing.T) {
	repo := storage.NewMemoryRepository()
	app := New(WithRepository(repo))

	result := app.RunStoryPages(context.Background(), time.Now(), nil)

	if len(result.Errors) == 0 {
		t.Error("Expected error when clustering engine not configured")
	}
}

func TestRunStoryPagesNoopPublisherSuccess(t *testing.T) {
	repo := storage.NewMemoryRepository()
	_, err := repo.Upsert(domain.Story{
		ID:          "test-1",
		Title:       "Test Story 1",
		Source:      "test-source",
		PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Summary:     "Summary 1",
		URL:         "https://example.com/test-1",
	})
	if err != nil {
		t.Fatalf("Failed to insert test story 1: %v", err)
	}
	_, err = repo.Upsert(domain.Story{
		ID:          "test-2",
		Title:       "Test Story 2",
		Source:      "test-source",
		PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Summary:     "Summary 2",
		URL:         "https://example.com/test-2",
	})
	if err != nil {
		t.Fatalf("Failed to insert test story 2: %v", err)
	}

	app := New(
		WithRepository(repo),
		WithClusterer(domain.NewClusterer()),
	)

	result := app.RunStoryPages(context.Background(), time.Now(), nil)

	if len(result.Errors) > 0 {
		t.Fatalf("Unexpected errors: %v", result.Errors)
	}
	if result.PagesBuilt == 0 {
		t.Error("Expected some pages built")
	}
}

func TestRunStoryPagesPerStoryFailureIsolation(t *testing.T) {
	repo := storage.NewMemoryRepository()

	for i := 0; i < 5; i++ {
		_, err := repo.Upsert(domain.Story{
			ID:          "test-" + string(rune('0'+i)),
			Title:       "Test Story " + string(rune('0'+i)),
			Source:      "test-source",
			PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Summary:     "Summary",
		})
		if err != nil {
			t.Fatalf("Failed to insert story %d: %v", i, err)
		}
	}

	app := New(
		WithRepository(repo),
		WithClusterer(domain.NewClusterer()),
	)

	publisher := &testPublisher{}

	app.RunStoryPages(context.Background(), time.Now(), publisher)

	if len(app.RunStoryPages(context.Background(), time.Now(), publisher).Errors) == 0 {
		t.Error("Expected some page failures")
	}
}

func TestRunStoryPagesIdempotent(t *testing.T) {
	repo := storage.NewMemoryRepository()
	_, err := repo.Upsert(domain.Story{
		ID:          "test-1",
		Title:       "Test Story",
		Source:      "test-source",
		PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Summary:     "Summary",
		URL:         "https://example.com/test-1",
	})
	if err != nil {
		t.Fatalf("Failed to insert test story: %v", err)
	}

	app := New(
		WithRepository(repo),
		WithClusterer(domain.NewClusterer()),
	)

	now := time.Now()
	result1 := app.RunStoryPages(context.Background(), now, &DefaultStoryPagePublisher{})
	result2 := app.RunStoryPages(context.Background(), now, &DefaultStoryPagePublisher{})

	if len(result1.Errors) > 0 {
		t.Errorf("First run had errors: %v", result1.Errors)
	}
	if len(result2.Errors) > 0 {
		t.Errorf("Second run had errors: %v", result2.Errors)
	}
}

type testPublisher struct {
	count int
}

func (p *testPublisher) Publish(ctx context.Context, page domain.StoryDetailPage) error {
	p.count++
	if p.count%2 == 0 {
		return nil
	}
	return &testError{msg: "simulated failure"}
}

type testError struct {
	msg string
}

func (e *testError) Error() string { return e.msg }
func (e *testError) Unwrap() error { return e }
