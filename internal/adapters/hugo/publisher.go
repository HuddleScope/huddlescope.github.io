// Package hugo provides a concrete StoryPagePublisher that renders and commits story pages as Hugo content files.
package hugo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// Publisher implements domain.StoryPagePublisher by writing a story detail page as a Hugo content file.
type Publisher struct {
	OutDir  string
	BaseURL string
}

// Publish writes the story detail page as a Markdown file with YAML front-matter.
func (p *Publisher) Publish(ctx context.Context, page domain.StoryDetailPage) error {
	// Validate inputs
	if p.OutDir == "" {
		return fmt.Errorf("OutDir must not be empty")
	}
	if page.Path == "" || page.Slug == "" {
		return fmt.Errorf("page Path and Slug must not be empty")
	}

	// Render the content
	markdown, err := RenderMarkdown(page, p.BaseURL)
	if err != nil {
		return fmt.Errorf("render failed: %w", err)
	}

	// Compute output path
	slug := string(page.Slug)
	outPath := filepath.Join(p.OutDir, "_content", "stories", slug+".md")

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("mkdir failed: %w", err)
	}

	// Write atomically: temp file then rename
	tmpPath := outPath + ".tmp"
	if err := os.WriteFile(tmpPath, markdown, 0o644); err != nil {
		return fmt.Errorf("temp write failed: %w", err)
	}

	if err := os.Rename(tmpPath, outPath); err != nil {
		// Clean up temp on failure
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename failed: %w", err)
	}

	return nil
}

// RenderMarkdown produces the Markdown content with YAML front-matter.
func RenderMarkdown(page domain.StoryDetailPage, baseURL string) ([]byte, error) {
	var buf strings.Builder

	// YAML front-matter
	fmt.Fprintf(&buf, "---\n")
	fmt.Fprintf(&buf, "title: %s\n", escapeYamlString(page.Headline))
	fmt.Fprintf(&buf, "slug: %s\n", page.Slug)
	fmt.Fprintf(&buf, "storyID: %s\n", page.StoryID)
	fmt.Fprintf(&buf, "description: %s\n", escapeYamlString(page.Headline))
	fmt.Fprintf(&buf, "source: multi\n")
	fmt.Fprintf(&buf, "canonical: %s\n", page.Path)
	fmt.Fprintf(&buf, "firstPublished: %s\n", formatTime(page.FirstPublishedAt))
	fmt.Fprintf(&buf, "lastPublished: %s\n", formatTime(page.LastPublishedAt))
	fmt.Fprintf(&buf, "firstSeen: %s\n", formatTime(page.FirstSeenAt))
	fmt.Fprintf(&buf, "articles:\n")

	// Article entries
	for _, art := range page.Articles {
		fmt.Fprintf(&buf, "  - source: %s\n", escapeYamlString(art.SourceName))
		fmt.Fprintf(&buf, "    sourceURL: %s\n", escapeYamlString(art.URL))
		fmt.Fprintf(&buf, "    title: %s\n", escapeYamlString(art.Title))
		fmt.Fprintf(&buf, "    published: %s\n", formatTime(art.PublishedAt))
	}

	fmt.Fprintf(&buf, "---\n\n")

	// Content body
	fmt.Fprintf(&buf, "# %s\n\n", page.Headline)
	fmt.Fprintf(&buf, "This page aggregates reporting from multiple sources on this story.\n\n")
	fmt.Fprintf(&buf, "## Reporting Sources\n\n")

	for _, art := range page.Articles {
		fmt.Fprintf(&buf, "### %s\n\n", art.SourceName)
		fmt.Fprintf(&buf, "**Title:** [%s](%s)\n\n", art.Title, art.URL)
		fmt.Fprintf(&buf, "**Published:** %s\n\n", formatTime(art.PublishedAt))
		fmt.Fprintf(&buf, "[View original article](%s)\n\n", art.URL)
	}

	fmt.Fprintf(&buf, "## Story Timeline\n\n")
	fmt.Fprintf(&buf, "- First published: %s\n", formatTime(page.FirstPublishedAt))
	fmt.Fprintf(&buf, "- Last published: %s\n", formatTime(page.LastPublishedAt))

	if baseURL != "" {
		fmt.Fprintf(&buf, "\n[Back to stories](%s)\n", baseURL+"/stories/")
	}

	return []byte(buf.String()), nil
}

// formatTime produces a stable formatted time string.
func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// escapeYamlString escapes a string for YAML double-quoted strings.
func escapeYamlString(s string) string {
	if s == "" {
		return ""
	}
	// Simple escaping: replace backslash, double-quote, newline
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

// NoopPublisher is a no-op publisher for testing or offline scenarios.
type NoopPublisher struct{}

func (p *NoopPublisher) Publish(ctx context.Context, page domain.StoryDetailPage) error {
	// No-op: in production, this would commit to Hugo or CMS
	return nil
}

// NewPublisher creates a new Publisher with the given output directory and base URL.
func NewPublisher(outDir, baseURL string) *Publisher {
	return &Publisher{
		OutDir:  outDir,
		BaseURL: baseURL,
	}
}