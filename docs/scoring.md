# HS-012: Explainable Homepage Scoring

## Overview
HuddleScope computes a configurable, deterministic, and explainable homepage score for story groups based on four components:
- Freshness (time since first article)
- Source diversity (unique sources)
- Event importance (entity coverage)
- Momentum (recent article activity)

## Formula

```
TotalScore = (
    FreshnessScore × FreshnessWeight +
    SourceDiversityScore × SourceDiversityWeight +
    ImportanceScore × ImportanceWeight +
    MomentumScore × MomentumWeight
) × EvidenceMultiplier
```

All scores are in the range [0, 100]. The weights must sum to 1.0.

## Components

### Freshness Score
Exponential decay based on age since first article publication.

```
decay = age / halfLife
FreshnessScore = 100 × (1 - 1/(1 + decay))
```

Default half-life: 12 hours.

- Age 0h → 100
- Age 12h → ~50
- Age 24h → ~33
- Age 48h → ~20

### Source Diversity Score
Based on unique source count, capped at MaxSources.

```
UniqueSources = min(unique_count, MaxSources)
SourceDiversityScore = 100 × UniqueSources / MaxSources
```

Default MaxSources: 10.

Prevents a single source from dominating through repeated syndication.

### Importance Score
Based on entity coverage (teams, players).

- 0 entities → 0
- 1 entity → 30
- 2 entities → 50
- 3 entities → 70
- 4 entities → 85
- 5+ entities → 100

### Momentum Score
Based on recent article activity within MomentumWindow.

```
MomentumScore = 100 × min(active_count, MomentumCap) / MomentumCap
```

Default MomentumWindow: 24 hours.
Default MomentumCap: 10 articles.

## Evidence Level

### Confirmed
Keywords: "confirmed", "concluded", "officially", "announced", "signed", "verified", "determined", "established", "certified", "statement", "declared", "confirmed report", "official statement"

### Unverified
Keywords: "reportedly", "report", "rumor", "alleged", "sources say", "claims", "unconfirmed", "unverified", "purportedly", "speculation"

### Unknown
No confirmed or unverified keywords found.

### Evidence Multiplier
- Confirmed → 1.00
- Unverified → 0.80 (demotes unverified claims)
- Unknown → 1.00

## Scoring API

```go
scorer := domain.NewScorer()

// Override defaults
policy := domain.ScorePolicy{
    FreshnessHalfLife: 6 * time.Hour,  // Faster decay
    FreshnessWeight:   0.45,
    MaxSources:        10,
    MomentumCap:       10,
}
scorer.WithPolicy(policy)

// Score a story group
breakdown := scorer.Score(group)

// Get notes explaining the score
for _, note := range breakdown.Notes {
    fmt.Println(note)
}

// Rank groups
ranked := scorer.Rank(groups)

// Select top N
top5 := scorer.SelectTop(groups, 5)
```

## Example Output

```
Score Breakdown for "NFL free agency rumors":
  TotalScore: 72.4
  FreshnessScore: 85.0 (3 hours old)
  SourceDiversityScore: 90.0 (9 unique sources)
  ImportanceScore: 85.0 (4 entities)
  MomentumScore: 100.0 (10 recent articles, capped)
  EvidenceLevel: unverified
  Notes:
    - freshness 85.0 after 3.0h age
    - 9 unique sources (capped at 10)
    - momentum saturated: 10 articles (cap 10)
    - evidence level: unverified
    - unverified claims: information should be treated with caution
```

## Configuration Options

| Option | Default | Description |
|--------|---------|-------------|
| FreshnessHalfLife | 12h | Time for freshness score to decay to 50% |
| FreshnessWeight | 0.40 | Weight for freshness component |
| SourceDiversityWeight | 0.30 | Weight for source diversity |
| ImportanceWeight | 0.20 | Weight for entity coverage |
| MomentumWeight | 0.10 | Weight for recent activity |
| MaxSources | 10 | Capped unique source count |
| MomentumCap | 10 | Capped article count for momentum |
| MomentumWindow | 24h | Time window for momentum articles |
| EvidenceMultiplierConfirmed | 1.00 | Multiplier for confirmed claims |
| EvidenceMultiplierUnverified | 0.80 | Multiplier for unverified claims |
| EvidenceMultiplierUnknown | 1.00 | Multiplier for neutral claims |

## Ordering Rules

1. Primary: TotalScore (descending)
2. Secondary: FirstPublishedAt (newer first)
3. Tertiary: Group ID (ascending, for determinism)

## Testing

Run tests:
```bash
go test ./internal/domain -run TestScorer -v
go test ./internal/application -run TestRunScoring -v
```

Coverage must be ≥80% per internal package.