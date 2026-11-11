package hugo

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
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

	// Check required fields
	expectedFields := []string{
		"title: Test Story",
		"slug: test-story-1",
		"storyID: test-123",
		"description: Test Story",
		"source: multi",
		"canonical: /stories/test-story-1/",
		"firstPublished: 2024-01-01T10:00:00Z",
		"lastPublished: 2024-01-01T12:00:00Z",
		"firstSeen: 2024-01-01T09:00:00Z",
		"articles:",
		"  - source: ESPN",
		"    sourceURL: https://espn.com/1",
		"    title: Test Article 1",
		"    published: 2024-01-01T10:00:00Z",
		"  - source: NFL.com",
		"    sourceURL: https://nfl.com/2",
		"    title: Test Article 2",
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

	outDir := filepath.Join(tmpDir, "public", "_content", "stories")
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
	outPath := filepath.Join(outDir, "_content", "stories", slug+".md")
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

	outDir := filepath.Join(tmpDir, "public", "_content", "stories")
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
	outPath := filepath.Join(outDir, "_content", "stories", slug+".md")
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

	outDir := filepath.Join(tmpDir, "public", "_content", "stories")
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
