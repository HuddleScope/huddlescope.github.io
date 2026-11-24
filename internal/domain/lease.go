package domain

import (
	"context"
	"time"
)

// Lease is a distributed, time-bounded mutual-exclusion grant. The design is
// deliberately single-purpose (one fixed key, "rss-ingest", shared by every
// host) so the mechanism stays small and auditable (issue #160 / HS-PORT-04).
//
// Fields are the persistence contract (ingest_lease columns). All timestamps
// are UTC; lease_until is computed server-side (SQL 'now') in every adapter so
// a skewed client clock can never grant or expire a lease (AC-4).
type Lease struct {
	Key          string
	HolderID     string
	FencingToken int64
	LeaseUntil   time.Time
	UpdatedAt    time.Time
}

// LeaseRepository is the persistence contract for the ingest lease.
// Implementations: sqlite (driver-agnostic over *sql.DB), turso (lazy libsql
// wrapper). The memory backend cannot coordinate across hosts and therefore
// has no lease repository (the factory returns ErrLeaseUnavailable).
//
// Contract (fail closed):
//   - Acquire returns ok=false (no error) only when another host holds a
//     non-expired lease. Any database failure returns a non-nil error; callers
//     must treat that as a hard failure, never as "proceed".
//   - Renew returns false (no error) when the row no longer belongs to the
//     caller (holder or fencing-token mismatch) or has expired. A successful
//     Renew is the authoritative proof of continued ownership (fenced).
//   - Release deletes the row only if holder_id AND fencing_token still match;
//     it never deletes another holder's lease. A missing row is not an error.
type LeaseRepository interface {
	// Acquire atomically grants the lease to holderID with a TTL. It succeeds
	// when the lease is absent or expired; it fails (ok=false) when another
	// holder's lease is still valid. On success the fencing token is
	// monotonically incremented so a just-deposed holder's stale token can no
	// longer renew or release (fencing, AC-4).
	Acquire(ctx context.Context, holderID string, ttl time.Duration) (*Lease, bool, error)
	// Renew extends a lease the caller still holds. Returns false (not error)
	// when the lease was lost or expired. The caller must stop all work when
	// Renew returns false. ttl is the same value the holder used on Acquire.
	Renew(ctx context.Context, lease *Lease, ttl time.Duration) (bool, error)
	// Release relinquishes the lease. It is a no-op (no error) when the row no
	// longer exists or the fencing token no longer matches.
	Release(ctx context.Context, lease *Lease) error
	Close() error
}
