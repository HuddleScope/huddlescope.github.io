package domain

import (
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
