package application

import (
	"context"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// seedTwoSameEventStories inserts two stories with identical titles/entities so
// they cluster into one group deterministically.
func seedTwoSameEventStories(t *testing.T, repo *storage.MemoryRepository, now time.Time) {
	t.Helper()
	seeds := []domain.Story{
		{ID: "seed-1", Title: "Dolphins Sign Wide Receiver", Source: "espn", PublishedAt: now.Add(-1 * time.Hour), Entities: []string{"dolphins"}},
		{ID: "seed-2", Title: "Dolphins Sign Wide Receiver", Source: "fox-sports", PublishedAt: now.Add(-2 * time.Hour), Entities: []string{"dolphins"}},
	}
	for _, s := range seeds {
		if _, err := repo.Upsert(s); err != nil {
			t.Fatalf("seed story %s: %v", s.ID, err)
		}
	}
}

// TestRunGrouping_WithCorrectionRules (AC-3b): a split rule is loaded from the
// rule repository and applied deterministically after clustering; resulting
// stories carry join-reason metadata. Uses a fresh repo per pass so the article
// set (and therefore the group ID the rule references) is identical both times.
func TestRunGrouping_WithCorrectionRules(t *testing.T) {
	now := time.Now().UTC()

	// Pass 1: discover the stable group ID for the two same-event seeds.
	repo1 := storage.NewMemoryRepository()
	seedTwoSameEventStories(t, repo1, now)
	app1 := New(WithRepository(repo1), WithClusterer(domain.NewClusterer()))
	if result := app1.RunGrouping(context.Background()); len(result.Errors) > 0 {
		t.Fatalf("pass1 RunGrouping errors: %v", result.Errors)
	}
	var groupID string
	stories1, err := repo1.List()
	if err != nil {
		t.Fatalf("list pass1: %v", err)
	}
	for _, s := range stories1 {
		if len(s.ArticleIDs) == 2 {
			groupID = s.ID
		}
	}
	if groupID == "" {
		t.Fatalf("expected a 2-article group after pass1")
	}

	// Pass 2: identical seeds + a split rule pulled from the rule repository.
	// Fresh repo so the article set matches pass1 (group ID is stable).
	repo2 := storage.NewMemoryRepository()
	seedTwoSameEventStories(t, repo2, now)
	ruleRepo := storage.NewMemoryRuleRepository()
	rule := domain.CorrectionRule{
		ID:         "rule-split-e2e",
		Kind:       domain.CorrectionSplit,
		StoryID:    groupID,
		ArticleIDs: []string{"seed-2"},
	}
	if err := ruleRepo.SaveRule(rule); err != nil {
		t.Fatalf("SaveRule: %v", err)
	}
	app2 := New(
		WithRepository(repo2),
		WithClusterer(domain.NewClusterer()),
		WithRuleRepository(ruleRepo),
	)
	if result := app2.RunGrouping(context.Background()); len(result.Errors) > 0 {
		t.Fatalf("pass2 RunGrouping errors: %v", result.Errors)
	}

	all, err := repo2.List()
	if err != nil {
		t.Fatalf("list pass2: %v", err)
	}
	var sawSplitOff, sawJoinReasons bool
	for _, s := range all {
		if len(s.ArticleIDs) == 1 && s.ArticleIDs[0] == "seed-2" {
			sawSplitOff = true
		}
		if len(s.JoinReasons) > 0 {
			sawJoinReasons = true
			for _, jd := range s.JoinReasons {
				if jd.Reason == "" || jd.GroupID == "" {
					t.Errorf("join reason missing fields: %+v", jd)
				}
			}
		}
	}
	if !sawSplitOff {
		t.Errorf("split rule did not produce a singleton group for seed-2")
	}
	if !sawJoinReasons {
		t.Errorf("no story carried join-reason metadata after grouping with rules")
	}
}

// TestRunGrouping_WithRuleRepository_MissingReferences: a rule referencing a
// non-existent group is surfaced as an error (no crash, input partition left
// unmodified downstream).
func TestRunGrouping_WithRuleRepository_MissingReferences(t *testing.T) {
	repo := storage.NewMemoryRepository()
	ruleRepo := storage.NewMemoryRuleRepository()

	now := time.Now().UTC()
	if _, err := repo.Upsert(domain.Story{
		ID: "solo-1", Title: "Packers Release Guard", Source: "espn", PublishedAt: now.Add(-1 * time.Hour),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := ruleRepo.SaveRule(domain.CorrectionRule{
		ID: "rule-orphan", Kind: domain.CorrectionSplit, StoryID: "no-such-group", ArticleIDs: []string{"solo-1"},
	}); err != nil {
		t.Fatalf("SaveRule: %v", err)
	}

	app := New(WithRepository(repo), WithClusterer(domain.NewClusterer()), WithRuleRepository(ruleRepo))
	result := app.RunGrouping(context.Background())
	if len(result.Errors) == 0 {
		t.Errorf("expected an error for orphan rule reference")
	}
}
