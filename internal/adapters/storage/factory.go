// Package storage provides repository factories.
package storage

import (
	"fmt"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage/sqlite"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// RepositoryOptions configures repository creation.
type RepositoryOptions struct {
	Backend string
	Path    string
}

// NewRepository returns a domain.Repository based on the backend option.
func NewRepository(opts RepositoryOptions) (domain.Repository, error) {
	switch opts.Backend {
	case "memory":
		return NewMemoryRepository(), nil
	case "sqlite-file":
		if opts.Path == "" {
			opts.Path = "data/huddlescope.db"
		}
		return sqlite.NewRepository(opts.Path)
	case "turso":
		// turso backend not yet implemented; fallback to memory with warning
		// The user's production deployment uses turso but currently runs on memory
		// (ingest/export hardcode NewMemoryRepository()). This preserves behavior.
		// TODO: implement turso driver when go-libsql is stable.
		return NewMemoryRepository(), fmt.Errorf("turso backend not yet implemented; using memory")
	default:
		return nil, fmt.Errorf("unknown backend %q; use memory, sqlite-file, or turso", opts.Backend)
	}
}