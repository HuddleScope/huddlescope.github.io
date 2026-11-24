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

// IngestRunRepository implements domain.IngestRunRepository for the Turso
// backend (lazy libsql wrapper, mirroring RetentionService, plan §4/§7/R7).
//
// Construction is network-free: buildDSN and sql.Open only parse the DSN.
// The first operation performs the lazy init (PRAGMA foreign_keys +
// migrations) and delegates the actual SQL to the driver-agnostic
// sqlite.IngestRunRepository, so the same queries run over modernc sqlite
// (tests) and libsql (production).
type IngestRunRepository struct {
	db       *sql.DB
	dsn      string
	initOnce sync.Once
	initErr  error
	inner    *sqlite.IngestRunRepository
}

// NewIngestRunRepository opens a Turso/libsql ingest-run repository. It does
// not Ping or query the database; the connection is established lazily on the
// first operation. databaseURL follows the same rules as turso.Open
// (libsql://host/path, optional authToken, or file:... for local testing).
func NewIngestRunRepository(databaseURL, authToken string) (*IngestRunRepository, error) {
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
	return &IngestRunRepository{db: db, dsn: dsn}, nil
}

// newIngestRunRepoFromDB wraps an existing *sql.DB handle (used by tests to
// prove the repository works over any database/sql driver; the production
// path is NewIngestRunRepository with the libsql driver).
func newIngestRunRepoFromDB(db *sql.DB, dsn string) *IngestRunRepository {
	return &IngestRunRepository{db: db, dsn: dsn}
}

// init performs the one-time setup: enable foreign keys (PRAGMA is
// per-connection; the modernc/libsql pool may hand out fresh connections)
// and apply migrations (idempotent). Errors are redacted: the DSN never
// appears in the returned error (RedactedDSN).
func (r *IngestRunRepository) init() error {
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
	r.inner = sqlite.NewIngestRunRepoFromDB(r.db)
	return nil
}

func (r *IngestRunRepository) wrapInitError(err error) error {
	msg := err.Error()
	if r.dsn != "" {
		msg = strings.ReplaceAll(msg, r.dsn, RedactedDSN(r.dsn))
	}
	return fmt.Errorf("initialize turso ingest runs: %s", msg)
}

// Create implements domain.IngestRunRepository.
func (r *IngestRunRepository) Create(ctx context.Context, run domain.IngestRun) error {
	if err := r.init(); err != nil {
		return err
	}
	return r.inner.Create(ctx, run)
}

// Finish implements domain.IngestRunRepository.
func (r *IngestRunRepository) Finish(ctx context.Context, run domain.IngestRun) error {
	if err := r.init(); err != nil {
		return err
	}
	return r.inner.Finish(ctx, run)
}

// LastSuccess implements domain.IngestRunRepository.
func (r *IngestRunRepository) LastSuccess(ctx context.Context) (*domain.IngestRun, bool, error) {
	if err := r.init(); err != nil {
		return nil, false, err
	}
	return r.inner.LastSuccess(ctx)
}

// LastAttempt implements domain.IngestRunRepository.
func (r *IngestRunRepository) LastAttempt(ctx context.Context) (*domain.IngestRun, bool, error) {
	if err := r.init(); err != nil {
		return nil, false, err
	}
	return r.inner.LastAttempt(ctx)
}

// Close closes the underlying database handle.
func (r *IngestRunRepository) Close() error {
	if r.db == nil {
		return nil
	}
	return r.db.Close()
}
