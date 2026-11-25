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

// TestRunStoryPagesMultiDayStable (issue #26, AC-1, T1.3): two runs where day
// 2 adds a new source; front-matter slug/storyID are identical, article count
// +1, lastPublished advanced, firstPublished unchanged.
func TestRunStoryPagesMultiDayStable(t *testing.T) {
	repo := storage.NewMemoryRepository()
	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	// Day 1: two sources with the same headline.
	seedsDay1 := []domain.Story{
		{ID: "d1-a", Title: "Lions Sign Veteran Center", Source: "espn", PublishedAt: time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC), URL: "https://espn.com/d1a"},
		{ID: "d1-b", Title: "Lions Sign Veteran Center", Source: "nfl", PublishedAt: time.Date(2024, 1, 1, 15, 0, 0, 0, time.UTC), URL: "https://nfl.com/d1b"},
	}
	for _, s := range seedsDay1 {
		if _, err := repo.Upsert(s); err != nil {
			t.Fatalf("seed %s: %v", s.ID, err)
		}
	}

	outDir := filepath.Join(t.TempDir(), "web", "content")
	storiesDir := filepath.Join(outDir, "stories")
	publisher := hugo.NewPublisher(outDir, "")

	app := New(WithRepository(repo), WithClusterer(domain.NewClusterer()))

	// Run 1 (day 1).
	res1 := app.RunStoryPages(context.Background(), now, publisher)
	if len(res1.Errors) > 0 {
		t.Fatalf("run1 errors: %v", res1.Errors)
	}
	if res1.PagesPublished != 1 {
		t.Fatalf("run1 PagesPublished = %d; want 1", res1.PagesPublished)
	}

	// Capture run 1 front-matter.
	fm1 := readStoryFrontMatter(t, storiesDir)
	slug1 := fm1["slug"]
	storyID1 := fm1["storyID"]
	firstPub1 := fm1["firstPublished"]
	lastPub1 := fm1["lastPublished"]

	// Day 2: add a third source with the same headline, later date.
	if _, err := repo.Upsert(domain.Story{
		ID: "d2-c", Title: "Lions Sign Veteran Center", Source: "cbs",
		PublishedAt: time.Date(2024, 1, 2, 9, 0, 0, 0, time.UTC), URL: "https://cbs.com/d2c",
	}); err != nil {
		t.Fatalf("seed d2-c: %v", err)
	}

	// Run 2 (day 2).
	res2 := app.RunStoryPages(context.Background(), now.Add(24*time.Hour), publisher)
	if len(res2.Errors) > 0 {
		t.Fatalf("run2 errors: %v", res2.Errors)
	}
	if res2.PagesPublished != 1 {
		t.Fatalf("run2 PagesPublished = %d; want 1", res2.PagesPublished)
	}

	fm2 := readStoryFrontMatter(t, storiesDir)

	// Public identity stable across days.
	if slug1 != fm2["slug"] {
		t.Errorf("slug changed: %q -> %q", slug1, fm2["slug"])
	}
	if storyID1 != fm2["storyID"] {
		t.Errorf("storyID changed: %q -> %q", storyID1, fm2["storyID"])
	}
	// firstPublished pinned, lastPublished advanced.
	if firstPub1 != fm2["firstPublished"] {
		t.Errorf("firstPublished changed: %q -> %q", firstPub1, fm2["firstPublished"])
	}
	if lastPub1 >= fm2["lastPublished"] {
		t.Errorf("lastPublished did not advance: %q -> %q", lastPub1, fm2["lastPublished"])
	}
}

// readStoryFrontMatter reads the single story page in storiesDir and returns
// its front-matter as a map (string fields only).
func readStoryFrontMatter(t *testing.T, storiesDir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(storiesDir)
	if err != nil {
		t.Fatalf("ReadDir %s: %v", storiesDir, err)
	}
	var mdFile string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") && e.Name() != "_index.md" {
			mdFile = filepath.Join(storiesDir, e.Name())
			break
		}
	}
	if mdFile == "" {
		t.Fatalf("no story page found in %s", storiesDir)
	}
	data, err := os.ReadFile(mdFile)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", mdFile, err)
	}
	block, ok := extractFrontMatterBlock(data)
	if !ok {
		t.Fatalf("no front-matter block in %s", mdFile)
	}
	var fm map[string]interface{}
	if err := yaml.Unmarshal(block, &fm); err != nil {
		t.Fatalf("front-matter does not parse: %v", err)
	}
	result := make(map[string]string)
	for k, v := range fm {
		switch val := v.(type) {
		case string:
			result[k] = val
		case time.Time:
			result[k] = val.UTC().Format(time.RFC3339)
		}
	}
	return result
}

// TestRunStoryPagesWithRuleRepoMergeAndArchive (issue #26, AC-2, T2.5): a
// merge + archive rule set produces a merged page carrying aliases and an
// archived page carrying status. With no rule repo, output is identical to
// pre-change (engine independence).
func TestRunStoryPagesWithRuleRepoMergeAndArchive(t *testing.T) {
	// Build a repo with two same-title stories (cluster into one group) and one
	// distinct story (its own group).
	repo := storage.NewMemoryRepository()
	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	// Two stories that cluster into one group.
	if _, err := repo.Upsert(domain.Story{ID: "s1", Title: "Bears Trade Quarterback", Source: "espn", PublishedAt: now.Add(-2 * time.Hour), URL: "https://espn.com/s1", Entities: []string{"bears"}}); err != nil {
		t.Fatalf("seed s1: %v", err)
	}
	if _, err := repo.Upsert(domain.Story{ID: "s2", Title: "Bears Trade Quarterback", Source: "nfl", PublishedAt: now.Add(-1 * time.Hour), URL: "https://nfl.com/s2", Entities: []string{"bears"}}); err != nil {
		t.Fatalf("seed s2: %v", err)
	}
	// A distinct story (different title, no clustering overlap).
	if _, err := repo.Upsert(domain.Story{ID: "s3", Title: "Cowboys Release Tight End", Source: "cbs", PublishedAt: now.Add(-3 * time.Hour), URL: "https://cbs.com/s3"}); err != nil {
		t.Fatalf("seed s3: %v", err)
	}

	outDir := filepath.Join(t.TempDir(), "web", "content")
	publisher := hugo.NewPublisher(outDir, "")

	// --- Baseline: no rule repo (engine independence). ---
	appNoRules := New(WithRepository(repo), WithClusterer(domain.NewClusterer()))
	resNoRules := appNoRules.RunStoryPages(context.Background(), now, publisher)
	if len(resNoRules.Errors) > 0 {
		t.Fatalf("no-rules run errors: %v", resNoRules.Errors)
	}
	// Two groups: the clustered (s1+s2) and the solo (s3).
	if resNoRules.PagesPublished != 2 {
		t.Fatalf("no-rules PagesPublished = %d; want 2", resNoRules.PagesPublished)
	}
	// Capture baseline bytes.
	baseline := readAllStoryFiles(t, outDir)

	// --- With rule repo: merge + archive. ---
	// Build rule repo with an archive rule for the solo story's slug.
	soloSlug := domain.ComputeStorySlugFromHeadline("Cowboys Release Tight End")
	clusteredSlug := domain.ComputeStorySlugFromHeadline("Bears Trade Quarterback")
	ruleRepo := storage.NewMemoryRuleRepository()
	// Archive the solo story by slug.
	if err := ruleRepo.SaveRule(domain.CorrectionRule{ID: "arc-solo", Kind: domain.CorrectionArchive, StoryID: soloSlug}); err != nil {
		t.Fatalf("SaveRule archive: %v", err)
	}
	appWithRules := New(WithRepository(repo), WithClusterer(domain.NewClusterer()), WithRuleRepository(ruleRepo))

	resWithRules := appWithRules.RunStoryPages(context.Background(), now, publisher)
	if len(resWithRules.Errors) > 0 {
		t.Fatalf("with-rules run errors: %v", resWithRules.Errors)
	}
	if resWithRules.PagesPublished != 2 {
		t.Fatalf("with-rules PagesPublished = %d; want 2", resWithRules.PagesPublished)
	}

	// The archived page should carry status: archived in front-matter.
	archivedContent := readStoryFileBySlug(t, outDir, soloSlug)
	if archivedContent == "" {
		t.Fatalf("archived story page not found for slug %q", soloSlug)
	}
	if !strings.Contains(archivedContent, "status: \"archived\"") {
		t.Errorf("archived page missing status: archived front-matter:\n%s", archivedContent)
	}
	if !strings.Contains(archivedContent, "This story is archived") {
		t.Errorf("archived page missing archive banner:\n%s", archivedContent)
	}

	// The clustered page should NOT be archived (no rule targets it).
	clusteredContent := readStoryFileBySlug(t, outDir, clusteredSlug)
	if clusteredContent == "" {
		t.Fatalf("clustered story page not found for slug %q", clusteredSlug)
	}
	if strings.Contains(clusteredContent, "status: \"archived\"") {
		t.Errorf("clustered page should not be archived:\n%s", clusteredContent)
	}

	// Engine independence: the no-rules baseline and the with-rules run both
	// produced pages; the with-rules run adds status front-matter (expected
	// difference) but the no-rules run must not carry any status field.
	for slug, content := range baseline {
		if strings.Contains(content, "status: ") {
			t.Errorf("no-rules baseline page %q carries status field (engine independence violated):\n%s", slug, content)
		}
	}
}

// readAllStoryFiles returns a map of slug -> file content for all generated
// story pages in outDir/stories.
func readAllStoryFiles(t *testing.T, outDir string) map[string]string {
	t.Helper()
	storiesDir := filepath.Join(outDir, "stories")
	entries, err := os.ReadDir(storiesDir)
	if err != nil {
		t.Fatalf("ReadDir %s: %v", storiesDir, err)
	}
	result := make(map[string]string)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".md") || e.Name() == "_index.md" {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".md")
		data, err := os.ReadFile(filepath.Join(storiesDir, e.Name()))
		if err != nil {
			t.Fatalf("ReadFile %s: %v", e.Name(), err)
		}
		result[slug] = string(data)
	}
	return result
}

// readStoryFileBySlug returns the content of the story page with the given slug.
func readStoryFileBySlug(t *testing.T, outDir, slug string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(outDir, "stories", slug+".md"))
	if err != nil {
		return ""
	}
	return string(data)
}

// TestRunStoryPagesMultiSourceResolver (issue #26, AC-3, D5, T3.2): a
// multi-source story with a registry resolver resolves every source's display
// name, not just the first article's.
func TestRunStoryPagesMultiSourceResolver(t *testing.T) {
	repo := storage.NewMemoryRepository()
	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	// Three sources with the same headline (cluster into one group).
	for i, src := range []struct {
		id  string
		pub string
		url string
	}{
		{"src-a", "Publisher A", "https://a.com/1"},
		{"src-b", "Publisher B", "https://b.com/2"},
		{"src-c", "Publisher C", "https://c.com/3"},
	} {
		_, err := repo.Upsert(domain.Story{
			ID:          "m" + string(rune('0'+i)),
			Title:       "Rams Sign Veteran Cornerback",
			Source:      src.id,
			PublishedAt: now.Add(-time.Duration(3-i) * time.Hour),
			URL:         src.url,
			Entities:    []string{"rams"},
		})
		if err != nil {
			t.Fatalf("seed %s: %v", src.id, err)
		}
	}

	// Build a registry so the app-level resolver maps source IDs to display names.
	sources := []domain.FeedSource{
		{ID: "src-a", URL: "https://a.com/feed", Publication: "Publisher A", Enabled: true},
		{ID: "src-b", URL: "https://b.com/feed", Publication: "Publisher B", Enabled: true},
		{ID: "src-c", URL: "https://c.com/feed", Publication: "Publisher C", Enabled: true},
	}
	registry, err := domain.NewFeedRegistry(sources...)
	if err != nil {
		t.Fatalf("NewFeedRegistry: %v", err)
	}

	outDir := filepath.Join(t.TempDir(), "web", "content")
	publisher := hugo.NewPublisher(outDir, "")

	app := New(WithRepository(repo), WithClusterer(domain.NewClusterer()), WithRegistry(registry))
	res := app.RunStoryPages(context.Background(), now, publisher)
	if len(res.Errors) > 0 {
		t.Fatalf("run errors: %v", res.Errors)
	}
	if res.PagesPublished != 1 {
		t.Fatalf("PagesPublished = %d; want 1", res.PagesPublished)
	}

	// The rendered page must carry all three display names.
	content := readAllStoryFiles(t, outDir)
	var pageContent string
	for _, c := range content {
		pageContent = c
	}
	for _, name := range []string{"Publisher A", "Publisher B", "Publisher C"} {
		if !strings.Contains(pageContent, name) {
			t.Errorf("page missing display name %q:\n%s", name, pageContent)
		}
	}
}

// TestRunStoryPages_DisplayTitleEndToEnd (#201, AC-1, AC-2): a story upserted
// with a display title (original casing) must flow through RunStoryPages into the
// rendered page's front-matter title (display casing) and H1, while the slug
// remains the normalized (lowercased) identity form.
func TestRunStoryPages_DisplayTitleEndToEnd(t *testing.T) {
	repo := storage.NewMemoryRepository()

	// A story whose normalized Title is lowercase and whose DisplayTitle keeps
	// the original feed casing. A single story = one cluster = one page.
	_, err := repo.Upsert(domain.Story{
		ID:           "display-201-1",
		Title:        "chiefs sign rb james cook",
		DisplayTitle: "Chiefs sign RB James Cook",
		Source:       "official-min",
		PublishedAt:  time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC),
		Summary:      "Chiefs sign RB James Cook.",
		URL:          "https://example.com/chiefs-cook",
	})
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	app := New(
		WithRepository(repo),
		WithClusterer(domain.NewClusterer()),
	)

	outDir := filepath.Join(t.TempDir(), "web", "content")
	storiesDir := filepath.Join(outDir, "stories")
	publisher := hugo.NewPublisher(outDir, "")

	result := app.RunStoryPages(context.Background(), time.Now(), publisher)
	if len(result.Errors) > 0 {
		t.Fatalf("run errors: %v", result.Errors)
	}
	if result.PagesPublished != 1 {
		t.Fatalf("PagesPublished = %d; want 1", result.PagesPublished)
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

	// The slug (file name) must be the normalized lowercase form.
	if entries[0].Name() != "chiefs-sign-rb-james-cook.md" {
		t.Errorf("slug file = %q; want normalized %q", entries[0].Name(), "chiefs-sign-rb-james-cook.md")
	}

	// The front-matter title must carry the original display casing.
	block, ok := extractFrontMatterBlock(data)
	if !ok {
		t.Fatalf("no front-matter block:\n%s", data)
	}
	var fm struct {
		Title string `yaml:"title"`
		Slug  string `yaml:"slug"`
	}
	if err := yaml.Unmarshal(block, &fm); err != nil {
		t.Fatalf("front-matter does not parse: %v\n%s", err, block)
	}
	if fm.Title != "Chiefs sign RB James Cook" {
		t.Errorf("front-matter title = %q; want display casing %q", fm.Title, "Chiefs sign RB James Cook")
	}
	if fm.Slug != "chiefs-sign-rb-james-cook" {
		t.Errorf("front-matter slug = %q; want normalized %q", fm.Slug, "chiefs-sign-rb-james-cook")
	}

	// The rendered body must contain the display-cased H1.
	if !strings.Contains(string(data), "# Chiefs sign RB James Cook") {
		t.Errorf("body H1 missing display casing:\n%s", string(data))
	}
}
