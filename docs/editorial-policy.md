# Editorial/source policy

HuddleScope is a non-commercial, independent news aggregator for NFL-related content. This document describes our editorial boundaries, copyright approach, and operational practices.

## What we publish

We do not republish full copyrighted articles or unlicensed images. Our publishing model is deliberately limited to ensure fair use and respect for source publishers.

- **What we do publish**
  - Source URL: direct link to the original article
  - Publisher credit: display name of the original publisher (ESPN, NFL.com, etc.)
  - Original link: clickable URL to the full article on the publisher's site
  - Short excerpt: original headline and a brief, original summary written in our words (not the full body)
  - Author, date/time, and source metadata

- **What we never publish**
  - Full article body or content
  - Images not licensed for republication
  - Translations without explicit permission

This approach is enforced in our rendering model (`internal/domain/storypage.go`), where the `PublisherArticle` struct deliberately omits article body/content fields.

## RSS availability is not blanket permission

Just because a source provides an RSS feed does not mean we have permission to republish its content. The feed is a delivery mechanism for news distribution, not a copyright license.

- **Per-source licensing**
  - Each source in `conf/sources.yaml` has `license` and `acceptable_use` fields
  - `acceptable_use` values: `allowed` (may use), `restricted` (limited use), `unknown` (do not use for republication)
  - Sources with `restricted` or `unknown` are not used for content republishing

- **What to do if a source changes**
  - If a publisher disables or removes RSS access, we stop using that source
  - If a publisher explicitly requests removal, see Data deletion and opt-out requests below

## Rate limiting and etiquette

We respect publisher resources and terms of service:

- **User-Agent**: Every request includes a `User-Agent` header with version and contact (`HSC_USER_AGENT`); defaults to `huddlescope/0.1` (workflow may override with `huddlescope-ci/0.1`)
- **Per-host rate limiting**: The RSS adapter (`internal/adapters/rss/fetcher.go`) enforces minimum intervals between requests to the same host
- **Retry policy**: Exponential backoff with jitter; maximum delay 30 seconds; only retries on transient failures (429 Too Many Requests, network errors)
- **ETag/304 support**: We respect HTTP 304 Not Modified responses to avoid unnecessary downloads

## Data deletion and opt-out requests

If a source publisher or content owner requests removal:

1. **Identify the data**
   - We store article metadata only: URL, title, summary, author, published_at, source_id, timestamps
   - We do not store full article body or images
   - Data lives in Turso Cloud (database), with exports to `web/data/homepage.json` for Hugo

2. **Deletion procedure**
   - Set `stories.status = 'removed'` for affected records
   - Re-run the export pipeline (`./dist/export`) to regenerate `web/data/homepage.json`
   - Deploy to GitHub Pages if the change should appear on the public site

3. **Response time commitment**
   - We aim to process removal requests within 7 days
   - If the request is valid, we remove the content from our system and deploy the change

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

## Operational notes

See `docs/ops-runbook.md` for detailed operational procedures:

- **Free-tier topology**: Cloudflare Workers (48/day cron), GitHub Actions, GitHub Pages, Turso free tier (~1 GB DB)
- **Backup/restore**: Operational Turso backup procedure (code stubs pending HS-041)
- **Monitoring**: Turso storage usage, Actions usage, Worker requests dashboard
- **Secret rotation**: TURSO_DATABASE_URL, TURSO_AUTH_TOKEN (GitHub Secrets), GITHUB_TOKEN (Cloudflare Workers, 90-day)
- **Local execution**: `make build`, `./dist/ingest`, `./dist/export`, `make site`, `make site-test`

## Known limitations

- **Backups**: The in-repo `BackupToFile`, `RestoreFromFile`, `Dump`, and `ApplyRetention` functions are currently stubs; the operational backup path uses Turso Cloud console or `turso db shell` (pending HS-041 implementation)
- **Retention**: Automatic archival of old data is not yet implemented; manual Turso queries required to archive rows older than 90 days (pending HS-041)