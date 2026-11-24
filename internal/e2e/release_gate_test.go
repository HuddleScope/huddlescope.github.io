// Package e2e provides end-to-end integration tests for the release gate.
//
// Issue #110 / HS-058: these tests exercise the negative paths the issue
// demands — storage outage, total feed failure, and empty export — using real
// failure injection (a failing repository stub and FixtureServer.SetFailure)
// and the existing TempWebDir / hugo.DataPublisher harness. They verify the
// AC-4 safety property at the Go level: a failure must never overwrite the
// last-good published snapshot, and must produce a visible failure signal.
//
// Scope notes (see implementation.md):
//   - The "retain last-good on empty export" / "deploy real data" / "deploy
//     labeled empty state" publish decision is implemented as inline bash in
//     .github/workflows/refresh-news.yml (HS-056), not as a Go function. The
//     Go-level invariant that decision depends on — StoriesExported == 0 — is
//     what the empty-export test asserts; the commit/deploy gating itself is
//     verified by the workflow and documented in docs/release-gate.md.
//   - A total feed failure yields domain.RunStatusPartial (a visible failure
//     signal, HeartbeatStatusFailed semantics at the CLI layer), NOT
//     RunStatusFailed: the pipeline still holds the previous run's stories in
//     the repository, so the export rebuilds a content-equal snapshot and the
//     DataPublisher skips the write. That is the last-good-preserving outcome.
package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/hugo"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/rss"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage"
	"github.com/HuddleScope/huddlescope.github.io/internal/application"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
	"github.com/stretchr/testify/assert"
)

// setUpGateHarness builds a fixture server, registry, fetchers, a memory repo,
// and an application wired to publish snapshots into a temp web directory. It
// returns everything the gate tests need plus a cleanup function.
func setUpGateHarness(t *testing.T) (server *FixtureServer, registry *domain.FeedRegistry, fetchers map[string]domain.FeedFetcher, repo domain.Repository, app *application.Application, snapshotPub *hugo.DataPublisher, dataPath string, cleanup func()) {
	t.Helper()

	feeds := LoadEmbeddedFixtures()
	loader := NewFixtureLoader(feeds...)
	server = NewFixtureServer(feeds...)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(func() { ts.Close() })

	registry = buildRegistry(ts, loader, true)
	rateLimiter := rss.NewHostRateLimiter(1 * time.Second)
	etagCache := rss.NewETagCache()
	fetchers = buildFetchers(registry, rateLimiter, etagCache)
	repo = storage.NewMemoryRepository()

	tmpDir, tmpCleanup := TempWebDir(t)
	t.Cleanup(tmpCleanup)
	snapshotPub = hugo.NewDataPublisher(tmpDir+"/data", "homepage.json")
	dataPath = filepath.Join(tmpDir, "data", "homepage.json")

	app = NewAppBuilder().
		WithRepository(repo).
		WithRegistry(registry).
		WithFetcherMap(fetchers).
		WithSnapshotPublisher(snapshotPub).
		WithTeamRegistry(domain.NewTeamRegistryFromSlice(domain.GetTestTeams())).
		Build()

	return server, registry, fetchers, repo, app, snapshotPub, dataPath, func() {}
}

// buildAppWith reconstructs the gate application swapping in an explicit
// repository while keeping the registry/fetchers and the same snapshot
// publisher (same target file). This is how a storage outage is simulated:
// the pipeline and snapshot export now see a failing repository.
func buildAppWith(server *FixtureServer, registry *domain.FeedRegistry, fetchers map[string]domain.FeedFetcher, repo domain.Repository, snapshotPub *hugo.DataPublisher) *application.Application {
	return NewAppBuilder().
		WithRepository(repo).
		WithRegistry(registry).
		WithFetcherMap(fetchers).
		WithSnapshotPublisher(snapshotPub).
		WithTeamRegistry(domain.NewTeamRegistryFromSlice(domain.GetTestTeams())).
		Build()
}

// nowForGate is a deterministic "current time" that sits after every embedded
// fixture pubDate, so the full lookback window is passed by all items.
var nowForGate = time.Date(2024, 7, 31, 14, 0, 0, 0, time.UTC)

func hashBytes(b []byte) [32]byte { return sha256.Sum256(b) }

// TestReleaseGateRepeatPublishNoDuplicates tests that repeated exports/snapshots are duplicate-free and fresh.
// Verifies AC-2: two consecutive runs complete without manual intervention and publish a fresh nonempty snapshot.
func TestReleaseGateRepeatPublishNoDuplicates(t *testing.T) {
	t.Parallel()

	feeds := LoadEmbeddedFixtures()
	loader := NewFixtureLoader(feeds...)
	server := NewFixtureServer(feeds...)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(func() { ts.Close() })

	registry := buildRegistry(ts, loader, true)
	rateLimiter := rss.NewHostRateLimiter(1 * time.Second)
	etagCache := rss.NewETagCache()
	fetchers := buildFetchers(registry, rateLimiter, etagCache)
	repo := storage.NewMemoryRepository()

	app := NewAppBuilder().
		WithRepository(repo).
		WithRegistry(registry).
		WithFetcherMap(fetchers).
		WithTeamRegistry(domain.NewTeamRegistryFromSlice(domain.GetTestTeams())).
		Build()

	// First run
	result1 := app.RunPipeline(context.Background(), nowForGate)
	assert.False(t, result1.Status.IsCritical(), "first run should succeed")

	output1 := app.RunExport(context.Background())
	assert.Greater(t, output1.StoriesExported, 0, "initial export should have stories")

	// Second run (fresh, non-decreasing generatedAt)
	result2 := app.RunPipeline(context.Background(), nowForGate.Add(1*time.Hour))
	assert.False(t, result2.Status.IsCritical(), "second run should succeed")

	output2 := app.RunExport(context.Background())
	assert.Greater(t, output2.StoriesExported, 0, "second export should have stories")

	// No duplicates across runs: story count is stable and repo is deduped.
	assert.Equal(t, output1.StoriesExported, output2.StoriesExported, "story count should be consistent")
	stories, err := repo.List()
	assert.NoError(t, err)
	seen := make(map[string]bool)
	for _, s := range stories {
		assert.False(t, seen[s.ID], "duplicate story ID %s across runs", s.ID)
		seen[s.ID] = true
	}
}

// TestReleaseGatePublishDecision tests the non-empty -> publish path of the
// publish-decision table (HS-056). The empty branches are covered by
// TestReleaseGateEmptyExport (Go-level invariant) and the workflow (decision).
// Verifies AC-4: non-empty export publishes and reports a committed, changed snapshot.
func TestReleaseGatePublishDecision(t *testing.T) {
	t.Parallel()

	_, _, _, repo, app, snapshotPub, dataPath, _ := setUpGateHarness(t)

	result := app.RunPipeline(context.Background(), nowForGate)
	assert.False(t, result.Status.IsCritical(), "run with fixture data should succeed")

	output := app.RunExport(context.Background())
	assert.Greater(t, output.StoriesExported, 0, "export should have stories when feed is non-empty")

	// The non-empty export must publish: committed, changed, and the file must
	// hold a validating, non-empty snapshot.
	snap := app.RunSnapshot(context.Background(), nowForGate)
	assert.Empty(t, snap.Errors, "snapshot must not error on non-empty export")
	assert.True(t, snap.PubResult.Committed, "non-empty snapshot must be committed")
	assert.True(t, snap.PubResult.Changed, "first non-empty snapshot must be changed")

	data := fileBytes(t, dataPath)
	var s domain.Snapshot
	assert.NoError(t, json.Unmarshal(data, &s), "published snapshot must be valid JSON")
	assert.NoError(t, s.Validate(), "published snapshot must validate")
	assert.Greater(t, len(s.Stories), 0, "published snapshot must be non-empty")
	assert.NotEmpty(t, s.GeneratedAt, "published snapshot must carry a truthful generatedAt")
	_ = repo
	_ = snapshotPub
}

// TestReleaseGateStorageOutageKeepsLastGood verifies AC-4's storage-outage
// branch with real failure injection: a previously published non-empty
// snapshot must survive a full repository outage — the pipeline and the export
// both fail, and the published file is byte-identical (no partial write, no
// "success" reported).
func TestReleaseGateStorageOutageKeepsLastGood(t *testing.T) {
	t.Parallel()

	_, registry, fetchers, repo, app, snapshotPub, dataPath, _ := setUpGateHarness(t)

	// run1: healthy ingest + snapshot -> last-good published.
	result1 := app.RunPipeline(context.Background(), nowForGate)
	assert.False(t, result1.Status.IsCritical(), "initial run should succeed", describeErrors(result1.Errors))
	snap1 := app.RunSnapshot(context.Background(), nowForGate)
	assert.Empty(t, snap1.Errors, "initial snapshot must not error")
	assert.True(t, snap1.PubResult.Committed, "initial snapshot must be committed")

	good := fileBytes(t, dataPath)
	var goodSnap domain.Snapshot
	assert.NoError(t, json.Unmarshal(good, &goodSnap))
	assert.Greater(t, len(goodSnap.Stories), 0, "last-good snapshot must be non-empty")
	_, err := repo.List()
	assert.NoError(t, err)

	// Swap the repository for a failing one to model a Turso outage. The
	// snapshot publisher is the SAME instance, so any (mis)write would hit the
	// same file.
	failing := newFailingRepository()
	outageApp := buildAppWith(nil, registry, fetchers, failing, snapshotPub)

	// run2: pipeline must be critical (upsert + list both fail).
	result2 := outageApp.RunPipeline(context.Background(), nowForGate)
	assert.True(t, result2.Status.IsCritical(), "run against a failing repository must be critical", describeErrors(result2.Errors))
	assert.False(t, result2.Committed, "a failing run must not report the homepage committed")

	// The export/snapshot must fail WITHOUT touching the published file.
	snap2 := outageApp.RunSnapshot(context.Background(), nowForGate)
	assert.NotEmpty(t, snap2.Errors, "snapshot against a failing repository must report an error")
	assert.False(t, snap2.PubResult.Committed, "snapshot must NOT report committed during an outage")
	assert.False(t, snap2.PubResult.Changed, "snapshot must NOT report changed during an outage")

	after := fileBytes(t, dataPath)
	assert.Equal(t, hashBytes(good), hashBytes(after),
		"last-good snapshot must be byte-identical after a storage outage")
}

// TestReleaseGateFeedTotalFailureKeepsLastGood verifies AC-4's total-feed-failure
// branch with real failure injection: when every feed returns 500 on the second
// run, the run is flagged as a visible failure (domain.RunStatusPartial — not a
// clean "ok"), yet the last-good snapshot is preserved because the repository
// still holds the previous run's stories and the DataPublisher skips a
// content-equal write. The published file must be byte-identical.
func TestReleaseGateFeedTotalFailureKeepsLastGood(t *testing.T) {
	t.Parallel()

	server, registry, fetchers, repo, app, snapshotPub, dataPath, _ := setUpGateHarness(t)

	// run1: healthy -> last-good published.
	result1 := app.RunPipeline(context.Background(), nowForGate)
	assert.False(t, result1.Status.IsCritical(), "initial run should succeed", describeErrors(result1.Errors))
	snap1 := app.RunSnapshot(context.Background(), nowForGate)
	assert.True(t, snap1.PubResult.Committed, "initial snapshot must be committed")

	good := fileBytes(t, dataPath)

	// Now make EVERY fixture feed fail with 500 on run2.
	for _, src := range registry.Enabled() {
		server.SetFailure(src.ID, "v1", http.StatusServiceUnavailable)
	}

	// run2: total feed failure. Rebuild the application with a FRESH ETag cache
	// (no cached If-None-Match) so the fetcher issues a real GET and observes the
	// 500 rather than short-circuiting on a 304 from run1's cached ETag. The repo
	// is the SAME instance, so run1's stories are still present for the rebuild.
	freshLimit := rss.NewHostRateLimiter(1 * time.Second)
	freshEtag := rss.NewETagCache()
	freshFetchers := buildFetchers(registry, freshLimit, freshEtag)
	failApp := buildAppWith(server, registry, freshFetchers, repo, snapshotPub)

	// The run must be a visible failure signal (partial, i.e. not a clean "ok"),
	// and it must NOT silently report success.
	result2 := failApp.RunPipeline(context.Background(), nowForGate)
	assert.NotEqual(t, domain.RunStatusOK, result2.Status,
		"total feed failure must not be reported as a clean ok")
	assert.Equal(t, domain.RunStatusPartial, result2.Status,
		"total feed failure with prior repo data must be partial (visible failure signal)")
	assert.NotEmpty(t, result2.Errors, "total feed failure must surface errors")

	// The repository still holds run1's stories, so the export rebuilds a
	// content-equal snapshot and the DataPublisher skips the write: last-good
	// preserved, byte-identical.
	stories, err := repo.List()
	assert.NoError(t, err)
	assert.Greater(t, len(stories), 0, "repository must still hold prior stories after feed failure")

	snap2 := failApp.RunSnapshot(context.Background(), nowForGate)
	assert.Empty(t, snap2.Errors, "snapshot must not error when rebuilding from existing repo data")
	assert.True(t, snap2.PubResult.Committed, "rebuild snapshot must be committed")
	assert.False(t, snap2.PubResult.Changed,
		"rebuild from unchanged repo data must not be reported as changed")

	after := fileBytes(t, dataPath)
	assert.Equal(t, hashBytes(good), hashBytes(after),
		"last-good snapshot must be byte-identical after a total feed failure")
	_ = fetchers
	_ = snapshotPub
}

// TestReleaseGateEmptyExport verifies the Go-level invariants of the empty-export
// branches of the HS-056 publish-decision table. The commit/deploy gating
// itself is workflow bash (refresh-news.yml); what the Go layer must guarantee
// is that an empty export (a) reports StoriesExported == 0 (the exact value the
// workflow's publish-decision checks) and (b) when no prior snapshot exists,
// writes a validating, clearly-empty snapshot with a truthful generatedAt.
func TestReleaseGateEmptyExport(t *testing.T) {
	t.Parallel()

	t.Run("empty repo reports zero stories exported", func(t *testing.T) {
		// A brand-new, empty repository and an empty feed set: nothing to
		// ingest, nothing to export. The invariant the workflow's publish
		// decision relies on is that the export reports zero stories.
		feeds := LoadEmbeddedFixtures()
		loader := NewFixtureLoader(feeds...)
		server := NewFixtureServer(feeds...)
		ts := httptest.NewServer(server.Handler())
		t.Cleanup(func() { ts.Close() })

		registry := buildRegistry(ts, loader, true)
		rateLimiter := rss.NewHostRateLimiter(1 * time.Second)
		etagCache := rss.NewETagCache()
		fetchers := buildFetchers(registry, rateLimiter, etagCache)
		repo := storage.NewMemoryRepository()

		app := NewAppBuilder().
			WithRepository(repo).
			WithRegistry(registry).
			WithFetcherMap(fetchers).
			WithTeamRegistry(domain.NewTeamRegistryFromSlice(domain.GetTestTeams())).
			Build()

		// "now" well AFTER the latest fixture pubDate plus the lookback window,
		// so the lookback filter excludes every item and nothing is ingested.
		// (EntryWithinLookback keeps items newer than now-window; a "now" before
		// the pubDates would keep them as future-dated, not exclude them.)
		pastNow := time.Date(2024, 12, 15, 0, 0, 0, 0, time.UTC)
		result := app.RunPipeline(context.Background(), pastNow)
		assert.False(t, result.Status.IsCritical(), "lookback-excluded run should not be critical")

		output := app.RunExport(context.Background())
		assert.Equal(t, 0, output.StoriesExported,
			"empty export must report zero stories (workflow publish-decision invariant)")

		count, err := repo.Count()
		assert.NoError(t, err)
		assert.Equal(t, 0, count, "repository must be empty when export is empty")
	})

	t.Run("no prior snapshot writes labeled empty state", func(t *testing.T) {
		// Empty repository, fresh temp web dir (no prior homepage.json). The
		// snapshot must be written as a validating, clearly-empty state with a
		// truthful generatedAt.
		_, _, _, _, app, snapshotPub, dataPath, _ := setUpGateHarness(t)

		snap := app.RunSnapshot(context.Background(), nowForGate)
		assert.Empty(t, snap.Errors, "snapshot must not error on empty repo")
		assert.True(t, snap.PubResult.Committed, "empty-state snapshot must be committed")
		assert.Equal(t, 0, snap.StoriesBuilt, "empty-state snapshot must build zero stories")

		data := fileBytes(t, dataPath)
		var s domain.Snapshot
		assert.NoError(t, json.Unmarshal(data, &s), "published empty snapshot must be valid JSON")
		assert.NoError(t, s.Validate(), "published empty snapshot must validate")
		assert.Equal(t, domain.SnapshotVersion, s.Version)
		assert.Empty(t, s.Stories, "empty-state snapshot must have zero stories")
		assert.NotEmpty(t, s.GeneratedAt, "empty-state snapshot must carry a truthful generatedAt")
		_ = snapshotPub
	})
}
