package turso

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage/sqlite"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// IngestLeaseRepository implements domain.LeaseRepository for the Turso
// backend (lazy libsql wrapper, mirroring IngestRunRepository, plan §3.3).
//
// Construction is network-free: buildDSN and sql.Open only parse the DSN. The
// first operation performs the lazy init (PRAGMA foreign_keys + migrations) and
// delegates the actual SQL to the driver-agnostic sqlite.IngestLeaseRepository,
// so the same queries run over modernc sqlite (tests) and libsql (production).
type IngestLeaseRepository struct {
	db       *sql.DB
	dsn      string
	initOnce sync.Once
	initErr  error
	inner    *sqlite.IngestLeaseRepository
}

// NewIngestLeaseRepository opens a Turso/libsql lease repository. It does not
// Ping or query the database; the connection is established lazily on the first
// operation. databaseURL follows the same rules as NewIngestRunRepository.
func NewIngestLeaseRepository(databaseURL, authToken string) (*IngestLeaseRepository, error) {
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
	return &IngestLeaseRepository{db: db, dsn: dsn}, nil
}

// newIngestLeaseRepoFromDB wraps an existing *sql.DB handle (used by tests to
// prove the repository works over any database/sql driver; the production path
// is NewIngestLeaseRepository with the libsql driver).
func newIngestLeaseRepoFromDB(db *sql.DB, dsn string) *IngestLeaseRepository {
	return &IngestLeaseRepository{db: db, dsn: dsn}
}

// init performs the one-time setup: enable foreign keys and apply migrations
// (idempotent). Errors are redacted: the DSN never appears in the returned
// error (RedactedDSN).
func (r *IngestLeaseRepository) init() error {
	r.initOnce.Do(func() {
		if r.dsn == "" {
			// Test handle without a DSN: run migrations only.
			if _, err := sqlite.MigrateDB(r.db); err != nil {
				r.initErr = err
			}
			return
		}
		if _, err := r.db.Exec("PRAGMA foreign_keys = ON"); err != nil {
			r.initErr = r.wrapInitError(err)
			return
		}
		if _, err := sqlite.MigrateDB(r.db); err != nil {
			r.initErr = r.wrapInitError(err)
		}
	})
	if r.initErr != nil {
		return r.initErr
	}
	r.inner = sqlite.NewIngestLeaseRepoFromDB(r.db)
	return nil
}

func (r *IngestLeaseRepository) wrapInitError(err error) error {
	msg := err.Error()
	if r.dsn != "" {
		msg = strings.ReplaceAll(msg, r.dsn, RedactedDSN(r.dsn))
	}
	return fmt.Errorf("initialize turso ingest lease: %s", msg)
}

// Acquire implements domain.LeaseRepository.
func (r *IngestLeaseRepository) Acquire(ctx context.Context, holderID string, ttl time.Duration) (*domain.Lease, bool, error) {
	if err := r.init(); err != nil {
		return nil, false, err
	}
	return r.inner.Acquire(ctx, holderID, ttl)
}

// Renew implements domain.LeaseRepository.
func (r *IngestLeaseRepository) Renew(ctx context.Context, lease *domain.Lease, ttl time.Duration) (bool, error) {
	if err := r.init(); err != nil {
		return false, err
	}
	return r.inner.Renew(ctx, lease, ttl)
}

// Release implements domain.LeaseRepository.
func (r *IngestLeaseRepository) Release(ctx context.Context, lease *domain.Lease) error {
	if err := r.init(); err != nil {
		return err
	}
	return r.inner.Release(ctx, lease)
}

// Close closes the underlying database handle.
func (r *IngestLeaseRepository) Close() error {
	if r.db == nil {
		return nil
	}
	return r.db.Close()
}
