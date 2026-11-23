// Package storage provides repository factories.
package storage

import (
	"fmt"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage/sqlite"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage/turso"
	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

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
