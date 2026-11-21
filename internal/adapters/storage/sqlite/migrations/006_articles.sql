-- 006_articles.sql (HS-064 / issue #119)
--
-- 1. Rebuilds `articles` so it can hold application-inserted rows:
--    - article_id is a canonical story/group id (16-hex), NOT a UUID 36
--    - source_id is a registry slug, no UUID FK (HS-064 relaxation)
--    - url/guid/title nullable; non-empty guids stay source-unique
--    - new `entities` TEXT (JSON array) for round-trip fidelity

-- 2. Rebuilds `articles_archive` as the mirror (adds `entities` before `archived_at`).
-- 3. Backfills `articles` from `stories.story_data`:
--    a) embedded `articleData` objects (checkpoint format),
--    b) `articleIDs` arrays (legacy format), hydrated from member story rows.
-- 4. Backfills `story_articles` links from both JSON shapes.
-- 5. Strips the now-redundant `articleData` key from `story_data` (articles are
--    the single copy in the articles table); every other story_data key is kept.
--
-- Runs exactly once per DB (schema_migrations guard; single transaction).
-- Turso/libSQL and modernc both expose the JSON1 functions used below.

PRAGMA foreign_keys = ON;

-- 1. Rebuild articles
CREATE TABLE articles_new (
    article_id    TEXT PRIMARY KEY,
    source_id     TEXT NOT NULL,
    url           TEXT,
    guid          TEXT,
    title         TEXT,
    summary       TEXT,
    author        TEXT,
    published_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    first_seen_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    entities      TEXT
);
INSERT INTO articles_new (article_id, source_id, url, guid, title, summary, author, published_at, first_seen_at)
    SELECT article_id, source_id, url, guid, title, summary, author, published_at, first_seen_at FROM articles;
DROP TABLE story_articles;             -- references articles; drop before the swap
DROP TABLE articles;
ALTER TABLE articles_new RENAME TO articles;
CREATE INDEX IF NOT EXISTS idx_articles_source_published ON articles(source_id, published_at DESC);
CREATE INDEX IF NOT EXISTS idx_articles_guid ON articles(guid);
CREATE INDEX IF NOT EXISTS idx_articles_url ON articles(url);
CREATE UNIQUE INDEX IF NOT EXISTS idx_articles_source_guid
    ON articles(source_id, guid) WHERE guid IS NOT NULL AND guid <> '';

-- 2. Rebuild archive mirror (same business-column order; entities before archived_at)
CREATE TABLE articles_archive_new (
    article_id    TEXT NOT NULL PRIMARY KEY,
    source_id     TEXT NOT NULL,
    url           TEXT,
    guid          TEXT,
    title         TEXT,
    summary       TEXT,
    author        TEXT,
    published_at  TEXT NOT NULL,
    first_seen_at TEXT NOT NULL,
    entities      TEXT,
    archived_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);
INSERT INTO articles_archive_new (article_id, source_id, url, guid, title, summary, author, published_at, first_seen_at, archived_at)
    SELECT article_id, source_id, url, guid, title, summary, author, published_at, first_seen_at, archived_at FROM articles_archive;
DROP TABLE articles_archive;
ALTER TABLE articles_archive_new RENAME TO articles_archive;

-- 3a. Backfill from embedded articleData objects (new/checkpoint format)
INSERT OR IGNORE INTO articles
    (article_id, source_id, url, guid, title, summary, author, published_at, first_seen_at, entities)
SELECT
    json_extract(a.value, '$.ID'),
    COALESCE(json_extract(a.value, '$.SourceID'), json_extract(st.story_data, '$.source'), ''),
    COALESCE(json_extract(a.value, '$.URL'), json_extract(st.story_data, '$.url'), json_extract(st.story_data, '$.canonicalURL'), ''),
    COALESCE(json_extract(a.value, '$.GUID'), ''),
    COALESCE(json_extract(a.value, '$.Title'), json_extract(st.story_data, '$.title'), ''),
    COALESCE(json_extract(a.value, '$.Summary'), json_extract(st.story_data, '$.summary'), ''),
    COALESCE(json_extract(a.value, '$.Author'), ''),
    COALESCE(json_extract(a.value, '$.PublishedAt'), st.published_at),
    COALESCE(json_extract(a.value, '$.FirstSeenAt'), st.first_seen_at),
    COALESCE((SELECT json_group_array(e.value) FROM json_each(a.value, '$.Entities') e), '[]')
FROM stories st
CROSS JOIN json_each(st.story_data, '$.articleData') a
WHERE st.story_data IS NOT NULL
  AND json_valid(st.story_data)
  AND a.type = 'object'
  AND json_extract(a.value, '$.ID') IS NOT NULL
  AND json_extract(a.value, '$.ID') <> '';

-- 3b. Backfill from legacy articleIDs, hydrated from member story rows
--     (member row wins; group story fields as fallback; OR IGNORE + NOT EXISTS)
INSERT OR IGNORE INTO articles
    (article_id, source_id, url, guid, title, summary, author, published_at, first_seen_at, entities)
SELECT
    aid.value,
    COALESCE(json_extract(ms.story_data, '$.source'), json_extract(st.story_data, '$.source'), ''),
    COALESCE(json_extract(ms.story_data, '$.url'), ms.canonical_url,
             json_extract(st.story_data, '$.url'), json_extract(st.story_data, '$.canonicalURL'), ''),
    COALESCE(ms.guid, json_extract(ms.story_data, '$.guid'), json_extract(st.story_data, '$.guid'), ''),
    COALESCE(ms.title, json_extract(ms.story_data, '$.title'), json_extract(st.story_data, '$.title'), ''),
    COALESCE(ms.summary, json_extract(ms.story_data, '$.summary'), json_extract(st.story_data, '$.summary'), ''),
    '',
    COALESCE(ms.published_at, st.published_at),
    COALESCE(ms.first_seen_at, st.first_seen_at),
    COALESCE((SELECT json_group_array(e.value) FROM json_each(COALESCE(ms.story_data, st.story_data), '$.entities') e), '[]')
FROM stories st
CROSS JOIN json_each(st.story_data, '$.articleIDs') aid
LEFT JOIN stories ms ON ms.story_key = aid.value
WHERE st.story_data IS NOT NULL
  AND json_valid(st.story_data)
  AND aid.value IS NOT NULL AND aid.value <> ''
  AND NOT EXISTS (SELECT 1 FROM articles ar WHERE ar.article_id = aid.value);

-- 4. Recreate link table + backfill links from both JSON shapes
CREATE TABLE story_articles (
    story_id   TEXT NOT NULL REFERENCES stories(story_id) ON DELETE CASCADE,
    article_id TEXT NOT NULL REFERENCES articles(article_id) ON DELETE CASCADE,
    joined_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    UNIQUE (story_id, article_id)
);
INSERT OR IGNORE INTO story_articles (story_id, article_id)
SELECT st.story_id, aid.value
FROM stories st
CROSS JOIN json_each(st.story_data, '$.articleIDs') aid
WHERE st.story_data IS NOT NULL
  AND json_valid(st.story_data)
  AND aid.value IS NOT NULL AND aid.value <> '';
INSERT OR IGNORE INTO story_articles (story_id, article_id)
SELECT st.story_id, json_extract(a.value, '$.ID')
FROM stories st
CROSS JOIN json_each(st.story_data, '$.articleData') a
WHERE st.story_data IS NOT NULL
  AND json_valid(st.story_data)
  AND json_extract(a.value, '$.ID') IS NOT NULL
  AND json_extract(a.value, '$.ID') <> '';
CREATE INDEX IF NOT EXISTS idx_story_articles_story ON story_articles(story_id);

-- 5. Strip embedded articleData (only the duplicated payload; keep everything else)
UPDATE stories
SET story_data = json_remove(story_data, '$.articleData')
WHERE story_data IS NOT NULL
  AND json_valid(story_data)
  AND json_extract(story_data, '$.articleData') IS NOT NULL;