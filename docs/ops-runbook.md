# Operations runbook — Publication heartbeat, free tier, and operations

## Manual dispatch

To manually trigger a refresh and optional Pages deployment:

```bash
# Basic dispatch (no deploy)
gh workflow run refresh-news.yml -R HuddleScope/huddlescope.github.io --field publish=false

# Dispatch with deploy to Pages
gh workflow run refresh-news.yml -R HuddleScope/huddlescope.github.io --field publish=true
```

Check run status:

```bash
gh run list -R HuddleScope/huddlescope.github.io --limit 10
gh run view <RUN_ID> -R HuddleScope/huddlescope.github.io
```

## Reading the heartbeat

After dispatch completes, check the heartbeat file:

```bash
# View committed heartbeat
gh run download <RUN_ID> -R HuddleScope/huddlescope.github.io
cat web/data/heartbeat.json

# Or locally after checkout
cat web/data/heartbeat.json
```

Heartbeat status values:

- `ok`: Successfully completed ingestion and publication
- `partial`: Completed with warnings (some sources failed)
- `failed`: Critical failure (run did not complete)

## Deciding staleness

A heartbeat is stale when `published_at` is missing, unparseable, or older than 90 minutes.

Manual check:

```bash
# Check heartbeat staleness (exit 1 if stale)
dist/heartbeat check --file web/data/heartbeat.json --stale-after 90m

# View file content and assess
jq . web/data/heartbeat.json
```

## Free-tier topology and zero-cost defaults

HuddleScope is designed to run entirely on free tiers:

| Component | Free tier limit | Current usage | Notes |
|-----------|----------------|---------------|-------|
| Cloudflare Workers | 100k requests/day; 50 scheduled events/day | 48 cron triggers/day | Cron runs every 30 minutes (7,37 * * * * UTC) |
| GitHub Actions | 2000 minutes/month (private repos) | ~5–10 minutes/run × 2–4 runs/day | CI and refresh/news workflows |
| GitHub Pages | Unlimited bandwidth, 1 GB storage | <10 MB (site + assets) | Static site only |
| Turso Cloud | ~1 GB free database storage | <100 MB estimated | Monitor storage growth; retention/archiving pending HS-041 |

**What degrades first** (and how to detect):
- Cloudflare Workers: Workers dashboard shows request count; if approaching 100k/day, trim sources or reduce cron frequency
- GitHub Actions: GitHub Actions usage page shows minutes; if approaching 2000, defer non-critical workflows
- Turso storage: Turso Cloud console or `turso db info` shows storage bytes; if approaching 1 GB, archive old data (pending HS-041)

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
   - Commit the change to `conf/sources.yaml`
   - Run the ingest pipeline (`./dist/ingest`) to stop polling
   - Re-run export (`./dist/export`) if needed

3. **Re-enable**
   - Set `enabled: true` and re-commit
   - The next scheduled run will resume data collection

## Turso backup/restore

**Important note**: The in-repo functions `BackupToFile`, `RestoreFromFile`, `Dump`, and `ApplyRetention` are currently stubs returning "not implemented". The current production backup path uses Turso Cloud services.

### Operational backup ( Turso Cloud)

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

### In-repo backup functions (pending HS-041)

When HS-041 is implemented, the following functions will be available:

- `BackupToFile(dbPath, backupPath)` — full Turso-compatible backup
- `RestoreFromFile(backupPath, dbPath)` — restore from backup file
- `Dump(dbPath)` — SQL dump as string
- `DumpToFile(dbPath, dumpPath)` — SQL dump to file
- `ApplyRetention(dbPath)` — archive rows older than 90 days in bounded batches

## Free-tier monitoring

| Metric | Alert threshold | Location |
|--------|-----------------|----------|
| Turso storage | 900 MB | Turso Cloud console |
| Cloudflare Worker requests | 90k/day | Cloudflare dashboard |
| GitHub Actions minutes | 1800/month | GitHub Actions usage |
| Data freshness | Heartbeat > 90 minutes stale | `dist/heartbeat check` |

**Response to warnings**:
- Turso near limit: archive old data (pending HS-041) or review sources for trimming
- Cloudflare near limit: reduce cron frequency or trim sources
- GitHub Actions near limit: defer non-critical workflows

## Secret rotation

### GitHub Actions secrets
- `TURSO_DATABASE_URL`
- `TURSO_AUTH_TOKEN`
- `GITHUB_TOKEN` (for refresh-workflow dispatch)

**Rotation procedure**:
1. Generate new secret(s) in the appropriate service
2. Update in GitHub repository settings → Secrets and variables → Actions
3. Re-deploy any workflows that use the secret (Cloudflare Worker if `GITHUB_TOKEN` changed)
4. Verify deployment succeeded
5. Rotate in application code (if hardcoded anywhere)

### Cloudflare Workers secrets
- `GITHUB_TOKEN` (worker-specific, 90-day recommended)

**Rotation procedure**:
1. Generate new token at GitHub Settings → Developer settings → Personal access tokens
2. Update in worker: `wrangler secret put GITHUB_TOKEN`
3. Re-deploy: `wrangler deploy`
4. Verify dispatch works: `gh run list` shows fresh trigger attempts

## Local execution steps

### Prerequisites
- Go 1.27+ (`go version`)
- Node.js 20+ for scheduler (`node --version`)
- wrangler CLI for Cloudflare Worker deployment (if needed)

### Quick start
```bash
# Clone and setup
git clone https://github.com/HuddleScope/huddlescope.github.io.git
cd huddlescope.github.io
cp .env.example .env
# Edit .env with your local values (optional; defaults will be used if omitted)

# Build Go binaries
make build
# Binaries appear in dist/ingest and dist/export

# Run ingest (fetch feeds)
./dist/ingest

# Export snapshot to Hugo data
./dist/export

# Build Hugo site
make site
# or run local dev server
make site-watch

# Run site verification
make site-test

# Run all checks (CI pipeline)
make check
```

### Make targets
- `make help` — Show all available targets
- `make build` — Build binaries
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

### Cloudflare Worker cron
- Worker name: `huddlescope-dispatcher`
- Cron schedule: `7,37 * * * *` UTC (every 30 minutes)
- Endpoint: POST to GitHub Actions dispatch for `refresh-news.yml`
- Timeout: 10 seconds per dispatch
- Retry: exponential backoff, 3 attempts

### GitHub Actions workflow
- File: `.github/workflows/refresh-news.yml`
- Trigger: `workflow_dispatch` (manual or via Cloudflare Worker)
- Parameters:
  - `publish` (boolean, default true): Deploy to Pages after successful build
- Jobs:
  - `build`: checkout → setup Go → build → ingest → export → data guard → install Hugo → build site → upload artifacts
  - `deploy`: configure Pages → upload artifact → deploy (only if `publish=true`)
- Permissions: least privilege; deploy job requires `pages: write` and `id-token: write`

### Data guard step
Before deploying, the workflow checks story count in `web/data/homepage.json`:
- If less than 1 story, the workflow restores `homepage.json` from the last-known-good committed version
- This prevents publishing an empty site

### Pages configuration
- Environment: `github-pages`
- Branch: `main`
- Production branch: enabled
- Deploy preview: disabled (require approval for PR branches)

## Alerting

GitHub Actions provides email/Slack notifications per workflow run. Configure in repository settings:

- Workflow run success/failure notifications
- Individual job failure notifications

Heartbeat staleness is visually indicated in workflow run output:

- Fresh heartbeat (within 90 minutes of `published_at`)
- Stale heartbeat (missing or older than threshold)

## Known issues

- Export always uses memory repository (Turso via HS-040 pending) ⇒ site content never changes beyond committed fixture data
- The heartbeat file commits to the repo; the refresh workflow does not push any data back to the default branch
- Backup/restore code stubs pending HS-041 implementation
- Retention/archiving pending HS-041 implementation