package domain

import (
	"fmt"
	"strings"
	"time"
)

// Story represents a single story in the system.
type Story struct {
	ID          string
	Title       string
	Source      string
	PublishedAt time.Time
	Summary     string
	FirstSeenAt time.Time
}

// FeedEntry represents a raw item fetched from a feed source.
type FeedEntry struct {
	Title       string
	Source      string
	PublishedAt time.Time
	Summary     string
	URL         string
}

// Source represents a news source or feed.
type Source struct {
	ID          string
	Name        string
	URL         string
	LastFetched *time.Time
	ItemCount   int
}

// NormalizeTitle normalizes a title for stable ID generation.
func NormalizeTitle(title string) string {
	// Trim whitespace
	s := strings.TrimSpace(title)
	// Collapse multiple spaces
	s = strings.Join(strings.Fields(s), " ")
	// Convert to lowercase for comparison
	return strings.ToLower(s)
}

// ComputeStoryID computes a stable ID for a story based on normalized metadata.
func ComputeStoryID(title, source string, publishedAt time.Time) string {
	normalizedTitle := NormalizeTitle(title)
	sourceLower := strings.ToLower(source)
	publishedStr := publishedAt.Format(time.RFC3339)
	return fmt.Sprintf("%s|%s|%s", normalizedTitle, sourceLower, publishedStr)
}

// Canonicalize creates a canonical Story from a FeedEntry with first-seen timestamp.
func (s Story) Canonicalize(entry FeedEntry) Story {
	return Story{
		ID:          ComputeStoryID(entry.Title, entry.Source, entry.PublishedAt),
		Title:       entry.Title,
		Source:      entry.Source,
		PublishedAt: entry.PublishedAt,
		Summary:     entry.Summary,
		FirstSeenAt: time.Now().UTC(),
	}
}

// IsEqual checks if two stories are the same by ID.
func (s Story) IsEqual(other Story) bool {
	return s.ID == other.ID
}
