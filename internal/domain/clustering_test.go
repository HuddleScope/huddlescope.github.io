package domain

import (
	"reflect"
	"testing"
	"time"
)

// Tests for TokenizeTitle
func TestTokenizeTitle(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"empty string", "", nil},
		{"single word", "hello", []string{"hello"}},
		{"multiple words", "hello world", []string{"hello", "world"}},
		{"with punctuation", "hello, world!", []string{"hello", "world"}},
		{"with stopwords", "the quick brown fox", []string{"quick", "brown", "fox"}},
		{"mixed case", "Hello WORLD test", []string{"hello", "world", "test"}},
		{"with numbers", " Super Bowl 50 ", []string{"super", "bowl", "50"}},
		{"only stopwords", "the and or but", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TokenizeTitle(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("TokenizeTitle(%q) = %v; expected %v", tt.input, result, tt.expected)
				return
			}
			for i, token := range result {
				if token != tt.expected[i] {
					t.Errorf("TokenizeTitle(%q)[%d] = %q; want %q", tt.input, i, token, tt.expected[i])
				}
			}
		})
	}
}

// Tests for JaccardSimilarity
func TestJaccardSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		tokens1  []string
		tokens2  []string
		expected float64
	}{
		{"both empty", nil, nil, 1.0},
		{"first empty", nil, []string{"a"}, 0.0},
		{"second empty", []string{"a"}, nil, 0.0},
		{"identical", []string{"a", "b"}, []string{"a", "b"}, 1.0},
		{"disjoint", []string{"a"}, []string{"b"}, 0.0},
		{"partial overlap", []string{"a", "b"}, []string{"b", "c"}, 1.0 / 3.0},
		{"different order", []string{"a", "b", "c"}, []string{"c", "a", "b"}, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := JaccardSimilarity(tt.tokens1, tt.tokens2)
			if result != tt.expected {
				t.Errorf("JaccardSimilarity(%v, %v) = %v; want %v", tt.tokens1, tt.tokens2, result, tt.expected)
			}
		})
	}
}

// Tests for TimeDiffHours
func TestTimeDiffHours(t *testing.T) {
	t1 := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 1, 15, 0, 0, 0, time.UTC)
	t3 := time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		t1       time.Time
		t2       time.Time
		expected float64
	}{
		{"3 hours", t1, t2, 3.0},
		{"same time", t1, t1, 0.0},
		{"24 hours", t1, t3, 24.0},
		{"reversed", t2, t1, 3.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TimeDiffHours(tt.t1, tt.t2)
			if result != tt.expected {
				t.Errorf("TimeDiffHours(%v, %v) = %v; want %v", tt.t1, tt.t2, result, tt.expected)
			}
		})
	}
}

// Tests for TimeWithinWindow
func TestTimeWithinWindow(t *testing.T) {
	t1 := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 1, 15, 0, 0, 0, time.UTC) // 3 hours
	t3 := time.Date(2024, 1, 2, 12, 1, 0, 0, time.UTC) // 24h 1m

	tests := []struct {
		name     string
		t1       time.Time
		t2       time.Time
		window   time.Duration
		expected bool
	}{
		{"within window", t1, t2, 48 * time.Hour, true},
		{"exactly at boundary", t1, t3, 24 * time.Hour, false},
		{"within 24h", t1, t3, 48 * time.Hour, true},
		{"same time", t1, t1, 1 * time.Hour, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TimeWithinWindow(tt.t1, tt.t2, tt.window)
			if result != tt.expected {
				t.Errorf("TimeWithinWindow(%v, %v, %v) = %v; want %v", tt.t1, tt.t2, tt.window, result, tt.expected)
			}
		})
	}
}

// Tests for NewArticle
func TestNewArticle(t *testing.T) {
	fe := FeedEntry{
		Title:       "  Test   Title  ",
		Source:      "  TEST-SOURCE  ",
		PublishedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Summary:     "<p>Test <b>summary</b></p>",
		URL:         "https://example.com/test",
		GUID:        "guid-123",
		Entities:    []string{"chiefs", "wr"},
	}

	article := NewArticle(fe, "article-1", time.Now().UTC().Format(time.RFC3339))

	if article.ID != "article-1" {
		t.Errorf("ID = %q; want article-1", article.ID)
	}
	if article.Title != "test title" {
		t.Errorf("Title = %q; want test title", article.Title)
	}
	if article.SourceID != "test-source" {
		t.Errorf("SourceID = %q; want test-source", article.SourceID)
	}
	if article.Summary != "Test summary" {
		t.Errorf("Summary = %q; want Test summary", article.Summary)
	}
	if article.URL != "https://example.com/test" {
		t.Errorf("URL = %q; want https://example.com/test", article.URL)
	}
	if len(article.Entities) != 2 {
		t.Errorf("Entities length = %d; want 2", len(article.Entities))
	}

	parsed, err := article.ParsePublishedAt()
	if err != nil {
		t.Fatalf("ParsePublishedAt error: %v", err)
	}
	if parsed.Year() != 2024 || parsed.Month() != 1 || parsed.Day() != 1 {
		t.Errorf("Parsed PublishedAt wrong: %v", parsed)
	}
}

// Tests for Article.HasOverlappingEntity
func TestArticle_HasOverlappingEntity(t *testing.T) {
	a1 := Article{
		ID:       "a1",
		Entities: []string{"chiefs", "eagles"},
	}
	a2 := Article{
		ID:       "a2",
		Entities: []string{"eagles", "bills"},
	}
	a3 := Article{
		ID:       "a3",
		Entities: []string{"lions", "rams"},
	}
	a4 := Article{
		ID:       "a4",
		Entities: []string{},
	}

	tests := []struct {
		name     string
		a        Article
		other    *Article
		expected bool
	}{
		{"shared entity", a1, &a2, true},
		{"no shared entity", a1, &a3, false},
		{"self", a1, &a1, true},
		{"no entities on self", a4, &a1, false},
		{"no entities on other", a1, &a4, false},
		{"both no entities", a4, &a4, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.a.HasOverlappingEntity(tt.other)
			if result != tt.expected {
				t.Errorf("HasOverlappingEntity() = %v; want %v", result, tt.expected)
			}
		})
	}
}

// Tests for StoryGroup
func TestNewStoryGroup(t *testing.T) {
	fe := FeedEntry{
		Title:       "Test Story",
		Source:      "test-source",
		PublishedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Summary:     "Summary",
		URL:         "https://example.com",
	}

	article := NewArticle(fe, "article-1", time.Now().UTC().Format(time.RFC3339))
	group := NewStoryGroup(article)

	if group.Headline != "test story" {
		t.Errorf("Headline = %q; want test story", group.Headline)
	}
	if len(group.Articles) != 1 {
		t.Errorf("Articles length = %d; want 1", len(group.Articles))
	}
	if group.Articles[0].ID != "article-1" {
		t.Errorf("Article ID = %q; want article-1", group.Articles[0].ID)
	}
}

func TestStoryGroup_AddArticle(t *testing.T) {
	fe1 := FeedEntry{
		Title:       "Later Title",
		Source:      "test-source",
		PublishedAt: time.Date(2024, 1, 1, 15, 0, 0, 0, time.UTC),
		Summary:     "Summary 1",
		URL:         "https://example.com/1",
	}

	fe2 := FeedEntry{
		Title:       "Earlier Title",
		Source:      "test-source",
		PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		Summary:     "Summary 2",
		URL:         "https://example.com/2",
	}

	article1 := NewArticle(fe1, "article-1", time.Now().UTC().Format(time.RFC3339))
	group := NewStoryGroup(article1)

	// Add earlier article - should update headline
	article2 := NewArticle(fe2, "article-2", time.Now().UTC().Format(time.RFC3339))
	group.AddArticle(article2)

	if group.Headline != "earlier title" {
		t.Errorf("Headline after add = %q; want earlier title", group.Headline)
	}
	if len(group.Articles) != 2 {
		t.Errorf("Articles length = %d; want 2", len(group.Articles))
	}
}

func TestComputeGroupID(t *testing.T) {
	fe1 := FeedEntry{
		Title:       "Test Title B",
		Source:      "test-source",
		PublishedAt: time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC),
		Summary:     "Summary",
		URL:         "https://example.com/b",
	}

	fe2 := FeedEntry{
		Title:       "Test Title A",
		Source:      "test-source",
		PublishedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Summary:     "Summary",
		URL:         "https://example.com/a",
	}

	article1 := NewArticle(fe1, "article-1", time.Now().UTC().Format(time.RFC3339))
	article2 := NewArticle(fe2, "article-2", time.Now().UTC().Format(time.RFC3339))

	// Test with unordered articles - should produce same ID as sorted order
	group1 := NewStoryGroup(article1)
	group1.AddArticle(article2)

	group2 := NewStoryGroup(article2)
	group2.AddArticle(article1)

	// IDs should be the same regardless of input order
	if group1.ID != group2.ID {
		t.Errorf("Group IDs differ: %q vs %q", group1.ID, group2.ID)
	}

	// ID should be 16 hex chars
	if len(group1.ID) != 16 {
		t.Errorf("Group ID length = %d; want 16", len(group1.ID))
	}
}

// Tests for Clusterer
func TestClusterer_Cluster(t *testing.T) {
	c := NewClusterer()

	// Test empty input
	result := c.Cluster(nil)
	if len(result) != 0 {
		t.Errorf("Cluster(nil) returned %d groups; want 0", len(result))
	}

	// Test single article
	fe1 := FeedEntry{
		Title:       "Chiefs Sign Free Agent",
		Source:      "espn",
		PublishedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Summary:     "Summary",
		URL:         "https://espn.com/1",
		Entities:    []string{"chiefs"},
	}

	articles := []Article{NewArticle(fe1, "a1", time.Now().UTC().Format(time.RFC3339))}
	result = c.Cluster(articles)

	if len(result) != 1 {
		t.Errorf("Cluster(1 article) returned %d groups; want 1", len(result))
	}
	if result[0].Headline != "chiefs sign free agent" {
		t.Errorf("Headline = %q; want chiefs sign free agent", result[0].Headline)
	}
}

func TestClusterer_Cluster_DuplicateCoverage(t *testing.T) {
	c := NewClusterer()

	// Three articles with similar titles, same time, same entity -> should be 1 group
	fe1 := FeedEntry{
		Title:       "Chiefs Sign WR John Doe",
		Source:      "espn",
		PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		Summary:     "ESPN report",
		URL:         "https://espn.com/1",
		Entities:    []string{"chiefs"},
	}

	fe2 := FeedEntry{
		Title:       "Chiefs Add WR John Doe",
		Source:      "fox-sports",
		PublishedAt: time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
		Summary:     "Fox Sports report",
		URL:         "https://foxsports.com/1",
		Entities:    []string{"chiefs"},
	}

	fe3 := FeedEntry{
		Title:       "Chiefs Ink WR John Doe",
		Source:      "cbs-sports",
		PublishedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Summary:     "CBS Sports report",
		URL:         "https://cbssports.com/1",
		Entities:    []string{"chiefs"},
	}

	articles := []Article{
		NewArticle(fe1, "a1", time.Now().UTC().Format(time.RFC3339)),
		NewArticle(fe2, "a2", time.Now().UTC().Format(time.RFC3339)),
		NewArticle(fe3, "a3", time.Now().UTC().Format(time.RFC3339)),
	}

	result := c.Cluster(articles)

	if len(result) != 1 {
		t.Errorf("Cluster(3 similar articles) returned %d groups; want 1 (duplicate coverage)", len(result))
	} else {
		if result[0].Size() != 3 {
			t.Errorf("Group size = %d; want 3 (all articles grouped)", result[0].Size())
		}
	}
}

func TestClusterer_Cluster_NoFalseMerge(t *testing.T) {
	c := NewClusterer()

	// Two articles with very different titles -> separate groups
	fe1 := FeedEntry{
		Title:       "Chiefs Sign WR X",
		Source:      "espn",
		PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		Summary:     "Summary 1",
		URL:         "https://espn.com/1",
		Entities:    []string{"chiefs"},
	}

	fe2 := FeedEntry{
		Title:       "Eagles Trade RB Y", // Very different: different verb, entity
		Source:      "fox-sports",
		PublishedAt: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC), // 14 days later
		Summary:     "Summary 2",
		URL:         "https://foxsports.com/1",
		Entities:    []string{"eagles"},
	}

	articles := []Article{
		NewArticle(fe1, "a1", time.Now().UTC().Format(time.RFC3339)),
		NewArticle(fe2, "a2", time.Now().UTC().Format(time.RFC3339)),
	}

	result := c.Cluster(articles)

	if len(result) != 2 {
		t.Errorf("Cluster(different articles) returned %d groups; want 2 (no false merge)", len(result))
	}
}

func TestClusterer_Cluster_SoloSource(t *testing.T) {
	c := NewClusterer()

	// Single article with no entities -> should be singleton group
	fe := FeedEntry{
		Title:       "Lone Article",
		Source:      "solo-feed",
		PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		Summary:     "Summary",
		URL:         "https://solo.com/1",
	}

	articles := []Article{NewArticle(fe, "a1", time.Now().UTC().Format(time.RFC3339))}
	result := c.Cluster(articles)

	if len(result) != 1 {
		t.Fatalf("Cluster(1 article) returned %d groups; want 1", len(result))
	}

	if result[0].Size() != 1 {
		t.Errorf("Group size = %d; want 1 (solo)", result[0].Size())
	}

	// Check that article URL is preserved (via first article in group)
	if result[0].Articles[0].URL != "https://solo.com/1" {
		t.Errorf("Article URL = %q; want https://solo.com/1", result[0].Articles[0].URL)
	}
}

func TestClusterer_Cluster_EntityGate(t *testing.T) {
	c := NewClusterer()

	// Same title structure, same time, but different entities -> separate groups
	fe1 := FeedEntry{
		Title:       "Lions Win Sunday",
		Source:      "det-news",
		PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		Summary:     "Lions win",
		URL:         "https://detnews.com/1",
		Entities:    []string{"lions"},
	}

	fe2 := FeedEntry{
		Title:       "Lions Win Sunday", // Same title!
		Source:      "philly-news",
		PublishedAt: time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
		Summary:     "Eagles win",
		URL:         "https://phillynews.com/1",
		Entities:    []string{"eagles"}, // But different entity
	}

	articles := []Article{
		NewArticle(fe1, "a1", time.Now().UTC().Format(time.RFC3339)),
		NewArticle(fe2, "a2", time.Now().UTC().Format(time.RFC3339)),
	}

	result := c.Cluster(articles)

	if len(result) != 2 {
		t.Errorf("Cluster(different entities, same title) returned %d groups; want 2 (entity gate)", len(result))
	}
}

func TestClusterer_Cluster_Idempotent(t *testing.T) {
	c := NewClusterer()

	fe1 := FeedEntry{
		Title:       "Test Article",
		Source:      "test-feed",
		PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		Summary:     "Summary",
		URL:         "https://example.com/1",
	}
	fe2 := FeedEntry{
		Title:       "Test Article 2",
		Source:      "test-feed",
		PublishedAt: time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
		Summary:     "Summary 2",
		URL:         "https://example.com/2",
	}

	articles := []Article{
		NewArticle(fe1, "a1", time.Now().UTC().Format(time.RFC3339)),
		NewArticle(fe2, "a2", time.Now().UTC().Format(time.RFC3339)),
	}

	// First run
	result1 := c.Cluster(articles)

	// Second run with same input
	result2 := c.Cluster(articles)

	if len(result1) != len(result2) {
		t.Errorf("Cluster idempotency failed: first run %d groups, second run %d groups", len(result1), len(result2))
	}

	for i := range result1 {
		if result1[i].ID != result2[i].ID {
			t.Errorf("Cluster idempotency failed: group %d ID differs", i)
		}
	}
}

func TestNewStoryFromGroup(t *testing.T) {
	fe1 := FeedEntry{
		Title:       "Earliest Title",
		Source:      "test-feed",
		PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		Summary:     "Summary 1",
		URL:         "https://example.com/1",
	}

	fe2 := FeedEntry{
		Title:       "Later Title",
		Source:      "test-feed",
		PublishedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Summary:     "Summary 2",
		URL:         "https://example.com/2",
	}

	article1 := NewArticle(fe1, "article-1", time.Now().UTC().Format(time.RFC3339))
	article2 := NewArticle(fe2, "article-2", time.Now().UTC().Format(time.RFC3339))

	group := NewStoryGroup(article1)
	group.AddArticle(article2)

	story := NewStoryFromGroup(group)

	if story.ID != group.ID {
		t.Errorf("Story ID = %q; want %q", story.ID, group.ID)
	}
	if story.Headline != "earliest title" {
		t.Errorf("Story Headline = %q; want earliest title", story.Headline)
	}
	if len(story.ArticleIDs) != 2 {
		t.Errorf("Story ArticleIDs length = %d; want 2", len(story.ArticleIDs))
	}
}

func TestGroupingResult_NewGroupingResult(t *testing.T) {
	result := NewGroupingResult()

	if result.GroupsCreated != 0 {
		t.Errorf("GroupsCreated = %d; want 0", result.GroupsCreated)
	}
	if result.GroupsMerged != 0 {
		t.Errorf("GroupsMerged = %d; want 0", result.GroupsMerged)
	}
	if result.ArticlesGrouped != 0 {
		t.Errorf("ArticlesGrouped = %d; want 0", result.ArticlesGrouped)
	}
	if result.ArticlesSolo != 0 {
		t.Errorf("ArticlesSolo = %d; want 0", result.ArticlesSolo)
	}
	if result.Errors != nil {
		t.Errorf("Errors = %v; want nil", result.Errors)
	}
}

// TestFingerprintSummary tests the new fingerprint summary function
func TestFingerprintSummary(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"empty string", "", nil},
		{"simple text", "hello world", []string{"hello", "world"}},
		{"with stopwords", "the quick brown fox", []string{"quick", "brown", "fox"}},
		{"mixed case", "Hello WORLD", []string{"hello", "world"}},
		{"punctuation", "Hello, world!", []string{"hello", "world"}},
		{"idempotent", "Hello World Test", []string{"hello", "world", "test"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FingerprintSummary(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("FingerprintSummary(%q) = %v; expected %v", tt.input, result, tt.expected)
			}
			for i, token := range result {
				if token != tt.expected[i] {
					t.Errorf("FingerprintSummary(%q)[%d] = %q; want %q", tt.input, i, token, tt.expected[i])
				}
			}
			// Test idempotency
			result2 := FingerprintSummary(tt.input)
			if !reflect.DeepEqual(result, result2) {
				t.Errorf("FingerprintSummary not idempotent")
			}
		})
	}
}

// TestClusterer_URLFastPath tests the URL fast path for canonical URLs
func TestClusterer_URLFastPath(t *testing.T) {
	c := NewClusterer()

	// Same canonical URL should merge regardless of tracking params
	fe1 := FeedEntry{
		Title:       "Chiefs Sign WR",
		Source:      "espn",
		PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		Summary:     "ESPN report",
		URL:         "https://espn.com/article?id=123&utm_source=news",
		Entities:    []string{"chiefs"},
	}

	fe2 := FeedEntry{
		Title:       "Chiefs Add WR John Doe", // Different title
		Source:      "espn",
		PublishedAt: time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
		Summary:     "ESPN report",
		URL:         "https://espn.com/article?id=123", // Same canonical URL
		Entities:    []string{"chiefs"},
	}

	articles := []Article{
		NewArticle(fe1, "a1", time.Now().UTC().Format(time.RFC3339)),
		NewArticle(fe2, "a2", time.Now().UTC().Format(time.RFC3339)),
	}

	result := c.Cluster(articles)

	if len(result) != 1 {
		t.Errorf("Cluster(URLEquivalent) returned %d groups; want 1 (URL fast path)", len(result))
	}
}

// TestClusterer_ChangedTitleSameLead tests AC-3: changed title but same lead should merge
func TestClusterer_ChangedTitleSameLead(t *testing.T) {
	c := NewClusterer()

	// Same summary lead, different titles
	fe1 := FeedEntry{
		Title:       "Chiefs Sign Free Agent John Doe",
		Source:      "espn",
		PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		Summary:     "The Kansas City Chiefs signed free agent John Doe to a multi-year contract.",
		URL:         "https://espn.com/1",
		Entities:    []string{"chiefs"},
	}

	fe2 := FeedEntry{
		Title:       "Chiefs AddVeteran Playerto Roster",
		Source:      "fox-sports",
		PublishedAt: time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
		Summary:     "The Kansas City Chiefs signed free agent John Doe to a multi-year contract.", // Same summary
		URL:         "https://foxsports.com/1",
		Entities:    []string{"chiefs"},
	}

	articles := []Article{
		NewArticle(fe1, "a1", time.Now().UTC().Format(time.RFC3339)),
		NewArticle(fe2, "a2", time.Now().UTC().Format(time.RFC3339)),
	}

	result := c.Cluster(articles)

	if len(result) != 1 {
		t.Errorf("Cluster(changed title, same lead) returned %d groups; want 1 (summary matching)", len(result))
	}
}

// TestClusterer_URLMatchSameSource tests URL match requires same source or entity overlap
func TestClusterer_URLMatchSameSource(t *testing.T) {
	c := NewClusterer()

	// Same URL but different sources (even with same entity) should merge
	fe1 := FeedEntry{
		Title:       "Chiefs Sign Player",
		Source:      "espn",
		PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		Summary:     "ESPN report",
		URL:         "https://espn.com/article?id=123",
		Entities:    []string{"chiefs"},
	}

	fe2 := FeedEntry{
		Title:       "Chiefs Sign Player", // Same title
		Source:      "cbs-sports",         // Different source but same entity
		PublishedAt: time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
		Summary:     "CBS Sports report",
		URL:         "https://cbssports.com/article?id=123", // Same canonical URL after www strip
		Entities:    []string{"chiefs"},
	}

	articles := []Article{
		NewArticle(fe1, "a1", time.Now().UTC().Format(time.RFC3339)),
		NewArticle(fe2, "a2", time.Now().UTC().Format(time.RFC3339)),
	}

	result := c.Cluster(articles)

	// Should merge because same canonical URL and shared entity
	if len(result) != 1 {
		t.Errorf("Cluster(same URL, same entity) returned %d groups; want 1", len(result))
	}
}

// TestClusterer_UrlDifferentNoEntity tests URL match requires either same source or entity overlap
func TestClusterer_URLDifferentNoEntity(t *testing.T) {
	c := NewClusterer()

	// Same URL but different sources and no entities -> should still merge
	fe1 := FeedEntry{
		Title:       "Breaking News",
		Source:      "news-com",
		PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		Summary:     "Breaking news story.",
		URL:         "https://news.com/article?id=123",
	}

	fe2 := FeedEntry{
		Title:       "Breaking News Updated",
		Source:      "news-org",
		PublishedAt: time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
		Summary:     "Breaking news story.",
		URL:         "https://news.org/article?id=123", // Same canonical URL
	}

	articles := []Article{
		NewArticle(fe1, "a1", time.Now().UTC().Format(time.RFC3339)),
		NewArticle(fe2, "a2", time.Now().UTC().Format(time.RFC3339)),
	}

	result := c.Cluster(articles)

	// Should merge because same canonical URL and neither has entities
	if len(result) != 1 {
		t.Errorf("Cluster(same URL, no entities) returned %d groups; want 1", len(result))
	}
}

// TestNewArticle_DisplayTitle (#201): NewArticle sets the normalized Title
// (lowercased) and the original-casing DisplayTitle from the same feed entry.
func TestNewArticle_DisplayTitle(t *testing.T) {
	fe := FeedEntry{
		Title:       "  Chiefs   sign RB James Cook  ",
		Source:      "official-min",
		PublishedAt: time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC),
		Summary:     "<p>Chiefs sign RB James Cook.</p>",
		URL:         "https://example.com/chiefs-cook",
	}
	a := NewArticle(fe, "a1", time.Now().UTC().Format(time.RFC3339))
	if a.Title != NormalizeTitle(fe.Title) {
		t.Errorf("Title = %q; want normalized %q", a.Title, NormalizeTitle(fe.Title))
	}
	if a.Title != "chiefs sign rb james cook" {
		t.Errorf("Title = %q; want %q", a.Title, "chiefs sign rb james cook")
	}
	if a.DisplayTitle != "Chiefs sign RB James Cook" {
		t.Errorf("DisplayTitle = %q; want %q", a.DisplayTitle, "Chiefs sign RB James Cook")
	}
	if a.Display() != "Chiefs sign RB James Cook" {
		t.Errorf("Display() = %q; want %q", a.Display(), "Chiefs sign RB James Cook")
	}
}

// TestArticle_Display_Fallback (#201): Article.Display() returns DisplayTitle
// when set, otherwise the normalized Title.
func TestArticle_Display_Fallback(t *testing.T) {
	both := Article{Title: "normalized", DisplayTitle: "Original Casing"}
	if both.Display() != "Original Casing" {
		t.Errorf("both set: Display() = %q; want %q", both.Display(), "Original Casing")
	}
	empty := Article{Title: "normalized"}
	if empty.Display() != "normalized" {
		t.Errorf("empty display: Display() = %q; want %q", empty.Display(), "normalized")
	}
	ws := Article{Title: "normalized", DisplayTitle: "   "}
	if ws.Display() != "normalized" {
		t.Errorf("whitespace display: Display() = %q; want %q", ws.Display(), "normalized")
	}
}

// TestNewStoryGroup_DisplayHeadline (#201): NewStoryGroup propagates the
// article's display headline, and AddArticle updates it when a new earliest
// article is added.
func TestNewStoryGroup_DisplayHeadline(t *testing.T) {
	fe := FeedEntry{
		Title:       "Chiefs Sign RB James Cook",
		Source:      "official-min",
		PublishedAt: time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC),
		URL:         "https://example.com/chiefs-cook",
	}
	a := NewArticle(fe, "a1", time.Now().UTC().Format(time.RFC3339))
	g := NewStoryGroup(a)
	if g.Headline != "chiefs sign rb james cook" {
		t.Errorf("Headline = %q; want normalized", g.Headline)
	}
	if g.DisplayHeadline != "Chiefs Sign RB James Cook" {
		t.Errorf("DisplayHeadline = %q; want %q", g.DisplayHeadline, "Chiefs Sign RB James Cook")
	}
}

// TestNewStoryFromGroup_DisplayTitle (#201): NewStoryFromGroup sets
// Story.DisplayTitle from the group's display headline, with the normalized
// Story.Title/Headline unchanged.
func TestNewStoryFromGroup_DisplayTitle(t *testing.T) {
	fe := FeedEntry{
		Title:       "Chiefs Sign RB James Cook",
		Source:      "official-min",
		PublishedAt: time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC),
		URL:         "https://example.com/chiefs-cook",
	}
	a := NewArticle(fe, "a1", time.Now().UTC().Format(time.RFC3339))
	g := NewStoryGroup(a)
	s := NewStoryFromGroup(g)
	if s.Title != "chiefs sign rb james cook" {
		t.Errorf("Story.Title = %q; want normalized", s.Title)
	}
	if s.Headline != "chiefs sign rb james cook" {
		t.Errorf("Story.Headline = %q; want normalized", s.Headline)
	}
	if s.DisplayTitle != "Chiefs Sign RB James Cook" {
		t.Errorf("Story.DisplayTitle = %q; want %q", s.DisplayTitle, "Chiefs Sign RB James Cook")
	}
}

// TestComputeGroupID_InvariantUnderDisplayTitle (#201, AC-2): display casing
// must not leak into the group ID. Two groups built from the same normalized
// title (different casing / display headline) must produce the same group ID.
func TestComputeGroupID_InvariantUnderDisplayTitle(t *testing.T) {
	a1 := Article{
		ID:           "a1",
		SourceID:     "official-min",
		Title:        "chiefs sign rb james cook",
		DisplayTitle: "Chiefs Sign RB James Cook",
		PublishedAt:  "2026-11-17T12:00:00Z",
	}
	a2 := Article{
		ID:           "a1",
		SourceID:     "official-min",
		Title:        "chiefs sign rb james cook",
		DisplayTitle: "chiefs sign rb james cook",
		PublishedAt:  "2026-11-17T12:00:00Z",
	}
	id1 := computeGroupID([]Article{a1})
	id2 := computeGroupID([]Article{a2})
	if id1 != id2 {
		t.Errorf("computeGroupID changed with display casing: %q != %q", id1, id2)
	}
}

// TestClusterer_DisplayTitleDoesNotAffectClustering (#201, AC-2): clustering
// and Jaccard tokenization operate on the normalized Title, so display casing
// must not change clustering decisions or group IDs.
func TestClusterer_DisplayTitleDoesNotAffectClustering(t *testing.T) {
	c := NewClusterer()

	fe1 := FeedEntry{
		Title:       "Chiefs Sign RB James Cook",
		Source:      "espn",
		PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		Summary:     "ESPN report",
		URL:         "https://espn.com/article?id=1",
		Entities:    []string{"chiefs"},
	}
	fe2 := FeedEntry{
		Title:       "chiefs sign rb james cook", // all-lowercase variant
		Source:      "cbs-sports",
		PublishedAt: time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
		Summary:     "CBS Sports report",
		URL:         "https://cbssports.com/article?id=1",
		Entities:    []string{"chiefs"},
	}

	mixed := []Article{
		NewArticle(fe1, "a1", time.Now().UTC().Format(time.RFC3339)),
		NewArticle(fe2, "a2", time.Now().UTC().Format(time.RFC3339)),
	}
	groupsMixed := c.Cluster(mixed)

	// Same feed entries but all-lowercase titles (no display casing).
	fe1l := fe1
	fe1l.Title = "chiefs sign rb james cook"
	fe2l := fe2
	fe2l.Title = "chiefs sign rb james cook"
	lower := []Article{
		NewArticle(fe1l, "a1", time.Now().UTC().Format(time.RFC3339)),
		NewArticle(fe2l, "a2", time.Now().UTC().Format(time.RFC3339)),
	}
	groupsLower := c.Cluster(lower)

	if len(groupsMixed) != len(groupsLower) {
		t.Fatalf("group count differs: mixed=%d lower=%d", len(groupsMixed), len(groupsLower))
	}
	for i := range groupsMixed {
		if groupsMixed[i].ID != groupsLower[i].ID {
			t.Errorf("group %d ID differs: mixed=%q lower=%q", i, groupsMixed[i].ID, groupsLower[i].ID)
		}
		if groupsMixed[i].Headline != groupsLower[i].Headline {
			t.Errorf("group %d Headline differs: mixed=%q lower=%q", i, groupsMixed[i].Headline, groupsLower[i].Headline)
		}
	}
	// The mixed-case group must carry a display headline with original casing.
	if len(groupsMixed) == 1 {
		if groupsMixed[0].DisplayHeadline == "" {
			t.Errorf("mixed group DisplayHeadline is empty; want original casing")
		}
	}
}

// TestTokenizeTitle_LowercasesInternally (#201, AC-2): TokenizeTitle lowercases
// internally, so mixed-case vs lowercase inputs produce identical tokens.
func TestTokenizeTitle_LowercasesInternally(t *testing.T) {
	mixed := TokenizeTitle("Chiefs Sign RB James Cook")
	lower := TokenizeTitle("chiefs sign rb james cook")
	if len(mixed) != len(lower) {
		t.Fatalf("token count differs: mixed=%d lower=%d", len(mixed), len(lower))
	}
	for i := range mixed {
		if mixed[i] != lower[i] {
			t.Errorf("token %d differs: mixed=%q lower=%q", i, mixed[i], lower[i])
		}
	}
}
