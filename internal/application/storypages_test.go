package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/hugo"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
	"gopkg.in/yaml.v3"
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

// TestRunStoryPagesHugoPublisherIdempotentAndStalePrune exercises the full
// publish + cleanup path with the real file-based publisher: two runs produce
// the same file set, stale generated pages are pruned, and hand-authored
// files (_index.md, marker-less pages) are preserved. (AC-4)
func TestRunStoryPagesHugoPublisherIdempotentAndStalePrune(t *testing.T) {
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

	outDir := filepath.Join(t.TempDir(), "web", "content")
	storiesDir := filepath.Join(outDir, "stories")
	if err := os.MkdirAll(storiesDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	// Pre-existing files: a stale generated page (marked) and the tracked index.
	staleContent, err := hugo.RenderMarkdown(domain.StoryDetailPage{
		Slug: "old-group", Path: "/stories/old-group/", StoryID: "old",
	}, "")
	if err != nil {
		t.Fatalf("RenderMarkdown failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(storiesDir, "old-group.md"), staleContent, 0o644); err != nil {
		t.Fatalf("WriteFile stale failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(storiesDir, "_index.md"), []byte("---\ntitle: Stories\n---\n"), 0o644); err != nil {
		t.Fatalf("WriteFile index failed: %v", err)
	}

	publisher := hugo.NewPublisher(outDir, "https://huddlescope.github.io")
	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	expectedSlug := domain.ComputeStorySlugFromHeadline("Test Story")

	run := func() (StoryPageResult, []string) {
		res := app.RunStoryPages(context.Background(), now, publisher)
		if len(res.Errors) > 0 {
			t.Fatalf("run had errors: %v", res.Errors)
		}
		entries, err := os.ReadDir(storiesDir)
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		return res, names
	}

	r1, names1 := run()
	if r1.PagesPublished != 1 {
		t.Fatalf("run 1 PagesPublished = %d; want 1", r1.PagesPublished)
	}
	if _, err := os.Stat(filepath.Join(storiesDir, "old-group.md")); !os.IsNotExist(err) {
		t.Errorf("stale old-group.md should be pruned after run 1")
	}
	if _, err := os.Stat(filepath.Join(storiesDir, expectedSlug+".md")); err != nil {
		t.Errorf("expected generated page %s.md missing: %v", expectedSlug, err)
	}

	r2, names2 := run()
	if r2.PagesPublished != 1 {
		t.Fatalf("run 2 PagesPublished = %d; want 1 (no duplicates)", r2.PagesPublished)
	}
	if strings.Join(names1, ",") != strings.Join(names2, ",") {
		t.Errorf("file set changed between runs: %v vs %v", names1, names2)
	}
	if _, err := os.Stat(filepath.Join(storiesDir, "_index.md")); err != nil {
		t.Errorf("_index.md must be preserved: %v", err)
	}
}

// TestRunStoryPagesCleanerErrorIsolation verifies that a failing cleanup is
// reported in Errors without failing the publish result. (AC-4)
func TestRunStoryPagesCleanerErrorIsolation(t *testing.T) {
	repo := storage.NewMemoryRepository()
	_, err := repo.Upsert(domain.Story{
		ID:          "test-1",
		Title:       "Test Story",
		Source:      "test-source",
		PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		URL:         "https://example.com/test-1",
	})
	if err != nil {
		t.Fatalf("Failed to insert test story: %v", err)
	}

	app := New(
		WithRepository(repo),
		WithClusterer(domain.NewClusterer()),
	)

	publisher := &failingCleanerPublisher{publisher: &DefaultStoryPagePublisher{}}
	result := app.RunStoryPages(context.Background(), time.Now(), publisher)

	if result.PagesPublished != 1 {
		t.Errorf("PagesPublished = %d; want 1 despite cleanup failure", result.PagesPublished)
	}
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Error(), "simulated cleanup failure") {
		t.Errorf("Errors = %v; want single cleanup failure reported", result.Errors)
	}
}

// TestRunStoryPagesUnsafeMetadataRoundTrip feeds unsafe source metadata
// (colons and quotes in the headline) through cluster → page → file and
// verifies the front-matter decodes back to identical values. (AC-2, AC-4)
func TestRunStoryPagesUnsafeMetadataRoundTrip(t *testing.T) {
	repo := storage.NewMemoryRepository()
	headline := `Giants: "sweep" confirmed`
	for i, source := range []string{"src-a", "src-b"} {
		_, err := repo.Upsert(domain.Story{
			ID:          "u" + string(rune('0'+i)),
			Title:       headline,
			Source:      source,
			PublishedAt: time.Date(2024, 1, 1, i, 0, 0, 0, time.UTC),
			URL:         "https://" + source + ".com/1",
		})
		if err != nil {
			t.Fatalf("Failed to insert story %s: %v", source, err)
		}
	}

	app := New(
		WithRepository(repo),
		WithClusterer(domain.NewClusterer()),
	)

	outDir := filepath.Join(t.TempDir(), "web", "content")
	storiesDir := filepath.Join(outDir, "stories")
	publisher := hugo.NewPublisher(outDir, "")

	result := app.RunStoryPages(context.Background(), time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC), publisher)
	if len(result.Errors) > 0 {
		t.Fatalf("run had errors: %v", result.Errors)
	}
	if result.PagesPublished != 1 {
		t.Fatalf("PagesPublished = %d; want 1 (one cluster for two sources)", result.PagesPublished)
	}

	entries, err := os.ReadDir(storiesDir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("stories dir has %d files; want 1", len(entries))
	}
	data, err := os.ReadFile(filepath.Join(storiesDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	var fm struct {
		Title    string `yaml:"title"`
		Slug     string `yaml:"slug"`
		Articles []struct {
			Source    string `yaml:"source"`
			SourceURL string `yaml:"sourceURL"`
			Title     string `yaml:"title"`
		} `yaml:"articles"`
	}
	block, ok := extractFrontMatterBlock(data)
	if !ok {
		t.Fatalf("no front-matter block:\n%s", data)
	}
	if err := yaml.Unmarshal(block, &fm); err != nil {
		t.Fatalf("front-matter does not parse: %v\n%s", err, block)
	}
	if fm.Title != headline {
		t.Errorf("title = %q; want %q", fm.Title, headline)
	}
	if len(fm.Articles) != 2 {
		t.Fatalf("articles = %d; want 2", len(fm.Articles))
	}
	for i, want := range []string{"https://src-a.com/1", "https://src-b.com/1"} {
		if fm.Articles[i].SourceURL != want {
			t.Errorf("articles[%d].sourceURL = %q; want %q", i, fm.Articles[i].SourceURL, want)
		}
	}
}

// extractFrontMatterBlock returns the YAML bytes between the opening and
// closing --- delimiters of a Markdown document.
func extractFrontMatterBlock(data []byte) ([]byte, bool) {
	text := string(data)
	if !strings.HasPrefix(text, "---\n") {
		return nil, false
	}
	rest := text[4:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, false
	}
	return []byte(rest[:end]), true
}

type failingCleanerPublisher struct {
	publisher *DefaultStoryPagePublisher
}

func (p *failingCleanerPublisher) Publish(ctx context.Context, page domain.StoryDetailPage) error {
	return p.publisher.Publish(ctx, page)
}

func (p *failingCleanerPublisher) CleanupStale(ctx context.Context, pages []domain.StoryDetailPage) error {
	return &testError{msg: "simulated cleanup failure"}
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
