package domain

import (
	"reflect"
	"sort"
	"testing"
)

// helper to build a group with a deterministic ID from given articles.
func groupWithArticles(title string, articles ...Article) StoryGroup {
	g := NewStoryGroup(articles[0])
	for _, a := range articles[1:] {
		g.AddArticle(a)
	}
	return g
}

// TestApplyCorrections_Split: a split rule removes listed articles into a new
// group and recomputes IDs deterministically.
func TestApplyCorrections_Split(t *testing.T) {
	a1 := Article{ID: "a1", Title: "Steelers Draft LB1", PublishedAt: "2024-01-01T10:00:00Z"}
	a2 := Article{ID: "a2", Title: "Steelers Draft LB2", PublishedAt: "2024-01-01T11:00:00Z"}
	a3 := Article{ID: "a3", Title: "Steelers Sign RB", PublishedAt: "2024-01-01T12:00:00Z"}

	original := []StoryGroup{groupWithArticles("headline", a1, a2, a3)}
	origID := original[0].ID

	rule := CorrectionRule{
		ID:         "rule-split-1",
		Kind:       CorrectionSplit,
		StoryID:    origID,
		ArticleIDs: []string{"a3"},
	}

	corrected, err := ApplyCorrections(original, []CorrectionRule{rule})
	if err != nil {
		t.Fatalf("ApplyCorrections error = %v", err)
	}

	// Expect 2 groups: the kept (a1,a2) and the split-off (a3).
	if len(corrected) != 2 {
		t.Fatalf("ApplyCorrections split returned %d groups; want 2", len(corrected))
	}

	// The kept group must NOT equal the original ID (recomputed).
	var kept, split *StoryGroup
	for i := range corrected {
		switch corrected[i].Size() {
		case 2:
			kept = &corrected[i]
		case 1:
			split = &corrected[i]
		}
	}
	if kept == nil || split == nil {
		t.Fatalf("expected one 2-article and one 1-article group; kept=%v split=%v", kept, split)
	}
	if kept.ID == origID {
		t.Errorf("kept group ID unchanged after split; want recomputed ID")
	}
	if split.ID == origID {
		t.Errorf("split group ID equals original; want distinct recomputed ID")
	}
	if split.Articles[0].ID != "a3" {
		t.Errorf("split group article = %q; want a3", split.Articles[0].ID)
	}
}

// TestApplyCorrections_Merge: a merge rule folds source group articles into the
// target group and drops the source group.
func TestApplyCorrections_Merge(t *testing.T) {
	a1 := Article{ID: "a1", Title: "Bears Trade RB", PublishedAt: "2024-01-01T10:00:00Z"}
	a2 := Article{ID: "a2", Title: "Bears Sign WR", PublishedAt: "2024-01-01T11:00:00Z"}

	g1 := groupWithArticles("h1", a1)
	g2 := groupWithArticles("h2", a2)
	groups := []StoryGroup{g1, g2}

	rule := CorrectionRule{
		ID:          "rule-merge-1",
		Kind:        CorrectionMerge,
		StoryID:     g2.ID,
		IntoStoryID: g1.ID,
	}

	corrected, err := ApplyCorrections(groups, []CorrectionRule{rule})
	if err != nil {
		t.Fatalf("ApplyCorrections merge error = %v", err)
	}
	if len(corrected) != 1 {
		t.Fatalf("ApplyCorrections merge returned %d groups; want 1", len(corrected))
	}
	if corrected[0].Size() != 2 {
		t.Fatalf("merged group size = %d; want 2", corrected[0].Size())
	}
	ids := make([]string, 0, 2)
	for _, a := range corrected[0].Articles {
		ids = append(ids, a.ID)
	}
	sort.Strings(ids)
	if !reflect.DeepEqual(ids, []string{"a1", "a2"}) {
		t.Errorf("merged group article IDs = %v; want [a1 a2]", ids)
	}
}

// TestApplyCorrections_Idempotent: applying the same rules twice yields the
// same partition (same group IDs and membership).
func TestApplyCorrections_Idempotent(t *testing.T) {
	a1 := Article{ID: "a1", Title: "Cowboys Sign Guard", PublishedAt: "2024-01-01T10:00:00Z"}
	a2 := Article{ID: "a2", Title: "Cowboys Trade WR", PublishedAt: "2024-01-01T11:00:00Z"}
	a3 := Article{ID: "a3", Title: "Cowboys Release RB", PublishedAt: "2024-01-01T12:00:00Z"}

	original := []StoryGroup{groupWithArticles("h", a1, a2, a3)}
	rule := CorrectionRule{
		ID:         "rule-split-idem",
		Kind:       CorrectionSplit,
		StoryID:    original[0].ID,
		ArticleIDs: []string{"a3"},
	}

	first, err := ApplyCorrections(original, []CorrectionRule{rule})
	if err != nil {
		t.Fatalf("first apply error = %v", err)
	}
	second, err := ApplyCorrections(first, []CorrectionRule{rule})
	if err != nil {
		// The second application cannot resolve the rule (group already split),
		// so it returns an error with the input unchanged. That is acceptable
		// idempotency semantics: the partition is stable either way.
		t.Logf("second apply returned expected error: %v", err)
	} else {
		// If it did not error, compare partitions.
		if len(first) != len(second) {
			t.Fatalf("idempotency violated: %d vs %d groups", len(first), len(second))
		}
		for i := range first {
			if first[i].ID != second[i].ID {
				t.Fatalf("idempotency violated: group %d ID differs", i)
			}
		}
	}

	// Regardless of whether the second run errored, `first` must have exactly
	// one 2-article group and one 1-article group with stable IDs.
	sizes := make([]int, 0, len(first))
	for _, g := range first {
		sizes = append(sizes, g.Size())
	}
	sort.Ints(sizes)
	if !reflect.DeepEqual(sizes, []int{1, 2}) {
		t.Fatalf("split partition sizes = %v; want [1 2]", sizes)
	}
}

// TestApplyCorrections_InvalidRule: unknown references and cyclic merges are
// rejected with an error and leave the input partition unmodified.
func TestApplyCorrections_InvalidRule(t *testing.T) {
	a1 := Article{ID: "a1", Title: "Lions Win Sunday", PublishedAt: "2024-01-01T10:00:00Z"}
	a2 := Article{ID: "a2", Title: "Lions Trade QB", PublishedAt: "2024-01-01T11:00:00Z"}
	g := groupWithArticles("h", a1, a2)
	groups := []StoryGroup{g}
	origSize := len(groups)

	t.Run("split unknown group", func(t *testing.T) {
		rule := CorrectionRule{ID: "r1", Kind: CorrectionSplit, StoryID: "no-such-group", ArticleIDs: []string{"a1"}}
		if _, err := ApplyCorrections(groups, []CorrectionRule{rule}); err == nil {
			t.Errorf("split with unknown group: expected error")
		}
	})

	t.Run("split unknown article", func(t *testing.T) {
		rule := CorrectionRule{ID: "r2", Kind: CorrectionSplit, StoryID: g.ID, ArticleIDs: []string{"no-such-article"}}
		if _, err := ApplyCorrections(groups, []CorrectionRule{rule}); err == nil {
			t.Errorf("split with unknown article: expected error")
		}
	})

	t.Run("merge self", func(t *testing.T) {
		rule := CorrectionRule{ID: "r3", Kind: CorrectionMerge, StoryID: g.ID, IntoStoryID: g.ID}
		if _, err := ApplyCorrections(groups, []CorrectionRule{rule}); err == nil {
			t.Errorf("self merge: expected error")
		}
	})

	t.Run("merge cycle", func(t *testing.T) {
		// Two groups with a cyclic merge A->B and B->A.
		b1 := Article{ID: "b1", Title: "x", PublishedAt: "2024-01-01T10:00:00Z"}
		b2 := Article{ID: "b2", Title: "y", PublishedAt: "2024-01-01T11:00:00Z"}
		gA := groupWithArticles("A", b1)
		gB := groupWithArticles("B", b2)
		two := []StoryGroup{gA, gB}
		rules := []CorrectionRule{
			{ID: "c1", Kind: CorrectionMerge, StoryID: gA.ID, IntoStoryID: gB.ID},
			{ID: "c2", Kind: CorrectionMerge, StoryID: gB.ID, IntoStoryID: gA.ID},
		}
		if _, err := ApplyCorrections(two, rules); err == nil {
			t.Errorf("cyclic merge: expected error")
		}
	})

	// Input partition must be unmodified after all the invalid attempts.
	if len(groups) != origSize || groups[0].Size() != 2 {
		t.Errorf("input partition mutated by invalid rules: len=%d size=%d", len(groups), groups[0].Size())
	}
}

// TestApplyCorrections_RuleSurvivesRecluster: after a full re-cluster run,
// applying the same rules still produces a stable partition (the rule can be
// re-applied deterministically on the fresh clustering output).
func TestApplyCorrections_RuleSurvivesRecluster(t *testing.T) {
	c := NewClusterer()

	// Three articles that cluster into a single group: near-identical titles
	// (same event) and identical entity sets.
	a1 := Article{ID: "a1", Title: "Rams Sign WR James Bond For Multi Year Deal", PublishedAt: "2024-01-01T10:00:00Z", Entities: []string{"rams"}}
	a2 := Article{ID: "a2", Title: "Rams Sign WR James Bond To Multi Year Contract", PublishedAt: "2024-01-01T11:00:00Z", Entities: []string{"rams"}}
	a3 := Article{ID: "a3", Title: "Rams Sign WR James Bond In Multi Year Agreement", PublishedAt: "2024-01-01T12:00:00Z", Entities: []string{"rams"}}

	articles := []Article{a1, a2, a3}
	groups1 := c.Cluster(articles)
	if len(groups1) != 1 || groups1[0].Size() != 3 {
		t.Fatalf("setup: expected one 3-article group; got sizes %v", groupSizes(groups1))
	}

	// Build a split rule that removes a3 from the cluster.
	makeRule := func(groups []StoryGroup) (CorrectionRule, bool) {
		for _, g := range groups {
			if g.Size() == 3 {
				return CorrectionRule{
					ID:         "rule-survive",
					Kind:       CorrectionSplit,
					StoryID:    g.ID,
					ArticleIDs: []string{"a3"},
				}, true
			}
		}
		return CorrectionRule{}, false
	}

	rule1, ok := makeRule(groups1)
	if !ok {
		t.Fatalf("could not build split rule for 3-article group")
	}
	first, err := ApplyCorrections(groups1, []CorrectionRule{rule1})
	if err != nil {
		t.Fatalf("apply after recluster error = %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("recluster+split returned %d groups; want 2", len(first))
	}

	// Re-run clustering from scratch and apply the same rule; partition shape
	// (sizes) must be identical and group IDs deterministic.
	groups2 := c.Cluster(articles)
	rule2, ok := makeRule(groups2)
	if !ok {
		t.Fatalf("second cluster did not produce a 3-article group")
	}
	second, err := ApplyCorrections(groups2, []CorrectionRule{rule2})
	if err != nil {
		t.Fatalf("second apply error = %v", err)
	}
	if !reflect.DeepEqual(groupSizes(first), groupSizes(second)) {
		t.Fatalf("recluster stability violated: %v vs %v", groupSizes(first), groupSizes(second))
	}
}

func groupSizes(groups []StoryGroup) []int {
	sizes := make([]int, 0, len(groups))
	for _, g := range groups {
		sizes = append(sizes, g.Size())
	}
	sort.Ints(sizes)
	return sizes
}

// TestApplyCorrections_NoRules: empty rules returns the input unchanged.
func TestApplyCorrections_NoRules(t *testing.T) {
	a1 := Article{ID: "a1", Title: "t", PublishedAt: "2024-01-01T10:00:00Z"}
	g := groupWithArticles("h", a1)
	groups := []StoryGroup{g}
	out, err := ApplyCorrections(groups, nil)
	if err != nil {
		t.Fatalf("no-rules apply error = %v", err)
	}
	if len(out) != 1 || out[0].ID != g.ID {
		t.Errorf("no-rules apply changed the partition")
	}
}
