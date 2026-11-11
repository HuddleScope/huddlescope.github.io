package domain

import (
	"testing"
	"time"
)

func TestClassifyEvidence(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected EvidenceLevel
	}{
		{"confirmed statement", "The team officially confirmed the signing today", EvidenceConfirmed},
		{"confirmed report", "Confirmed report: player signed contract", EvidenceConfirmed},
		{"verified news", "Verified news source reported the trade", EvidenceConfirmed},
		{"official statement", "Official statement released about the injury", EvidenceConfirmed},
		{"rumor", "Sources say the team is considering a trade", EvidenceUnverified},
		{"alleged", "The player allegedly violated team rules", EvidenceUnverified},
		{"unconfirmed reports", "Unconfirmed reports of a trade", EvidenceUnverified},
		{"purportedly", "The move purportedly happened yesterday", EvidenceUnverified},
		{"speculative", "Speculative reports suggest a trade", EvidenceUnverified},
		{"unknown plain", "The team plays today", EvidenceUnknown},
		{"empty", "", EvidenceUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyEvidence(tt.text)
			if result != tt.expected {
				t.Errorf("ClassifyEvidence(%q) = %v, want %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestScorePolicy_Validate(t *testing.T) {
	tests := []struct {
		name    string
		policy  ScorePolicy
		wantErr bool
	}{
		{"valid weights", ScorePolicy{
			FreshnessWeight:              0.4,
			SourceDiversityWeight:        0.3,
			ImportanceWeight:             0.2,
			MomentumWeight:               0.1,
			MaxSources:                   10,
			MomentumCap:                  10,
			MomentumWindow:               24 * time.Hour,
			EvidenceMultiplierConfirmed:  1.0,
			EvidenceMultiplierUnverified: 0.8,
			EvidenceMultiplierUnknown:    1.0,
		}, false},
		{"weights sum to 1.1", ScorePolicy{
			FreshnessWeight:       0.45,
			SourceDiversityWeight: 0.3,
			ImportanceWeight:      0.2,
			MomentumWeight:        0.15,
			MaxSources:            10,
			MomentumCap:           10,
			MomentumWindow:        24 * time.Hour,
		}, true},
		{"weights sum to 0.9", ScorePolicy{
			FreshnessWeight:       0.35,
			SourceDiversityWeight: 0.3,
			ImportanceWeight:      0.2,
			MomentumWeight:        0.05,
			MaxSources:            10,
			MomentumCap:           10,
			MomentumWindow:        24 * time.Hour,
		}, true},
		{"negative freshness weight", ScorePolicy{
			FreshnessWeight:       -0.1,
			SourceDiversityWeight: 0.3,
			ImportanceWeight:      0.2,
			MomentumWeight:        0.4,
			MaxSources:            10,
			MomentumCap:           10,
			MomentumWindow:        24 * time.Hour,
		}, true},
		{"negative max sources", ScorePolicy{
			FreshnessWeight:       0.4,
			SourceDiversityWeight: 0.3,
			ImportanceWeight:      0.2,
			MomentumWeight:        0.1,
			MaxSources:            0,
			MomentumCap:           10,
			MomentumWindow:        24 * time.Hour,
		}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.policy.Validate()
			if tt.wantErr && err == nil {
				t.Errorf("Validate() expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestExtractSourceBreakdown(t *testing.T) {
	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	window := 24 * time.Hour

	// Articles within window (4 total)
	articles := []Article{
		{ID: "a1", SourceID: "source-a", PublishedAt: now.Add(-1 * time.Hour).Format("2006-01-02T15:04:05Z")},
		{ID: "a2", SourceID: "source-a", PublishedAt: now.Add(-2 * time.Hour).Format("2006-01-02T15:04:05Z")},
		{ID: "a3", SourceID: "source-b", PublishedAt: now.Add(-3 * time.Hour).Format("2006-01-02T15:04:05Z")},
		{ID: "a4", SourceID: "source-c", PublishedAt: now.Add(-5 * time.Hour).Format("2006-01-02T15:04:05Z")},
	}

	// Article outside window (should be excluded)
	articles = append(articles, Article{
		ID: "a5", SourceID: "source-d", PublishedAt: now.Add(-48 * time.Hour).Format("2006-01-02T15:04:05Z"),
	})

	group := StoryGroup{
		ID:               "test-group",
		Articles:         articles,
		FirstPublishedAt: now.Add(-5 * time.Hour),
	}

	breakdown := ExtractSourceBreakdown(group, now, window)

	if breakdown.TotalArticles != 4 {
		t.Errorf("TotalArticles = %d, want 4", breakdown.TotalArticles)
	}
	if breakdown.UniqueSources != 3 {
		t.Errorf("UniqueSources = %d, want 3", breakdown.UniqueSources)
	}
}

func TestScorer_ComputeFreshnessScore(t *testing.T) {
	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	s := NewScorer().WithTime(now)

	// Fresh (0h old): score = 100 * 1/(1+0) = 100
	fresh := s.computeFreshnessScore(now)
	if fresh != 100 {
		t.Errorf("Fresh score = %.2f, want 100", fresh)
	}

	// Half-life (12h old): score = 100 * 1/(1+1) = 50
	half := s.computeFreshnessScore(now.Add(-12 * time.Hour))
	if half != 50 {
		t.Errorf("Half-life score = %.2f, want 50", half)
	}

	// Very old (48h): score = 100 * 1/(1+4) = 20
	old := s.computeFreshnessScore(now.Add(-48 * time.Hour))
	if old != 20 {
		t.Errorf("Old score = %.2f, want 20", old)
	}

	// Zero time (fallback)
	zero := s.computeFreshnessScore(time.Time{})
	if zero != 50 {
		t.Errorf("Zero time score = %.2f, want 50", zero)
	}
}

func TestScorer_ComputeSourceDiversityScore(t *testing.T) {
	s := NewScorer()

	// 1 source
	b1 := SourceBreakdown{TotalArticles: 5, UniqueSources: 1}
	score1 := s.computeSourceDiversityScore(b1)
	expected1 := 100.0 / float64(s.Policy.MaxSources) // 10
	if score1 < 8 || score1 > 12 {
		t.Errorf("1 source score = %.2f, want ~%.2f", score1, expected1)
	}

	// Max sources
	bMax := SourceBreakdown{TotalArticles: 100, UniqueSources: 10}
	scoreMax := s.computeSourceDiversityScore(bMax)
	if scoreMax < 95 {
		t.Errorf("Max sources score = %.2f, want ~100", scoreMax)
	}

	// Over max (capped)
	bOver := SourceBreakdown{TotalArticles: 200, UniqueSources: 20}
	scoreOver := s.computeSourceDiversityScore(bOver)
	if scoreOver < 95 {
		t.Errorf("Over max score = %.2f, want capped ~100", scoreOver)
	}
}

func TestScorer_ComputeImportanceScore(t *testing.T) {
	s := NewScorer()

	// No entities (fallback score)
	single := StoryGroup{
		ID:       "single",
		Articles: []Article{{Entities: []string{}}},
	}
	score := s.computeImportanceScore(single)
	if score != 50 {
		t.Errorf("No entities score = %.2f, want 50", score)
	}

	// 5+ entities
	five := StoryGroup{
		ID: "five",
		Articles: []Article{
			{Entities: []string{"ne", "nyg", "phi", "dal", "gb"}},
		},
	}
	score5 := s.computeImportanceScore(five)
	if score5 != 100 {
		t.Errorf("5 entities score = %.2f, want 100", score5)
	}
}

func TestScorer_ComputeMomentumScore(t *testing.T) {
	s := NewScorer()

	// Below cap
	b1 := SourceBreakdown{TotalArticles: 3}
	score1 := s.computeMomentumScore(b1)
	expected1 := 30.0 // 3/10 * 100
	if score1 != expected1 {
		t.Errorf("3 articles score = %.2f, want %.2f", score1, expected1)
	}

	// At cap
	b10 := SourceBreakdown{TotalArticles: 10}
	score10 := s.computeMomentumScore(b10)
	if score10 != 100 {
		t.Errorf("10 articles score = %.2f, want 100", score10)
	}

	// Over cap (capped)
	b20 := SourceBreakdown{TotalArticles: 20}
	score20 := s.computeMomentumScore(b20)
	if score20 != 100 {
		t.Errorf("20 articles score = %.2f, want capped 100", score20)
	}
}

func TestScorer_Score(t *testing.T) {
	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	s := NewScorer().WithTime(now)

	group := StoryGroup{
		ID:               "test",
		Headline:         "Trade rumors swirl around star player",
		Articles:         []Article{{SourceID: "espn", Title: "Trade rumors", Summary: "Sources say a deal is near", PublishedAt: now.Add(-2 * time.Hour).Format("2006-01-02T15:04:05Z")}},
		FirstPublishedAt: now.Add(-2 * time.Hour),
		LastPublishedAt:  now.Add(-2 * time.Hour),
	}

	breakdown := s.Score(group)

	// Check all scores are in valid range
	if breakdown.TotalScore < 0 || breakdown.TotalScore > 100 {
		t.Errorf("TotalScore = %.2f, want [0,100]", breakdown.TotalScore)
	}
	if breakdown.FreshnessScore < 0 || breakdown.FreshnessScore > 100 {
		t.Errorf("FreshnessScore = %.2f, want [0,100]", breakdown.FreshnessScore)
	}
	if breakdown.SourceDiversityScore < 0 || breakdown.SourceDiversityScore > 100 {
		t.Errorf("SourceDiversityScore = %.2f, want [0,100]", breakdown.SourceDiversityScore)
	}
	if breakdown.ImportanceScore < 0 || breakdown.ImportanceScore > 100 {
		t.Errorf("ImportanceScore = %.2f, want [0,100]", breakdown.ImportanceScore)
	}
	if breakdown.MomentumScore < 0 || breakdown.MomentumScore > 100 {
		t.Errorf("MomentumScore = %.2f, want [0,100]", breakdown.MomentumScore)
	}

	// Check evidence level (unverified keywords present)
	if breakdown.EvidenceLevel != EvidenceUnverified {
		t.Errorf("EvidenceLevel = %v, want unverified", breakdown.EvidenceLevel)
	}

	// Check notes contain expected info
	foundEvidence := false
	foundSources := false
	foundMomentum := false
	for _, n := range breakdown.Notes {
		if n == "unverified claims: information should be treated with caution" {
			foundEvidence = true
		}
		if contains(n, "unique sources") {
			foundSources = true
		}
		if contains(n, "momentum") {
			foundMomentum = true
		}
	}
	if !foundEvidence {
		t.Errorf("Notes missing evidence caution: %v", breakdown.Notes)
	}
	if !foundSources {
		t.Errorf("Notes missing sources: %v", breakdown.Notes)
	}
	if !foundMomentum {
		t.Errorf("Notes missing momentum: %v", breakdown.Notes)
	}

	// Idempotency: score same group twice
	breakdown2 := s.Score(group)
	if breakdown.TotalScore != breakdown2.TotalScore {
		t.Errorf("Idempotency failed: %v != %v", breakdown.TotalScore, breakdown2.TotalScore)
	}
}

func TestScorer_Rank(t *testing.T) {
	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	s := NewScorer().WithTime(now)

	// Higher score first
	high := StoryGroup{ID: "high", Articles: []Article{{PublishedAt: now.Format("2006-01-02T15:04:05Z")}}}
	low := StoryGroup{ID: "low", Articles: []Article{{PublishedAt: now.Add(-48 * time.Hour).Format("2006-01-02T15:04:05Z")}}}

	ranked := s.Rank([]StoryGroup{low, high})
	if len(ranked) != 2 {
		t.Fatalf("Expected 2 items, got %d", len(ranked))
	}
	if ranked[0].ID != "high" {
		t.Errorf("First item = %s, want high", ranked[0].ID)
	}
	if ranked[1].ID != "low" {
		t.Errorf("Second item = %s, want low", ranked[1].ID)
	}

	// Tie-breaker: newer firstPublishedAt
	high1 := StoryGroup{ID: "high1", Articles: []Article{{PublishedAt: now.Add(-1 * time.Hour).Format("2006-01-02T15:04:05Z")}}}
	high2 := StoryGroup{ID: "high2", Articles: []Article{{PublishedAt: now.Add(-2 * time.Hour).Format("2006-01-02T15:04:05Z")}}}

	ranked2 := s.Rank([]StoryGroup{high2, high1})
	if ranked2[0].ID != "high1" {
		t.Errorf("Tie-breaker first = %s, want high1 (newer)", ranked2[0].ID)
	}

	// ID tie-breaker when same score and time
	tie1 := StoryGroup{ID: "aaa", Articles: []Article{{PublishedAt: now.Format("2006-01-02T15:04:05Z")}}}
	tie2 := StoryGroup{ID: "bbb", Articles: []Article{{PublishedAt: now.Format("2006-01-02T15:04:05Z")}}}

	ranked3 := s.Rank([]StoryGroup{tie2, tie1})
	if ranked3[0].ID != "aaa" {
		t.Errorf("ID tie-breaker first = %s, want aaa", ranked3[0].ID)
	}
}

func TestScorer_SelectTop(t *testing.T) {
	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	s := NewScorer().WithTime(now)

	groups := []StoryGroup{
		{ID: "g1", Articles: []Article{{PublishedAt: now.Format("2006-01-02T15:04:05Z")}}},
		{ID: "g2", Articles: []Article{{PublishedAt: now.Add(-1 * time.Hour).Format("2006-01-02T15:04:05Z")}}},
		{ID: "g3", Articles: []Article{{PublishedAt: now.Add(-2 * time.Hour).Format("2006-01-02T15:04:05Z")}}},
	}

	top1 := s.SelectTop(groups, 1)
	if len(top1) != 1 || top1[0].ID != "g1" {
		t.Errorf("SelectTop(1) = %v, want [g1]", top1)
	}

	top2 := s.SelectTop(groups, 2)
	if len(top2) != 2 || top2[0].ID != "g1" || top2[1].ID != "g2" {
		t.Errorf("SelectTop(2) = %v, want [g1, g2]", top2)
	}

	topAll := s.SelectTop(groups, 10)
	if len(topAll) != 3 {
		t.Errorf("SelectTop(10) length = %d, want 3", len(topAll))
	}
}
