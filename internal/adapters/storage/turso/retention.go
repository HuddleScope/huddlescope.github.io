package turso

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage/sqlite"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// RetentionService implements domain.RetentionService for the Turso backend.
// It wraps a libsql *sql.DB handle and delegates to the driver-agnostic
// DB-level functions in the sqlite package (same pattern as Repository,
// plan §1/§2).
//
// Construction is network-free: buildDSN and sql.Open only parse the DSN.
// The first operation performs the lazy init (PRAGMA foreign_keys +
// migrations), so configuration errors surface lazily, redacted, on first use.
type RetentionService struct {
	db       *sql.DB
	dsn      string
	initOnce sync.Once
	initErr  error
}

// NewRetentionService opens a Turso/libsql retention service. It does not
// Ping or query the database; the connection is established on the first
// operation. databaseURL follows the same rules as turso.Open
// (libsql://host/path, optional authToken, or file:... for local testing).
func NewRetentionService(databaseURL, authToken string) (*RetentionService, error) {
	if databaseURL == "" {
		return nil, &ConfigError{
			Msg:    "TURSO_DATABASE_URL is required",
			Solve:  "set TURSO_DATABASE_URL to a Turso database URL (e.g., libsql://org-db.turso.io)",
			Secret: false,
		}
	}
	dsn, err := buildDSN(databaseURL, authToken)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("libsql", dsn)
	if err != nil {
		return nil, wrapError(err, dsn)
	}
	return &RetentionService{db: db, dsn: dsn}, nil
}

// newRetentionServiceFromDB wraps an existing *sql.DB handle (used by tests
// to prove the service works over any database/sql driver; the production
// path is NewRetentionService with the libsql driver).
func newRetentionServiceFromDB(db *sql.DB, dsn string) *RetentionService {
	return &RetentionService{db: db, dsn: dsn}
}

// init performs the one-time connection setup: enable foreign keys (so
// archiving an article/story cascade-removes its link rows, plan §6.3) and
// apply migrations (idempotent). Errors are redacted: the DSN never appears
// in the returned error (RedactedDSN).
func (s *RetentionService) init() error {
	s.initOnce.Do(func() {
		if s.dsn == "" {
			// Test handle without a DSN: run migrations only.
			if _, err := sqlite.MigrateDB(s.db); err != nil {
				s.initErr = err
			}
			return
		}
		if _, err := s.db.Exec("PRAGMA foreign_keys = ON"); err != nil {
			s.initErr = s.wrapInitError(err)
			return
		}
		if _, err := sqlite.MigrateDB(s.db); err != nil {
			s.initErr = s.wrapInitError(err)
		}
	})
	return s.initErr
}

func (s *RetentionService) wrapInitError(err error) error {
	msg := err.Error()
	if s.dsn != "" {
		msg = strings.ReplaceAll(msg, s.dsn, RedactedDSN(s.dsn))
	}
	return fmt.Errorf("initialize turso retention: %s", msg)
}

// ExportPackage implements domain.RetentionService (never deletes).
func (s *RetentionService) ExportPackage(ctx context.Context, outPath string, pol domain.RetentionPolicy) (domain.PackageReport, error) {
	if err := s.init(); err != nil {
		return domain.PackageReport{}, err
	}
	rows, manifest, err := sqlite.ExportPackageDB(s.db, outPath, pol.RetentionDays, pol.BatchLimit, pol.SourceBackend)
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

// Apply implements domain.RetentionService (archive + budget check).
func (s *RetentionService) Apply(ctx context.Context, pol domain.RetentionPolicy) (domain.RetentionReport, error) {
	if err := s.init(); err != nil {
		return domain.RetentionReport{}, err
	}
	batchLimit := pol.BatchLimit
	if batchLimit <= 0 {
		batchLimit = domain.DefaultArchiveBatchLimit
	}
	days := pol.RetentionDays
	if days <= 0 {
		days = domain.DefaultRetentionDays
	}
	freeBytes := pol.FreeBytes
	if freeBytes <= 0 {
		freeBytes = domain.DefaultStorageBudgetBytes
	}

	archived, err := sqlite.ArchiveOldDB(s.db, days, batchLimit)
	if err != nil {
		return domain.RetentionReport{}, err
	}
	size, err := sqlite.StorageEstimateDB(s.db)
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
		BudgetOK:      size <= freeBytes,
	}, nil
}

// Import implements domain.RetentionService (idempotent restore).
func (s *RetentionService) Import(ctx context.Context, pkgPath string) (domain.PackageReport, error) {
	if err := s.init(); err != nil {
		return domain.PackageReport{}, err
	}
	rows, manifest, err := sqlite.ImportPackageDB(s.db, pkgPath)
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
func (s *RetentionService) Status(ctx context.Context) (domain.RetentionStatus, error) {
	if err := s.init(); err != nil {
		return domain.RetentionStatus{}, err
	}
	size, err := sqlite.StorageEstimateDB(s.db)
	if err != nil {
		return domain.RetentionStatus{}, err
	}
	ver, err := sqlite.SchemaVersionDB(s.db)
	if err != nil {
		return domain.RetentionStatus{}, err
	}
	return domain.RetentionStatus{
		Backend:       "turso",
		SchemaVersion: ver,
		DBSizeBytes:   size,
		BudgetOK:      size <= domain.DefaultStorageBudgetBytes,
	}, nil
}

// Close closes the database handle.
func (s *RetentionService) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}
