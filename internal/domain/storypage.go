// Package domain defines the view model and rendering port for static story detail pages.
package domain

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// StorySlug is a URL-safe slug derived from the headline.
// It uses the representative (earliest) article's normalized headline.
// The slug remains stable if later sources are added to the same story group.
type StorySlug string

// StoryPath is the full URL path for a story detail page.
type StoryPath string

// StoryPageKey is a stable identifier for a story page derived from the representative article.
type StoryPageKey string

// StoryDetailPage represents a single, canonical story page with metadata from all sources.
type StoryDetailPage struct {
	Slug    StorySlug
	Path    StoryPath
	StoryID string
	// PageKey is the stable, representative-derived public identity of the
	// story page (issue #26, D1). Unlike StoryID (an internal group hash that
	// changes when members change), PageKey is invariant while later articles
	// arrive and changes only when the representative changes (merge or
	// backfill), which aliasing absorbs.
	PageKey StoryPageKey
	// Status is the story lifecycle state (issue #26, AC-2). Zero value = active.
	Status StoryStatus
	// Aliases are identities of stories merged into this one (issue #26, AC-2).
	Aliases          []StoryAlias
	Headline         string
	FirstPublishedAt time.Time
	LastPublishedAt  time.Time
	FirstSeenAt      time.Time
	Articles         []PublisherArticle
}

// PublisherArticle is a metadata-only representation of a source article.
// It intentionally omits full article body/content to avoid publishing copyrighted material.
type PublisherArticle struct {
	SourceID    string
	SourceName  string // resolved via SourceNameResolver
	Title       string
	URL         string
	PublishedAt time.Time
}

// SourceNameResolver resolves a source ID to its display name.
type SourceNameResolver interface {
	Resolve(sourceID string) string
}

// StaticSourceResolver is a simple in-memory resolver.
type StaticSourceResolver struct {
	mapping map[string]string
}

// Resolve returns the display name for a source ID.
func (r *StaticSourceResolver) Resolve(sourceID string) string {
	if r.mapping == nil {
		return sourceID
	}
	if name, ok := r.mapping[sourceID]; ok {
		return name
	}
	return sourceID
}

// NewStaticSourceResolver creates a resolver from a map.
func NewStaticSourceResolver(mapping map[string]string) *StaticSourceResolver {
	return &StaticSourceResolver{mapping: mapping}
}

// AddMapping adds or updates a source name mapping.
func (r *StaticSourceResolver) AddMapping(sourceID, name string) {
	if r.mapping == nil {
		r.mapping = make(map[string]string)
	}
	r.mapping[sourceID] = name
}

// BuildStoryDetailPage creates a story detail page from a story group and resolver.
// It picks the representative article (earliest PublishedAt, ties on smallest canonical URL).
// Articles without a URL are omitted from the listing (AC4: graceful handling).
func BuildStoryDetailPage(group StoryGroup, resolver SourceNameResolver, now time.Time) (StoryDetailPage, error) {
	if len(group.Articles) == 0 {
		return StoryDetailPage{}, fmt.Errorf("cannot build page: empty story group")
	}

	// Pick representative: earliest PublishedAt, ties on smallest canonical URL
	rep := pickRepresentative(group.Articles)

	// Display headline (original casing where available, normalized fallback)
	// (#201). The slug/page key below remain computed from the normalized
	// representative title so identity stays stable (AC-2).
	displayHeadline := group.DisplayHeadline
	if strings.TrimSpace(displayHeadline) == "" {
		displayHeadline = group.Headline
	}

	// Build article list, skipping those without URL
	var articles []PublisherArticle
	for _, art := range group.Articles {
		if strings.TrimSpace(art.URL) == "" {
			continue // AC4: skip gracefully
		}
		articles = append(articles, PublisherArticle{
			SourceID:    art.SourceID,
			SourceName:  resolver.Resolve(art.SourceID),
			Title:       art.Display(),
			URL:         art.URL,
			PublishedAt: art.ParsePublishedAtOrZero(),
		})
	}

	if len(articles) == 0 {
		return StoryDetailPage{}, fmt.Errorf("cannot build page: no articles have a valid URL")
	}

	// Chronological timeline (issue #26, D6): sort by PublishedAt, ties on URL,
	// so the rendered listing is chronological across multi-day updates.
	sort.Slice(articles, func(i, j int) bool {
		if !articles[i].PublishedAt.Equal(articles[j].PublishedAt) {
			return articles[i].PublishedAt.Before(articles[j].PublishedAt)
		}
		return articles[i].URL < articles[j].URL
	})

	// Compute stable slug from the representative's normalized headline
	slug := StorySlug(ComputeStorySlugFromHeadline(rep.Title))

	page := StoryDetailPage{
		Slug:             slug,
		Path:             StoryPath("/stories/" + string(slug) + "/"),
		StoryID:          group.ID,
		PageKey:          ComputeStoryPageKey(rep),
		Status:           group.Status,
		Aliases:          group.Aliases,
		Headline:         displayHeadline,
		FirstPublishedAt: group.FirstPublishedAt,
		LastPublishedAt:  group.LastPublishedAt,
		FirstSeenAt:      now.UTC(),
		Articles:         articles,
	}

	return page, nil
}

// pickRepresentative selects the article with the earliest PublishedAt.
// Ties break on smallest canonical URL for determinism.
func pickRepresentative(articles []Article) Article {
	if len(articles) == 0 {
		return Article{}
	}

	// Already sorted by time from StoryGroup, but ensure deterministic sort
	sorted := make([]Article, len(articles))
	copy(sorted, articles)

	sort.Slice(sorted, func(i, j int) bool {
		ti := sorted[i].ParsePublishedAtOrZero()
		tj := sorted[j].ParsePublishedAtOrZero()

		if ti.Equal(tj) {
			// Tie-break on canonical URL
			return strings.Compare(sorted[i].URL, sorted[j].URL) < 0
		}

		return ti.Before(tj)
	})

	return sorted[0]
}

// ComputeStorySlugFromHeadline creates a URL-safe slug from a headline.
// It normalizes title, strips punctuation, collapses whitespace, lowercases.
func ComputeStorySlugFromHeadline(headline string) string {
	if headline == "" {
		return "untitled"
	}

	// Normalize title first
	normalized := NormalizeTitle(headline)

	// Replace spaces with hyphens
	slug := strings.ReplaceAll(normalized, " ", "-")

	// Remove non-alphanumeric characters except hyphens
	var result []rune
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result = append(result, r)
		}
	}

	// Remove consecutive hyphens
	slug = strings.Join(strings.FieldsFunc(string(result), func(r rune) bool {
		return r == '-'
	}), "-")

	// Trim trailing hyphens
	slug = strings.TrimSuffix(slug, "-")

	if slug == "" {
		return "untitled"
	}

	return slug
}

// ComputeStoryPageKey computes a stable identifier for a story page.
// Based on ComputeStoryID for consistency with HS-010.
func ComputeStoryPageKey(rep Article) StoryPageKey {
	return StoryPageKey(ComputeStoryID(rep.Title, rep.SourceID, rep.ParsePublishedAtOrZero()))
}

// StoryPagePublisher defines the interface for rendering and committing story pages.
type StoryPagePublisher interface {
	Publish(ctx context.Context, page StoryDetailPage) error
}

// StoryPageResult captures the result of a page build operation.
type StoryPageResult struct {
	Page         StoryDetailPage
	BuildTime    time.Time
	ArticleCount int
}
