package domain

import (
	"testing"
	"time"
)

// AC-1: Entity overlap never merges on its own. Two articles sharing an entity
// but describing different events must not merge, even with a cross-source
// canonical-URL pattern match (the URL fast path must not bypass the constraint).

func TestClusterer_EntityOverlapNoMerge(t *testing.T) {
	c := NewClusterer()

	// Same team ("chiefs"), different events, same canonical URL pattern across
	// different domains. Before the fix, the URL fast path + any single entity
	// overlap merged these. After the fix, the entity sets are only partially
	// compatible (same "chiefs" but different player entities), and the titles
	// are unrelated, so they must NOT merge.
	fe1 := FeedEntry{
		Title:       "Chiefs Sign Cornerback",
		Source:      "espn",
		PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		Summary:     "Kansas City added a cornerback on a one-year deal.",
		URL:         "https://espn.com/article?id=42",
		Entities:    []string{"chiefs", "cornerback"},
	}
	fe2 := FeedEntry{
		Title:       "Chiefs Trade Quarterback",
		Source:      "fox-sports",
		PublishedAt: time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
		Summary:     "Kansas City sent a quarterback to another club.",
		URL:         "https://foxsports.com/article?id=42", // same URL path on another domain
		Entities:    []string{"chiefs", "quarterback"},
	}

	articles := []Article{
		NewArticle(fe1, "a1", time.Now().UTC().Format(time.RFC3339)),
		NewArticle(fe2, "a2", time.Now().UTC().Format(time.RFC3339)),
	}

	result := c.Cluster(articles)
	if len(result) != 2 {
		t.Fatalf("Cluster(shared team, different events, same URL pattern) returned %d groups; want 2 (entity constraint blocks merge)", len(result))
	}
}

// TestClusterer_DuplicateCoverage_MultiOutlet guards against over-correction:
// the same team + same event covered by three outlets must still merge into one
// group. This is the AC-1 "identical/compatible entity sets preserve true
// duplicate-coverage merging" half.
func TestClusterer_DuplicateCoverage_MultiOutlet(t *testing.T) {
	c := NewClusterer()

	// Three outlets, same team, same event (identical entity sets).
	fe1 := FeedEntry{
		Title:       "Steelers Draft Linebacker Marcus Lee",
		Source:      "espn",
		PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		Summary:     "Pittsburgh selected linebacker Marcus Lee in the third round.",
		URL:         "https://espn.com/draft-1",
		Entities:    []string{"steelers", "marcus-lee"},
	}
	fe2 := FeedEntry{
		Title:       "Steelers Select Linebacker Marcus Lee",
		Source:      "fox-sports",
		PublishedAt: time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
		Summary:     "Pittsburgh selected linebacker Marcus Lee in the third round.",
		URL:         "https://foxsports.com/draft-1",
		Entities:    []string{"steelers", "marcus-lee"},
	}
	fe3 := FeedEntry{
		Title:       "Steelers Take Linebacker Marcus Lee",
		Source:      "cbs-sports",
		PublishedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Summary:     "Pittsburgh selected linebacker Marcus Lee in the third round.",
		URL:         "https://cbssports.com/draft-1",
		Entities:    []string{"steelers", "marcus-lee"},
	}

	articles := []Article{
		NewArticle(fe1, "a1", time.Now().UTC().Format(time.RFC3339)),
		NewArticle(fe2, "a2", time.Now().UTC().Format(time.RFC3339)),
		NewArticle(fe3, "a3", time.Now().UTC().Format(time.RFC3339)),
	}

	result := c.Cluster(articles)
	if len(result) != 1 {
		t.Fatalf("Cluster(3 outlets, same team+event) returned %d groups; want 1 (duplicate coverage must merge)", len(result))
	}
	if result[0].Size() != 3 {
		t.Errorf("group size = %d; want 3", result[0].Size())
	}
}

// TestClusterer_BoilerplateSummaryNoMerge: identical boilerplate leads with
// shared-but-not-identical entities must not merge via the summary fast path.
func TestClusterer_BoilerplateSummaryNoMerge(t *testing.T) {
	c := NewClusterer()

	fe1 := FeedEntry{
		Title:       "Bears Trade Running Back",
		Source:      "espn",
		PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		Summary:     "The league continued its offseason shuffle on Tuesday.",
		URL:         "https://espn.com/trade-1",
		Entities:    []string{"bears", "runningback"},
	}
	fe2 := FeedEntry{
		Title:       "Cowboys Sign Guard",
		Source:      "fox-sports",
		PublishedAt: time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
		Summary:     "The league continued its offseason shuffle on Tuesday.", // same boilerplate lead
		URL:         "https://foxsports.com/sign-1",
		Entities:    []string{"cowboys", "guard"},
	}

	articles := []Article{
		NewArticle(fe1, "a1", time.Now().UTC().Format(time.RFC3339)),
		NewArticle(fe2, "a2", time.Now().UTC().Format(time.RFC3339)),
	}

	result := c.Cluster(articles)
	if len(result) != 2 {
		t.Fatalf("Cluster(boilerplate summary, disjoint entities) returned %d groups; want 2", len(result))
	}
}

// TestEntityAffinity is a table test for the entity-set affinity function.
func TestEntityAffinity(t *testing.T) {
	tests := []struct {
		name     string
		a        Article
		b        Article
		expected float64
	}{
		{"both entity-less", Article{}, Article{}, 1.0},
		{"one side entity-less", Article{Entities: []string{"chiefs"}}, Article{}, 1.0},
		{"identical sets", Article{Entities: []string{"chiefs", "qb"}}, Article{Entities: []string{"qb", "chiefs"}}, 1.0},
		{"disjoint sets", Article{Entities: []string{"chiefs"}}, Article{Entities: []string{"eagles"}}, 0.0},
		{"partial overlap 1of4", Article{Entities: []string{"chiefs", "a", "b"}}, Article{Entities: []string{"chiefs", "c"}}, 1.0 / 4.0},
		{"normalization case", Article{Entities: []string{"Chiefs"}}, Article{Entities: []string{"chiefs"}}, 1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EntityAffinity(tt.a, tt.b)
			if got != tt.expected {
				t.Errorf("EntityAffinity(%v, %v) = %v; want %v", tt.a.Entities, tt.b.Entities, got, tt.expected)
			}
		})
	}
}

// TestEntityAffinity_WhitespaceOnlyIgnores: whitespace-only entities are
// ignored, so an article with only blank entities is treated as entity-less.
func TestEntityAffinity_WhitespaceOnlyIgnores(t *testing.T) {
	a := Article{Entities: []string{"  ", "\t"}}
	b := Article{Entities: []string{"chiefs"}}
	// a is effectively entity-less -> neutral 1.0
	if got := EntityAffinity(a, b); got != 1.0 {
		t.Errorf("EntityAffinity(whitespace-only, chiefs) = %v; want 1.0", got)
	}
}
