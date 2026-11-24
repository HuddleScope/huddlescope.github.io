package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// newIngestRunsTestDB opens a temp-file sqlite DB with all migrations applied
// and returns a driver-agnostic IngestRunRepository plus the raw handle.
func newIngestRunsTestDB(t *testing.T) (*IngestRunRepository, *sql.DB) {
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
	return NewIngestRunRepoFromDB(db), db
}

func utcTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t.UTC()
}

func TestIngestRunCreateFinish(t *testing.T) {
	repo, _ := newIngestRunsTestDB(t)
	ctx := context.Background()

	started := utcTime("2026-01-01T10:00:00Z")
	run := domain.IngestRun{
		RunID: "run-1", Origin: "vps", StartedAt: started,
		Status: domain.IngestRunRunning,
	}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// After Create the row must be running with no finished_at.
	attempt, ok, err := repo.LastAttempt(ctx)
	if err != nil || !ok {
		t.Fatalf("LastAttempt: %v ok=%v", err, ok)
	}
	if attempt.Status != domain.IngestRunRunning || attempt.FinishedAt != nil {
		t.Errorf("attempt after create = %+v", attempt)
	}

	// No success yet (AC-2: abandoned running is not a success).
	if _, ok, _ := repo.LastSuccess(ctx); ok {
		t.Error("LastSuccess before finish should be empty")
	}

	finished := utcTime("2026-01-01T10:05:00Z")
	run.Status = domain.IngestRunOK
	run.FinishedAt = &finished
	run.SourcesChecked = 3
	run.SourcesSucceeded = 3
	run.EntriesFetched = 12
	run.StoriesCreated = 5
	run.StoriesUpdated = 2
	if err := repo.Finish(ctx, run); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	success, ok, err := repo.LastSuccess(ctx)
	if err != nil || !ok {
		t.Fatalf("LastSuccess: %v ok=%v", err, ok)
	}
	if success.Status != domain.IngestRunOK || !success.FinishedAt.Equal(finished) {
		t.Errorf("success = %+v", success)
	}
	if success.SourcesChecked != 3 || success.EntriesFetched != 12 {
		t.Errorf("counters = %+v", success)
	}
}

// TestIngestRunUTCRoundTrip (AC-1): stored timestamps are UTC RFC3339
// (20 chars, trailing Z) and round-trip equal.
func TestIngestRunUTCRoundTrip(t *testing.T) {
	repo, db := newIngestRunsTestDB(t)
	ctx := context.Background()

	// A non-UTC input time must be stored as UTC.
	local := time.Date(2026, 3, 15, 12, 30, 45, 0, time.FixedZone("EST", -5*3600))
	run := domain.IngestRun{RunID: "utc-1", Origin: "local", StartedAt: local}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}

	var stored string
	if err := db.QueryRow(`SELECT started_at FROM ingest_runs WHERE run_id = 'utc-1'`).Scan(&stored); err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(stored) != 20 || !strings.HasSuffix(stored, "Z") {
		t.Errorf("stored timestamp not UTC RFC3339: %q", stored)
	}
	if got, err := parseTime(stored); err != nil || !got.Equal(local.UTC()) {
		t.Errorf("round trip = %v %v, want %v", got, err, local.UTC())
	}

	// Round-trip through the repository API.
	attempt, ok, err := repo.LastAttempt(ctx)
	if err != nil || !ok {
		t.Fatalf("LastAttempt: %v", err)
	}
	if !attempt.StartedAt.Equal(local.UTC()) {
		t.Errorf("start = %v, want %v", attempt.StartedAt, local.UTC())
	}
}

// TestLastSuccessSemantics (AC-2): failed rows with finished_at are excluded;
// zero-entry ok rows are included; the most recent success wins.
func TestLastSuccessSemantics(t *testing.T) {
	repo, _ := newIngestRunsTestDB(t)
	ctx := context.Background()

	mk := func(id, status, finished string, entries int) domain.IngestRun {
		var fin *time.Time
		if finished != "" {
			f := utcTime(finished)
			fin = &f
		}
		st, err := domain.ParseIngestRunStatus(status)
		if err != nil {
			t.Fatalf("status %q: %v", status, err)
		}
		return domain.IngestRun{
			RunID: id, Origin: "test", StartedAt: utcTime(finished),
			Status: st, FinishedAt: fin, EntriesFetched: entries,
		}
	}

	// failed run (has finished_at) must NOT count.
	if err := repo.Finish(ctx, mk("r-fail", "failed", "2026-01-05T00:00:00Z", 0)); err != nil {
		t.Fatalf("finish failed run: %v", err)
	}
	// partial run counts.
	if err := repo.Finish(ctx, mk("r-part", "partial", "2026-01-04T00:00:00Z", 3)); err != nil {
		t.Fatalf("finish partial run: %v", err)
	}
	// ok run with zero new entries counts and is the most recent.
	if err := repo.Finish(ctx, mk("r-ok", "ok", "2026-01-06T00:00:00Z", 0)); err != nil {
		t.Fatalf("finish ok run: %v", err)
	}

	success, ok, err := repo.LastSuccess(ctx)
	if err != nil || !ok {
		t.Fatalf("LastSuccess: %v ok=%v", err, ok)
	}
	if success.RunID != "r-ok" {
		t.Errorf("last success = %q, want r-ok", success.RunID)
	}
}

// TestAbandonedRunningNotSuccess (AC-2): a running row (Create only) never
// counts as success, even if it has a started_at after the last real success.
func TestAbandonedRunningNotSuccess(t *testing.T) {
	repo, _ := newIngestRunsTestDB(t)
	ctx := context.Background()

	okFin := utcTime("2026-01-01T00:00:00Z")
	if err := repo.Finish(ctx, domain.IngestRun{
		RunID: "ok-1", Origin: "t", StartedAt: utcTime("2025-12-31T00:00:00Z"),
		Status: domain.IngestRunOK, FinishedAt: &okFin,
	}); err != nil {
		t.Fatalf("finish ok: %v", err)
	}
	// Abandoned running row started later.
	if err := repo.Create(ctx, domain.IngestRun{
		RunID: "run-2", Origin: "t", StartedAt: utcTime("2026-01-02T00:00:00Z"),
	}); err != nil {
		t.Fatalf("create running: %v", err)
	}

	success, ok, err := repo.LastSuccess(ctx)
	if err != nil || !ok {
		t.Fatalf("LastSuccess: %v", err)
	}
	if success.RunID != "ok-1" {
		t.Errorf("last success = %q, want ok-1", success.RunID)
	}
	// LastAttempt is the abandoned running row.
	attempt, ok, err := repo.LastAttempt(ctx)
	if err != nil || !ok {
		t.Fatalf("LastAttempt: %v", err)
	}
	if attempt.RunID != "run-2" || attempt.Status != domain.IngestRunRunning {
		t.Errorf("last attempt = %+v", attempt)
	}
}

// TestTwoConnectionsSameDecision (AC-1): two repository instances opened on
// the SAME file DB (simulated two hosts) make identical decisions.
func TestTwoConnectionsSameDecision(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "shared.db")

	open := func() *IngestRunRepository {
		t.Helper()
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })
		if _, err := MigrateDB(db); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		return NewIngestRunRepoFromDB(db)
	}

	hostA := open()
	hostB := open()
	ctx := context.Background()

	fin := utcTime("2026-01-01T10:00:00Z")
	seed := domain.IngestRun{
		RunID: "shared-1", Origin: "raspberry", StartedAt: utcTime("2026-01-01T09:00:00Z"),
		Status: domain.IngestRunOK, FinishedAt: &fin,
	}
	if err := hostA.Finish(ctx, seed); err != nil {
		t.Fatalf("hostA finish: %v", err)
	}

	a, okA, errA := hostA.LastSuccess(ctx)
	b, okB, errB := hostB.LastSuccess(ctx)
	if errA != nil || errB != nil || !okA || !okB {
		t.Fatalf("mismatch: A=%v/%v B=%v/%v", a, errA, b, errB)
	}
	if a.RunID != b.RunID || !a.FinishedAt.Equal(*b.FinishedAt) {
		t.Errorf("divergent reads: A=%+v B=%+v", a, b)
	}
}

// TestIngestRunRepo_Race (AC-8): concurrent Create/Finish incl. duplicate
// run_id, designed to run under -race. A busy_timeout is set so local
// modernc sqlite serializes concurrent writes instead of failing with
// SQLITE_BUSY (the Turso path handles concurrency server-side).
func TestIngestRunRepo_Race(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "race.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	// Serialize the local file DB to a single connection: modernc sqlite
	// (journal mode, no WAL) is single-writer, and a pool of connections
	// would otherwise race on the write lock (SQLITE_BUSY). This still
	// exercises concurrent goroutines calling the repository (the -race
	// detector checks the repo's shared state); the Turso path handles
	// real multi-host concurrency server-side.
	db.SetMaxOpenConns(1)
	if _, err := MigrateDB(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := NewIngestRunRepoFromDB(db)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := "race-" + strings.Repeat("x", n%3) // duplicates
			started := utcTime("2026-01-01T00:00:00Z")
			run := domain.IngestRun{RunID: id, Origin: "race", StartedAt: started}
			if err := repo.Create(ctx, run); err != nil {
				t.Errorf("Create %s: %v", id, err)
				return
			}
			fin := utcTime("2026-01-01T00:00:01Z")
			run.Status = domain.IngestRunOK
			run.FinishedAt = &fin
			if err := repo.Finish(ctx, run); err != nil {
				t.Errorf("Finish %s: %v", id, err)
			}
		}(i)
	}
	wg.Wait()

	// The repo must still answer consistently.
	if _, _, err := repo.LastAttempt(ctx); err != nil {
		t.Errorf("LastAttempt after race: %v", err)
	}
}

// TestIngestRunCreateIdempotent: duplicate Create for the same run_id does not
// error (INSERT OR REPLACE) and resets to running.
func TestIngestRunCreateIdempotent(t *testing.T) {
	repo, _ := newIngestRunsTestDB(t)
	ctx := context.Background()

	run := domain.IngestRun{RunID: "dup-1", Origin: "t", StartedAt: utcTime("2026-01-01T00:00:00Z")}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("duplicate Create: %v", err)
	}
	attempt, ok, err := repo.LastAttempt(ctx)
	if err != nil || !ok {
		t.Fatalf("LastAttempt: %v", err)
	}
	if attempt.Status != domain.IngestRunRunning {
		t.Errorf("after duplicate create status = %v, want running", attempt.Status)
	}
}
