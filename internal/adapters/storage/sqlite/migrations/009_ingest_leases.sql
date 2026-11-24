-- HS-PORT-04 / issue #160: distributed mutual exclusion for the ingest
-- pipeline (primary vs backup host) + the two new run statuses the issue
-- mandates be *registered* on the ingest_runs history table.
--
-- This migration does TWO things, both forward-only:
--
--   1. Creates the ingest_lease table (new, single fixed key "rss-ingest").
--   2. Rebuilds ingest_runs in place to widen the status CHECK from
--      (running, ok, partial, failed) to also accept
--      (skipped_locked, skipped_fresh).
--
-- WHY REBUILD ingest_runs:
--   SQLite cannot ALTER a CHECK constraint in place. The issue requires the
--   runner to REGISTER skipped_locked / skipped_fresh rows, so the CHECK must
--   admit them. The standard forward-only rebuild is:
--     CREATE ingest_runs_new (extended CHECK) -> INSERT OR IGNORE ... SELECT
--     -> DROP ingest_runs -> ALTER TABLE RENAME -> recreate index.
--
-- DATA CARRY-OVER (loss-free):
--   INSERT OR IGNORE ... SELECT copies every existing row verbatim. The new
--   table's column set is identical to the old one (same names/order), so no
--   column is dropped or renamed. INSERT OR IGNORE keeps the step idempotent
--   if the migration body is ever re-applied (a run_id already present is
--   ignored, never duplicated).
--
-- IDEMPOTENCY ON RE-APPLICATION:
--   MigrateDB is guarded by schema_migrations (version 9 recorded after this
--   body runs), so the body never runs twice on a healthy DB. The INSERT OR
--   IGNORE above is a belt-and-suspenders safeguard only; the DROP/RENAME pair
--   is the destructive step, which is why we rely on schema_migrations.
--
-- RETENTION (carried over from 008, unchanged):
--   rows are small and bounded; prune older than 90 days:
--     DELETE FROM ingest_runs
--     WHERE COALESCE(finished_at, started_at) <
--           strftime('%Y-%m-%dT%H:%M:%SZ','now','-90 days');

-- (1) The lease table. lease_key is a fixed single-row key ("rss-ingest");
-- the schema is written generically (PRIMARY KEY on lease_key) so a future
-- additional lease would not require another migration, but the issue ships
-- only the one ingest key. holder_id is sanitized [a-z0-9-] by the runner and
-- contains no secrets. fencing_token is monotonically increasing and is the
-- authoritative ownership proof: a holder whose token is advanced by a newer
-- acquirer can no longer renew or release.
CREATE TABLE IF NOT EXISTS ingest_lease (
    lease_key TEXT PRIMARY KEY,
    holder_id TEXT NOT NULL,
    fencing_token INTEGER NOT NULL,
    lease_until TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- (2) Rebuild ingest_runs with the extended status CHECK.
--     The new column set is byte-for-byte the old one plus the wider CHECK;
--     INSERT OR IGNORE ... SELECT carries over every row unchanged.
CREATE TABLE IF NOT EXISTS ingest_runs_new (
    run_id TEXT PRIMARY KEY CHECK (length(run_id) BETWEEN 1 AND 64),
    origin TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL,
    finished_at TEXT,
    status TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'ok', 'partial', 'failed', 'skipped_locked', 'skipped_fresh')),
    sources_checked INTEGER NOT NULL DEFAULT 0,
    sources_succeeded INTEGER NOT NULL DEFAULT 0,
    entries_fetched INTEGER NOT NULL DEFAULT 0,
    stories_created INTEGER NOT NULL DEFAULT 0,
    stories_updated INTEGER NOT NULL DEFAULT 0,
    error_summary TEXT NOT NULL DEFAULT ''
);

INSERT OR IGNORE INTO ingest_runs_new (
    run_id, origin, started_at, finished_at, status,
    sources_checked, sources_succeeded, entries_fetched,
    stories_created, stories_updated, error_summary
)
SELECT
    run_id, origin, started_at, finished_at, status,
    sources_checked, sources_succeeded, entries_fetched,
    stories_created, stories_updated, error_summary
FROM ingest_runs;

DROP TABLE ingest_runs;
ALTER TABLE ingest_runs_new RENAME TO ingest_runs;

-- Recreate the index on the rebuilt table (CREATE INDEX IF NOT EXISTS is safe:
-- the index name is unique and the old index was dropped with the old table).
CREATE INDEX IF NOT EXISTS idx_ingest_runs_finished ON ingest_runs(finished_at DESC, status);
