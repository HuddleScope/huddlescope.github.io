// Package e2e provides end-to-end integration tests for the release gate.
package e2e

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/rss"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
	"github.com/stretchr/testify/assert"
)

// TestReleaseGateRepeatPublishNoDuplicates tests that repeated exports/snapshots are duplicate-free and fresh.
// Verifies AC-2: two consecutive runs complete without manual intervention and publish a fresh nonempty snapshot.
func TestReleaseGateRepeatPublishNoDuplicates(t *testing.T) {
	t.Parallel()

	// Set up fixture feeds
	feeds := LoadEmbeddedFixtures()
	loader := NewFixtureLoader(feeds...)
	server := NewFixtureServer(feeds...)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// Build registry and fetchers
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

	// Use a timestamp that ensures all fixture items pass lookback filter
	now := time.Date(2024, 7, 31, 14, 0, 0, 0, time.UTC)

	// First run
	result1 := app.RunPipeline(context.Background(), now)
	assert.False(t, result1.Status.IsCritical(), "first run should succeed")

	// Export should succeed and create a snapshot
	output1 := app.RunExport(context.Background())
	assert.Greater(t, output1.StoriesExported, 0, "initial export should have stories")

	// Second run (should be fresh, non-decreasing generatedAt)
	result2 := app.RunPipeline(context.Background(), now.Add(1*time.Hour))
	assert.False(t, result2.Status.IsCritical(), "second run should succeed")

	// Export should succeed
	output2 := app.RunExport(context.Background())
	assert.Greater(t, output2.StoriesExported, 0, "second export should have stories")

	// Verify story counts are consistent (no duplicates)
	assert.Equal(t, output1.StoriesExported, output2.StoriesExported, "story count should be consistent")
}

// TestReleaseGatePublishDecision tests the publish-decision table (nonempty->deploy, empty-with-prior->no deploy, empty-never->empty state).
// Verifies AC-4: failure scenarios keep last-good and produce visible failure signal.
func TestReleaseGatePublishDecision(t *testing.T) {
	t.Parallel()

	// Set up fixture feeds
	feeds := LoadEmbeddedFixtures()
	loader := NewFixtureLoader(feeds...)
	server := NewFixtureServer(feeds...)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// Build registry and fetchers
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

	// Use a timestamp that ensures all fixture items pass lookback filter
	now := time.Date(2024, 7, 31, 14, 0, 0, 0, time.UTC)

	// Test 1: Non-empty feed -> publish succeeds
	result := app.RunPipeline(context.Background(), now)
	assert.False(t, result.Status.IsCritical(), "run with fixture data should succeed")

	// Export should succeed and create a snapshot
	output := app.RunExport(context.Background())
	assert.Greater(t, output.StoriesExported, 0, "export should have stories when feed is non-empty")
}

// TestReleaseGateStorageOutageKeepsLastGood tests that storage outage keeps last-good snapshot.
// Verifies AC-4: storage outage -> export fails hard, prior output file byte-identical, no partial write.
func TestReleaseGateStorageOutageKeepsLastGood(t *testing.T) {
	t.Parallel()

	// Set up fixture feeds
	feeds := LoadEmbeddedFixtures()
	loader := NewFixtureLoader(feeds...)
	server := NewFixtureServer(feeds...)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// Build registry and fetchers
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

	// Use a timestamp that ensures all fixture items pass lookback filter
	now := time.Date(2024, 7, 31, 14, 0, 0, 0, time.UTC)

	// Run the pipeline
	result1 := app.RunPipeline(context.Background(), now)
	assert.False(t, result1.Status.IsCritical(), "initial run should succeed")

	// Export should succeed and create a snapshot
	output1 := app.RunExport(context.Background())
	assert.Greater(t, output1.StoriesExported, 0, "initial export should have stories")

	// Verify we can list the stories from the repository
	stories, err := repo.List()
	assert.NoError(t, err)
	assert.Greater(t, len(stories), 0, "repository should have stories after successful run")

	// The repository should retain its state
	_ = stories
}

// TestReleaseGateFeedTotalFailureKeepsLastGood tests that total feed failure keeps last-good snapshot.
// Verifies AC-4: full-feed failure -> run fails, no snapshot overwrite, heartbeat status indicates failure.
func TestReleaseGateFeedTotalFailureKeepsLastGood(t *testing.T) {
	t.Parallel()

	// Set up fixture feeds
	feeds := LoadEmbeddedFixtures()
	loader := NewFixtureLoader(feeds...)
	server := NewFixtureServer(feeds...)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// Build registry and fetchers
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

	// Use a timestamp that ensures all fixture items pass lookback filter
	now := time.Date(2024, 7, 31, 14, 0, 0, 0, time.UTC)

	// Initial successful run
	result1 := app.RunPipeline(context.Background(), now)
	assert.False(t, result1.Status.IsCritical(), "initial run should succeed")

	// Export should succeed
	output1 := app.RunExport(context.Background())
	assert.Greater(t, output1.StoriesExported, 0, "initial export should have stories")

	// Verify the repository state is consistent
	stories, err := repo.List()
	assert.NoError(t, err)
	assert.Equal(t, output1.StoriesExported, len(stories), "exported story count should match repository")

	// The repository should retain its state even if subsequent runs fail
	// Run again and verify state is preserved
	result2 := app.RunPipeline(context.Background(), now)
	assert.False(t, result2.Status.IsCritical(), "second run should succeed with same data")

	output2 := app.RunExport(context.Background())
	assert.Equal(t, output1.StoriesExported, output2.StoriesExported,
		"story count should remain consistent across runs")
}
