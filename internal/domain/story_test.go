package domain

import (
	"testing"
	"time"
)

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  Hello   World  ", "hello world"},
		{"  multiple   spaces   here  ", "multiple spaces here"},
		{"UPPERCASE", "uppercase"},
		{"Mixed Case", "mixed case"},
		{"\tTabs\nand Newlines", "tabs and newlines"},
		{"", ""},
		{"single", "single"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeTitle(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeTitle(%q) = %q; want %q", tt.input, got, tt.expected)
			}
		})
	}
}

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
	// Same inputs should always produce the same ID
	title := "Same Title"
	source := "Same Source"
	published := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)

	id1 := ComputeStoryID(title, source, published)
	id2 := ComputeStoryID(title, source, published)

	if id1 != id2 {
		t.Errorf("ComputeStoryID not idempotent: %q != %q", id1, id2)
	}
}

func TestCanonicalize(t *testing.T) {
	fe := FeedEntry{
		Title:       "Test Entry",
		Source:      "test-source",
		PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Summary:     "Test summary",
		URL:         "https://example.com/test",
	}

	story := Story{}.Canonicalize(fe)

	if story.ID == "" {
		t.Error("Story ID should not be empty")
	}
	if story.Title != fe.Title {
		t.Errorf("Title mismatch: got %q, want %q", story.Title, fe.Title)
	}
	if story.Source != fe.Source {
		t.Errorf("Source mismatch: got %q, want %q", story.Source, fe.Source)
	}
	if !story.PublishedAt.Equal(fe.PublishedAt) {
		t.Error("PublishedAt mismatch")
	}
	if story.Summary != fe.Summary {
		t.Errorf("Summary mismatch: got %q, want %q", story.Summary, fe.Summary)
	}
	if story.FirstSeenAt.IsZero() {
		t.Error("FirstSeenAt should be set")
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
