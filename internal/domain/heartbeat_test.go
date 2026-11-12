package domain

import (
	"fmt"
	"testing"
	"time"
)

func TestHeartbeat_Validate(t *testing.T) {
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	tests := []struct {
		name    string
		hb      *Heartbeat
		wantErr bool
	}{
		{
			name: "valid heartbeat",
			hb: &Heartbeat{
				Version:        HeartbeatVersion,
				Status:         HeartbeatStatusOK,
				RunID:          "test-run-id",
				AttemptedAt:    nowStr,
				SourcesChecked: 1,
				SourcesFailed:  0,
			},
			wantErr: false,
		},
		{
			name: "missing version",
			hb: &Heartbeat{
				Status:      HeartbeatStatusOK,
				RunID:       "test-run-id",
				AttemptedAt: nowStr,
			},
			wantErr: true,
		},
		{
			name: "unsupported version",
			hb: &Heartbeat{
				Version:     "999",
				Status:      HeartbeatStatusOK,
				RunID:       "test-run-id",
				AttemptedAt: nowStr,
			},
			wantErr: true,
		},
		{
			name: "missing run_id",
			hb: &Heartbeat{
				Version:     HeartbeatVersion,
				Status:      HeartbeatStatusOK,
				AttemptedAt: nowStr,
			},
			wantErr: true,
		},
		{
			name: "missing attempted_at",
			hb: &Heartbeat{
				Version:    HeartbeatVersion,
				Status:     HeartbeatStatusOK,
				RunID:      "test-run-id",
				IngestedAt: &nowStr,
			},
			wantErr: true,
		},
		{
			name: "invalid attempted_at",
			hb: &Heartbeat{
				Version:     HeartbeatVersion,
				Status:      HeartbeatStatusOK,
				RunID:       "test-run-id",
				AttemptedAt: "not-a-date",
			},
			wantErr: true,
		},
		{
			name: "invalid ingested_at",
			hb: &Heartbeat{
				Version:     HeartbeatVersion,
				Status:      HeartbeatStatusOK,
				RunID:       "test-run-id",
				AttemptedAt: nowStr,
				IngestedAt:  strPtr("not-a-date"),
			},
			wantErr: true,
		},
		{
			name: "invalid published_at",
			hb: &Heartbeat{
				Version:     HeartbeatVersion,
				Status:      HeartbeatStatusOK,
				RunID:       "test-run-id",
				AttemptedAt: nowStr,
				PublishedAt: strPtr("not-a-date"),
			},
			wantErr: true,
		},
		{
			name: "ingested_at and published_at set",
			hb: &Heartbeat{
				Version:     HeartbeatVersion,
				Status:      HeartbeatStatusOK,
				RunID:       "test-run-id",
				AttemptedAt: nowStr,
				IngestedAt:  &nowStr,
				PublishedAt: &nowStr,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.hb.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Heartbeat.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHeartbeat_IsStale(t *testing.T) {
	now := time.Now().UTC()
	fresh := now.Add(-5 * time.Minute).Format(time.RFC3339)
	stale := now.Add(-2 * time.Hour).Format(time.RFC3339)

	tests := []struct {
		name      string
		hb        *Heartbeat
		threshold time.Duration
		wantStale bool
	}{
		{
			name: "fresh heartbeat",
			hb: &Heartbeat{
				Version:     HeartbeatVersion,
				Status:      HeartbeatStatusOK,
				RunID:       "test",
				AttemptedAt: now.Format(time.RFC3339),
				PublishedAt: &fresh,
			},
			threshold: 90 * time.Minute,
			wantStale: false,
		},
		{
			name: "stale heartbeat",
			hb: &Heartbeat{
				Version:     HeartbeatVersion,
				Status:      HeartbeatStatusOK,
				RunID:       "test",
				AttemptedAt: now.Format(time.RFC3339),
				PublishedAt: &stale,
			},
			threshold: 90 * time.Minute,
			wantStale: true,
		},
		{
			name: "missing published_at",
			hb: &Heartbeat{
				Version:     HeartbeatVersion,
				Status:      HeartbeatStatusOK,
				RunID:       "test",
				AttemptedAt: now.Format(time.RFC3339),
			},
			threshold: 90 * time.Minute,
			wantStale: true,
		},
		{
			name: "invalid published_at format",
			hb: &Heartbeat{
				Version:     HeartbeatVersion,
				Status:      HeartbeatStatusOK,
				RunID:       "test",
				AttemptedAt: now.Format(time.RFC3339),
				PublishedAt: strPtr("not-a-date"),
			},
			threshold: 90 * time.Minute,
			wantStale: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.hb.IsStale(now, tt.threshold)
			if got != tt.wantStale {
				t.Errorf("Heartbeat.IsStale() = %v, want %v", got, tt.wantStale)
			}
		})
	}
}

func TestHeartbeat_FromRunRecord(t *testing.T) {
	now := time.Now().UTC()
	runID := "test-run-123"

	tests := []struct {
		name    string
		record  RunRecord
		want    *Heartbeat
		wantErr bool
	}{
		{
			name: "successful run",
			record: RunRecord{
				RunID:     RunID(runID),
				StartedAt: now.Add(-1 * time.Hour),
				EndedAt:   &now,
				Status:    RunStatusOK,
				Sources: []SourceRunStatus{
					{SourceID: "espn", EntriesFetched: 10, StoriesCreated: 8, StoriesUpdated: 1, StoriesSkipped: 1},
				},
				EntriesFetched: 10,
				StoriesCreated: 8,
				StoriesUpdated: 1,
				StoriesSkipped: 1,
				Committed:      true,
			},
			want: &Heartbeat{
				Version:        HeartbeatVersion,
				Status:         HeartbeatStatusOK,
				RunID:          runID,
				AttemptedAt:    now.UTC().Format(time.RFC3339),
				IngestedAt:     strPtr(now.UTC().Format(time.RFC3339)),
				SourcesChecked: 1,
				SourcesFailed:  0,
			},
			wantErr: false,
		},
		{
			name: "partial run with one failed source",
			record: RunRecord{
				RunID:     RunID(runID),
				StartedAt: now.Add(-1 * time.Hour),
				EndedAt:   &now,
				Status:    RunStatusPartial,
				Sources: []SourceRunStatus{
					{SourceID: "espn", EntriesFetched: 10, StoriesCreated: 10},
					{SourceID: "nfl.com", EntriesFetched: 0, Errors: []error{fmt.Errorf("connection refused")}},
				},
				EntriesFetched: 10,
				StoriesCreated: 8,
				StoriesUpdated: 1,
				StoriesSkipped: 1,
				Committed:      true,
			},
			want: &Heartbeat{
				Version:        HeartbeatVersion,
				Status:         HeartbeatStatusPartial,
				RunID:          runID,
				AttemptedAt:    now.UTC().Format(time.RFC3339),
				IngestedAt:     strPtr(now.UTC().Format(time.RFC3339)),
				SourcesChecked: 2,
				SourcesFailed:  1,
				Errors:         []string{"[source:nfl.com] connection refused"},
			},
			wantErr: false,
		},
		{
			name: "failed run with top-level error",
			record: RunRecord{
				RunID:     RunID(runID),
				StartedAt: now.Add(-1 * time.Hour),
				EndedAt:   &now,
				Status:    RunStatusFailed,
				Errors:    []error{fmt.Errorf("repository not configured")},
				Committed: false,
			},
			want: &Heartbeat{
				Version:        HeartbeatVersion,
				Status:         HeartbeatStatusFailed,
				RunID:          runID,
				AttemptedAt:    now.UTC().Format(time.RFC3339),
				SourcesChecked: 0,
				SourcesFailed:  0,
				Errors:         []string{"repository not configured"},
			},
			wantErr: false,
		},
		{
			name: "successful run with top-level error appended",
			record: RunRecord{
				RunID:     RunID(runID),
				StartedAt: now.Add(-1 * time.Hour),
				EndedAt:   &now,
				Status:    RunStatusPartial,
				Sources: []SourceRunStatus{
					{SourceID: "espn", EntriesFetched: 10, StoriesCreated: 10},
				},
				Errors:    []error{fmt.Errorf("non-fatal warning")},
				Committed: true,
			},
			want: &Heartbeat{
				Version:        HeartbeatVersion,
				Status:         HeartbeatStatusPartial,
				RunID:          runID,
				AttemptedAt:    now.UTC().Format(time.RFC3339),
				IngestedAt:     strPtr(now.UTC().Format(time.RFC3339)),
				SourcesChecked: 1,
				SourcesFailed:  0,
				Errors:         []string{"non-fatal warning"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hb := NewHeartbeat(tt.record.RunID.String())
			hb.FromRunRecord(tt.record, now)

			if err := hb.Validate(); err != nil {
				if !tt.wantErr {
					t.Errorf("Heartbeat.Validate() error = %v, wantErr %v", err, tt.wantErr)
				}
			} else if tt.wantErr {
				t.Errorf("Heartbeat.Validate() error = nil, wantErr true")
			}

			if hb.Status != tt.want.Status {
				t.Errorf("Status = %v, want %v", hb.Status, tt.want.Status)
			}
			if hb.RunID != tt.want.RunID {
				t.Errorf("RunID = %v, want %v", hb.RunID, tt.want.RunID)
			}
			if hb.SourcesChecked != tt.want.SourcesChecked {
				t.Errorf("SourcesChecked = %d, want %d", hb.SourcesChecked, tt.want.SourcesChecked)
			}
			if hb.SourcesFailed != tt.want.SourcesFailed {
				t.Errorf("SourcesFailed = %d, want %d", hb.SourcesFailed, tt.want.SourcesFailed)
			}
			// For errors, check count matches
			if len(hb.Errors) != len(tt.want.Errors) {
				t.Errorf("Errors count = %d, want %d: %v", len(hb.Errors), len(tt.want.Errors), hb.Errors)
			}
			// Verify top-level error if present
			if len(tt.record.Errors) > 0 && len(hb.Errors) > 0 {
				if !containsString(hb.Errors, tt.record.Errors[0].Error()) {
					t.Errorf("Top-level error not found in heartbeat errors: %v", hb.Errors)
				}
			}
		})
	}
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func TestHeartbeat_MergePublication(t *testing.T) {
	now := time.Now().UTC()
	oldTime := now.Add(-10 * time.Minute).Format(time.RFC3339)
	newTime := now.Format(time.RFC3339)

	tests := []struct {
		name   string
		hb     *Heartbeat
		result PublicationResult
		want   *Heartbeat
	}{
		{
			name: "committed publication updates published_at",
			hb: &Heartbeat{
				Version:     HeartbeatVersion,
				Status:      HeartbeatStatusOK,
				RunID:       "test",
				AttemptedAt: now.Format(time.RFC3339),
				PublishedAt: strPtr(oldTime),
			},
			result: PublicationResult{
				Committed:   true,
				PublishedAt: newTime,
				Errors:      nil,
			},
			want: &Heartbeat{
				Version:     HeartbeatVersion,
				Status:      HeartbeatStatusOK,
				RunID:       "test",
				AttemptedAt: now.Format(time.RFC3339),
				PublishedAt: strPtr(newTime),
			},
		},
		{
			name: "failed publish keeps published_at and sets status failed",
			hb: &Heartbeat{
				Version:     HeartbeatVersion,
				Status:      HeartbeatStatusOK,
				RunID:       "test",
				AttemptedAt: now.Format(time.RFC3339),
				PublishedAt: strPtr(oldTime),
			},
			result: PublicationResult{
				Committed:   false,
				PublishedAt: "",
				Errors:      []error{fmt.Errorf("write failed")},
			},
			want: &Heartbeat{
				Version:     HeartbeatVersion,
				Status:      HeartbeatStatusFailed,
				RunID:       "test",
				AttemptedAt: now.Format(time.RFC3339),
				PublishedAt: strPtr(oldTime), // unchanged
				Errors:      []string{"[publish] write failed"},
			},
		},
		{
			name: "later published_at overwrites earlier",
			hb: &Heartbeat{
				Version:     HeartbeatVersion,
				Status:      HeartbeatStatusOK,
				RunID:       "test",
				AttemptedAt: now.Format(time.RFC3339),
				PublishedAt: strPtr(oldTime),
			},
			result: PublicationResult{
				Committed:   true,
				PublishedAt: newTime,
				Errors:      nil,
			},
			want: &Heartbeat{
				Version:     HeartbeatVersion,
				Status:      HeartbeatStatusOK,
				RunID:       "test",
				AttemptedAt: now.Format(time.RFC3339),
				PublishedAt: strPtr(newTime),
			},
		},
		{
			name: "earlier published_at does not overwrite newer",
			hb: &Heartbeat{
				Version:     HeartbeatVersion,
				Status:      HeartbeatStatusOK,
				RunID:       "test",
				AttemptedAt: now.Format(time.RFC3339),
				PublishedAt: strPtr(newTime),
			},
			result: PublicationResult{
				Committed:   true,
				PublishedAt: oldTime,
				Errors:      nil,
			},
			want: &Heartbeat{
				Version:     HeartbeatVersion,
				Status:      HeartbeatStatusOK,
				RunID:       "test",
				AttemptedAt: now.Format(time.RFC3339),
				PublishedAt: strPtr(newTime), // unchanged
			},
		},
		{
			name: "failed publish on previously successful run keeps published_at",
			hb: &Heartbeat{
				Version:     HeartbeatVersion,
				Status:      HeartbeatStatusOK,
				RunID:       "test",
				AttemptedAt: now.Format(time.RFC3339),
				PublishedAt: strPtr(oldTime),
			},
			result: PublicationResult{
				Committed:   false,
				PublishedAt: "",
				Errors:      []error{fmt.Errorf("commit failed")},
			},
			want: &Heartbeat{
				Version:     HeartbeatVersion,
				Status:      HeartbeatStatusFailed,
				RunID:       "test",
				AttemptedAt: now.Format(time.RFC3339),
				PublishedAt: strPtr(oldTime), // preserved
				Errors:      []string{"[publish] commit failed"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.hb.MergePublication(tt.result, now)
			if err := tt.hb.Validate(); err != nil {
				t.Errorf("Heartbeat.Validate() error = %v", err)
			}
			if tt.hb.Status != tt.want.Status {
				t.Errorf("Status = %v, want %v", tt.hb.Status, tt.want.Status)
			}
			if tt.hb.PublishedAt != nil && tt.want.PublishedAt != nil {
				if *tt.hb.PublishedAt != *tt.want.PublishedAt {
					t.Errorf("PublishedAt = %v, want %v", *tt.hb.PublishedAt, *tt.want.PublishedAt)
				}
			} else if (tt.hb.PublishedAt == nil) != (tt.want.PublishedAt == nil) {
				t.Errorf("PublishedAt mismatch")
			}
		})
	}
}

func strPtr(s string) *string {
	return &s
}
