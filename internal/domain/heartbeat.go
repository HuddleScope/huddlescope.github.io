// Package domain contains domain models and interfaces for heartbeat and observability.
package domain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// HeartbeatStatus represents the outcome of a refresh run.
type HeartbeatStatus int

const (
	HeartbeatStatusOK      HeartbeatStatus = iota // Refresh completed successfully
	HeartbeatStatusPartial                        // Ingestion or export succeeded with warnings
	HeartbeatStatusFailed                         // Critical failure: run did not complete
)

func (s HeartbeatStatus) String() string {
	switch s {
	case HeartbeatStatusOK:
		return "ok"
	case HeartbeatStatusPartial:
		return "partial"
	case HeartbeatStatusFailed:
		return "failed"
	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}

// Heartbeat represents the state and freshness of a refresh run.
type Heartbeat struct {
	Version        string          `json:"version"`
	Status         HeartbeatStatus `json:"status"`
	RunID          string          `json:"run_id"`
	AttemptedAt    string          `json:"attempted_at"`
	IngestedAt     *string         `json:"ingested_at,omitempty"`
	PublishedAt    *string         `json:"published_at,omitempty"`
	SourcesChecked int             `json:"sources_checked"`
	SourcesFailed  int             `json:"sources_failed"`
	Errors         []string        `json:"errors,omitempty"`
}

// NewHeartbeat creates a new heartbeat with the given run ID.
func NewHeartbeat(runID string) *Heartbeat {
	return &Heartbeat{
		Version:        HeartbeatVersion,
		RunID:          runID,
		Status:         HeartbeatStatusOK,
		SourcesChecked: 0,
		SourcesFailed:  0,
	}
}

// HeartbeatVersion is the current version of the heartbeat format.
const HeartbeatVersion = "1"

// Validate checks the heartbeat for required fields and valid values.
func (h *Heartbeat) Validate() error {
	if h.Version == "" {
		return errors.New("version required")
	}
	if h.Version != HeartbeatVersion {
		return fmt.Errorf("unsupported version %q; expected %q", h.Version, HeartbeatVersion)
	}
	if h.RunID == "" {
		return errors.New("run_id required")
	}
	if h.AttemptedAt == "" {
		return errors.New("attempted_at required")
	}
	// Validate attempted_at is parseable RFC3339
	if _, err := time.Parse(time.RFC3339, h.AttemptedAt); err != nil {
		return fmt.Errorf("invalid attempted_at format: %w", err)
	}
	if h.IngestedAt != nil {
		if _, err := time.Parse(time.RFC3339, *h.IngestedAt); err != nil {
			return fmt.Errorf("invalid ingested_at format: %w", err)
		}
	}
	if h.PublishedAt != nil {
		if _, err := time.Parse(time.RFC3339, *h.PublishedAt); err != nil {
			return fmt.Errorf("invalid published_at format: %w", err)
		}
	}
	return nil
}

// IsStale returns true if the heartbeat is stale (no published_at or age > threshold).
func (h *Heartbeat) IsStale(now time.Time, threshold time.Duration) bool {
	if h.PublishedAt == nil {
		return true
	}
	published, err := time.Parse(time.RFC3339, *h.PublishedAt)
	if err != nil {
		return true
	}
	return now.Sub(published) > threshold
}

// FromRunRecord builds a heartbeat from a run record.
// Error strings are capped at 200 chars and limited to 5 per source and 20 total.
// IngestedAt is set to EndedAt when available (end of ingest phase).
// Top-level record.Errors are appended to per-source errors.
func (h *Heartbeat) FromRunRecord(record RunRecord, now time.Time) {
	h.AttemptedAt = now.UTC().Format(time.RFC3339)
	h.Status = heartbeatStatusFromRunStatus(record.Status)
	if record.EndedAt != nil {
		t := record.EndedAt.UTC().Format(time.RFC3339)
		h.IngestedAt = &t
	}

	// Sources checked/failed from per-source status
	h.SourcesChecked = len(record.Sources)
	for _, src := range record.Sources {
		if src.HasErrors() {
			h.SourcesFailed++
		}
		// Collect up to 5 errors per source, capped to 200 chars each
		sourceErrors := make([]string, 0, len(src.Errors))
		for _, err := range src.Errors {
			sourceErrors = append(sourceErrors, limitString(err.Error(), 200))
			if len(sourceErrors) >= 5 {
				break
			}
		}
		if len(sourceErrors) > 0 {
			h.Errors = append(h.Errors, fmt.Sprintf("[source:%s] %s", src.SourceID, strings.Join(sourceErrors, "; ")))
		}
	}

	// Append top-level run errors (pipeline-level failures)
	for _, err := range record.Errors {
		h.Errors = append(h.Errors, limitString(err.Error(), 200))
	}

	// Limit total error count to 20
	if len(h.Errors) > 20 {
		h.Errors = h.Errors[:20]
	}
}

// MergePublication merges publish outcome into the heartbeat.
// If committed, sets published_at. If failed, keeps published_at (last known good).
func (h *Heartbeat) MergePublication(result PublicationResult, now time.Time) {
	h.AttemptedAt = now.UTC().Format(time.RFC3339)
	if result.Committed {
		if h.PublishedAt == nil {
			h.PublishedAt = &result.PublishedAt
		} else {
			// Update to latest if new is newer
			newTime, err := time.Parse(time.RFC3339, result.PublishedAt)
			if err == nil {
				oldTime, _ := time.Parse(time.RFC3339, *h.PublishedAt)
				if newTime.After(oldTime) {
					h.PublishedAt = &result.PublishedAt
				}
			}
		}
		// Update status if success
		if h.Status != HeartbeatStatusFailed {
			h.Status = HeartbeatStatusOK
		}
	} else if result.Errors != nil && len(result.Errors) > 0 {
		// Failed publish: keep published_at (don't update), set status to failed
		h.Status = HeartbeatStatusFailed
		// Add publish error(s)
		pubErrors := make([]string, 0, len(result.Errors))
		for _, err := range result.Errors {
			pubErrors = append(pubErrors, limitString(err.Error(), 200))
			if len(pubErrors) >= 5 {
				break
			}
		}
		h.Errors = append(h.Errors, fmt.Sprintf("[publish] %s", strings.Join(pubErrors, "; ")))
		// Limit total error count
		if len(h.Errors) > 20 {
			h.Errors = h.Errors[:20]
		}
	}
}

// limitString truncates a string to maxLen and adds "..." if truncated.
func limitString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	trunc := maxLen - 3
	if trunc < 0 {
		trunc = 0
	}
	return s[:trunc] + "..."
}

// DefaultHeartbeatPublisher is a no-op publisher for offline/dev.
type DefaultHeartbeatPublisher struct{}

func (p *DefaultHeartbeatPublisher) Publish(ctx context.Context, hb *Heartbeat) error {
	// No-op: in production, this would write to file or other storage
	return nil
}

// HeartbeatPublisher is the interface for publishing heartbeat files.
type HeartbeatPublisher interface {
	Publish(ctx context.Context, hb *Heartbeat) error
}

// HeartbeatStatusFromRunStatus converts RunStatus to HeartbeatStatus.
func heartbeatStatusFromRunStatus(s RunStatus) HeartbeatStatus {
	switch s {
	case RunStatusOK:
		return HeartbeatStatusOK
	case RunStatusPartial:
		return HeartbeatStatusPartial
	case RunStatusFailed:
		return HeartbeatStatusFailed
	default:
		return HeartbeatStatusFailed
	}
}
