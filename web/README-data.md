# Homepage data contract — HS-015, HS-017, HS-021

This directory contains data files consumed by Hugo for the homepage.

## `homepage.json`

The `homepage.json` file is the renderable projection of the domain `StoryGroup` (internal/domain/clustering.go)
scored by HS-012 (`internal/domain/scoring.go`). It powers the homepage display.

### Schema (v1)

```json
{
  "version": "1",
  "generatedAt": "2026-11-11T17:00:00Z",
## `heartbeat.json`

The `heartbeat.json` file tracks the status and freshness of each refresh run (ingestion, ranking, export).

### Schema (v1)

```json
{
  "version": "1",
  "status": "ok",
  "run_id": "e3b0c44298fc1c149afbf4c8996fb92427ae4194",
  "attempted_at": "2026-11-12T17:00:00Z",
  "ingested_at": "2026-11-12T17:00:05Z",
  "published_at": "2026-11-12T17:00:10Z",
  "sources_checked": 10,
  "sources_failed": 1,
  "errors": [
    "[source:nfl.com] connection refused",
    "repository not configured"
  ]
}
```

### Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `version` | string | yes | Schema version (currently `"1"`) |
| `status` | string | yes | One of `ok`, `partial`, `failed` |
| `run_id` | string | yes | Unique run identifier |
| `attempted_at` | string | yes | RFC3339 timestamp of run start |
| `ingested_at` | string (optional) | no | RFC3339 timestamp of ingestion completion |
| `published_at` | string (optional) | no | RFC3339 timestamp of last successful publication |
| `sources_checked` | integer | yes | Number of feed sources polled |
| `sources_failed` | integer | yes | Number of sources that failed with errors |
| `errors` | array of strings | no | Up to 20 errors (first 5 per source) |

### Status values

- `ok`: Run completed successfully (no ingestion or export errors)
- `partial`: Run completed with warnings (some sources failed but homepage committed)
- `failed`: Critical failure (run did not complete or homepage not committed)

### Freshness check

A heartbeat is considered stale when:
- `published_at` is missing, OR
- `published_at` is unparseable, OR
- `published_at` is older than the configured threshold (default: 90 minutes)

### Who writes it

- `cmd/ingest`: writes heartbeat with `attempted_at`, `ingested_at`, status and per-source errors
- `cmd/export`: reads existing heartbeat, merges publication result with `published_at`
- Both binaries write to `web/data/heartbeat.json` (configurable via `HSC_HEARTBEAT_FILE`)

### Who reads it

- GitHub Actions workflows (refresh, heartbeat-check)
- External monitoring (curl, webhooks)
- Humans via `gh workflow run` + `cat web/data/heartbeat.json`

### Guard/restore behavior

- `refresh-news.yml` data guard step: restores `web/data/homepage.json` AND `web/data/heartbeat.json` if export yields <1 story
- Commit step: commits whatever state exists (or no-op if unchanged)

### Error handling

- Heartbeat write failures are logged but do not affect run exit codes
- Validation: version must be `"1"`; timestamps must be RFC3339; strings capped to 200 chars
- Error count capped: 5 per source, 20 total; secret patterns (password, secret, token, etc.) blocked in errors
  "stories": [
    {
      "id": "top-free-agents-2024",
      "title": "Top 10 free agent signings of the offseason",
      "summary": "Short, original summary (not full article text).",
      "firstPublishedAt": "2024-07-25T15:30:00Z",
      "lastPublishedAt": "2024-08-01T10:00:00Z",
      "score": 81.5,
      "evidenceLevel": "confirmed",
      "publishers": [
        { "id": "nfl-com", "name": "NFL.com", "url": "https://www.nfl.com/news/..." },
        { "id": "espn-nfl", "name": "ESPN",    "url": "https://www.espn.com/nfl/..." }
      ]
    }
  ],
  "latest": [
    {
      "source_id": "nfl-com",
      "source_name": "NFL.com",
      "title": "Latest story headline",
      "url": "https://www.nfl.com/news/latest",
      "published_at": "2026-11-11T17:00:00Z"
    }
  ],
  "teams": [
    {
      "slug": "49ers",
      "name": "San Francisco 49ers",
      "short_name": "49ers",
      "abbreviation": "SF"
    }
  ]
}
```

#### Top-level fields

- `version` (required, string): export format version (currently `"1"`).
- `generatedAt` (required, string, RFC3339): snapshot timestamp used for the "Data as of" metadata. Never implies real-time.

#### Story fields (within `stories` array)

- `id` (required, string): unique identifier (same as the content page slug under `/stories/`).
- `title` (required, string): headline, not the full article.
- `summary` (required, string): short, original summary (not full copyrighted article text).
- `firstPublishedAt` (required, string, RFC3339): original publication time.
- `lastPublishedAt` (required, string, RFC3339): last update time; used for sorting "Latest updates".
- `score` (required, number 0–100): HS-012 explainable score. Higher means more relevant.
- `evidenceLevel` (required, one of: `confirmed`, `unverified`, `unknown`): mirrors `domain.EvidenceLevel`.
- `publishers` (required, array): unique independent publishers deduplicated by `id`.

  - `id` (required, string): source identifier.
  - `name` (required, string): publisher display name.
  - `url` (required, string): direct link to the original coverage.

#### Latest articles (within `latest` array)

- `source_id` (required, string): source identifier.
- `source_name` (required, string): publisher display name.
- `title` (required, string): article headline.
- `url` (required, string): direct link to the article.
- `published_at` (required, string, RFC3339): article publication time.

#### Teams (within `teams` array)

- `slug` (required, string): team identifier used in URLs.
- `name` (required, string): full team name.
- `short_name` (required, string): abbreviated team name.
- `abbreviation` (required, string): two- or three-letter team abbreviation.

### Determinism and idempotency

The export produces deterministic output:

- Stories are sorted by `id` ascending.
- Publishers within each story are sorted by `id` ascending.
- Latest articles are sorted by `published_at` descending, then `source_id` descending.
- Teams are sorted by `slug` ascending.
- The `generatedAt` timestamp is the only wall-clock field; content comparison excludes it for idempotency.

The Go engine compares content (excluding `generatedAt`) with the existing file on disk and skips the write if unchanged.
This ensures the site build produces identical output when no relevant content has changed.

### No-secret and no-body guarantees

The export contract enforces:
- Summaries are bounded at 1024 characters to prevent full copyrighted article text from leaking.
- Secret patterns (`password`, `token`, `api_key`, etc.) are scanned in summaries and rejected.
- Only whitelisted fields are exported; database credentials and private metadata are never included.

### Implementation notes

- The Go engine produces `homepage.json` via `cmd/export/main.go` when #17 runs.
- This directory is the *only* contract between the engine and the rendering layer.
- `internal/domain/scoring.go.ClassifyEvidence` maps to `evidenceLevel` one-to-one.
- Score ordering (desc) is applied by the template; the JSON file itself can be unsorted.
- `version` enables forward-compatible schema evolution.

### Empty state

An empty array `{"version":"1","generatedAt":"…","stories":[],"latest":[],"teams":[]}` is valid and renders the "No stories published yet" message.

### Fallback

If `homepage.json` is missing or fails to parse, Hugo renders the fallback empty state.