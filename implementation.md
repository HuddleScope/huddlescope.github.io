# Implementation Notes — Issue #158 (HS-PORT-02): Portable Configuration and Runtime

## Summary

Made the `huddlescope` binary portable across VPS/Raspberry Pi (ingest to
Turso without repo/web-data writes) and GitHub Actions (export + publish
site-data files). Added config precedence (flags > env > file > defaults),
absolute path support with fail-fast validation, a `HSC_PUBLISH_SITE_DATA`
gate for site-data writes, an opt-in JSON run log, and deployment config
install tooling.

## Changes by file

### `internal/config/config.go`
- Added `Overrides` struct (`SourcesFile`, `TeamsFile`, `Timeout`, `LogLevel`)
  carrying CLI-flag values.
- `Load(path)` is now a wrapper around `LoadWith(path, o Overrides)`.
- `LoadWith` applies the precedence chain: defaults → config file → env → flag
  overrides (applied LAST so flags always win).
- `loadEnvFile` now parses `HSC_SOURCES_FILE`, `HSC_TEAMS_FILE`,
  `HSC_PUBLISH_SITE_DATA`, `HSC_JSON_LOG`.
- `loadFromEnv` now reads `HSC_PUBLISH_SITE_DATA` and `HSC_JSON_LOG` (lowercased).
- `applyOverrides` applies flag overrides (zero values ignored).
- New `Config` fields: `PublishSiteData` (string), `JSONLog` (string) —
  non-secret, included in `MarshalJSON` output.
- `ShouldPublishSiteData()` — `auto` (default: dev writes, production skips),
  `true` (always), `false` (never). Case-insensitive.
- `ShouldJSONLog()` — `true`/`1`/`yes`/`on` → true, else false.
- `Validate()` — rejects explicitly-set relative paths: input files
  (`SourcesFile`, `TeamsFile`) in all envs; output paths (`HeartbeatFile`,
  `FeedsFile`, `DataDir`, `HugoContentDir`, `StoragePath`) in production.
  Built-in defaults remain valid. `relativeExplicit` helper distinguishes
  operator-set relative paths from the default.

### `internal/types/deps.go`
- `LoadConfigFunc` changed to `func(path string, o config.Overrides)
  (config.Config, error)`.
- `DefaultLoadConfig` now calls `config.LoadWith`.

### `internal/cli/deps.go`
- `DefaultLoadConfig` updated to the new 2-arg signature.

### `internal/cli/root.go`
- Added persistent flags `--sources-file` and `--teams-file`.
- Updated `Long` help text with the precedence chain and absolute-path note.

### `internal/cli/flags.go` (new)
- `loadConfigFromFlags(cmd, d)` returns a `types.LoadConfigFunc` closure that
  reads the persistent flags (`config`, `sources-file`, `teams-file`,
  `log-level`, `timeout`) and calls `d.LoadConfig(cfgPath, overrides)`. Only
  explicitly-set flags (`Changed`) are applied. `lookupFlag` helper checks both
  local and persistent flag sets.

### `internal/cli/{ingest,export,config,heartbeat}.go`
- Wired `runner.WithLoadConfig(loadConfigFromFlags(cmd, d))` instead of
  `runner.WithLoadConfig(d.LoadConfig)`.

### `internal/cli/status.go`
- `openIngestRuns(cmd, d)` now uses `loadConfigFromFlags` (was
  `d.LoadConfig(cfgPath)`).

### `internal/cli/retention.go`
- `loadRetentionConfig` now uses `loadConfigFromFlags` (was
  `d.LoadConfig(cfgPath)`).

### `internal/runner/runner.go`
- Added `config` and `encoding/json` imports.
- `RunIngest`, `RunExport`, `CheckConfig` call `r.loadConfig("",
  config.Overrides{})` (new 2-arg signature).
- `RunIngest`: one-line memory/sqlite-file backend warning (stderr); gates
  heartbeat/feeds publishers on `cfg.ShouldPublishSiteData()`; opt-in JSON run
  log (`buildJSONRunLog`) when `cfg.ShouldJSONLog()`.
- `buildJSONRunLog` — strict field whitelist (time, run_id, origin, command,
  status, exit_code, committed, counters); no secrets.
- `orAuto` helper — normalizes empty `PublishSiteData` to "auto" for logs.

### `internal/config/config_test.go`
- Appended 14 new tests for LoadWith, ShouldPublishSiteData, ShouldJSONLog,
  Validate path validation.

### `internal/runner/runner_portable_test.go` (new)
- 9 tests for memory warning, site-data gating, JSON log.

### `internal/cli/portable_test.go` (new)
- 6 tests for persistent flags and flag precedence.

### Test double updates
- All `WithLoadConfig(func(path string) ...)` closures in `runner_test.go`,
  `runner_lease_test.go`, `root_test.go`, `runners_test.go`,
  `ingest_lease_test.go`, `status_test.go`, `deps_test.go` updated to the new
  2-arg `func(path string, _ config.Overrides) ...` signature.

### `.env.example`
- Added `HSC_SOURCES_FILE`, `HSC_TEAMS_FILE`, `HSC_PUBLISH_SITE_DATA`,
  `HSC_JSON_LOG` with documentation.

### `conf/deploy.production.example.env` (new)
- Example production deploy config (VPS).

### `conf/rpi.production.example.env` (new)
- Example Raspberry Pi production config with absolute paths under
  `/opt/huddlescope/`.

### `scripts/install-config.sh` (new, executable)
- Installs a deploy config with `0600` permissions
  (`deploy` → `/etc/huddlescope/deploy.env`, `rpi` →
  `/opt/huddlescope/huddlescope.env`).

### `.github/workflows/refresh-news.yml`
- Added `HSC_PUBLISH_SITE_DATA: "true"` to the build job `env:` block so
  GitHub Actions keeps the committed `web/data` files fresh.

### `README.md`
- Added "Portable Deployment" subsection under Configuration and new env var
  rows (`HSC_RUNNER_ORIGIN`, `HSC_PUBLISH_SITE_DATA`, `HSC_JSON_LOG`).

### `docs/portable-deployment.md` (new)
- Detailed guide: precedence, env vars, path independence, VPS/RPi/Actions
  procedures, fail-fast diagnostics, security, cross-compile reference.

## Design decisions

1. **Overrides travel through `LoadConfigFunc`**: The `LoadConfigFunc` type
   gained a `config.Overrides` parameter. The CLI wires
   `loadConfigFromFlags` which reads the persistent flags and calls
   `d.LoadConfig(cfgPath, overrides)`. The runner keeps calling
   `r.loadConfig("", config.Overrides{})`; the flags are resolved in the CLI
   closure. This avoids changing the runner's call shape while honoring the
   precedence chain.

2. **`ShouldPublishSiteData` default = env-dependent**: `auto` writes in
   dev/test and skips in production. This makes the repo-less production host
   safe by default (no `web/data` creation) while keeping dev usage working
   without config changes.

3. **Path validation only for explicitly-set paths**: The `relativeExplicit`
   helper rejects a path only if it's non-empty AND not the exact built-in
   default AND relative. This keeps the documented dev defaults (relative
   `conf/*.yaml`, `web/data`) working while rejecting operator-set relative
   paths.

4. **JSON log field whitelist**: `buildJSONRunLog` builds a struct with a fixed
   set of non-secret fields and marshals it. No config secrets (Turso token,
   DSN) are reachable from the function's inputs (`RunRecord`, `origin`
   string, `exitCode` int), so no secret can appear.

## Out of scope (handled by other issues)

- Dashboard reads from Turso (HS-PORT-07 / issue #154) — the run indicators
  are persisted to storage; the dashboard reads them later.
- Scheduler lease/identity (HS-PORT-04 / issue #160) — already merged; the
  workflow runs ingest as backup.
- Hugo build portability (HS-PORT-05 / issue #156) — not touched.
