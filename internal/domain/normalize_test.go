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

// TestDisplayNormalizeTitle (#201): the display normalization preserves case and
// punctuation while trimming, collapsing whitespace and applying NFC — unlike
// NormalizeTitle it never lowercases.
func TestDisplayNormalizeTitle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Chiefs sign RB James Cook", "Chiefs sign RB James Cook"},
		{"  Hello   World  ", "Hello World"},
		{"  multiple   spaces   here  ", "multiple spaces here"},
		{"UPPERCASE", "UPPERCASE"},
		{"Mixed Case", "Mixed Case"},
		{"\tTabs\nand Newlines", "Tabs and Newlines"},
		{"", ""},
		{"single", "single"},
		{"  Leading and trailing   ", "Leading and trailing"},
		{"Tab\there", "Tab here"},
		{"Caf\u00e9", "Caf\u00e9"},                 // pre-composed
		{"Cafe\u0301", "Caf\u00e9"},                // decomposed -> NFC composes
		{"re\u0301sume\u0301", "r\u00e9sum\u00e9"}, // decomposed -> NFC
		{"Hello, World!", "Hello, World!"},         // punctuation preserved
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := DisplayNormalizeTitle(tt.input)
			if got != tt.expected {
				t.Errorf("DisplayNormalizeTitle(%q) = %q; want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// TestDisplayNormalizeTitleKeepsCase (#201): a headline whose identity title is
// lowercase must retain its original feed casing in the display value.
func TestDisplayNormalizeTitleKeepsCase(t *testing.T) {
	feed := "  Chiefs   sign RB James Cook  "
	display := DisplayNormalizeTitle(feed)
	normalized := NormalizeTitle(feed)
	if display != "Chiefs sign RB James Cook" {
		t.Errorf("display = %q; want %q", display, "Chiefs sign RB James Cook")
	}
	if normalized != "chiefs sign rb james cook" {
		t.Errorf("normalized = %q; want %q", normalized, "chiefs sign rb james cook")
	}
	// The two must differ (display keeps case, identity lowercases)
	if display == normalized {
		t.Errorf("display and normalized unexpectedly equal: %q", display)
	}
}

// TestDisplayNormalizeTitleIdempotent: normalizing an already-normalized display
// title must be stable.
func TestDisplayNormalizeTitleIdempotent(t *testing.T) {
	title := "  Test   Title  "
	once := DisplayNormalizeTitle(title)
	twice := DisplayNormalizeTitle(once)
	if once != twice {
		t.Errorf("DisplayNormalizeTitle not idempotent: %q != %q", once, twice)
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

func TestNormalizeUnicode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Café pre-composed", "Caf\u00E9", "Caf\u00E9"},
		{"Café decomposed", "Cafe\u0301", "Caf\u00E9"},
		{"résumé pre-composed", "r\u00E9sum\u00E9", "r\u00E9sum\u00E9"},
		{"résumé decomposed", "re\u0301sume\u0301", "r\u00E9sum\u00E9"},
		{"ASCII passthrough", "hello world", "hello world"},
		{"empty string", "", ""},
		{"already NFC", "café", "café"},
		{"mixed ASCII and accented", "Cafe\u0301 résumé", "Caf\u00E9 résumé"},
		{"invalid UTF-8 passthrough", "\xff\xfe", "\xff\xfe"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeUnicode(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeUnicode(%q) = %q; want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNormalizeUnicodeIdempotent(t *testing.T) {
	// Use a decomposed input (e) + combining acute accent (U+0301)
	input := "cafe\u0301"
	normalized := NormalizeUnicode(input)
	normalized2 := NormalizeUnicode(normalized)
	if normalized != normalized2 {
		t.Errorf("NormalizeUnicode not idempotent: %q != %q", normalized, normalized2)
	}
	// Verify normalization actually changes the decomposed form
	if normalized == input {
		t.Errorf("NormalizeUnicode did not change decomposed input; NFC should have composed it")
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

// TestCanonicalizeEmptySummary (#122): a feed item without description and
// content must canonicalize with a deterministic title-derived summary so it
// never fails snapshot validation downstream.
func TestCanonicalizeEmptySummary(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		entry        FeedEntry
		wantSummary  string
		wantNonEmpty bool
	}{
		{
			"empty summary falls back to sanitized title",
			FeedEntry{
				Title:       "  Young players factor in Vikings win at Packers  ",
				Source:      "official-min",
				PublishedAt: now,
				Summary:     "",
				URL:         "https://www.vikings.com/news/young-players-factor-packers-win",
			},
			SanitizeHTML("  Young players factor in Vikings win at Packers  ", 1024),
			true,
		},
		{
			"whitespace-only summary falls back to title",
			FeedEntry{
				Title:       "Headline",
				Source:      "official-min",
				PublishedAt: now,
				Summary:     "   ",
				URL:         "https://www.vikings.com/news/headline",
			},
			"Headline",
			true,
		},
		{
			"non-empty summary is preserved as before",
			FeedEntry{
				Title:       "Headline",
				Source:      "official-min",
				PublishedAt: now,
				Summary:     "<p>Real summary</p>",
				URL:         "https://www.vikings.com/news/real",
			},
			SanitizeHTML("<p>Real summary</p>", 1024),
			true,
		},
		{
			"empty summary and empty title stays empty (title failure is a separate validation case)",
			FeedEntry{
				Title:       "",
				Source:      "official-min",
				PublishedAt: now,
				Summary:     "",
				URL:         "https://www.vikings.com/news/blank",
			},
			"",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			story := Story{}.Canonicalize(tt.entry)

			if story.Summary != tt.wantSummary {
				t.Errorf("Summary: got %q, want %q", story.Summary, tt.wantSummary)
			}
			gotNonEmpty := strings.TrimSpace(story.Summary) != ""
			if gotNonEmpty != tt.wantNonEmpty {
				t.Errorf("Summary non-empty: got %v, want %v", gotNonEmpty, tt.wantNonEmpty)
			}
			// Fallback summary must fit the 1024-byte validation bound
			if len(story.Summary) > 1024 {
				t.Errorf("Summary exceeds 1024 bytes: %d", len(story.Summary))
			}
			// Identity fields must not be affected by the fallback
			if story.ID != ComputeStoryID(tt.entry.Title, tt.entry.Source, tt.entry.PublishedAt) {
				t.Errorf("ID changed by summary fallback: %q", story.ID)
			}
			if story.DedupeKey != DedupeKey(tt.entry.Source, tt.entry.GUID, tt.entry.URL) {
				t.Errorf("DedupeKey changed by summary fallback: %q", story.DedupeKey)
			}
		})
	}
}

// TestCanonicalizeEmptySummaryDeterministic (#122): the same description-less
// entry must always canonicalize to the same summary.
func TestCanonicalizeEmptySummaryDeterministic(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	entry := FeedEntry{
		Title:       "Deterministic Headline",
		Source:      "official-min",
		PublishedAt: now,
		Summary:     "",
		URL:         "https://www.vikings.com/news/deterministic",
	}

	first := Story{}.Canonicalize(entry).Summary
	for i := 0; i < 10; i++ {
		if got := (Story{}).Canonicalize(entry).Summary; got != first {
			t.Fatalf("summary not deterministic on pass %d: %q != %q", i, got, first)
		}
	}
}
