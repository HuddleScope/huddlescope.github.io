# Operations runbook — Provisioning, refresh, failure diagnosis, and free tier

This runbook is written for a maintainer with zero prior knowledge of the
deployment. It covers first-time provisioning, manual refresh, failure
diagnosis, free-tier limits and a cost review, secret rotation, and local
execution. Related documents: `docs/release-checklist.md` (operator release
checklist + evidence rule), `docs/release-gate.md` (HS-058 release gate),
`docs/worker-deployment.md` (Cloudflare Worker first-time deployment).

## Initial provisioning (first time, from scratch)

Perform these steps once per environment. Secrets are referenced by **name
only**; never write secret values into the repo, this document, or workflow YAML.

### 1. Turso database and tokens

1. Create the database (Turso console → Databases → New, or CLI):
   ```bash
   turso db create huddlescope
   turso db show huddlescope          # note the database URL
   ```
2. Create an auth token for that database:
   ```bash
   turso db tokens create huddlescope
   ```
3. Set the repository secrets (GitHub repo → Settings → Secrets and variables →
   Actions, or CLI):
   ```bash
   gh secret set TURSO_DATABASE_URL -R HuddleScope/huddlescope.github.io
   gh secret set TURSO_AUTH_TOKEN -R HuddleScope/huddlescope.github.io
   ```
4. `refresh-news.yml` runs with `HSC_STORAGE_BACKEND=turso` and validates both
   secrets at startup: a missing or invalid value fails the ingest step with a
   clear, **redacted** error (no values are logged). Verify after setting them:
   dispatch the workflow (see Manual refresh) and confirm the ingest step passes.

### 2. Cloudflare Worker (scheduler)

The full first-time flow (Cloudflare account, fine-grained PAT, `wrangler
login`, `wrangler deploy`, `wrangler secret put GITHUB_TOKEN`, verification) is
in `docs/worker-deployment.md` (HS-053). Essentials:

- Worker `huddlescope-dispatcher` (source: `scheduler/`, config:
  `scheduler/wrangler.toml`), cron `7,37 * * * *` UTC (48 dispatches/day).
- The worker's `GITHUB_TOKEN` is a **Cloudflare secret**:
  ```bash
  cd scheduler
  npx wrangler secret put GITHUB_TOKEN
  npx wrangler deploy
  ```
- The worker's PAT must **never** be added to GitHub repo secrets or workflow
  YAML; it lives only in Cloudflare secret storage.

### 3. GitHub: environment, workflow, Pages

- Ensure the `github-pages` GitHub environment exists
  (repo → Settings → Environments). Both the `build` job (reads the environment
  for protection rules) and the `deploy` job (needs `pages: write` +
  `id-token: write`) use it.
- Pages source must be **GitHub Actions** (repo → Settings → Pages → Build and
  deployment → Source: "GitHub Actions"), **not** a branch. The deploy job
  (`actions/deploy-pages@v5`) publishes the `web/public/` artifact.
- Default branch: `main`.
- `secrets.GITHUB_TOKEN` is provided automatically by Actions for the
  checkout/commit steps; no manual token is needed for it.

### 4. Two separate secret stores — do not mix

| Store | Secret | Used by |
|-------|--------|---------|
| GitHub repo secrets (Actions) | `TURSO_DATABASE_URL`, `TURSO_AUTH_TOKEN` | `refresh-news.yml` (build job) |
| Cloudflare Worker secrets | `GITHUB_TOKEN` | `huddlescope-dispatcher` (cron dispatch) |

The Cloudflare worker `GITHUB_TOKEN` is **never** a GitHub repo secret and
**never** appears in workflow YAML. Rotating one does not affect the other
(see Secret rotation).

## Manual refresh

Dispatch a refresh run (with optional Pages deploy) and verify the result:

```bash
# Dispatch without deploy
gh workflow run refresh-news.yml -R HuddleScope/huddlescope.github.io --field publish=false

# Dispatch with deploy to Pages
gh workflow run refresh-news.yml -R HuddleScope/huddlescope.github.io --field publish=true

# Wait for completion (bounded; the build job takes ~5 minutes)
gh run list -R HuddleScope/huddlescope.github.io --workflow refresh-news.yml --limit 1 --json databaseId,url
gh run wait --run-id <RUN_ID> -R HuddleScope/huddlescope.github.io --timeout 15m
gh run view <RUN_ID> -R HuddleScope/huddlescope.github.io
```

Read the run's publish decision in the log ("Stories in exported snapshot: N
(committed: M)") to know whether the snapshot was committed and deployed.

**Reading the heartbeat.** Note that the refresh workflow's commit steps do
**not** push (see Known issues — missing push step): the committed
`web/data/heartbeat.json` is only updated by manual commits to `main`. To read
what the *latest run* produced, use the run log (step "Commit heartbeat" shows
the file content via the commit diff) or the committed file:

```bash
cat web/data/heartbeat.json                 # committed state (may be stale)
gh run view <RUN_ID> -R HuddleScope/huddlescope.github.io --log | grep -A5 'Commit heartbeat'
jq . web/data/heartbeat.json
```

Heartbeat status values: `ok` (ingestion and publication completed),
`partial` (completed with warnings, some sources failed), `failed` (critical
failure).

The committed `web/data/homepage.json` and `web/data/feeds.json` are likewise
only updated by manual commits; the live site is updated from run artifacts,
not from the committed files.

## Migrating from the legacy cron scripts (HS-PORT-09)

HuddleScope used to run news ingestion as a series of shell/cron steps
(`refresh-news.yml` dispatching ad-hoc scripts, per-feed curl fetches, a
separate score/rank script, and a hand-rolled "is it fresh?" check). Issue #165
(HS-PORT-09) consolidates all of that into the unified `huddlescope` CLI. The
legacy steps map onto CLI commands as follows;

| Legacy step (cron script)                        | Unified CLI command                              | Notes |
| ------------------------------------------------ | ------------------------------------------------ | ----- |
| Fetch all RSS feeds (per-feed `curl` + parse)    | `huddlescope ingest run --mode primary`          | The runner fetches every enabled source through the registry; a per-feed failure is non-fatal (the run is `partial`, not `failed`). |
| "Is the data fresh yet?" ad-hoc check            | `huddlescope ingest should-run --output json`    | Prints `{should_run, reason, last_successful_ingest_at}`; the reason is `never_succeeded`, `stale`, or `fresh`. Exit 0 either way; non-zero only on a DB query error (fail closed). |
| Record the run outcome / last success            | (automatic during `ingest run`) + `huddlescope ingest status` | Every run is recorded in the shared `ingest_runs` table; `ingest status` reads the last attempt/success from the shared source of truth. |
| Score + rank stories                             | (inside `ingest run`)                            | Scoring/ranking is part of the pipeline, not a separate script. |
| Export homepage.json + story pages               | `huddlescope export run`                         | Reads the stored snapshot and publishes `web/data/homepage.json` + story pages. `--dry-run` previews without committing. |
| Write heartbeat.json                             | (automatic during `ingest run`)                  | `heartbeat.json` + `feeds.json` are published as site-data when `HSC_PUBLISH_SITE_DATA=true` (GitHub Actions sets this; a repo-less VPS leaves it unset). |
| Heartbeat staleness gate (`heartbeat-check.yml`) | `huddlescope heartbeat check --file … --stale-after 90m` | Exit 1 if stale; a missing file is a clean exit 0 (first run). |
| Distributed mutual exclusion (one ingester)      | (automatic during `ingest run`)                  | The runner holds a shared `ingest_lease` (fenced, server-side time). `--mode backup` re-checks freshness before AND after acquiring the lease (TOCTOU-safe). |
| Validate config                                  | `huddlescope config check`                       | Validates and prints the resolved config (no network). |

### Command reference

```bash
# Ingest (primary host, e.g. the Raspberry Pi):
huddlescope ingest run --mode primary            # fetch + group + score + rank + commit
huddlescope ingest run --mode backup             # re-check freshness; skip if fresh (no fetch)

# Freshness decision (cron-ready; parse the JSON):
huddlescope ingest should-run --stale-after 75m --output json

# Run history (from the shared DB):
huddlescope ingest status --output json

# Export to Hugo (homepage.json + story pages):
huddlescope export run                # or --dry-run to preview

# Heartbeat staleness:
huddlescope heartbeat check --file web/data/heartbeat.json --stale-after 90m

# Validate config (no network):
huddlescope config check
```

**Configuration.** All settings come from environment variables (`HSC_*`), a
`.env`-format file via `--config`, or CLI flags (precedence: flags > env >
file > defaults). The storage backend is the single source of truth for run
history and the lease: `HSC_STORAGE_BACKEND=turso` (shared, remote) or
`sqlite-file` (a shared file for single-host/failover tests). In production the
backend must be non-memory.

**Offline test clock (dev only).** `HSC_FAKE_NOW=<RFC3339>` pins the freshness
decisions deterministically in `dev` (never honored in `production` — setting
it there fails closed). The e2e matrix uses it to test `should-run` freshness
without waiting real time. It is a test-only override and must never appear in a
production `.env`.

## Recovery: controlled cut and rollback (HS-PORT-09)

This is the procedure for switching the production host from the legacy cron
pipeline to the unified CLI (the "cut"), and the rollback if the cut goes
wrong. The cut is **controlled**: the legacy pipeline keeps running (read-only)
until the CLI's first run is verified healthy, and rollback is a one-line cron
change plus a heartbeat check.

### Pre-cut checklist

1. **Snapshot the legacy state.** Record the last successful legacy run and the
   current committed `web/data/heartbeat.json`:
   ```bash
   cp web/data/heartbeat.json /tmp/heartbeat.before-cut.json
   gh run list -R HuddleScope/huddlescope.github.io --workflow refresh-news.yml --limit 1 --json databaseId,url
   ```
2. **Verify the CLI against the same config.** Build and smoke the binary with
   the production `.env` (no network side effects from `config check`):
   ```bash
   make build
   ./dist/huddlescope config check     # must exit 0 against the production config
   ```
3. **Run the offline e2e matrix** to confirm the binary + failover paths are
   green on this build:
   ```bash
   make e2e                            # binary suite + 8 failover scenarios, offline
   ```
4. **Confirm the storage backend is shared and non-memory** (`turso` in
   production): `HSC_STORAGE_BACKEND` must be `turso` (or `sqlite-file` for a
   single shared file), and `TURSO_DATABASE_URL` / `TURSO_AUTH_TOKEN` present.
   `huddlescope config check` fails closed otherwise.
5. **Stakeholder sign-off** (the "two-host cut" needs an operator on the
   standby host): confirm who holds the production `.env` and the Turso tokens.

### The cut (steps a–i)

The cut is a sequence of controlled steps. Each step is independently
verifiable; stop and roll back (below) if any step fails.

| Step | Action | Verification |
| ---- | ------ | ------------ |
| **a** | On the primary host, **disable the legacy cron** (do not delete it yet — keep it for rollback): `sudo systemctl stop huddlescope-cron && sudo systemctl disable huddlescope-cron` (or comment out the crontab entries). | `systemctl is-active huddlescope-cron` reports `inactive`. |
| **b** | Install the unified binary: `make build` (or copy `dist/huddlescope` from a release). | `./dist/huddlescope version` prints the stamped version + revision. |
| **c** | Point the host's config at the shared storage backend: set `HSC_STORAGE_BACKEND=turso`, `TURSO_DATABASE_URL`, `TURSO_AUTH_TOKEN`, `HSC_ENV=production`, `HSC_RUNNER_ORIGIN=<raspberry\|vps>`. | `./dist/huddlescope config check` exits 0 and shows `turso`. |
| **d** | Do a **dry-run freshness read** to confirm the CLI sees the shared run history: `./dist/huddlescope ingest status --output json`. | The `state` is `healthy`/`degraded`/`unknown` (not a DB error). |
| **e** | Run the **first CLI ingest**: `./dist/huddlescope ingest run --mode primary`. | Exit 0; `Homepage committed successfully`; a new `ingest_runs` row appears. |
| **f** | Verify the **heartbeat + site-data** were published: `cat web/data/heartbeat.json` (GitHub Actions) or the run log. | `status` is `ok` (or `partial` with a known failing feed); `published_at` is fresh. |
| **g** | Run the **heartbeat gate**: `./dist/huddlescope heartbeat check --file web/data/heartbeat.json --stale-after 90m`. | Exit 0 (not stale). |
| **h** | (If two hosts) On the **standby host**, run `./dist/huddlescope ingest run --mode backup` to confirm it correctly **skips** when the primary is fresh: it must print "Ingest skipped" and exit 0 without fetching. | Exit 0; `ingest status` shows a `skipped_fresh` row from the standby origin. |
| **i** | **Enable the new cron** for the CLI: schedule `./dist/huddlescope ingest run --mode primary` (primary host) and `--mode backup` (standby) at the same cadence the legacy cron used. | `systemctl is-active huddlescope-unified` (or the new crontab entry) reports `active`; a `should-run` check confirms the cadence. |

### Rollback

Rollback is the inverse of the cut and is safe at any point before the legacy
cron is deleted:

1. **Disable the new CLI cron** (stop the unified service / remove the crontab
   entry).
2. **Re-enable the legacy cron** (the one disabled in step a): `sudo systemctl
   enable huddlescope-cron && sudo systemctl start huddlescope-cron`.
3. **Verify the legacy pipeline resumed**: dispatch a legacy refresh and confirm
   the committed `web/data/heartbeat.json` updates and the heartbeat gate passes:
   `./dist/huddlescope heartbeat check --file web/data/heartbeat.json --stale-after 90m`.
4. **Compare against the pre-cut snapshot** (`/tmp/heartbeat.before-cut.json`) to
   confirm the rollback restored the expected state.
5. Only after the rollback is verified **and** the root cause of the cut failure
   is understood, re-attempt the cut (or keep the legacy pipeline indefinitely —
   the CLI is additive; nothing is lost by rolling back).

**What is NOT lost on rollback.** The unified CLI writes to the *same* shared
storage (Turso / `ingest_runs` / `ingest_lease`) the legacy pipeline reads, and
it never mutates the committed site files outside the normal publish path. A
rollback therefore reverts to the legacy pipeline's behavior without data loss;
the only divergence is that the CLI's run-history rows remain in the shared DB
(visible via `ingest status`) — harmless.

### Evidence template (fill in after each cut attempt)

```
Cut date (UTC): 2026-11-25T15:00:00Z
Operator: <name>
Build: huddlescope version <v> (commit <sha>)
Primary host: <raspberry|vps>  Standby host: <raspberry|vps>
Step a–i results: PASS / FAIL (list any FAIL)
First CLI ingest run_id: <run-id>   status: ok|partial|failed
Heartbeat gate: exit 0 (not stale) / exit 1 (stale)
Standby backup skip: skipped_fresh confirmed / not confirmed
Outcome: CUT COMPLETE / ROLLED BACK
Rollback reason (if any): <root cause>
```

## Deciding staleness

A heartbeat is stale when `published_at` is missing, unparseable, or older than
90 minutes (the `heartbeat-check.yml` gate uses the same threshold).

```bash
# Check committed heartbeat staleness (exit 1 if stale)
./dist/huddlescope heartbeat check --file web/data/heartbeat.json --stale-after 90m

# View file content and assess
jq . web/data/heartbeat.json
```

`make build` produces the single `dist/huddlescope` binary (unified CLI:
`ingest run`, `export`, `heartbeat check`, `config`; persistent `-config` flag
for a `.env`-format file — see `.env.example`).

## Diagnosing a failed refresh

Map the symptom to the workflow step in `.github/workflows/refresh-news.yml`,
then apply the action. The failure matrix below mirrors the release-gate matrix
in `docs/release-gate.md`.

| Symptom | Likely cause | Action |
|---------|--------------|--------|
| Build job fails in "Run ingest pipeline" with a redacted storage error | Missing/invalid `TURSO_DATABASE_URL` or `TURSO_AUTH_TOKEN` (validated at startup) | Re-set repo secrets (Initial provisioning §1); re-dispatch |
| Build job fails in "Run ingest pipeline" with feed errors | RSS fetch failures (timeouts, network) | Per-source failures are isolated; a total feed failure fails the run. Check `conf/sources.yaml` `enabled` flags; re-dispatch after network issues |
| Build job fails in "Export snapshot to Hugo data" | Export error (e.g., storage read failure) | Read the step log; verify Turso access; re-dispatch |
| Run succeeds but site unchanged; log says "retaining last-known-good homepage (no deploy)" | Export produced 0 stories **while a prior real snapshot exists** — publish decision intentionally skips commit **and** deploy | Not an incident: last-good content is retained by design (HS-056). Investigate why feeds yielded nothing (see export-empty row) |
| Run succeeds; site shows "No stories published yet" | Zero stories and **no prior real snapshot** (never a successful export) — publish decision deploys the labelled empty state | Expected first-run behaviour (HS-056). Investigate the empty export: in run log "Publish decision" ("Stories in exported snapshot: 0"), check ingest article age vs. `HSC_LOOKBACK=24h`; the production database may hold only stale articles |
| Run fails in "Verify no demo markers" | Sample-data signatures (`example.com`, `seed-run-`, demo story IDs) in `web/public/` | A local fixture leak or a committed demo file. Do **not** bypass the gate; fix the data source, rebuild |
| Deploy job fails ("Deploy to Pages") | Pages upload/deploy failure (permissions, artifact) | Check deploy job log; verify `github-pages` environment, Pages source = GitHub Actions, `pages: write` + `id-token: write` permissions |
| `heartbeat-check` workflow red while `refresh-news` is green | Committed `web/data/heartbeat.json` is stale (>90 min). **Known cause:** the refresh workflow has no `git push` step, so its commits never reach `main` (see Known issues) | Until fixed: manually commit a fresh `web/data/heartbeat.json` to `main`. File/track the workflow fix so data files track real runs |
| No new Actions runs for >35 min | Worker cron not firing: expired/invalid Cloudflare `GITHUB_TOKEN`, or scheduled-event quota exhausted | Check Cloudflare dashboard (worker logs: 401 = token issue; quota = free-tier limit); rotate the worker `GITHUB_TOKEN` (`wrangler secret put GITHUB_TOKEN`) |
| Run succeeds, export nonempty, but live site shows old content | Pages deploy skipped by the publish decision, or deploy job failed | Compare the run's publish decision and deploy job status with the live site's "Data as of" timestamp |

## Free-tier topology and cost review

HuddleScope is designed to run entirely on free tiers. Verified 2026-11-24
against the live deployment and official documentation:

| Component | Free-tier limit | Current usage | Notes |
|-----------|----------------|---------------|-------|
| Cloudflare Workers | Per `scheduler/wrangler.toml` comment (citing the Cloudflare limits page): 100k requests/day; 50 scheduled events/day | 48 cron triggers/day | Cron `7,37 * * * *` UTC. The 48/50 proximity is a **known exposure**: re-verify the current official free-plan scheduled-event limit (https://developers.cloudflare.com/workers/platform/limits/) before relying on it; if the limit is ≤48, reduce cron frequency or move off free tier (follow-up issue) |
| GitHub Actions | Standard GitHub-hosted runners in **public** repositories are free (this repo is public; https://docs.github.com/en/billing/managing-billing-for-github-actions/about-billing-for-github-actions) | ~5–10 minutes/run estimate × 48 runs/day | Estimates are estimates: measure with `gh run list` + run durations |
| GitHub Pages | 1 GB storage (GitHub Free plan) | <10 MB (site + assets) estimate | Static site only; verify in repo → Settings → Pages |
| Turso Cloud | Free database storage per Turso's current free plan (re-verify at https://turso.tech/pricing/) | <100 MB estimate | Check with `turso db info huddlescope` |

**Conclusion:** zero paid services are required by this configuration. The only
free-tier number that cannot be re-verified from the repository is the
Cloudflare scheduled-event limit (recorded above as a known exposure). No
`wrangler.toml` change is made here; if the official limit is lower than
48/day, file a follow-up issue to reduce cron frequency or upgrade the plan.

**Usage check commands** (repeatable):

```bash
# GitHub Actions: recent runs and their durations
gh run list -R HuddleScope/huddlescope.github.io --limit 50 --json databaseId,createdAt,displayTitle

# Turso storage
turso db info huddlescope

# Live site size (rough)
curl -s https://huddlescope.github.io/sitemap.xml | grep -c '<loc>'
```

**What degrades first** (and how to detect):
- Cloudflare Workers: Workers dashboard shows request count and scheduled-event
  count; if approaching the limits, trim sources or reduce cron frequency.
- GitHub Actions: usage visible in repo → Settings → Actions → Logs / billing
  dashboard; minutes are free for standard runners in public repos, but
  artifact storage (500 MB free) is the first thing to exhaust — the workflow
  uploads `web/public/` with `retention-days: 7`.
- Turso storage: `turso db info` or the Turso console; if approaching the free
  limit, archive old data (HS-041, implemented in the sqlite package).

## Free-tier monitoring

| Metric | Alert threshold | Location |
|--------|-----------------|----------|
| Turso storage | 900 MB | Turso Cloud console / `turso db info` |
| Cloudflare Worker requests | 90k/day | Cloudflare dashboard |
| GitHub Actions artifact storage | 400 MB of the 500 MB free allowance | GitHub billing dashboard |
| Data freshness | Ingest: Turso `ingest should-run --stale-after 90m`; Publication: HTTP fetch of deployed `data/heartbeat.json` `published_at` > 90 min | `heartbeat-check.yml` (Turso-backed + HTTP fetch, not checkout files) |

**Response to warnings**:
- Turso near limit: archive old data (HS-041, implemented in sqlite package) or review sources for trimming
- Cloudflare near limit: reduce cron frequency or trim sources
- Actions artifact storage near limit: lower `retention-days` on the site-build artifact

## Feed disable procedure

To disable a feed source without deleting it permanently:

1. **Disable in configuration**
   ```yaml
   # conf/sources.yaml
   sources:
     - id: source-id
       name: Source Name
       # ... other fields
       enabled: false  # change to false
   ```

2. **Commit and deploy**
   - Commit the change to `conf/sources.yaml` on `main`
   - The next scheduled refresh (≤30 min) stops polling the feed; no manual
     pipeline run is required in production (locally:
     `./dist/huddlescope ingest run` then `./dist/huddlescope export`)

3. **Re-enable**
   - Set `enabled: true` and re-commit
   - The next scheduled run will resume data collection

## Turso backup/restore

See `internal/adapters/storage/sqlite/backup.go` and
`internal/adapters/storage/sqlite/retention.go` for the in-repo implementation
(HS-041).

### Operational backup (Turso Cloud)

1. **Export from Turso**
   ```bash
   # Via Turso Cloud console: Database > Export > Download
   # Or via CLI:
   turso db shell huddlescope "SELECT * FROM stories;" > stories_dump.sql
   ```

2. **Backup workflow** (manual, quarterly recommended)
   - Export current database to a local SQL file
   - Store the file securely (not in the repo; it may contain content identifiers)
   - Verify restore on a fresh database before relying on it

3. **Restore from backup**
   - Create a fresh Turso database
   - Run the SQL dump: `turso db shell new-db < backup.sql`
   - Point your application to the new database

### In-repo backup functions (HS-041, implemented)

The following functions are available in the sqlite package:

- `BackupToFile(dbPath, backupPath)` — full Turso-compatible backup
- `RestoreFromFile(backupPath, dbPath)` — restore from backup file
- `Dump(dbPath)` — SQL dump as string
- `DumpToFile(dbPath, dumpPath)` — SQL dump to file
- `ApplyRetention(dbPath)` — archive rows older than 90 days in bounded batches

These functions are now wired into the retention pipeline via the
`huddlescope retention` CLI (issue #120) and the `run-retention` step in the
`refresh-news.yml` workflow.

## Retention and archiving (HS-065 / issue #120)

Hot tables (`articles`, `stories`, `ranking_snapshots`) are bounded to a
retention window (default **90 days**, `HSC_RETENTION_DAYS`). Rows older than
the window are moved into `*_archive` mirrors (they stay in the same database;
the archives are *not* dropped). Before archiving, the expiring rows are
exported to a portable package that is committed to `archive/retention/` so
nothing is ever lost.

### How the pipeline runs

1. **Export** — `huddlescope retention export --output archive/retention`
   writes rows older than the window to `retention-<UTC-stamp>.tar.gz`
   (format `huddlescope-retention-package`, v1: `manifest.json` +
   `tables/{articles,stories,ranking_snapshots}.sql` with `INSERT OR REPLACE`
   statements in bounded 500-row transactions). Export never deletes anything.
   A run with no expiring rows writes nothing.
2. **Commit** — the workflow commits the new package to `archive/retention/`
   in Git *before* archiving. If the export or commit fails, the build job
   fails and archiving never runs (packages must be persisted — AC-2).
3. **Apply** — `huddlescope retention apply` archives the expiring rows in
   bounded batches and checks the storage budget.
4. **Import (restore)** — `huddlescope retention import --package <file>`
   validates the package (format v1, whitelisted tables, statement classifier,
   size cap) and restores rows into the hot tables. Import is idempotent:
   re-importing the same package does not duplicate rows.

### Local usage

```bash
# Full local run (export + apply)
huddlescope retention run
# Export only (no archiving)
huddlescope retention run --dry-run
# Individual steps
huddlescope retention export --output archive/retention
huddlescope retention apply
huddlescope retention import --package archive/retention/retention-20260101T000000Z.tar.gz
huddlescope retention status
```

Disable retention locally or in CI with `HSC_RETENTION_ENABLED=false`.

### Restore procedure (archive → hot)

1. Identify the package in `archive/retention/` (chronological by timestamp).
2. Verify it: `./dist/huddlescope retention import --package archive/retention/<pkg>.tar.gz`
   (idempotent; safe to run twice).
3. If the restore must roll back, delete the restored rows by their primary
   keys (the package manifest lists per-table row counts) — or restore the
   database from a Turso Cloud backup taken before the import.

### Safety properties

- Packages contain **only** the three whitelisted hot tables — never sources,
  tokens, correction rules, or link tables.
- Import executes only `INSERT OR REPLACE` / `BEGIN` / `COMMIT` statements
  classified against the file's declared table; anything else aborts the
  import.
- The manifest pins `format` + `version: 1`; unknown formats or versions are
  rejected rather than guessed.
- Packages are portable: no absolute paths, no foreign keys, stable primary
  keys.

## Secret rotation

### GitHub Actions secrets
- `TURSO_DATABASE_URL`
- `TURSO_AUTH_TOKEN`
- `GITHUB_TOKEN` (provided automatically by Actions; not a manually stored secret)

**Rotation procedure** (Turso pair):
1. Generate a new token in Turso (`turso db tokens create huddlescope`) and
   revoke the old one after the new one is verified.
2. Update in GitHub repository settings → Secrets and variables → Actions
   (`gh secret set …`).
3. Dispatch a refresh run (`gh workflow run refresh-news.yml … --field publish=false`)
   and verify the ingest step passes.

### Cloudflare Workers secrets
- `GITHUB_TOKEN` (worker-specific, 90-day recommended)

**Rotation procedure**:
1. Generate a new token at GitHub → Settings → Developer settings → Personal
   access tokens (fine-grained, repo scope: workflows).
2. Update in worker: `cd scheduler && npx wrangler secret put GITHUB_TOKEN`
3. Re-deploy: `npx wrangler deploy`
4. Verify dispatch works: `gh run list -R HuddleScope/huddlescope.github.io
   --workflow refresh-news.yml --limit 5` shows fresh cron-triggered runs.

These two stores are independent (see Initial provisioning §4): rotating the
worker token never touches repo secrets, and vice versa.

## Local execution steps

### Prerequisites (versions CI actually uses)
- Go 1.27.4 (`go.mod`; CI installs from `go.mod` via `go-version-file`)
- Node.js ≥20 for the scheduler (`scheduler/package.json` engines); CI installs Node 22
- Hugo 0.169.0 (extended) — pinned in `refresh-news.yml` and `site.yml`
- wrangler `^4.163.0` (`scheduler/package.json` devDependency)
- `wrangler` CLI for Cloudflare Worker deployment (if needed)

### Quick start
```bash
# Clone and setup
git clone https://github.com/HuddleScope/huddlescope.github.io.git
cd huddlescope.github.io
cp .env.example .env
# Edit .env with your local values (optional; defaults will be used if omitted)

# Build the unified Go binary
make build
# Binary appears in dist/huddlescope

# Run ingest (fetch feeds) — local default backend is memory/sqlite-file, not turso
./dist/huddlescope ingest run
# or with a .env-format config file:
./dist/huddlescope ingest run -config .env

# Export snapshot to Hugo data
./dist/huddlescope export
# or with config:
./dist/huddlescope export -config .env

# Check heartbeat staleness
./dist/huddlescope heartbeat check --file web/data/heartbeat.json --stale-after 90m

# Build Hugo site
make site
# or run local dev server
make site-watch

# Run site verification
make site-test

# Run all checks (CI pipeline)
make check
```

Local development: point ingest/export at the gitignored fixture dir
(`HSC_HEARTBEAT_FILE`, `HSC_FEEDS_FILE`, `HSC_DATA_DIR`, `HSC_DATA_FILE` — see
`.env.example`) so a local run never dirties the tracked `web/data/` files, and
use `make site-seed` for representative sample content (see
`web/README-data.md`).

### Make targets
- `make help` — Show all available targets
- `make build` — Build the unified `dist/huddlescope` binary
- `make fmt` — Format Go files
- `make lint` — Run golangci-lint (may fail locally due to Go version mismatch; rely on CI)
- `make test` — Run vet and tests with race detector
- `make coverage` — Run tests with coverage report
- `make security` — Run gosec and govulncheck (may fail locally; rely on CI)
- `make check` — Run fmt, lint, test, coverage, security
- `make site` — Build Hugo site
- `make site-test` — Run site verification script
- `make site-watch` — Run Hugo server for local development

## Cloudflare cron and GitHub Pages configuration

See `docs/worker-deployment.md` for step-by-step first-time deployment (Cloudflare
account, token creation, `wrangler login`, verification).

### Cloudflare Worker cron
- Worker name: `huddlescope-dispatcher`
- Cron schedule: `7,37 * * * *` UTC (every 30 minutes, 48 dispatches/day)
- Endpoint: POST to GitHub Actions dispatch for `refresh-news.yml`
- Timeout: 10 seconds per dispatch (`REQUEST_TIMEOUT_MS` in `scheduler/src/index.js`)
- Retry: exponential backoff with Retry-After support, bounded attempts (see `scheduler/src/index.js`)

### GitHub Actions workflow
- File: `.github/workflows/refresh-news.yml`
- Trigger: `workflow_dispatch` (manual or via Cloudflare Worker cron)
- Parameters:
  - `publish` (boolean, default true): Deploy to Pages after successful build
  - `run-retention` (boolean, default true): Run retention after build
  - `force_ingest` (boolean, default false): Force backup ingest even if Turso is fresh (maintenance only; still respects the distributed lease)
- Jobs:
  - `build`: checkout → setup Go → build → **Decide should-run** (Turso freshness) → **Backup ingest** (gated on should_run || force_ingest) → **Export** (always; re-publishes site-data from DB) → **Publish decision** (gated on export success) → **Commit+push site-data** (gated on export success; `[skip ci]`; `contents: write` at step level) → install Hugo → site verification → build site → demo-marker gate → retention → upload artifacts → Pages upload (gated)
  - `deploy`: configure Pages → upload artifact → deploy (only if `publish=true` **and** the publish decision is `true`)
- Permissions: least privilege; default `contents: read`; the site-data push step gets `contents: write` at step level; deploy job requires `pages: write` and `id-token: write`
- **Fixed (issue #163):** the single `commit-and-push-site-data` step stages `web/data/{heartbeat,feeds,homepage}.json`, commits with `[skip ci]`, and pushes. The commit is gated on `steps.export.conclusion == 'success'`.

### Publish decision step (HS-056)
After export, the workflow compares the freshly exported story count with the
previously committed snapshot (`git show HEAD:`):
- ≥ 1 stories → commit the snapshot and deploy to Pages
- Zero stories, but the committed snapshot previously had ≥ 1 story → skip the homepage commit and the Pages deploy; the last successfully deployed (real) content is retained
- Zero stories, and no prior real snapshot (never a successful export) → deploy the clearly-labelled "No stories published yet" empty state with a truthful `generatedAt`
- A failed ingest or export fails the build job, so the deploy job is skipped and the last-good Pages deployment is preserved
The build also greps `web/public/` for sample-data markers (`example.com`, `seed-run-`, demo story IDs) and hard-fails before any Pages upload, so sample content can never be presented as current NFL news (the old "data guard" restore of a committed demo file was removed). See `web/README-data.md` for the full contract.

### Pages configuration
- Environment: `github-pages`
- Source: **GitHub Actions** (not a branch)
- Branch: `main`
- Deploy preview: disabled (require approval for PR branches)

## Alerting

GitHub Actions provides email/Slack notifications per workflow run. Configure in repository settings:

- Workflow run success/failure notifications
- Individual job failure notifications

Heartbeat staleness is visually indicated in the `heartbeat-check` workflow run
output (it fails the run when the committed heartbeat is missing,
unparseable, or older than 90 minutes).

## Backup ingest procedure (issue #163 / HS-PORT-07)

The GitHub Actions backup ingest runs only when Turso is stale (>75 min). The
`should-run` decision step queries Turso and gates the ingest step. When
`force_ingest=true` is set, the staleness pre-check is skipped but the
distributed lease is still enforced (a held lease produces `skipped_locked`).

### Manual forced ingest (maintenance)

1. Dispatch the workflow with `force_ingest=true`:
   ```bash
   gh workflow run refresh-news.yml -R HuddleScope/huddlescope.github.io \
     -f publish=false -f force_ingest=true
   ```
2. Monitor the run: `gh run watch` or check the Actions log.
3. The ingest step will run `ingest run --mode backup --stale-after 75m --force`.
   If another host holds the lease, it will be skipped (`skipped_locked`) and
   exit 0 — this is expected, not an error.
4. The export step always runs and re-publishes site-data from the DB.

### Verifying ingest freshness from Turso

```bash
# From a host with Turso credentials:
HSC_STORAGE_BACKEND=turso \
HSC_TURSO_DATABASE_URL=$TURSO_DATABASE_URL \
HSC_TURSO_AUTH_TOKEN=$TURSO_AUTH_TOKEN \
./dist/huddlescope ingest should-run --stale-after 75m --output json

# Expected output:
# {
#   "should_run": false,
#   "reason": "last successful ingest is fresh (age 12m0s <= threshold 1h15m0s)",
#   "last_successful_ingest_at": "2026-01-15T11:48:00Z"
# }
```

## Heartbeat health check (issue #163 / AC-9)

The `heartbeat-check.yml` workflow is split into two independent checks:

1. **Ingest freshness (Turso)** — `huddlescope ingest should-run --stale-after 90m --output json`.
   Fail-closed on DB error. Answers "did any host successfully ingest recently?"
2. **Publication freshness (deployed site)** — HTTP `curl` of
   `https://huddlescope.github.io/data/heartbeat.json`, parse `published_at`,
   compare to 90-minute threshold. Answers "is the live site fresh?"

Neither check reads the checkout's `web/data/heartbeat.json` (a historical
repo file) as a remote signal.

## Known issues

- **Fixed (issue #163):** The missing push step is now the single
  `commit-and-push-site-data` step, gated on export success, with `[skip ci]`
  and step-level `contents: write`.
- **Fixed (issue #163):** The heartbeat check no longer reads checkout files;
  it uses Turso-backed `ingest should-run` and an HTTP fetch of the deployed
  site's `data/heartbeat.json`.
- **Empty production export (verified 2026-11-24):** the export writes 0
  stories to `homepage.json` while Turso holds 3017 ranked stories older than
  the 24 h lookback; the live site shows the labelled empty state (correct
  HS-056 behaviour). Follow-up issue required so fresh articles are published.
- The refresh workflow runs in production with `HSC_STORAGE_BACKEND=turso`.
  The Turso backend is implemented via `libsql-client-go`
  (`internal/adapters/storage/turso`); production mode requires
  `TURSO_DATABASE_URL` and `TURSO_AUTH_TOKEN`, which are validated at startup
  (a missing value fails the ingest step with a clear, redacted error). Local
  runs can use `sqlite-file` or `memory`.
- The heartbeat file is committed to the repo; the refresh workflow does not
  push any data back to the default branch (see missing push step above).
- Backup/restore code implemented in sqlite package (HS-041), not yet wired
  into the pipeline
- Retention/archiving implemented in sqlite package (HS-041), not yet wired
  into the pipeline

## Correction rules: story lifecycle and aliases (issue #26)

Correction rules are stored overrides applied after clustering. Issue #26 adds
lifecycle and alias kinds alongside the existing split/merge kinds. Rules
persist in the `correction_rules` table (migration 007); the `kind` column is
untyped TEXT, so new kinds require **no schema migration**. Lifecycle rules
reference stories by their **stable slug** (carried in the `story_id` column),
not by group ID.

### Rule kinds

| Kind | `story_id` | `into_story_id` | `article_ids` | Effect |
|------|-----------|----------------|--------------|--------|
| `split` | group to split (group ID) | — | articles to remove | Re-partitions the cluster |
| `merge` | source group (group ID) | target group (group ID) | — | Folds source into target; records an alias |
| `archive` | story slug | — | — | Sets status to `archived` |
| `close` | story slug | — | — | Sets status to `closed` |
| `reactivate` | story slug | — | — | Sets status back to `active` |
| `alias` | target story slug | alias slug to record | — | Appends an explicit alias (representative-swap escape hatch) |

### Applying a lifecycle rule

1. **Determine the story slug.** The slug is the stable public identity, derived from the representative article's headline. Read it from the story page's `slug:` front-matter (e.g. `/stories/{slug}/`).
2. **Save the rule.** Use the `CorrectionRuleRepository` (`SaveRule`) with the appropriate kind. For `archive`/`close`/`reactivate`, set `StoryID` to the slug. For `alias`, set `StoryID` to the target slug and `IntoStoryID` to the alias slug.
3. **Verify.** The next `RunStoryPages` pass applies the rule. The story page will carry `status: "archived"` (or `"closed"`) front-matter and an archive banner; alias rules append to the `aliases:` front-matter and "Merged From" section.

### Slug validation

Lifecycle slugs are restricted to the `[a-z0-9-]` charset (the same charset
`ComputeStorySlugFromHeadline` emits). Slugs with uppercase, underscores,
slashes, or whitespace are rejected at `Validate()` time — this keeps them
path-safe and G306-safe.

### Known limitations

- **No undo of a merge.** Merging records an alias but does not re-split articles. To revert, you would need to manually re-seed the articles.
- **No undo of an archive automatically.** A `reactivate` rule restores `active` status but does not recreate removed articles.
- **Ambiguous slugs fail loudly.** If a lifecycle rule's slug matches more than one group's representative slug, `ApplyLifecycle` returns an error and the partition is left unmodified. This can occur after a split of a same-title group.
- **Alias cap.** Alias lists are capped at 20 entries (front-matter and body) to keep output bounded for very large merges.
- **Representative swap.** If a story's representative article changes (e.g. a new earlier-dated source arrives), the slug changes and the old slug is recorded as an alias automatically (via merge) or can be recorded explicitly (via `alias` rule). The merged page takes over the source's URL via Hugo-native `aliases:` front-matter.

## Team registry

The team registry is now stored in `conf/teams.yaml` and loaded via
`config.TeamRegistry()`. The former database tables (`teams` and
`article_teams`) were orphaned and removed in migration 005 (HS-063).

The `ranking_snapshots` table retains a `team_id` column but it is now a plain
business key, not a foreign key to the teams table. This allows retention
archiving and backups to continue working without referencing the dropped
tables.

To view or interact with team data:
1. Inspect `conf/teams.yaml` directly
2. Use the application's team registry API via `config.TeamRegistry()` in code
3. The ranking system uses team identity from the configuration, not the database

## Ingest run history and freshness (issue #159 / HS-PORT-03)

The `ingest_runs` table (migration 008) records one row per ingest run —
run-level history shared by all hosts (Raspberry, VPS, GitHub Actions) via
Turso. It complements, and does not duplicate, the per-feed `feeds.json`
health data and the publication heartbeat.

### Retention limit

Rows are small (~1 KB) and bounded (~50/day across all hosts). There is no
automatic prune; recommend pruning rows older than 90 days
(`domain.DefaultRetentionDays`) manually or via a cron:

```sql
DELETE FROM ingest_runs
WHERE COALESCE(finished_at, started_at) <
      strftime('%Y-%m-%dT%H:%M:%SZ','now','-90 days');
```

### Ingest health vs publication health

- **Ingest health** (`ingest status` / `ingest should-run`): DB-sourced, from
  `ingest_runs`. Answers "did any host successfully ingest recently?"
- **Publication health** (`heartbeat check --file`): file-sourced, from
  `web/data/heartbeat.json`. Answers "was the last homepage committed?"

These are different concepts and are intentionally not merged.

### `should-run` caller contract (fail closed)

`huddlescope ingest should-run` exits 0 on a correct query **regardless** of
the `should_run` true/false decision. It exits non-zero (with no verdict on
stdout) when the database is unreachable, the schema is invalid, or the
backend is not a shared source of truth (e.g., `memory`). **Callers must treat
a non-zero exit as "cannot decide" and must NOT default to running** — a blind
backup run on DB failure is exactly the failure mode this issue removes.

## Distributed mutual exclusion for ingest (issue #160 / HS-PORT-04)

Multiple hosts (Raspberry/VPS primary + GitHub Actions backup) can run the
ingest pipeline. A shared-database lease guarantees **exactly one host does RSS
work at a time**; the other omits visibly and does **not** count as a false
error.

### The lease

- **Table:** `ingest_lease` (migration 009). Single fixed key `rss-ingest`.
- **Columns:** `holder_id`, `fencing_token` (monotonic), `lease_until`,
  `updated_at`. All timestamps are **server-side** (SQL `now`); a skewed client
  clock can never grant or expire a lease.
- **TTL:** 20 minutes. **Renewal:** every 5 minutes (must stay < TTL so a
  single network blip does not cause a spurious hand-off).
- **Fencing:** each successful (re)acquire increments `fencing_token`. Renew and
  Release are both guarded on `(holder_id, fencing_token)`, so a lagged holder
  whose token was advanced by a newer acquirer can neither renew nor release —
  it stops making changes (its in-flight upserts are bounded by the existing
  GUID/canonical-URL dedup and are harmless no-op duplicates).

### `ingest run` modes and flags (now live, previously hidden no-ops)

| Flag | Meaning |
|------|---------|
| `--mode primary` | Acquire the lease and run. If held, register `skipped_locked`, print a visible skip, exit 0. |
| `--mode backup` | Pre-check freshness (skip + exit 0 if fresh, no lease touch); then acquire; then **re-check freshness inside the held lease** just before fetching (TOCTOU). If fresh in lock, release, register `skipped_fresh`, exit 0 — no RSS work. |
| `--stale-after 75m` | Freshness threshold for the backup checks. |
| `--force` | Skips the staleness **pre-check only**. It **NEVER** skips the lease. Documented manual operation. |

Exit codes: success and all skips → `0`; lease DB failure, lost lease during
the pipeline, or pipeline failure → non-zero (fail closed; never alters
`last_successful_ingest_at`).

### New run statuses (migration 009 widens the `ingest_runs` CHECK)

- `skipped_locked` — could not acquire the lease (another host holds it).
- `skipped_fresh` — held the lease but the in-lock re-check said fresh.

Both are **non-success**: `IsSuccess()` stays `ok||partial`, so they are
invisible to `LastSuccess` / `DecideShouldRun` and never update
`last_successful_ingest_at` (the issue requirement).

### Failure recovery (no manual row deletion)

If the primary dies, its lease simply **expires** after the 20-minute TTL.
Another host's next `Acquire` is a single conditional upsert that takes over the
expired row (fencing token +1) — **recovery is automatic; never delete the
`ingest_lease` row by hand.** An expired row is just re-acquired.

### Deploy order (load-bearing)

Migration 009 widens the `ingest_runs` status CHECK and adds `ingest_lease`.
Because the rebuild is forward-only:

1. **Deploy the new binary to ALL hosts before the backup starts dispatching**
   with `--mode backup`. Old binaries do not understand `skipped_*` rows.
2. The PR ships the binary + workflow together; the workflow ingest step now
   passes `--mode backup --stale-after 75m`.
3. Rollback: reverting the PR is safe — the residual schema is harmless (old
   binaries never read `ingest_lease`; the widened CHECK is backward compatible
   for all old status values). If a rolled-back host serves `ingest status`
   while new hosts write `skipped_*` rows, `status` exits non-zero on that host
   (freshness queries are unaffected — they filter `ok`/`partial` in SQL).

### Backends

- `turso` and `sqlite-file` backends provide the lease (shared store).
- The `memory` backend **cannot** coordinate across hosts: the factory returns
  `ErrLeaseUnavailable`, the runner warns and proceeds without mutual
  exclusion. Use this only for local/dev; never for two live hosts.

Remote coordination is **database-only** (no local files/flock), per the issue.
Writes are assumed to hit the single Turso primary node.
