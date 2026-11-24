package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestIngestRunStatus_String(t *testing.T) {
	cases := []struct {
		s    IngestRunStatus
		want string
	}{
		{IngestRunRunning, "running"},
		{IngestRunOK, "ok"},
		{IngestRunPartial, "partial"},
		{IngestRunFailed, "failed"},
		{IngestRunSkippedLocked, "skipped_locked"},
		{IngestRunSkippedFresh, "skipped_fresh"},
		{IngestRunStatus(99), "unknown(99)"},
	}
	for _, c := range cases {
		if got := c.s.String(); got != c.want {
			t.Errorf("String() = %q, want %q", got, c.want)
		}
	}
}

func TestParseIngestRunStatus(t *testing.T) {
	cases := []struct {
		in   string
		want IngestRunStatus
	}{
		{"running", IngestRunRunning},
		{"ok", IngestRunOK},
		{"partial", IngestRunPartial},
		{"failed", IngestRunFailed},
		{"skipped_locked", IngestRunSkippedLocked},
		{"skipped_fresh", IngestRunSkippedFresh},
	}
	for _, c := range cases {
		if s, err := ParseIngestRunStatus(c.in); err != nil || s != c.want {
			t.Errorf("ParseIngestRunStatus(%q) = %v, %v", c.in, s, err)
		}
	}
	if _, err := ParseIngestRunStatus("bogus"); err == nil {
		t.Error("expected error for unknown status")
	}
}

// TestIngestRunSkippedStatusesNotSuccess (issue #160): the two skipped_*
// statuses are non-success by construction, so they are invisible to
// LastSuccess / DecideShouldRun and never update last_successful_ingest_at.
func TestIngestRunSkippedStatusesNotSuccess(t *testing.T) {
	if IngestRunSkippedLocked.IsSuccess() {
		t.Error("skipped_locked must not be a success")
	}
	if IngestRunSkippedFresh.IsSuccess() {
		t.Error("skipped_fresh must not be a success")
	}
	if IngestRunOK.IsSuccess() != true || IngestRunPartial.IsSuccess() != true {
		t.Error("ok/partial must remain success")
	}
}

func TestIngestRunStatus_JSONRoundTrip(t *testing.T) {
	for _, s := range []IngestRunStatus{IngestRunRunning, IngestRunOK, IngestRunPartial, IngestRunFailed, IngestRunSkippedLocked, IngestRunSkippedFresh} {
		var round IngestRunStatus
		if err := jsonUnmarshal(`"`+s.String()+`"`, &round); err != nil || round != s {
			t.Errorf("JSON round trip %v failed: %v %v", s, round, err)
		}
	}
}

func jsonUnmarshal(data string, v interface{}) error {
	return json.Unmarshal([]byte(data), v)
}

func TestIngestRun_IsSuccess(t *testing.T) {
	if !(IngestRun{Status: IngestRunOK}).IsSuccess() {
		t.Error("ok should be success")
	}
	if !(IngestRun{Status: IngestRunPartial}).IsSuccess() {
		t.Error("partial should be success")
	}
	if (IngestRun{Status: IngestRunFailed}).IsSuccess() {
		t.Error("failed should not be success")
	}
	if (IngestRun{Status: IngestRunRunning}).IsSuccess() {
		t.Error("running should not be success")
	}
}

// TestNewIngestRunFromRecord (AC-2): status is RECOMPUTED from per-source
// outcomes, not copied from RunRecord.Status.
func TestNewIngestRunFromRecord(t *testing.T) {
	base := func() RunRecord {
		return RunRecord{
			RunID:     "test-run-1",
			StartedAt: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
			Status:    RunStatusOK,
		}
	}

	t.Run("all sources failed -> failed", func(t *testing.T) {
		rec := base()
		rec.Status = RunStatusPartial // the app's buggy status: never Failed
		rec.Sources = []SourceRunStatus{
			{SourceID: "a", Errors: []error{errors.New("a down")}},
			{SourceID: "b", Errors: []error{errors.New("b down")}},
		}
		run := NewIngestRunFromRecord(rec, "vps")
		if run.Status != IngestRunFailed {
			t.Errorf("status = %v, want failed", run.Status)
		}
		if run.SourcesChecked != 2 || run.SourcesSucceeded != 0 {
			t.Errorf("counters = %d/%d, want 2/0", run.SourcesChecked, run.SourcesSucceeded)
		}
		if run.ErrorSummary == "" {
			t.Error("expected error summary")
		}
		if run.Origin != "vps" {
			t.Errorf("origin = %q, want vps", run.Origin)
		}
	})

	t.Run("1 of 3 failed -> partial", func(t *testing.T) {
		rec := base()
		rec.Status = RunStatusPartial
		rec.Sources = []SourceRunStatus{
			{SourceID: "a"},
			{SourceID: "b", Errors: []error{errors.New("b down")}},
			{SourceID: "c"},
		}
		run := NewIngestRunFromRecord(rec, "raspberry")
		if run.Status != IngestRunPartial {
			t.Errorf("status = %v, want partial", run.Status)
		}
		if run.SourcesChecked != 3 || run.SourcesSucceeded != 2 {
			t.Errorf("counters = %d/%d, want 3/2", run.SourcesChecked, run.SourcesSucceeded)
		}
	})

	t.Run("0 failed, 0 new entries -> ok", func(t *testing.T) {
		rec := base()
		rec.Status = RunStatusOK
		rec.Sources = []SourceRunStatus{{SourceID: "a"}}
		ended := time.Date(2026, 1, 1, 10, 5, 0, 0, time.UTC)
		rec.EndedAt = &ended
		run := NewIngestRunFromRecord(rec, "github-backup")
		if run.Status != IngestRunOK {
			t.Errorf("status = %v, want ok", run.Status)
		}
		if run.FinishedAt == nil || !run.FinishedAt.Equal(ended) {
			t.Errorf("finished_at = %v, want %v", run.FinishedAt, ended)
		}
		if run.EntriesFetched != 0 {
			t.Errorf("entries_fetched = %d, want 0", run.EntriesFetched)
		}
	})

	t.Run("no sources but run-level error -> failed", func(t *testing.T) {
		rec := base()
		rec.AddError(errors.New("repository not configured"))
		run := NewIngestRunFromRecord(rec, "local")
		if run.Status != IngestRunFailed {
			t.Errorf("status = %v, want failed", run.Status)
		}
		if !strings.Contains(run.ErrorSummary, "repository not configured") {
			t.Errorf("error summary = %q", run.ErrorSummary)
		}
	})

	t.Run("sources ok but publish failed -> not ok", func(t *testing.T) {
		rec := base()
		rec.Sources = []SourceRunStatus{{SourceID: "a"}}
		rec.AddError(errors.New("homepage publish failed"))
		run := NewIngestRunFromRecord(rec, "local")
		if run.Status == IngestRunOK {
			t.Error("run with publish error must not be ok")
		}
	})
}

func TestNewIngestRunFromRecord_ErrorSummarySanitized(t *testing.T) {
	rec := RunRecord{
		RunID: "x", StartedAt: time.Now().UTC(),
		Errors: []error{errors.New("connect failed: token=abc123456789012345\nsecond line")},
	}
	run := NewIngestRunFromRecord(rec, "local")
	// The secret assignment is marked redacted.
	if !strings.Contains(run.ErrorSummary, "[REDACTED]") {
		t.Errorf("error summary should be redacted: %q", run.ErrorSummary)
	}
	// Newlines/control chars are stripped to a single line.
	if strings.ContainsAny(run.ErrorSummary, "\n\r") {
		t.Errorf("error summary has newlines: %q", run.ErrorSummary)
	}
	// The long token value (>=16 chars, secretAssignRe form) is not verbatim.
	if strings.Contains(run.ErrorSummary, "abc123456789012345") {
		t.Errorf("error summary leaks full token: %q", run.ErrorSummary)
	}
}

// TestDecideShouldRun (AC-1): boundary, clock skew, never-succeeded.
func TestDecideShouldRun_NeverSucceeded(t *testing.T) {
	d := DecideShouldRun(nil, time.Now(), 75*time.Minute)
	if !d.ShouldRun || d.Reason != ReasonNeverSucceeded || d.LastSuccessfulAt != nil {
		t.Errorf("decision = %+v", d)
	}
	// A run that never finished is also "never succeeded"
	running := &IngestRun{Status: IngestRunRunning}
	d = DecideShouldRun(running, time.Now(), 75*time.Minute)
	if !d.ShouldRun || d.Reason != ReasonNeverSucceeded {
		t.Errorf("decision = %+v", d)
	}
}

func TestDecideShouldRun_Fresh(t *testing.T) {
	last := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	success := &IngestRun{Status: IngestRunOK, FinishedAt: &last}
	now := last.Add(10 * time.Minute)
	d := DecideShouldRun(success, now, 75*time.Minute)
	if d.ShouldRun || d.Reason != ReasonFresh {
		t.Errorf("decision = %+v", d)
	}
	if d.Age != 10*time.Minute {
		t.Errorf("age = %v, want 10m", d.Age)
	}
}

func TestDecideShouldRun_Boundary(t *testing.T) {
	last := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	success := &IngestRun{Status: IngestRunPartial, FinishedAt: &last}
	// age exactly = threshold -> fresh (<=)
	d := DecideShouldRun(success, last.Add(75*time.Minute), 75*time.Minute)
	if d.ShouldRun || d.Reason != ReasonFresh {
		t.Errorf("boundary: decision = %+v, want fresh", d)
	}
	// age just over threshold -> stale
	d = DecideShouldRun(success, last.Add(75*time.Minute+1*time.Second), 75*time.Minute)
	if !d.ShouldRun || d.Reason != ReasonStale {
		t.Errorf("over boundary: decision = %+v, want stale", d)
	}
}

// TestDecideShouldRun_ClockSkew (AC-1): last_success in the future relative
// to the host clock -> age clamped to 0, fresh, never negative.
func TestDecideShouldRun_ClockSkew(t *testing.T) {
	future := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)
	success := &IngestRun{Status: IngestRunOK, FinishedAt: &future}
	past := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	d := DecideShouldRun(success, past, 75*time.Minute)
	if d.Age != 0 {
		t.Errorf("age = %v, want 0 (clamped)", d.Age)
	}
	if d.ShouldRun || d.Reason != ReasonFresh {
		t.Errorf("decision = %+v, want fresh", d)
	}
}
