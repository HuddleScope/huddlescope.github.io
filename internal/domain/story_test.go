package domain

import (
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
