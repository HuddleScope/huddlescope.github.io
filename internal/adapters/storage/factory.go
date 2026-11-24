// Package storage provides repository factories.
package storage

import (
	"errors"
	"fmt"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage/sqlite"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage/turso"
	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// ErrLeaseUnavailable is returned by NewLeaseRepository for the memory backend
// (and any backend that cannot coordinate across hosts). The runner treats it
// as "proceed without a lease" and emits a warning — the per-host memory store
// cannot do distributed mutual exclusion, so local/dev use simply skips the
// lease (issue #160 / plan §3.3). Remote coordination never uses local
// files/flock; turso (and sqlite-file, for a single-host file share) backends
// DO provide the lease.
var ErrLeaseUnavailable = errors.New("lease unavailable: backend cannot coordinate across hosts")

// NewRepository returns a domain.Repository based on the backend option.
func NewRepository(cfg config.StorageConfig) (domain.Repository, error) {
	switch cfg.Backend {
	case "memory":
		return NewMemoryRepository(), nil
	case "sqlite-file":
		if cfg.Path == "" {
			cfg.Path = "data/huddlescope.db"
		}
		return sqlite.NewRepository(cfg.Path)
	case "turso":
		return turso.NewRepository(cfg.TursoURL, cfg.TursoToken)
	default:
		return nil, fmt.Errorf("unknown backend %q; use memory, sqlite-file, or turso", cfg.Backend)
	}
}

// NewIngestRunRepository returns a domain.IngestRunRepository for the given
// backend (issue #159 / HS-PORT-03). memory returns a per-host volatile
// store (freshness CLI over it must fail closed); sqlite-file opens the DB
// (applying migrations) and wraps the same handle; turso opens a lazy
// libsql connection (no network at construction).
func NewIngestRunRepository(cfg config.StorageConfig) (domain.IngestRunRepository, error) {
	switch cfg.Backend {
	case "memory":
		return NewMemoryIngestRunRepository(), nil
	case "sqlite-file":
		if cfg.Path == "" {
			cfg.Path = "data/huddlescope.db"
		}
		repo, err := sqlite.NewRepository(cfg.Path)
		if err != nil {
			return nil, err
		}
		return sqlite.NewIngestRunRepoFromDB(repo.DB()), nil
	case "turso":
		return turso.NewIngestRunRepository(cfg.TursoURL, cfg.TursoToken)
	default:
		return nil, fmt.Errorf("unknown backend %q; use memory, sqlite-file, or turso", cfg.Backend)
	}
}

// NewLeaseRepository returns a domain.LeaseRepository for the given backend
// (issue #160 / HS-PORT-04). turso opens a lazy libsql connection (no network
// at construction); sqlite-file opens the DB (applying migrations) and wraps
// the same handle; memory returns ErrLeaseUnavailable (a per-host volatile
// store cannot do cross-host coordination — the runner warns and proceeds).
func NewLeaseRepository(cfg config.StorageConfig) (domain.LeaseRepository, error) {
	switch cfg.Backend {
	case "memory":
		return nil, ErrLeaseUnavailable
	case "sqlite-file":
		if cfg.Path == "" {
			cfg.Path = "data/huddlescope.db"
		}
		repo, err := sqlite.NewRepository(cfg.Path)
		if err != nil {
			return nil, err
		}
		return sqlite.NewIngestLeaseRepoFromDB(repo.DB()), nil
	case "turso":
		return turso.NewIngestLeaseRepository(cfg.TursoURL, cfg.TursoToken)
	default:
		return nil, fmt.Errorf("unknown backend %q; use memory, sqlite-file, or turso", cfg.Backend)
	}
}

// NewRuleRepository returns a domain.CorrectionRuleRepository for the given
// backend. For memory it returns an in-memory rule store; for sqlite-file and
// turso it opens the DB (applying migrations) and wraps the same connection.
// It is decoupled from NewRepository so callers that only need rules do not
// have to construct a story repository (engine independence, AC-4).
func NewRuleRepository(cfg config.StorageConfig) (domain.CorrectionRuleRepository, error) {
	switch cfg.Backend {
	case "memory":
		return NewMemoryRuleRepository(), nil
	case "sqlite-file":
		if cfg.Path == "" {
			cfg.Path = "data/huddlescope.db"
		}
		repo, err := sqlite.NewRepository(cfg.Path)
		if err != nil {
			return nil, err
		}
		return sqlite.NewRuleRepository(repo.DB()), nil
	case "turso":
		repo, err := turso.NewRepository(cfg.TursoURL, cfg.TursoToken)
		if err != nil {
			return nil, err
		}
		r, ok := repo.(*turso.Repository)
		if !ok {
			return nil, fmt.Errorf("turso.NewRepository returned unexpected type")
		}
		return sqlite.NewRuleRepository(r.DB()), nil
	default:
		return nil, fmt.Errorf("unknown backend %q; use memory, sqlite-file, or turso", cfg.Backend)
	}
}
