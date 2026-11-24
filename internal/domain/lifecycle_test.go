package domain

import (
	"sort"
	"testing"
)

// groupsFromArticles builds a deterministic set of single-article groups from
// the given articles (one group per article).
func lifecycleGroups(t *testing.T, articles ...Article) []StoryGroup {
	t.Helper()
	groups := make([]StoryGroup, 0, len(articles))
	for _, a := range articles {
		groups = append(groups, NewStoryGroup(a))
	}
	return groups
}

// repSlugOf returns the representative slug of the first group in groups.
func repSlugOf(t *testing.T, g StoryGroup) StorySlug {
	t.Helper()
	slug := groupRepSlug(g)
	if slug == "" {
		t.Fatalf("group has no representative slug")
	}
	return slug
}

// TestApplyLifecycle_ArchiveClose sets Status via slug reference.
func TestApplyLifecycle_ArchiveClose(t *testing.T) {
	a1 := Article{ID: "a1", Title: "Bills Sign Tight End", PublishedAt: "2024-01-01T10:00:00Z"}
	a2 := Article{ID: "a2", Title: "Patriots Trade Cornerback", PublishedAt: "2024-01-01T11:00:00Z"}
	groups := lifecycleGroups(t, a1, a2)

	slug1 := repSlugOf(t, groups[0])
	slug2 := repSlugOf(t, groups[1])

	rules := []CorrectionRule{
		{ID: "arc-1", Kind: CorrectionArchive, StoryID: string(slug1)},
		{ID: "cls-1", Kind: CorrectionClose, StoryID: string(slug2)},
	}

	out, err := ApplyLifecycle(groups, rules)
	if err != nil {
		t.Fatalf("ApplyLifecycle error = %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("ApplyLifecycle returned %d groups; want 2", len(out))
	}
	var bySlug = map[StorySlug]StoryStatus{}
	for _, g := range out {
		bySlug[groupRepSlug(g)] = g.Status
	}
	if bySlug[slug1] != StatusArchived {
		t.Errorf("slug1 status = %q; want %q", bySlug[slug1], StatusArchived)
	}
	if bySlug[slug2] != StatusClosed {
		t.Errorf("slug2 status = %q; want %q", bySlug[slug2], StatusClosed)
	}

	// Input partition must be unmodified (Status zero-valued).
	for _, g := range groups {
		if g.Status != "" {
			t.Errorf("input partition mutated: group status = %q; want empty", g.Status)
		}
	}
}

// TestApplyLifecycle_Reactivate restores active status.
func TestApplyLifecycle_Reactivate(t *testing.T) {
	a1 := Article{ID: "a1", Title: "Eagles Release Wide Receiver", PublishedAt: "2024-01-01T10:00:00Z"}
	groups := lifecycleGroups(t, a1)
	slug := repSlugOf(t, groups[0])

	// First archive, then reactivate in a second apply (idempotent semantics
	// across runs: the rule set changes, so the result changes accordingly).
	archived, err := ApplyLifecycle(groups, []CorrectionRule{{ID: "arc", Kind: CorrectionArchive, StoryID: string(slug)}})
	if err != nil {
		t.Fatalf("archive error = %v", err)
	}
	if archived[0].Status != StatusArchived {
		t.Fatalf("status after archive = %q; want %q", archived[0].Status, StatusArchived)
	}

	reactivated, err := ApplyLifecycle(archived, []CorrectionRule{{ID: "react", Kind: CorrectionReactivate, StoryID: string(slug)}})
	if err != nil {
		t.Fatalf("reactivate error = %v", err)
	}
	if reactivated[0].Status != StatusActive {
		t.Errorf("status after reactivate = %q; want %q", reactivated[0].Status, StatusActive)
	}
}

// TestApplyLifecycle_Conflict: conflicting archive+reactivate on one slug is an
// error and the partition is left unmodified.
func TestApplyLifecycle_Conflict(t *testing.T) {
	a1 := Article{ID: "a1", Title: "Ravens Sign Running Back", PublishedAt: "2024-01-01T10:00:00Z"}
	groups := lifecycleGroups(t, a1)
	slug := repSlugOf(t, groups[0])

	rules := []CorrectionRule{
		{ID: "arc", Kind: CorrectionArchive, StoryID: string(slug)},
		{ID: "react", Kind: CorrectionReactivate, StoryID: string(slug)},
	}
	if _, err := ApplyLifecycle(groups, rules); err == nil {
		t.Error("conflicting archive+reactivate: expected error")
	}
	// archive+close is also a conflict (different terminal states).
	rules2 := []CorrectionRule{
		{ID: "arc", Kind: CorrectionArchive, StoryID: string(slug)},
		{ID: "cls", Kind: CorrectionClose, StoryID: string(slug)},
	}
	if _, err := ApplyLifecycle(groups, rules2); err == nil {
		t.Error("conflicting archive+close: expected error")
	}
	// Duplicate same-state is NOT a conflict (idempotent).
	rules3 := []CorrectionRule{
		{ID: "arc-a", Kind: CorrectionArchive, StoryID: string(slug)},
		{ID: "arc-b", Kind: CorrectionArchive, StoryID: string(slug)},
	}
	out, err := ApplyLifecycle(groups, rules3)
	if err != nil {
		t.Fatalf("duplicate archive should not conflict: %v", err)
	}
	if out[0].Status != StatusArchived {
		t.Errorf("status = %q; want %q", out[0].Status, StatusArchived)
	}
}

// TestApplyLifecycle_UnknownSlug: a lifecycle rule referencing an unknown slug
// is an error and the partition is left unmodified.
func TestApplyLifecycle_UnknownSlug(t *testing.T) {
	a1 := Article{ID: "a1", Title: "Bengals Trade Guard", PublishedAt: "2024-01-01T10:00:00Z"}
	groups := lifecycleGroups(t, a1)
	origSize := len(groups)

	rule := CorrectionRule{ID: "arc-orphan", Kind: CorrectionArchive, StoryID: "no-such-story"}
	if _, err := ApplyLifecycle(groups, []CorrectionRule{rule}); err == nil {
		t.Error("archive with unknown slug: expected error")
	}
	if len(groups) != origSize {
		t.Errorf("input partition mutated: len=%d; want %d", len(groups), origSize)
	}
}

// TestApplyLifecycle_Idempotent: applying the same lifecycle rule set twice
// yields the same partition (same statuses and alias slugs).
func TestApplyLifecycle_Idempotent(t *testing.T) {
	a1 := Article{ID: "a1", Title: "Dolphins Sign Defensive End", PublishedAt: "2024-01-01T10:00:00Z"}
	a2 := Article{ID: "a2", Title: "Jets Sign Safety", PublishedAt: "2024-01-01T11:00:00Z"}
	groups := lifecycleGroups(t, a1, a2)
	slug1 := repSlugOf(t, groups[0])
	slug2 := repSlugOf(t, groups[1])

	rules := []CorrectionRule{
		{ID: "arc", Kind: CorrectionArchive, StoryID: string(slug1)},
		{ID: "alias", Kind: CorrectionAlias, StoryID: string(slug1), IntoStoryID: string(slug2)},
	}

	first, err := ApplyLifecycle(groups, rules)
	if err != nil {
		t.Fatalf("first apply error = %v", err)
	}
	second, err := ApplyLifecycle(first, rules)
	if err != nil {
		t.Fatalf("second apply error = %v", err)
	}

	statuses := func(gs []StoryGroup) map[StorySlug]StoryStatus {
		m := make(map[StorySlug]StoryStatus)
		for _, g := range gs {
			m[groupRepSlug(g)] = g.Status
		}
		return m
	}
	aliasSlugs := func(gs []StoryGroup) map[StorySlug][]StorySlug {
		m := make(map[StorySlug][]StorySlug)
		for _, g := range gs {
			s := groupRepSlug(g)
			for _, a := range g.Aliases {
				m[s] = append(m[s], a.Slug)
			}
		}
		return m
	}
	if statuses(first) != nil && statuses(second) != nil {
		s1, s2 := statuses(first), statuses(second)
		for k, v := range s1 {
			if s2[k] != v {
				t.Errorf("idempotency violated: slug %q status %q vs %q", k, v, s2[k])
			}
		}
	}
	// Alias lists must be identical (order + content).
	a1first := aliasSlugs(first)[slug1]
	a1second := aliasSlugs(second)[slug1]
	sort.Slice(a1first, func(i, j int) bool { return a1first[i] < a1first[j] })
	sort.Slice(a1second, func(i, j int) bool { return a1second[i] < a1second[j] })
	if len(a1first) != len(a1second) {
		t.Fatalf("alias count differs: %d vs %d", len(a1first), len(a1second))
	}
	for i := range a1first {
		if a1first[i] != a1second[i] {
			t.Fatalf("alias %d differs: %q vs %q", i, a1first[i], a1second[i])
		}
	}
}

// TestApplyLifecycle_AliasAppends records an explicit alias on the target group.
func TestApplyLifecycle_AliasAppends(t *testing.T) {
	a1 := Article{ID: "a1", Title: "Chiefs Sign Receiver", PublishedAt: "2024-01-01T10:00:00Z"}
	a2 := Article{ID: "a2", Title: "Raiders Trade Tight End", PublishedAt: "2024-01-01T11:00:00Z"}
	groups := lifecycleGroups(t, a1, a2)
	slug1 := repSlugOf(t, groups[0])
	slug2 := repSlugOf(t, groups[1])

	rules := []CorrectionRule{
		{ID: "alias-1", Kind: CorrectionAlias, StoryID: string(slug1), IntoStoryID: string(slug2)},
	}
	out, err := ApplyLifecycle(groups, rules)
	if err != nil {
		t.Fatalf("ApplyLifecycle alias error = %v", err)
	}
	var target *StoryGroup
	for i := range out {
		if groupRepSlug(out[i]) == slug1 {
			target = &out[i]
		}
	}
	if target == nil {
		t.Fatalf("target group not found")
	}
	if len(target.Aliases) != 1 {
		t.Fatalf("target aliases = %d; want 1", len(target.Aliases))
	}
	if target.Aliases[0].Slug != slug2 {
		t.Errorf("alias slug = %q; want %q", target.Aliases[0].Slug, slug2)
	}
}

// TestApplyLifecycle_AliasSelfDeduped: an alias rule whose slug equals the
// target's own representative slug is dropped (representative-earlier-than-target
// case; the merged page takes over the source's URL).
func TestApplyLifecycle_AliasSelfDeduped(t *testing.T) {
	a1 := Article{ID: "a1", Title: "Buccaneers Sign Linebacker", PublishedAt: "2024-01-01T10:00:00Z"}
	groups := lifecycleGroups(t, a1)
	slug1 := repSlugOf(t, groups[0])

	rules := []CorrectionRule{
		{ID: "alias-self", Kind: CorrectionAlias, StoryID: string(slug1), IntoStoryID: string(slug1)},
	}
	// Validate should reject self-alias at intrinsic validation time.
	if err := rules[0].Validate(); err == nil {
		t.Error("self-alias rule: expected Validate error")
	}
}
