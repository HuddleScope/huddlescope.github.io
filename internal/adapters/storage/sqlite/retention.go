package sqlite

import (
	"fmt"
)

// RetentionWindow defines the number of days to keep ranking snapshots in the hot table.
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

// ArchiveOld moves rows older than maxAge from hot tables to archive schema.
// Returns the number of rows archived per table.
func ArchiveOld(dbPath string, maxAgeDays int) (map[string]int64, error) {
	// Placeholder for now.
	return nil, fmt.Errorf("not implemented")
}

// StorageEstimate returns the current database size estimate.
func StorageEstimate(dbPath string) (int64, error) {
	// Placeholder for now.
	return 0, fmt.Errorf("not implemented")
}

// ApplyRetention archives old data and checks storage budget.
// Returns true if budget is OK, false if exceeded.
func ApplyRetention(dbPath string) (bool, error) {
	// Placeholder for now.
	return true, fmt.Errorf("not implemented")
}
