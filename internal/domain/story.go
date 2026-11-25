package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// UpsertStatus represents the result of an upsert operation.
type UpsertStatus int

const (
	UpsertCreated UpsertStatus = iota
	UpsertUpdated
	UpsertUnchanged
)

func (s UpsertStatus) String() string {
	switch s {
	case UpsertCreated:
		return "created"
	case UpsertUpdated:
		return "updated"
	case UpsertUnchanged:
		return "unchanged"
	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}

// Story represents a single story in the system.
type Story struct {
	ID           string
	Title        string
	DisplayTitle string // original feed casing for display; empty falls back to Title (#201)
	Source       string
	PublishedAt  time.Time
	Summary      string
	URL          string
	ArticleData  []Article // full article data for articles in this story (HS-064)
	ArticleIDs   []string  // IDs of articles in this story group (HS-010)
	Headline     string    // normalized headline for the story (HS-010)
	GUID         string
	CanonicalURL string
	DedupeKey    string
	Entities     []string // normalized team slugs, player names (HS-010)
	FirstSeenAt  time.Time
	UpdatedAt    time.Time
	// JoinReasons records per-article join decisions (AC-3a). Persisted as
	// "joinReasons" in the story JSON; old payloads without the key unmarshal
	// to an empty slice (backward compatible).
	JoinReasons []JoinDecision
}

// Display returns the display headline for the story: the original feed
// casing (DisplayTitle) when set, otherwise the normalized Title (#201). This
// is the single fallback point every display surface uses.
func (s Story) Display() string {
	if d := strings.TrimSpace(s.DisplayTitle); d != "" {
		return s.DisplayTitle
	}
	return s.Title
}

// MarshalJSON implements json.Marshaler for Story.
// Note: articleData is deprecated for persistence; populated by storage layer
func (s Story) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID           string         `json:"id"`
		Title        string         `json:"title"`
		DisplayTitle string         `json:"displayTitle"`
		Source       string         `json:"source"`
		PublishedAt  string         `json:"publishedAt"`
		Summary      string         `json:"summary"`
		URL          string         `json:"url"`
		ArticleIDs   []string       `json:"articleIDs"`
		Headline     string         `json:"headline"`
		GUID         string         `json:"guid"`
		CanonicalURL string         `json:"canonicalURL"`
		DedupeKey    string         `json:"dedupeKey"`
		Entities     []string       `json:"entities"`
		FirstSeenAt  string         `json:"firstSeenAt"`
		UpdatedAt    string         `json:"updatedAt"`
		JoinReasons  []JoinDecision `json:"joinReasons"`
	}{
		ID:           s.ID,
		Title:        s.Title,
		DisplayTitle: s.DisplayTitle,
		Source:       s.Source,
		PublishedAt:  s.PublishedAt.Format(time.RFC3339),
		Summary:      s.Summary,
		URL:          s.URL,
		ArticleIDs:   s.ArticleIDs,
		Headline:     s.Headline,
		GUID:         s.GUID,
		CanonicalURL: s.CanonicalURL,
		DedupeKey:    s.DedupeKey,
		Entities:     s.Entities,
		FirstSeenAt:  s.FirstSeenAt.Format(time.RFC3339),
		UpdatedAt:    s.UpdatedAt.Format(time.RFC3339),
		JoinReasons:  s.JoinReasons,
	})
}

// UnmarshalJSON implements json.Unmarshaler for Story.
// Supports both old format (articleIDs) and new format (articleData) for backward compatibility.
func (s *Story) UnmarshalJSON(data []byte) error {
	var tmp struct {
		ID           string         `json:"id"`
		Title        string         `json:"title"`
		DisplayTitle string         `json:"displayTitle"`
		Source       string         `json:"source"`
		PublishedAt  string         `json:"publishedAt"`
		Summary      string         `json:"summary"`
		URL          string         `json:"url"`
		ArticleData  []Article      `json:"articleData"`
		ArticleIDs   []string       `json:"articleIDs"`
		Headline     string         `json:"headline"`
		GUID         string         `json:"guid"`
		CanonicalURL string         `json:"canonicalURL"`
		DedupeKey    string         `json:"dedupeKey"`
		Entities     []string       `json:"entities"`
		FirstSeenAt  string         `json:"firstSeenAt"`
		UpdatedAt    string         `json:"updatedAt"`
		JoinReasons  []JoinDecision `json:"joinReasons"`
	}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	s.ID = tmp.ID
	s.Title = tmp.Title
	s.DisplayTitle = tmp.DisplayTitle
	s.Source = tmp.Source
	t, err := time.Parse(time.RFC3339, tmp.PublishedAt)
	if err != nil {
		return err
	}
	s.PublishedAt = t
	s.Summary = tmp.Summary
	s.URL = tmp.URL
	s.ArticleData = tmp.ArticleData
	s.ArticleIDs = tmp.ArticleIDs
	s.Headline = tmp.Headline
	s.GUID = tmp.GUID
	s.CanonicalURL = tmp.CanonicalURL
	s.DedupeKey = tmp.DedupeKey
	s.Entities = tmp.Entities
	t2, err := time.Parse(time.RFC3339, tmp.FirstSeenAt)
	if err != nil {
		return err
	}
	s.FirstSeenAt = t2
	t3, err := time.Parse(time.RFC3339, tmp.UpdatedAt)
	if err != nil {
		return err
	}
	s.UpdatedAt = t3
	// JoinReasons is optional; old payloads omit the key and unmarshal to nil.
	s.JoinReasons = tmp.JoinReasons
	return nil
}
