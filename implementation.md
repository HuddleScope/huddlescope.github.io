# Implementation: Issue #120 — Wire retention to the pipeline; add export-to-package and import

Implemented `autopilot/issue-120` per the saved plan (plan §1–§7). Retention is
now a real, backend-agnostic pipeline: expiring rows in the hot tables
(`articles`, `stories`, `ranking_snapshots`) are exported to a portable
`huddlescope-retention-package` (v1) before they are archived into the
`*_archive` mirrors, and packages can be restored idempotently via import.
The pipeline is wired into the `huddlescope` CLI (`retention` command), the
GitHub Actions refresh workflow (`run-retention` steps, export → commit →
apply), and the config surface (`HSC_RETENTION_ENABLED`, `HSC_RETENTION_DAYS`).

No unrelated work was touched; the pre-existing (unwired) HS-041 retention
functions were refactored into `*sql.DB`-level functions that the existing
path-based API now wraps, so all prior behavior and tests are preserved.

## What was built

### Domain contract (`internal/domain/retention.go`, new)

- `RetentionPolicy` (window, batch limit, free-tier budget, source backend)
  with sane defaults (`90` days, `500` rows/batch, `512 MiB`).
- `PackageReport`, `RetentionReport`, `RetentionStatus` report types.
- `RetentionService` interface: `ExportPackage` (no-op-safe, never deletes),
  `Apply` (archive + budget check), `Import` (validated restore), `Status`,
  `Close`.

### Driver-agnostic storage layer

- `internal/adapters/storage/sqlite/retentiondb.go` (new): DB-level
  `ArchiveOldDB`, `StorageEstimateDB`, `SchemaVersionDB`,
  `ApplyRetentionDB` operating on an open `*sql.DB` handle — works identically
  on the modernc sqlite driver and the libsql (Turso) driver.
- `internal/adapters/storage/sqlite/retentionpkg.go` (new): package
  export/import. Export writes `manifest.json` + `tables/<table>.sql`
  (`INSERT OR REPLACE` in bounded 500-row transactions) into a `.tar.gz`
  (0600, temp-file + atomic rename); no-op when nothing expires. Import
  validates format/version, the 3-table whitelist, per-file row counts, and
  classifies every statement (only `INSERT OR REPLACE`/`BEGIN`/`COMMIT`
  allowed; `DROP`/`DELETE`/`UPDATE`/`ALTER`/`CREATE` rejected; path traversal
  rejected; size cap enforced).
- `internal/adapters/storage/sqlite/retention.go` (refactored): the original
  path-based HS-041 API (`ArchiveOld`, `StorageEstimate`,
  `ApplyRetention`, …) is now a thin wrapper over the DB-level functions on a
  single modernc handle — existing behavior and tests preserved.
- `internal/adapters/storage/sqlite/retentionservice.go` (new): `DBService`
  implementing `domain.RetentionService` on a `*sql.DB` handle.
- `internal/adapters/storage/turso/retention.go` (new): Turso
  `RetentionService` — construction is network-free (`buildDSN` + `sql.Open`
  only; no `Ping`), matching the Turso backend's lazy style; first operation
  runs `PRAGMA foreign_keys = ON` + migrations once; errors are redacted
  (`RedactedDSN`, never leaks the token).
- `internal/adapters/storage/retention.go` (new): `NewRetentionService(cfg)`
  factory: `sqlite-file` (eager migrations), `turso` (lazy), `memory`
  (documented no-op: export/import fail loudly — packages must be persisted;
  apply is a safe no-op), unknown backend errors.

### CLI (`internal/cli/retention.go`, new; wired in `root.go`, `deps.go`)

`huddlescope retention` with subcommands:

- `run` — export then apply; `--dry-run` exports only; any export failure
  aborts before apply (AC-2).
- `export` — write expiring rows to `--output` (default
  `archive/retention`); no-op-safe, exit 0 when nothing expired.
- `apply` — archive expiring rows, check storage budget.
- `import --package <file>` — validated, idempotent restore (flag required).
- `status` — enabled/window/backend/schema/size/budget.

All honor `HSC_RETENTION_ENABLED=false` (clean no-op) and `--window`/
`HSC_RETENTION_DAYS`. `types.Deps` gains `NewRetentionService` (default:
`storage.NewRetentionService`) with a `WithNewRetentionService` option for
tests.

### Config (`internal/config/config.go`)

- `Config.RetentionEnabled` (default `true`) + `Config.RetentionWindowDays`
  (default `90`); env `HSC_RETENTION_ENABLED` (bool-ish) and
  `HSC_RETENTION_DAYS` (positive int) via both `loadFromEnv` and
  `loadEnvFile` (unknown-key-permissive, invalid values rejected); validation
  accepts zero (unset) and rejects negative; included in `Redacted`/
  `MarshalJSON` safe config. `.env.example` documents both.

### Pipeline wiring

- `Makefile`: new `retention` target (`./dist/huddlescope retention run`).
- `.github/workflows/refresh-news.yml`: new `run-retention` input (default
  `true`) and three steps **after** all DB reads (ingest, export, site build):
  `retention export` → `commit retention package` (skips cleanly on a no-op
  export; any failure fails the job) → `retention apply`. No
  `continue-on-error` anywhere, so apply can never run without a persisted
  package in Git (AC-2).
- `archive/retention/README.md` (new): documents the package store and the
  restore command.

### Docs

- `docs/ops-runbook.md`: new "Retention and archiving (HS-065 / issue #120)"
  section — pipeline order, local commands, restore procedure, safety
  properties; the stale "not yet wired" note from HS-041 is corrected.
- `docs/editorial-policy.md`: archive-retention note added to the data
  deletion section (archives are not dropped; opt-outs apply to archives;
  restore via `retention import`).

## Tests added (targeted regression coverage)

- `sqlite/retentiondb_test.go`: archive across all three tables (exact counts,
  hot vs archive, idempotent second run), bounded batching (1200 rows),
  storage estimate, schema version, apply + budget-exceeded policy.
- `sqlite/retentionpkg_test.go`: export selects only expiring rows; no-op
  export writes nothing; export→delete→import round-trip is exact (unicode,
  embedded quotes/newlines, NULLs, JSON `story_data` preserved byte-for-byte);
  import idempotency (double import, no duplicates); FK-safe import with
  `foreign_keys=ON` (fresh rows' links survive); malicious-package rejection
  (extra manifest table, `DROP` statement, wrong format, row-count mismatch,
  path-traversal member); valid empty package imports.
- `turso/retention_test.go`: service over a real `*sql.DB` handle
  (driver-agnostic proof) covering export/apply/import/status; lazy
  `NewRetentionService` (no network at construction); first-operation failure
  with token redaction; `init()` DSN branch; empty-URL rejection.
- `storage/retention_test.go`: factory per backend — memory no-op semantics
  (export/import fail loudly, apply no-op, status ok), sqlite-file
  (migrated, schema 7), unknown backend error, turso empty-URL error.
- `cli/retention_test.go`: command wiring (all five subcommands), disabled
  config no-op, `--dry-run` skips apply, export failure aborts apply, export
  no-op message, import `--package` required, import success, status output.
- `config/config_retention_test.go`: env parsing (enabled/days/invalid),
  validation (negative rejected, zero accepted), env-file parsing.
- `types/deps_test.go`: `DefaultNewRetentionService` default constructor.

## Verification

- `make check` — **all checks passed**: build, vet, golangci-lint (0
  issues), `go test ./...`, coverage gates (all internal packages at/above
  thresholds; turso 74.4%, types back above 70%), gosec (0 issues; two
  path-scoped exclusions added to the Makefile's `-exclude-rules` for the
  import file-open G304 and the cobra flag-mark G104, consistent with the
  existing backup.go exclusions), govulncheck (no vulnerabilities).
- End-to-end smoke test with `HSC_STORAGE_BACKEND=sqlite-file`: seeded old +
  fresh rows across all three tables, ran `retention run` → package
  `retention-<UTC-stamp>.tar.gz` written with 3 rows (1 per table), apply
  archived exactly those 3 rows, `status` reports `enabled=true window=90
  schema_version=7 budget_ok=true`. Smoke-test artifacts removed.
- `refresh-news.yml` YAML validated; `huddlescope retention --help` renders.

## Key files

- `internal/domain/retention.go` (new) — contract + defaults.
- `internal/adapters/storage/sqlite/retentiondb.go` (new) — DB-level archive/estimate/version/apply.
- `internal/adapters/storage/sqlite/retentionpkg.go` (new) — package export/import + validation.
- `internal/adapters/storage/sqlite/retentionservice.go` (new) — `DBService`.
- `internal/adapters/storage/sqlite/retention.go` (refactored) — path-based API wraps DB-level.
- `internal/adapters/storage/turso/retention.go` (new) — Turso service (lazy, redacted).
- `internal/adapters/storage/retention.go` (+ `_test.go`) (new) — backend factory.
- `internal/cli/retention.go` (+ `_test.go`) (new) — `hs retention` command tree.
- `internal/cli/root.go`, `internal/cli/deps.go`, `internal/types/deps.go` (+ test) — wiring.
- `internal/config/config.go` (+ `config_retention_test.go`) — config surface.
- `.github/workflows/refresh-news.yml` — export → commit → apply steps + input.
- `Makefile`, `.env.example`, `archive/retention/README.md`, `docs/ops-runbook.md`, `docs/editorial-policy.md`.

## Deviations from the plan

- None material. The plan's `archive/retention/` location is used verbatim
  (the directory's README is the only committed content; packages land there
  at runtime). The in-memory backend is a documented no-op rather than a
  silent in-memory store, per AC-2 (packages must be persisted).
