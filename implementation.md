# Implementation — Issue #252: [PUB-06] Isolate database retention/archival from news publication

Status: **COMPLETE** — implemented exactly per the saved plan (`plan.md`). No
replan was required; every fact the plan relies on (the retention engine, the
CLI surface, the workflow coupling, the contract tests) was re-verified against
the source before implementing.

## What was wrong

`refresh-news.yml` bundled retention work into the news publication build job:
a `run-retention` dispatch input, a `retention-export` step, a
`commit-and-push-retention` step (with a `gh pr` fallback and a `gh pr merge`
step), a `cleanup-retention-pr` step, and a `retention-apply` step, plus a
job-level `contents: write` permission and `SITE_DATA_PAT` env. Retention was
thus tightly coupled to every publication, mutated Git for archive commits,
held elevated write permission on the hot path, and had no independent schedule
or operator gate.

## What changed (all changes; production code was NOT touched)

The retention engine (`internal/adapters/storage/sqlite/retentiondb.go`,
`retentionpkg.go`), the `retention` CLI subcommands (`internal/cli/retention.go`),
and `internal/domain/retention.go` already implement the fail-safe primitives
the issue needs (export is non-destructive; apply archives-then-deletes in one
transaction per batch; import is a validated, idempotent restore; status reports
the storage budget). **No Go production code was modified.** The change is purely
orchestration + tests + docs:

### 1. `.github/workflows/retention.yml` (NEW — dedicated retention workflow)
- `schedule` (`0 3 * * 1` UTC, weekly) + `workflow_dispatch` with an `apply`
  input (default **`false`** = dry-run) and a `window` input (days).
- `concurrency: group: huddlescope-retention`, `cancel-in-progress: false`
  (dedicated group, no in-progress cancel — AC-3).
- Workflow-level `permissions: contents: read` only (least privilege, no
  escalation — AC-2).
- Job steps (order enforced): `status` (pre) → `export` (writes
  `dist/retention/retention-<UTC>.tar.gz`; no-op when nothing expired) →
  `verify` (sha256 + `retention import` into a **throwaway `sqlite-file` DB** —
  AC-4) → `persist` (only when `apply=true`; pushes the package + a secret-free
  `manifest.json` to the private archive repo `HuddleScope/huddlescope-retention-archive`
  via `gh api` using the scoped `RETENTION_ARCHIVE_PAT` injected only as the
  step's `GH_TOKEN`) → `apply` (gated on `apply=true` AND `persist.conclusion
  == 'success'` — fail closed, no deletion without a durable copy — AC-2) →
  `status` (post) + `report` (secret-free JSON, 90-day diagnostic artifact) →
  `cleanup` (best-effort `if: always()`, log-only, never masks failure) removes
  a pushed commit if the run later fails.
- `RETENTION_ARCHIVE_PAT` missing or persist failing → `apply` never runs. Until
  the operator provisions the archive repo + secret, the workflow runs **dry-run
  only**.

### 2. `.github/workflows/refresh-news.yml` (surgeon — retention removed)
- Removed the `run-retention` dispatch input.
- Removed the `retention-export`, `commit-and-push-retention`,
  `cleanup-retention-pr`, and `retention-apply` steps.
- Removed the build job's `permissions: contents: write` (now inherits
  workflow-level `contents: read`).
- Removed all `SITE_DATA_PAT` env references (the env block no longer references
  it).
- Updated the header/permissions/step comments to document the isolation.

### 3. Contract tests
- `internal/e2e/workflow_contract_test.go`:
  - `TestWorkflowContract_RefreshNews_Structure`: the old "retention pipeline
    order" block was repurposed into an **absence + no-Git-mutation** assertion
    (build job holds no job-level permissions; no step's `run` contains
    `git push` / `git commit` / `gh pr`).
  - New `TestWorkflowContract_RefreshNews_RetentionAbsence`: refresh-news.yml
    contains no `retention-export`/`commit-and-push-retention`/
    `cleanup-retention-pr`/`retention-apply`/`run-retention`/`SITE_DATA_PAT`/
    `contents: write`.
  - New `TestWorkflowContract_Retention_Structure`: validates retention.yml
    (triggers, dedicated concurrency, `contents: read` only, and the
    `export → verify → persist → apply` step order with `apply` gated on persist
    success + the `apply=true` input).
  - Added `stepIdx` helper; `findStepByNameIn` now non-fatal (returns index -1).
- `internal/cli/workflows_test.go`:
  - `TestRefreshNewsWorkflow_Issue247`: repurposed the retention-commit
    assertions into absence assertions (no retention commit/apply step, no
    `git add archive/retention`, no `contents: write`, no `SITE_DATA_PAT`).
  - New `TestRetentionWorkflow_Issue252`: asserts retention.yml exists, is
    dry-run-by-default (`apply` default `false`), has a schedule, a dedicated
    concurrency group with no cancel, `contents: read` (not write), the scoped
    PAT, and `apply` gated on persist success.

### 4. Regression tests (targeted, per the plan)
- `internal/adapters/storage/sqlite/retentiondb_test.go`:
  - `TestArchiveOldDB_WindowBoundary_RefreshedRowNotDeleted` (AC-3): a row whose
    `published_at` moves into the window between export and apply is not
    archived/deleted.
  - `TestArchiveOldDB_ArchiveBeforeDeleteInvariant` (AC-3): every archived row
    is present in its `*_archive` mirror and hot+archive counts are
    complementary.
  - `TestExportApplyImportRoundTrip` (AC-4): export → apply → export(no-op) →
    import restores the archived rows (idempotent).
- `internal/adapters/storage/sqlite/retentionpkg_test.go`:
  - Extended `TestImportRejectsMaliciousPackage`: wrong version, a
    non-whitelisted table member, an INSERT targeting a different table, and a
    row-count mismatch.
  - `TestImportRejectsOversizeAndTruncatedPackage` (AC-4): a package over the
    size cap is rejected; a truncated (cut) gzip stream is rejected.

### 5. Documentation
- `archive/retention/README.md`: states it is local-development convenience
  only; CI no longer writes or commits here; packages persist to the private
  archive repo.
- `docs/ops-runbook.md`: rewrote "Retention and archiving" for the dedicated
  workflow (durable storage = in-Turso `*_archive` + private archive repo;
  fail-closed gate; pipeline steps; manual dispatch with `apply`/`window`;
  access controls; archive expiry; free-tier budget evaluation; recovery
  procedure; safety properties incl. window-boundary + concurrency invariants).
  Updated the refresh-news pipeline description/permissions, secret rotation
  (added `RETENTION_ARCHIVE_PAT`), and the fixed-issue notes.
- `docs/release-gate.md`: removed the retention line from the refresh pipeline
  description.
- `docs/editorial-policy.md`: updated the archive-retention note to the durable
  private archive repo + dedicated workflow, and the pipeline-status line.

## Verification

Local commands (all passing in the worktree): `go build ./...`, `go vet ./...`,
`gofmt -l internal/ cmd/` (clean), `go test ./...`, `make build`, `make e2e`
(contract tests run under the e2e package), `./dist/huddlescope retention
status|help`. `make security` (gosec/govulncheck) and `make lint` (golangci-lint)
depend on toolchain availability; no production Go code changed, so the risk
surface is limited to the new YAML and test files. Both workflow YAML files
parse cleanly.

## Out of scope / explicit non-goals

- No Turso backup (out of scope).
- No changes to the retention engine, `retention` CLI, or domain logic.
- No changes to Turso schema, site.yml, deploy.sh, or the scheduler (Cloudflare)
  — the weekly `schedule` trigger is inside retention.yml (a GitHub workflow),
  not Cloudflare.
- Provisioning the actual private archive repo + PAT is an operator step
  documented in the runbook; until then the workflow is dry-run only.

## Final verification pass (this phase)

All 14 local verification commands pass from the repo root:
`make check`, `make test`, `gofmt -l` (clean), `make lint`, `make security`,
`make check-coverage`, `make build`, `./dist/huddlescope version`,
`./dist/huddlescope --help`, `./dist/huddlescope config check`, `make e2e`,
`bash -n scripts/deploy.sh`, `make cross-build`, `npm --prefix scheduler install`,
`npm --prefix scheduler test`.

Fix applied in this phase: `internal/e2e/workflow_contract_test.go:605` tripped
staticcheck QF1001 (De Morgan). Rewrote `!(a && b && c)` as
`a >= b || b >= c || c >= a` for the step-order assertion. No production or
behavioral change; `make check` and `make lint` now report 0 issues.
