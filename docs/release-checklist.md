# HuddleScope v0.1 release checklist

## Evidence rule

A box is checked **only** with (a) a link to immutable evidence (commit SHA, PR,
workflow-run URL) or (b) a repeatable command a maintainer can run. Items that
cannot be verified from this environment stay `[ ]` under
**Pending production verification**, each carrying the exact repeatable command
that would verify it. A checked item that later proves wrong is demoted back to
unchecked with the evidence of the failure — it is never silently re-checked.

Items whose verification was performed live against production are marked
*verified 2026-11-24* with run links; re-verify with the same commands before
each release.

## Pre-launch checks

- [x] SEO assets — *evidence: `scripts/site-test.sh` (site.yml workflow, hard
  gate on every site build)*
  - [x] `robots.txt` emitted at `/robots.txt` (site-test build assertions)
  - [x] `sitemap.xml` emitted at `/sitemap.xml` (site-test build assertions)
  - [x] Per-page `<meta name="description">` rendered (site-test assertions;
    live page carries `meta name="description"`)
  - [x] Open Graph tags (`og:title`, `og:description`, `og:url`, `og:type`, `og:site_name`)
    (live `https://huddlescope.github.io/` serves all five)
  - [x] Twitter Card tags (`twitter:card`, `twitter:title`, `twitter:description`)
    (live page carries them)
  - [x] Canonical links on all pages (live page carries `rel="canonical"`)
  - Repeatable: `bash scripts/site-test.sh` (overlays `web/fixtures/`, see below)
- [x] Accessibility — *evidence: `scripts/site-test.sh` assertions*
  - [x] `<html lang="...">` present
  - [x] Skip link present and targets `<main id="main-content" tabindex="-1">`
  - [x] Semantic HTML (`<main>`, `<header>`, `<nav>`, `<article>`, `<footer>`)
  - [x] Keyboard navigation (`aria-label`, `aria-labelledby`, `aria-current="page"`)
  - [x] 404.html emitted (was previously disabled)
  - Repeatable: `bash scripts/site-test.sh`
- [x] Links and content — *evidence: `scripts/site-test.sh` assertions +
  `conf/sources.yaml` registry fields*
  - [x] No broken internal links (scan performed by site-test build)
  - [x] No orphan story pages (all homepage story IDs have matching content)
  - [x] All feeds have attribution (source name, URL, license, acceptable use —
    `license`/`acceptable_use` fields in `conf/sources.yaml`, enforced by
    `docs/editorial-policy.md`)
- [x] Data safety — *evidence: export contract + domain validation tests*
  - [x] Secret patterns rejected in exported summaries — `internal/domain`
    snapshot `Validate()` (repeatable: `go test ./internal/domain/`); contract
    in `web/README-data.md` ("No-secret and no-body guarantees")
  - [x] No copyright infringement (no full articles, only metadata+summaries —
    `internal/domain/storypage.go` omits body fields; `docs/editorial-policy.md`)
  - [x] No private data in published artifacts (whitelisted export fields only;
    `web/README-data.md` contract + `go test ./internal/adapters/hugo/`)
- [x] Cache validation
  - [x] Build is idempotent (same output on repeated builds — `scripts/site-test.sh`
    idempotency path: "outputs identical across builds")
  - [x] `scripts/site-test.sh` passes both locally and in CI (Site workflow:
    `.github/workflows/site.yml` hard gate)
  - [x] Refresh workflow can be dispatched (deploy to Pages) — *verified
    2026-11-24*: Cloudflare cron fires `refresh-news.yml` every 30 minutes
    (`7,37 * * * *` UTC, `scheduler/wrangler.toml`) and deploys to Pages;
    20/20 recent runs `success`. Evidence:
    https://github.com/HuddleScope/huddlescope.github.io/actions/runs/45410553677 ,
    https://github.com/HuddleScope/huddlescope.github.io/actions/runs/45405306823 ,
    https://github.com/HuddleScope/huddlescope.github.io/actions/runs/45400148224 .
    Repeatable:
    `gh run list -R HuddleScope/huddlescope.github.io --workflow refresh-news.yml --json url,conclusion --limit 20`
- [ ] Deployment — *mixed verified state; see Current production state below*
  - [x] GitHub Pages site is deployed — *verified 2026-11-24*: deploy job runs
    on each refresh run (artifact-based `actions/deploy-pages@v5`); live URL
    returns HTTP 200. Repeatable: `curl -s -o /dev/null -w '%{http_code}' https://huddlescope.github.io/`
  - [x] Live URL serves Hugo site (not Jekyll README) — *verified 2026-11-24*:
    live page carries `meta name="generator" content="Hugo 0.169.0"`, Open
    Graph/Twitter metadata, canonical link. Repeatable:
    `curl -s https://huddlescope.github.io/ | grep -o 'name="generator" content="Hugo [0-9.]*"'`
  - [ ] `heartbeat.json` committed on dispatch — *verified FALSE 2026-11-24*:
    the refresh workflow's commit steps create local commits in the ephemeral
    runner but the workflow has **no `git push` step**, so commits never reach
    `main`. The committed `web/data/heartbeat.json` is stale
    (`published_at: 2026-11-23T16:59:07Z`) and the `heartbeat-check` workflow
    (90-minute staleness gate) fails on every run. Evidence: run log
    https://github.com/HuddleScope/huddlescope.github.io/actions/runs/45410553677
    ("[main …] Update heartbeat.json" — commit exists only in the runner),
    failing checks
    https://github.com/HuddleScope/huddlescope.github.io/actions/runs/45339042557 .
    Repeatable: `cat web/data/heartbeat.json` (check `published_at`) and
    `gh run list -R HuddleScope/huddlescope.github.io --workflow heartbeat-check.yml --json url,conclusion --limit 5`
  - [ ] `homepage.json` reflecting fresh feed data — *verified FALSE 2026-11-24*:
    the export writes **0 stories** to `web/data/homepage.json` while Turso
    holds 3017 ranked (stale, >24 h old) stories; the publish decision deploys
    the labelled "No stories published yet" empty state (the correct HS-056
    behaviour for a snapshot with no fresh stories). Live homepage shows the
    labelled empty state. Evidence: run log (above) — "Stories in exported
    snapshot: 0 (committed: 0)". Repeatable:
    `curl -s https://huddlescope.github.io/ | grep -o 'No stories published yet'`

## Current production state (verified 2026-11-24)

Verified live via `gh run list` / run logs / `curl` against
`https://huddlescope.github.io/`:

| Aspect | State | Evidence / repeatable command |
|--------|-------|-------------------------------|
| Cloudflare cron → `refresh-news.yml` | Green: dispatches every 30 min, all runs `success` | `gh run list -R HuddleScope/huddlescope.github.io --workflow refresh-news.yml --json url,conclusion --limit 20` |
| Ingest (Turso backend) | Working: ranks 3017 stories (all older than the 24 h lookback) | run log step "Run ingest pipeline" |
| Export → `homepage.json` | **Degraded: 0 stories exported** (no articles within the lookback window) | run log step "Publish decision": "Stories in exported snapshot: 0" |
| Pages deploy | Working: labelled empty state deployed (HS-056 never-real path) | `curl -s https://huddlescope.github.io/ \| grep -o 'No stories published yet'` |
| Workflow commit steps → `main` | **Broken: no `git push` step in `refresh-news.yml`; commits never land on `main`** | `git log origin/main -- web/data/` vs. run log "Commit heartbeat" |
| `heartbeat-check` workflow | **Red: committed heartbeat stale (>90 min)** — consequence of the missing push step | `gh run list -R HuddleScope/huddlescope.github.io --workflow heartbeat-check.yml --json url,conclusion --limit 5` |

Two follow-up issues must be filed before the v0.1 milestone closes (outside
this documentation issue's scope): (1) add a push step (or artifact-based data
update) to `refresh-news.yml` so committed `web/data/*` reflects real runs;
(2) fix the export so fresh articles within the lookback window are published.

## Pending production verification

Items not yet verifiable from this environment. Each carries the command that
would verify it; check the box **only** after the observation is recorded.

- [ ] Release-gate live evidence recorded (HS-058 / issue #110): two consecutive
  scheduled runs publishing a fresh nonempty snapshot; live homepage and
  `/status/feeds/` consistent with Turso; failure drills; evidence attached to
  #110. Command: `bash scripts/release-gate.sh --live` (see `docs/release-gate.md`)
- [ ] Repeated refresh produces no duplicate stories on the live site.
  Command: run `refresh-news.yml` twice (`gh workflow run … --field publish=true`)
  and compare story counts on `https://huddlescope.github.io/`
- [ ] Export again publishes ≥1 story (after the follow-up above lands).
  Command: `jq '.stories | length' web/data/homepage.json` after a real refresh

## Known limitations (recorded for v0.1)

- **Turso persistence (HS-040)**: Implemented. The Turso backend exists in
  `internal/adapters/storage/turso` (`libsql-client-go`); `StorageConfig`
  (PR #179) selects the backend; `refresh-news.yml` runs production with
  `HSC_STORAGE_BACKEND=turso` and validates `TURSO_DATABASE_URL` +
  `TURSO_AUTH_TOKEN` at startup (redacted error if missing).
- **Cloudflare Worker delivery (HS-019)**: Deployed per HS-053
  (`docs/worker-deployment.md`); cron `7,37 * * * *` UTC (48 dispatches/day,
  `scheduler/wrangler.toml`) verified firing 2026-11-24. Remaining open item:
  production release-gate evidence (HS-058 / issue #110) — see Pending
  production verification.
- **Production data freshness**: As of 2026-11-24 the live site shows the
  labelled empty state because the export produces 0 fresh stories (see
  Current production state). The empty-state rendering is the designed HS-056
  behaviour; the empty export itself is a tracked follow-up.
- **Feed health dashboard (HS-38)**: The live dashboard of RSS feed status is
  not yet implemented (code referenced in the issue exists in a feature branch
  but is not committed). Pending.
- **Stale docs (HS-050)**: `docs/ci-status.md` removed; reference consolidated
  onto `docs/ops-runbook.md`. Resolved.

## Operator-facing v0.1 release checklist

The operator who closes v0.1 must confirm, in order:

- [ ] Live site reachable and serving the Hugo build:
  `curl -s https://huddlescope.github.io/` (live URL: https://huddlescope.github.io/)
- [ ] GitHub release `v0.1` created (none exists as of 2026-11-24 —
  `gh release list -R HuddleScope/huddlescope.github.io`)
- [ ] Release notes published at `docs/release-notes-v0.1.md` (in this repo)
- [ ] Links recorded to the latest **actual successful** `refresh-news` runs:
  `gh run list -R HuddleScope/huddlescope.github.io --workflow refresh-news.yml --json url,conclusion --limit 5`
- [ ] Release-gate evidence attached to issue #110 (HS-058):
  `bash scripts/release-gate.sh --live` output + run links
  (`docs/release-gate.md` describes the sections)
- [ ] **Milestone gate**: close the v0.1 milestone **only after** the HS-058
  release gate (issue #110) passes with live evidence recorded. Issue #110 is
  closed (PR #194, 2026-11-24 — gate tests and workflow), but the live-evidence
  follow-up is still pending and the Current production state above lists the
  two defects that must be cleared first. Do not close the milestone on the
  issue's closed state alone.
- [ ] Website status badge (if desired) added to README

## Rollback plan

If critical issues are discovered after release:

1. Revert the PR or commit on main and push.
2. Force re-deploy GitHub Pages (dispatch refresh with `publish=true`).
3. Restore `web/data/homepage.json` from last known good commit if needed.
4. Update GitHub release with rollback note.

## Notes

- The `scripts/site-test.sh` section added by this PR (HS-023 launch verification)
  replaces the hardcoded unverified/evidence checks with data-driven assertions
  based on `homepage.json` contents.
- Since HS-056, `web/data/` tracks only `heartbeat.json`. The 2024 sample
  content lives in `web/fixtures/` (test-only), and the homepage renders a
  clearly-labelled empty state until the first real refresh export commits
  `web/data/homepage.json` (see `web/README-data.md`). For local development,
  `make site-seed` copies the fixtures into the production paths (untracked).
- The refresh workflow's `publish-decision` step (HS-056) gates the homepage
  commit and the Pages deploy: ≥1 story → deploy; 0 stories with a prior real
  snapshot → skip commit and deploy (last-good retained); 0 stories never-real
  → deploy the labelled empty state. A hard demo-marker gate blocks any Pages
  upload containing sample-data signatures. See `web/README-data.md`.
- Operational procedures (provisioning, manual refresh, failure diagnosis,
  cost review): `docs/ops-runbook.md`. Release-gate runbook: `docs/release-gate.md`.
