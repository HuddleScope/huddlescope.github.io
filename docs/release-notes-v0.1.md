# HuddleScope v0.1 release notes

**Status**: v0.1 features are shipped. Milestone closure is **gated on the
HS-058 release gate (issue #110) passing with live evidence recorded**. Issue
#110 is closed (PR #194, 2026-11-24 — release-gate tests and workflow), but the
live-evidence follow-up (`bash scripts/release-gate.sh --live` →
`docs/release-gate.md` + issue comment) is pending, and two production defects
are tracked in `docs/release-checklist.md` (Current production state).

**Release date**: 2026-11-13  
**GitHub Pages**: https://huddlescope.github.io/

## What ships

HuddleScope v0.1 is the first publicly available version of an independent NFL
news aggregator built with a modular Go engine, Turso database, and static Hugo
site generation.

### Site features

- **Curated NFL news**: Stories aggregated from national and team-level RSS feeds
  (ESPN, NFL.com, CBS Sports, FOX Sports, team official feeds).
- **Story scoring**: Algorithmic scoring based on publisher count, entity
  extraction, and recency (scored 0–100).
- **Evidence levels**: Confirmed vs unverified (rumor/speculation) badges for
  each story.
- **Multi-source pages**: Stories appearing in multiple feeds show all original
  articles with timestamps and direct links.
- **Team pages**: 32 NFL team pages with publication history and source links.

### SEO & accessibility

- Full sitemap (`/sitemap.xml`) and `robots.txt`
- Open Graph and Twitter Card metadata for social sharing
- Skip link, keyboard navigation, semantic HTML, dark mode support
- `404.html` served for unknown paths

### Data & verification

- Deterministic JSON export to Hugo `web/data/homepage.json`
- Local refresh workflow triggers ingestion, export, Hugo build, Pages deploy
- `scripts/site-test.sh` runs before every build (CI and dispatch)
- Publish decision (HS-056): the workflow commits and deploys a snapshot only
  when it has ≥1 story; zero stories with a prior real snapshot retains the
  last-good deployment; zero stories never-real deploys a clearly-labelled
  empty state. A demo-marker gate blocks any Pages upload containing sample
  data (see `web/README-data.md`).

## Architecture

```
Cloudflare Worker (cron) ─┐
                          ├─► GitHub Action (refresh-news.yml)
GitHub Pages deploy ◄─────┤
                          ├─► Go ingest (RSS fetch, cluster, score)
Hugo site ◄───────────────┤
                          └─► Export → web/data/homepage.json
```

The single Go binary `dist/huddlescope` exposes `ingest run`, `export`, and
`heartbeat check` subcommands; the Cloudflare Worker
(`scheduler/`, cron `7,37 * * * *` UTC) dispatches the refresh workflow.

## Evidence (verified 2026-11-24)

- **Live site**: https://huddlescope.github.io/ — serves the Hugo build
  (verified: HTTP 200, `meta name="generator" content="Hugo 0.169.0"`).
  As of this verification the homepage shows the labelled
  "No stories published yet" empty state (export currently produces 0 fresh
  stories — see Known limitations).
- **Successful `refresh-news` runs** (cron-triggered, every 30 minutes):
  - https://github.com/HuddleScope/huddlescope.github.io/actions/runs/45410553677 (2026-11-24T14:07Z)
  - https://github.com/HuddleScope/huddlescope.github.io/actions/runs/45405306823 (2026-11-24T13:37Z)
  - https://github.com/HuddleScope/huddlescope.github.io/actions/runs/45400148224 (2026-11-24T13:07Z)
  - Repeatable: `gh run list -R HuddleScope/huddlescope.github.io --workflow refresh-news.yml --json url,conclusion --limit 20`
- **Release gate**: `docs/release-gate.md` (HS-058 runbook; `bash scripts/release-gate.sh --live`
  for production evidence, `--local-only` for offline drills). Live evidence is
  **pending HS-058 follow-up** — record it before closing the v0.1 milestone.
- **Site verification**: `scripts/site-test.sh` (hard gate in `site.yml` and in
  every `refresh-news` build).

## Cost review (condensed)

Zero paid services are required by the v0.1 configuration (full review with
usage-check commands: `docs/ops-runbook.md` → Free-tier topology and cost
review):

- **GitHub Actions**: standard runners in public repositories are free (this
  repo is public; GitHub billing docs). Artifact storage (500 MB free) is the
  first thing to exhaust — runs upload `web/public/` with 7-day retention.
- **Cloudflare Workers**: cron fires 48 scheduled events/day. The free-plan
  scheduled-event limit (50/day per the `scheduler/wrangler.toml` comment)
  could not be re-verified from the repository; treat the 48/50 proximity as a
  known exposure and check the official Cloudflare limits page.
- **GitHub Pages**: 1 GB storage; the site is well under it (<10 MB estimate).
- **Turso Cloud**: free database storage; `turso db info huddlescope` to check.

## Known limitations

- **Production data freshness (open)**: As of 2026-11-24 the export writes 0
  stories to `homepage.json` while Turso holds 3017 ranked stories older than
  the 24 h lookback; the live site shows the labelled empty state (correct
  HS-056 behaviour). Follow-up issue tracked in `docs/release-checklist.md`.
- **Missing push step (open)**: `refresh-news.yml` commits `web/data/*`
  locally but never pushes, so committed data files are stale and the
  `heartbeat-check` workflow fails on staleness. Follow-up issue tracked in
  `docs/release-checklist.md`.
- **Storage**: Turso persistence is implemented (`internal/adapters/storage/turso`,
  `libsql-client-go`; `StorageConfig` from PR #179). Production runs with
  `HSC_STORAGE_BACKEND=turso`; local runs use `sqlite-file` or `memory`.
- **Delivery**: Cloudflare Worker deployed per HS-053
  (`docs/worker-deployment.md`); cron verified firing 2026-11-24. Remaining
  open item: release-gate live evidence (HS-058 / issue #110).
- **RSS health**: Live dashboard pending (HS-38).

## Upgrading

No upgrade required — this is the first public release. The Cloudflare Worker
cron (every 30 minutes) triggers the refresh workflow, which rebuilds and
deploys the site per the HS-056 publish decision.

## Operator checklist

See `docs/release-checklist.md` → **Operator-facing v0.1 release checklist**
(live site, GitHub release, release notes, successful run links, release-gate
evidence, milestone gate).

## Feedback

File issues at: https://github.com/HuddleScope/huddlescope.github.io/issues

## License

MIT — see `LICENSE` file.
