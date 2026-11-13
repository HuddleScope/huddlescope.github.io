package domain

import (
	"fmt"
	"testing"
	"time"
)

func TestFeedSource_Validate(t *testing.T) {
	tests := []struct {
		name   string
		source FeedSource
		valid  bool
	}{
		{"valid", FeedSource{ID: "test", URL: "https://example.com", Priority: 0, Enabled: true}, true},
		{"valid with timeout", FeedSource{ID: "test", URL: "https://example.com", Priority: 1, Timeout: 60 * time.Second}, true},
		{"empty id", FeedSource{URL: "https://example.com"}, false},
		{"empty url", FeedSource{ID: "test"}, false},
		{"invalid url scheme", FeedSource{ID: "test", URL: "ftp://example.com"}, false},
		{"negative timeout", FeedSource{ID: "test", URL: "https://example.com", Timeout: -1}, false},
		{"negative priority", FeedSource{ID: "test", URL: "https://example.com", Priority: -1}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.source.Validate()
			if tt.valid && err != nil {
				t.Errorf("Validate() error = %v; want nil", err)
			}
			if !tt.valid && err == nil {
				t.Errorf("Validate() error = nil; want error")
			}
		})
	}
}

func TestFeedSource_ValidateDuplicateID(t *testing.T) {
	sources := []FeedSource{
		{ID: "dup", URL: "https://a.com", Priority: 0, Enabled: true},
		{ID: "dup", URL: "https://b.com", Priority: 1, Enabled: true},
	}

	_, err := NewFeedRegistry(sources...)
	if err == nil {
		t.Fatalf("NewFeedRegistry() error = nil; want duplicate ID error")
	}
	if !contains(err.Error(), "duplicate") {
		t.Errorf("NewFeedRegistry() error = %v; want 'duplicate'", err)
	}
}

func TestFeedSource_ValidateAllInvalid(t *testing.T) {
	sources := []FeedSource{
		{URL: "https://a.com", Priority: 0, Enabled: true}, // empty ID
		{ID: "b", Priority: 1, Enabled: true},              // empty URL
	}

	_, err := NewFeedRegistry(sources...)
	if err == nil {
		t.Fatalf("NewFeedRegistry() error = nil; want errors")
	}
	errStr := err.Error()
	// First validation error should be for empty ID
	if !contains(errStr, "ID must not be empty") {
		t.Errorf("NewFeedRegistry() error = %v; want 'ID must not be empty'", err)
	}
}

func TestFeedRegistry_All(t *testing.T) {
	sources := []FeedSource{
		{ID: "c", URL: "https://c.com", Priority: 2, Enabled: true},
		{ID: "a", URL: "https://a.com", Priority: 1, Enabled: true},
		{ID: "b", URL: "https://b.com", Priority: 1, Enabled: true}, // same priority as a
	}

	registry, err := NewFeedRegistry(sources...)
	if err != nil {
		t.Fatalf("NewFeedRegistry() error = %v", err)
	}

	all := registry.All()
	if len(all) != 3 {
		t.Fatalf("All() length = %d; want 3", len(all))
	}

	// Check sorted by priority, then ID
	if all[0].ID != "a" {
		t.Errorf("All()[0].ID = %s; want a (lowest priority)", all[0].ID)
	}
	if all[1].ID != "b" {
		t.Errorf("All()[1].ID = %s; want b (same priority, later ID)", all[1].ID)
	}
	if all[2].ID != "c" {
		t.Errorf("All()[2].ID = %s; want c (highest priority)", all[2].ID)
	}

	// Verify copies (mutation doesn't affect registry)
	all[0].ID = "modified"
	all2 := registry.All()
	if all2[0].ID == "modified" {
		t.Errorf("All() returned alias, not copy; registry sources modified")
	}
}

func TestFeedRegistry_Enabled(t *testing.T) {
	sources := []FeedSource{
		{ID: "a", URL: "https://a.com", Priority: 2, Enabled: false},
		{ID: "b", URL: "https://b.com", Priority: 1, Enabled: true},
		{ID: "c", URL: "https://c.com", Priority: 3, Enabled: true},
		{ID: "d", URL: "https://d.com", Priority: 0, Enabled: false},
	}

	registry, err := NewFeedRegistry(sources...)
	if err != nil {
		t.Fatalf("NewFeedRegistry() error = %v", err)
	}

	enabled := registry.Enabled()
	if len(enabled) != 2 {
		t.Fatalf("Enabled() length = %d; want 2", len(enabled))
	}

	// Should be sorted by priority (ascending)
	if enabled[0].ID != "b" {
		t.Errorf("Enabled()[0].ID = %s; want b (priority 1)", enabled[0].ID)
	}
	if enabled[1].ID != "c" {
		t.Errorf("Enabled()[1].ID = %s; want c (priority 3)", enabled[1].ID)
	}
}

func TestFeedRegistry_Get(t *testing.T) {
	sources := []FeedSource{
		{ID: "x", URL: "https://x.com", Priority: 0, Enabled: true},
	}

	registry, err := NewFeedRegistry(sources...)
	if err != nil {
		t.Fatalf("NewFeedRegistry() error = %v", err)
	}

	src, ok := registry.Get("x")
	if !ok {
		t.Fatalf("Get(\"x\") ok = false; want true")
	}
	if src.ID != "x" {
		t.Errorf("Get(\"x\").ID = %s; want x", src.ID)
	}

	_, ok = registry.Get("missing")
	if ok {
		t.Errorf("Get(\"missing\") ok = true; want false")
	}
}

func TestFeedRegistry_RecordStatus(t *testing.T) {
	sources := []FeedSource{
		{ID: "test", URL: "https://test.com", Priority: 0, Enabled: true},
	}

	registry, err := NewFeedRegistry(sources...)
	if err != nil {
		t.Fatalf("NewFeedRegistry() error = %v", err)
	}

	// Record error
	errExpected := fmt.Errorf("connection refused")
	registry.RecordStatus("test", false, errExpected)

	status := registry.Status("test")
	if status.LastError == nil || status.LastError.Error() != errExpected.Error() {
		t.Errorf("Status().LastError = %v; want %v", status.LastError, errExpected)
	}
	if status.LastOK {
		t.Errorf("Status().LastOK = true; want false")
	}
	if status.LastAttemptAt == nil {
		t.Error("Status().LastAttemptAt = nil; want non-nil")
	}

	// Record success
	registry.RecordStatus("test", true, nil)
	status2 := registry.Status("test")
	if !status2.LastOK {
		t.Errorf("Status().LastOK = false; want true")
	}
	if status2.LastError != nil {
		t.Errorf("Status().LastError = %v; want nil", status2.LastError)
	}
}

func TestFeedRegistry_Count(t *testing.T) {
	sources := []FeedSource{
		{ID: "a", URL: "https://a.com", Priority: 0, Enabled: true},
		{ID: "b", URL: "https://b.com", Priority: 1, Enabled: false},
	}

	registry, err := NewFeedRegistry(sources...)
	if err != nil {
		t.Fatalf("NewFeedRegistry() error = %v", err)
	}

	if registry.Count() != 2 {
		t.Errorf("Count() = %d; want 2", registry.Count())
	}
	if registry.CountEnabled() != 1 {
		t.Errorf("CountEnabled() = %d; want 1", registry.CountEnabled())
	}
}

func TestFeedRegistry_Validate(t *testing.T) {
	registry, err := NewFeedRegistry(
		FeedSource{ID: "a", URL: "https://a.com", Priority: 0, Enabled: true},
	)
	if err != nil {
		t.Fatalf("NewFeedRegistry() error = %v", err)
	}

	if err := registry.Validate(); err != nil {
		t.Errorf("Validate() error = %v; want nil", err)
	}
}

func TestAcceptableUse_String(t *testing.T) {
	tests := []struct {
		au       AcceptableUse
		expected string
	}{
		{AcceptanceUnknown, "unknown"},
		{AcceptanceAllowed, "allowed"},
		{AcceptanceRestricted, "restricted"},
	}

	for _, tt := range tests {
		if tt.au.String() != tt.expected {
			t.Errorf("AcceptableUse(%d).String() = %s; want %s", tt.au, tt.au.String(), tt.expected)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestUpsertStatus_String(t *testing.T) {
	tests := []struct {
		s        UpsertStatus
		expected string
	}{
		{UpsertCreated, "created"},
		{UpsertUpdated, "updated"},
		{UpsertUnchanged, "unchanged"},
	}

	for _, tt := range tests {
		if tt.s.String() != tt.expected {
			t.Errorf("UpsertStatus(%d).String() = %s; want %s", tt.s, tt.s.String(), tt.expected)
		}
	}
}

func TestDedupeKey(t *testing.T) {
	tests := []struct {
		name     string
		sourceID string
		guid     string
		url      string
		expected string
	}{
		{"guid present", "feed-1", "guid-123", "https://example.com/article", "feed-1|guid-123"},
		{"guid with whitespace", "feed-1", "  guid-456  ", "https://example.com/article", "feed-1|guid-456"},
		{"url fallback", "feed-2", "", "https://example.com/article", "feed-2|https://example.com/article"},
		{"url with trailing slash", "feed-3", "", "https://example.com/article/", "feed-3|https://example.com/article/"},
		{"source ID normalized", "FEED_1", "guid-789", "https://example.com", "feed-1|guid-789"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DedupeKey(tt.sourceID, tt.guid, tt.url)
			if got != tt.expected {
				t.Errorf("DedupeKey(%q, %q, %q) = %q; want %q", tt.sourceID, tt.guid, tt.url, got, tt.expected)
			}
		})
	}
}

func TestStory_MeaningfulFieldsChanged(t *testing.T) {
	base := Story{
		ID:          "id-1",
		Title:       "Original Title",
		Source:      "source-1",
		PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Summary:     "Original summary",
		URL:         "https://example.com/original",
		GUID:        "guid-1",
		Entities:    []string{"nfl"}, // add entities for comparison tests
		FirstSeenAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	tests := []struct {
		name     string
		incoming Story
		want     bool
	}{
		{"unchanged", base, false},
		{"title changed", Story{Title: "New Title", Source: base.Source, PublishedAt: base.PublishedAt, Summary: base.Summary, URL: base.URL, GUID: base.GUID, FirstSeenAt: base.FirstSeenAt, UpdatedAt: base.UpdatedAt}, true},
		{"summary changed", Story{Summary: "New Summary", Title: base.Title, Source: base.Source, PublishedAt: base.PublishedAt, URL: base.URL, GUID: base.GUID, FirstSeenAt: base.FirstSeenAt, UpdatedAt: base.UpdatedAt}, true},
		{"url changed", Story{URL: "https://example.com/new", CanonicalURL: "https://example.com/new", Title: base.Title, Source: base.Source, PublishedAt: base.PublishedAt, Summary: base.Summary, GUID: base.GUID, FirstSeenAt: base.FirstSeenAt, UpdatedAt: base.UpdatedAt}, true},
		{"publishedAt changed", Story{PublishedAt: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC), Title: base.Title, Source: base.Source, Summary: base.Summary, URL: base.URL, GUID: base.GUID, FirstSeenAt: base.FirstSeenAt, UpdatedAt: base.UpdatedAt}, true},
		{"guid changed", Story{GUID: "guid-2", Title: base.Title, Source: base.Source, PublishedAt: base.PublishedAt, Summary: base.Summary, URL: base.URL, FirstSeenAt: base.FirstSeenAt, UpdatedAt: base.UpdatedAt}, true},
		{"firstSeenAt changed (ignored)", Story{FirstSeenAt: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), Title: base.Title, Source: base.Source, PublishedAt: base.PublishedAt, Summary: base.Summary, URL: base.URL, GUID: base.GUID, Entities: base.Entities, UpdatedAt: base.UpdatedAt}, false},
		{"one new entity", Story{Entities: []string{"nfl", "nba"}, Title: base.Title, Source: base.Source, PublishedAt: base.PublishedAt, Summary: base.Summary, URL: base.URL, GUID: base.GUID, FirstSeenAt: base.FirstSeenAt, UpdatedAt: base.UpdatedAt}, true},
		{"entities same order", Story{Entities: []string{"nfl"}, Title: base.Title, Source: base.Source, PublishedAt: base.PublishedAt, Summary: base.Summary, URL: base.URL, GUID: base.GUID, FirstSeenAt: base.FirstSeenAt, UpdatedAt: base.UpdatedAt}, false},
		{"entities different order", Story{Entities: []string{"nfl"}, Title: base.Title, Source: base.Source, PublishedAt: base.PublishedAt, Summary: base.Summary, URL: base.URL, GUID: base.GUID, FirstSeenAt: base.FirstSeenAt, UpdatedAt: base.UpdatedAt}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := base.MeaningfulFieldsChanged(tt.incoming)
			if got != tt.want {
				t.Errorf("MeaningfulFieldsChanged() = %v; want %v", got, tt.want)
			}
		})
	}
}

func Test_sliceEqual(t *testing.T) {
	tests := []struct {
		name string
		a    []string
		b    []string
		want bool
	}{
		{"empty", []string{}, []string{}, true},
		{"both nil", nil, nil, true},
		{"one nil", nil, []string{"a"}, false},
		{"length mismatch", []string{"a"}, []string{"a", "b"}, false},
		{"same content same order", []string{"a", "b"}, []string{"a", "b"}, true},
		{"same content different order", []string{"a", "b"}, []string{"b", "a"}, true},
		{"different content", []string{"a"}, []string{"b"}, false},
		{"duplicate in one", []string{"a", "a"}, []string{"a"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sliceEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("sliceEqual(%v, %v) = %v; want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
