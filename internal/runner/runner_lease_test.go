package runner

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/publish"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/rss"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage"
	"github.com/HuddleScope/huddlescope.github.io/internal/application"
	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// ---------------------------------------------------------------------------
// Fakes for the distributed-lease orchestration tests (issue #160 / HS-PORT-04).
// ---------------------------------------------------------------------------

const testLeaseKey = "rss-ingest"

// fakeLeaseRepo implements domain.LeaseRepository with in-memory state so the
// runner's lease lifecycle (acquire/skip/release/lost) can be exercised without
// a real database.
type fakeLeaseRepo struct {
	mu        sync.Mutex
	state     func() (holder string, token int64, until time.Time)
	mutate    func(holder string, token int64, until time.Time)
	AcquireFn func(holderID string, ttl time.Duration) (*domain.Lease, bool, error)
	RenewFn   func(lease *domain.Lease, ttl time.Duration) (bool, error)
	ReleaseFn func(lease *domain.Lease) error

	acquireCalls int
	renewCalls   int
	releaseCalls int
}

// newFakeLeaseRepo builds a fake lease repo over in-memory (holder,token,until).
func newFakeLeaseRepo(holder string, token int64, until time.Time) *fakeLeaseRepo {
	f := &fakeLeaseRepo{}
	f.state = func() (string, int64, time.Time) { return holder, token, until }
	f.mutate = func(h string, t int64, u time.Time) {} // no-op: fixed state
	return f
}

func (f *fakeLeaseRepo) Acquire(ctx context.Context, holderID string, ttl time.Duration) (*domain.Lease, bool, error) {
	f.mu.Lock()
	f.acquireCalls++
	fn := f.AcquireFn
	f.mu.Unlock()
	if fn != nil {
		return fn(holderID, ttl)
	}
	// Default: win if the stored lease is absent/expired, else lose.
	holder, token, until := f.state()
	now := time.Now().UTC()
	if holder == "" || (until.IsZero() || now.After(until)) {
		newToken := int64(1)
		if holder != "" {
			newToken = token + 1
		}
		return &domain.Lease{Key: testLeaseKey, HolderID: holderID, FencingToken: newToken, LeaseUntil: now.Add(ttl), UpdatedAt: now}, true, nil
	}
	return &domain.Lease{Key: testLeaseKey, HolderID: holder, FencingToken: token, LeaseUntil: until, UpdatedAt: now}, false, nil
}

func (f *fakeLeaseRepo) Renew(ctx context.Context, lease *domain.Lease, ttl time.Duration) (bool, error) {
	f.mu.Lock()
	f.renewCalls++
	fn := f.RenewFn
	f.mu.Unlock()
	if fn != nil {
		return fn(lease, ttl)
	}
	// Fenced: renew only if the stored lease still matches (holder, token).
	holder, token, _ := f.state()
	hold := lease != nil && holder == lease.HolderID && token == lease.FencingToken
	return hold, nil
}

func (f *fakeLeaseRepo) Release(ctx context.Context, lease *domain.Lease) error {
	f.mu.Lock()
	f.releaseCalls++
	fn := f.ReleaseFn
	f.mu.Unlock()
	if fn != nil {
		return fn(lease)
	}
	return nil
}

func (f *fakeLeaseRepo) Close() error { return nil }

// fakeIngestRunRepo implements domain.IngestRunRepository and records the runs
// it saw so tests can assert which status was registered.
type fakeIngestRunRepo struct {
	mu            sync.Mutex
	lastSuccessAt *time.Time
	created       []domain.IngestRun
	finished      []domain.IngestRun
}

func (f *fakeIngestRunRepo) Create(ctx context.Context, run domain.IngestRun) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, run)
	return nil
}

func (f *fakeIngestRunRepo) Finish(ctx context.Context, run domain.IngestRun) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finished = append(f.finished, run)
	return nil
}

func (f *fakeIngestRunRepo) LastSuccess(ctx context.Context) (*domain.IngestRun, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lastSuccessAt == nil {
		return nil, false, nil
	}
	return &domain.IngestRun{RunID: "prev-success", Origin: "vps", FinishedAt: f.lastSuccessAt, Status: domain.IngestRunOK}, true, nil
}

func (f *fakeIngestRunRepo) LastAttempt(ctx context.Context) (*domain.IngestRun, bool, error) {
	return nil, false, nil
}

func (f *fakeIngestRunRepo) Close() error { return nil }

func (f *fakeIngestRunRepo) createdStatuses() []domain.IngestRunStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]domain.IngestRunStatus, 0, len(f.created))
	for _, r := range f.created {
		out = append(out, r.Status)
	}
	return out
}

// leaseTestConfig builds a valid config with one enabled source so the
// pipeline can run to completion quickly. Heartbeat/feeds/data paths point at
// the test's temp dir so no artifacts pollute the worktree.
func leaseTestConfig(t *testing.T) config.Config {
	tmp := t.TempDir()
	return config.Config{
		UserAgent:           "test/0.1",
		RequestTimeout:      30 * time.Second,
		LookbackWindow:      24 * time.Hour,
		SimilarityThreshold: 0.75,
		TemporalWindow:      48 * time.Hour,
		HeartbeatFile:       filepath.Join(tmp, "heartbeat.json"),
		FeedsFile:           filepath.Join(tmp, "feeds.json"),
		DataDir:             tmp,
		DataFile:            "homepage.json",
		HugoContentDir:      filepath.Join(tmp, "content"),
		RunnerOrigin:        "github-backup",
		StorageBackend:      "memory",
		Teams:               []config.Team{{Slug: "nfc", Name: "Test Team", ShortName: "TST", Abbreviation: "TST"}},
		Sources:             []config.Source{{ID: "src-1", Name: "Test Feed", URL: "https://example.invalid/feed.xml", Enabled: true}},
	}
}

// leaseTestRunner wires a runner with a successful single-source pipeline and
// injects the given lease and runs repos. A nil lease simulates the degraded
// "no lease" path.
func leaseTestRunner(t *testing.T, lease *fakeLeaseRepo, runs *fakeIngestRunRepo, fetch domain.FeedFetcher) *Runner {
	t.Helper()
	return NewRunner(
		WithOutput(&bytes.Buffer{}),
		WithErrorOutput(&bytes.Buffer{}),
		WithLoadConfig(func(path string, _ config.Overrides) (config.Config, error) { return leaseTestConfig(t), nil }),
		WithNewRepository(func(cfg config.StorageConfig) (domain.Repository, error) {
			return storage.NewMemoryRepository(), nil
		}),
		WithNewRuleRepo(func(cfg config.StorageConfig) (domain.CorrectionRuleRepository, error) {
			return nil, fmt.Errorf("no rules in test")
		}),
		WithNewIngestRunRepo(func(cfg config.StorageConfig) (domain.IngestRunRepository, error) {
			return runs, nil
		}),
		WithNewLeaseRepo(func(cfg config.StorageConfig) (domain.LeaseRepository, error) {
			if lease == nil {
				// Simulate the memory-backend degradation: the factory returns
				// ErrLeaseUnavailable, and the runner warns + proceeds (no lease).
				return nil, storage.ErrLeaseUnavailable
			}
			return lease, nil
		}),
		WithNewApp(func(opts []application.Option) *application.Application {
			return application.New(opts...)
		}),
		WithNewPublisher(func() *publish.LogPublisher {
			return publish.NewLogPublisher()
		}),
		WithNewFetcher(func(source domain.FeedSource, opts ...rss.Option) domain.FeedFetcher {
			if fetch != nil {
				return fetch
			}
			return fastFetcher{entry: domain.FeedEntry{
				Title:       "Test story",
				Source:      source.ID,
				PublishedAt: time.Now().UTC().Add(-time.Hour),
				Summary:     "A test summary",
				URL:         "https://example.invalid/story-1",
				GUID:        "guid-1",
			}}
		}),
	)
}

type fastFetcher struct{ entry domain.FeedEntry }

func (f fastFetcher) Fetch(ctx context.Context, sourceID string) ([]domain.FeedEntry, error) {
	return []domain.FeedEntry{f.entry}, nil
}

// blockingFetcher blocks until ctx is cancelled, so the pipeline stays alive
// long enough for the renewal goroutine to fire (lease-lost test).
type blockingFetcher struct{ entry domain.FeedEntry }

func (f blockingFetcher) Fetch(ctx context.Context, sourceID string) ([]domain.FeedEntry, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func ptrTime(t time.Time) *time.Time { return &t }

// hasStatus reports whether statuses contains s.
func hasStatus(statuses []domain.IngestRunStatus, s domain.IngestRunStatus) bool {
	for _, v := range statuses {
		if v == s {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// AC-1: lease held -> skipped_locked, visible skip, exit 0, no pipeline run.
// ---------------------------------------------------------------------------

func TestRunIngest_LeaseHeld_SkipsLocked(t *testing.T) {
	other := time.Now().UTC().Add(20 * time.Minute)
	lease := newFakeLeaseRepo("vps:host-a:abcd1234", 5, other)
	runs := &fakeIngestRunRepo{}

	r := leaseTestRunner(t, lease, runs, nil)
	result := r.RunIngest(context.Background(), "primary", 75*time.Minute, false)

	if result.ExitCode != 0 {
		t.Errorf("skipped_locked must exit 0, got %d (%v)", result.ExitCode, result.Error)
	}
	if !hasStatus(runs.createdStatuses(), domain.IngestRunSkippedLocked) {
		t.Errorf("expected a skipped_locked run to be registered, got %v", runs.createdStatuses())
	}
	if hasStatus(runs.createdStatuses(), domain.IngestRunRunning) {
		t.Error("a running row must NOT be registered when the lease is held")
	}
	if result.Result != nil {
		t.Error("the pipeline result must be nil when the lease is held")
	}
	if lease.acquireCalls != 1 {
		t.Errorf("exactly one Acquire expected, got %d", lease.acquireCalls)
	}
}

// ---------------------------------------------------------------------------
// AC-3 (backup pre-check): fresh data -> skip + exit 0 WITHOUT touching the lease.
// ---------------------------------------------------------------------------

func TestRunIngest_BackupPreCheckFresh_NoLeaseTouch(t *testing.T) {
	lease := newFakeLeaseRepo("", 0, time.Time{})
	runs := &fakeIngestRunRepo{lastSuccessAt: ptrTime(time.Now().UTC().Add(-10 * time.Minute))} // fresh

	r := leaseTestRunner(t, lease, runs, nil)
	result := r.RunIngest(context.Background(), "backup", 75*time.Minute, false)

	if result.ExitCode != 0 {
		t.Errorf("fresh backup pre-check must exit 0, got %d (%v)", result.ExitCode, result.Error)
	}
	if lease.acquireCalls != 0 {
		t.Errorf("pre-check-fresh must not Acquire the lease, got %d", lease.acquireCalls)
	}
	if len(runs.created) != 0 {
		t.Errorf("pre-check-fresh should not register a run row, got %d", len(runs.created))
	}
	if result.Result != nil {
		t.Error("pipeline must not run on pre-check-fresh")
	}
}

// ---------------------------------------------------------------------------
// AC-3 (TOCTOU): stale pre-check -> acquire -> in-lock re-check fresh ->
// skipped_fresh + release + exit 0, NO RSS fetch.
// ---------------------------------------------------------------------------

func TestRunIngest_BackupInLockFresh_SkipsFresh(t *testing.T) {
	// Start stale (2h) so the pre-check passes; on the first Acquire simulate a
	// primary recovering (lastSuccess becomes fresh), so the in-lock re-check
	// finds the data fresh.
	runs := &fakeIngestRunRepo{lastSuccessAt: ptrTime(time.Now().UTC().Add(-2 * time.Hour))}
	lease := newFakeLeaseRepo("", 0, time.Time{})
	fresh := time.Now().UTC().Add(-5 * time.Minute)
	lease.AcquireFn = func(holderID string, ttl time.Duration) (*domain.Lease, bool, error) {
		runs.lastSuccessAt = ptrTime(fresh) // primary recovered while we acquired
		now := time.Now().UTC()
		return &domain.Lease{Key: testLeaseKey, HolderID: holderID, FencingToken: 1, LeaseUntil: now.Add(ttl), UpdatedAt: now}, true, nil
	}

	r := leaseTestRunner(t, lease, runs, nil)
	result := r.RunIngest(context.Background(), "backup", 75*time.Minute, false)

	if result.ExitCode != 0 {
		t.Errorf("in-lock-fresh must exit 0, got %d (%v)", result.ExitCode, result.Error)
	}
	if !hasStatus(runs.createdStatuses(), domain.IngestRunSkippedFresh) {
		t.Errorf("expected skipped_fresh to be registered, got %v", runs.createdStatuses())
	}
	if hasStatus(runs.createdStatuses(), domain.IngestRunRunning) {
		t.Error("a running row must NOT be registered when in-lock-fresh")
	}
	if lease.releaseCalls != 1 {
		t.Errorf("lease must be released after in-lock-fresh, got %d releases", lease.releaseCalls)
	}
	if result.Result != nil {
		t.Error("pipeline must not run when in-lock-fresh")
	}
}

// ---------------------------------------------------------------------------
// AC-3 (--force): force skips the staleness pre-check but NEVER the lease. A
// held lease still produces skipped_locked even under --force.
// ---------------------------------------------------------------------------

func TestRunIngest_ForceNeverSkipsLease(t *testing.T) {
	other := time.Now().UTC().Add(20 * time.Minute)
	lease := newFakeLeaseRepo("vps:host-a:abcd1234", 5, other)
	runs := &fakeIngestRunRepo{lastSuccessAt: ptrTime(time.Now().UTC().Add(-2 * time.Hour))}

	r := leaseTestRunner(t, lease, runs, nil)
	result := r.RunIngest(context.Background(), "backup", 75*time.Minute, true) // force

	if result.ExitCode != 0 {
		t.Errorf("forced held-lease run must exit 0, got %d (%v)", result.ExitCode, result.Error)
	}
	if lease.acquireCalls != 1 {
		t.Errorf("--force must still Acquire the lease, got %d", lease.acquireCalls)
	}
	if !hasStatus(runs.createdStatuses(), domain.IngestRunSkippedLocked) {
		t.Errorf("forced held-lease run must register skipped_locked, got %v", runs.createdStatuses())
	}
}

// ---------------------------------------------------------------------------
// AC-4 (fencing): a lease lost during the pipeline -> run forced to failed,
// no commit, non-zero exit.
// ---------------------------------------------------------------------------

func TestRunIngest_LeaseLostForcesFailed(t *testing.T) {
	// Shorten the renewal interval so the renewal goroutine fires promptly, and
	// use a blocking fetcher so the pipeline stays alive until the renewal (which
	// reports the lease lost) cancels runCtx.
	origInterval := LeaseRenewInterval
	LeaseRenewInterval = 5 * time.Millisecond
	t.Cleanup(func() { LeaseRenewInterval = origInterval })

	lease := newFakeLeaseRepo("", 0, time.Time{})
	// The first Renew reports the lease as lost (false).
	lease.RenewFn = func(lease *domain.Lease, ttl time.Duration) (bool, error) {
		return false, nil
	}
	runs := &fakeIngestRunRepo{lastSuccessAt: ptrTime(time.Now().UTC().Add(-2 * time.Hour))}

	r := leaseTestRunner(t, lease, runs, blockingFetcher{entry: domain.FeedEntry{Title: "x", Source: "src-1", URL: "https://example.invalid/x", GUID: "g", PublishedAt: time.Now().UTC()}})
	result := r.RunIngest(context.Background(), "primary", 75*time.Minute, false)

	if result.ExitCode == 0 {
		t.Errorf("lost-lease run must exit non-zero, got 0 (%v)", result.Error)
	}
	if result.Result == nil {
		t.Fatal("expected a run record even when the lease is lost")
	}
	if result.Result.Status != domain.RunStatusFailed {
		t.Errorf("lost-lease run must be forced to failed, got %v", result.Result.Status)
	}
	if result.Result.Committed {
		t.Error("lost-lease run must NOT commit")
	}
	if lease.renewCalls == 0 {
		t.Error("the renewal goroutine must have attempted at least one Renew")
	}
}

// ---------------------------------------------------------------------------
// Degraded mode: no lease (memory backend / ErrLeaseUnavailable) -> warn,
// proceed, success.
// ---------------------------------------------------------------------------

func TestRunIngest_LeaseUnavailable_DegradedProceeds(t *testing.T) {
	runs := &fakeIngestRunRepo{lastSuccessAt: ptrTime(time.Now().UTC().Add(-2 * time.Hour))}

	// A nil lease simulates the degraded "no lease" path (the runner proceeds
	// without mutual exclusion).
	r := leaseTestRunner(t, nil, runs, nil)
	result := r.RunIngest(context.Background(), "primary", 75*time.Minute, false)

	if result.ExitCode != 0 {
		t.Errorf("degraded (no lease) primary run must succeed, got %d (%v)", result.ExitCode, result.Error)
	}
	if result.Result == nil {
		t.Fatal("the pipeline must have run without a lease (degraded)")
	}
	if !hasStatus(runs.createdStatuses(), domain.IngestRunRunning) {
		t.Errorf("a running row must be registered for a real run, got %v", runs.createdStatuses())
	}
}

// ---------------------------------------------------------------------------
// Holder-ID sanitization (plan §7: no secrets, no shell metacharacters).
// ---------------------------------------------------------------------------

func TestSanitizeHolderID(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"github-backup:host-a:ab12cd34", "github-backup:host-a:ab12cd34"},
		{"VPS:MyHost:ABCD", "vps:myhost:abcd"},             // lowercased
		{"origin@host/path:token", "originhostpath:token"}, // @ . / dropped
		{"bad!@#$%^&*()", "bad"},                           // only [a-z0-9:-] kept
		{"", "unknown"},                                    // empty -> unknown
		{"UPPER:Case-123", "upper:case-123"},               // preserves - and :
	}
	for _, c := range cases {
		if got := sanitizeHolderID(c.in); got != c.want {
			t.Errorf("sanitizeHolderID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHolderIDOf(t *testing.T) {
	if got := holderIDOf(nil); got != "unknown" {
		t.Errorf("holderIDOf(nil) = %q, want unknown", got)
	}
	if got := holderIDOf(&domain.Lease{HolderID: "vps:x"}); got != "vps:x" {
		t.Errorf("holderIDOf = %q, want vps:x", got)
	}
}
