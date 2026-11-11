package domain

import (
	"encoding/base64"
	"testing"
	"time"
)

func TestRunStatus_String(t *testing.T) {
	tests := []struct {
		status RunStatus
		want   string
	}{
		{RunStatusOK, "ok"},
		{RunStatusPartial, "partial"},
		{RunStatusFailed, "failed"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("RunStatus.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRunStatus_IsCritical(t *testing.T) {
	tests := []struct {
		status RunStatus
		want   bool
	}{
		{RunStatusOK, false},
		{RunStatusPartial, false},
		{RunStatusFailed, true},
	}
	for _, tt := range tests {
		t.Run(tt.status.String(), func(t *testing.T) {
			if got := tt.status.IsCritical(); got != tt.want {
				t.Errorf("RunStatus.IsCritical() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRunStatus_ExitCode(t *testing.T) {
	tests := []struct {
		status RunStatus
		want   int
	}{
		{RunStatusOK, 0},
		{RunStatusPartial, 0},
		{RunStatusFailed, 1},
	}
	for _, tt := range tests {
		t.Run(tt.status.String(), func(t *testing.T) {
			if got := tt.status.ExitCode(); got != tt.want {
				t.Errorf("RunStatus.ExitCode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRunID_NewRunID(t *testing.T) {
	id1 := NewRunID()
	id2 := NewRunID()

	// Should be unique
	if id1 == id2 {
		t.Errorf("NewRunID() generated duplicate IDs: %v", id1)
	}

	// Should be non-empty
	if id1.String() == "" {
		t.Error("NewRunID() returned empty string")
	}

	// Should be URL-safe base64
	_, err := base64.URLEncoding.DecodeString(string(id1))
	if err != nil {
		t.Errorf("NewRunID() produced non-URL-safe ID: %v", err)
	}
}

func TestSourceRunStatus_AddError(t *testing.T) {
	s := SourceRunStatus{}
	s.AddError(nil) // Should not panic or change status
	if s.HasErrors() {
		t.Error("HasErrors() after AddError(nil) should be false")
	}

	s.AddError(nil)
	if s.HasErrors() {
		t.Error("HasErrors() after multiple AddError(nil) should be false")
	}

	s.AddError(testErr)
	if !s.HasErrors() {
		t.Error("HasErrors() after AddError(non-nil) should be true")
	}
	if len(s.Errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(s.Errors))
	}
}

func TestRunRecord_AddSourceStatus(t *testing.T) {
	r := RunRecord{}
	src := SourceRunStatus{
		SourceID:       "test-source",
		EntriesFetched: 5,
		StoriesCreated: 2,
		StoriesUpdated: 1,
		StoriesSkipped: 2,
	}
	r.AddSourceStatus(src)

	if r.EntriesFetched != 5 {
		t.Errorf("EntriesFetched = %d, want 5", r.EntriesFetched)
	}
	if r.StoriesCreated != 2 {
		t.Errorf("StoriesCreated = %d, want 2", r.StoriesCreated)
	}
	if r.StoriesUpdated != 1 {
		t.Errorf("StoriesUpdated = %d, want 1", r.StoriesUpdated)
	}
	if r.StoriesSkipped != 2 {
		t.Errorf("StoriesSkipped = %d, want 2", r.StoriesSkipped)
	}
	if len(r.Sources) != 1 {
		t.Errorf("Sources count = %d, want 1", len(r.Sources))
	}
}

func TestRunRecord_AddError(t *testing.T) {
	r := RunRecord{Status: RunStatusOK}

	// Add non-critical error
	r.AddError(testErr)
	if len(r.Errors) == 0 {
		t.Error("HasErrors() should be true after AddError")
	}
	if r.Status != RunStatusFailed {
		t.Errorf("Status = %s, want Failed", r.Status)
	}

	// Add another error (should stay Failed)
	r.AddError(testErr)
	if r.Status != RunStatusFailed {
		t.Errorf("Status after second error = %s, want Failed", r.Status)
	}
}

func TestRunRecord_MarkPartial(t *testing.T) {
	tests := []struct {
		start RunStatus
		want  RunStatus
	}{
		{RunStatusOK, RunStatusPartial},
		{RunStatusPartial, RunStatusPartial},
		{RunStatusFailed, RunStatusFailed}, // Should not downgrade
	}
	for _, tt := range tests {
		t.Run(string(tt.start.String()), func(t *testing.T) {
			r := RunRecord{Status: tt.start}
			r.MarkPartial()
			if r.Status != tt.want {
				t.Errorf("MarkPartial(): status = %s, want %s", r.Status, tt.want)
			}
		})
	}
}

func TestRunRecord_MarkOK(t *testing.T) {
	tests := []struct {
		start RunStatus
		want  RunStatus
	}{
		{RunStatusOK, RunStatusOK},
		{RunStatusPartial, RunStatusOK},
		{RunStatusFailed, RunStatusFailed}, // Should not upgrade
	}
	for _, tt := range tests {
		t.Run(string(tt.start.String()), func(t *testing.T) {
			r := RunRecord{Status: tt.start}
			r.MarkOK()
			if r.Status != tt.want {
				t.Errorf("MarkOK(): status = %s, want %s", r.Status, tt.want)
			}
		})
	}
}

func TestEntryWithinLookback(t *testing.T) {
	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	window := 24 * time.Hour

	tests := []struct {
		publishedAt time.Time
		want        bool
		desc        string
	}{
		{now, true, "published at now"},
		{now.Add(-12 * time.Hour), true, "within window"},
		{now.Add(-23*time.Hour - 59*time.Minute), true, "just within window"},
		{now.Add(-25 * time.Hour), false, "outside window"},
		{now.Add(-48 * time.Hour), false, "well outside window"},
		{time.Time{}, true, "zero timestamp (undated)"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := EntryWithinLookback(tt.publishedAt, now, window)
			if got != tt.want {
				t.Errorf("EntryWithinLookback(%v, now, %v) = %v, want %v", tt.publishedAt, window, got, tt.want)
			}
		})
	}

	// window <= 0 means no lookback (all pass)
	t.Run("no lookback window", func(t *testing.T) {
		got := EntryWithinLookback(now.Add(-7*24*time.Hour), now, 0)
		if got != true {
			t.Errorf("window=0: EntryWithinLookback(7d ago, now, 0) = %v, want true", got)
		}
	})
}

func TestAggregateRunRecords(t *testing.T) {
	r1 := RunRecord{
		Status:         RunStatusOK,
		StoriesCreated: 10,
		StoriesSkipped: 5,
	}
	r2 := RunRecord{
		Status:         RunStatusPartial,
		StoriesCreated: 3,
		StoriesSkipped: 1,
		Errors:         []error{testErr},
	}

	agg := AggregateRunRecords(r1, r2)

	if agg.Status != RunStatusPartial {
		t.Errorf("Aggregated status = %s, want Partial", agg.Status)
	}
	if agg.StoriesCreated != 13 {
		t.Errorf("Aggregated created = %d, want 13", agg.StoriesCreated)
	}
	if agg.StoriesSkipped != 6 {
		t.Errorf("Aggregated skipped = %d, want 6", agg.StoriesSkipped)
	}
	if len(agg.Errors) != 1 {
		t.Errorf("Aggregated errors = %d, want 1", len(agg.Errors))
	}
}

// testErr is a test error value.
var testErr error = &testError{}

// testError is a unique error for testing.
type testError struct{}

func (e *testError) Error() string { return "test error" }
