// Package domain contains core domain models and interfaces.
package domain

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"
)

// RunStatus represents the outcome of a run (ingestion, ranking, export).
type RunStatus int

const (
	RunStatusOK      RunStatus = iota
	RunStatusPartial           // Some sources failed but run completed and homepage committed
	RunStatusFailed            // Critical failure: run did not complete or homepage not committed
)

func (s RunStatus) String() string {
	switch s {
	case RunStatusOK:
		return "ok"
	case RunStatusPartial:
		return "partial"
	case RunStatusFailed:
		return "failed"
	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}

// IsCritical returns true if the run should exit with a non-zero exit code.
func (s RunStatus) IsCritical() bool {
	return s == RunStatusFailed
}

// ExitCode returns the process exit code.
func (s RunStatus) ExitCode() int {
	if s.IsCritical() {
		return 1
	}
	return 0
}

// RunID is a unique identifier for a run.
type RunID string

// NewRunID generates a unique run ID.
func NewRunID() RunID {
	b := make([]byte, 16)
	_, _ = rand.Read(b) //nolint:errcheck // best effort
	return RunID(base64.URLEncoding.EncodeToString(b))
}

// String implements fmt.Stringer.
func (r RunID) String() string {
	return string(r)
}

// SourceRunStatus captures the outcome of fetching a single source.
type SourceRunStatus struct {
	SourceID       string
	EntriesFetched int
	StoriesCreated int
	StoriesUpdated int
	StoriesSkipped int
	Errors         []error
	FetchedAt      *time.Time
}

// AddError appends an error to the source status.
func (s *SourceRunStatus) AddError(err error) {
	if err == nil {
		return
	}
	s.Errors = append(s.Errors, err)
}

// HasErrors returns true if any errors were recorded.
func (s *SourceRunStatus) HasErrors() bool {
	return len(s.Errors) > 0
}

// RunRecord captures the outcome of a complete run.
type RunRecord struct {
	RunID           RunID
	StartedAt       time.Time
	EndedAt         *time.Time
	IngestedAt      *time.Time // Ingestion phase completion timestamp
	LastPublishedAt *time.Time // Most recent article publishedAt
	Status          RunStatus
	Sources         []SourceRunStatus
	EntriesFetched  int
	StoriesCreated  int
	StoriesUpdated  int
	StoriesSkipped  int
	GroupsCreated   int
	ArticlesGrouped int
	ArticlesSolo    int
	Homepage        *Homepage
	Committed       bool // Homepage was successfully committed
	Errors          []error
}

// AddSourceStatus appends a source status to the run record.
func (r *RunRecord) AddSourceStatus(status SourceRunStatus) {
	r.Sources = append(r.Sources, status)
	r.EntriesFetched += status.EntriesFetched
	r.StoriesCreated += status.StoriesCreated
	r.StoriesUpdated += status.StoriesUpdated
	r.StoriesSkipped += status.StoriesSkipped
}

// AddError appends an error to the run record and updates status.
func (r *RunRecord) AddError(err error) {
	if err == nil {
		return
	}
	r.Errors = append(r.Errors, err)
	// Mark as Failed only if no failure already set
	if r.Status != RunStatusFailed {
		r.Status = RunStatusFailed
	}
}

// MarkPartial marks the run as partial (if not already Failed).
func (r *RunRecord) MarkPartial() {
	if r.Status != RunStatusFailed {
		r.Status = RunStatusPartial
	}
}

// MarkOK marks the run as OK (if not already Failed/Partial).
func (r *RunRecord) MarkOK() {
	if r.Status == RunStatusOK || r.Status == RunStatusFailed {
		return
	}
	r.Status = RunStatusOK
}

// Homepage represents the curated list of stories for the homepage.
type Homepage struct {
	RankedStories []Story
	Notes         []string
}

// EntryWithinLookback returns true if the entry's publishedAt is within the lookback window.
// - window <= 0: all entries pass (no lookback)
// - publishedAt.IsZero(): entry passes (undated entries are kept for idempotency)
// - otherwise: publishedAt must be after now-window
func EntryWithinLookback(publishedAt, now time.Time, window time.Duration) bool {
	if window <= 0 {
		return true
	}
	if publishedAt.IsZero() {
		return true
	}
	return publishedAt.After(now.Add(-window))
}

// AggregateRunRecords merges multiple run records (e.g., across multiple phases).
func AggregateRunRecords(recs ...RunRecord) RunRecord {
	if len(recs) == 0 {
		return RunRecord{}
	}
	// Start with first record as base
	result := recs[0]
	for _, r := range recs[1:] {
		if result.Status != RunStatusFailed && r.Status == RunStatusFailed {
			result.Status = RunStatusFailed
		} else if result.Status == RunStatusOK && r.Status == RunStatusPartial {
			result.Status = RunStatusPartial
		}
		result.Errors = append(result.Errors, r.Errors...)
		result.StoriesCreated += r.StoriesCreated
		result.StoriesUpdated += r.StoriesUpdated
		result.StoriesSkipped += r.StoriesSkipped
		result.GroupsCreated += r.GroupsCreated
		result.ArticlesGrouped += r.ArticlesGrouped
		result.ArticlesSolo += r.ArticlesSolo
	}
	return result
}
