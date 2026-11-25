-- 010_display_title.sql (issue #201 / UI-04)
--
-- Adds a `display_title` column to `articles` and `stories` that carries the
-- original feed casing (trim + collapse whitespace + NFC, case/punctuation
-- preserved). The existing `title`/`headline` columns continue to hold the
-- normalized (lowercased) identity value; `display_title` is display-only.
--
-- Empty display_title falls back to the normalized title at the domain layer
-- (#201, AC-3). We backfill existing rows with the normalized value so that
-- pre-change rows render exactly as today (the accepted trade-off: no
-- one-time backfill of original casing — those rows regain display casing on
-- re-ingestion while the feed still serves the article).
--
-- `ALTER TABLE ... ADD COLUMN` with a constant DEFAULT is additive and
-- Turso/libSQL-compatible. The schema_migrations guard (version 10) ensures
-- this runs exactly once per DB. Single transaction, no data loss.

PRAGMA foreign_keys = ON;

-- articles: backfill from the normalized title (NOT NULL, default '').
ALTER TABLE articles ADD COLUMN display_title TEXT NOT NULL DEFAULT '';
UPDATE articles SET display_title = COALESCE(title, '');

-- stories: backfill from the normalized headline, falling back to title.
ALTER TABLE stories ADD COLUMN display_title TEXT NOT NULL DEFAULT '';
UPDATE stories SET display_title = COALESCE(headline, COALESCE(title, ''));
