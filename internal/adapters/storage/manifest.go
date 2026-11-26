// Package storage: deployed-manifest store factory (issue #247 / PUB-01).
//
// NewDeployedManifestStore returns a domain.DeployedManifestStore for the
// configured backend:
//
//   - turso: lazy libsql handle; first operation applies migrations
//     (production deploy path; the only backend that coordinates across hosts)
//   - sqlite-file: modernc handle with migrations applied at construction
//     (local file; durable on a single host, used for local seeding/tests)
//   - memory: error (fail closed). The memory backend is volatile and cannot
//     durably hold deploy state; a manifest that vanishes on restart would let
//     the gate mis-evaluate, so we refuse rather than pretend it is durable.
//   - unknown: error
package storage

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage/sqlite"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage/turso"
	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// NewDeployedManifestStore returns a domain.DeployedManifestStore for the
// configured storage backend (see package doc for the per-backend contract).
func NewDeployedManifestStore(cfg config.StorageConfig) (domain.DeployedManifestStore, error) {
	switch cfg.Backend {
	case "memory":
		return nil, fmt.Errorf("deployed manifest store: memory backend cannot durably store deploy state (fail closed); use turso or sqlite-file")
	case "sqlite-file":
		path := cfg.Path
		if path == "" {
			path = "data/huddlescope.db"
		}
		db, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, fmt.Errorf("open manifest db: %w", err)
		}
		if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("enable foreign keys: %w", err)
		}
		if _, err := sqlite.MigrateDB(db); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("migrate manifest db: %w", err)
		}
		return sqliteManifestStore{db: db}, nil
	case "turso":
		return turso.NewDeployedManifestStore(cfg.TursoURL, cfg.TursoToken)
	default:
		return nil, fmt.Errorf("unknown backend %q; use memory, sqlite-file, or turso", cfg.Backend)
	}
}

// sqliteManifestStore wraps a *sql.DB handle for the sqlite-file backend.
type sqliteManifestStore struct {
	db *sql.DB
}

func (s sqliteManifestStore) Get() (*domain.DeployedManifest, error) {
	return sqlite.GetDeployedManifest(s.db)
}

func (s sqliteManifestStore) Put(m domain.DeployedManifest) error {
	return sqlite.PutDeployedManifest(s.db, m)
}

func (s sqliteManifestStore) Close() error { return s.db.Close() }
