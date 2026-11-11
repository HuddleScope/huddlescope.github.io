package domain

import (
	"testing"
	"time"
)

func TestComputeStorySlugFromHeadline(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "untitled"},
		{"  ", "untitled"},
		{"  Test   Story Title  ", "test-story-title"},
		{"NFL Playoffs: Divisional Round Preview", "nfl-playoffs-divisional-round-preview"},
		{"Team Names with Uppercase: Cowboys vs. Eagles", "team-names-with-uppercase-cowboys-vs-eagles"},
		{"Special!@#$Characters & Symbols", "specialcharacters-symbols"},
		{"Multiple---Hyphens---Here", "multiple-hyphens-here"},
		{"Trailing Hyphen-", "trailing-hyphen"},
	}

	for _, tt := range tests {
		got := ComputeStorySlugFromHeadline(tt.input)
		if got != tt.expected {
			t.Errorf("ComputeStorySlugFromHeadline(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestStorySlugStableWhenAddingLaterSource(t *testing.T) {
	earlyArt := Article{
		ID:          "early-1",
		SourceID:    "espn-nfl",
		Title:       "Same Headline",
		Summary:     "Early article",
		URL:         "https://espn.com/early",
		PublishedAt: "2024-01-01T10:00:00Z",
		FirstSeenAt: "2024-01-01T09:00:00Z",
		Entities:    []string{"nfl"},
	}

	lateArt := Article{
		ID:          "late-1",
		SourceID:    "nfl-com",
		Title:       "Same Headline",
		Summary:     "Late article",
		URL:         "https://nfl.com/late",
		PublishedAt: "2024-01-01T12:00:00Z",
		FirstSeenAt: "2024-01-01T09:00:00Z",
		Entities:    []string{"nfl"},
	}

	group1 := NewStoryGroup(earlyArt)
	group1.AddArticle(lateArt)

	group2 := NewStoryGroup(lateArt)
	group2.AddArticle(earlyArt)

	resolver := NewStaticSourceResolver(map[string]string{
		"espn-nfl": "ESPN",
		"nfl-com":  "NFL.com",
	})

	page1, err := BuildStoryDetailPage(group1, resolver, time.Now())
	if err != nil {
		t.Fatalf("BuildStoryDetailPage(group1) failed: %v", err)
	}

	page2, err := BuildStoryDetailPage(group2, resolver, time.Now())
	if err != nil {
		t.Fatalf("BuildStoryDetailPage(group2) failed: %v", err)
	}

	if page1.Slug != page2.Slug {
		t.Errorf("Slug not stable: group1=%q, group2=%q", page1.Slug, page2.Slug)
	}

	if page1.Headline != "Same Headline" {
		t.Errorf("Headline incorrect: got %q", page1.Headline)
	}

	expectedFirst := earlyArt.ParsePublishedAtOrZero()
	if !page1.FirstPublishedAt.Equal(expectedFirst) {
		t.Errorf("FirstPublishedAt wrong: got %v, want %v", page1.FirstPublishedAt, expectedFirst)
	}

	expectedLast := lateArt.ParsePublishedAtOrZero()
	if !page1.LastPublishedAt.Equal(expectedLast) {
		t.Errorf("LastPublishedAt wrong: got %v, want %v", page1.LastPublishedAt, expectedLast)
	}
}

func TestBuildStoryDetailPageListsAll(t *testing.T) {
	articles := []Article{
		{
			ID:          "src1-art1",
			SourceID:    "espn-nfl",
			Title:       "Article 1",
			Summary:     "Summary 1",
			URL:         "https://espn.com/1",
			PublishedAt: "2024-01-01T10:00:00Z",
			FirstSeenAt: "2024-01-01T09:00:00Z",
		},
		{
			ID:          "src2-art1",
			SourceID:    "nfl-com",
			Title:       "Article 2",
			Summary:     "Summary 2",
			URL:         "https://nfl.com/2",
			PublishedAt: "2024-01-01T11:00:00Z",
			FirstSeenAt: "2024-01-01T09:00:00Z",
		},
		{
			ID:          "src3-art1",
			SourceID:    "cbs-nfl",
			Title:       "Article 3",
			Summary:     "Summary 3",
			URL:         "https://cbs.com/3",
			PublishedAt: "2024-01-01T12:00:00Z",
			FirstSeenAt: "2024-01-01T09:00:00Z",
		},
	}

	resolver := NewStaticSourceResolver(map[string]string{
		"espn-nfl": "ESPN",
		"nfl-com":  "NFL.com",
		"cbs-nfl":  "CBS Sports",
	})

	group := NewStoryGroup(articles[0])
	group.AddArticle(articles[1])
	group.AddArticle(articles[2])

	page, err := BuildStoryDetailPage(group, resolver, time.Now())
	if err != nil {
		t.Fatalf("BuildStoryDetailPage failed: %v", err)
	}

	if len(page.Articles) != 3 {
		t.Errorf("Expected 3 articles, got %d", len(page.Articles))
	}

	sourceNames := make(map[string]bool)
	for _, art := range page.Articles {
		sourceNames[art.SourceName] = true
	}
	expectedNames := map[string]bool{
		"ESPN":       true,
		"NFL.com":    true,
		"CBS Sports": true,
	}
	if !equalMaps(sourceNames, expectedNames) {
		t.Errorf("Source names mismatch: got %v, want %v", sourceNames, expectedNames)
	}
}

func TestBuildStoryDetailPageOmitsEmptyURL(t *testing.T) {
	articles := []Article{
		{
			ID:          "valid-1",
			SourceID:    "espn-nfl",
			Title:       "Valid Article",
			Summary:     "Summary",
			URL:         "https://espn.com/1",
			PublishedAt: "2024-01-01T10:00:00Z",
			FirstSeenAt: "2024-01-01T09:00:00Z",
		},
		{
			ID:          "invalid-1",
			SourceID:    "nfl-com",
			Title:       "Invalid Article (no URL)",
			Summary:     "No URL",
			URL:         "",
			PublishedAt: "2024-01-01T11:00:00Z",
			FirstSeenAt: "2024-01-01T09:00:00Z",
		},
	}

	resolver := NewStaticSourceResolver(map[string]string{
		"espn-nfl": "ESPN",
		"nfl-com":  "NFL.com",
	})

	group := NewStoryGroup(articles[0])
	group.AddArticle(articles[1])

	page, err := BuildStoryDetailPage(group, resolver, time.Now())
	if err != nil {
		t.Fatalf("BuildStoryDetailPage failed: %v", err)
	}

	if len(page.Articles) != 1 {
		t.Errorf("Expected 1 article after filtering, got %d", len(page.Articles))
	}

	if page.Articles[0].URL != "https://espn.com/1" {
		t.Errorf("Expected valid article URL, got %q", page.Articles[0].URL)
	}
}

func TestBuildEmptyGroupErrors(t *testing.T) {
	resolver := NewStaticSourceResolver(map[string]string{})

	group := StoryGroup{
		ID:        "empty-group",
		Headline:  "Empty Group",
		Articles:  []Article{},
		CreatedAt: time.Now(),
	}

	_, err := BuildStoryDetailPage(group, resolver, time.Now())
	if err == nil {
		t.Error("Expected error for empty group, got nil")
	}
}

func TestBuildAllInvalidErrors(t *testing.T) {
	articles := []Article{
		{
			ID:          "no-url-1",
			SourceID:    "espn-nfl",
			Title:       "No URL 1",
			Summary:     "No URL",
			URL:         "",
			PublishedAt: "2024-01-01T10:00:00Z",
			FirstSeenAt: "2024-01-01T09:00:00Z",
		},
		{
			ID:          "no-url-2",
			SourceID:    "nfl-com",
			Title:       "No URL 2",
			Summary:     "No URL",
			URL:         "",
			PublishedAt: "2024-01-01T11:00:00Z",
			FirstSeenAt: "2024-01-01T09:00:00Z",
		},
	}

	resolver := NewStaticSourceResolver(map[string]string{})

	group := StoryGroup{
		ID:        "invalid-group",
		Headline:  "Invalid Group",
		Articles:  articles,
		CreatedAt: time.Now(),
	}

	_, err := BuildStoryDetailPage(group, resolver, time.Now())
	if err == nil {
		t.Error("Expected error when all URLs invalid, got nil")
	}
}

func TestBuildIdempotent(t *testing.T) {
	articles := []Article{
		{
			ID:          "art-1",
			SourceID:    "espn-nfl",
			Title:       "Idempotent Test",
			Summary:     "Summary",
			URL:         "https://espn.com/1",
			PublishedAt: "2024-01-01T10:00:00Z",
			FirstSeenAt: "2024-01-01T09:00:00Z",
		},
	}

	resolver := NewStaticSourceResolver(map[string]string{
		"espn-nfl": "ESPN",
	})

	group := NewStoryGroup(articles[0])

	page1, err1 := BuildStoryDetailPage(group, resolver, time.Now())
	page2, err2 := BuildStoryDetailPage(group, resolver, time.Now())

	if err1 != nil || err2 != nil {
		t.Fatalf("Build failed: err1=%v, err2=%v", err1, err2)
	}

	if page1.Slug != page2.Slug {
		t.Errorf("Slug not idempotent: page1=%q, page2=%q", page1.Slug, page2.Slug)
	}

	if page1.Path != page2.Path {
		t.Errorf("Path not idempotent: page1=%q, page2=%q", page1.Path, page2.Path)
	}

	if page1.Headline != page2.Headline {
		t.Errorf("Headline not idempotent: page1=%q, page2=%q", page1.Headline, page2.Headline)
	}

	if len(page1.Articles) != len(page2.Articles) {
		t.Errorf("Article count differs: page1=%d, page2=%d", len(page1.Articles), len(page2.Articles))
	}
}

func TestPickRepresentative(t *testing.T) {
	t1 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC)
	t3 := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	articles := []Article{
		{
			ID:          "late",
			SourceID:    "espn",
			Title:       "Late Article",
			URL:         "https://espn.com/late",
			PublishedAt: t2.Format(time.RFC3339),
		},
		{
			ID:          "early",
			SourceID:    "nfl",
			Title:       "Early Article",
			URL:         "https://nfl.com/early",
			PublishedAt: t1.Format(time.RFC3339),
		},
		{
			ID:          "mid",
			SourceID:    "cbs",
			Title:       "Mid Article",
			URL:         "https://cbs.com/mid",
			PublishedAt: t3.Format(time.RFC3339),
		},
	}

	rep := pickRepresentative(articles)

	if rep.ID != "early" {
		t.Errorf("Expected early article as representative, got %q", rep.ID)
	}

	if rep.URL != "https://nfl.com/early" {
		t.Errorf("Expected early article URL, got %q", rep.URL)
	}
}

func TestPickRepresentativeTieBreaker(t *testing.T) {
	t1 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)

	articles := []Article{
		{
			ID:          "b-url",
			SourceID:    "espn",
			Title:       "Article",
			URL:         "https://espn.com/b",
			PublishedAt: t1.Format(time.RFC3339),
		},
		{
			ID:          "a-url",
			SourceID:    "nfl",
			Title:       "Article",
			URL:         "https://nfl.com/a",
			PublishedAt: t1.Format(time.RFC3339),
		},
	}

	rep := pickRepresentative(articles)

	if rep.URL != "https://espn.com/b" {
		t.Errorf("Expected tie-breaker to pick smallest URL, got %q", rep.URL)
	}
}

func TestStaticSourceResolver(t *testing.T) {
	resolver := NewStaticSourceResolver(map[string]string{
		"espn-nfl": "ESPN",
		"nfl-com":  "NFL.com",
	})

	if resolver.Resolve("espn-nfl") != "ESPN" {
		t.Errorf("Failed to resolve espn-nfl")
	}

	if resolver.Resolve("unknown") != "unknown" {
		t.Errorf("Expected unknown source to return source ID")
	}
}

func equalMaps(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}
