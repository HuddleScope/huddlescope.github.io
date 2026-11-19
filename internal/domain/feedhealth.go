// Package domain contains core domain models and interfaces.
package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// FeedHealthVersion is the current version of the feed health export format.
const FeedHealthVersion = "1"

// FeedHealth represents the complete feed health snapshot for the dashboard.
type FeedHealth struct {
	Version     string            `json:"version"`
	GeneratedAt string            `json:"generatedAt"`
	RunID       string            `json:"run_id"`
	RunStatus   RunStatus         `json:"run_status"`
	Duration    time.Duration     `json:"duration_ms"`
	Sources     []FeedHealthFeed  `json:"feeds"`
	Summary     FeedHealthSummary `json:"summary"`
}

// FeedHealthSummary holds aggregated metrics about the run.
type FeedHealthSummary struct {
	Configured int `json:"configured"`
	Enabled    int `json:"enabled"`
	Disabled   int `json:"disabled"`
	Processed  int `json:"processed"`
	Failed     int `json:"failed"`
	NoUpdates  int `json:"no_updates"`
	Healthy    int `json:"healthy"`
	NotChecked int `json:"not_checked"`
	Stale      int `json:"stale"`
	Entries    int `json:"entries"`
	New        int `json:"new"`
	Duplicates int `json:"duplicates"`
	Groups     int `json:"groups"`
}

// FeedHealthFeed represents a single feed and its health status.
type FeedHealthFeed struct {
	SourceID string     `json:"source_id"`
	Name     string     `json:"name"`
	URL      string     `json:"url"`
	Category string     `json:"category,omitempty"`
	Status   FeedStatus `json:"status"`
	// StatusCSS is the string representation for Hugo template use
	StatusCSS     string        `json:"statusCSS,omitempty"`
	LastChecked   *time.Time    `json:"last_checked,omitempty"`
	LastSucceeded *time.Time    `json:"last_succeeded,omitempty"`
	EntriesTotal  int           `json:"entries_total"`
	New           int           `json:"new"`
	Duplicates    int           `json:"duplicates"`
	Duration      time.Duration `json:"duration_ms"`
	HTTPStatus    int           `json:"http_status"`
	Error         string        `json:"error,omitempty"`
	Stale         bool          `json:"stale"`
}

// FeedStatus represents the health status of a feed.
type FeedStatus int

const (
	FeedStatusHealthy FeedStatus = iota
	FeedStatusNoUpdates
	FeedStatusFailed
	FeedStatusDisabled
	FeedStatusNotChecked
)

func (s FeedStatus) String() string {
	switch s {
	case FeedStatusHealthy:
		return "healthy"
	case FeedStatusNoUpdates:
		return "no_updates"
	case FeedStatusFailed:
		return "failed"
	case FeedStatusDisabled:
		return "disabled"
	case FeedStatusNotChecked:
		return "not_checked"
	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}

// MarshalJSON implements json.Marshaler for FeedStatus, outputting
// a human-readable string instead of an integer. This ensures Hugo
// templates receive "healthy"/"no_updates"/etc for CSS classes and
// data attributes on the feed dashboard page.
func (s FeedStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// UnmarshalJSON implements json.Unmarshaler for FeedStatus,
// parsing a JSON string back to the correct const value.
func (s *FeedStatus) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	switch str {
	case "healthy":
		*s = FeedStatusHealthy
	case "no_updates":
		*s = FeedStatusNoUpdates
	case "failed":
		*s = FeedStatusFailed
	case "disabled":
		*s = FeedStatusDisabled
	case "not_checked":
		*s = FeedStatusNotChecked
	default:
		*s = FeedStatus(0) // default to healthy for unknown values
	}
	return nil
}

// DeriveFeedStatusInput holds the inputs needed to derive a feed's status.
type DeriveFeedStatusInput struct {
	Enabled        bool
	CheckedAt      *time.Time
	SucceededAt    *time.Time
	Errors         []error
	EntriesTotal   int
	StoriesCreated int
	StoriesSkipped int
}

// DeriveFeedStatus determines the health status of a feed based on inputs.
func DeriveFeedStatus(in DeriveFeedStatusInput) FeedStatus {
	// Disabled takes precedence
	if !in.Enabled {
		return FeedStatusDisabled
	}

	// Not checked (never attempted)
	if in.CheckedAt == nil {
		return FeedStatusNotChecked
	}

	// Has errors → Failed
	if len(in.Errors) > 0 {
		return FeedStatusFailed
	}

	// Success with new content → Healthy
	if in.StoriesCreated > 0 {
		return FeedStatusHealthy
	}

	// Success but no new content → NoUpdates
	return FeedStatusNoUpdates
}

// IsStale returns true if a feed is stale (enabled but no successful check in >24h).
func IsStale(enabled bool, succeededAt *time.Time, now time.Time) bool {
	if !enabled || succeededAt == nil {
		return false
	}
	return now.Sub(*succeededAt) > 24*time.Hour
}

// SanitizeFeedError sanitizes an error for display in the dashboard.
// Redacts secrets (tokens, keys, passwords) and caps length.
func SanitizeFeedError(err error) string {
	if err == nil {
		return ""
	}
	return limitAndRedact(err.Error(), 500)
}

// SanitizeName sanitizes a feed name for display.
// Redacts secrets and caps length.
func SanitizeName(name string) string {
	return limitAndRedact(name, 200)
}

// SanitizeURL sanitizes a feed URL for display.
// Redacts secrets and caps length.
func SanitizeURL(url string) string {
	return limitAndRedact(url, 500)
}

// limitAndRedact truncates s to maxLen and redacts secrets, adding "..." if truncated.
func limitAndRedact(s string, maxLen int) string {
	if len(s) == 0 {
		return ""
	}
	redacted := redactSecrets(s)
	if len(redacted) <= maxLen {
		return redacted
	}
	trunc := maxLen - 3
	if trunc < 0 {
		trunc = 0
	}
	return redacted[:trunc] + "..."
}

// redactSecrets replaces common secret patterns with [REDACTED].
func redactSecrets(s string) string {
	replacer := strings.NewReplacer(
		// GitHub tokens
		"ghp_", "[REDACTED]",
		"gho_", "[REDACTED]",
		// OAuth tokens
		"access_token=", "access_token=[REDACTED]",
		"refresh_token=", "refresh_token=[REDACTED]",
		"token=", "token=[REDACTED]",
		"Bearer ", "Bearer [REDACTED]",
		// Common secret patterns
		"password=", "password=[REDACTED]",
		"api_key=", "api_key=[REDACTED]",
		"apikey=", "apikey=[REDACTED]",
		"private_key=", "private_key=[REDACTED]",
		"auth_token=", "auth_token=[REDACTED]",
		"secret=", "secret=[REDACTED]",
		"database_url=", "database_url=[REDACTED]",
		"db_password=", "db_password=[REDACTED]",
		"aws_secret=", "aws_secret=[REDACTED]",
		// Regex-like patterns for tokens (case-insensitive)
	)
	// Apply basic replacements
	result := replacer.Replace(s)

	// Additional regex-like checks (case-insensitive)
	lower := strings.ToLower(s)

	// Detect ghp_ followed by 36 alphanumeric chars
	if idx := strings.Index(lower, "ghp_"); idx >= 0 {
		end := idx + 4
		for end < len(s) && (s[end] == '-' || s[end] == '_' || (s[end] >= 'a' && s[end] <= 'z') || (s[end] >= '0' && s[end] <= '9')) {
			end++
		}
		if end-idx >= 4 {
			result = result[:idx] + "[REDACTED]" + result[end:]
		}
	}

	// Detect gho_ followed by 36 alphanumeric chars
	if idx := strings.Index(lower, "gho_"); idx >= 0 {
		end := idx + 4
		for end < len(s) && (s[end] == '-' || s[end] == '_' || (s[end] >= 'a' && s[end] <= 'z') || (s[end] >= '0' && s[end] <= '9')) {
			end++
		}
		if end-idx >= 4 {
			result = result[:idx] + "[REDACTED]" + result[end:]
		}
	}

	return result
}

// Render produces canonical JSON with stable field order.
func (f FeedHealth) Render() ([]byte, error) {
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	return b, nil
}

// Validate checks the feed health snapshot for required fields and security constraints.
func (f FeedHealth) Validate() error {
	if f.Version != FeedHealthVersion {
		return fmt.Errorf("invalid version %q; must be %q", f.Version, FeedHealthVersion)
	}
	if f.GeneratedAt == "" {
		return errors.New("generatedAt required")
	}
	if _, err := time.Parse(time.RFC3339, f.GeneratedAt); err != nil {
		return fmt.Errorf("invalid generatedAt format: %w", err)
	}
	if f.RunID == "" {
		return errors.New("run_id required")
	}
	if f.Sources == nil {
		return errors.New("sources must not be nil")
	}
	for i, feed := range f.Sources {
		if feed.SourceID == "" {
			return fmt.Errorf("feed %d has empty source_id", i)
		}
		if feed.Status == FeedStatus(0) {
			return fmt.Errorf("feed %d (%s) has empty status", i, feed.SourceID)
		}
		if feed.Error != "" && len(feed.Error) > 500 {
			return fmt.Errorf("feed %d (%s) error too long (%d chars)", i, feed.SourceID, len(feed.Error))
		}
		// Check for potential secret leaks in error (defense in depth)
		if isSecretPattern(feed.Error) {
			return fmt.Errorf("feed %d (%s) error appears to contain secrets", i, feed.SourceID)
		}
	}
	if f.Summary.Configured < 0 {
		return errors.New("summary.configured must be >= 0")
	}
	if f.Summary.Enabled < 0 {
		return errors.New("summary.enabled must be >= 0")
	}
	return nil
}

// ContentEqual returns true if content matches excluding the generatedAt timestamp.
func (f FeedHealth) ContentEqual(other FeedHealth) bool {
	if f.Version != other.Version {
		return false
	}
	if len(f.Sources) != len(other.Sources) {
		return false
	}
	if f.RunID != other.RunID {
		return false
	}
	if f.RunStatus != other.RunStatus {
		return false
	}
	if f.Duration != other.Duration {
		return false
	}

	// Compare feeds
	for i := range f.Sources {
		if !f.Sources[i].equal(other.Sources[i]) {
			return false
		}
	}

	// Compare summary
	if f.Summary.Configured != other.Summary.Configured {
		return false
	}
	if f.Summary.Enabled != other.Summary.Enabled {
		return false
	}
	if f.Summary.Disabled != other.Summary.Disabled {
		return false
	}
	if f.Summary.Processed != other.Summary.Processed {
		return false
	}
	if f.Summary.Failed != other.Summary.Failed {
		return false
	}
	if f.Summary.NoUpdates != other.Summary.NoUpdates {
		return false
	}
	if f.Summary.Healthy != other.Summary.Healthy {
		return false
	}
	if f.Summary.NotChecked != other.Summary.NotChecked {
		return false
	}
	if f.Summary.Stale != other.Summary.Stale {
		return false
	}
	if f.Summary.Entries != other.Summary.Entries {
		return false
	}
	if f.Summary.New != other.Summary.New {
		return false
	}
	if f.Summary.Duplicates != other.Summary.Duplicates {
		return false
	}
	if f.Summary.Groups != other.Summary.Groups {
		return false
	}

	return true
}

func (f FeedHealthFeed) equal(other FeedHealthFeed) bool {
	if f.SourceID != other.SourceID {
		return false
	}
	if f.Name != other.Name {
		return false
	}
	if f.URL != other.URL {
		return false
	}
	if f.Category != other.Category {
		return false
	}
	if f.Status != other.Status {
		return false
	}
	if (f.LastChecked == nil) != (other.LastChecked == nil) {
		return false
	}
	if f.LastChecked != nil && other.LastChecked != nil && !f.LastChecked.Equal(*other.LastChecked) {
		return false
	}
	if (f.LastSucceeded == nil) != (other.LastSucceeded == nil) {
		return false
	}
	if f.LastSucceeded != nil && other.LastSucceeded != nil && !f.LastSucceeded.Equal(*other.LastSucceeded) {
		return false
	}
	if f.EntriesTotal != other.EntriesTotal {
		return false
	}
	if f.New != other.New {
		return false
	}
	if f.Duplicates != other.Duplicates {
		return false
	}
	if f.Duration != other.Duration {
		return false
	}
	if f.HTTPStatus != other.HTTPStatus {
		return false
	}
	if f.Error != other.Error {
		return false
	}
	if f.Stale != other.Stale {
		return false
	}
	return true
}

// secretAssignRe matches a secret keyword in assignment style, e.g.
// "token=abc...", "password: '...'", or a long opaque bearer token.
// A bare word never matches: news prose mentions words like "secret",
// "token" and "Secretary" constantly, and substring matching on them
// rejects otherwise valid stories (it once killed the whole production
// export because a summary contained a player quote).
var secretAssignRe = regexp.MustCompile(`(?i)\b(password|passwd|secret|token|credential|api[_-]?key|private[_-]?key|auth[_-]?token|access[_-]?token|refresh[_-]?token|aws[_-]?secret|database_url|db_password)\s*[:=]\s*("[^"]{16,}"|'[^']{16,}'|[A-Za-z0-9_\-]{16,})`)

// secretConnRe matches a database connection URL assigned to a DB key
// ("database_url=postgres://..." or "dsn=mysql://..."): secretAssignRe's
// value character class does not cover the "://" in the URL scheme.
var secretConnRe = regexp.MustCompile(`(?i)\b\w*(database_url|db_url|db_dsn|dsn|connection_string)\s*[:=]\s*\w+://`)

// bearerRe matches "Bearer <long opaque value>" (Authorization header leaks).
var bearerRe = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._\-]{16,}`)

// isSecretPattern checks for secret-looking content (case-insensitive).
// Defense in depth: flags credential values in assignment form, while
// ordinary English use of the keywords stays clean.
func isSecretPattern(s string) bool {
	if s == "" {
		return false
	}
	return secretAssignRe.MatchString(s) || bearerRe.MatchString(s) || secretConnRe.MatchString(s)
}

// IngestionRecorder records ingestion run results for historical analysis.
type IngestionRecorder interface {
	Record(ctx context.Context, record RunRecord) error
}

// DefaultIngestionRecorder is a no-op recorder for offline/dev.
type DefaultIngestionRecorder struct{}

func (r *DefaultIngestionRecorder) Record(ctx context.Context, record RunRecord) error {
	// No-op: in production with a DB, this would write ingestion_runs and source_fetches
	return nil
}
