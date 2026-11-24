package turso

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// newTestIngestLeaseRepo wraps a local modernc sqlite handle in the Turso
// IngestLeaseRepository via the network-free test constructor. This proves the
// repository is driver-agnostic (works over any database/sql handle), while the
// production path uses the libsql driver (NewIngestLeaseRepository).
func newTestIngestLeaseRepo(t *testing.T) *IngestLeaseRepository {
	t.Helper()
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return newIngestLeaseRepoFromDB(db, "")
}

// TestTursoIngestLease_AcquireRelease (AC-1, driver-agnostic proof): the same
// queries that run over libsql in production run here over modernc sqlite.
func TestTursoIngestLease_AcquireRelease(t *testing.T) {
	repo := newTestIngestLeaseRepo(t)
	t.Cleanup(func() { _ = repo.Close() })
	ctx := context.Background()

	lease, ok, err := repo.Acquire(ctx, "vps:host-a", 20*time.Minute)
	if err != nil || !ok {
		t.Fatalf("first acquire: %v ok=%v", err, ok)
	}
	if lease.HolderID != "vps:host-a" || lease.FencingToken != 1 {
		t.Errorf("lease = %+v, want holder=vps:host-a token=1", lease)
	}
	// Renew (fenced) succeeds for the current holder.
	if ok, _ := repo.Renew(ctx, lease, 20*time.Minute); !ok {
		t.Error("renew of the current holder should succeed")
	}
	// Release (fenced) deletes the row; a second release is a no-op.
	if err := repo.Release(ctx, lease); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := repo.Release(ctx, lease); err != nil {
		t.Errorf("second release should be a no-op, got %v", err)
	}
	// After release, a new acquire resets the token to 1.
	if lease2, ok, _ := repo.Acquire(ctx, "vps:host-b", 20*time.Minute); !ok || lease2.FencingToken != 1 {
		t.Errorf("post-release acquire = %+v ok=%v, want token=1", lease2, ok)
	}
}

// TestTursoIngestLease_FencingLost (AC-4, driver-agnostic proof): a holder whose
// token was advanced by a newer acquirer can neither renew nor release.
func TestTursoIngestLease_FencingLost(t *testing.T) {
	repo := newTestIngestLeaseRepo(t)
	ctx := context.Background()

	leaseA, _, _ := repo.Acquire(ctx, "host-a", 20*time.Minute)
	// Expire host-a's lease by moving lease_until into the past, then host-b
	// takes it over (token 2).
	if _, err := repo.db.Exec(`UPDATE ingest_lease SET lease_until = strftime('%Y-%m-%dT%H:%M:%SZ','now','-1 hour')`); err != nil {
		t.Fatalf("expire: %v", err)
	}
	leaseB, _, err := repo.Acquire(ctx, "host-b", 20*time.Minute)
	if err != nil || leaseB.FencingToken != 2 {
		t.Fatalf("host-b acquire = %+v token=%d err=%v, want token=2", leaseB, leaseB.FencingToken, err)
	}
	// host-a's stale token: renew false, release no-op (does not delete host-b).
	if ok, _ := repo.Renew(ctx, leaseA, 20*time.Minute); ok {
		t.Error("stale host-a renew must be false")
	}
	if err := repo.Release(ctx, leaseA); err != nil {
		t.Errorf("stale host-a release should be a no-op, got %v", err)
	}
	// host-b still holds it (its lease is not expired and its token matches).
	if ok, _ := repo.Renew(ctx, leaseB, 20*time.Minute); !ok {
		t.Error("host-b should still hold a renewable lease after host-a's stale release")
	}
}

// TestTursoIngestLease_InitErrorRedaction (AC-5): connection errors never leak
// the DSN auth token (mirrors the runs repo).
func TestTursoIngestLease_InitErrorRedaction(t *testing.T) {
	dsn := "libsql://db.turso.io/default?authToken=secret-token-456"
	repo := newIngestLeaseRepoFromDB(nil, dsn)
	err := repo.wrapInitError(&leaseRedactProbeErr{msg: "failed to connect to " + dsn})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "secret-token-456") {
		t.Errorf("error leaks token: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "authToken=***") {
		t.Errorf("error should contain redacted token: %q", err.Error())
	}
}

// leaseRedactProbeErr is a dummy error whose message embeds the DSN.
type leaseRedactProbeErr struct{ msg string }

func (e *leaseRedactProbeErr) Error() string { return e.msg }

// TestTursoIngestLease_LazyOpen: sql.Open is lazy; construction is network-free.
func TestTursoIngestLease_LazyOpen(t *testing.T) {
	repo, err := NewIngestLeaseRepository("libsql://example.invalid/db", "")
	if err != nil {
		t.Fatalf("construction should not touch the network: %v", err)
	}
	defer func() { _ = repo.Close() }() //nolint:errcheck
}

// TestTursoIngestLease_EmptyURL: an empty URL is a config error.
func TestTursoIngestLease_EmptyURL(t *testing.T) {
	if _, err := NewIngestLeaseRepository("", ""); err == nil {
		t.Fatal("expected error for empty database URL")
	}
}

// TestTursoIngestLease_InitDSNBranch: a real local handle with a non-empty DSN
// exercises init()'s PRAGMA foreign_keys + MigrateDB branch (no network).
func TestTursoIngestLease_InitDSNBranch(t *testing.T) {
	tmp := t.TempDir()
	dbPath := "file:" + filepath.Join(tmp, "init.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := newIngestLeaseRepoFromDB(db, dbPath)
	if err := repo.init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	if repo.inner == nil {
		t.Error("inner repository not set after init")
	}
}
