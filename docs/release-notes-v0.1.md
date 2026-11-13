# HuddleScope v0.1 release notes

**Release date**: 2026-11-13  
**GitHub Pages**: https://huddlescope.github.io/

## What ships

HuddleScope v0.1 is the first publicly available version of an independent NFL
news aggregator built with a modular Go engine, Turso database, and static Hugo
site generation.

### Site features

- **Curated NFL news**: Stories aggregated from national and team-level RSS feeds
  (ESPN, NFL.com,CBS Sports,FOX Sports, teamOfficial feeds).
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

## Architecture

```
Cloudflare Worker (cron) ─┐
                          ├─► GitHub Action (refresh-news.yml)
GitHub Pages deploy ◄─────┤
                          ├─► Go ingest (RSS fetch, cluster, score)
Hugo site ◄───────────────┤
                          └─► Export → web/data/homepage.json
```

## Known limitations

- **Storage**: In-memory repository. Turso persistence (HS-040) pending.
- **Delivery status**: Cloudflare Worker dispatch verified manually only.
- **RSS health**: Live dashboard pending (HS-38).

## Upgrading

No upgrade required — this is the first public release. New commits to `main`
automatically trigger a refresh workflow that rebuilds and deploys the site.

## Feedback

File issues at: https://github.com/HuddleScope/huddlescope.github.io/issues

## License

MIT — see `LICENSE` file.