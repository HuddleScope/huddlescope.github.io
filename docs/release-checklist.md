# HuddleScope v0.1 release checklist

## Pre-launch checks

- [x] SEO assets
  - [x] `robots.txt` emitted at `/robots.txt`
  - [x] `sitemap.xml` emitted at `/sitemap.xml`
  - [x] Per-page `<meta name="description">` rendered
  - [x] Open Graph tags (`og:title`, `og:description`, `og:url`, `og:type`, `og:site_name`)
  - [x] Twitter Card tags (`twitter:card`, `twitter:title`, `twitter:description`)
  - [x] Canonical links on all pages

- [x] Accessibility
  - [x] `<html lang="...">` present
  - [x] Skip link present and targets `<main id="main-content" tabindex="-1">`
  - [x] Semantic HTML (`<main>`, `<header>`, `<nav>`, `<article>`, `<footer>`)
  - [x] Keyboard navigation (`aria-label`, `aria-labelledby`, `aria-current="page"`)
  - [x] 404.html emitted (was previously disabled)

- [x] Links and content
  - [x] No broken internal links (scan performed)
  - [x] No orphan story pages (all homepage story IDs have matching content)
  - [x] All feeds have attribution (source name, URL, license, acceptable use)

- [x] Data safety
  - [x] `secret scan` passes (no ghp_/gho_/xoxb_ tokens in artifacts)
  - [x] No copyright infringement (no full articles, only metadata+summaries)
  - [x] No private data in published artifacts

- [x] Cache validation
  - [x] Build is idempotent (same output on repeated builds)
  - [x] `scripts/site-test.sh` passes both locally and in CI
  - [x] Refresh workflow can be dispatched (deploy to Pages)

- [x] Deployment
  - [x] GitHub Pages site is deployed
  - [x] Live URL serves Hugo site (not Jekyll README)
  - [x] heartbeat.json committed on dispatch
  - [x] homepage.json reflecting fresh feed data

## Post-launch tasks

- [ ] GitHub release v0.1 created
- [ ] Release notes published at `docs/release-notes-v0.1.md`
- [ ] Issue #23 closed after merge
- [ ] Website status badge (if desired) added to README

## Known limitations (recorded for v0.1)

- **Turso persistence (HS-040)**: The Go ingestion pipeline runs with an in-memory
  repository and discards all fetched entries on exit. Full Turso persistence
  requires PR #59 (depends on #68) — mark as incomplete until Turso adapter is
  implemented.

- **Cloudflare Worker delivery (HS-019)**: The Cloudflare Worker dispatches the
  refresh workflow, but deployment status cannot be verified from this environment
  (requires wrangler + Cloudflare credentials). Cron schedule is configured and
  dispatch works when run manually.

- **Feed health dashboard (HS-38)**: The live dashboard of RSS feed status is
  not yet implemented (code referenced in issue exists in a feature branch but
  is not committed). Pending.

- **Stale docs (HS-050)**: `docs/ci-status.md` references removed make targets
  and may be misleading. Pending.

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
- The seed homepage data has been updated to match the three real story content
  pages (`top-free-agents-2024`, `pro-bowl-2025-format-changes`,
  `preseason-week-1-overview`).