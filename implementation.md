# Implementation Notes — Issue #165 (HS-PORT-09): Failover e2e, migration guide, recovery procedure

## Summary

HS-PORT-09 is the final porting issue that consolidates the legacy cron
pipeline into the unified `huddlescope` CLI. This change adds:

1. **A test-only injected-clock hook** (`HSC_FAKE_NOW`) so freshness
   decisions can be tested deterministically without waiting real time — honored
   in `dev`, **fails closed** in `production`, and never used for lease
   expiry (which is server-side time).
2. **A binary-level e2e suite** (`internal/e2e/binary_test.go`) that builds the
   real `./cmd/huddlescope` binary and execs every CLI command against a
   127.0.0.1 fixture RSS server + a local `sqlite-file` DB.
3. **A failover e2e suite** (`internal/e2e/failover_test.go`, build tag `e2e`)
   implementing the 8 failover scenarios from plan §5 against the real
   `sqlite-file` backend + shared `ingest_lease`.
4. **An offline e2e matrix** (`scripts/e2e-matrix.sh` + `make e2e`) wired into
   the CI `build` job.
5. **A migration guide** (legacy-step → CLI-command mapping) and a **recovery
   procedure** (controlled cut a–i + rollback) in `docs/ops-runbook.md`, with
   pointers in `README.md`.

No `git commit` / `git push` / GitHub mutation is performed; all changes are
local file edits in the `autopilot/issue-165` worktree.

---

## AC-1: Freshness logic is testable — injected clock, production fails closed

### Change

- `internal/config/config.go`
  - Added `Config.FakeNow string` (raw `HSC_FAKE_NOW`, RFC3339, non-secret).
  - `loadFromEnv` reads `HSC_FAKE_NOW`; `loadEnvFile` parses `HSC_FAKE_NOW`
    from a `--config` file (stored raw; validated by `EffectiveNow`).
  - Added `func (c Config) EffectiveNow(realNow time.Time) (time.Time, error)`:
    - empty `FakeNow` → returns `realNow` (no behavior change).
    - invalid RFC3339 → **error** (always, regardless of env — a typo'd clock
      must surface loudly, never be silently ignored).
    - valid + `Env == "production"` → **error** (fail closed; a doctored clock
      in production is a configuration fault).
    - valid + `dev` / unset env → returns the fixed time (UTC).
  - `SafeConfig` / `MarshalJSON` include `FakeNow` (non-secret flag marker).
- `internal/runner/runner.go`
  - `RunIngest` resolves `cfg.EffectiveNow(r.now())` right after config load +
    validation; on error it returns exit 1 with the error (fail closed). All
    freshness/pipeline clock uses in the method now flow through a local
    `nowFn()` that returns the resolved effective time.
  - `CheckHeartbeat` loads config (best-effort) and resolves `EffectiveNow`; a
    doctored clock with a loadable config fails closed (exit 1). The staleness
    decision uses the effective clock.
- `internal/cli/status.go`
  - `runIngestStatus` and `runIngestShouldRun` resolve `cfg.EffectiveNow(d.Now())`
    and use it (instead of the raw injected clock) for the freshness decision;
    an error is returned (fail closed).

### Why this design

The clock is a `func() time.Time` (no error return), so fail-closed must happen
at the call sites where an error can propagate (`RunIngest`, `CheckHeartbeat`,
`runIngestStatus`, `runIngestShouldRun`). Each resolves `EffectiveNow` once,
right after config is available, and uses the fixed time for that operation.
`HSC_FAKE_NOW` never affects the lease: `IngestLeaseRepository` uses
server-side `strftime('now')` for every grant/expiry/renewal, independent of any
Go clock (AC-1 boundary, documented in the failover suite).

### Evidence

- Unit tests `internal/config/config_test.go`:
  - `TestEffectiveNowHonorsDevFakeNow`, `TestEffectiveNowHonorsFakeNowWhenEnvUnset`,
    `TestEffectiveNowProductionRejectsFakeNow`, `TestEffectiveNowProductionWithoutFakeNowUsesRealClock`,
    `TestEffectiveNowInvalidFakeNowRejected`, `TestLoadFromEnvFakeNow`,
    `TestLoadEnvFileFakeNow`, `TestRedactedKeepsFakeNow`.
- Binary-level: `TestBinaryProductionRejectsFakeNow` — `HSC_ENV=production` +
  `HSC_FAKE_NOW` set → `ingest should-run` exits non-zero with the "test-only"
  rejection message (fail closed at the process boundary).
- config package coverage: **89.1%** (gate ≥80%).

---

## AC-2: E2E test coverage — binary-level suite + 8 failover scenarios

### Binary-level suite (`internal/e2e/binary_test.go`)

`TestMain` builds `./cmd/huddlescope` once (module-cache, no network, no `make`
ordering dependency). Each test execs the binary in a **hermetic env**
(`minimalEnv` + only the keys the test sets — no `TURSO_*` / `GITHUB_*` /
`HSC_*` inherited) against a local `sqlite-file` DB in `t.TempDir()` and a
127.0.0.1 fixture RSS server, with `HSC_FAKE_NOW` injected.

Commands exercised (all at the process boundary):
- `version` — stable `huddlescope version <v>` line + revision/built/target block.
- `config check` — renders the resolved config (exits 0, no network).
- `ingest run --mode primary` — ingests fixture stories, commits homepage,
  writes `heartbeat.json` to the **temp** data dir (never `web/data`).
- `ingest status` (text + JSON) — reads run history from the shared DB; fresh DB
  reports "no runs recorded"; after a run reports the last success.
- `ingest should-run` (JSON) — never_succeeded → should_run=true; after a
  successful run with the injected clock → fresh → should_run=false.
- `export run` — produces a validating `homepage.json` with a `stories` key in
  the temp data dir.
- `heartbeat check` — missing file → clean exit 0; published fresh file →
  "Stale: no".
- `ingest should-run --stale-after <bad>` — a bad duration fails non-zero.
- `HSC_ENV=production` + `HSC_FAKE_NOW` — `should-run` fails closed (non-zero).

### Failover suite (`internal/e2e/failover_test.go`, `//go:build e2e`)

Gated behind the `e2e` build tag so it does **not** run under `make test` /
`go test ./...`; it runs only under `make e2e`. Uses the real `sqlite-file`
backend (`sqlite.NewRepository` → `NewIngestRunRepoFromDB` /
`NewIngestLeaseRepoFromDB`) on a shared `t.TempDir()` DB file. Lease expiry is
exercised with a short real-time TTL (the lease uses server-side second-granular
time; a short TTL + bounded sleep keeps the tests deterministic without the
20-minute production TTL).

The 8 scenarios (plan §5):
- **S1** `TestFailoverS1SingleHostPrimary` — acquire → register running → finish
  OK → release; exactly one `ok` run; lease is free after release.
- **S2** `TestFailoverS2CrashedHolderLeaseExpires` — A acquires + registers
  running, then crashes (no release). After TTL elapses, B takes over
  (token+1), finishes OK. B's run is the single success; A's never-finished
  running row is not counted as a success.
- **S3** `TestFailoverS3LockedBackupSkip` — A holds a valid lease; B (backup)
  gets ok=false (no error), registers `skipped_locked`, no fetch; A still holds.
- **S4** `TestFailoverS4TOCTOUPrimaryRecovers` — A finishes OK; B acquires,
  in-lock re-check finds fresh → releases lease, registers `skipped_fresh`, no
  fetch; only A's run is the success.
- **S5** `TestFailoverS5FencingBlocksStaleHolder` — A (token 1) lags; B takes
  over (token 2). A's stale token can't renew (false) and its release is a
  no-op that does NOT delete B's lease.
- **S6** `TestFailoverS6WriteFailureNotFalseOK` — real SQLite write lock
  (`BEGIN IMMEDIATE` on one conn; a second conn with a short `busy_timeout`
  attempts a write) → SQLITE_BUSY error surfaces, no false ok; after `ROLLBACK`
  writes succeed again.
- **S7** `TestFailoverS7ClockSkewFutureIsFresh` — a future `lastSuccess` is
  treated as fresh (age clamped to 0), never stale; with a 0 threshold still
  fresh.
- **S8** `TestFailoverS8TwoHandlesSameFileSingleWriter` — two independent repo
  handles on the same DB file; the lease table enforces single-writer mutual
  exclusion (the second acquirer loses with no error); both handles agree on the
  shared run-tracking state.

### Evidence

- `make e2e` → `scripts/e2e-matrix.sh` runs `go test ./internal/e2e/ -tags e2e
  -race -count=1 -v`. All binary + failover tests PASS (observed: S1–S8 green;
  binary suite green; the matrix guards against host `TURSO_*` / `GITHUB_*` env
  leaking).
- `go test ./...` (make test) also passes — the binary suite (no tag) runs
  there; the failover suite is excluded by its build tag.

---

## AC-3: Migration guide + recovery procedure

### `docs/ops-runbook.md`

Added two sections (inserted before "Deciding staleness"):
- **"Migrating from the legacy cron scripts (HS-PORT-09)"** — a legacy-step →
  CLI-command mapping table (fetch feeds → `ingest run`; freshness check →
  `ingest should-run`; score/rank → inside `ingest run`; export → `export run`;
  heartbeat → `heartbeat check`; mutual exclusion → automatic lease; validate →
  `config check`), a command reference block, and a note on `HSC_FAKE_NOW`
  (dev-only, fails closed in production).
- **"Recovery: controlled cut and rollback (HS-PORT-09)"** — a pre-cut checklist
  (snapshot legacy state, smoke `config check`, run `make e2e`, confirm shared
  non-memory backend, sign-off), the **cut steps a–i** in a table (disable legacy
  cron → install binary → point at shared storage → dry-run `ingest status` →
  first `ingest run --mode primary` → verify heartbeat/site-data → heartbeat gate
  → standby `--mode backup` skip verification → enable new cron), a **rollback**
  procedure (disable new cron → re-enable legacy cron → verify legacy resumed →
  compare to pre-cut snapshot → re-attempt only after root cause), a "what is
  not lost on rollback" note, and an **evidence template** to fill in after each
  cut attempt.

### `README.md`

Added `make e2e` to the Make-target list, an `ingest should-run` (cron-ready)
example, a `make e2e` example, and a "Migrating from the legacy cron scripts /
recovery" section pointing to the two new runbook sections.

---

## AC-4: `make e2e` runs the matrix offline; not in `make test`

- `Makefile` — new `e2e` target runs `scripts/e2e-matrix.sh`; added to `.PHONY`.
  The `test` target is unchanged (`go vet ./... && go test ./... -race` — no
  `e2e` tag, so the failover suite is excluded).
- `scripts/e2e-matrix.sh` (executable) —
  - **Offline guard**: refuses to run if any host `TURSO_DATABASE_URL`,
    `TURSO_AUTH_TOKEN`, `GITHUB_TOKEN`, `GITHUB_REPOSITORY` env var is set
    (catches operator misconfiguration).
  - Builds `dist/huddlescope-e2e` (CGO_ENABLED=0, -trimpath).
  - Runs `go test ./internal/e2e/ -tags e2e -race -count=1 -v` (compiles both the
    binary suite and the failover suite).
  - Reports PASS with the offline guarantee.
- `.github/workflows/ci.yml` — added an "E2E matrix (offline)" step to the
  existing `build` job (after the Smoke step), running `make e2e`. The step
  carries the offline invariant in its comment. `site.yml` is untouched.

---

## AC-9: Existing gates still pass (offline)

- `go build ./...` — OK.
- `go vet ./...` — clean.
- `gofmt -l` over all `.go` (excluding vendor/dist) — clean.
- `go test ./... -race` — all packages pass (no `git push`; no external
  services).
- `make check-coverage` — passes: core packages ≥80% (config 89.1%, cli 79.5%
  is the ≥70% bucket, application 85.6%, domain 85.4%), other internal ≥70%;
  `e2e` is a test-only package (skipped), `runner` is the orchestration layer
  (skipped).
- `make e2e` — passes offline.
- YAML validity of `.github/workflows/ci.yml` — verified with `python3 yaml`.

---

## local_verify failure fixes (this handoff)

The prior `local_verify` phase failed `make check`, `make lint`, and (flakily)
`make e2e`. Root causes and fixes:

### 1. `make check` / `make lint` — errcheck violation in `binary_test.go`

`golangci-lint` (errcheck) rejected `internal/e2e/binary_test.go:43`:
`os.Stderr.WriteString(...)` error unchecked. Fixed by switching to
`fmt.Fprintln(os.Stderr, ...)` whose error return is checked. `make lint`
now reports 0 issues; `make check` passes.

### 2. `make e2e` — flaky `TestReleaseGateFeedTotalFailureKeepsLastGood`

Symptom: roughly once in 5–20 runs, the release-gate test failed with
`rebuild from unchanged repo data must not be reported as changed` while the
byte-identical-file assertion still passed — i.e. `Changed=true` although the
file content was unchanged. Instrumented runs (logging every `TempWebDir`
allocation and every `DataPublisher.Publish` call) showed the cause:

- `TempWebDir` (test harness, `internal/e2e/all_test.go`) built its temp path
  from `time.Now().UnixNano()`. Two **parallel** tests allocating a web dir in
  the same nanosecond collided on the **same directory**, so two independent
  `DataPublisher` instances wrote to the **same** `homepage.json` and trampled
  each other's snapshots.
- The failing test's second publish then read the *other* test's snapshot
  (`ContentEqual` false) and rewrote — `Changed=true` — while the rewritten
  bytes were byte-identical to its own `good` snapshot (fixed
  `generatedAt`), exactly matching the observed single-assertion failure.

Fix: `TempWebDir` now uses `os.MkdirTemp` (atomic, guaranteed-unique
directory creation) instead of hand-rolled timestamp names.

Verification: `go test -tags e2e -race ./internal/e2e/ -count=1` run **40/40
clean** after the fix (the flake reproduced on the first instrumented run
before it).

### 3. `MemoryRepository.List()` — non-deterministic ordering (hardening)

`List()` returned articles in map iteration order. The snapshot builder only
tie-breaks *within* clusters, so cross-cluster story order could vary between
runs of an otherwise-identical pipeline. `List()` now returns articles sorted
by `ID`, making every downstream ordering (fetch loop, grouping, snapshots)
deterministic. Regression test:
`TestMemoryRepositoryListDeterministicOrder` (N upserts → N sorted reads × 50
repetitions).

### 4. Residual flake risk

With unique per-test data dirs, deterministic `List()` ordering, and atomic
publish (tmp + rename in `DataPublisher`), the release-gate tests are fully
hermetic per test; no shared mutable state remains between e2e tests
(`GetTestTeams` is read-only static data).

---

## Files changed

| File | Change |
| ---- | ------ |
| `internal/config/config.go` | `FakeNow` field; env + file parsing; `EffectiveNow` method; `SafeConfig`/`MarshalJSON` include it. |
| `internal/config/config_test.go` | 8 unit tests for the clock hook (dev honor, env-unset, production reject, invalid reject, env/file load, redaction). |
| `internal/runner/runner.go` | `RunIngest` + `CheckHeartbeat` resolve `EffectiveNow` (fail closed) and use it for freshness. |
| `internal/cli/status.go` | `runIngestStatus` + `runIngestShouldRun` resolve `EffectiveNow` for the freshness decision. |
| `internal/cli/deps.go` | Restored `config` import; `DefaultNow` unchanged (the clock hook is resolved in the runner/CLI call sites). |
| `internal/cli/root.go` | Restored original `NewRootCommand` (the earlier config-aware-now default was removed in favor of call-site resolution). |
| `internal/e2e/binary_test.go` | **New** — binary-level suite (build + exec the CLI against fixture server + local sqlite); errcheck fix: `fmt.Fprintln` for unchecked `os.Stderr.WriteString`. |
| `internal/e2e/failover_test.go` | **New** (`//go:build e2e`) — 8 failover scenarios on the real sqlite-file backend + shared lease. |
| `internal/e2e/all_test.go` | `TempWebDir` now uses `os.MkdirTemp` (unique paths) — fixes the parallel-test directory collision that made release-gate tests flaky. |
| `internal/adapters/storage/memory.go` | `List()` returns articles sorted by `ID` (deterministic ordering). |
| `internal/adapters/storage/memory_test.go` | `TestMemoryRepositoryListDeterministicOrder` regression test. |
| `scripts/e2e-matrix.sh` | **New** (executable) — offline e2e matrix runner. |
| `Makefile` | `e2e` target + `.PHONY`. |
| `.github/workflows/ci.yml` | "E2E matrix (offline)" step in the `build` job. |
| `docs/ops-runbook.md` | Migration guide + recovery (controlled cut a–i + rollback + evidence template). |
| `README.md` | `make e2e`, `ingest should-run` examples, migration/recovery pointer. |

## Acceptance criteria status

- **AC-1** ✅ — `HSC_FAKE_NOW` hook; production fails closed (unit + binary test).
- **AC-2** ✅ — binary suite + 8 failover scenarios (real sqlite + shared lease).
- **AC-3** ✅ — migration guide + recovery procedure in the runbook; README pointers.
- **AC-4** ✅ — `make e2e` runs the matrix offline; not in `make test`; CI step added.
- **AC-9** ✅ — `go build` / `go test ./...` / coverage gate all pass offline; no external services; no `git push`.

## Local checks (run in this worktree)

- `go build ./...` — OK
- `go vet ./...` — clean
- `gofmt -l` (all `.go`) — clean
- `go test ./... -race` — PASS (all packages)
- `make check` — PASS (build + vet + tests + coverage + nosec + govulncheck)
- `make lint` — PASS (golangci-lint: 0 issues)
- `make e2e` — PASS (offline; binary + failover suites); additionally 40/40
  consecutive `go test -tags e2e -race ./internal/e2e/ -count=1` runs clean
  after the `TempWebDir` fix (flake eliminated)
- `make check-coverage` — PASS (core ≥80%, other internal ≥70%)
- `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"` — OK
