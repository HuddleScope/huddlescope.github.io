package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// LeaseKeyIngest is the single fixed lease key shared by every host that may
// run the ingest pipeline (issue #160 / HS-PORT-04).
const LeaseKeyIngest = "rss-ingest"

// IngestLeaseRepository implements domain.LeaseRepository over an existing
// *sql.DB handle (driver-agnostic: modernc sqlite locally, libsql for Turso).
// The caller owns the handle and its migrations.
//
// Design invariants (issue #160):
//   - Server-side time only. Every grant, expiry comparison, and renewal uses
//     SQL strftime(...,'now'), so a skewed client clock can never grant or
//     expire a lease (AC-4). The client-supplied TTL only supplies the
//     second-level offset.
//   - Atomic acquire via a single conditional upsert. One statement is
//     serialized by SQLite/Turso (server-side), avoiding multi-statement
//     transaction-consistency risk over the HTTP libsql driver.
//   - Fencing. The fencing token is monotonically incremented on each
//     successful (re)acquire. Renew and Release are both guarded on
//     (holder_id, fencing_token), so a lagged holder whose token was advanced
//     by a newer acquirer can neither renew nor release (AC-4).
//   - The caller (the runner) supplies the TTL on both Acquire and Renew, so
//     the repository never has to re-derive it.
type IngestLeaseRepository struct {
	db *sql.DB
}

// NewIngestLeaseRepoFromDB wraps an existing *sql.DB handle. It does not open
// or migrate the database (mirrors NewIngestRunRepoFromDB).
func NewIngestLeaseRepoFromDB(db *sql.DB) *IngestLeaseRepository {
	return &IngestLeaseRepository{db: db}
}

// dbNowPlus builds a server-side timestamp "now + n seconds" as the strftime
// modifier argument ("+<n> seconds"). Negative offsets are not used here.
func dbNowPlus(seconds int64) string {
	return fmt.Sprintf("+%d seconds", seconds)
}

// Acquire implements domain.LeaseRepository. It atomically grants the lease to
// holderID: a fresh grant (token 1) when the lease is absent, or a takeover
// (token + 1) when the existing lease has expired. When another holder's
// lease is still valid the upsert is a no-op and ok=false is returned (no
// error). The ok outcome is decided by the statement's changes() count:
// 1 means we won (insert or takeover), 0 means a valid lease is held.
func (r *IngestLeaseRepository) Acquire(ctx context.Context, holderID string, ttl time.Duration) (*domain.Lease, bool, error) {
	ctx, cancel := opCtx(ctx)
	defer cancel()

	ttlSec := int64(ttl.Seconds())

	// Single conditional upsert — server-serialized, no multi-statement txn:
	//   - no row                      -> insert a fresh grant (token 1)
	//   - row exists, expired         -> take it over (token + 1)
	//   - row exists, not expired     -> no-op (WHERE clause fails)
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO ingest_lease (lease_key, holder_id, fencing_token, lease_until, updated_at)
		VALUES (?, ?, 1,
		        strftime('%Y-%m-%dT%H:%M:%SZ','now', ?),
		        strftime('%Y-%m-%dT%H:%M:%SZ','now'))
		ON CONFLICT(lease_key) DO UPDATE SET
			holder_id     = excluded.holder_id,
			fencing_token = ingest_lease.fencing_token + 1,
			lease_until   = excluded.lease_until,
			updated_at    = excluded.updated_at
		WHERE ingest_lease.lease_until < strftime('%Y-%m-%dT%H:%M:%SZ','now')`,
		LeaseKeyIngest, holderID, dbNowPlus(ttlSec))
	if err != nil {
		return nil, false, fmt.Errorf("acquire lease %q: %w", LeaseKeyIngest, err)
	}
	// changes()==1 means we won (fresh insert or takeover of an expired lease);
	// changes()==0 means a valid lease is held and our upsert was a no-op. We
	// must NOT decide ok by reading back holder_id: that would be wrong when two
	// hosts share the same holder_id (e.g., repeated CI runs) — a failed upsert
	// still shows that holder on read-back.
	n, err := res.RowsAffected()
	if err != nil {
		return nil, false, fmt.Errorf("acquire lease %q (rows): %w", LeaseKeyIngest, err)
	}
	if n == 0 {
		// Lost: return the current holder (or nil) so the runner can report who
		// holds it. No error — a skip, not a fault.
		lease, rerr := r.read(ctx)
		if rerr != nil {
			return nil, false, rerr
		}
		return lease, false, nil
	}

	// Won: read back the committed token / lease_until / updated_at.
	lease, err := r.read(ctx)
	if err != nil {
		return nil, false, err
	}
	if lease == nil {
		// Should not happen after a winning write; synthesize from the request.
		lease = &domain.Lease{Key: LeaseKeyIngest, HolderID: holderID, FencingToken: 1, LeaseUntil: time.Now().UTC().Add(ttl), UpdatedAt: time.Now().UTC()}
	}
	return lease, true, nil
}

// Renew implements domain.LeaseRepository. It extends the lease iff
// (holder_id, fencing_token) still match — the token match IS the fencing
// check (a lagged holder whose token was advanced can never renew). Returns
// false (no error) when the lease was lost (re-acquired by another host) or
// expired. A successful renewal is the authoritative proof of ownership.
func (r *IngestLeaseRepository) Renew(ctx context.Context, lease *domain.Lease, ttl time.Duration) (bool, error) {
	ctx, cancel := opCtx(ctx)
	defer cancel()
	if lease == nil {
		return false, nil
	}
	ttlSec := int64(ttl.Seconds())

	res, err := r.db.ExecContext(ctx, `
		UPDATE ingest_lease
		SET lease_until = strftime('%Y-%m-%dT%H:%M:%SZ','now', ?),
		    updated_at  = strftime('%Y-%m-%dT%H:%M:%SZ','now')
		WHERE lease_key = ? AND holder_id = ? AND fencing_token = ?`,
		dbNowPlus(ttlSec), lease.Key, lease.HolderID, lease.FencingToken)
	if err != nil {
		return false, fmt.Errorf("renew lease %q: %w", lease.Key, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("renew lease %q (rows): %w", lease.Key, err)
	}
	return n > 0, nil
}

// Release implements domain.LeaseRepository. It deletes the row only if
// holder_id AND fencing_token still match — it never deletes another holder's
// lease. A missing row is not an error (idempotent release on exit).
func (r *IngestLeaseRepository) Release(ctx context.Context, lease *domain.Lease) error {
	ctx, cancel := opCtx(ctx)
	defer cancel()
	if lease == nil {
		return nil
	}
	if _, err := r.db.ExecContext(ctx, `
		DELETE FROM ingest_lease
		WHERE lease_key = ? AND holder_id = ? AND fencing_token = ?`,
		lease.Key, lease.HolderID, lease.FencingToken); err != nil {
		return fmt.Errorf("release lease %q: %w", lease.Key, err)
	}
	return nil
}

// read returns the current lease row for the fixed key, or (nil, nil) when
// there is none.
func (r *IngestLeaseRepository) read(ctx context.Context) (*domain.Lease, error) {
	var (
		holderID string
		token    int64
		until    string
		updated  string
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT holder_id, fencing_token, lease_until, updated_at
		FROM ingest_lease WHERE lease_key = ?`, LeaseKeyIngest).
		Scan(&holderID, &token, &until, &updated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read lease %q: %w", LeaseKeyIngest, err)
	}
	untilT, err := parseTime(until)
	if err != nil {
		return nil, fmt.Errorf("parse lease_until %q: %w", until, err)
	}
	updatedT, err := parseTime(updated)
	if err != nil {
		return nil, fmt.Errorf("parse lease updated_at %q: %w", updated, err)
	}
	return &domain.Lease{
		Key:          LeaseKeyIngest,
		HolderID:     holderID,
		FencingToken: token,
		LeaseUntil:   untilT,
		UpdatedAt:    updatedT,
	}, nil
}

// Close implements domain.LeaseRepository. The repository does not own the
// handle (provided by the caller); closing the shared handle would break
// sibling repositories, so only the reference is released (mirrors
// IngestRunRepository.Close).
func (r *IngestLeaseRepository) Close() error {
	return nil
}

// Ensure the struct satisfies the domain contract at compile time.
var _ domain.LeaseRepository = (*IngestLeaseRepository)(nil)
