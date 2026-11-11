# HuddleScope site

This directory contains the Hugo source for the HuddleScope web site.

## Structure

- `content/` — Markdown pages and articles (home, stories, sources, about)
- `layouts/` — Hugo templates (base, home, list, single, partials)
- `assets/css/main.css` — responsive, mobile-first styles with dark mode
- `assets/js/main.js` — minimal, no tracking
- `data/` — data files consumed by Hugo (see `data/README.md`)
- `static/` — static assets (favicon, etc.)
- `hugo.toml` — site configuration

## Commands

- `make site` — build site to `web/public/`
- `make site-test` — verify build (success/failure/idempotency)
- `make site-watch` — local preview server

See [`../README.md`](../README.md) for the full development workflow.