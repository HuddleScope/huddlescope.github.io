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
