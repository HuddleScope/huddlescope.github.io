# Implementation — Issue #227: Automate Cloudflare Cron Worker deployment and first-time bootstrap

Phase: implement. Follows the saved plan (`autopilot-state/artifacts/227/plan.md`) exactly.
No Go/worker code, no `wrangler.toml` values, no `refresh-news.yml` changes. No `git commit`/`push`, no GitHub mutation (Go owns those).

## Files changed

| # | File | Action | Purpose (AC) |
|---|------|--------|--------------|
| 1 | `.github/workflows/deploy-cloudflare-worker.yml` | **Create** | Primary deliverable — manual `workflow_dispatch` deploy workflow with `validate_only` (default true), fail-fast secret check, `npm ci` + `npm test` + `wrangler whoami` + `secret list` + `deploy --dry-run` gate, atomic `wrangler deploy` + post-deploy `/ping` smoke, least-privilege `contents: read`. (AC-2, AC-5, AC-7) |
| 2 | `scheduler/src/index.test.js` | **Modify** | Append one `wrangler.toml` guard `describe` block (2 tests): pin `name`/`crons == ["7,37 * * * *"]` and `workers_dev == true` via a dependency-free minimal TOML text parse. Fails CI on any cron-schedule or hosting-mode drift. (AC-3, AC-6 regression guard) |
| 3 | `docs/worker-deployment.md` | **Extend** | Add "Deployment via GitHub Actions" section (one-time `CLOUDFLARE_*` secret provisioning, validate-only vs deploy, two-`GITHUB_TOKEN` warning, secret-scope note), a first-time bootstrap checklist (10 verifiable steps), and 3 new rows in the incident table. (AC-1, AC-3, AC-4) |
| 4 | `docs/ops-runbook.md` | **Modify** | Initial-provisioning §2: deploy-automation + rollback pointers; §4 secret-store table gains `CLOUDFLARE_API_TOKEN`/`CLOUDFLARE_ACCOUNT_ID` row; Secret rotation gains a "Cloudflare deploy credentials" procedure. (AC-1, AC-4) |
| 5 | `scheduler/README.md` | **Modify** | One-paragraph pointer to the CI deploy workflow under "Deployment steps". (AC-2) |

## Acceptance criteria → how met
- **AC-1** Bootstrap: `docs/worker-deployment.md` new "First-time bootstrap checklist" — reproducible 10-step sequence (account → PAT → `wrangler login` → `deploy` → `secret put GITHUB_TOKEN` → `/ping` → cron `:07/:37` `gh run list` correlation) plus the CI-deploy section.
- **AC-2** Redeploy from Actions: new workflow authenticates from `CLOUDFLARE_API_TOKEN` + `CLOUDFLARE_ACCOUNT_ID` (Wrangler 4 env-var auth, no `wrangler login` in CI) and uploads via `wrangler deploy` when `validate_only=false`.
- **AC-3** Cron verified: guard test pins `crons == ["7,37 * * * *"]`; workflow post-deploy smoke curls `/ping` and asserts `hasToken: true`; docs document `204 = queued, not completed` (already present, restated).
- **AC-4** Secret docs: worker-deployment.md + ops-runbook.md document token scope (account-scoped, **Workers Scripts — Edit** only), provisioning, 90-day rotation, the two-`GITHUB_TOKEN` distinction, and an extended incident table.
- **AC-5** Checks gate deploy: workflow step ordering (fail-fast secret check → `npm ci` → `npm test` → `whoami` → `secret list` → `deploy --dry-run` → `deploy`) — any non-zero step fails the run; a failed deploy never claims success.
- **AC-6** Free tier: guard test asserts `workers_dev = true`; docs restate 48 ≤ 50 events/day.
- **AC-7** Explicit-trigger only: grep-verified the **only** trigger is `workflow_dispatch` (no `push:`/`pull_request:`/`schedule:`); `permissions: contents: read` only (no `actions:`/`id-token:`/`packages:`); no secret value appears in workflow code.

## Tests added (regression)
- `scheduler/src/index.test.js` → `describe('AC-3/AC-6 guard: wrangler.toml pins cron schedule and free tier')`:
  - `must schedule cron triggers at "7,37 * * * *"` (also pins `name`).
  - `must keep workers_dev = true (free tier, no paid domain)`.
  - Both use a self-contained minimal TOML text parser (inline + multi-line `triggers` table forms); no new dependencies.

Full suite: `npm --prefix scheduler test` → **32 pass / 0 fail** (was 30; +2 guard tests). Existing mocked unit tests (204/401/403/404/422/429/5xx, SecretError, retry/backoff, masking, static trigger-only guard) unchanged and still green.

## Local verification (all green)
```
make test        → all Go packages ok (vet + race)
make build       → dist/huddlescope built
make e2e         → e2e matrix PASS (offline)
gofmt check      → GOFMT_OK (no unformatted .go)
npm ci           → scheduler deps installed
npm test         → 32 pass / 0 fail
```
Workflow YAML validated (`yaml.safe_load` → OK); grep-confirmed single trigger + least-privilege permissions.

## Notes / deviations
None. Implementation matches the plan. The guard-test TOML parser was implemented to support both the current single-line inline `triggers = { crons = [...] }` form and the multi-line form, so it stays correct if the formatting ever changes.
