// Package domain contains core domain models and interfaces.
package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// EvidenceLevel indicates the confidence level of an assertion.
type EvidenceLevel int

const (
	EvidenceUnknown EvidenceLevel = iota
	EvidenceUnverified
	EvidenceConfirmed
)

func (e EvidenceLevel) String() string {
	switch e {
	case EvidenceConfirmed:
		return "confirmed"
	case EvidenceUnverified:
		return "unverified"
	default:
		return "unknown"
	}
}

// ClassifyEvidence returns the evidence level based on text content.
// It uses simple keyword heuristics to distinguish rumor, unverified claims, and confirmed facts.
// Note: "unconfirmed" and "unverified" take priority over "confirmed" to avoid false positives.
// This is a heuristic only; successor issues may improve this logic.
func ClassifyEvidence(text string) EvidenceLevel {
	if text == "" {
		return EvidenceUnknown
	}
	lower := strings.ToLower(text)

	// Unverified/rumor keywords (checked first to avoid false positives)
	unverifiedKeywords := []string{
		"unconfirmed", "unverified",
		"reportedly", "rumor", "alleged", "sources say",
		"claims", "sources", "say", "rumors", "allegedly",
		"purportedly", "purported", "speculation", "speculative",
	}

	for _, kw := range unverifiedKeywords {
		if strings.Contains(lower, kw) {
			return EvidenceUnverified
		}
	}

	// Confirmed keywords (checked after unverified to avoid false positives)
	confirmedKeywords := []string{
		"confirmed report", "confirmed news",
		"officially confirmed", "official statement",
		"confirmed", "concluded", "officially", "announced", "signed",
		"verified", "determined", "established", "certified",
		"statement", "declared", "announced today",
	}

	for _, kw := range confirmedKeywords {
		if strings.Contains(lower, kw) {
			return EvidenceConfirmed
		}
	}

	return EvidenceUnknown
}

// ScorePolicy holds scoring configuration with documented defaults.
// All weights sum to 1.0 for a normalized 0-100 score.
type ScorePolicy struct {
	// Freshness: decay half-life in hours (0 = no decay)
	FreshnessHalfLife time.Duration
	// Component weights (must sum to 1.0)
	FreshnessWeight       float64
	SourceDiversityWeight float64
	ImportanceWeight      float64
	MomentumWeight        float64
	// Syndication caps
	MaxSources     int
	MomentumCap    int
	MomentumWindow time.Duration
	// Evidence multipliers (unverified content receives less weight)
	EvidenceMultiplierConfirmed  float64
	EvidenceMultiplierUnverified float64
	EvidenceMultiplierUnknown    float64
}

// DefaultScorePolicy returns default scoring configuration.
func DefaultScorePolicy() ScorePolicy {
	return ScorePolicy{
		FreshnessHalfLife:            12 * time.Hour,
		FreshnessWeight:              0.40,
		SourceDiversityWeight:        0.30,
		ImportanceWeight:             0.20,
		MomentumWeight:               0.10,
		MaxSources:                   10,
		MomentumCap:                  10,
		MomentumWindow:               24 * time.Hour,
		EvidenceMultiplierConfirmed:  1.00,
		EvidenceMultiplierUnverified: 0.80,
		EvidenceMultiplierUnknown:    1.00,
	}
}

// Validate checks the policy configuration.
func (p *ScorePolicy) Validate() error {
	if p.FreshnessWeight < 0 || p.FreshnessWeight > 1 {
		return fmt.Errorf("freshness weight must be in [0,1]")
	}
	if p.SourceDiversityWeight < 0 || p.SourceDiversityWeight > 1 {
		return fmt.Errorf("source diversity weight must be in [0,1]")
	}
	if p.ImportanceWeight < 0 || p.ImportanceWeight > 1 {
		return fmt.Errorf("importance weight must be in [0,1]")
	}
	if p.MomentumWeight < 0 || p.MomentumWeight > 1 {
		return fmt.Errorf("momentum weight must be in [0,1]")
	}
	sum := p.FreshnessWeight + p.SourceDiversityWeight +
		p.ImportanceWeight + p.MomentumWeight
	if sum < 0.99 || sum > 1.01 {
		return fmt.Errorf("weights must sum to 1.0 (got %.4f)", sum)
	}
	if p.MaxSources <= 0 {
		return fmt.Errorf("max sources must be positive")
	}
	if p.MomentumCap <= 0 {
		return fmt.Errorf("momentum cap must be positive")
	}
	if p.MomentumWindow <= 0 {
		return fmt.Errorf("momentum window must be positive")
	}
	return nil
}

// SourceBreakdown captures per-source counts for scoring.
type SourceBreakdown struct {
	TotalArticles int
	UniqueSources int
}

// ExtractSourceBreakdown analyzes a story group and returns per-source counts.
// Only counts articles within the momentum window.
func ExtractSourceBreakdown(group StoryGroup, now time.Time, window time.Duration) SourceBreakdown {
	bySource := make(map[string]int)
	nowMinusWindow := now.Add(-window)

	for _, a := range group.Articles {
		pubTime := a.ParsePublishedAtOrZero()
		// Count articles within momentum window
		if pubTime.After(nowMinusWindow) {
			sid := NormalizeSourceID(a.SourceID)
			bySource[sid]++
		}
	}

	total := 0
	for _, count := range bySource {
		total += count
	}

	return SourceBreakdown{
		TotalArticles: total,
		UniqueSources: len(bySource),
	}
}

// ScoreBreakdown captures per-component values and total score (0-100).
type ScoreBreakdown struct {
	TotalScore           float64 // 0-100
	FreshnessScore       float64 // 0-100
	SourceDiversityScore float64 // 0-100
	ImportanceScore      float64 // 0-100
	MomentumScore        float64 // 0-100
	EvidenceLevel        EvidenceLevel
	Notes                []string // human-readable explanation
}

// Scorer computes scores using a policy and deterministic time anchor.
type Scorer struct {
	Policy ScorePolicy
	Now    time.Time
}

// NewScorer creates a new Scorer with default policy and current time.
func NewScorer() *Scorer {
	return &Scorer{
		Policy: DefaultScorePolicy(),
		Now:    time.Now().UTC(),
	}
}

// WithPolicy overrides the scoring policy.
func (s *Scorer) WithPolicy(p ScorePolicy) *Scorer {
	s.Policy = p
	return s
}

// WithTime sets a deterministic time anchor.
func (s *Scorer) WithTime(t time.Time) *Scorer {
	s.Now = t.UTC()
	return s
}

// computeFreshnessScore returns a score [0,100] based on article age.
// Uses exponential decay with the configured half-life.
func (s *Scorer) computeFreshnessScore(firstPublishedAt time.Time) float64 {
	if firstPublishedAt.IsZero() {
		return 50.0 // neutral when unknown
	}
	age := s.Now.Sub(firstPublishedAt)
	if age < 0 {
		age = 0
	}
	halfLife := s.Policy.FreshnessHalfLife
	if halfLife <= 0 {
		halfLife = 12 * time.Hour // fallback
	}
	// Decay factor: 1 / (1 + age/halfLife)
	// score = 100 * decay_factor
	// age=0 → 100, age=halfLife → 50, age=∞ → 0
	decay := 1.0 / (1.0 + age.Hours()/halfLife.Hours())
	score := 100.0 * decay
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

// computeSourceDiversityScore returns a score based on unique source count.
// Caps per-source contribution to prevent syndication dominance.
func (s *Scorer) computeSourceDiversityScore(breakdown SourceBreakdown) float64 {
	if breakdown.UniqueSources <= 0 {
		return 0
	}
	if breakdown.TotalArticles <= 0 {
		return 0
	}
	// Deduplicate: count unique sources, capped
	unique := breakdown.UniqueSources
	if unique > s.Policy.MaxSources {
		unique = s.Policy.MaxSources
	}
	// Normalize: log scale from 1 to MaxSources maps to [0,100]
	score := 100.0 * float64(unique) / float64(s.Policy.MaxSources)
	return score
}

// computeImportanceScore returns a score based on entities (teams, players).
// More entities indicate a higher-importance story.
func (s *Scorer) computeImportanceScore(group StoryGroup) float64 {
	if len(group.Articles) == 0 {
		return 0
	}
	// Collect unique entities
	entities := make(map[string]bool)
	for _, a := range group.Articles {
		for _, e := range a.Entities {
			entities[e] = true
		}
	}
	// More entities → higher importance, capped
	count := len(entities)
	if count <= 0 {
		return 50.0 // neutral
	}
	// Map count to [0,100] with diminishing returns
	// Example: 1 entity = 30, 5+ entities = 100
	switch count {
	case 1:
		return 30
	case 2:
		return 50
	case 3:
		return 70
	case 4:
		return 85
	default: // count >= 5
		return 100
	}
}

// computeMomentumScore returns a score based on recent article activity.
// Counts articles within momentum window, capped.
func (s *Scorer) computeMomentumScore(breakdown SourceBreakdown) float64 {
	if breakdown.TotalArticles <= 0 {
		return 0
	}
	articles := breakdown.TotalArticles
	if articles > s.Policy.MomentumCap {
		articles = s.Policy.MomentumCap
	}
	// Normalize: [0,MomentumCap] → [0,100]
	score := 100.0 * float64(articles) / float64(s.Policy.MomentumCap)
	return score
}

// Score computes a full breakdown for a story group.
func (s *Scorer) Score(group StoryGroup) ScoreBreakdown {
	breakdown := ExtractSourceBreakdown(group, s.Now, s.Policy.MomentumWindow)

	// Compute component scores
	freshness := s.computeFreshnessScore(group.FirstPublishedAt)
	sourceDiv := s.computeSourceDiversityScore(breakdown)
	importance := s.computeImportanceScore(group)
	momentum := s.computeMomentumScore(breakdown)

	// Evidence level (use first article as representative)
	var evidence EvidenceLevel
	if len(group.Articles) > 0 {
		evidence = ClassifyEvidence(group.Articles[0].Title + " " + group.Articles[0].Summary)
	}

	// Apply evidence multiplier
	var mult float64
	switch evidence {
	case EvidenceConfirmed:
		mult = s.Policy.EvidenceMultiplierConfirmed
	case EvidenceUnverified:
		mult = s.Policy.EvidenceMultiplierUnverified
	default:
		mult = s.Policy.EvidenceMultiplierUnknown
	}

	// Compute total score (0-100)
	total := (freshness*s.Policy.FreshnessWeight +
		sourceDiv*s.Policy.SourceDiversityWeight +
		importance*s.Policy.ImportanceWeight +
		momentum*s.Policy.MomentumWeight) * mult

	// Build notes for explainability
	notes := make([]string, 0, 5)
	notes = append(notes, fmt.Sprintf("freshness %.2f after %.1fh age", freshness, s.Now.Sub(group.FirstPublishedAt).Hours()))

	if breakdown.UniqueSources == s.Policy.MaxSources {
		notes = append(notes, fmt.Sprintf("source diversity capped at %d sources", breakdown.UniqueSources))
	} else {
		notes = append(notes, fmt.Sprintf("%d unique sources", breakdown.UniqueSources))
	}

	if breakdown.TotalArticles > s.Policy.MomentumCap {
		notes = append(notes, fmt.Sprintf("momentum saturated: %d articles (cap %d)", breakdown.TotalArticles, s.Policy.MomentumCap))
	} else {
		notes = append(notes, fmt.Sprintf("momentum: %d recent articles", breakdown.TotalArticles))
	}

	notes = append(notes, fmt.Sprintf("evidence level: %s", evidence))

	// If evidence is unverified, add a caution note
	if evidence == EvidenceUnverified {
		notes = append(notes, "unverified claims: information should be treated with caution")
	}

	return ScoreBreakdown{
		TotalScore:           total,
		FreshnessScore:       freshness,
		SourceDiversityScore: sourceDiv,
		ImportanceScore:      importance,
		MomentumScore:        momentum,
		EvidenceLevel:        evidence,
		Notes:                notes,
	}
}

// Rank sorts story groups by score (desc), then by first published (desc), then by ID (asc).
func (s *Scorer) Rank(groups []StoryGroup) []StoryGroup {
	sorted := make([]StoryGroup, len(groups))
	copy(sorted, groups)

	sort.Slice(sorted, func(i, j int) bool {
		// Primary: score desc
		si := s.Score(sorted[i]).TotalScore
		sj := s.Score(sorted[j]).TotalScore
		if si != sj {
			return si > sj
		}
		// Tie-break 1: newer firstPublishedAt
		if !sorted[i].FirstPublishedAt.Equal(sorted[j].FirstPublishedAt) {
			return sorted[i].FirstPublishedAt.After(sorted[j].FirstPublishedAt)
		}
		// Tie-break 2: ID asc (deterministic)
		return sorted[i].ID < sorted[j].ID
	})

	return sorted
}

// SelectTop returns the top N groups after ranking.
func (s *Scorer) SelectTop(groups []StoryGroup, n int) []StoryGroup {
	ranked := s.Rank(groups)
	if n >= len(ranked) {
		return ranked
	}
	return ranked[:n]
}
