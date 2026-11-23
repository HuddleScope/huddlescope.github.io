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
