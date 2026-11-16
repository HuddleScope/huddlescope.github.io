-- HS-041: Archive tables for data retention (hot table mirrors, top-level, no schema)
-- Forward-only; added after HS-005's initial schema and HS-040's structural cleanup.
-- archive tables store historical rows moved from hot tables for compliance/retention.
-- Top-level (no CREATE SCHEMA) because modernc.org/sqlite does not support schemas.

-- articles_archive: archive of articles (mirrors articles columns + archived_at)
CREATE TABLE IF NOT EXISTS articles_archive (
    article_id TEXT NOT NULL PRIMARY KEY CHECK (length(article_id) = 36),
    source_id TEXT NOT NULL,
    url TEXT NOT NULL,
    guid TEXT,
    title TEXT NOT NULL,
    summary TEXT,
    author TEXT,
    published_at TEXT NOT NULL,
    first_seen_at TEXT NOT NULL,
    archived_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

-- stories_archive: archive of stories (mirrors stories columns + archived_at)
CREATE TABLE IF NOT EXISTS stories_archive (
    story_id TEXT NOT NULL PRIMARY KEY CHECK (length(story_id) = 36),
    title TEXT NOT NULL,
    summary TEXT,
    published_at TEXT NOT NULL,
    first_seen_at TEXT NOT NULL,
    story_key TEXT NOT NULL,
    canonical_url TEXT,
    guid TEXT,
    story_data TEXT,
    dedupe_key TEXT,
    updated_at TEXT,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived', 'duplicate', 'removed')),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    headline TEXT,
    article_ids TEXT,
    archived_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

-- ranking_snapshots_archive: archive of ranking snapshots
CREATE TABLE IF NOT EXISTS ranking_snapshots_archive (
    snapshot_id TEXT NOT NULL PRIMARY KEY CHECK (length(snapshot_id) = 36),
    rank_date TEXT NOT NULL,
    team_id TEXT NOT NULL,
    rank INTEGER NOT NULL,
    metric TEXT NOT NULL,
    value REAL NOT NULL,
    archived_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);