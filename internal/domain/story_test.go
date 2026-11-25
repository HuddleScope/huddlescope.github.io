package domain

import (
	"strings"
	"testing"
	"time"
)

func TestComputeStoryID(t *testing.T) {
	title := "  Test   Story Title  "
	source := "TEST-SOURCE"
	published := time.Date(2024, 1, 15, 12, 30, 0, 0, time.UTC)

	expectedID := "test story title|test-source|2024-01-15T12:30:00Z"

	got := ComputeStoryID(title, source, published)
	if got != expectedID {
		t.Errorf("ComputeStoryID() = %q; want %q", got, expectedID)
	}
}

func TestComputeStoryIDIdempotent(t *testing.T) {
	title := "Same Title"
	source := "Same Source"
	published := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)

	id1 := ComputeStoryID(title, source, published)
	id2 := ComputeStoryID(title, source, published)

	if id1 != id2 {
		t.Errorf("ComputeStoryID not idempotent: %q != %q", id1, id2)
	}
}

func TestComputeStoryIDUnicodeNormalization(t *testing.T) {
	// Pre-composed é (U+00E9)
	precomposed := "Caf\u00E9 Story"
	// Decomposed e + combining acute accent (U+0301)
	decomposed := "Cafe\u0301 Story"
	source := "test-source"
	published := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)

	idPrecomposed := ComputeStoryID(precomposed, source, published)
	idDecomposed := ComputeStoryID(decomposed, source, published)

	if idPrecomposed != idDecomposed {
		t.Errorf("ComputeStoryID not Unicode-normalized: pre-composed %q != decomposed %q", idPrecomposed, idDecomposed)
	}
}

func TestIsEqual(t *testing.T) {
	id := "test-id"
	s1 := Story{ID: id}
	s2 := Story{ID: id}
	s3 := Story{ID: "other-id"}

	if !s1.IsEqual(s2) {
		t.Error("Stories with same ID should be equal")
	}
	if s1.IsEqual(s3) {
		t.Error("Stories with different IDs should not be equal")
	}
}

func TestFeedEntryGUID(t *testing.T) {
	fe := FeedEntry{
		Title:       "Test Entry",
		Source:      "test-source",
		PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Summary:     "Test summary",
		URL:         "https://example.com/test",
		GUID:        "guid-123",
	}

	if fe.GUID != "guid-123" {
		t.Errorf("FeedEntry.GUID: got %q, want %q", fe.GUID, "guid-123")
	}
}

func TestStoryURL(t *testing.T) {
	s := Story{
		ID:          "test-id",
		Title:       "Test Title",
		Source:      "test-source",
		PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Summary:     "Test summary",
		URL:         "https://example.com/test",
		FirstSeenAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	if s.URL != "https://example.com/test" {
		t.Errorf("Story.URL: got %q, want %q", s.URL, "https://example.com/test")
	}
}

func TestStoryCanonicalizeIntegration(t *testing.T) {
	fe := FeedEntry{
		Title:       "  Test   Entry  ",
		Source:      "  TEST SOURCE  ",
		PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Summary:     "<p>Test <b>summary</b></p>",
		URL:         "https://example.com/test",
	}

	story := Story{}.Canonicalize(fe)

	// Verify normalization
	if story.Title != "test entry" {
		t.Errorf("Title not normalized: got %q", story.Title)
	}
	if story.Source != "test-source" {
		t.Errorf("Source not normalized: got %q", story.Source)
	}
	if !story.PublishedAt.Equal(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PublishedAt not normalized: got %v", story.PublishedAt)
	}
	if story.Summary != "Test summary" {
		t.Errorf("Summary not sanitized: got %q", story.Summary)
	}
	if story.URL != "https://example.com/test" {
		t.Errorf("URL not preserved: got %q", story.URL)
	}
}

func TestStoryCanonicalizeIdempotent(t *testing.T) {
	fe := FeedEntry{
		Title:       "Test Title",
		Source:      "test-source",
		PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Summary:     "Test summary",
		URL:         "https://example.com/test",
	}

	story1 := Story{}.Canonicalize(fe)

	if story1.ID != ComputeStoryID(fe.Title, fe.Source, fe.PublishedAt) {
		t.Errorf("Canonicalize ID mismatch: %q != %q", story1.ID, ComputeStoryID(fe.Title, fe.Source, fe.PublishedAt))
	}
	if story1.URL != CanonicalURLEntry(fe.URL) {
		t.Errorf("Canonicalize URL mismatch: %q != %q", story1.URL, CanonicalURLEntry(fe.URL))
	}
}

// TestStory_JSONRoundTrip_DisplayTitle (#201): a Story with a DisplayTitle set
// must round-trip through JSON while preserving the value, and the JSON output
// must include the displayTitle key.
func TestStory_JSONRoundTrip_DisplayTitle(t *testing.T) {
	s := Story{
		ID:           "id-1",
		Title:        "chiefs sign rb james cook",
		DisplayTitle: "Chiefs sign RB James Cook",
		Source:       "official-min",
		Summary:      "summary",
		URL:          "https://example.com/chiefs-cook",
	}

	data, err := s.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if !strings.Contains(string(data), `"displayTitle"`) {
		t.Errorf("JSON does not include displayTitle key: %s", string(data))
	}

	var decoded Story
	if err := decoded.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if decoded.DisplayTitle != "Chiefs sign RB James Cook" {
		t.Errorf("round-trip DisplayTitle = %q; want %q", decoded.DisplayTitle, "Chiefs sign RB James Cook")
	}
	if decoded.Title != "chiefs sign rb james cook" {
		t.Errorf("round-trip Title = %q; want %q", decoded.Title, "chiefs sign rb james cook")
	}
}

// TestStory_UnmarshalJSON_LegacyNoDisplayTitle (#201, AC-3): a legacy story_data
// JSON payload without the displayTitle key must decode to an empty DisplayTitle
// so the repository can fall back to the normalized Title/Headline.
func TestStory_UnmarshalJSON_LegacyNoDisplayTitle(t *testing.T) {
	legacy := `{
		"id": "id-1",
		"title": "chiefs sign rb james cook",
		"headline": "chiefs sign rb james cook",
		"source": "official-min",
		"publishedAt": "2026-01-01T10:00:00Z",
		"firstSeenAt": "2026-01-01T09:00:00Z",
		"updatedAt": "2026-01-01T10:00:00Z",
		"url": "https://example.com/chiefs-cook",
		"summary": "summary"
	}`
	var s Story
	if err := s.UnmarshalJSON([]byte(legacy)); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if s.DisplayTitle != "" {
		t.Errorf("legacy story DisplayTitle = %q; want empty", s.DisplayTitle)
	}
	// Display() falls back to the normalized Title.
	if s.Display() != "chiefs sign rb james cook" {
		t.Errorf("Display() = %q; want normalized title fallback", s.Display())
	}
}

// TestStory_JSONRoundTrip_DisplayTitleEmpty (#201): a Story with an empty
// DisplayTitle (legacy shape) still round-trips; the displayTitle key is present
// (consistent with the other always-present fields) but empty, and the decoded
// value is empty so the repository falls back to Title.
func TestStory_JSONRoundTrip_DisplayTitleEmpty(t *testing.T) {
	s := Story{ID: "id-1", Title: "normalized title"}
	data, err := s.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var decoded Story
	if err := decoded.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if decoded.DisplayTitle != "" {
		t.Errorf("round-trip DisplayTitle = %q; want empty", decoded.DisplayTitle)
	}
	// Display() falls back to the normalized title.
	if decoded.Display() != "normalized title" {
		t.Errorf("Display() = %q; want normalized title fallback", decoded.Display())
	}
}
