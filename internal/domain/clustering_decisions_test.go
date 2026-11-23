package domain

import (
	"encoding/json"
	"testing"
	"time"
)

// TestClusterWithDecisions_ReasonCodes verifies that each merge path emits
// exactly one join decision with populated scores and a valid reason code.
func TestClusterWithDecisions_ReasonCodes(t *testing.T) {
	c := NewClusterer()

	// URL fast path: same source, same canonical URL.
	fe1 := FeedEntry{
		Title:       "Chiefs Sign WR",
		Source:      "espn",
		PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		Summary:     "report",
		URL:         "https://espn.com/article?id=123&utm_source=x",
		Entities:    []string{"chiefs"},
	}
	fe2 := FeedEntry{
		Title:       "Chiefs Add WR John Doe",
		Source:      "espn",
		PublishedAt: time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
		Summary:     "report",
		URL:         "https://espn.com/article?id=123",
		Entities:    []string{"chiefs"},
	}
	articles := []Article{
		NewArticle(fe1, "a1", time.Now().UTC().Format(time.RFC3339)),
		NewArticle(fe2, "a2", time.Now().UTC().Format(time.RFC3339)),
	}
	groups, decisions := c.ClusterWithDecisions(articles)
	if len(groups) != 1 {
		t.Fatalf("URL path: expected 1 group, got %d", len(groups))
	}
	// a2 joined a1 -> exactly one decision with reason "url".
	if len(decisions) != 1 {
		t.Fatalf("URL path: expected 1 decision, got %d", len(decisions))
	}
	if decisions[0].Reason != "url" {
		t.Errorf("URL path: reason = %q; want url", decisions[0].Reason)
	}
	if decisions[0].GroupID == "" || decisions[0].ArticleID == "" || decisions[0].SourceID == "" {
		t.Errorf("URL path: decision missing group/article/source id: %+v", decisions[0])
	}
	if decisions[0].EntityAffinity != 1.0 {
		t.Errorf("URL path: entity affinity = %v; want 1.0", decisions[0].EntityAffinity)
	}

	// Scored title path: same entity, near-identical titles, different sources,
	// different URLs.
	fe3 := FeedEntry{
		Title:       "Steelers Draft Linebacker Marcus Lee",
		Source:      "espn",
		PublishedAt: time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC),
		Summary:     "Pittsburgh selected linebacker Marcus Lee in the third round of the draft today.",
		URL:         "https://espn.example/draft-1",
		Entities:    []string{"steelers"},
	}
	fe4 := FeedEntry{
		Title:       "Steelers Draft Linebacker Marcus Lee To Pittsburgh",
		Source:      "fox-sports",
		PublishedAt: time.Date(2024, 1, 2, 11, 0, 0, 0, time.UTC),
		Summary:     "Pittsburgh selected linebacker Marcus Lee in the third round of the draft today.",
		URL:         "https://foxsports.example/draft-2",
		Entities:    []string{"steelers"},
	}
	articles2 := []Article{
		NewArticle(fe3, "a3", time.Now().UTC().Format(time.RFC3339)),
		NewArticle(fe4, "a4", time.Now().UTC().Format(time.RFC3339)),
	}
	_, decisions2 := c.ClusterWithDecisions(articles2)
	if len(decisions2) != 1 {
		t.Fatalf("title path: expected 1 decision, got %d", len(decisions2))
	}
	d := decisions2[0]
	if d.Reason == "" {
		t.Errorf("title path: empty reason")
	}
	if d.TitleScore <= 0 {
		t.Errorf("title path: title score = %v; want > 0", d.TitleScore)
	}
	if d.Threshold != c.SimilarityThreshold {
		t.Errorf("title path: threshold = %v; want %v", d.Threshold, c.SimilarityThreshold)
	}
}

// TestCluster_KeepsDecisions_UrlFastPath ensures the thin Cluster() wrapper
// still works and clusters identically to ClusterWithDecisions.
func TestCluster_KeepsDecisions_UrlFastPath(t *testing.T) {
	c := NewClusterer()
	fe1 := FeedEntry{
		Title:       "Breaking News",
		Source:      "news-com",
		PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		Summary:     "x",
		URL:         "https://news.com/article?id=123",
	}
	fe2 := FeedEntry{
		Title:       "Breaking News Updated",
		Source:      "news-org",
		PublishedAt: time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC),
		Summary:     "x",
		URL:         "https://news.org/article?id=123",
	}
	articles := []Article{
		NewArticle(fe1, "a1", time.Now().UTC().Format(time.RFC3339)),
		NewArticle(fe2, "a2", time.Now().UTC().Format(time.RFC3339)),
	}
	plain := c.Cluster(articles)
	withDec, _ := c.ClusterWithDecisions(articles)
	if len(plain) != len(withDec) {
		t.Fatalf("Cluster and ClusterWithDecisions disagree: %d vs %d", len(plain), len(withDec))
	}
	for i := range plain {
		if plain[i].ID != withDec[i].ID {
			t.Errorf("group %d ID differs between Cluster and ClusterWithDecisions", i)
		}
	}
}

// TestStory_MarshalUnmarshal_WithJoinReasons verifies the round-trip through
// Story JSON, including backward compatibility (old JSON without joinReasons
// unmarshals to an empty slice).
func TestStory_MarshalUnmarshal_WithJoinReasons(t *testing.T) {
	story := Story{
		ID:          "story-1",
		Title:       "t",
		Source:      "s",
		PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		FirstSeenAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC),
		JoinReasons: []JoinDecision{
			{GroupID: "g1", ArticleID: "a1", SourceID: "s", Reason: "url", EntityAffinity: 1.0, Threshold: 1.0, At: "2024-01-01T10:00:00Z"},
		},
	}

	data, err := json.Marshal(story)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Story
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(back.JoinReasons) != 1 || back.JoinReasons[0].Reason != "url" {
		t.Errorf("round-trip JoinReasons = %+v; want 1 url decision", back.JoinReasons)
	}

	// Backward compatibility: old JSON without joinReasons.
	oldJSON := `{"id":"story-2","title":"t","source":"s","publishedAt":"2024-01-01T10:00:00Z","firstSeenAt":"2024-01-01T10:00:00Z","updatedAt":"2024-01-01T10:00:00Z"}`
	var compat Story
	if err := json.Unmarshal([]byte(oldJSON), &compat); err != nil {
		t.Fatalf("unmarshal old json: %v", err)
	}
	if compat.JoinReasons != nil {
		t.Errorf("old JSON JoinReasons = %+v; want nil", compat.JoinReasons)
	}
}

// TestCluster_NilArticles: empty/nil input yields no groups or decisions.
func TestCluster_NilArticles(t *testing.T) {
	c := NewClusterer()
	if groups, decisions := c.ClusterWithDecisions(nil); groups != nil || decisions != nil {
		t.Errorf("nil input: expected nil groups/decisions, got %v / %v", groups, decisions)
	}
}

// TestCluster_MalformedTimes: an article with an unparseable PublishedAt must
// not panic and should form a solo group.
func TestCluster_MalformedTimes(t *testing.T) {
	c := NewClusterer()
	bad := Article{ID: "bad", Title: "No Time", SourceID: "s", PublishedAt: "not-a-time"}
	good := Article{ID: "good", Title: "No Time", SourceID: "s", PublishedAt: "2024-01-01T10:00:00Z"}
	groups, _ := c.ClusterWithDecisions([]Article{bad, good})
	if groups == nil {
		t.Fatalf("malformed time: expected non-nil groups")
	}
}

// TestCluster_EmptyEntityStrings: whitespace-only entities are ignored and do
// not block merging.
func TestCluster_EmptyEntityStrings(t *testing.T) {
	c := NewClusterer()
	a1 := Article{ID: "a1", Title: "Same Title Here", SourceID: "s", PublishedAt: "2024-01-01T10:00:00Z", Entities: []string{"  ", "x"}}
	a2 := Article{ID: "a2", Title: "Same Title Here", SourceID: "s", PublishedAt: "2024-01-01T11:00:00Z"}
	groups, _ := c.ClusterWithDecisions([]Article{a1, a2})
	if len(groups) != 1 {
		t.Errorf("empty-entity merge: expected 1 group, got %d", len(groups))
	}
}

// TestCluster_ConcurrentSafe: concurrent clustering must not race (make test
// runs with -race).
func TestCluster_ConcurrentSafe(t *testing.T) {
	c := NewClusterer()
	articles := make([]Article, 0, 20)
	for i := 0; i < 10; i++ {
		articles = append(articles, Article{
			ID:          string(rune('a'+i)) + "1",
			Title:       "Shared Team Event Title",
			SourceID:    "s",
			PublishedAt: "2024-01-01T10:00:00Z",
			Entities:    []string{"team"},
		})
	}
	done := make(chan bool, 2)
	for g := 0; g < 2; g++ {
		go func() {
			_ = c.Cluster(articles)
			done <- true
		}()
	}
	for g := 0; g < 2; g++ {
		<-done
	}
}
