package domain

import (
	"encoding/json"
	"fmt"
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
	Source       string
	PublishedAt  time.Time
	Summary      string
	URL          string
	ArticleIDs   []string // IDs of articles in this story group (HS-010)
	Headline     string   // normalized headline for the story (HS-010)
	GUID         string
	CanonicalURL string
	DedupeKey    string
	Entities     []string // normalized team slugs, player names (HS-010)
	FirstSeenAt  time.Time
	UpdatedAt    time.Time
}

// MarshalJSON implements json.Marshaler for Story.
func (s Story) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID           string   `json:"id"`
		Title        string   `json:"title"`
		Source       string   `json:"source"`
		PublishedAt  string   `json:"publishedAt"`
		Summary      string   `json:"summary"`
		URL          string   `json:"url"`
		ArticleIDs   []string `json:"articleIDs"`
		Headline     string   `json:"headline"`
		GUID         string   `json:"guid"`
		CanonicalURL string   `json:"canonicalURL"`
		DedupeKey    string   `json:"dedupeKey"`
		Entities     []string `json:"entities"`
		FirstSeenAt  string   `json:"firstSeenAt"`
		UpdatedAt    string   `json:"updatedAt"`
	}{
		ID:           s.ID,
		Title:        s.Title,
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
	})
}

// UnmarshalJSON implements json.Unmarshaler for Story.
func (s *Story) UnmarshalJSON(data []byte) error {
	var tmp struct {
		ID           string   `json:"id"`
		Title        string   `json:"title"`
		Source       string   `json:"source"`
		PublishedAt  string   `json:"publishedAt"`
		Summary      string   `json:"summary"`
		URL          string   `json:"url"`
		ArticleIDs   []string `json:"articleIDs"`
		Headline     string   `json:"headline"`
		GUID         string   `json:"guid"`
		CanonicalURL string   `json:"canonicalURL"`
		DedupeKey    string   `json:"dedupeKey"`
		Entities     []string `json:"entities"`
		FirstSeenAt  string   `json:"firstSeenAt"`
		UpdatedAt    string   `json:"updatedAt"`
	}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	s.ID = tmp.ID
	s.Title = tmp.Title
	s.Source = tmp.Source
	t, err := time.Parse(time.RFC3339, tmp.PublishedAt)
	if err != nil {
		return err
	}
	s.PublishedAt = t
	s.Summary = tmp.Summary
	s.URL = tmp.URL
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
	return nil
}
