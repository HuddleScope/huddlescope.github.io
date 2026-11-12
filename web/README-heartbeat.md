# Heartbeat data contract — HS-021

This directory contains the heartbeat file for publication observability.

## `heartbeat.json`

The `web/data/heartbeat.json` file tracks the status and freshness of each refresh run (ingestion, ranking, export).

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
| `version` | string | yes | Schema version (currently "1") |
| `status` | string | yes | One of `ok`, `partial`, `failed` |
| `run_id` | string | yes | Unique run identifier |
| `attempted_at` | string | yes | RFC3339 timestamp of run start |
| `ingested_at` | string (optional) | no | RFC3339 timestamp of ingestion completion (set on success) |
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
- Validation: version must be "1"; timestamps must be RFC3339; strings capped to 200 chars
- Error count capped: 5 per source, 20 total; secret patterns (password, secret, token, etc.) blocked in errors