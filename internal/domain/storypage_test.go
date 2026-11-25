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

// TestStoryPageIdentityStableAcrossDays: day-1 articles cluster into a group;
// a later-dated article from a new source is added; the slug, PageKey, and
// path are all unchanged (issue #26, AC-1, T1.1). The group ID may change
// (membership hash) but the public identity must not.
func TestStoryPageIdentityStableAcrossDays(t *testing.T) {
	day1Early := Article{
		ID:          "d1-early",
		SourceID:    "espn-nfl",
		Title:       "Cowboys Sign Veteran Safety",
		Summary:     "Day 1 early",
		URL:         "https://espn.com/d1-early",
		PublishedAt: "2024-01-01T09:00:00Z",
	}
	day1Late := Article{
		ID:          "d1-late",
		SourceID:    "nfl-com",
		Title:       "Cowboys Sign Veteran Safety",
		Summary:     "Day 1 late",
		URL:         "https://nfl.com/d1-late",
		PublishedAt: "2024-01-01T15:00:00Z",
	}
	day2 := Article{
		ID:          "d2-new",
		SourceID:    "cbs-nfl",
		Title:       "Cowboys Sign Veteran Safety",
		Summary:     "Day 2 new source",
		URL:         "https://cbs.com/d2-new",
		PublishedAt: "2024-01-02T09:00:00Z",
	}

	// Day 1: two articles cluster into one group.
	groupDay1 := NewStoryGroup(day1Early)
	groupDay1.AddArticle(day1Late)

	resolver := NewStaticSourceResolver(map[string]string{
		"espn-nfl": "ESPN",
		"nfl-com":  "NFL.com",
		"cbs-nfl":  "CBS Sports",
	})

	pageDay1, err := BuildStoryDetailPage(groupDay1, resolver, time.Date(2024, 1, 1, 23, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("BuildStoryDetailPage day1: %v", err)
	}

	// Day 2: a new source adds a later-dated article to the same story.
	groupDay2 := NewStoryGroup(day1Early)
	groupDay2.AddArticle(day1Late)
	groupDay2.AddArticle(day2)

	pageDay2, err := BuildStoryDetailPage(groupDay2, resolver, time.Date(2024, 1, 2, 23, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("BuildStoryDetailPage day2: %v", err)
	}

	// Public identity must be stable across days.
	if pageDay1.Slug != pageDay2.Slug {
		t.Errorf("Slug changed across days: %q -> %q", pageDay1.Slug, pageDay2.Slug)
	}
	if pageDay1.PageKey != pageDay2.PageKey {
		t.Errorf("PageKey changed across days: %q -> %q", pageDay1.PageKey, pageDay2.PageKey)
	}
	if pageDay1.Path != pageDay2.Path {
		t.Errorf("Path changed across days: %q -> %q", pageDay1.Path, pageDay2.Path)
	}
	// Article count advanced.
	if len(pageDay1.Articles) != 2 || len(pageDay2.Articles) != 3 {
		t.Errorf("article count day1=%d day2=%d; want 2 and 3", len(pageDay1.Articles), len(pageDay2.Articles))
	}
	// firstPublished pinned, lastPublished advanced.
	if !pageDay1.FirstPublishedAt.Equal(pageDay2.FirstPublishedAt) {
		t.Errorf("FirstPublishedAt changed: %v -> %v", pageDay1.FirstPublishedAt, pageDay2.FirstPublishedAt)
	}
	if !pageDay2.LastPublishedAt.After(pageDay1.LastPublishedAt) {
		t.Errorf("LastPublishedAt did not advance: %v -> %v", pageDay1.LastPublishedAt, pageDay2.LastPublishedAt)
	}
}

// TestBuildStoryDetailPageChronological: articles are sorted by PublishedAt
// (ties on URL) so the rendered timeline is chronological (issue #26, AC-3, D6, T3.1).
func TestBuildStoryDetailPageChronological(t *testing.T) {
	articles := []Article{
		{
			ID:          "late",
			SourceID:    "cbs",
			Title:       "Story",
			URL:         "https://cbs.com/late",
			PublishedAt: "2024-01-03T12:00:00Z",
		},
		{
			ID:          "early",
			SourceID:    "espn",
			Title:       "Story",
			URL:         "https://espn.com/early",
			PublishedAt: "2024-01-01T10:00:00Z",
		},
		{
			ID:          "mid",
			SourceID:    "nfl",
			Title:       "Story",
			URL:         "https://nfl.com/mid",
			PublishedAt: "2024-01-02T11:00:00Z",
		},
	}

	group := NewStoryGroup(articles[0])
	group.AddArticle(articles[1])
	group.AddArticle(articles[2])

	resolver := NewStaticSourceResolver(map[string]string{})
	page, err := BuildStoryDetailPage(group, resolver, time.Now())
	if err != nil {
		t.Fatalf("BuildStoryDetailPage: %v", err)
	}

	if len(page.Articles) != 3 {
		t.Fatalf("articles = %d; want 3", len(page.Articles))
	}
	// Expect chronological order: early, mid, late.
	wantURLs := []string{"https://espn.com/early", "https://nfl.com/mid", "https://cbs.com/late"}
	for i, want := range wantURLs {
		if page.Articles[i].URL != want {
			t.Errorf("articles[%d].URL = %q; want %q (chronological order violated)", i, page.Articles[i].URL, want)
		}
	}
}

// TestBuildStoryDetailPageResolverNamesEveryArticle: the resolver's display
// names are applied to every article in the listing, not only the first
// (issue #26, D5, T3.1).
func TestBuildStoryDetailPageResolverNamesEveryArticle(t *testing.T) {
	articles := []Article{
		{ID: "a1", SourceID: "src-a", Title: "Story", URL: "https://a.com/1", PublishedAt: "2024-01-01T10:00:00Z"},
		{ID: "a2", SourceID: "src-b", Title: "Story", URL: "https://b.com/2", PublishedAt: "2024-01-01T11:00:00Z"},
		{ID: "a3", SourceID: "src-c", Title: "Story", URL: "https://c.com/3", PublishedAt: "2024-01-01T12:00:00Z"},
	}

	group := NewStoryGroup(articles[0])
	group.AddArticle(articles[1])
	group.AddArticle(articles[2])

	resolver := NewStaticSourceResolver(map[string]string{
		"src-a": "Publisher A",
		"src-b": "Publisher B",
		"src-c": "Publisher C",
	})

	page, err := BuildStoryDetailPage(group, resolver, time.Now())
	if err != nil {
		t.Fatalf("BuildStoryDetailPage: %v", err)
	}
	want := map[string]string{
		"https://a.com/1": "Publisher A",
		"https://b.com/2": "Publisher B",
		"https://c.com/3": "Publisher C",
	}
	for _, art := range page.Articles {
		if want[art.URL] != art.SourceName {
			t.Errorf("article %q SourceName = %q; want %q", art.URL, art.SourceName, want[art.URL])
		}
	}
}

// TestBuildStoryDetailPage_DisplayHeadline (#201): the page Headline carries the
// original display casing when the group has a DisplayHeadline, while the Slug
// and PageKey remain derived from the normalized representative title (AC-2).
func TestBuildStoryDetailPage_DisplayHeadline(t *testing.T) {
	art := Article{
		ID:           "a1",
		SourceID:     "official-min",
		Title:        "chiefs sign rb james cook", // normalized identity
		DisplayTitle: "Chiefs sign RB James Cook", // original casing
		Summary:      "summary",
		URL:          "https://example.com/chiefs-cook",
		PublishedAt:  "2024-01-01T10:00:00Z",
	}
	group := NewStoryGroup(art)

	resolver := NewStaticSourceResolver(map[string]string{"official-min": "Official"})
	page, err := BuildStoryDetailPage(group, resolver, time.Now())
	if err != nil {
		t.Fatalf("BuildStoryDetailPage: %v", err)
	}

	// Headline uses display casing.
	if page.Headline != "Chiefs sign RB James Cook" {
		t.Errorf("Headline = %q; want display casing %q", page.Headline, "Chiefs sign RB James Cook")
	}
	// Slug is derived from the normalized title (lowercased).
	if page.Slug != StorySlug("chiefs-sign-rb-james-cook") {
		t.Errorf("Slug = %q; want normalized slug", page.Slug)
	}
	// Article title uses display casing.
	if len(page.Articles) != 1 {
		t.Fatalf("expected 1 article, got %d", len(page.Articles))
	}
	if page.Articles[0].Title != "Chiefs sign RB James Cook" {
		t.Errorf("Articles[0].Title = %q; want display casing", page.Articles[0].Title)
	}
}

// TestBuildStoryDetailPage_DisplayHeadlineFallback (#201, AC-3): when the group
// has no DisplayHeadline (legacy), the page Headline falls back to the group
// Headline (normalized) and the slug/page key are unchanged.
func TestBuildStoryDetailPage_DisplayHeadlineFallback(t *testing.T) {
	art := Article{
		ID:          "a1",
		SourceID:    "official-min",
		Title:       "chiefs sign rb james cook",
		Summary:     "summary",
		URL:         "https://example.com/chiefs-cook",
		PublishedAt: "2024-01-01T10:00:00Z",
	}
	group := NewStoryGroup(art)

	resolver := NewStaticSourceResolver(map[string]string{"official-min": "Official"})
	page, err := BuildStoryDetailPage(group, resolver, time.Now())
	if err != nil {
		t.Fatalf("BuildStoryDetailPage: %v", err)
	}
	if page.Headline != "chiefs sign rb james cook" {
		t.Errorf("Headline = %q; want normalized fallback", page.Headline)
	}
	if page.Slug != StorySlug("chiefs-sign-rb-james-cook") {
		t.Errorf("Slug = %q; want normalized slug", page.Slug)
	}
}

// TestStoryPageSlugInvariantUnderDisplayTitle (#201, AC-2): two groups built from
// the same normalized title but with different display casing must produce the
// same Slug and PageKey, while their Headlines (display) may differ.
func TestStoryPageSlugInvariantUnderDisplayTitle(t *testing.T) {
	resolver := NewStaticSourceResolver(map[string]string{"official-min": "Official"})

	mixedArt := Article{
		ID:           "a1",
		SourceID:     "official-min",
		Title:        "chiefs sign rb james cook",
		DisplayTitle: "Chiefs sign RB James Cook",
		Summary:      "s",
		URL:          "https://example.com/chiefs-cook",
		PublishedAt:  "2024-01-01T10:00:00Z",
	}
	lowerArt := Article{
		ID:           "a1",
		SourceID:     "official-min",
		Title:        "chiefs sign rb james cook",
		DisplayTitle: "chiefs sign rb james cook",
		Summary:      "s",
		URL:          "https://example.com/chiefs-cook",
		PublishedAt:  "2024-01-01T10:00:00Z",
	}

	pageMixed, err := BuildStoryDetailPage(NewStoryGroup(mixedArt), resolver, time.Now())
	if err != nil {
		t.Fatalf("BuildStoryDetailPage(mixed): %v", err)
	}
	pageLower, err := BuildStoryDetailPage(NewStoryGroup(lowerArt), resolver, time.Now())
	if err != nil {
		t.Fatalf("BuildStoryDetailPage(lower): %v", err)
	}

	if pageMixed.Slug != pageLower.Slug {
		t.Errorf("Slug differs under display casing: mixed=%q lower=%q", pageMixed.Slug, pageLower.Slug)
	}
	if pageMixed.PageKey != pageLower.PageKey {
		t.Errorf("PageKey differs under display casing: mixed=%q lower=%q", pageMixed.PageKey, pageLower.PageKey)
	}
	// The display Headlines should reflect each group's display casing.
	if pageMixed.Headline != "Chiefs sign RB James Cook" {
		t.Errorf("mixed Headline = %q; want display casing", pageMixed.Headline)
	}
	if pageLower.Headline != "chiefs sign rb james cook" {
		t.Errorf("lower Headline = %q; want normalized", pageLower.Headline)
	}
}
