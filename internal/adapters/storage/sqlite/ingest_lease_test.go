package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// newLeaseTestDB opens a temp-file sqlite DB with all migrations applied and
// returns a driver-agnostic IngestLeaseRepository plus the raw handle.
func newLeaseTestDB(t *testing.T) (*IngestLeaseRepository, *sql.DB) {
	t.Helper()
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := MigrateDB(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewIngestLeaseRepoFromDB(db), db
}

// expireLeaseDB directly sets the stored lease_until into the past to
// deterministically simulate an expired lease (server-side 'now' is the only
// clock, so real-time sleep + second-granularity rounding would be flaky).
func expireLeaseDB(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`UPDATE ingest_lease SET lease_until = strftime('%Y-%m-%dT%H:%M:%SZ','now','-1 hour')`); err != nil {
		t.Fatalf("expire row: %v", err)
	}
}

// TestLeaseAcquireRelease (AC-1): a single host acquires, then releases; a
// second host can acquire after the release (token resets to 1).
func TestLeaseAcquireRelease(t *testing.T) {
	repo, _ := newLeaseTestDB(t)
	ctx := context.Background()

	lease, ok, err := repo.Acquire(ctx, "host-a", 20*time.Minute)
	if err != nil || !ok {
		t.Fatalf("first acquire: %v ok=%v", err, ok)
	}
	if lease.HolderID != "host-a" || lease.FencingToken != 1 {
		t.Errorf("first lease = %+v, want holder=host-a token=1", lease)
	}

	// Release (guarded by holder_id+fencing_token) deletes the row.
	if err := repo.Release(ctx, lease); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if got, _ := repo.read(ctx); got != nil {
		t.Errorf("lease should be gone after release, got %+v", got)
	}

	// After release, host-b acquires fresh with token reset to 1.
	leaseB, okB, errB := repo.Acquire(ctx, "host-b", 20*time.Minute)
	if errB != nil || !okB {
		t.Fatalf("host-b acquire after release: %v ok=%v", errB, okB)
	}
	if leaseB.HolderID != "host-b" || leaseB.FencingToken != 1 {
		t.Errorf("host-b lease = %+v, want holder=host-b token=1", leaseB)
	}
}

// TestLeaseAcquireNotExpired (AC-1): while host-a holds a valid lease, host-b
// cannot acquire (ok=false, no error), and host-a's lease is unchanged.
func TestLeaseAcquireNotExpired(t *testing.T) {
	repo, _ := newLeaseTestDB(t)
	ctx := context.Background()

	if _, ok, err := repo.Acquire(ctx, "host-a", 20*time.Minute); err != nil || !ok {
		t.Fatalf("host-a acquire: %v ok=%v", err, ok)
	}
	_, wonB, err := repo.Acquire(ctx, "host-b", 20*time.Minute)
	if err != nil {
		t.Fatalf("host-b acquire: %v", err)
	}
	if wonB {
		t.Error("host-b should not win while host-a holds a valid lease")
	}
	got, _ := repo.read(ctx)
	if got == nil || got.HolderID != "host-a" || got.FencingToken != 1 {
		t.Errorf("host-a lease should be intact, got %+v", got)
	}
}

// TestLeaseExpiryReacquire (AC-2): after the lease expires, another host
// acquires (token increments) WITHOUT any manual row deletion, and the dead
// holder's renew returns false.
func TestLeaseExpiryReacquire(t *testing.T) {
	repo, db := newLeaseTestDB(t)
	ctx := context.Background()

	leaseA, ok, err := repo.Acquire(ctx, "host-a", 20*time.Minute)
	if err != nil || !ok {
		t.Fatalf("host-a acquire: %v ok=%v", err, ok)
	}
	if leaseA.FencingToken != 1 {
		t.Errorf("host-a token = %d, want 1", leaseA.FencingToken)
	}

	// Immediately host-b cannot win (lease valid).
	if _, wonB, _ := repo.Acquire(ctx, "host-b", 20*time.Minute); wonB {
		t.Error("host-b should not win before expiry")
	}

	// Simulate expiry by moving lease_until into the past (server clock only).
	expireLeaseDB(t, db)

	// host-b now acquires the expired lease: token increments to 2. No row
	// deletion occurred — the conditional upsert just took over the stale row.
	leaseB, ok, err := repo.Acquire(ctx, "host-b", 20*time.Minute)
	if err != nil || !ok {
		t.Fatalf("host-b reacquire: %v ok=%v", err, ok)
	}
	if leaseB.HolderID != "host-b" || leaseB.FencingToken != 2 {
		t.Errorf("host-b lease = %+v, want holder=host-b token=2", leaseB)
	}

	// host-a's now-stale lease can no longer renew (token mismatch).
	if ok, _ := repo.Renew(ctx, leaseA, 20*time.Minute); ok {
		t.Error("host-a renew should return false after losing the lease")
	}
}

// TestLeaseRenewGuardedByToken (AC-4 fencing): a holder whose fencing token was
// advanced by a newer acquirer can neither renew nor release.
func TestLeaseRenewGuardedByToken(t *testing.T) {
	repo, db := newLeaseTestDB(t)
	ctx := context.Background()

	leaseA, _, _ := repo.Acquire(ctx, "host-a", 20*time.Minute)
	expireLeaseDB(t, db)
	leaseB, _, err := repo.Acquire(ctx, "host-b", 20*time.Minute)
	if err != nil {
		t.Fatalf("host-b acquire: %v", err)
	}
	if leaseB.FencingToken != 2 {
		t.Fatalf("host-b token = %d, want 2", leaseB.FencingToken)
	}

	// host-a's stale token: renew -> false, release -> no-op (does NOT delete
	// host-b's lease).
	if ok, _ := repo.Renew(ctx, leaseA, 20*time.Minute); ok {
		t.Error("stale host-a renew must be false")
	}
	if err := repo.Release(ctx, leaseA); err != nil {
		t.Errorf("stale host-a release should be a no-op, got %v", err)
	}
	got, _ := repo.read(ctx)
	if got == nil || got.HolderID != "host-b" {
		t.Errorf("host-b lease should survive host-a's release, got %+v", got)
	}
}

// TestLeaseRenewSuccess (AC-4): the current holder can renew; the row's
// lease_until advances past a past expiry, and the token is unchanged.
func TestLeaseRenewSuccess(t *testing.T) {
	repo, db := newLeaseTestDB(t)
	ctx := context.Background()

	lease, _, _ := repo.Acquire(ctx, "host-a", 20*time.Minute)
	tokenBefore := lease.FencingToken

	// Make the stored lease expired, then renew: the renewed lease_until must be
	// in the future (proving renew extends the grant) and the token unchanged.
	expireLeaseDB(t, db)
	ok, err := repo.Renew(ctx, lease, 20*time.Minute)
	if err != nil || !ok {
		t.Fatalf("Renew = %v %v, want true nil", ok, err)
	}
	got, _ := repo.read(ctx)
	if got == nil {
		t.Fatal("lease missing after renew")
	}
	if got.FencingToken != tokenBefore {
		t.Errorf("renew changed token: %d -> %d", tokenBefore, got.FencingToken)
	}
	if !got.LeaseUntil.After(time.Now().UTC().Add(19 * time.Minute)) {
		t.Errorf("renewed lease_until %v should be ~now+20m (future)", got.LeaseUntil)
	}
}

// TestLeaseServerSideTime (AC-4): grant uses SQL 'now' (LeaseUntil ~ now + ttl),
// not a hardcoded or client-supplied timestamp. The stored value must be within
// a sane window of the client wall clock.
func TestLeaseServerSideTime(t *testing.T) {
	repo, db := newLeaseTestDB(t)
	ctx := context.Background()

	clientNow := time.Now().UTC()
	_, ok, err := repo.Acquire(ctx, "host-a", 20*time.Minute)
	if err != nil || !ok {
		t.Fatalf("acquire: %v ok=%v", err, ok)
	}
	var until string
	if err := db.QueryRow(`SELECT lease_until FROM ingest_lease WHERE lease_key = 'rss-ingest'`).Scan(&until); err != nil {
		t.Fatalf("query lease_until: %v", err)
	}
	untilT, err := parseTime(until)
	if err != nil {
		t.Fatalf("parse lease_until: %v", err)
	}
	want := clientNow.Add(20 * time.Minute)
	diff := untilT.Sub(want)
	if diff < -2*time.Minute || diff > 2*time.Minute {
		t.Errorf("lease_until %v not ~ now+20m (%v), diff %v", untilT, want, diff)
	}
}

// TestLeaseConcurrentAcquire (AC-1, -race): many goroutines racing to acquire the
// same key; exactly one wins with token 1, all others get ok=false. A single-
// connection file DB serializes local writes (see TestIngestRunRepo_Race; Turso
// serializes server-side), so the conditional upsert is the coordination.
func TestLeaseConcurrentAcquire(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "race.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1) // serialize local writes (see TestIngestRunRepo_Race)
	if _, err := MigrateDB(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := NewIngestLeaseRepoFromDB(db)
	ctx := context.Background()

	const n = 16
	okCount := make(chan bool, n)
	for i := 0; i < n; i++ {
		go func() {
			_, ok, _ := repo.Acquire(ctx, "racer", 20*time.Minute)
			okCount <- ok
		}()
	}
	wins := 0
	for i := 0; i < n; i++ {
		if <-okCount {
			wins++
		}
	}
	if wins != 1 {
		t.Errorf("exactly one acquire should win (same holder), got %d", wins)
	}
}

// TestLeaseNilLeaseRenewRelease guards the nil-lease edge (renew false, release
// no-op) — defensive against a nil held after a degraded start.
func TestLeaseNilLease(t *testing.T) {
	repo, _ := newLeaseTestDB(t)
	ctx := context.Background()
	if ok, err := repo.Renew(ctx, nil, 20*time.Minute); err != nil || ok {
		t.Errorf("nil renew = %v %v, want false nil", ok, err)
	}
	if err := repo.Release(ctx, nil); err != nil {
		t.Errorf("nil release = %v, want nil", err)
	}
}

// TestLeaseCloseIsNoop documents that Close releases only the reference (the
// repo does not own the handle).
func TestLeaseCloseIsNoop(t *testing.T) {
	repo, _ := newLeaseTestDB(t)
	if err := repo.Close(); err != nil {
		t.Errorf("Close = %v, want nil", err)
	}
	// The DB is still usable after Close (repo does not own the handle).
	if _, err := repo.read(context.Background()); err != nil {
		t.Errorf("read after Close = %v, want nil", err)
	}
}
