package domain

import "context"

// Retention defaults (issue #120 / HS-065). They mirror the historical
// HS-041 constants in internal/adapters/storage/sqlite (RetentionWindow,
// ArchiveBatchLimit) and the 1 GB Turso free-tier budget.
const (
	// DefaultRetentionDays is the hot-table retention window in days.
	DefaultRetentionDays = 90
	// DefaultArchiveBatchLimit bounds rows moved per archival batch.
	DefaultArchiveBatchLimit = 500
	// DefaultStorageBudgetBytes is the free-tier storage budget (1 GB).
	DefaultStorageBudgetBytes = 1_000_000_000
)

// RetentionPolicy describes one retention run: the age window applied to the
// hot tables (articles, stories, ranking_snapshots), the archival batch limit,
// the storage budget checked after archiving, and the backend label recorded
// in exported packages.
type RetentionPolicy struct {
	RetentionDays int
	BatchLimit    int
	FreeBytes     int64
	SourceBackend string
}

// NewRetentionPolicy returns a RetentionPolicy with production defaults.
func NewRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		RetentionDays: DefaultRetentionDays,
		BatchLimit:    DefaultArchiveBatchLimit,
		FreeBytes:     DefaultStorageBudgetBytes,
	}
}

// SizeBudgetOK reports whether usedBytes is within the policy budget.
func (p RetentionPolicy) SizeBudgetOK(usedBytes int64) bool {
	return usedBytes <= p.FreeBytes
}

// PackageReport describes a retention package that was exported (export) or
// restored (import). For a no-op export (no rows expired) Noop is true and
// PackagePath is empty.
type PackageReport struct {
	PackagePath   string
	PackageID     string
	GeneratedAt   string
	SchemaVersion int
	RetentionDays int
	Rows          map[string]int64
	TotalRows     int64
	Noop          bool
}

// HasRows reports whether the report covers at least one row.
func (r PackageReport) HasRows() bool {
	return r.TotalRows > 0
}

// RetentionReport describes the outcome of applying retention: how many rows
// were archived per hot table and whether the post-archive size is within the
// storage budget.
type RetentionReport struct {
	Archived      map[string]int64
	TotalArchived int64
	DBSizeBytes   int64
	BudgetOK      bool
}

// RetentionStatus is a point-in-time snapshot of retention state for the
// configured backend.
type RetentionStatus struct {
	Backend       string
	Enabled       bool
	RetentionDays int
	SchemaVersion int
	DBSizeBytes   int64
	BudgetOK      bool
}

// RetentionService applies the retention policy on a storage backend.
//
// Contract (issue #120):
//   - ExportPackage writes rows older than the window to a portable package
//     and NEVER deletes anything. A failed export must abort any subsequent
//     Apply (the caller orders the steps; CI commits the package in between).
//   - Apply archives expiring rows into *_archive tables and checks the
//     storage budget. Callers must have persisted the package first.
//   - Import restores a previously exported package into the hot tables.
//     Import is idempotent (INSERT OR REPLACE with stable primary keys).
type RetentionService interface {
	ExportPackage(ctx context.Context, outPath string, pol RetentionPolicy) (PackageReport, error)
	Apply(ctx context.Context, pol RetentionPolicy) (RetentionReport, error)
	Import(ctx context.Context, pkgPath string) (PackageReport, error)
	Status(ctx context.Context) (RetentionStatus, error)
	Close() error
}
