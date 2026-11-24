package storage

import (
	"testing"

	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// TestMemoryRuleRepository_RoundTrip: save/list through the in-memory rule repo.
func TestMemoryRuleRepository_RoundTrip(t *testing.T) {
	repo := NewMemoryRuleRepository()

	rules := []domain.CorrectionRule{
		{ID: "r1", Kind: domain.CorrectionSplit, StoryID: "s1", ArticleIDs: []string{"a1"}},
		{ID: "r2", Kind: domain.CorrectionMerge, StoryID: "s2", IntoStoryID: "s3"},
	}
	for _, r := range rules {
		if err := repo.SaveRule(r); err != nil {
			t.Fatalf("SaveRule(%s) error = %v", r.ID, err)
		}
	}

	got, err := repo.ListRules()
	if err != nil {
		t.Fatalf("ListRules() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListRules() returned %d; want 2", len(got))
	}
	// Deterministic ID-sorted order.
	if got[0].ID != "r1" || got[1].ID != "r2" {
		t.Errorf("ListRules() order = %v, %v; want r1, r2", got[0].ID, got[1].ID)
	}

	// Invalid rule rejected.
	if err := repo.SaveRule(domain.CorrectionRule{ID: "", Kind: domain.CorrectionSplit}); err == nil {
		t.Errorf("SaveRule(invalid) expected error")
	}
}

// TestNewRuleRepository_Memory: factory returns an in-memory rule repo.
func TestNewRuleRepository_Memory(t *testing.T) {
	repo, err := NewRuleRepository(config.StorageConfig{Backend: "memory"})
	if err != nil {
		t.Fatalf("NewRuleRepository(memory) error = %v", err)
	}
	if repo == nil {
		t.Fatal("NewRuleRepository(memory) returned nil")
	}
}

// TestMemoryRuleRepository_LifecycleKindsRoundTrip (issue #26, AC-2, T2.4):
// archive/close/reactivate/alias rules save and list through the in-memory rule
// repo with the `kind` text round-tripping (no schema migration needed).
func TestMemoryRuleRepository_LifecycleKindsRoundTrip(t *testing.T) {
	repo := NewMemoryRuleRepository()

	rules := []domain.CorrectionRule{
		{ID: "arc-1", Kind: domain.CorrectionArchive, StoryID: "story-a"},
		{ID: "cls-1", Kind: domain.CorrectionClose, StoryID: "story-b"},
		{ID: "react-1", Kind: domain.CorrectionReactivate, StoryID: "story-c"},
		{ID: "alias-1", Kind: domain.CorrectionAlias, StoryID: "story-d", IntoStoryID: "old-slug"},
	}
	for _, r := range rules {
		if err := repo.SaveRule(r); err != nil {
			t.Fatalf("SaveRule(%s) error = %v", r.ID, err)
		}
	}

	got, err := repo.ListRules()
	if err != nil {
		t.Fatalf("ListRules() error = %v", err)
	}
	byID := make(map[string]domain.CorrectionRule, len(got))
	for _, r := range got {
		byID[r.ID] = r
	}
	// Assert kind text round-trips for each lifecycle kind.
	if r, ok := byID["arc-1"]; !ok || r.Kind != domain.CorrectionArchive || r.StoryID != "story-a" {
		t.Errorf("arc-1 round-trip failed: %+v", r)
	}
	if r, ok := byID["cls-1"]; !ok || r.Kind != domain.CorrectionClose || r.StoryID != "story-b" {
		t.Errorf("cls-1 round-trip failed: %+v", r)
	}
	if r, ok := byID["react-1"]; !ok || r.Kind != domain.CorrectionReactivate || r.StoryID != "story-c" {
		t.Errorf("react-1 round-trip failed: %+v", r)
	}
	if r, ok := byID["alias-1"]; !ok || r.Kind != domain.CorrectionAlias || r.StoryID != "story-d" || r.IntoStoryID != "old-slug" {
		t.Errorf("alias-1 round-trip failed: %+v", r)
	}
}
