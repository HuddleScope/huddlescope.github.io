package domain

import "time"

// Story represents a single story in the system.
type Story struct {
	ID          string
	Title       string
	Source      string
	PublishedAt time.Time
	Summary     string
	FirstSeenAt time.Time
}
