// Package turso: deployed-manifest store (issue #247 / PUB-01).
//
// The Turso/libsql manifest store wraps a shared *sql.DB handle and delegates
// to the driver-agnostic sqlite.DeployedManifest functions (same pattern as
// RetentionService and Repository).
//
// Construction is network-free: buildDSN and sql.Open only parse the DSN. The
// first operation performs the lazy init (PRAGMA foreign_keys + migrations),
// so configuration errors surface lazily, redacted, on first use.
package turso

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage/sqlite"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// DeployedManifestStore implements domain.DeployedManifestStore for the Turso
// backend.
type DeployedManifestStore struct {
	db       *sql.DB
	dsn      string
	initOnce sync.Once
	initErr  error
}

// NewDeployedManifestStore opens a Turso/libsql deployed-manifest store. It
// does not Ping or query the database; the connection is established on the
// first operation (lazy init applies migrations).
func NewDeployedManifestStore(databaseURL, authToken string) (*DeployedManifestStore, error) {
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
	return &DeployedManifestStore{db: db, dsn: dsn}, nil
}

// newDeployedManifestStoreFromDB wraps an existing *sql.DB handle (tests prove
// the store works over any database/sql driver; production uses libsql).
func newDeployedManifestStoreFromDB(db *sql.DB, dsn string) *DeployedManifestStore {
	return &DeployedManifestStore{db: db, dsn: dsn}
}

// init performs the one-time connection setup: enable foreign keys and apply
// migrations (idempotent). Errors are redacted: the DSN never appears in the
// returned error.
func (s *DeployedManifestStore) init() error {
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

func (s *DeployedManifestStore) wrapInitError(err error) error {
	msg := err.Error()
	if s.dsn != "" {
		msg = strings.ReplaceAll(msg, s.dsn, RedactedDSN(s.dsn))
	}
	return fmt.Errorf("initialize turso deployed manifest store: %s", msg)
}

// Get implements domain.DeployedManifestStore.
func (s *DeployedManifestStore) Get() (*domain.DeployedManifest, error) {
	if err := s.init(); err != nil {
		return nil, err
	}
	return sqlite.GetDeployedManifest(s.db)
}

// Put implements domain.DeployedManifestStore.
func (s *DeployedManifestStore) Put(m domain.DeployedManifest) error {
	if err := s.init(); err != nil {
		return err
	}
	return sqlite.PutDeployedManifest(s.db, m)
}

// Close closes the database handle.
func (s *DeployedManifestStore) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}
