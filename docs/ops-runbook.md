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
| Data freshness | Heartbeat > 90 minutes stale | `./dist/huddlescope heartbeat check --file web/data/heartbeat.json --stale-after 90m` |

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
- Jobs:
  - `build`: checkout → setup Go → build → ingest → export → commit heartbeat → publish decision → commit homepage (gated) → commit feed health → install Hugo → site verification → build site → demo-marker gate → upload artifacts → Pages upload (gated)
  - `deploy`: configure Pages → upload artifact → deploy (only if `publish=true` **and** the publish decision is `true`)
- Permissions: least privilege; deploy job requires `pages: write` and `id-token: write`
- **Known gap:** the commit steps (heartbeat/homepage/feeds) create local
  commits but the workflow has no `git push` step, so they never reach `main`.
  See Known issues and the diagnosis table above.

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

## Known issues

- **Missing push step (verified 2026-11-24):** `refresh-news.yml` commits
  `web/data/heartbeat.json`, `homepage.json`, and `feeds.json` locally but
  never pushes them, so the committed data files are stale and the
  `heartbeat-check` workflow fails on staleness while `refresh-news` stays
  green. Follow-up issue required. Evidence:
  https://github.com/HuddleScope/huddlescope.github.io/actions/runs/45410553677
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
