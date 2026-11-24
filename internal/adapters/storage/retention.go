package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage/sqlite"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage/turso"
	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// NewRetentionService returns a domain.RetentionService for the configured
// storage backend (issue #120):
//
//   - sqlite-file: modernc handle with migrations applied at construction
//     (local file; safe to connect eagerly)
//   - turso: libsql handle; NO Ping at construction, first operation connects
//     (mirrors the lazy behavior of the Turso backend)
//   - memory: documented no-op service with a warning — retention packages
//     must be persisted, and the memory backend is volatile, so Export and
//     Import fail loudly while Apply is a safe no-op
//   - unknown: error
func NewRetentionService(cfg config.StorageConfig) (domain.RetentionService, error) {
	switch cfg.Backend {
	case "memory":
		log.Print("retention: memory backend configured; retention packages cannot be persisted (export/import will fail)")
		return memoryRetentionService{}, nil
	case "sqlite-file":
		path := cfg.Path
		if path == "" {
			path = "data/huddlescope.db"
		}
		db, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, fmt.Errorf("open retention db: %w", err)
		}
		if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("enable foreign keys: %w", err)
		}
		if _, err := sqlite.MigrateDB(db); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("migrate retention db: %w", err)
		}
		return sqlite.NewDBService(db, "sqlite-file"), nil
	case "turso":
		return turso.NewRetentionService(cfg.TursoURL, cfg.TursoToken)
	default:
		return nil, fmt.Errorf("unknown backend %q; use memory, sqlite-file, or turso", cfg.Backend)
	}
}

// memoryRetentionService is the documented no-op RetentionService for the
// volatile memory backend. Export and Import fail loudly (packages must be
// persisted; AC-2), Apply archives nothing (there is nothing durable to
// archive), and Status reports an empty state.
type memoryRetentionService struct{}

func (memoryRetentionService) ExportPackage(_ context.Context, _ string, _ domain.RetentionPolicy) (domain.PackageReport, error) {
	return domain.PackageReport{}, sqlite.ErrNotSupported
}

func (memoryRetentionService) Apply(_ context.Context, _ domain.RetentionPolicy) (domain.RetentionReport, error) {
	return domain.RetentionReport{}, nil
}

func (memoryRetentionService) Import(_ context.Context, _ string) (domain.PackageReport, error) {
	return domain.PackageReport{}, sqlite.ErrNotSupported
}

func (memoryRetentionService) Status(_ context.Context) (domain.RetentionStatus, error) {
	return domain.RetentionStatus{Backend: "memory", BudgetOK: true}, nil
}

func (memoryRetentionService) Close() error { return nil }
