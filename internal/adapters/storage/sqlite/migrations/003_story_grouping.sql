-- HS-010: Add cross-source story grouping columns
-- Run after migration 002 (source registry)

-- Add headline column to stories table (nullable for backward compatibility)
ALTER TABLE stories ADD COLUMN headline TEXT;

-- Add article_ids column to stories table (JSON array of article IDs, nullable)
ALTER TABLE stories ADD COLUMN article_ids TEXT;

-- Update index to include headline for duplicate detection
-- (original index: CREATE INDEX idx_stories_source_guid ON stories(source, guid);)
-- Keep existing index; headline is informational only for cross-source grouping