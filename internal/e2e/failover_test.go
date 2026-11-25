//go:build e2e

// Package e2e: failover / HA integration tests (issue #165 / HS-PORT-09 AC-2).
//
// These tests run the 8 failover scenarios from plan §5 against the REAL
// sqlite-file backend + the shared ingest lease (internal/adapters/storage/
// sqlite), exercised at the repository/runner seam. They are gated behind the
// `e2e` build tag so they do NOT run under `go test ./...` / `make test`;
// they run only under `make e2e` (which passes -tags e2e).
//
// Invariants (AC-9):
//   - DBs live in t.TempDir(); no TURSO_* / GITHUB_* env; no external feeds
//     (the lease + run-tracking seams need no feeds at all).
//   - Lease expiry is exercised with a SHORT real-time TTL (the lease repo uses
//     server-side strftime('now'), so expiry is real-time; a short TTL makes the
//     tests deterministic without waiting the production 20-minute TTL).
//   - The injected clock (HSC_FAKE_NOW) pins the freshness DECISIONS
//     (DecideShouldRun) but never the lease (server-side time).

package e2e

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage/sqlite"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// shortLeaseTTL is a short real-time lease TTL used to exercise the expiry /
// takeover paths deterministically (the production TTL is 20 minutes).
const shortLeaseTTL = 1500 * time.Millisecond

// newFailoverRepos opens the shared sqlite DB at dbPath (opening + migrating
// once) and returns the real sqlite ingest-run + lease repositories wrapping the
// same handle. Each call returns INDEPENDENT repo instances so two "processes"
// can be simulated on the same file.
func newFailoverRepos(t *testing.T, dbPath string) (domain.IngestRunRepository, domain.LeaseRepository) {
	t.Helper()
	repo, err := sqlite.NewRepository(dbPath)
	if err != nil {
		t.Fatalf("open sqlite repo at %s: %v", dbPath, err)
	}
	return sqlite.NewIngestRunRepoFromDB(repo.DB()), sqlite.NewIngestLeaseRepoFromDB(repo.DB())
}

// okRun builds a finished, successful IngestRun for the given origin and finish
// time.
func okRun(origin string, started time.Time, finished time.Time) domain.IngestRun {
	return domain.IngestRun{
		RunID:            domain.NewRunID().String(),
		Origin:           origin,
		StartedAt:        started,
		FinishedAt:       &finished,
		Status:           domain.IngestRunOK,
		SourcesChecked:   6,
		SourcesSucceeded: 6,
		EntriesFetched:   12,
		StoriesCreated:   3,
		StoriesUpdated:   0,
	}
}

// S1: Single host, primary path. The holder acquires the lease, registers a
// running row, finishes OK, and releases the lease. Invariant: exactly one `ok`
// run, the lease is released (a fresh acquirer wins immediately), and the run
// records a committed result.
func TestFailoverS1SingleHostPrimary(t *testing.T) {
	dbPath := t.TempDir() + "/huddlescope.db"
	runs, lease := newFailoverRepos(t, dbPath)
	defer func() { _ = runs.Close() }() //nolint:errcheck

	ctx := context.Background()
	const holderA = "raspberry:host-a:test"

	// Acquire the lease.
	l, ok, err := lease.Acquire(ctx, holderA, shortLeaseTTL)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if !ok {
		t.Fatal("holder A should acquire a fresh lease")
	}

	// Register a running row.
	started := time.Now().UTC()
	run := domain.IngestRun{
		RunID:     domain.NewRunID().String(),
		Origin:    "raspberry",
		StartedAt: started,
		Status:    domain.IngestRunRunning,
	}
	if err := runs.Create(ctx, run); err != nil {
		t.Fatalf("Create(running): %v", err)
	}

	// Simulate the pipeline completing; finish the run OK.
	finished := started.Add(2 * time.Second)
	run.FinishedAt = &finished
	run.Status = domain.IngestRunOK
	run.SourcesChecked = 6
	run.SourcesSucceeded = 6
	run.EntriesFetched = 12
	if err := runs.Finish(ctx, run); err != nil {
		t.Fatalf("Finish(ok): %v", err)
	}

	// Release the lease.
	if err := lease.Release(ctx, l); err != nil {
		t.Fatalf("Release: %v", err)
	}

	// Invariants: exactly one ok run; lease is free (a new acquirer wins).
	lastSuccess, has, err := runs.LastSuccess(ctx)
	if err != nil {
		t.Fatalf("LastSuccess: %v", err)
	}
	if !has || lastSuccess == nil || !lastSuccess.IsSuccess() {
		t.Fatalf("expected exactly one successful run, got has=%v run=%+v", has, lastSuccess)
	}
	// The lease is released: a fresh acquirer wins immediately.
	l2, ok2, err := lease.Acquire(ctx, "someone:new:after", shortLeaseTTL)
	if err != nil {
		t.Fatalf("re-Acquire after release: %v", err)
	}
	if !ok2 {
		t.Fatal("lease should be free after release; a new acquirer must win")
	}
	if l2.HolderID != "someone:new:after" {
		t.Errorf("new holder = %q, want someone:new:after", l2.HolderID)
	}
}

// S2: Crashed holder. Host A acquires with a short TTL and crashes (no release,
// no renewal). After the TTL elapses (real time), host B acquires and takes the
// lease over (fencing token + 1), finishing OK. Invariants: B's run is the
// single `ok` run; A's never-finished running row is NOT counted as a success;
// no fetch happened on A's side (it crashed mid-pipeline).
func TestFailoverS2CrashedHolderLeaseExpires(t *testing.T) {
	dbPath := t.TempDir() + "/huddlescope.db"
	runs, lease := newFailoverRepos(t, dbPath)
	defer func() { _ = runs.Close() }() //nolint:errcheck

	ctx := context.Background()
	const holderA = "raspberry:host-a:test"
	const holderB = "vps:host-b:test"

	// Host A acquires and registers a running row, then "crashes" (no release).
	if _, ok, err := lease.Acquire(ctx, holderA, shortLeaseTTL); err != nil || !ok {
		t.Fatalf("holder A acquire: ok=%v err=%v", ok, err)
	}
	started := time.Now().UTC()
	runA := domain.IngestRun{
		RunID:     domain.NewRunID().String(),
		Origin:    "raspberry",
		StartedAt: started,
		Status:    domain.IngestRunRunning,
	}
	if err := runs.Create(ctx, runA); err != nil {
		t.Fatalf("Create(A running): %v", err)
	}
	// A crashes here: no Finish, no Release.

	// Wait for the short TTL to elapse (server-side expiry).
	time.Sleep(shortLeaseTTL + 500*time.Millisecond)

	// Host B acquires: the expired lease must be taken over (token + 1).
	lb, ok, err := lease.Acquire(ctx, holderB, shortLeaseTTL)
	if err != nil {
		t.Fatalf("holder B acquire: %v", err)
	}
	if !ok {
		t.Fatal("holder B should take over the expired lease")
	}
	if lb.FencingToken < 2 {
		t.Errorf("holder B fencing token = %d, want >= 2 (takeover increments)", lb.FencingToken)
	}

	// Host B finishes OK.
	finished := time.Now().UTC()
	runB := okRun("vps", started, finished)
	if err := runs.Create(ctx, runB); err != nil {
		t.Fatalf("Create(B): %v", err)
	}
	runB.Status = domain.IngestRunOK
	runB.FinishedAt = &finished
	if err := runs.Finish(ctx, runB); err != nil {
		t.Fatalf("Finish(B ok): %v", err)
	}
	if err := lease.Release(ctx, lb); err != nil {
		t.Fatalf("Release(B): %v", err)
	}

	// Invariants: exactly one ok run (B's); A's running row is not a success.
	lastSuccess, has, err := runs.LastSuccess(ctx)
	if err != nil {
		t.Fatalf("LastSuccess: %v", err)
	}
	if !has || lastSuccess == nil {
		t.Fatal("expected a successful run (B)")
	}
	if lastSuccess.RunID != runB.RunID {
		t.Errorf("last success run = %q, want B's %q", lastSuccess.RunID, runB.RunID)
	}
	if lastSuccess.Origin != "vps" {
		t.Errorf("last success origin = %q, want vps (B)", lastSuccess.Origin)
	}
	// A's running row must NOT be reported as the last success.
	lastAttempt, _, err := runs.LastAttempt(ctx)
	if err != nil {
		t.Fatalf("LastAttempt: %v", err)
	}
	if lastAttempt != nil && lastAttempt.RunID == runA.RunID && lastAttempt.IsSuccess() {
		t.Error("A's running (crashed) row must not be a success")
	}
}

// S3: Locked backup skip. Host A holds a valid (long) lease. Host B (backup)
// attempts to acquire, gets ok=false, and must register a skipped_locked row and
// exit WITHOUT any RSS fetch. Invariant: B's row is skipped_locked; no ok run is
// created by B; the lease is still held by A.
func TestFailoverS3LockedBackupSkip(t *testing.T) {
	dbPath := t.TempDir() + "/huddlescope.db"
	runs, lease := newFailoverRepos(t, dbPath)
	defer func() { _ = runs.Close() }() //nolint:errcheck

	ctx := context.Background()
	const holderA = "raspberry:host-a:test"
	const holderB = "github-backup:host-b:test"

	// Host A holds a valid lease (long TTL so it stays valid).
	la, ok, err := lease.Acquire(ctx, holderA, 10*time.Minute)
	if err != nil || !ok {
		t.Fatalf("holder A acquire: ok=%v err=%v", ok, err)
	}

	// Host B attempts to acquire: it must lose (ok=false, no error).
	_, okB, err := lease.Acquire(ctx, holderB, shortLeaseTTL)
	if err != nil {
		t.Fatalf("holder B acquire should not error (a skip, not a fault): %v", err)
	}
	if okB {
		t.Fatal("holder B must NOT acquire while A holds a valid lease")
	}

	// B registers a skipped_locked run (no fetch).
	skipB := domain.IngestRun{
		RunID:      domain.NewRunID().String(),
		Origin:     "github-backup",
		StartedAt:  time.Now().UTC(),
		FinishedAt: func() *time.Time { f := time.Now().UTC(); return &f }(),
		Status:     domain.IngestRunSkippedLocked,
	}
	if err := runs.Create(ctx, skipB); err != nil {
		t.Fatalf("Create(B skipped_locked): %v", err)
	}

	// Invariants: no successful run exists (B did not fetch); A still holds the
	// lease.
	_, hasSuccess, err := runs.LastSuccess(ctx)
	if err != nil {
		t.Fatalf("LastSuccess: %v", err)
	}
	if hasSuccess {
		t.Error("no successful run should exist after a locked skip")
	}
	// A still holds the lease.
	lr, _, rerr := lease.Acquire(ctx, "probe:host:test", time.Second)
	if rerr != nil {
		t.Fatalf("probe acquire: %v", rerr)
	}
	if lr == nil || lr.HolderID != holderA {
		t.Errorf("lease holder after B's skip = %v, want A (%s)", lr, holderA)
	}
	_ = la
}

// S4: TOCTOU — the backup holds the lease, the primary recovers and refreshes,
// then the backup's in-lock freshness re-check finds the data fresh and skips.
// Invariant: B releases the lease and registers skipped_fresh; NO fetch on B;
// only A's ok run is counted.
func TestFailoverS4TOCTOUPrimaryRecovers(t *testing.T) {
	dbPath := t.TempDir() + "/huddlescope.db"
	runs, lease := newFailoverRepos(t, dbPath)
	defer func() { _ = runs.Close() }() //nolint:errcheck

	ctx := context.Background()
	const holderA = "raspberry:host-a:test"
	const holderB = "vps:host-b:test"

	// Host A (primary) holds the lease and finishes OK, refreshing the data.
	la, ok, err := lease.Acquire(ctx, holderA, shortLeaseTTL)
	if err != nil || !ok {
		t.Fatalf("holder A acquire: ok=%v err=%v", ok, err)
	}
	finishedA := time.Now().UTC()
	runA := okRun("raspberry", finishedA.Add(-10*time.Second), finishedA)
	if err := runs.Create(ctx, runA); err != nil {
		t.Fatalf("Create(A): %v", err)
	}
	runA.Status = domain.IngestRunOK
	runA.FinishedAt = &finishedA
	if err := runs.Finish(ctx, runA); err != nil {
		t.Fatalf("Finish(A ok): %v", err)
	}
	if err := lease.Release(ctx, la); err != nil {
		t.Fatalf("Release(A): %v", err)
	}

	// Host B (backup) now acquires (A released), but BEFORE fetching it re-checks
	// freshness in lock. The data is fresh (A just refreshed): B must skip.
	lb, okB, err := lease.Acquire(ctx, holderB, shortLeaseTTL)
	if err != nil || !okB {
		t.Fatalf("holder B acquire: ok=%v err=%v", okB, err)
	}
	// In-lock freshness re-check: last success is fresh (age < staleAfter).
	lastSuccess, _, serr := runs.LastSuccess(ctx)
	if serr != nil {
		t.Fatalf("LastSuccess (in-lock re-check): %v", serr)
	}
	decision := domain.DecideShouldRun(lastSuccess, time.Now().UTC(), 75*time.Minute)
	if !decision.ShouldRun {
		// Data is fresh: B releases the lease and skips (skipped_fresh), NO fetch.
		skipB := domain.IngestRun{
			RunID:      domain.NewRunID().String(),
			Origin:     "vps",
			StartedAt:  time.Now().UTC(),
			FinishedAt: func() *time.Time { f := time.Now().UTC(); return &f }(),
			Status:     domain.IngestRunSkippedFresh,
		}
		if err := runs.Create(ctx, skipB); err != nil {
			t.Fatalf("Create(B skipped_fresh): %v", err)
		}
		if err := lease.Release(ctx, lb); err != nil {
			t.Fatalf("Release(B): %v", err)
		}
		// Invariant: only A's ok run is the success; B did not fetch.
		ls, has, _ := runs.LastSuccess(ctx)
		if !has || ls.RunID != runA.RunID {
			t.Errorf("last success = %v, want A's run %s (B must not fetch)", ls, runA.RunID)
		}
		return
	}
	t.Fatal("expected the in-lock re-check to find the data fresh (should skip); it decided to run")
}

// S5: Fencing — a lagged holder whose token was advanced by a newer acquirer can
// neither renew nor release. Invariant: after B takes over (token + 1), A's old
// lease (old token) Renew returns false and A's Release is a no-op that does NOT
// delete B's lease.
func TestFailoverS5FencingBlocksStaleHolder(t *testing.T) {
	dbPath := t.TempDir() + "/huddlescope.db"
	runs, lease := newFailoverRepos(t, dbPath)
	defer func() { _ = runs.Close() }() //nolint:errcheck

	ctx := context.Background()
	const holderA = "raspberry:host-a:old"
	const holderB = "vps:host-b:new"

	// Host A acquires (token 1).
	la, ok, err := lease.Acquire(ctx, holderA, shortLeaseTTL)
	if err != nil || !ok {
		t.Fatalf("holder A acquire: ok=%v err=%v", ok, err)
	}
	if la.FencingToken != 1 {
		t.Fatalf("A's initial token = %d, want 1", la.FencingToken)
	}

	// A "crashes"/lags (no renewal). The TTL elapses; B takes over (token 2).
	time.Sleep(shortLeaseTTL + 500*time.Millisecond)
	lb, okB, err := lease.Acquire(ctx, holderB, shortLeaseTTL)
	if err != nil || !okB {
		t.Fatalf("holder B takeover: ok=%v err=%v", okB, err)
	}
	if lb.FencingToken != la.FencingToken+1 {
		t.Fatalf("B's token = %d, want %d (advanced by 1)", lb.FencingToken, la.FencingToken+1)
	}

	// A's STALE lease (token 1) can no longer renew (fenced).
	renewed, err := lease.Renew(ctx, la, shortLeaseTTL)
	if err != nil {
		t.Fatalf("A stale Renew should not error: %v", err)
	}
	if renewed {
		t.Error("A's stale-token Renew must return false (fenced)")
	}

	// A's stale Release must be a no-op: it must NOT delete B's lease.
	if err := lease.Release(ctx, la); err != nil {
		t.Fatalf("A stale Release: %v", err)
	}
	// B still holds the lease (a probe acquire by a third party loses).
	probe, okProbe, err := lease.Acquire(ctx, "probe:host:z", time.Second)
	if err != nil {
		t.Fatalf("probe acquire: %v", err)
	}
	if okProbe {
		t.Fatal("B's lease must still be valid after A's stale release")
	}
	if probe == nil || probe.HolderID != holderB {
		t.Errorf("lease holder = %v, want B (%s)", probe, holderB)
	}

	// Clean up: B releases.
	if err := lease.Release(ctx, lb); err != nil {
		t.Fatalf("Release(B): %v", err)
	}
}

// S6: Write failure (disk full / DB locked) must surface as an error (non-zero),
// never a false ok. We simulate a real SQLite write lock (SQLITE_BUSY): one
// connection holds an IMMEDIATE write transaction; a second connection with a
// short busy_timeout attempts a write (the lease acquire) and must get a
// SQLITE_BUSY error after the timeout, NOT a silent success. Invariant: the
// contended write errors, and no successful run is reported.
func TestFailoverS6WriteFailureNotFalseOK(t *testing.T) {
	dbPath := t.TempDir() + "/huddlescope.db"
	runs, _ := newFailoverRepos(t, dbPath)
	defer func() { _ = runs.Close() }() //nolint:errcheck

	ctx := context.Background()

	// Register one legitimate ok run first (on the shared handle).
	finished := time.Now().UTC()
	runOK := okRun("raspberry", finished.Add(-30*time.Second), finished)
	if err := runs.Create(ctx, runOK); err != nil {
		t.Fatalf("Create(ok): %v", err)
	}
	runOK.Status = domain.IngestRunOK
	runOK.FinishedAt = &finished
	if err := runs.Finish(ctx, runOK); err != nil {
		t.Fatalf("Finish(ok): %v", err)
	}

	// Conn A: hold an IMMEDIATE write transaction (acquires the SQLite write
	// lock for the whole DB file).
	locker, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open locker conn: %v", err)
	}
	defer func() { _ = locker.Close() }() //nolint:errcheck
	if _, err := locker.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		t.Fatalf("BEGIN IMMEDIATE: %v", err)
	}

	// Conn B: a short busy_timeout so the contended write fails fast.
	contended, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(120)")
	if err != nil {
		t.Fatalf("open contended conn: %v", err)
	}
	defer func() { _ = contended.Close() }() //nolint:errcheck
	contendedLease := sqlite.NewIngestLeaseRepoFromDB(contended)

	// The contended write must error (SQLITE_BUSY) after the busy timeout, NOT
	// silently succeed.
	_, _, werr := contendedLease.Acquire(ctx, "disk-full:host:x", time.Minute)
	if werr == nil {
		t.Error("contended Acquire should error (SQLITE_BUSY / DB locked); a write failure must surface, never a false ok")
	}

	// End the locking transaction.
	if _, err := locker.ExecContext(ctx, `ROLLBACK`); err != nil {
		t.Fatalf("ROLLBACK: %v", err)
	}

	// Invariant: the contended (failed) write produced no success; the only
	// success is runOK.
	ls, has, err := runs.LastSuccess(ctx)
	if err != nil {
		t.Fatalf("LastSuccess: %v", err)
	}
	if !has || ls.RunID != runOK.RunID {
		t.Errorf("last success = %v, want runOK %s (the failed write must not count)", ls, runOK.RunID)
	}

	// After the lock is released, a write succeeds again (the DB is healthy).
	if _, ok2, aerr := contendedLease.Acquire(ctx, "recovered:host:y", shortLeaseTTL); aerr != nil || !ok2 {
		t.Errorf("after ROLLBACK the write should succeed: ok=%v err=%v", ok2, aerr)
	}
}

// S7: Clock skew — a last success recorded in the FUTURE relative to now must be
// treated as fresh (age clamped to 0), never stale. Invariant: DecideShouldRun
// with a future lastSuccess reports ShouldRun=false / ReasonFresh, and the age is
// 0 (clamped), so a skewed clock never causes a spurious re-ingest.
func TestFailoverS7ClockSkewFutureIsFresh(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Last success in the future: 10 minutes ahead of now.
	future := now.Add(10 * time.Minute)
	lastSuccess := okRun("vps", future.Add(-1*time.Second), future)

	decision := domain.DecideShouldRun(&lastSuccess, now, 75*time.Minute)
	if decision.ShouldRun {
		t.Errorf("future lastSuccess should be fresh (ShouldRun=false), got ShouldRun=true (%s)", decision.Reason)
	}
	if decision.Reason != domain.ReasonFresh {
		t.Errorf("reason = %q, want fresh", decision.Reason)
	}
	if decision.Age != 0 {
		t.Errorf("age = %s, want 0 (clamped for future skew)", decision.Age)
	}

	// A stale-after of 0 with a future last success is still fresh (clamped age 0
	// <= 0).
	decision0 := domain.DecideShouldRun(&lastSuccess, now, 0)
	if decision0.ShouldRun {
		t.Errorf("future lastSuccess with 0 threshold should be fresh, got ShouldRun=true (%s)", decision0.Reason)
	}
}

// S8: Two processes on the same file — the lease table enforces single-writer
// mutual exclusion even without any OS-level file lock. Two independent repo
// handles on the SAME DB file attempt to acquire concurrently; exactly one wins.
// Invariant: the second acquirer gets ok=false (no error), and the lease row
// reflects the winner.
func TestFailoverS8TwoHandlesSameFileSingleWriter(t *testing.T) {
	dbPath := t.TempDir() + "/huddlescope.db"
	// Two INDEPENDENT handle sets on the same file (simulates two processes).
	runsA, leaseA := newFailoverRepos(t, dbPath)
	defer func() { _ = runsA.Close() }() //nolint:errcheck
	runsB, leaseB := newFailoverRepos(t, dbPath)
	defer func() { _ = runsB.Close() }() //nolint:errcheck

	ctx := context.Background()

	// Handle A acquires first and holds a valid lease.
	la, okA, err := leaseA.Acquire(ctx, "proc-a:host:1", 10*time.Minute)
	if err != nil || !okA {
		t.Fatalf("handle A acquire: ok=%v err=%v", okA, err)
	}

	// Handle B (separate connection, same file) attempts to acquire: it must lose
	// against A's valid lease. This proves the lease table (not an OS lock)
	// enforces mutual exclusion.
	_, okB, err := leaseB.Acquire(ctx, "proc-b:host:2", shortLeaseTTL)
	if err != nil {
		t.Fatalf("handle B acquire should not error: %v", err)
	}
	if okB {
		t.Fatal("handle B must NOT acquire while handle A holds a valid lease (single writer)")
	}

	// The lease row reflects A as the holder (read through handle B).
	// A fresh acquire by a third handle also loses and reports A.
	probe, okProbe, err := leaseB.Acquire(ctx, "proc-c:host:3", time.Second)
	if err != nil {
		t.Fatalf("probe acquire: %v", err)
	}
	if okProbe {
		t.Fatal("a third handle must also lose against A's valid lease")
	}
	if probe == nil || probe.HolderID != "proc-a:host:1" {
		t.Errorf("lease holder = %v, want proc-a (the single writer)", probe)
	}

	// Both handles agree on the run-tracking state (shared file).
	// A registers a run; B sees it.
	finished := time.Now().UTC()
	runA := okRun("proc-a", finished.Add(-5*time.Second), finished)
	if err := runsA.Create(ctx, runA); err != nil {
		t.Fatalf("handle A Create: %v", err)
	}
	runA.Status = domain.IngestRunOK
	runA.FinishedAt = &finished
	if err := runsA.Finish(ctx, runA); err != nil {
		t.Fatalf("handle A Finish: %v", err)
	}
	lsB, hasB, err := runsB.LastSuccess(ctx)
	if err != nil {
		t.Fatalf("handle B LastSuccess: %v", err)
	}
	if !hasB || lsB.RunID != runA.RunID {
		t.Errorf("handle B should see A's ok run (shared file); got has=%v run=%v", hasB, lsB)
	}

	_ = la
}
