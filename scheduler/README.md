# Cloudflare Worker Cron Dispatcher — HS-019

This Cloudflare Worker triggers GitHub Actions workflows on a 30-minute cadence for the HuddleScope repository.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                              Cloudflare Workers Free                                 │
│                                                                                      │
│  ┌───────────────────────────────────────────────────────────────────────────────┐ │
│  │                            huddlescope-dispatcher                              │ │
│  │                                                                                │ │
│  │  scheduled(entry): cron("7,37 * * * *") UTC → POST /actions/workflows/...    │ │
│  │     │                                                                          │ │
│  │     └─► 10s timeout + exponential backoff (3 attempts)                       │ │
│  │               │                                                                │ │
│  │               └─► GitHub REST API dispatch endpoint                          │ │
│  │                      (204 Accepted = queued, not completed)                  │ │
│  └───────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                      │
└─────────────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                              GitHub Actions (Refresh News)                         │
│                                                                                      │
│  ┌───────────────────────────────────────────────────────────────────────────────┐ │
│  │                        refresh-news.yml workflow                               │ │
│  │                                                                                │ │
│  │  workflow_dispatch trigger ──► ingress ──► concurrency (cancel-in-progress)  │ │
│  │     │                                                                          │ │
│  │     └─► build job: ingest → export → Hugo build                              │ │
│  │     └─► deploy job (optional): Pages artifact upload                        │ │
│  └───────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                      │
└─────────────────────────────────────────────────────────────────────────────────────┘
```

### Why Cloudflare Workers?
- **Cost**: Free tier covers 100k requests/day and 50 scheduled events/day. Our cron runs 48 times/day.
- **Reliability**: Native cron triggers with exponential backoff and bounded retries.
- **Isolation**: No Go runtime dependencies; keeps the Go engine hosting-independent.

### Why a separate Worker instead of GitHub Actions cron?
GitHub Actions itself does not support native cron scheduling — it must be triggered externally (e.g., GitHub's own Actions Schedule, third-party cron, or Cloudflare Workers). This approach avoids the overhead of managing multiple GitHub Actions workflows and ensures a single, authoritative scheduler.

## Installation & Deployment

### Prerequisites
- Node.js 20+ (`node --version`)
- Wrangler CLI (`npm install -g wrangler`)
- GitHub personal access token with `repo` scope

### Token creation (fine-grained)

1. Go to GitHub → Settings → Developer settings → Personal access tokens → Tokens (classic)
2. Click "Generate new token" → "Generate new token (classic)"
3. Set:
   - Note: `huddlescope-cf-worker`
   - Expiration: 90 days (rotate quarterly)
   - Select "Only select repositories" → `huddlescope.github.io`
   - Select "Select permissions"
     - **Repository permissions** → **Actions**: Write (only)
4. Click "Generate token" and **copy the token immediately** (it won't be shown again)

### Deployment steps

```bash
cd scheduler

# Install dependencies
npm install

# Set the GitHub token as a secret (never commit this)
wrangler secret put GITHUB_TOKEN

# Deploy to Cloudflare Workers
wrangler deploy
```

The deployment outputs the Workers URL (e.g., `https://huddlescope-dispatcher.<your-subdomain>.workers.dev`).

### Verify deployment

```bash
# Health check (soft validation of token presence)
curl -H "Authorization: Bearer YOUR_TOKEN" \
  https://huddlescope-dispatcher.<your-subdomain>.workers.dev/ping
```

Expected response:
```json
{
  "status": "ok",
  "timestamp": "2026-11-12T10:00:00.000Z",
  "hasToken": true
}
```

## Quotas and Limits (Cloudflare Workers Free)

| Resource | Free Tier Limit | Our Usage | Status |
|----------|-----------------|-----------|--------|
| Daily requests | 100,000 | ~48 dispatches + health checks | ✓ OK |
| CPU per request | 10 ms | ~1–2 ms per dispatch | ✓ OK |
| Scheduled events/day | 50 | 48 (7,37 * * * *) | ✓ OK |
| Scheduled payload | 128 KiB | ~200 bytes (dispatch JSON) | ✓ OK |

Reference: [Cloudflare Workerslimits](https://developers.cloudflare.com/workers/platform/limits/)

## Scheduled execution semantics

- **Cron**: `7,37 * * * *` UTC — runs at :07 and :37 past every hour (48 runs/day)
- **Latency**: Actual trigger time may vary by seconds due to Cloudflare's scheduling queue
- **Idempotency**: The GitHub Actions workflow uses `concurrency: cancel-in-progress` to prevent overlapping runs

## 204 Accepted ≠ Completed

The GitHub REST API returns **HTTP 204 No Content** when a workflow dispatch request is accepted and queued. This means:

- ✅ 204: Dispatch queued successfully
- ❌ 204 does NOT mean the workflow completed
- ❌ 204 does NOT indicate success or failure of the underlying run

To monitor actual workflow status:

1. Inspect the `workflow_dispatch` event in GitHub Actions UI
2. Check the workflow run status in `https://github.com/HuddleScope/huddlescope.github.io/actions`
3. Use the GitHub REST API to poll the run status:
   ```bash
   gh api repos/HuddleScope/huddlescope.github.io/actions/runs \
     --jq '.workflow_runs[] | select(.name=="Refresh news")'
   ```

See `docs/ci-status.md` for real-time status and known issues.

## Operations runbook

### View trigger logs

```bash
# Latest worker logs
wrangler logs

# Filter for scheduled events
wrangler logs --level=info --event=scheduled_dispatch
```

### Troubleshooting

| Symptom | Likely cause | Action |
|---------|--------------|--------|
| 401 Unauthorized | Token expired or revoked | Regenerate token with `wrangler secret put GITHUB_TOKEN` |
| 403 Forbidden | Token lacks Actions:write scope | Create new token with Actions:write permission |
| 404 Not Found | Workflow name or repo incorrect | Verify `WORKFLOW_NAME` and `REPO_*` constants in `src/index.js` |
| 5xx errors | GitHub API transient issues | Worker will retry up to 3 times; check worker logs for details |
| 429 Too Many Requests | GitHub rate limit exceeded | Worker honors `Retry-After` header; check logs for delay duration |
| Cron missing trigger | Worker not deployed or config invalid | Deploy again: `wrangler deploy`; verify `triggers.crons` in `wrangler.toml` |
| 204 returned but no workflow run | GitHub queue delay | Wait 1–2 minutes; check GitHub Actions UI for dispatched run |

### Redeploy

```bash
cd scheduler
wrangler deploy
```

The new version is deployed atomically. Current in-flight dispatches continue on the previous version.

### Rollback

If the current version is broken:

```bash
# List deployments
wrangler deployments list

# Rollback to previous version
wrangler deployments rollback
```

## Development

```bash
# Local dev server (HTTP handler only; scheduled events not available)
npm run dev

# Run tests
npm test

# Format
npx prettier --write src/*.js
```

## Security

- The `GITHUB_TOKEN` is stored as a Cloudflare secret, never in source or logs
- All logs are structured JSON; the code explicitly omits the token from log output
- The worker runs on Cloudflare's edge — no IP exposure
- Token rotation: quarterly, as per best practices

## License

MIT — see `LICENSE` in the root repository.