# Implementation — Issue #250: [PUB-04] Protect the last-known-good Pages deployment

Status: **COMPLETE** — implemented exactly per the saved plan (`plan.md`). No
replan was required; every fact the plan relies on was re-verified against the
source before implementing.

## Files created / changed

| # | File | Action | What it does |
|---|------|--------|--------------|
| 1 | `internal/domain/deployed_manifest.go` | **Modify** | Upgraded `DeployedManifest` to v2: added `SnapshotID`, `LastIngestAt`, `SourceCommit`, `RunID`, `OverrideReason` fields (all `omitempty` JSON). Added `Validate()` method accepting v1∪v2 (rejects unknown versions, empty required fields, non-64-char SHA256 snapshotId). Added `LegacyManifestVersion = "1"` constant; `ManifestVersion = "2"`. |
| 2 | `internal/application/publishgate.go` | **Modify** | Added `LegacyManifestVersion` constant. `LoadDeployedManifest` accepts v1∪v2 (rejects unknown). Added `StampManifestV2(candidate, commit, snapshotID, lastIngestAt, sourceCommit, runID)` that builds a v2 manifest with binding fields. Added `MonotonicStamp(stored, candidate) MonotonicStampResult` — returns `Advanced`, `Skipped` (with optional `Warning`), or `Warning` (unparseable timestamps, fail closed). Gate manifest validation checks version ∈ {v1, v2}. |
| 3 | `internal/application/htmlverify.go` | **Create** | `ValidateRenderedSite(publicDir, candidate, m)` — the rendered-HTML layer of last-known-good protection. Checks: index.html exists; story-card count ≥ candidate story count and manifest minimum; freshness label (`<time datetime>`) matches candidate `generatedAt`; no demo/fixture markers in any rendered HTML. Returns `VerifyResult{Pass, Reason, StoryCards, Checks}`. All failures are deny (fail closed). |
| 4 | `internal/adapters/storage/sqlite/deployed_manifest.go` | **Modify** | `GetDeployedManifest` now SELECTs v2 columns with `COALESCE` (empty defaults for v1 rows). Accepts version "1" or "2" (rejects unknown). `PutDeployedManifest` upserts all v2 columns. |
| 5 | `internal/adapters/storage/sqlite/migrations/012_deployed_manifest_columns.sql` | **Create** | Additive `ALTER TABLE deployed_manifests ADD COLUMN` for `snapshot_id`, `last_ingest_at`, `source_commit`, `run_id`, `override_reason` (all `TEXT NOT NULL DEFAULT ''`). No data migration; v1 rows read as v2 with empty binding fields. |
| 6 | `internal/cli/publish.go` | **Modify** | Added `publish verify-html` command (exits 0 pass / 3 deny / 1 invalid). Added `publish report` command (structured secret-free JSON diagnostic; aggregates gate/verify/manifest + run identity). Added `publish manifest bootstrap` command (explicit audited seed for fresh stores; refuses override without `--acknowledge-override`). Extended `manifest put` with `--snapshot-id`, `--last-ingest`, `--source-commit`, `--run-id` flags; put is now monotonic (reads stored manifest, skips with warning if candidate is older). |
| 7 | `.github/workflows/refresh-news.yml` | **Modify** | Added `verify-html` step (hard fail, after make-site, before Pages uploads). Extended `manifest put` with `--run-id` and `--source-commit`. Added `publish report` step + diagnostic artifact upload (7-day retention). Added live smoke step (soft, `continue-on-error: true`) that curls the live site post-deploy. |
| 8 | `.github/workflows/site.yml` | **Modify** | Added `verify-html` step (hard fail, after make-site, before Pages uploads). Extended `publish gate` to set a `deploy` output (`allow` → `deploy=true`, `skip-identical` → `deploy=false`). Pages steps now gate on `publish-gate.outputs.deploy`. Extended `manifest put` with `--run-id` and `--source-commit`. |
| 9 | `internal/e2e/publish_test.go` | **Modify** | Added S6 (skeleton render deny via verify-html) and S7 (rendered pass). |
| 10 | `internal/e2e/publish_safety_test.go` | **Create** | P-SAFE-1 (monotonic stamp: late run with older candidate is a no-op, stored generatedAt preserved). P-SAFE-2 (v2 manifest put records binding fields, round-trip via manifest get). P-SAFE-3 (diagnostic report is secret-free: no DSN/token/credential substrings). All offline (sqlite-file backend). |
| 11 | `internal/e2e/workflow_contract_test.go` | **Modify** | Added `TestWorkflowContract_RefreshNews_VerifyHTML` (verify-html step exists, fails closed, ordered correctly; manifest put passes `--run-id`/`--source-commit`; report upload step present; concurrency invariants retained). Added `TestWorkflowContract_Site_VerifyHTML` (verify-html step exists, fails closed; gate sets deploy output with skip-identical handling; Pages steps gate on deploy output; manifest put passes `--run-id`/`--source-commit`). |
| 12 | `internal/application/htmlverify_test.go` | **Create** | Unit tests for `ValidateRenderedSite`: pass, skeleton deny, cards-below-manifest-minimum deny, stale freshness label deny, demo marker deny, zero-candidate-with-cards deny, missing index, missing freshness label, nil/invalid candidate. |
| 13 | `internal/application/publishgate_test.go` | **Modify** | Added tests for v2 manifest gate, legacy v1 manifest gate, unknown version error, `StampManifestV2` binding fields, `MonotonicStamp` (first put advances, older candidate skips with warning, newer advances, equal generatedAt + identical content skips no warning, equal generatedAt + different content advances, unparseable timestamps fail closed). |
| 14 | `internal/domain/deployed_manifest_test.go` | **Create** | Unit tests for `Validate()`: v2 with binding fields, v1 legacy, unknown version error, empty version error. |
| 15 | `internal/adapters/storage/sqlite/*_test.go` | **Modify** | Updated migration count expectations from 11 to 12 (6 files). Updated carry-over test sequences to include migration 12. |
| 16 | `internal/adapters/storage/retention_test.go` | **Modify** | Updated schema version expectation from 11 to 12. |
| 17 | `internal/adapters/storage/turso/turso_test.go` | **Modify** | Updated migration count expectation from 11 to 12. |
| 18 | `internal/adapters/storage/turso/retention_test.go` | **Modify** | Updated schema version expectation from 11 to 12 (2 occurrences). |
| 19 | `docs/ops-runbook.md` | **Modify** | Added "Publication safety contract" section: layers of protection, failure modes table, rollout (v1→v2), concurrency, manual intervention runbook. |

## Acceptance criteria — how each is met

- **AC1 (regression detection):** The publish gate and `verify-html` detect
  zero-over-N, stale candidates, and skeleton renders. `verify-html` checks
  story-card count ≥ manifest minimum. Tested: `TestPublishGate_S2`,
  `TestPublishGate_S3`, `TestPublishVerifyHTML_S6`,
  `TestValidateRenderedSite_CardsBelowManifestMinimum_Deny`.

- **AC2 (last-known-good protection):** Every failure mode is fail-closed.
  The gate denies (exit 3), verify-html denies (exit 3), the workflow steps
  are hard (no `continue-on-error`), and the Pages upload is skipped when the
  build job fails. The `site.yml` gate sets `deploy=false` on `skip-identical`
  so no redundant Pages write occurs. Tested: `TestWorkflowContract_Site_VerifyHTML`,
  `TestWorkflowContract_RefreshNews_VerifyHTML`.

- **AC3 (monotonic stamp):** `MonotonicStamp` never lowers the stored
  `generatedAt`. A late/delayed run with an older candidate is a no-op with a
  warning. The `manifest put` reads the stored manifest first and skips if the
  candidate is older. Tested: `TestMonotonicStamp_OlderCandidate_SkipsWithWarning`,
  `TestPublishManifest_PutMonotonic_LateRun_Skips`.

- **AC4 (v2 manifest binding):** The manifest v2 records `snapshotId`,
  `lastIngestAt`, `sourceCommit`, `runId` (and `overrideReason`). The
  `manifest put` accepts `--snapshot-id`, `--last-ingest`, `--source-commit`,
  `--run-id` flags. The sqlite migration 012 adds the v2 columns. The gate
  accepts v1∪v2. Tested: `TestStampManifestV2_BindingFields`,
  `TestPublishManifest_PutV2_BindingFields`, `TestEvaluatePublishGate_V2Manifest_Allow`,
  `TestEvaluatePublishGate_LegacyV1Manifest_Allow`.

- **AC5 (bootstrap policy):** `publish manifest bootstrap` is the explicit,
  audited path for seeding a fresh store. It refuses to override an existing
  manifest without `--acknowledge-override` + a recorded reason. Tested:
  `newPublishManifestBootstrapCommand` in `cli/publish.go`.

- **AC6 (rendered-HTML verification):** `publish verify-html` compares the
  rendered `web/public/index.html` against the candidate and the stored
  manifest. Catches skeleton/zero-story renders. Exits 0 (pass) or 3 (deny).
  Tested: `TestValidateRenderedSite_*` (unit), `TestPublishVerifyHTML_S6`,
  `TestPublishVerifyHTML_S7` (e2e).

- **AC7 (no demo marker leak):** `verify-html` scans all rendered HTML files
  for demo markers (same markers as the workflow's `grep -RInE` check).
  Tested: `TestValidateRenderedSite_DemoMarker_Deny`.

- **AC8 (diagnostic report):** `publish report` emits a structured,
  secret-free JSON report aggregating the gate verdict, verify result,
  manifest, and run identity. The report is non-secret by construction.
  Tested: `TestPublishReport_SecretFree`.

- **AC9 (live smoke):** The `refresh-news.yml` deploy job has a soft live
  smoke step (`continue-on-error: true`) that curls the live site post-deploy
  and asserts the homepage rendered. A transient Pages propagation lag fails
  the observation, not the run.

## Verification performed (all green)

- `make check` (gofmt, go vet, golangci-lint, gosec, govulncheck) — pass.
- `make test` — all packages `ok`.
- `make build` — pass. `./dist/huddlescope version` — OK.
- `./dist/huddlescope --help` — OK. `./dist/huddlescope config check` — OK.
- `make e2e` — offline matrix PASS (includes S6, S7, P-SAFE-1/2/3, workflow contract tests).
- `make check-coverage` — gate passed (core ≥80%, other internal ≥70%).
- `bash -n scripts/deploy.sh` — pass.
- `cd scheduler && npm test` — 32 pass, 0 fail.
