package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// DBService implements domain.RetentionService on top of an open *sql.DB
// handle. It is driver-agnostic: the same implementation serves the modernc
// sqlite driver (sqlite-file backend) and, via the turso adapter, the libsql
// driver (Turso backend) (issue #120, plan §1).
type DBService struct {
	db          *sql.DB
	backend     string
	budgetBytes int64
}

// NewDBService wraps an existing *sql.DB handle. The handle is closed by
// Close. The caller is responsible for having applied migrations (MigrateDB)
// and, where FK cascade semantics matter, enabling PRAGMA foreign_keys.
func NewDBService(db *sql.DB, backend string) *DBService {
	return &DBService{
		db:          db,
		backend:     backend,
		budgetBytes: domain.DefaultStorageBudgetBytes,
	}
}

// ExportPackage implements domain.RetentionService. It never deletes rows; a
// no-op export (nothing expiring) returns Noop=true and writes no file.
func (s *DBService) ExportPackage(_ context.Context, outPath string, pol domain.RetentionPolicy) (domain.PackageReport, error) {
	rows, manifest, err := ExportPackageDB(s.db, outPath, pol.RetentionDays, pol.BatchLimit, pol.SourceBackend)
	if err != nil {
		return domain.PackageReport{}, err
	}
	total := int64(0)
	for _, n := range rows {
		total += n
	}
	report := domain.PackageReport{
		PackageID:     manifest.PackageID,
		GeneratedAt:   manifest.GeneratedAt,
		SchemaVersion: manifest.SchemaVersion,
		RetentionDays: manifest.RetentionDays,
		Rows:          rows,
		TotalRows:     total,
		Noop:          total == 0,
	}
	if !report.Noop {
		report.PackagePath = outPath
	}
	return report, nil
}

// Apply implements domain.RetentionService: archive expiring rows into the
// *_archive mirrors, then check the storage budget. Callers must have
// persisted the exported package first.
func (s *DBService) Apply(_ context.Context, pol domain.RetentionPolicy) (domain.RetentionReport, error) {
	if pol.FreeBytes > 0 {
		s.budgetBytes = pol.FreeBytes
	} else {
		s.budgetBytes = domain.DefaultStorageBudgetBytes
	}
	batchLimit := pol.BatchLimit
	if batchLimit <= 0 {
		batchLimit = domain.DefaultArchiveBatchLimit
	}
	days := pol.RetentionDays
	if days <= 0 {
		days = domain.DefaultRetentionDays
	}

	archived, err := ArchiveOldDB(s.db, days, batchLimit)
	if err != nil {
		return domain.RetentionReport{}, err
	}
	size, err := StorageEstimateDB(s.db)
	if err != nil {
		return domain.RetentionReport{}, err
	}
	total := int64(0)
	for _, n := range archived {
		total += n
	}
	return domain.RetentionReport{
		Archived:      archived,
		TotalArchived: total,
		DBSizeBytes:   size,
		BudgetOK:      size <= s.budgetBytes,
	}, nil
}

// Import implements domain.RetentionService: validate and restore a
// previously exported package into the hot tables (idempotent).
func (s *DBService) Import(_ context.Context, pkgPath string) (domain.PackageReport, error) {
	rows, manifest, err := ImportPackageDB(s.db, pkgPath)
	if err != nil {
		return domain.PackageReport{}, err
	}
	total := int64(0)
	for _, n := range rows {
		total += n
	}
	return domain.PackageReport{
		PackagePath:   pkgPath,
		PackageID:     manifest.PackageID,
		GeneratedAt:   manifest.GeneratedAt,
		SchemaVersion: manifest.SchemaVersion,
		RetentionDays: manifest.RetentionDays,
		Rows:          rows,
		TotalRows:     total,
	}, nil
}

// Status implements domain.RetentionService.
func (s *DBService) Status(_ context.Context) (domain.RetentionStatus, error) {
	size, err := StorageEstimateDB(s.db)
	if err != nil {
		return domain.RetentionStatus{}, err
	}
	ver, err := SchemaVersionDB(s.db)
	if err != nil {
		return domain.RetentionStatus{}, err
	}
	return domain.RetentionStatus{
		Backend:       s.backend,
		SchemaVersion: ver,
		DBSizeBytes:   size,
		BudgetOK:      size <= s.budgetBytes,
	}, nil
}

// Close closes the underlying database handle.
func (s *DBService) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// ErrNotSupported is returned by backends that cannot persist retention
// packages (e.g., the in-memory backend).
var ErrNotSupported = fmt.Errorf("retention is not supported on this backend")
