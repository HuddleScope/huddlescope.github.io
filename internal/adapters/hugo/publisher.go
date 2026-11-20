// Package hugo provides a concrete StoryPagePublisher that renders and commits story pages as Hugo content files.
package hugo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// generatorValue is the front-matter field value identifying pages generated
// by huddlescope.
const generatorValue = "huddlescope"

// generatorMarker is the full front-matter line identifying generated pages.
// Stale cleanup only ever deletes files carrying this marker, so
// hand-authored pages and test fixtures are never removed.
const generatorMarker = "generator: " + generatorValue

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

	// Compute output path: OutDir is the Hugo content directory (e.g. web/content)
	slug := string(page.Slug)
	outPath := filepath.Join(p.OutDir, "stories", slug+".md")

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(outPath), 0o750); err != nil {
		return fmt.Errorf("mkdir failed: %w", err)
	}

	// Write atomically: temp file then rename
	tmpPath := outPath + ".tmp"
	// #nosec G306: published story content is public static site content and must stay world-readable
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
	fmt.Fprintf(&buf, "title: %s\n", yamlString(page.Headline))
	fmt.Fprintf(&buf, "slug: %s\n", yamlString(string(page.Slug)))
	fmt.Fprintf(&buf, "storyID: %s\n", yamlString(page.StoryID))
	fmt.Fprintf(&buf, "description: %s\n", yamlString(page.Headline))
	fmt.Fprintf(&buf, "source: multi\n")
	fmt.Fprintf(&buf, "canonical: %s\n", yamlString(string(page.Path)))
	fmt.Fprintf(&buf, "generator: %s\n", generatorValue)
	fmt.Fprintf(&buf, "firstPublished: %s\n", formatTime(page.FirstPublishedAt))
	fmt.Fprintf(&buf, "lastPublished: %s\n", formatTime(page.LastPublishedAt))
	fmt.Fprintf(&buf, "firstSeen: %s\n", formatTime(page.FirstSeenAt))
	fmt.Fprintf(&buf, "articles:\n")

	// Article entries
	for _, art := range page.Articles {
		fmt.Fprintf(&buf, "  - source: %s\n", yamlString(art.SourceName))
		fmt.Fprintf(&buf, "    sourceURL: %s\n", yamlString(art.URL))
		fmt.Fprintf(&buf, "    title: %s\n", yamlString(art.Title))
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

// escapeYamlString escapes a string for use inside YAML double-quoted strings.
func escapeYamlString(s string) string {
	// Simple escaping: replace backslash, double-quote, newline
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

// yamlString renders a string as a YAML double-quoted scalar so values with
// colons, leading indicators, quotes or empty strings round-trip unchanged.
func yamlString(s string) string {
	return "\"" + escapeYamlString(s) + "\""
}

// CleanupStale removes generated story pages whose slugs are no longer part of
// the published set. Only files carrying the generator marker are deleted;
// _index.md, hand-authored pages and unparseable files are always kept.
// It returns an error describing per-file failures so callers can report them
// without failing the export.
func (p *Publisher) CleanupStale(ctx context.Context, pages []domain.StoryDetailPage) error {
	if p.OutDir == "" {
		return fmt.Errorf("OutDir must not be empty")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	storiesDir := filepath.Join(p.OutDir, "stories")
	entries, err := os.ReadDir(storiesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // directory does not exist yet; nothing to clean
		}
		return fmt.Errorf("read stories dir failed: %w", err)
	}

	current := make(map[string]bool, len(pages))
	for _, page := range pages {
		current[string(page.Slug)] = true
	}

	var errs []error
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".md") || name == "_index.md" {
			continue // collection index and non-markdown files are never touched
		}
		slug := strings.TrimSuffix(name, ".md")
		if current[slug] {
			continue // still part of the published set
		}

		data, err := os.ReadFile(filepath.Join(storiesDir, name))
		if err != nil {
			errs = append(errs, fmt.Errorf("stale cleanup: read %s failed: %w", name, err))
			continue
		}
		if !hasGeneratorMarker(data) {
			continue // hand-authored or fixture page; keep
		}
		if err := os.Remove(filepath.Join(storiesDir, name)); err != nil {
			errs = append(errs, fmt.Errorf("stale cleanup: remove %s failed: %w", name, err))
		}
	}
	return errors.Join(errs...)
}

// hasGeneratorMarker reports whether the YAML front-matter block of a Markdown
// document contains the generator marker line. Files without a front-matter
// block or without the marker are considered non-generated.
func hasGeneratorMarker(data []byte) bool {
	text := string(data)
	if !strings.HasPrefix(text, "---\n") {
		return false
	}
	rest := text[4:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return false
	}
	block := rest[:end]
	for _, line := range strings.Split(block, "\n") {
		if strings.TrimSpace(line) == generatorMarker {
			return true
		}
	}
	return false
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
