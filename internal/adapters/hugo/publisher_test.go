package hugo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
	"gopkg.in/yaml.v3"
)

func TestRenderMarkdown(t *testing.T) {
	page := domain.StoryDetailPage{
		Slug:             "test-story-1",
		Path:             "/stories/test-story-1/",
		StoryID:          "test-123",
		Headline:         "Test Story",
		FirstPublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		LastPublishedAt:  time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		FirstSeenAt:      time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC),
		Articles: []domain.PublisherArticle{
			{
				SourceID:    "espn-nfl",
				SourceName:  "ESPN",
				Title:       "Test Article 1",
				URL:         "https://espn.com/1",
				PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
			},
			{
				SourceID:    "nfl-com",
				SourceName:  "NFL.com",
				Title:       "Test Article 2",
				URL:         "https://nfl.com/2",
				PublishedAt: time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
			},
		},
	}

	baseURL := "https://huddlescope.github.io"
	markdown, err := RenderMarkdown(page, baseURL)
	if err != nil {
		t.Fatalf("RenderMarkdown failed: %v", err)
	}

	result := string(markdown)

	// Check required fields (string values are double-quoted for YAML safety)
	expectedFields := []string{
		"title: \"Test Story\"",
		"slug: \"test-story-1\"",
		"storyID: \"test-123\"",
		"description: \"Test Story\"",
		"source: multi",
		"canonical: \"/stories/test-story-1/\"",
		"generator: huddlescope",
		"firstPublished: 2024-01-01T10:00:00Z",
		"lastPublished: 2024-01-01T12:00:00Z",
		"firstSeen: 2024-01-01T09:00:00Z",
		"articles:",
		"  - source: \"ESPN\"",
		"    sourceURL: \"https://espn.com/1\"",
		"    title: \"Test Article 1\"",
		"    published: 2024-01-01T10:00:00Z",
		"  - source: \"NFL.com\"",
		"    sourceURL: \"https://nfl.com/2\"",
		"    title: \"Test Article 2\"",
		"    published: 2024-01-01T11:00:00Z",
		"## Reporting Sources",
		"ESPN",
		"[View original article](https://espn.com/1)",
		"NFL.com",
		"[View original article](https://nfl.com/2)",
		"## Story Timeline",
		"- First published: 2024-01-01T10:00:00Z",
		"- Last published: 2024-01-01T12:00:00Z",
		"[Back to stories](https://huddlescope.github.io/stories/)",
	}

	for _, f := range expectedFields {
		if !contains(result, f) {
			t.Errorf("Missing expected field in rendered markdown:\n%s\n\nMissing: %q", result, f)
		}
	}
}

func TestRenderMarkdownNoBodyOrImages(t *testing.T) {
	// AC3: metadata only - no article body or unlicensed images
	page := domain.StoryDetailPage{
		Slug:     "test-page",
		Path:     "/stories/test-page/",
		StoryID:  "test-123",
		Headline: "Test",
		Articles: []domain.PublisherArticle{{Title: "Art", URL: "https://example.com"}},
	}

	markdown, _ := RenderMarkdown(page, "")

	// Ensure no "<body>" or image tags
	if contains(string(markdown), "<body") {
		t.Error("Rendered markdown contains <body> tag (should be metadata only)")
	}
	if contains(string(markdown), ".jpg") || contains(string(markdown), ".png") || contains(string(markdown), ".gif") {
		t.Error("Rendered markdown contains image references (should be metadata only)")
	}
}

func TestPublishSuccess(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hugo-publisher-test-*")
	if err != nil {
		t.Fatalf("Temp dir creation failed: %v", err)
	}
	_ = os.RemoveAll(tmpDir)

	outDir := filepath.Join(tmpDir, "web", "content")
	p := NewPublisher(outDir, "https://huddlescope.github.io")

	page := domain.StoryDetailPage{
		Slug:             "test-page",
		Path:             "/stories/test-page/",
		StoryID:          "test-123",
		Headline:         "Test Page",
		FirstPublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		LastPublishedAt:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		FirstSeenAt:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Articles:         []domain.PublisherArticle{{SourceID: "espn", Title: "Test", URL: "https://espn.com", PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}},
	}

	err = p.Publish(context.Background(), page)
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Verify file exists
	slug := string(page.Slug)
	outPath := filepath.Join(outDir, "stories", slug+".md")
	if _, err := os.Stat(outPath); os.IsNotExist(err) {
		t.Errorf("Expected output file not created at %s", outPath)
	}
}

func TestPublishIdempotent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hugo-idempotent-*")
	if err != nil {
		t.Fatalf("Temp dir creation failed: %v", err)
	}
	_ = os.RemoveAll(tmpDir)

	outDir := filepath.Join(tmpDir, "web", "content")
	p := NewPublisher(outDir, "https://huddlescope.github.io")

	page := domain.StoryDetailPage{
		Slug:             "test-page",
		Path:             "/stories/test-page/",
		StoryID:          "test-123",
		Headline:         "Test Page",
		FirstPublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		LastPublishedAt:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		FirstSeenAt:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Articles:         []domain.PublisherArticle{{SourceID: "espn", Title: "Test", URL: "https://espn.com", PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}},
	}

	// First publish
	err = p.Publish(context.Background(), page)
	if err != nil {
		t.Fatalf("First publish failed: %v", err)
	}

	// Read first content
	slug := string(page.Slug)
	outPath := filepath.Join(outDir, "stories", slug+".md")
	content1, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("Read first content failed: %v", err)
	}

	// Second publish (same page)
	err = p.Publish(context.Background(), page)
	if err != nil {
		t.Fatalf("Second publish failed: %v", err)
	}

	// Compare content
	content2, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("Read second content failed: %v", err)
	}

	if string(content1) != string(content2) {
		t.Error("Publish not idempotent: output differs between runs")
	}
}

func TestPublishEmptyErrors(t *testing.T) {
	p := NewPublisher("", "https://huddlescope.github.io")
	page := domain.StoryDetailPage{
		Slug:     "test",
		Path:     "/stories/test/",
		StoryID:  "test",
		Headline: "Test",
	}

	err := p.Publish(context.Background(), page)
	if err == nil {
		t.Error("Expected error for empty OutDir")
	}
}

func TestPublishBadDir(t *testing.T) {
	p := NewPublisher("/nonexistent/path/that/should/not/exist", "https://huddlescope.github.io")
	page := domain.StoryDetailPage{
		Slug:     "test",
		Path:     "/stories/test/",
		StoryID:  "test",
		Headline: "Test",
	}

	err := p.Publish(context.Background(), page)
	if err == nil {
		t.Error("Expected error for non-existent directory")
	}
}

func TestNoopPublisher(t *testing.T) {
	var p NoopPublisher
	page := domain.StoryDetailPage{
		Slug:     "test",
		Path:     "/stories/test/",
		StoryID:  "test",
		Headline: "Test",
	}

	// Should succeed without writing anything
	err := p.Publish(context.Background(), page)
	if err != nil {
		t.Fatalf("NoopPublisher.Publish failed: %v", err)
	}
}

func TestPublishEmptyPageErrors(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hugo-empty-*")
	if err != nil {
		t.Fatalf("Temp dir creation failed: %v", err)
	}
	_ = os.RemoveAll(tmpDir)

	outDir := filepath.Join(tmpDir, "web", "content")
	p := NewPublisher(outDir, "https://huddlescope.github.io")

	page := domain.StoryDetailPage{
		Slug:     "", // Empty slug
		Path:     "",
		StoryID:  "",
		Headline: "",
	}

	err = p.Publish(context.Background(), page)
	if err == nil {
		t.Error("Expected error for empty page fields")
	}
}

func TestRenderMarkdownUnsafeValuesRoundTrip(t *testing.T) {
	// AC4: unsafe source metadata (colons, quotes, leading indicators, newlines,
	// backslashes, empty values) must survive a YAML parse with identical values.
	tests := []struct {
		name         string
		page         domain.StoryDetailPage
		wantTitle    string
		wantSource   string
		wantURL      string
		wantArtTitle string
	}{
		{
			name: "headline-with-colon",
			page: domain.StoryDetailPage{
				Slug: "a-b", Path: "/stories/a-b/", StoryID: "id-1",
				Headline: "A: B reacts to C: D",
				Articles: []domain.PublisherArticle{{SourceID: "s", SourceName: "Src", Title: "t", URL: "https://x.com/a", PublishedAt: time.Now().UTC().Truncate(time.Second)}},
			},
			wantTitle:  "A: B reacts to C: D",
			wantSource: "Src", wantURL: "https://x.com/a", wantArtTitle: "t",
		},
		{
			name: "title-leading-quote",
			page: domain.StoryDetailPage{
				Slug: "q", Path: "/stories/q/", StoryID: "id-2",
				Headline: `"Quoted" headline`,
				Articles: []domain.PublisherArticle{{SourceID: "s", SourceName: `"Quoted"`, Title: `said "hi"`, URL: "https://x.com/q", PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}},
			},
			wantTitle:  `"Quoted" headline`,
			wantSource: `"Quoted"`, wantURL: "https://x.com/q", wantArtTitle: `said "hi"`,
		},
		{
			name: "leading-indicators",
			page: domain.StoryDetailPage{
				Slug: "ind", Path: "/stories/ind/", StoryID: "id-3",
				Headline: "- leading dash # and & anchor",
				Articles: []domain.PublisherArticle{{SourceID: "-s", SourceName: "#hash", Title: "&amp", URL: "https://x.com/ind", PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}},
			},
			wantTitle:  "- leading dash # and & anchor",
			wantSource: "#hash", wantURL: "https://x.com/ind", wantArtTitle: "&amp",
		},
		{
			name: "embedded-newline-and-backslash",
			page: domain.StoryDetailPage{
				Slug: "nl", Path: "/stories/nl/", StoryID: "id-4",
				Headline: "line1\nline2\\back",
				Articles: []domain.PublisherArticle{{SourceID: "s", SourceName: "S", Title: "a\nb\\c", URL: "https://x.com/nl", PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}},
			},
			wantTitle:  "line1\nline2\\back",
			wantSource: "S", wantURL: "https://x.com/nl", wantArtTitle: "a\nb\\c",
		},
		{
			name: "empty-strings",
			page: domain.StoryDetailPage{
				Slug: "empty", Path: "/stories/empty/", StoryID: "id-5",
				Headline: "",
				Articles: []domain.PublisherArticle{{SourceID: "s", SourceName: "", Title: "", URL: "https://x.com/empty", PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}},
			},
			wantTitle:  "",
			wantSource: "", wantURL: "https://x.com/empty", wantArtTitle: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			markdown, err := RenderMarkdown(tt.page, "")
			if err != nil {
				t.Fatalf("RenderMarkdown failed: %v", err)
			}

			frontMatter, ok := extractFrontMatter(markdown)
			if !ok {
				t.Fatalf("no YAML front-matter block in rendered markdown:\n%s", markdown)
			}

			var fm struct {
				Title       string `yaml:"title"`
				Slug        string `yaml:"slug"`
				StoryID     string `yaml:"storyID"`
				Description string `yaml:"description"`
				Canonical   string `yaml:"canonical"`
				Generator   string `yaml:"generator"`
				Articles    []struct {
					Source    string `yaml:"source"`
					SourceURL string `yaml:"sourceURL"`
					Title     string `yaml:"title"`
				} `yaml:"articles"`
			}
			if err := yaml.Unmarshal(frontMatter, &fm); err != nil {
				t.Fatalf("front-matter does not parse as YAML: %v\n%s", err, frontMatter)
			}

			if fm.Title != tt.wantTitle {
				t.Errorf("title = %q; want %q", fm.Title, tt.wantTitle)
			}
			if fm.Description != tt.wantTitle {
				t.Errorf("description = %q; want %q (headline)", fm.Description, tt.wantTitle)
			}
			if fm.Slug != string(tt.page.Slug) {
				t.Errorf("slug = %q; want %q", fm.Slug, string(tt.page.Slug))
			}
			if fm.Canonical != string(tt.page.Path) {
				t.Errorf("canonical = %q; want %q", fm.Canonical, string(tt.page.Path))
			}
			if fm.Generator != "huddlescope" {
				t.Errorf("generator = %q; want %q", fm.Generator, "huddlescope")
			}
			if len(fm.Articles) != 1 {
				t.Fatalf("articles = %d; want 1", len(fm.Articles))
			}
			if fm.Articles[0].Source != tt.wantSource {
				t.Errorf("articles[0].source = %q; want %q", fm.Articles[0].Source, tt.wantSource)
			}
			if fm.Articles[0].SourceURL != tt.wantURL {
				t.Errorf("articles[0].sourceURL = %q; want %q", fm.Articles[0].SourceURL, tt.wantURL)
			}
			if fm.Articles[0].Title != tt.wantArtTitle {
				t.Errorf("articles[0].title = %q; want %q", fm.Articles[0].Title, tt.wantArtTitle)
			}
		})
	}
}

// extractFrontMatter returns the bytes between the opening and closing ---
// delimiters of a Markdown document.
func extractFrontMatter(data []byte) ([]byte, bool) {
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

func TestCleanupStaleRemovesOnlyGeneratedStaleFiles(t *testing.T) {
	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "web", "content")
	storiesDir := filepath.Join(outDir, "stories")
	if err := os.MkdirAll(storiesDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	p := NewPublisher(outDir, "https://huddlescope.github.io")

	// Currently published page: kept even if stale-looking
	live := domain.StoryDetailPage{Slug: "live-story", Path: "/stories/live-story/", StoryID: "live"}
	if err := p.Publish(context.Background(), live); err != nil {
		t.Fatalf("Publish live failed: %v", err)
	}

	// Stale generated page (marker present, slug not in set): must be removed
	stalePath := filepath.Join(storiesDir, "stale-story.md")
	staleContent, err := RenderMarkdown(domain.StoryDetailPage{Slug: "stale-story", Path: "/stories/stale-story/", StoryID: "stale"}, "")
	if err != nil {
		t.Fatalf("RenderMarkdown stale failed: %v", err)
	}
	if err := os.WriteFile(stalePath, staleContent, 0o644); err != nil {
		t.Fatalf("WriteFile stale failed: %v", err)
	}

	// Hand-authored page (no marker): must be kept
	manualPath := filepath.Join(storiesDir, "manual-story.md")
	if err := os.WriteFile(manualPath, []byte("---\ntitle: Manual\nslug: manual-story\n---\n\nBody\n"), 0o644); err != nil {
		t.Fatalf("WriteFile manual failed: %v", err)
	}

	// Collection index: must be kept
	indexPath := filepath.Join(storiesDir, "_index.md")
	if err := os.WriteFile(indexPath, []byte("---\ntitle: Stories\n---\n"), 0o644); err != nil {
		t.Fatalf("WriteFile index failed: %v", err)
	}

	err = p.CleanupStale(context.Background(), []domain.StoryDetailPage{live})
	if err != nil {
		t.Fatalf("CleanupStale failed: %v", err)
	}

	for _, name := range []string{"live-story.md", "manual-story.md", "_index.md"} {
		if _, err := os.Stat(filepath.Join(storiesDir, name)); err != nil {
			t.Errorf("expected %s to be kept: %v", name, err)
		}
	}
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Errorf("expected stale-story.md to be removed (err=%v)", err)
	}
}

func TestCleanupStaleKeepsUnparseableGeneratedFile(t *testing.T) {
	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "web", "content")
	storiesDir := filepath.Join(outDir, "stories")
	if err := os.MkdirAll(storiesDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	// Marker outside any front-matter block: not a valid generated page → keep
	noFM := filepath.Join(storiesDir, "broken.md")
	if err := os.WriteFile(noFM, []byte("# no front matter\n\n"+generatorMarker+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	p := NewPublisher(outDir, "")
	if err := p.CleanupStale(context.Background(), nil); err != nil {
		t.Fatalf("CleanupStale failed: %v", err)
	}
	if _, err := os.Stat(noFM); err != nil {
		t.Errorf("expected file without front-matter to be kept: %v", err)
	}
}

func TestCleanupStaleNoDirNoop(t *testing.T) {
	p := NewPublisher(filepath.Join(t.TempDir(), "missing"), "")
	if err := p.CleanupStale(context.Background(), nil); err != nil {
		t.Errorf("CleanupStale on missing dir = %v; want nil", err)
	}
}

func TestCleanupStaleEmptyOutDirErrors(t *testing.T) {
	p := NewPublisher("", "")
	if err := p.CleanupStale(context.Background(), nil); err == nil {
		t.Error("CleanupStale with empty OutDir; want error")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || len(haystack) > len(needle) && findSubstring(haystack, needle))
}

func findSubstring(haystack, needle string) bool {
	for i := 0; i <= len(haystack)-len(needle); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// TestRenderMarkdownStatusAndAliases (issue #26, AC-2, T2.6): front-matter
// status + aliases render, archive banner and "Merged from" section appear.
func TestRenderMarkdownStatusAndAliases(t *testing.T) {
	page := domain.StoryDetailPage{
		Slug:             "merged-story",
		Path:             "/stories/merged-story/",
		StoryID:          "internal-hash",
		PageKey:          "rep-derived-key",
		Status:           domain.StatusArchived,
		Headline:         "Merged Story",
		FirstPublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		LastPublishedAt:  time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC),
		FirstSeenAt:      time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC),
		Aliases: []domain.StoryAlias{
			{Slug: "source-story-a", Headline: "Source A", MergedAt: "2024-01-02T10:00:00Z"},
			{Slug: "source-story-b", Headline: "Source B", MergedAt: "2024-01-02T10:00:00Z"},
		},
		Articles: []domain.PublisherArticle{{SourceID: "espn", SourceName: "ESPN", Title: "Art", URL: "https://espn.com/a", PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)}},
	}

	markdown, err := RenderMarkdown(page, "")
	if err != nil {
		t.Fatalf("RenderMarkdown failed: %v", err)
	}
	result := string(markdown)

	// storyID renders the PageKey (D1), not the internal hash.
	if !contains(result, `storyID: "rep-derived-key"`) {
		t.Errorf("storyID should render PageKey, got:\n%s", result)
	}
	if contains(result, "internal-hash") {
		t.Errorf("internal StoryID hash must not be rendered:\n%s", result)
	}
	// status front-matter.
	if !contains(result, `status: "archived"`) {
		t.Errorf("status front-matter missing:\n%s", result)
	}
	// aliases front-matter (Hugo-native).
	if !contains(result, "aliases:") {
		t.Errorf("aliases front-matter missing:\n%s", result)
	}
	if !contains(result, `- "source-story-a"`) || !contains(result, `- "source-story-b"`) {
		t.Errorf("aliases entries missing:\n%s", result)
	}
	// Archive banner in body.
	if !contains(result, "This story is archived") {
		t.Errorf("archive banner missing:\n%s", result)
	}
	// Merged-from section in body.
	if !contains(result, "## Merged From") {
		t.Errorf("Merged From section missing:\n%s", result)
	}
}

// TestRenderMarkdownActiveStatusOmitted (issue #26, D7): an active story (zero
// or active status) renders no status front-matter, keeping output
// byte-identical to pre-issue #26 for active pages.
func TestRenderMarkdownActiveStatusOmitted(t *testing.T) {
	page := domain.StoryDetailPage{
		Slug:     "active-story",
		Path:     "/stories/active-story/",
		StoryID:  "hash",
		PageKey:  "key",
		Status:   domain.StatusActive,
		Headline: "Active Story",
		Articles: []domain.PublisherArticle{{SourceID: "espn", SourceName: "ESPN", Title: "Art", URL: "https://espn.com/a"}},
	}
	markdown, _ := RenderMarkdown(page, "")
	if contains(string(markdown), "status:") {
		t.Errorf("active page must not render status front-matter:\n%s", markdown)
	}
	// Zero status (unset) also renders no status.
	pageZero := page
	pageZero.Status = ""
	markdownZero, _ := RenderMarkdown(pageZero, "")
	if contains(string(markdownZero), "status:") {
		t.Errorf("zero-status page must not render status front-matter:\n%s", markdownZero)
	}
}

// TestRenderMarkdownAliasCap (issue #26, risk #3, T2.6): alias front-matter and
// body are capped at 20.
func TestRenderMarkdownAliasCap(t *testing.T) {
	// Build 25 aliases.
	aliases := make([]domain.StoryAlias, 25)
	for i := range aliases {
		aliases[i] = domain.StoryAlias{Slug: domain.StorySlug(fmt.Sprintf("alias-%02d", i)), MergedAt: "2024-01-01T00:00:00Z"}
	}
	page := domain.StoryDetailPage{
		Slug: "capped", Path: "/stories/capped/", StoryID: "h", PageKey: "k",
		Headline: "Capped", Aliases: aliases,
		Articles: []domain.PublisherArticle{{SourceID: "s", SourceName: "S", Title: "t", URL: "https://x.com/c"}},
	}
	markdown, _ := RenderMarkdown(page, "")
	result := string(markdown)

	// Count alias entries in front-matter (lines starting with "  - "alias-").
	fmCount := 0
	for _, line := range strings.Split(result, "\n") {
		if strings.HasPrefix(line, `  - "alias-`) {
			fmCount++
		}
	}
	if fmCount != 20 {
		t.Errorf("front-matter alias count = %d; want 20 (capped)", fmCount)
	}
	// Body "Merged From" section also capped at 20. Count all body alias list
	// items (lines starting with "- alias-") — these only appear in the
	// Merged From section (front-matter entries are indented and quoted).
	bodyCount := 0
	for _, line := range strings.Split(result, "\n") {
		if strings.HasPrefix(line, "- alias-") {
			bodyCount++
		}
	}
	if bodyCount != 20 {
		t.Errorf("body alias count = %d; want 20 (capped)", bodyCount)
	}
}

// TestRenderMarkdownClosedStatus (issue #26, AC-2): a closed story renders the
// status and an archive-style banner.
func TestRenderMarkdownClosedStatus(t *testing.T) {
	page := domain.StoryDetailPage{
		Slug: "closed-story", Path: "/stories/closed-story/", StoryID: "h", PageKey: "k",
		Status: domain.StatusClosed, Headline: "Closed Story",
		Articles: []domain.PublisherArticle{{SourceID: "s", SourceName: "S", Title: "t", URL: "https://x.com/c"}},
	}
	markdown, _ := RenderMarkdown(page, "")
	result := string(markdown)
	if !contains(result, `status: "closed"`) {
		t.Errorf("closed status front-matter missing:\n%s", result)
	}
	if !contains(result, "This story is archived") {
		t.Errorf("closed story should render archive-style banner:\n%s", result)
	}
}
