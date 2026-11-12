# Homepage data contract — HS-015, HS-017

This directory contains data files consumed by Hugo for the homepage.

## `homepage.json`

The `homepage.json` file is the renderable projection of the domain `StoryGroup` (internal/domain/clustering.go)
scored by HS-012 (`internal/domain/scoring.go`). It powers the homepage display.

### Schema (v1)

```json
{
  "version": "1",
  "generatedAt": "2026-11-11T17:00:00Z",
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