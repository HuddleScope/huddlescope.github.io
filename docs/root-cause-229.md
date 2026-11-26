# Root-Cause Report — Issue #229: NFL stories disappearing after refresh or Pages deployment

## Executive summary

The disappearance is caused by **two unguarded, non-serialized GitHub Pages publishers racing each other**: `refresh-news.yml` deploys a freshly exported, fully-populated site (with individual story pages), while `site.yml` deploys on *any* green CI on `main` building from the **committed** `web/data/homepage.json` — which does NOT contain the individual story content (those files are generated at export time and never committed). When a `site.yml` deploy lands after a `refresh-news` deploy, it **replaces the full site with a skeleton site** whose story detail pages no longer exist.

This report references real workflow executions (run IDs, commit SHAs, artifact hashes) collected read-only via `gh api` on 2026-11-25 (T-1 min to base `b922e71`).

## Evidence

### E1 — Committed snapshot vs. live site are different revisions

| Item | Value |
|------|-------|
| Committed `web/data/homepage.json` @ `b922e71` | `generatedAt: 2026-11-25T17:05:00Z`, **3729 stories** |
| Deployed site (HTTP `https://huddlescope.github.io/`) | featured story `2026-11-25T17:22:41Z` (Baltimorebeatdown preview) |
| `web/content/stories/` in committed repo | `_index.md` only — **no individual story pages committed** |

The live site is ~17 min **newer** than the committed snapshot. Committed ≠ deployed.

### E2 — The failing deploy pair (smoking gun)

| Run | Workflow | Created | Head SHA | Conclusion | `github-pages` artifact |
|-----|----------|---------|----------|------------|------------------------|
| [45671828869](https://github.com/HuddleScope/huddlescope.github.io/actions/runs/45671828869) (#420) | Refresh news | 2026-11-25T18:07:12Z | `b922e71` | success | artifact `15049813469`, **2,695,225 bytes** — 3,790 story pages, 3,807 files |
| [45673311370](https://github.com/HuddleScope/huddlescope.github.io/actions/runs/45673311370) (#282) | Site | 2026-11-25T18:15:43Z | `0d7142773` (site-data merge) | success | artifact `15051160935`, **16,066 bytes** — **0 story pages**, only listing/static files |

Extracted artifact contents:

- `refresh-news` artifact: `/stories/*/index.html` for 3,790 stories (full site).
- `site.yml` artifact: `about/`, `status/`, `sources/`, `stories/index.html` (listing only), `index.html` — **no individual story pages**.

**Transition:** `refresh-news` #420 deployed the full site at ~18:14 (artifact created 18:14:12Z). Seven minutes later, `site.yml` #282 deployed the skeleton site (artifact 18:16:11Z). Pages is last-writer-wins: after #282, every `/stories/<slug>/` URL 404s even though the homepage listing still renders (it reads from Hugo data at build time). This is exactly the reported symptom: "stories disappear after a refresh … even though previously visible stories existed."

### E3 — Cancellation inside the publish window (hypothesis 3, confirmed in code)

`refresh-news.yml` run history (2026-11-25): runs #419 (18:04:15Z) and #416 (17:03:33Z) were **cancelled** by newer dispatches while `cancel-in-progress: true`. The concurrency group was per-`ref` (`refresh-news-${{ github.ref }}`) and **independent** of `site.yml`'s `site-…` group, so `deploy-pages` calls from the two workflows interleaved with no shared ordering.

### E4 — Unpinned checkout in `site.yml` (hypothesis 4, confirmed in code)

`site.yml`'s deploy job used `actions/checkout` with **no `ref`** under `workflow_run`; the exact SHA built/deployed was not recorded in the workflow. Run #282's `head_sha` (`0d7142773`) is a site-data merge commit, not the CI-validated commit the operator expected.

### E5 — Guard compares committed, not deployed (hypothesis 2, confirmed in code)

`refresh-news.yml`'s old `Publish decision` step compared the fresh export to `git show HEAD:web/data/homepage.json`. No record of the last *deployed* snapshot existed anywhere (no manifest, no deployed marker), so committed ≠ deployed whenever runs, cancellations, or PRs interleaved.

### E6 — Empty-export semantics (hypothesis 5, confirmed as contributing path)

`HSC_LOOKBACK=24h` + ranking filters can legitimately yield 0 stories while Turso holds thousands of older rows (`docs/ops-runbook.md`, verified 2026-11-24: 0 exported while Turso held 3017). The old guard retained last-good *only within* refresh-news and only against the *committed* file; `site.yml` had no guard at all, so a site deploy after a zero export shipped zero stories unconditionally.

### E7 — Retention (hypothesis 6, excluded by inspection)

`retention export` runs before the site-data commit step (the package ships in the PR) and `retention apply` runs only after the PR merge, after all DB reads and after the Hugo build. Ordering is safe as written; no evidence of retention contributing to the observed disappearances.

## Exact faulty transition(s)

1. **Primary:** `site.yml` deploy (#282, `0d7142773`, 18:16:11Z) overwrote the full `refresh-news` deployment (#420, `b922e71`, ~18:14Z) with a skeleton build — the committed `web/data/homepage.json` has story metadata but the committed `web/content/stories/` has no story pages, so the Hugo build produced a site with no individual story URLs. This fired because the two publishers were unguarded and non-serialized (E2, E3).
2. **Contributing:** the committed snapshot lagged the deployed snapshot by ~17 min (E1), so even a "correct" site.yml build from `main` would have shipped older data than what a concurrent refresh had just published.
3. **Latent:** `cancel-in-progress: true` allowed refresh runs to be cancelled inside the publish window (E3), and the unpinned checkout (E4) made the deployed revision ambiguous.

## Fix (this PR)

A single authoritative, serialized, monotonic publish decision: a tracked `web/data/deployed.json` manifest plus the pure, offline `huddlescope publish gate` / `publish stamp` CLI used by **both** workflows, under a **shared concurrency group** `huddlescope-pages-deploy` with `cancel-in-progress: false`. `deny` and `skip-identical` verdicts make stale or zero-over-N deployments impossible; `site.yml` restores last-deployed data (via `git archive` of the manifest commit) on deny so design changes still ship without a data regression.
