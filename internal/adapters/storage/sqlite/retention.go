package sqlite

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// RetentionWindow defines the number of days to keep rows in the hot tables.
const RetentionWindow = 90

// ArchiveBatchLimit defines the maximum rows to move in a single batch.
const ArchiveBatchLimit = 500

// StoragePolicy holds retention configuration.
type StoragePolicy struct {
	RetentionDays int
	BatchLimit    int
	FreeBytes     int64
}

func NewStoragePolicy() StoragePolicy {
	return StoragePolicy{
		RetentionDays: RetentionWindow,
		BatchLimit:    ArchiveBatchLimit,
		FreeBytes:     1_000_000_000, // 1 GB
	}
}

// SizeBudgetOK returns true if the current usage is within the free tier budget.
func (p StoragePolicy) SizeBudgetOK(usedBytes int64) bool {
	return usedBytes <= p.FreeBytes
}

// openRetentionDB opens a handle for a local sqlite database path and applies
// migrations. An empty path maps to a shared in-memory database.
func openRetentionDB(dbPath string) (*sql.DB, error) {
	if dbPath == "" {
		dbPath = "file::memory:?cache=shared"
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if _, err := MigrateDB(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

// ArchiveOld moves rows older than maxAge from hot tables to archive tables in
// bounded batches. Returns the number of rows archived per table.
//
// It is a thin wrapper around ArchiveOldDB for local sqlite file paths
// (HS-041); the DB-level function serves both drivers (issue #120).
func ArchiveOld(dbPath string, maxAgeDays int) (map[string]int64, error) {
	db, err := openRetentionDB(dbPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()

	return ArchiveOldDB(db, maxAgeDays, ArchiveBatchLimit)
}

// StorageEstimate returns the current database size in bytes.
func StorageEstimate(dbPath string) (int64, error) {
	db, err := openRetentionDB(dbPath)
	if err != nil {
		return 0, err
	}
	defer func() { _ = db.Close() }()

	return StorageEstimateDB(db)
}

// ApplyRetention archives old data and checks storage budget.
// Returns true if budget is OK, false if exceeded.
func ApplyRetention(dbPath string) (bool, error) {
	db, err := openRetentionDB(dbPath)
	if err != nil {
		return false, err
	}
	defer func() { _ = db.Close() }()

	return ApplyRetentionDB(db)
}
