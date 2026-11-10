// Package domain contains core domain models and interfaces.
package domain

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// IngestionResult represents the result of an ingestion run.
type IngestionResult struct {
	EntriesFetched int
	StoriesCreated int
	StoriesUpdated int
	StoriesSkipped int
	Errors         []error
}

// ExportResult represents the result of an export run.
type ExportResult struct {
	StoriesExported int
	Errors          []error
}

// Repository interface for story storage.
type Repository interface {
	Upsert(story Story) error
	List() ([]Story, error)
	Count() (int, error)
}

// FeedFetcher interface for fetching entries from feeds.
type FeedFetcher interface {
	Fetch(sourceID string) ([]FeedEntry, error)
}

// FeedEntry represents a raw item fetched from a feed source.
type FeedEntry struct {
	Title       string
	Source      string
	PublishedAt time.Time
	Summary     string
	URL         string
	GUID        string
}

// Source represents a news source or feed.
type Source struct {
	ID          string
	Name        string
	URL         string
	LastFetched *time.Time
	ItemCount   int
	// Additional registry fields (HS-006)
	Publication   string
	Priority      int
	Enabled       bool
	Language      string
	Timeout       time.Duration
	License       string
	AcceptableUse AcceptableUse
}

// AcceptableUse captures publisher acceptable-use posture.
type AcceptableUse int

const (
	AcceptanceUnknown AcceptableUse = iota
	AcceptanceAllowed
	AcceptanceRestricted
)

func (au AcceptableUse) String() string {
	switch au {
	case AcceptanceAllowed:
		return "allowed"
	case AcceptanceRestricted:
		return "restricted"
	default:
		return "unknown"
	}
}

// FeedSource represents a configurable RSS/Atom feed source with full metadata.
type FeedSource struct {
	ID            string
	URL           string
	Publication   string
	Priority      int
	Enabled       bool
	Language      string
	Timeout       time.Duration
	License       string
	AcceptableUse AcceptableUse
}

// Validate validates a FeedSource.
func (fs *FeedSource) Validate() error {
	if fs.ID == "" {
		return fmt.Errorf("feed source ID must not be empty")
	}
	if fs.URL == "" {
		return fmt.Errorf("feed source %s URL must not be empty", fs.ID)
	}
	// URL scheme validation
	if !strings.HasPrefix(strings.ToLower(fs.URL), "http://") &&
		!strings.HasPrefix(strings.ToLower(fs.URL), "https://") {
		return fmt.Errorf("feed source %s URL must use http or https", fs.ID)
	}
	// Timeout validation (non-negative)
	if fs.Timeout < 0 {
		return fmt.Errorf("feed source %s timeout must be >= 0", fs.ID)
	}
	// Priority validation
	if fs.Priority < 0 {
		return fmt.Errorf("feed source %s priority must be >= 0", fs.ID)
	}
	return nil
}

// FeedRegistry manages a collection of feed sources with runtime status tracking.
type FeedRegistry struct {
	mu      sync.RWMutex
	sources []FeedSource
	byID    map[string]*FeedSource
	status  map[string]*PollStatus
}

// PollStatus tracks runtime polling status per feed source.
type PollStatus struct {
	LastAttemptAt *time.Time
	LastError     error
	LastOK        bool
}

// NewFeedRegistry creates a registry from a slice of FeedSource.
func NewFeedRegistry(sources ...FeedSource) (*FeedRegistry, error) {
	registry := &FeedRegistry{
		sources: make([]FeedSource, 0, len(sources)),
		byID:    make(map[string]*FeedSource),
		status:  make(map[string]*PollStatus),
	}

	seenIDs := make(map[string]bool)
	for i, s := range sources {
		if err := s.Validate(); err != nil {
			return nil, fmt.Errorf("source %d (%s) invalid: %w", i+1, s.ID, err)
		}
		if seenIDs[s.ID] {
			return nil, fmt.Errorf("duplicate source ID %q", s.ID)
		}
		seenIDs[s.ID] = true

		fs := s // copy to avoid aliasing
		registry.sources = append(registry.sources, fs)
		registry.byID[s.ID] = &registry.sources[len(registry.sources)-1]
		registry.status[s.ID] = &PollStatus{}
	}

	return registry, nil
}

// All returns all sources (copy), sorted by priority (ascending), then by ID.
func (r *FeedRegistry) All() []FeedSource {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]FeedSource, len(r.sources))
	copy(result, r.sources)

	// Sort by priority (ascending), then by ID for deterministic order
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i].Priority > result[j].Priority ||
				(result[i].Priority == result[j].Priority && result[i].ID > result[j].ID) {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result
}

// Enabled returns only enabled sources (copy), sorted by priority then ID.
func (r *FeedRegistry) Enabled() []FeedSource {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var enabled []FeedSource
	for _, s := range r.sources {
		if s.Enabled {
			enabled = append(enabled, s)
		}
	}

	// Sort by priority (ascending), then by ID
	for i := 0; i < len(enabled)-1; i++ {
		for j := i + 1; j < len(enabled); j++ {
			if enabled[i].Priority > enabled[j].Priority ||
				(enabled[i].Priority == enabled[j].Priority && enabled[i].ID > enabled[j].ID) {
				enabled[i], enabled[j] = enabled[j], enabled[i]
			}
		}
	}

	return enabled
}

// Get returns a source by ID, and a boolean indicating if found.
func (r *FeedRegistry) Get(id string) (*FeedSource, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	src, ok := r.byID[id]
	return src, ok
}

// RecordStatus records the result of a poll attempt.
func (r *FeedRegistry) RecordStatus(id string, ok bool, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	if ps, exists := r.status[id]; exists {
		ps.LastAttemptAt = &now
		ps.LastError = err
		ps.LastOK = ok
	}
}

// Status returns the current poll status for a source.
func (r *FeedRegistry) Status(id string) PollStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if ps, exists := r.status[id]; exists {
		return *ps
	}
	return PollStatus{}
}

// Validate validates all sources in the registry.
func (r *FeedRegistry) Validate() error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var err error
	for i, s := range r.sources {
		if e := s.Validate(); e != nil {
			err = fmt.Errorf("%w; source %d (%s) invalid: %v", err, i+1, s.ID, e)
		}
	}
	return err
}

// Count returns the number of sources in the registry.
func (r *FeedRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.sources)
}

// CountEnabled returns the number of enabled sources.
func (r *FeedRegistry) CountEnabled() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, s := range r.sources {
		if s.Enabled {
			count++
		}
	}
	return count
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
		Title:       NormalizeTitle(entry.Title),
		Source:      NormalizeSourceID(entry.Source),
		PublishedAt: NormalizeTimestamp(entry.PublishedAt),
		Summary:     SanitizeHTML(entry.Summary, 1024),
		URL:         CanonicalURLEntry(entry.URL),
		FirstSeenAt: time.Now().UTC(),
	}
}

// IsEqual checks if two stories are the same by ID.
func (s Story) IsEqual(other Story) bool {
	return s.ID == other.ID
}
