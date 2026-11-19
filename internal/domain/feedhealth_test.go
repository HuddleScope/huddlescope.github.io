package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestFeedStatus_JSONRoundTrip(t *testing.T) {
	cases := []struct {
		status FeedStatus
		want   string
	}{
		{FeedStatusHealthy, "healthy"},
		{FeedStatusNoUpdates, "no_updates"},
		{FeedStatusFailed, "failed"},
		{FeedStatusDisabled, "disabled"},
		{FeedStatusNotChecked, "not_checked"},
	}

	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			// Marshal to JSON
			data, err := json.Marshal(tc.status)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			wantJSON := `"` + tc.want + `"`
			if string(data) != wantJSON {
				t.Errorf("Marshal: got %s, want %s", data, wantJSON)
			}

			// Unmarshal back
			var decoded FeedStatus
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if decoded != tc.status {
				t.Errorf("Unmarshal: got %v (%s), want %v (%s)",
					decoded, decoded.String(), tc.status, tc.status.String())
			}
		})
	}
}

func TestFeedStatus_String(t *testing.T) {
	tests := []struct {
		status FeedStatus
		want   string
	}{
		{FeedStatusHealthy, "healthy"},
		{FeedStatusNoUpdates, "no_updates"},
		{FeedStatusFailed, "failed"},
		{FeedStatusDisabled, "disabled"},
		{FeedStatusNotChecked, "not_checked"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("FeedStatus.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeriveFeedStatus(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name string
		in   DeriveFeedStatusInput
		want FeedStatus
	}{
		{
			name: "disabled source",
			in: DeriveFeedStatusInput{
				Enabled:      false,
				CheckedAt:    &now,
				SucceededAt:  &now,
				Errors:       nil,
				EntriesTotal: 0,
			},
			want: FeedStatusDisabled,
		},
		{
			name: "not checked yet",
			in: DeriveFeedStatusInput{
				Enabled:      true,
				CheckedAt:    nil,
				SucceededAt:  nil,
				Errors:       nil,
				EntriesTotal: 0,
			},
			want: FeedStatusNotChecked,
		},
		{
			name: "successful with new content",
			in: DeriveFeedStatusInput{
				Enabled:        true,
				CheckedAt:      &now,
				SucceededAt:    &now,
				Errors:         nil,
				EntriesTotal:   10,
				StoriesCreated: 5,
				StoriesSkipped: 2,
			},
			want: FeedStatusHealthy,
		},
		{
			name: "successful but no new content",
			in: DeriveFeedStatusInput{
				Enabled:        true,
				CheckedAt:      &now,
				SucceededAt:    &now,
				Errors:         nil,
				EntriesTotal:   10,
				StoriesCreated: 0,
				StoriesSkipped: 10,
			},
			want: FeedStatusNoUpdates,
		},
		{
			name: "failed with error",
			in: DeriveFeedStatusInput{
				Enabled:      true,
				CheckedAt:    &now,
				SucceededAt:  nil,
				Errors:       []error{testErr},
				EntriesTotal: 0,
			},
			want: FeedStatusFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveFeedStatus(tt.in)
			if got != tt.want {
				t.Errorf("DeriveFeedStatus() = %v, want %v", got.String(), tt.want.String())
			}
		})
	}
}

func TestIsStale(t *testing.T) {
	now := time.Now().UTC()
	yesterday := now.Add(-25 * time.Hour)
	twoHoursAgo := now.Add(-2 * time.Hour)

	tests := []struct {
		name      string
		enabled   bool
		succeeded *time.Time
		now       time.Time
		want      bool
	}{
		{
			name:      "disabled is never stale",
			enabled:   false,
			succeeded: &yesterday,
			now:       now,
			want:      false,
		},
		{
			name:      "never succeeded is not stale",
			enabled:   true,
			succeeded: nil,
			now:       now,
			want:      false,
		},
		{
			name:      "within 24h is not stale",
			enabled:   true,
			succeeded: &twoHoursAgo,
			now:       now,
			want:      false,
		},
		{
			name:      "over 24h is stale",
			enabled:   true,
			succeeded: &yesterday,
			now:       now,
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsStale(tt.enabled, tt.succeeded, tt.now)
			if got != tt.want {
				t.Errorf("IsStale(enabled=%v, succeededAt=%v, now) = %v, want %v", tt.enabled, tt.succeeded, got, tt.want)
			}
		})
	}
}

func TestSanitizeFeedError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		checkFn func(string) error
	}{
		{
			name: "nil error",
			err:  nil,
			checkFn: func(got string) error {
				if len(got) != 0 {
					return fmt.Errorf("len = %d, want 0", len(got))
				}
				return nil
			},
		},
		{
			name: "simple error",
			err:  testErr,
			checkFn: func(got string) error {
				if got != "test error" {
					return fmt.Errorf("got %q, want %q", got, "test error")
				}
				return nil
			},
		},
		{
			name: "ghp_ token redacted",
			err:  fmt.Errorf("failed: ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"),
			checkFn: func(got string) error {
				if !strings.Contains(got, "[REDACTED]") {
					return fmt.Errorf("expected redaction, got %s", got)
				}
				if strings.Contains(got, "ghp_") {
					return fmt.Errorf("token not redacted in %s", got)
				}
				return nil
			},
		},
		{
			name: "gho_ token redacted",
			err:  fmt.Errorf("token: gho_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"),
			checkFn: func(got string) error {
				if !strings.Contains(got, "[REDACTED]") {
					return fmt.Errorf("expected redaction, got %s", got)
				}
				if strings.Contains(got, "gho_") {
					return fmt.Errorf("token not redacted in %s", got)
				}
				return nil
			},
		},
		{
			name: "access_token redacted",
			err:  fmt.Errorf("error: access_token=secret123"),
			checkFn: func(got string) error {
				if !strings.Contains(got, "[REDACTED]") {
					return fmt.Errorf("expected redaction, got %s", got)
				}
				return nil
			},
		},
		{
			name: "password redacted",
			err:  fmt.Errorf("db_password=secret123"),
			checkFn: func(got string) error {
				if !strings.Contains(got, "[REDACTED]") {
					return fmt.Errorf("expected redaction, got %s", got)
				}
				return nil
			},
		},
		{
			name: "over length truncated",
			err:  fmt.Errorf("%s", strings.Repeat("a", 1000)),
			checkFn: func(got string) error {
				if len(got) > 500 {
					return fmt.Errorf("len = %d, should be <= 500", len(got))
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeFeedError(tt.err)
			if err := tt.checkFn(got); err != nil {
				t.Errorf("SanitizeFeedError() %s", err)
			}
		})
	}
}

func TestIsSecretPattern(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty", "", false},
		{"plain prose", "Quarterback throws two touchdowns", false},
		{"prose quote with secret", `I've just got to continue to get better. That's not a secret`, false},
		{"secretary in title", "U.S. Secretary of Education honors students for improved attendance", false},
		{"token as role word", "Coaches use him as a token: receiver on third down", false},
		{"oauth2 fetch error", "oauth2: cannot fetch token: 401 Unauthorized", false},
		{"token assignment", "error: access_token=123456789012345678", true},
		{"password quoted", `password: "hunter2hunter2hunter2"`, true},
		{"bearer header", "Authorization: Bearer ghp_123456789012345678", true},
		{"database url assignment", "database_url=postgres://user:pw@db:5432/feed", true},
		{"redacted marker", "token=[REDACTED]", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSecretPattern(tt.input); got != tt.want {
				t.Errorf("isSecretPattern(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeName(t *testing.T) {
	got := SanitizeName("Test Name")
	if len(got) != len("Test Name") {
		t.Errorf("SanitizeName() len = %d, want %d", len(got), len("Test Name"))
	}
}

func TestSanitizeURL(t *testing.T) {
	got := SanitizeURL("https://example.com")
	if len(got) != len("https://example.com") {
		t.Errorf("SanitizeURL() len = %d, want %d", len(got), len("https://example.com"))
	}
}

func TestFeedHealth_Render(t *testing.T) {
	fh := FeedHealth{
		Version:     FeedHealthVersion,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		RunID:       "test-run-id",
		RunStatus:   RunStatusOK,
		Sources:     []FeedHealthFeed{},
		Summary:     FeedHealthSummary{},
	}

	b, err := fh.Render()
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if len(b) == 0 {
		t.Error("Render() returned empty bytes")
	}
}

func TestFeedHealth_Validate(t *testing.T) {
	tests := []struct {
		name string
		fh   FeedHealth
		want error
	}{
		{
			name: "valid minimal",
			fh: FeedHealth{
				Version:     FeedHealthVersion,
				GeneratedAt: time.Now().UTC().Format(time.RFC3339),
				RunID:       "test-id",
				RunStatus:   RunStatusOK,
				Sources:     []FeedHealthFeed{},
				Summary:     FeedHealthSummary{},
			},
			want: nil,
		},
		{
			name: "invalid version",
			fh: FeedHealth{
				Version:     "invalid",
				GeneratedAt: time.Now().UTC().Format(time.RFC3339),
				RunID:       "test-id",
				RunStatus:   RunStatusOK,
				Sources:     []FeedHealthFeed{},
				Summary:     FeedHealthSummary{},
			},
			want: fmt.Errorf("invalid version"),
		},
		{
			name: "empty generatedAt",
			fh: FeedHealth{
				Version:     FeedHealthVersion,
				GeneratedAt: "",
				RunID:       "test-id",
				RunStatus:   RunStatusOK,
				Sources:     []FeedHealthFeed{},
				Summary:     FeedHealthSummary{},
			},
			want: errors.New("generatedAt required"),
		},
		{
			name: "empty run_id",
			fh: FeedHealth{
				Version:     FeedHealthVersion,
				GeneratedAt: time.Now().UTC().Format(time.RFC3339),
				RunID:       "",
				RunStatus:   RunStatusOK,
				Sources:     []FeedHealthFeed{},
				Summary:     FeedHealthSummary{},
			},
			want: errors.New("run_id required"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fh.Validate()
			if tt.want == nil && err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
			if tt.want != nil && err == nil {
				t.Errorf("Validate() expected error %v, got nil", tt.want)
			}
			if tt.want != nil && err != nil {
				if !strings.Contains(err.Error(), tt.want.Error()) {
					t.Errorf("Validate() error %v does not contain %v", err, tt.want)
				}
			}
		})
	}
}

func TestFeedHealth_ContentEqual(t *testing.T) {
	now := time.Now().UTC()

	fh1 := FeedHealth{
		Version:     FeedHealthVersion,
		GeneratedAt: now.Format(time.RFC3339),
		RunID:       "test-id",
		RunStatus:   RunStatusOK,
		Sources:     []FeedHealthFeed{},
		Summary:     FeedHealthSummary{Configured: 10},
	}

	fh2 := FeedHealth{
		Version:     FeedHealthVersion,
		GeneratedAt: now.Add(time.Minute).Format(time.RFC3339), // different timestamp
		RunID:       "test-id",
		RunStatus:   RunStatusOK,
		Sources:     []FeedHealthFeed{},
		Summary:     FeedHealthSummary{Configured: 10},
	}

	fh3 := FeedHealth{
		Version:     FeedHealthVersion,
		GeneratedAt: now.Format(time.RFC3339),
		RunID:       "test-id",
		RunStatus:   RunStatusOK,
		Sources:     []FeedHealthFeed{},
		Summary:     FeedHealthSummary{Configured: 5}, // different summary
	}

	if !fh1.ContentEqual(fh2) {
		t.Errorf("ContentEqual() with different generatedAt should be true")
	}
	if fh1.ContentEqual(fh3) {
		t.Errorf("ContentEqual() with different summary should be false")
	}
}

func TestFeedHealthFeed_equal(t *testing.T) {
	now := time.Now().UTC()
	feed1 := FeedHealthFeed{
		SourceID:     "test-source",
		Name:         "Test Feed",
		URL:          "https://example.com",
		Category:     "national",
		Status:       FeedStatusHealthy,
		LastChecked:  &now,
		EntriesTotal: 10,
		New:          5,
		Duplicates:   5,
		HTTPStatus:   200,
		Stale:        false,
	}

	feed2 := FeedHealthFeed{
		SourceID:     "test-source",
		Name:         "Test Feed",
		URL:          "https://example.com",
		Category:     "national",
		Status:       FeedStatusHealthy,
		LastChecked:  &now,
		EntriesTotal: 10,
		New:          5,
		Duplicates:   5,
		HTTPStatus:   200,
		Stale:        false,
	}

	feed3 := FeedHealthFeed{
		SourceID:     "test-source",
		Name:         "Test Feed",
		URL:          "https://example.com",
		Category:     "national",
		Status:       FeedStatusHealthy,
		LastChecked:  &now,
		EntriesTotal: 10,
		New:          5,
		Duplicates:   4, // different
		HTTPStatus:   200,
		Stale:        false,
	}

	if !feed1.equal(feed2) {
		t.Errorf("equal() with same values should be true")
	}
	if feed1.equal(feed3) {
		t.Errorf("equal() with different duplicates should be false")
	}
}
