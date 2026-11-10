-- HS-006: Add source registry columns to sources table
-- Forward-only migration; Turso/SQLite compatible.

-- Add priority column (lower = polled first)
ALTER TABLE sources ADD COLUMN priority INTEGER NOT NULL DEFAULT 0;

-- Add enabled flag to disable sources without deleting them
ALTER TABLE sources ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1;

-- Add language column for BCP-47 language tags (e.g., en, es)
ALTER TABLE sources ADD COLUMN language TEXT NOT NULL DEFAULT 'en';

-- Add timeout_seconds for per-source HTTP timeout overrides (0 = global default)
ALTER TABLE sources ADD COLUMN timeout_seconds INTEGER NOT NULL DEFAULT 0;

-- Add last_status for runtime polling status (not persisted across runs)
ALTER TABLE sources ADD COLUMN last_status TEXT NOT NULL DEFAULT '';

-- Add license column for known license or acceptable-use notes
ALTER TABLE sources ADD COLUMN license TEXT NOT NULL DEFAULT '';

-- Add acceptable_use for publisher acceptable-use posture
ALTER TABLE sources ADD COLUMN acceptable_use TEXT NOT NULL DEFAULT 'unknown';

-- Create indexes for common queries
CREATE INDEX IF NOT EXISTS idx_sources_priority ON sources(priority);
CREATE INDEX IF NOT EXISTS idx_sources_enabled ON sources(enabled);
CREATE INDEX IF NOT EXISTS idx_sources_enabled_priority ON sources(enabled, priority);

-- Note: last_status is runtime-only and will be cleared on application restart.
-- The sources table stores static configuration; last_status is tracked in memory by the application.