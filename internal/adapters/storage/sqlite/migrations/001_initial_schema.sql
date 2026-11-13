-- HS-005: Initial schema with normalized tables for sources, articles, stories, teams, and ranking history
-- Forward-only migrations; SQLite compatible.

-- Enable foreign keys (must be set per connection)
PRAGMA foreign_keys = ON;

-- sources: master feed sources
-- slug = stable business key derived from config id; guid = optional feed-level GUID
CREATE TABLE IF NOT EXISTS sources (
    source_id TEXT PRIMARY KEY CHECK (length(source_id) = 36),  -- UUID
    slug TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    feed_url TEXT NOT NULL,
    guid TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')) CHECK (length(created_at) = 20 AND substr(created_at, 20) = 'Z')
);

-- source_fetches: fetch history per source
CREATE TABLE IF NOT EXISTS source_fetches (
    fetch_id TEXT PRIMARY KEY CHECK (length(fetch_id) = 36),  -- UUID
    source_id TEXT NOT NULL REFERENCES sources(source_id) ON DELETE CASCADE,
    guid TEXT,
    feed_title TEXT,
    fetched_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')) CHECK (length(fetched_at) = 20 AND substr(fetched_at, 20) = 'Z'),
    items_total INTEGER NOT NULL DEFAULT 0,
    items_new INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_source_fetches_source_fetched ON source_fetches(source_id, fetched_at DESC);

-- articles: raw feed entries, can be associated to multiple stories
CREATE TABLE IF NOT EXISTS articles (
    article_id TEXT PRIMARY KEY CHECK (length(article_id) = 36),  -- UUID
    source_id TEXT NOT NULL REFERENCES sources(source_id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    guid TEXT,
    title TEXT NOT NULL,
    summary TEXT,
    author TEXT,
    published_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')) CHECK (length(published_at) = 20 AND substr(published_at, 20) = 'Z'),
    first_seen_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')) CHECK (length(first_seen_at) = 20 AND substr(first_seen_at, 20) = 'Z'),
    UNIQUE(source_id, guid),
    UNIQUE(url)
);

CREATE INDEX IF NOT EXISTS idx_articles_source_published ON articles(source_id, published_at DESC);
CREATE INDEX IF NOT EXISTS idx_articles_guid ON articles(guid);

-- stories: normalized narratives built from articles
-- story_key = normalized(title|source|published) for stable dedup
CREATE TABLE IF NOT EXISTS stories (
    story_id TEXT PRIMARY KEY CHECK (length(story_id) = 36),  -- UUID
    title TEXT NOT NULL,
    summary TEXT,
    published_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')) CHECK (length(published_at) = 20 AND substr(published_at, 20) = 'Z'),
    first_seen_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')) CHECK (length(first_seen_at) = 20 AND substr(first_seen_at, 20) = 'Z'),
    story_key TEXT NOT NULL UNIQUE,
    canonical_url TEXT,
    guid TEXT,
    story_data TEXT,  -- JSON payload for full story persistence
    dedupe_key TEXT,  -- secondary dedup key for deduplication
    updated_at TEXT,  -- last update timestamp
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived', 'duplicate', 'removed')),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')) CHECK (length(created_at) = 20 AND substr(created_at, 20) = 'Z')
);

CREATE INDEX IF NOT EXISTS idx_stories_status_firstseen ON stories(status, first_seen_at DESC);

-- story_articles: many-to-many link between stories and articles
CREATE TABLE IF NOT EXISTS story_articles (
    story_id TEXT NOT NULL REFERENCES stories(story_id) ON DELETE CASCADE,
    article_id TEXT NOT NULL REFERENCES articles(article_id) ON DELETE CASCADE,
    PRIMARY KEY(story_id, article_id)
);

CREATE INDEX IF NOT EXISTS idx_story_articles_article ON story_articles(article_id);

-- teams: sports team metadata
CREATE TABLE IF NOT EXISTS teams (
    team_id TEXT PRIMARY KEY CHECK (length(team_id) = 36),  -- UUID
    slug TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    abbreviation TEXT NOT NULL UNIQUE
);

-- article_teams: team attribution per article
CREATE TABLE IF NOT EXISTS article_teams (
    article_id TEXT NOT NULL REFERENCES articles(article_id) ON DELETE CASCADE,
    team_id TEXT NOT NULL REFERENCES teams(team_id) ON DELETE CASCADE,
    PRIMARY KEY(article_id, team_id)
);

CREATE INDEX IF NOT EXISTS idx_article_teams_team ON article_teams(team_id);

-- ranking_snapshots: bounded ranking history (hot table, 90-day retention policy)
CREATE TABLE IF NOT EXISTS ranking_snapshots (
    snapshot_id TEXT PRIMARY KEY CHECK (length(snapshot_id) = 36),  -- UUID
    rank_date TEXT NOT NULL CHECK (length(rank_date) = 10 AND substr(rank_date, 5, 1) = '-' AND substr(rank_date, 8, 1) = '-'),  -- YYYY-MM-DD
    team_id TEXT NOT NULL REFERENCES teams(team_id) ON DELETE CASCADE,
    rank INTEGER NOT NULL CHECK (rank > 0),
    metric TEXT NOT NULL,
    value REAL NOT NULL,
    UNIQUE(rank_date, metric, team_id)
);

CREATE INDEX IF NOT EXISTS idx_ranking_snapshots_date ON ranking_snapshots(rank_date);
CREATE INDEX IF NOT EXISTS idx_ranking_snapshots_team_date ON ranking_snapshots(team_id, rank_date);

-- ingestion_runs: ingestion execution history
CREATE TABLE IF NOT EXISTS ingestion_runs (
    run_id TEXT PRIMARY KEY CHECK (length(run_id) = 36),  -- UUID
    started_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')) CHECK (length(started_at) = 20 AND substr(started_at, 20) = 'Z'),
    finished_at TEXT,
    sources_total INTEGER NOT NULL DEFAULT 0,
    sources_ok INTEGER NOT NULL DEFAULT 0,
    articles_new INTEGER NOT NULL DEFAULT 0,
    stories_new INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'completed', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_ingestion_runs_started ON ingestion_runs(started_at DESC);