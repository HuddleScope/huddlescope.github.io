-- 007_correction_rules.sql (issue #25 / HS-042)
--
-- Additive migration: a single `correction_rules` table storing versioned,
-- deterministic split/merge correction rules applied after automatic clustering.
-- No existing data is altered; the table starts empty.
--
-- article_ids is a JSON array of article IDs (sorted at application time).
-- created_by is plain text and never rendered with unescaped data.

PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS correction_rules (
    id            TEXT PRIMARY KEY,
    kind          TEXT NOT NULL,
    story_id      TEXT NOT NULL DEFAULT '',
    into_story_id TEXT NOT NULL DEFAULT '',
    article_ids   TEXT NOT NULL DEFAULT '[]',
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    created_by    TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_correction_rules_kind ON correction_rules(kind);
