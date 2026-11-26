-- 011_deployed_manifests.sql (issue #247 / PUB-01)
--
-- Durable, single-row store for the deployed-manifest state that issue #229
-- used to keep in the tracked file web/data/deployed.json. Issue #247 moves
-- that generated state out of Git: the manifest now lives in the shared
-- database so the publish gate can compare a freshly exported candidate
-- against the last DEPLOYED snapshot on every fresh (repo-less or clean)
-- checkout without any committed file.
--
-- The row is non-secret by construction (version, RFC3339 timestamps, a
-- content SHA-256, a commit SHA, and integer counts) — identical to the JSON
-- manifest the gate already consumes. Exactly one row exists (the latest
-- deployment); `put` upserts on the constant key '1'.
--
-- Forward-only, additive, Turso/libSQL-compatible (plain TEXT/INTEGER, no
-- CHECK that Turso rejects). The schema_migrations guard (version 11) runs
-- this exactly once per DB. Single transaction, no data loss.

PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS deployed_manifests (
    manifest_key TEXT PRIMARY KEY CHECK (manifest_key = '1'),
    version TEXT NOT NULL,
    deployed_at TEXT NOT NULL,
    generated_at TEXT NOT NULL,
    -- `commit` is a SQL reserved word (BEGIN/COMMIT); quote it everywhere.
    "commit" TEXT NOT NULL DEFAULT '',
    content_sha256 TEXT NOT NULL DEFAULT '',
    stories INTEGER NOT NULL DEFAULT 0,
    latest INTEGER NOT NULL DEFAULT 0
);
