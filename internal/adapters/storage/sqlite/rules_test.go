package sqlite

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// TestCorrectionRules_Migrate007 verifies the 007 migration creates the
// correction_rules table.
func TestCorrectionRules_Migrate007(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	defer func() { _ = os.Remove(dbPath) }()

	if _, err := Migrate(dbPath); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	repo, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	defer func() { _ = repo.Close() }()

	var n int
	if err := repo.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='correction_rules'`).Scan(&n); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if n != 1 {
		t.Errorf("correction_rules table count = %d; want 1", n)
	}
}

// TestSaveListRules_RoundTrip_Sqlite: save then list rules through the sqlite
// RuleRepository.
func TestSaveListRules_RoundTrip_Sqlite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	defer func() { _ = os.Remove(dbPath) }()

	repo, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	defer func() { _ = repo.Close() }()

	ruleRepo := NewRuleRepository(repo.db)

	rules := []domain.CorrectionRule{
		{
			ID:         "r-split",
			Kind:       domain.CorrectionSplit,
			StoryID:    "story-1",
			ArticleIDs: []string{"a1", "a2"},
			CreatedBy:  "ops",
		},
		{
			ID:          "r-merge",
			Kind:        domain.CorrectionMerge,
			StoryID:     "story-2",
			IntoStoryID: "story-3",
			CreatedBy:   "ops",
		},
	}
	for _, r := range rules {
		if err := ruleRepo.SaveRule(r); err != nil {
			t.Fatalf("SaveRule(%s) error = %v", r.ID, err)
		}
	}

	got, err := ruleRepo.ListRules()
	if err != nil {
		t.Fatalf("ListRules() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListRules() returned %d; want 2", len(got))
	}

	byID := make(map[string]domain.CorrectionRule, len(got))
	for _, r := range got {
		byID[r.ID] = r
	}
	if r, ok := byID["r-split"]; !ok || r.Kind != domain.CorrectionSplit || len(r.ArticleIDs) != 2 || r.CreatedBy != "ops" {
		t.Errorf("r-split round-trip failed: %+v", r)
	}
	if r, ok := byID["r-merge"]; !ok || r.Kind != domain.CorrectionMerge || r.IntoStoryID != "story-3" {
		t.Errorf("r-merge round-trip failed: %+v", r)
	}

	// Invalid rule must be rejected.
	if err := ruleRepo.SaveRule(domain.CorrectionRule{ID: "", Kind: domain.CorrectionSplit}); err == nil {
		t.Errorf("SaveRule(invalid) expected error")
	}
}

// TestRuleRepository_LifecycleKindsRoundTrip (issue #26, AC-2, T2.4):
// archive/close/reactivate/alias rules save and list through sqlite with the
// `kind` text and slug references (carried in story_id / into_story_id columns)
// round-tripping. No schema migration is required.
func TestRuleRepository_LifecycleKindsRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	repo, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	ruleRepo := NewRuleRepository(repo.db)

	rules := []domain.CorrectionRule{
		{ID: "arc-1", Kind: domain.CorrectionArchive, StoryID: "story-a"},
		{ID: "cls-1", Kind: domain.CorrectionClose, StoryID: "story-b"},
		{ID: "react-1", Kind: domain.CorrectionReactivate, StoryID: "story-c"},
		{ID: "alias-1", Kind: domain.CorrectionAlias, StoryID: "story-d", IntoStoryID: "old-slug"},
	}
	for _, r := range rules {
		if err := ruleRepo.SaveRule(r); err != nil {
			t.Fatalf("SaveRule(%s) error = %v", r.ID, err)
		}
	}

	got, err := ruleRepo.ListRules()
	if err != nil {
		t.Fatalf("ListRules() error = %v", err)
	}
	byID := make(map[string]domain.CorrectionRule, len(got))
	for _, r := range got {
		byID[r.ID] = r
	}
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
