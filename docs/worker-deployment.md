# First-time Cloudflare Worker deployment — HS-053

This runbook walks you through provisioning the Cloudflare Worker (`huddlescope-dispatcher`) that triggers GitHub Actions workflows on a 30-minute cadence for the HuddleScope repository.

## Prerequisites

- **Free Cloudflare account** — Sign up at [Cloudflare](https://www.cloudflare.com) (no paid plan required).
- **workers.dev free subdomain** — During Cloudflare account setup, you receive a free `*.workers.dev` subdomain (e.g., `yourname.workers.dev`). You do **not** need to buy a custom domain.
- **macOS with git and Node.js 20+** — The deployment steps assume macOS. Install git and Node.js via Homebrew:

  ```bash
  /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
  brew install git node
  ```

- **GitHub personal access token (PAT)** — You need a fine-grained PAT with Actions permissions before deployment.

## Token creation (fine-grained)

1. Go to GitHub → Settings → Developer settings → Personal access tokens → Fine-grained tokens → **Generate new token**.
2. Set:
   - **Note**: `huddlescope-cf-worker`
   - **Expiration**: 90 days (rotate quarterly as per best practices).
   - **Repository access**: Only select repositories → **huddlescope.github.io**
   - **Permissions** → **Repository permissions** → **Actions** → **Read and write**
   - **Organization approvals**: If your account is part of a GitHub organization that restricts PAT usage, request approval for this token.
3. Click **Generate token** and copy the token immediately (it is shown only once).

> **Security note**: Never commit the token to Git, store it in workflow YAML files, or add it to `.env.example`. The worker stores it as a Cloudflare secret.

> **Cloudflare secret vs GitHub Actions secrets**: The `GITHUB_TOKEN` you set here is stored ONLY in Cloudflare secret storage. GitHub Actions workflows in this repository (for example, `refresh-news.yml`) read their own repository secrets; in this repo the refresh-news workflow uses `TURSO_DATABASE_URL` and `TURSO_AUTH_TOKEN` for the Turso database—a completely separate store. Do NOT add the GitHub PAT to GitHub repository secrets or embed it in workflow YAML. The worker is the only consumer of this token. The Actions-only secrets `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID` (used by `deploy-cloudflare-worker.yml`) are likewise separate: they grant *Workers Scripts — Edit* on the Cloudflare account and must never be reused for anything else.

## Authentication and deployment

1. Clone the repository and enter the scheduler directory:

   ```bash
   git clone https://github.com/HuddleScope/huddlescope.github.io.git
   cd huddlescope.github.io/scheduler
   ```

2. Install dependencies:

   ```bash
   npm install
   ```

3. Log in to Cloudflare:

   ```bash
   npx wrangler login
   ```

   This opens a browser window. Authorize with your Cloudflare account.

4. Verify your account details:

   ```bash
   npx wrangler whoami
   ```

   You should see your Cloudflare account name and ID.

5. Deploy the worker:

   ```bash
   npx wrangler deploy
   ```

   Expected output shows the deployed worker URL:

   ```text
   https://huddlescope-dispatcher.<your-subdomain>.workers.dev
   ```

6. Store the GitHub token as a Cloudflare secret:

   ```bash
   npx wrangler secret put GITHUB_TOKEN
   ```

   Enter or paste the token when prompted. The secret is encrypted and never appears in logs or source code.

## Deployment via GitHub Actions (redeploy, issue #227)

After the first-time bootstrap below, all future deploys are automated by the
`deploy-cloudflare-worker.yml` workflow. It is **manual only**
(`workflow_dispatch`) — no push or PR ever deploys — and it runs the full
pre-deploy gate (`npm ci` → `npm test` → `wrangler whoami` → `wrangler secret
list` → `wrangler deploy --dry-run`) before any upload. A failing step fails
the run; a failed deploy never claims success.

### One-time repository secret provisioning

1. Create a Cloudflare API token: Cloudflare dashboard → My Profile → API
   Tokens → **Create token** → *Edit Cloudflare Workers* template (or a
   fine-grained token). Scope it to **one account** with permission
   **Workers Scripts — Edit** only. Copy the token.
2. Note your Cloudflare account ID (dashboard → account overview).
3. Set both repository secrets (name only — never write values into the repo
   or workflow YAML):

   ```bash
   gh secret set CLOUDFLARE_API_TOKEN -R HuddleScope/huddlescope.github.io
   gh secret set CLOUDFLARE_ACCOUNT_ID -R HuddleScope/huddlescope.github.io
   ```

### Deploying

- **Validate only (default, safe):** GitHub → Actions → *Deploy Cloudflare
  Worker* → Run workflow, leave `validate_only` checked. Runs every check and
  a dry-run bundle without uploading.
- **Deploy:** uncheck `validate_only` (set it to `false`). After the checks
  pass, the workflow uploads the worker and smoke-checks the deployed
  `/ping` endpoint (expects `hasToken: true`).

The workflow's `GITHUB_TOKEN` handling: it **verifies** the worker secret
exists (`wrangler secret list`) but never creates or rotates it. The worker's
PAT is provisioned exactly once, manually, during first-time bootstrap, and
rotated manually (see Token rotation below).

> **Two different `GITHUB_TOKEN`s — do not confuse them.** The Cloudflare
> Worker secret `GITHUB_TOKEN` is a long-lived fine-grained PAT stored only in
> Cloudflare secret storage; the GitHub Actions `GITHUB_TOKEN` in other
> workflows of this repo is ephemeral and injected per run. Rotating one
> never affects the other.

Rollback paths (reverting the workflow file, `wrangler deployments rollback`,
disabling the cron trigger, secret rotation) are covered under **Rollback and
safe disable** below and in `docs/ops-runbook.md`.

## First-time bootstrap checklist (reproducible)

Use this checklist for a fresh Cloudflare account (or when re-provisioning).
It maps the narrative steps above to a verifiable sequence.

- [ ] 1. Free Cloudflare account created; free `*.workers.dev` subdomain active (no paid plan, no custom domain).
- [ ] 2. Fine-grained GitHub PAT created (`huddlescope-cf-worker`, repo access: this repo only, Actions: Read and write, 90-day expiry).
- [ ] 3. `cd scheduler && npm install` (Node 20+).
- [ ] 4. `npx wrangler login` (browser) — or set `CLOUDFLARE_API_TOKEN`+`CLOUDFLARE_ACCOUNT_ID` env for CI-style auth.
- [ ] 5. `npx wrangler whoami` shows the expected account/ID.
- [ ] 6. `npx wrangler deploy` succeeds; note the deployed `workers.dev` URL.
- [ ] 7. `npx wrangler secret put GITHUB_TOKEN` (paste the PAT). Verify: `npx wrangler secret list` shows `GITHUB_TOKEN`.
- [ ] 8. Health check: `curl https://huddlescope-dispatcher.<subdomain>.workers.dev/ping` returns `status: ok`.
- [ ] 9. Cron verified: at the next :07/:37 UTC window, `gh run list -R HuddleScope/huddlescope.github.io --workflow refresh-news.yml` shows a new `workflow_dispatch` run (204 = queued, not completed).
- [ ] 10. (Optional, for CI deploys) Set repo secrets `CLOUDFLARE_API_TOKEN` + `CLOUDFLARE_ACCOUNT_ID` and run `deploy-cloudflare-worker.yml` with `validate_only=true` once.

## Verification

After deployment, verify that the worker is callable and the cron triggers at least one GitHub Actions run.

1. **Confirm secret is set**:

   ```bash
   cd scheduler
   npx wrangler secret list
   ```

   You should see `GITHUB_TOKEN` listed.

2. **Test the health endpoint (soft validation)**:

   ```bash
   # Replace YOUR_TOKEN with the GitHub token and <subdomain> with your workers.dev subdomain
   curl -H "Authorization: Bearer YOUR_TOKEN" \
     https://huddlescope-dispatcher.<your-subdomain>.workers.dev/ping
   ```

   Expected response:

   ```json
   {
     "status": "ok",
     "timestamp": "2026-11-17T12:00:00.000Z",
     "hasToken": true
   }
   ```

3. **Explicit dispatch-run test** (the reliable verification):

   The cron runs at minute 07 and 37 past every hour (07:07, 07:37, 08:07, 08:37, … UTC). Record the current time, wait for the next :07/:37 window, and check for a new workflow run.

   ```bash
   # Before the next cron window, list recent runs
   gh run list -R HuddleScope/huddlescope.github.io \
     --workflow refresh-news.yml \
     --json createdAt,displayTitle,event,status,conclusion \
     --limit 10
   ```

   Note the latest `createdAt` timestamp. Wait until the next :07 or :37 minute (UTC) and re-run the same command. You should see a new `workflow_dispatch` run listed.

   > **Note**: The GitHub API returns HTTP 204 Accepted for dispatch requests, which means the run was *queued*, not that it completed. The `created` event indicates the dispatch succeeded.

4. **Optional: Tail worker logs in real time**:

   ```bash
   npx wrangler tail
   ```

   You’ll see structured logs including publication dispatch events with timestamps matching the :07/:37 UTC schedule.

## Cron triggers and propagation delay

- **Schedule**: `7,37 * * * *` UTC (twice per hour).
- **Cloudflare dashboard**: Workers & Pages → Your worker → Triggers shows the configured cron.
- **Propagation delay**: After setting or editing a cron trigger, there may be a few minutes for the change to propagate across Cloudflare’s edge network. The next scheduled execution occurs at the next matching minute.

## Observability

### Worker Logs (dashboard)

- Workers & Pages → Your worker → Logs
- Search for `publication_dispatch` to see cron trigger events.

### Manual dispatch (for testing)

If you want to trigger a run outside the cron schedule:

```bash
# Without deploy to Pages
gh workflow run refresh-news.yml -R HuddleScope/huddlescope.github.io --field publish=false

# With deploy to Pages
gh workflow run refresh-news.yml -R HuddleScope/huddlescope.github.io --field publish=true
```

Check run status:

```bash
gh run list -R HuddleScope/huddlescope.github.io --limit 10
gh run view <RUN_ID> -R HuddleScope/huddlescope.github.io
```

## Incident diagnostics

| Symptom | Likely cause | Action |
|---------|--------------|--------|
| `401 Unauthorized` | Token expired or revoked | Regenerate token and run `npx wrangler secret put GITHUB_TOKEN` again |
| `403 Forbidden` | Token lacks Actions:write scope | Create new fine-grained token with Actions: Read and write |
| `404 Not Found` | Workflow name or repo incorrect | Verify `WORKFLOW_NAME` (`refresh-news.yml`) and `REPO_OWNER/REPO_NAME` in `src/index.js`; check `refresh-news.yml` exists in `.github/workflows/` |
| `422 Unprocessable` | Workflow disabled or invalid ref | Ensure `refresh-news.yml` exists and is not disabled; verify the `main` branch exists |
| `429 Too Many Requests` | GitHub rate limit exceeded | Worker honors the `Retry-After` header; wait for the suggested delay |
| `5xx errors` | GitHub API transient issues | Worker retries up to 3 times with exponential backoff; check worker logs for details |
| Cron missing trigger | Worker not deployed or config invalid | Run `npx wrangler deploy`; verify `triggers.crons` in `wrangler.toml` |
| `deploy-cloudflare-worker` fails at "Fail fast" | Repo secrets `CLOUDFLARE_API_TOKEN`/`CLOUDFLARE_ACCOUNT_ID` not set | Set them (see *Deployment via GitHub Actions*); re-run with `validate_only=true` |
| `deploy-cloudflare-worker` fails at secret list | Worker secret `GITHUB_TOKEN` missing | `cd scheduler && npx wrangler secret put GITHUB_TOKEN` (manual, one-off) |
| `deploy-cloudflare-worker` deploy step 403 | Cloudflare token scope too narrow | Re-issue the token with account scope + **Workers Scripts — Edit**; update `CLOUDFLARE_API_TOKEN` |

## Token rotation and expiry monitoring

- **Rotation cycle**: Every 90 days (matching token expiry).
- **Pre-expiry monitoring**: Set a calendar reminder 7 days before expiry.
- **Rotation procedure**:
  1. Generate a new fine-grained token at GitHub Settings → Developer settings.
  2. Update the Cloudflare secret:

     ```bash
     cd scheduler
     npx wrangler secret put GITHUB_TOKEN
     ```

  3. Re-deploy the worker:

     ```bash
     npx wrangler deploy
     ```

  4. Verify the next cron trigger fires successfully (see **Explicit dispatch-run test** above).

## Rollback and safe disable

### Rollback to previous version

If the latest deployment is broken:

```bash
cd scheduler
npx wrangler deployments list
npx wrangler deployments rollback
```

### Safe disable (no repo change needed)

To temporarily stop cron dispatches without touching the code or config:

1. Go to Cloudflare dashboard → Workers & Pages → Your worker.
2. Navigate to **Triggers**.
3. Remove or disable the cron trigger entry.

The worker remains deployed and callable via `/ping`, but the cron will not fire. Re-enable the trigger later to resume scheduled dispatches.

## Known issues

- **Worker logs**: The worker explicitly omits the `GITHUB_TOKEN` from all log output to prevent accidental leakage.
- **Cron queue lag**: The actual dispatch time may vary by a few seconds due to Cloudflare’s scheduling queue. The `createdAt` timestamp of the GitHub run is the authoritative reference.
- **204 Accepted ≠ Completed**: HTTP 204 from the dispatch endpoint means the run was queued, not that it succeeded. Check the workflow run status for completion.