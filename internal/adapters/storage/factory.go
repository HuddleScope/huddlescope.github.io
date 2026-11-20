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
