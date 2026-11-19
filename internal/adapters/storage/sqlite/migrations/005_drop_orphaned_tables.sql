-- HS-063 (issue #118): drop orphaned teams and article_teams tables.
-- Team metadata lives in conf/teams.yaml via config.TeamRegistry(); the DB
-- tables were never read or written by application code.
--
-- Order matters and is required with PRAGMA foreign_keys = ON (set by the
-- migration runner at connection level):
--   1. article_teams references teams -> drop it first (index
--      idx_article_teams_team is removed automatically with the table).
--   2. ranking_snapshots has a resolved FK to teams and must survive (used
--      by retention archiving and backups). Rebuild it without the FK:
--      the team_id column is kept as plain TEXT NOT NULL, matching the
--      ranking_snapshots_archive mirror in 004.
--   3. teams is now unreferenced -> drop it.

-- 1. article_teams (referencing table)
DROP TABLE IF EXISTS article_teams;

-- 2. ranking_snapshots rebuild (SQLite has no DROP CONSTRAINT)
CREATE TABLE ranking_snapshots_new (
    snapshot_id TEXT PRIMARY KEY CHECK (length(snapshot_id) = 36),  -- UUID
    rank_date TEXT NOT NULL CHECK (length(rank_date) = 10 AND substr(rank_date, 5, 1) = '-' AND substr(rank_date, 8, 1) = '-'),
    team_id TEXT NOT NULL,  -- FK to teams removed by HS-063; team identity is a business key from conf/teams.yaml
    rank INTEGER NOT NULL CHECK (rank > 0),
    metric TEXT NOT NULL,
    value REAL NOT NULL,
    UNIQUE(rank_date, metric, team_id)
);
INSERT INTO ranking_snapshots_new (snapshot_id, rank_date, team_id, rank, metric, value)
    SELECT snapshot_id, rank_date, team_id, rank, metric, value FROM ranking_snapshots;
DROP TABLE ranking_snapshots;
ALTER TABLE ranking_snapshots_new RENAME TO ranking_snapshots;
CREATE INDEX idx_ranking_snapshots_date ON ranking_snapshots(rank_date);
CREATE INDEX idx_ranking_snapshots_team_date ON ranking_snapshots(team_id, rank_date);

-- 3. teams (now unreferenced)
DROP TABLE IF EXISTS teams;