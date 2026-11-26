-- 012_deployed_manifest_columns.sql (issue #250 / PUB-04)
--
-- Adds the v2 binding columns to the single-row deployed_manifests table
-- (migration 011) that issue #229/#247 used to record the last successful
-- deployment. Issue #250 binds the manifest to the exact validated artifact
-- and run: snapshotId (the content-derived site-snapshot ID, issue #249),
-- lastIngestAt, sourceCommit, runID, and an audited overrideReason for the
-- verified bootstrap override path.
--
-- Forward-only, additive, Turso/libSQL-compatible (plain TEXT columns with
-- '' defaults — same convention as 011, no CHECK that Turso rejects). Legacy
-- v1 rows become valid v2-readable rows with zero-value (empty) new fields.
-- The schema_migrations guard (version 12) runs this exactly once per DB.
-- Single transaction, no data loss, no data touched.

PRAGMA foreign_keys = ON;

ALTER TABLE deployed_manifests ADD COLUMN snapshot_id TEXT NOT NULL DEFAULT '';
ALTER TABLE deployed_manifests ADD COLUMN last_ingest_at TEXT NOT NULL DEFAULT '';
ALTER TABLE deployed_manifests ADD COLUMN source_commit TEXT NOT NULL DEFAULT '';
ALTER TABLE deployed_manifests ADD COLUMN run_id TEXT NOT NULL DEFAULT '';
ALTER TABLE deployed_manifests ADD COLUMN override_reason TEXT NOT NULL DEFAULT '';
