# Homepage data contract — HS-015

This directory contains data files consumed by Hugo for the homepage.

## `homepage.json`

The `homepage.json` file is the renderable projection of the domain `StoryGroup` (internal/domain/clustering.go)
scored by HS-012 (`internal/domain/scoring.go`). It powers the homepage display.

### Schema

```json
{
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
  ]
}
```

#### Top-level fields

- `generatedAt` (required, string, RFC3339): snapshot timestamp used for the "Data as of" metadata. Never implies real-time.

#### Story fields

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

### Implementation notes

- The Go engine produces `homepage.json` via the `export` command when #17 runs.
- This directory is the *only* contract between the engine and the rendering layer.
- `internal/domain/scoring.go.ClassifyEvidence` maps to `evidenceLevel` one-to-one.
- Score ordering (desc) is applied by the template; the JSON file itself can be unsorted.

### Empty state

An empty array `{"generatedAt":"…","stories":[]}` is valid and renders the "No stories published yet" message.

### Fallback

If `homepage.json` is missing or fails to parse, Hugo renders the fallback empty state.