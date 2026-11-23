# Implementation Report: Issue #25 — Cross-publisher event clustering with entity-aware constraints

## Status: COMPLETE

Entity overlap is now a **constraint** (not a merge signal) in the clustering
algorithm. `JoinDecision` metadata is persisted on each story, deterministic
split/merge `CorrectionRule`s are supported with a dedicated storage layer, and
the feature is wired through config, application, runner, and storage-factory
layers. All acceptance criteria are met; tests and the coverage gate pass.

No `git commit`, `git push`, or GitHub mutations were performed.

---

## ✅ Completed Changes

### 1. Domain — entity-aware clustering constraint (AC-1)
**File**: `internal/domain/clustering.go`

- `EntityAffinity(a, b Article) float64` — Jaccard affinity over
  NFC-normalized, de-duplicated entity sets.
  - both entity-less → `1.0` (neutral)
  - one side entity-less → `1.0` (neutral)
  - otherwise Jaccard(|A∩B| / |A∪B|)
- `Article.NormalizedEntitySet() map[string]bool` — shared helper;
  whitespace-only entities ignored.
- `Clusterer.EntityAffinityMin float64` (default `0.5`, constant
  `DefaultEntityAffinityMin`) — the minimum entity-set compatibility required
  before two entity-bearing articles may merge.
- `ClusterWithDecisions(articles) ([]StoryGroup, []JoinDecision)` — the core
  clustering loop, which now applies the entity constraint on **both** the
  URL fast path and the scored path:
  - URL fast path merges across sources only when sources match, entity sets
    are identical (`affinity >= 1.0`), or one/both sides are entity-less.
    Partial-overlap-only URL matches fall through to the scored path, where the
    affinity floor applies.
  - Scored path: when both articles carry entities, `affinity < EntityAffinityMin`
    blocks merging (this kills the "same team name on two unrelated events"
    false positive).
  - The boilerplate-summary fast path (`math.Max(combined, summary)`) is only
    allowed when entity sets are identical or neutral; otherwise the title
    alone must clear the threshold (closes the boilerplate-lead bypass).
- `Cluster(articles) []StoryGroup` is now a thin wrapper over
  `ClusterWithDecisions`, returning only the groups (backward compatible).
- `StoryGroup.JoinReasons []JoinDecision` — the group carries its join
  decisions; `NewStoryFromGroup` copies them to `Story.JoinReasons`.
- `JoinDecision` struct (group, article, source, reason code, title/summary
  Jaccard scores, entity affinity, threshold, timestamp).

### 2. Domain — correction rules (AC-3)
**File**: `internal/domain/corrections.go` (new)

- `CorrectionRule` struct + `CorrectionKind` (`split`, `merge`).
- `CorrectionRule.Validate()` — intrinsic validation (malformed rules rejected).
- `CorrectionRule.ValidateAgainstGroups(groups)` — references resolve against
  the current partition (unknown group/article → error).
- `ApplyCorrections(groups, rules) ([]StoryGroup, error)` — deterministic:
  - rules applied in canonical (ID-sorted) order;
  - idempotent: re-applying the same rules yields the same partition;
  - cyclic merges rejected (A→B + B→A detection);
  - group IDs recomputed deterministically after any re-partitioning;
  - input partition never mutated on error (a fresh copy is returned);
  - join reasons preserved: split keeps them on the surviving group; merge
    combines source + target join reasons.

### 3. Domain — story & source interfaces
- `internal/domain/story.go` — `Story.JoinReasons []JoinDecision` with
  backward-compatible JSON marshal/unmarshal (old JSON without `joinReasons`
  unmarshals to `nil`).
- `internal/domain/source.go` — `CorrectionRuleRepository` interface
  (`SaveRule`, `ListRules`).

### 4. Config (AC-3 wiring)
**File**: `internal/config/config.go`

- `Config.EntityAffinityMin float64` with default handling, env var
  `HSC_CLUSTER_ENTITY_AFFINITY`, env-file parsing, and validation (zero
  treated as unset).
- Wired into `ClustererSettings()`.

### 5. Storage adapters
- `internal/adapters/storage/memory.go` — `MemoryRuleRepository` (in-memory,
  deterministic ID-sorted `ListRules`).
- `internal/adapters/storage/sqlite/migrations/007_correction_rules.sql` (new)
  — `correction_rules` table.
- `internal/adapters/storage/sqlite/repository.go` — `RuleRepository`
  (`SaveRule`/`ListRules`) + `DB()` accessor.
- `internal/adapters/storage/turso/repository.go` — `DB()` accessor exposed;
  Turso serves the same `sqlite.RuleRepository`.
- `internal/adapters/storage/factory.go` — `storage.NewRuleRepository` for
  memory/sqlite/turso backends.

### 6. Application layer (AC-3b)
**File**: `internal/application/app.go`

- `WithRuleRepository` option + `ruleRepository` field.
- `applyCorrections` helper loads rules (when a rule repository is injected)
  and applies them after clustering in the grouping, scoring, and pipeline
  paths. Rules referencing missing groups surface as an error (no crash).

### 7. Runner & types wiring
- `internal/types/deps.go` — `NewRuleRepoFunc`, `DefaultNewRuleRepository`,
  `Deps.NewRuleRepo`.
- `internal/runner/runner.go` — `Runner` constructs and injects the rule
  repository (`WithRuleRepository`) after building the clusterer/scorer.

---

## 📋 Acceptance Criteria Status

| AC | Description | Status | Notes |
|----|-------------|--------|-------|
| AC-1 | Entity overlap is a constraint, not a merge signal | ✅ | URL fast path + scored path both gated; same-team/different-events do not merge; true duplicate coverage still merges |
| AC-2 | Benchmark dataset with FP/FN gates | ✅ | `internal/domain/testdata/clustering_bench.json` + `TestClusterer_Benchmark*` gate FP=0, FN≤2 |
| AC-3a | Persist `JoinDecision` metadata per story | ✅ | `Story.JoinReasons`; backward-compatible JSON |
| AC-3b | Deterministic split/merge `CorrectionRule`s + storage | ✅ | `ApplyCorrections` (idempotent, cycle-safe) + `CorrectionRuleRepository` (memory/sqlite/turso) + integration through app/runner |
| AC-3c | Config + coverage gate | ✅ | `HSC_CLUSTER_ENTITY_AFFINITY` env; coverage gate passes |

---

## 🧪 Test Results

```
go vet ./...                       ✅ PASS
go test ./...                      ✅ PASS (all packages)
scripts/coverage-check.sh          ✅ PASS (core ≥80%, other internal ≥70%)
```

Coverage:

```
internal/application           86.1% (≥80% ✅)
internal/config                87.8% (≥80% ✅)
internal/domain                85.2% (≥80% ✅)
internal/adapters/storage       76.0% (≥70% ✅)
internal/adapters/storage/sqlite 80.2% (≥70% ✅)
internal/adapters/storage/turso  70.4% (≥70% ✅)
internal/types                  70.2% (≥70% ✅)
```

### New / updated tests

**Domain** (`internal/domain/`):
- `clustering_entity_test.go` — AC-1: entity-overlap-no-merge, multi-outlet
  duplicate coverage merge, boilerplate-summary no-merge, `EntityAffinity`
  table test, whitespace-only entity handling.
- `clustering_decisions_test.go` — AC-3a: `ClusterWithDecisions` reason codes,
  `Cluster` thin-wrapper parity, `Story` JSON round-trip incl. backward
  compatibility, nil/malformed-time/empty-entity edge cases, concurrent safety.
- `clustering_bench_test.go` — AC-2: benchmark dataset FP/FN gate,
  idempotency, whole-case partition vs ground truth.
- `corrections_test.go` — AC-3: split, merge, idempotency, invalid-rule
  rejection (unknown group/article, self-merge, cycles), re-cluster survival,
  no-op (no rules).

**Adapters** (`internal/adapters/storage/`):
- `rules_test.go` — memory rule repo round-trip + factory wiring.
- `sqlite/rules_test.go` — 007 migration creates `correction_rules`;
  save/list round-trip; invalid rule rejected.

**Application** (`internal/application/`):
- `app_corrections_test.go` — AC-3b: full RunGrouping with a split rule loaded
  from the rule repository; join reasons present on resulting stories; orphan
  rule reference surfaces an error without crashing.

**Migrations** (existing tests updated for 007):
- `sqlite/migrations_test.go`, `sqlite/repository_test.go`, `turso/turso_test.go`
  — migration count 6 → 7.

---

## 📚 Related Files

### Created
- `internal/domain/corrections.go`
- `internal/domain/corrections_test.go`
- `internal/domain/clustering_entity_test.go`
- `internal/domain/clustering_decisions_test.go`
- `internal/domain/clustering_bench_test.go`
- `internal/domain/testdata/clustering_bench.json`
- `internal/adapters/storage/rules_test.go`
- `internal/adapters/storage/sqlite/rules_test.go`
- `internal/adapters/storage/sqlite/migrations/007_correction_rules.sql`
- `internal/application/app_corrections_test.go`

### Modified
- `internal/domain/clustering.go`
- `internal/domain/story.go`
- `internal/domain/source.go`
- `internal/config/config.go`
- `internal/adapters/storage/memory.go`
- `internal/adapters/storage/factory.go`
- `internal/adapters/storage/sqlite/repository.go`
- `internal/adapters/storage/sqlite/migrations_test.go`
- `internal/adapters/storage/sqlite/repository_test.go`
- `internal/adapters/storage/turso/repository.go`
- `internal/adapters/storage/turso/turso_test.go`
- `internal/application/app.go`
- `internal/runner/runner.go`
- `internal/types/deps.go`

---

*Implementation completed for issue #25.*
