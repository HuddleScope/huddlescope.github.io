package domain

import (
	"strings"
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
		{"  Leading and trailing   ", "leading and trailing"},
		{"Tab\there", "tab here"},
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

func TestNormalizeTitleIdempotent(t *testing.T) {
	title := "  Test   Title  "
	normalized := NormalizeTitle(title)
	normalized2 := NormalizeTitle(normalized)
	if normalized != normalized2 {
		t.Errorf("NormalizeTitle not idempotent: %q != %q", normalized, normalized2)
	}
}

func TestNormalizeSourceID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  test-source  ", "test-source"},
		{"TEST_SOURCE", "test-source"},
		{"test source", "test-source"},
		{"Test Source ID", "test-source-id"},
		{"Mixed_Case Source", "mixed-case-source"},
		{"", ""},
		{"  ", ""},
		{"single", "single"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeSourceID(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeSourceID(%q) = %q; want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNormalizeTimestamp(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Time
		expected time.Time
	}{
		{"UTC time", time.Date(2024, 1, 15, 12, 30, 0, 0, time.UTC), time.Date(2024, 1, 15, 12, 30, 0, 0, time.UTC)},
		{"Non-UTC time", time.Date(2024, 1, 15, 12, 30, 0, 0, time.FixedZone("EST", -5*3600)), time.Date(2024, 1, 15, 17, 30, 0, 0, time.UTC)},
		{"Zero time", time.Time{}, time.Time{}},
		{"Year 0", time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{}},
		{"Year 10000", time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{}},
		{"Year 9999", time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC), time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeTimestamp(tt.input)
			if !got.Equal(tt.expected) {
				t.Errorf("NormalizeTimestamp(%v) = %v; want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSanitizeHTML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxBytes int
		expected string
	}{
		{"plain text", "Hello world", 1024, "Hello world"},
		{"simple HTML", "<p>Hello <b>world</b></p>", 1024, "Hello world"},
		{"script tag", "<p>Hello</p><script>alert('xss')</script>", 1024, "Hello"},
		{"style tag", "<p>Hello</p><style>.x{color:red}</style>", 1024, "Hello"},
		{"nested tags", "<div><p>Text</p></div>", 1024, "Text"},
		{"truncate", "<p>Hello world</p>", 5, "Hello"},
		{"truncate UTF-8", "<p>Café résumé</p>", 6, "Café"},
		{"empty string", "", 1024, ""},
		{"complex HTML", `<div>
			<header><h1>Title</h1></header>
			<script>evil()</script>
			<style>.x{}</style>
			<p>Body content</p>
		</div>`, 1024, "Title Body content"},
		{"whitespace collapse", "<p>  Hello   World  </p>", 1024, "Hello World"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeHTML(tt.input, tt.maxBytes)
			if got != tt.expected {
				t.Errorf("SanitizeHTML(%q, %d) = %q; want %q", tt.input, tt.maxBytes, got, tt.expected)
			}
		})
	}
}

func TestSanitizeHTMLUseDefault(t *testing.T) {
	html := "<p>" + strings.Repeat("a", 2000) + "</p>"
	got := SanitizeHTML(html, 0)
	if len(got) > 1024 {
		t.Errorf("SanitizeHTML with maxBytes=0 should use default 1024, got %d bytes", len(got))
	}
}

func TestSanitizeHTMLIdempotent(t *testing.T) {
	html := "<p>Hello <b>world</b></p>"
	sanitized := SanitizeHTML(html, 1024)
	sanitized2 := SanitizeHTML(sanitized, 1024)
	if sanitized != sanitized2 {
		t.Errorf("SanitizeHTML not idempotent: %q != %q", sanitized, sanitized2)
	}
}

func TestCanonicalize(t *testing.T) {
	tests := []struct {
		name     string
		fe       FeedEntry
		checkURL string
	}{
		{"basic entry", FeedEntry{
			Title:       "  Test Entry  ",
			Source:      "TEST SOURCE",
			PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Summary:     "<p>Test <b>summary</b></p>",
			URL:         "https://example.com/test",
		}, "https://example.com/test"},
		{"empty URL", FeedEntry{
			Title:       "Test",
			Source:      "test-source",
			PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Summary:     "Test",
			URL:         "",
		}, ""},
		{"title normalization", FeedEntry{
			Title:       "  Test   Entry  ",
			Source:      "test-source",
			PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Summary:     "Test",
			URL:         "https://example.com/test",
		}, "https://example.com/test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			story := Story{}.Canonicalize(tt.fe)

			if story.ID == "" {
				t.Error("Story ID should not be empty")
			}
			if tt.fe.Title != "" && story.Title != NormalizeTitle(tt.fe.Title) {
				t.Errorf("Title: got %q, want normalized %q", story.Title, NormalizeTitle(tt.fe.Title))
			}
			if story.Source != NormalizeSourceID(tt.fe.Source) {
				t.Errorf("Source: got %q, want normalized %q", story.Source, NormalizeSourceID(tt.fe.Source))
			}
			expectedTime := NormalizeTimestamp(tt.fe.PublishedAt)
			if !story.PublishedAt.Equal(expectedTime) {
				t.Errorf("PublishedAt: got %v, want normalized %v", story.PublishedAt, expectedTime)
			}
			if story.Summary != SanitizeHTML(tt.fe.Summary, 1024) {
				t.Errorf("Summary: got %q, want normalized %q", story.Summary, SanitizeHTML(tt.fe.Summary, 1024))
			}
			if story.URL != tt.checkURL {
				t.Errorf("URL: got %q, want %q", story.URL, tt.checkURL)
			}
			// FirstSeenAt is set by Repository on first insertion, not by Canonicalize
			if !story.FirstSeenAt.IsZero() {
				t.Errorf("FirstSeenAt should be zero after Canonicalize (set by Repository), got %v", story.FirstSeenAt)
			}
		})
	}
}

func TestCanonicalizeIdempotent(t *testing.T) {
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
