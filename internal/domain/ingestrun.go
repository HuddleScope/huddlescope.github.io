package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// IngestRunStatus is the status of a tracked ingest run. The string values
// are the persistence contract (ingest_runs.status) and match the legacy
// status vocabulary minus 'completed' (which maps to OK, plan §1.2).
type IngestRunStatus int

const (
	IngestRunRunning IngestRunStatus = iota
	IngestRunOK
	IngestRunPartial
	IngestRunFailed
)

// String returns the stable persistence/JSON string form.
func (s IngestRunStatus) String() string {
	switch s {
	case IngestRunRunning:
		return "running"
	case IngestRunOK:
		return "ok"
	case IngestRunPartial:
		return "partial"
	case IngestRunFailed:
		return "failed"
	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}

// ParseIngestRunStatus parses a persisted status string.
func ParseIngestRunStatus(s string) (IngestRunStatus, error) {
	switch s {
	case "running":
		return IngestRunRunning, nil
	case "ok":
		return IngestRunOK, nil
	case "partial":
		return IngestRunPartial, nil
	case "failed":
		return IngestRunFailed, nil
	default:
		return 0, fmt.Errorf("unknown ingest run status %q", s)
	}
}

// MarshalJSON implements json.Marshaler (string form).
func (s IngestRunStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// UnmarshalJSON implements json.Unmarshaler (string form).
func (s *IngestRunStatus) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	parsed, err := ParseIngestRunStatus(str)
	if err != nil {
		return err
	}
	*s = parsed
	return nil
}

// IsSuccess reports whether the run counts as a successful ingest for
// freshness purposes: ok and partial both produced a completed, stored
// result. Running (never finished) and failed never do.
func (s IngestRunStatus) IsSuccess() bool {
	return s == IngestRunOK || s == IngestRunPartial
}

// IngestRun is one run-level ingest record (ingest_runs row). It complements
// (not duplicates) the per-feed domain.FeedHealth / feeds.json data.
type IngestRun struct {
	RunID            string
	Origin           string // raspberry | vps | github-backup | legacy | local
	StartedAt        time.Time
	FinishedAt       *time.Time
	Status           IngestRunStatus
	SourcesChecked   int
	SourcesSucceeded int
	EntriesFetched   int
	StoriesCreated   int
	StoriesUpdated   int
	ErrorSummary     string // sanitized; never raw error text
}

// IsSuccess reports whether this run updates last_successful_ingest_at.
func (r IngestRun) IsSuccess() bool {
	return r.Status.IsSuccess()
}

// NewIngestRunFromRecord derives a persisted run from the pipeline's RunRecord.
// Status is RECOMPUTED from per-source outcomes (plan §1.4): RunRecord.Status
// is RunStatusPartial for every source failure and never demotes to Failed
// when all sources fail, which would wrongly count an all-feeds-failed round
// as a success. Derivation:
//   - all checked sources failed (or no sources checked with an error) -> failed
//   - >=1 succeeded and >=1 failed                                    -> partial
//   - none failed                                                     -> ok
//
// (a zero-new-entries run with no failures is still ok; AC-2)
func NewIngestRunFromRecord(rec RunRecord, origin string) IngestRun {
	succeeded := 0
	checked := 0
	var firstErr error
	for _, s := range rec.Sources {
		checked++
		if len(s.Errors) > 0 {
			if firstErr == nil {
				firstErr = s.Errors[0]
			}
		} else {
			succeeded++
		}
	}
	failed := checked - succeeded

	status := IngestRunOK
	if checked > 0 && failed == checked {
		status = IngestRunFailed
	} else if failed > 0 {
		status = IngestRunPartial
	}

	// A RunRecord-level error (e.g., publish failure) always means the run
	// did not complete successfully.
	if len(rec.Errors) > 0 {
		if firstErr == nil {
			firstErr = rec.Errors[0]
		}
		if status != IngestRunFailed {
			// If sources were all fine but the run still has errors the
			// completion was compromised: degrade, never claim full ok.
			if status == IngestRunOK {
				status = IngestRunFailed
			} else {
				status = IngestRunPartial
			}
		}
	}

	run := IngestRun{
		RunID:            string(rec.RunID),
		Origin:           origin,
		StartedAt:        rec.StartedAt.UTC(),
		Status:           status,
		SourcesChecked:   checked,
		SourcesSucceeded: succeeded,
		EntriesFetched:   rec.EntriesFetched,
		StoriesCreated:   rec.StoriesCreated,
		StoriesUpdated:   rec.StoriesUpdated,
	}
	if rec.EndedAt != nil {
		end := rec.EndedAt.UTC()
		run.FinishedAt = &end
	}
	if firstErr != nil {
		// Sanitize (token/password/DSN redaction, 500-char cap) and strip
		// newlines/control chars so the value is a single-line summary.
		run.ErrorSummary = sanitizeRunError(firstErr)
	}
	return run
}

// sanitizeRunError redacts secrets and returns a single-line summary.
// It uses the existing SanitizeFeedError (keyword-level redaction) and then
// applies the regex-based secret detectors so that secret *values* are
// masked too (the keyword replacer alone leaves the value intact).
func sanitizeRunError(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	s = redactSecretValues(s)
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r < 0x20 {
			return ' '
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		s = s[:497] + "..."
	}
	return s
}

// redactSecretValues masks the value of secret-looking assignments and
// bearer tokens using the same regexes that FeedHealth.Validate() uses for
// detection. This closes the gap where the keyword replacer redacts the
// keyword but leaves the secret value verbatim.
func redactSecretValues(s string) string {
	if s == "" {
		return s
	}
	s = secretAssignRe.ReplaceAllStringFunc(s, func(m string) string {
		if i := strings.IndexAny(m, ":="); i >= 0 {
			return m[:i+1] + "[REDACTED]"
		}
		return "[REDACTED]"
	})
	s = secretConnRe.ReplaceAllStringFunc(s, func(m string) string {
		if i := strings.Index(m, "://"); i >= 0 {
			return m[:i] + "://[REDACTED]"
		}
		return m
	})
	s = bearerRe.ReplaceAllString(s, "Bearer [REDACTED]")
	return s
}

// ShouldRunDecision is the outcome of a freshness check (plan §2/AC-5).
type ShouldRunDecision struct {
	ShouldRun bool
	// Reason is one of "fresh", "stale", "never_succeeded" (stable contract).
	Reason string
	// LastSuccessfulAt is nil when no successful run exists yet.
	LastSuccessfulAt *time.Time
	// Age is now - LastSuccessfulAt clamped to >= 0 (clock-skew safe, AC-1).
	Age time.Duration
}

// Reason constants (stable JSON contract values).
const (
	ReasonFresh          = "fresh"
	ReasonStale          = "stale"
	ReasonNeverSucceeded = "never_succeeded"
)

// DecideShouldRun decides whether a backup ingest should run, purely from the
// DB-sourced last success (never from local files/clock-relative heuristics
// beyond the provided now). Age is clamped to >= 0 so a host whose clock is
// skewed ahead of the DB never reports negative age (AC-1).
//
// Semantics:
//   - lastSuccess nil                      -> should_run=true, "never_succeeded"
//   - age > staleAfter                     -> should_run=true, "stale"
//   - age <= staleAfter (incl. exactly)    -> should_run=false, "fresh"
func DecideShouldRun(lastSuccess *IngestRun, now time.Time, staleAfter time.Duration) ShouldRunDecision {
	if lastSuccess == nil || lastSuccess.FinishedAt == nil {
		return ShouldRunDecision{ShouldRun: true, Reason: ReasonNeverSucceeded}
	}
	age := now.Sub(lastSuccess.FinishedAt.UTC())
	if age < 0 {
		age = 0
	}
	fin := lastSuccess.FinishedAt.UTC()
	decision := ShouldRunDecision{
		LastSuccessfulAt: &fin,
		Age:              age,
	}
	if age > staleAfter {
		decision.ShouldRun = true
		decision.Reason = ReasonStale
	} else {
		decision.ShouldRun = false
		decision.Reason = ReasonFresh
	}
	return decision
}

// IngestRunRepository is the persistence contract for run-level ingest
// history. Implementations: sqlite (driver-agnostic over *sql.DB), turso
// (lazy libsql wrapper), memory (per-host, fail-closed for freshness CLI).
type IngestRunRepository interface {
	// Create inserts a run row with status forced to running (the run just
	// started). Idempotent per run_id (INSERT OR REPLACE semantics).
	Create(ctx context.Context, run IngestRun) error
	// Finish updates an existing run row by run_id: status, finished_at,
	// counters, and (sanitized) error summary.
	Finish(ctx context.Context, run IngestRun) error
	// LastSuccess returns the most recent successful run (status ok/partial
	// with finished_at), ordered by finished_at. ok=false when no such run.
	LastSuccess(ctx context.Context) (*IngestRun, bool, error)
	// LastAttempt returns the most recent run of any status, ordered by
	// COALESCE(finished_at, started_at). ok=false when no runs at all.
	LastAttempt(ctx context.Context) (*IngestRun, bool, error)
	Close() error
}
