// Package runner: export site-data publication tests (issue #163 / HS-PORT-07 / AC-8).
package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/publish"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/rss"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage"
	"github.com/HuddleScope/huddlescope.github.io/internal/application"
	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// exportFakeIngestRunRepo implements domain.IngestRunRepository for export
// tests, returning a fixed last attempt and last success.
type exportFakeIngestRunRepo struct {
	lastAttempt *domain.IngestRun
	lastSuccess *domain.IngestRun
	err         error // if set, LastAttempt/LastSuccess return this error
}

func (f *exportFakeIngestRunRepo) Create(ctx context.Context, run domain.IngestRun) error {
	return nil
}
func (f *exportFakeIngestRunRepo) Finish(ctx context.Context, run domain.IngestRun) error {
	return nil
}
func (f *exportFakeIngestRunRepo) LastSuccess(ctx context.Context) (*domain.IngestRun, bool, error) {
	if f.err != nil {
		return nil, false, f.err
	}
	if f.lastSuccess == nil {
		return nil, false, nil
	}
	return f.lastSuccess, true, nil
}
func (f *exportFakeIngestRunRepo) LastAttempt(ctx context.Context) (*domain.IngestRun, bool, error) {
	if f.err != nil {
		return nil, false, f.err
	}
	if f.lastAttempt == nil {
		return nil, false, nil
	}
	return f.lastAttempt, true, nil
}
func (f *exportFakeIngestRunRepo) Close() error { return nil }

// exportTestConfig builds a valid config for export tests.
func exportTestConfig(t *testing.T, publishSiteData, env string) config.Config {
	t.Helper()
	tmp := t.TempDir()
	webDataDir := filepath.Join(tmp, "web", "data")
	contentDir := filepath.Join(tmp, "content")
	// Ensure dirs exist (the publishers create them, but let's be explicit).
	_ = os.MkdirAll(webDataDir, 0o750) //nolint:errcheck
	_ = os.MkdirAll(contentDir, 0o750) //nolint:errcheck

	return config.Config{
		UserAgent:           "test/0.1",
		RequestTimeout:      30 * time.Second,
		LookbackWindow:      24 * time.Hour,
		SimilarityThreshold: 0.75,
		TemporalWindow:      48 * time.Hour,
		HeartbeatFile:       filepath.Join(webDataDir, "heartbeat.json"),
		FeedsFile:           filepath.Join(webDataDir, "feeds.json"),
		DataDir:             webDataDir,
		DataFile:            "homepage.json",
		HugoContentDir:      contentDir,
		HugoBaseURL:         "https://example.com",
		RunnerOrigin:        "github-backup",
		StorageBackend:      "memory",
		PublishSiteData:     publishSiteData,
		Env:                 env,
		Teams:               []config.Team{{Slug: "nfc", Name: "Test Team", ShortName: "TST", Abbreviation: "TST"}},
		Sources:             []config.Source{{ID: "src-1", Name: "Test Feed", URL: "https://example.invalid/feed.xml", Enabled: true}},
	}
}

// exportTestRunner wires a runner for export tests.
func exportTestRunner(t *testing.T, cfg config.Config, runsRepo domain.IngestRunRepository) *Runner {
	t.Helper()
	return NewRunner(
		WithOutput(&bytes.Buffer{}),
		WithErrorOutput(&bytes.Buffer{}),
		WithLoadConfig(func(path string, _ config.Overrides) (config.Config, error) { return cfg, nil }),
		WithNewRepository(func(cfg config.StorageConfig) (domain.Repository, error) {
			return storage.NewMemoryRepository(), nil
		}),
		WithNewIngestRunRepo(func(cfg config.StorageConfig) (domain.IngestRunRepository, error) {
			return runsRepo, nil
		}),
		WithNewApp(func(opts []application.Option) *application.Application {
			return application.New(opts...)
		}),
		WithNewPublisher(func() *publish.LogPublisher {
			return publish.NewLogPublisher()
		}),
		WithNewFetcher(func(source domain.FeedSource, opts ...rss.Option) domain.FeedFetcher {
			return fastFetcher{entry: domain.FeedEntry{
				Title:       "Test story",
				Source:      source.ID,
				PublishedAt: time.Now().UTC().Add(-time.Hour),
				Summary:     "A test summary",
				URL:         "https://example.invalid/story-1",
				GUID:        "guid-1",
			}}
		}),
		WithNow(func() time.Time {
			return time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
		}),
	)
}

// TestExportPublishSiteDataTrue verifies that RunExport with
// HSC_PUBLISH_SITE_DATA=true re-publishes heartbeat.json and feeds.json from
// the shared DB (issue #163 / HS-PORT-07 / AC-8).
func TestExportPublishSiteDataTrue(t *testing.T) {
	t.Parallel()

	cfg := exportTestConfig(t, "true", "dev")
	now := time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC)
	endedAt := now

	runsRepo := &exportFakeIngestRunRepo{
		lastAttempt: &domain.IngestRun{
			RunID:            "export-test-run",
			Origin:           "github-backup",
			StartedAt:        now.Add(-5 * time.Minute),
			FinishedAt:       &endedAt,
			Status:           domain.IngestRunOK,
			SourcesChecked:   4,
			SourcesSucceeded: 4,
			EntriesFetched:   12,
			StoriesCreated:   2,
			StoriesUpdated:   1,
		},
		lastSuccess: &domain.IngestRun{
			RunID:      "export-test-run",
			Origin:     "github-backup",
			StartedAt:  now.Add(-5 * time.Minute),
			FinishedAt: &endedAt,
			Status:     domain.IngestRunOK,
		},
	}

	r := exportTestRunner(t, cfg, runsRepo)
	result := r.RunExport(context.Background(), false, cfg.DataDir, "homepage.json", cfg.HugoContentDir)
	assert.NoError(t, result.Error)

	// Heartbeat file should be updated with the re-projected run.
	hbPath := cfg.HeartbeatFile
	data, err := os.ReadFile(hbPath) //nolint:gosec
	require.NoError(t, err, "heartbeat.json should exist after export")
	var hb domain.Heartbeat
	require.NoError(t, json.Unmarshal(data, &hb))
	assert.Equal(t, "export-test-run", hb.RunID, "heartbeat RunID should be the re-projected DB run")
	assert.Equal(t, domain.HeartbeatStatusOK, hb.Status)
	assert.NotNil(t, hb.PublishedAt, "PublishedAt should be set for successful runs")

	// Feeds file should exist with the re-projected summary.
	feedsPath := cfg.FeedsFile
	data, err = os.ReadFile(feedsPath) //nolint:gosec
	require.NoError(t, err, "feeds.json should exist after export")
	var feeds domain.FeedHealth
	require.NoError(t, json.Unmarshal(data, &feeds))
	assert.Equal(t, domain.FeedHealthVersion, feeds.Version)
	assert.Equal(t, "export-test-run", feeds.RunID)
	assert.Equal(t, domain.RunStatusOK, feeds.RunStatus)
	assert.Equal(t, 12, feeds.Summary.Entries)
	assert.Equal(t, 2, feeds.Summary.New)
	assert.Equal(t, 4, feeds.Summary.Configured)
	assert.Equal(t, 4, feeds.Summary.Processed)
}

// TestExportPublishSiteDataAutoInProduction verifies that in production
// (auto mode), RunExport does NOT write site-data files (issue #163 / AC-8).
func TestExportPublishSiteDataAutoInProduction(t *testing.T) {
	t.Parallel()

	cfg := exportTestConfig(t, "", "production") // auto → production → skip
	// Production config validation requires Turso credentials.
	cfg.TursoDatabaseURL = "libsql://test.db"
	cfg.TursoAuthToken = "test-token"
	cfg.StorageBackend = "turso"
	now := time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC)
	endedAt := now

	runsRepo := &exportFakeIngestRunRepo{
		lastAttempt: &domain.IngestRun{
			RunID:            "prod-run",
			Origin:           "vps",
			StartedAt:        now.Add(-5 * time.Minute),
			FinishedAt:       &endedAt,
			Status:           domain.IngestRunOK,
			SourcesChecked:   1,
			SourcesSucceeded: 1,
			EntriesFetched:   5,
			StoriesCreated:   1,
		},
		lastSuccess: &domain.IngestRun{
			RunID:      "prod-run",
			Origin:     "vps",
			StartedAt:  now.Add(-5 * time.Minute),
			FinishedAt: &endedAt,
			Status:     domain.IngestRunOK,
		},
	}

	// With turso backend, the newRepository will try to connect to Turso.
	// We override it to return a memory repository instead.
	var out, errOut bytes.Buffer
	r := NewRunner(
		WithOutput(&out),
		WithErrorOutput(&errOut),
		WithLoadConfig(func(path string, _ config.Overrides) (config.Config, error) { return cfg, nil }),
		WithNewRepository(func(cfg config.StorageConfig) (domain.Repository, error) {
			return storage.NewMemoryRepository(), nil
		}),
		WithNewIngestRunRepo(func(cfg config.StorageConfig) (domain.IngestRunRepository, error) {
			return runsRepo, nil
		}),
		WithNewApp(func(opts []application.Option) *application.Application {
			return application.New(opts...)
		}),
		WithNewPublisher(func() *publish.LogPublisher {
			return publish.NewLogPublisher()
		}),
		WithNewFetcher(func(source domain.FeedSource, opts ...rss.Option) domain.FeedFetcher {
			return fastFetcher{entry: domain.FeedEntry{
				Title:       "Test story",
				Source:      source.ID,
				PublishedAt: time.Now().UTC().Add(-time.Hour),
				Summary:     "A test summary",
				URL:         "https://example.invalid/story-1",
				GUID:        "guid-1",
			}}
		}),
		WithNow(func() time.Time {
			return time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
		}),
	)

	result := r.RunExport(context.Background(), false, cfg.DataDir, "homepage.json", cfg.HugoContentDir)
	assert.NoError(t, result.Error)

	// In production (auto), site-data files should NOT be written.
	if _, err := os.Stat(cfg.HeartbeatFile); !os.IsNotExist(err) {
		t.Error("expected heartbeat.json to NOT exist in production (auto mode)")
	}
	if _, err := os.Stat(cfg.FeedsFile); !os.IsNotExist(err) {
		t.Error("expected feeds.json to NOT exist in production (auto mode)")
	}
}

// TestExportPublishSiteDataNoRuns verifies that export with no recorded ingest
// runs logs a skip message and does not write files (issue #163 / AC-8).
func TestExportPublishSiteDataNoRuns(t *testing.T) {
	t.Parallel()

	cfg := exportTestConfig(t, "true", "dev")

	// Empty ingest-run repo (no runs recorded).
	runsRepo := &exportFakeIngestRunRepo{}

	r := exportTestRunner(t, cfg, runsRepo)
	result := r.RunExport(context.Background(), false, cfg.DataDir, "homepage.json", cfg.HugoContentDir)
	assert.NoError(t, result.Error)

	// No runs → no site-data files written.
	if _, err := os.Stat(cfg.HeartbeatFile); !os.IsNotExist(err) {
		t.Error("expected heartbeat.json to NOT exist when no runs recorded")
	}
	if _, err := os.Stat(cfg.FeedsFile); !os.IsNotExist(err) {
		t.Error("expected feeds.json to NOT exist when no runs recorded")
	}
}

// TestExportPublishSiteDataFailedRun verifies that export with a failed
// ingest run still publishes site-data (with the failed status) so the
// dashboard can see the failure (issue #163 / AC-8).
func TestExportPublishSiteDataFailedRun(t *testing.T) {
	t.Parallel()

	cfg := exportTestConfig(t, "true", "dev")
	now := time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC)
	endedAt := now

	runsRepo := &exportFakeIngestRunRepo{
		lastAttempt: &domain.IngestRun{
			RunID:            "failed-run",
			Origin:           "vps",
			StartedAt:        now.Add(-5 * time.Minute),
			FinishedAt:       &endedAt,
			Status:           domain.IngestRunFailed,
			SourcesChecked:   4,
			SourcesSucceeded: 2,
			EntriesFetched:   3,
			StoriesCreated:   0,
			StoriesUpdated:   0,
			ErrorSummary:     "2 of 4 sources failed",
		},
		// lastSuccess is nil (the only run failed).
	}

	r := exportTestRunner(t, cfg, runsRepo)
	result := r.RunExport(context.Background(), false, cfg.DataDir, "homepage.json", cfg.HugoContentDir)
	assert.NoError(t, result.Error)

	// Heartbeat should reflect the failed run.
	hbPath := cfg.HeartbeatFile
	data, err := os.ReadFile(hbPath) //nolint:gosec
	require.NoError(t, err, "heartbeat.json should exist even for failed runs")
	var hb domain.Heartbeat
	require.NoError(t, json.Unmarshal(data, &hb))
	assert.Equal(t, "failed-run", hb.RunID)
	assert.Equal(t, domain.HeartbeatStatusFailed, hb.Status)
	// PublishedAt should NOT be set for failed runs.
	assert.Nil(t, hb.PublishedAt, "PublishedAt should be nil for failed runs")

	// Feeds should show the failed status and the failed count.
	feedsPath := cfg.FeedsFile
	data, err = os.ReadFile(feedsPath) //nolint:gosec
	require.NoError(t, err, "feeds.json should exist even for failed runs")
	var feeds domain.FeedHealth
	require.NoError(t, json.Unmarshal(data, &feeds))
	assert.Equal(t, domain.RunStatusFailed, feeds.RunStatus)
	assert.Equal(t, 2, feeds.Summary.Failed, "failed count should be sources_checked - sources_succeeded")
	assert.Equal(t, 3, feeds.Summary.Entries)
	assert.Equal(t, 0, feeds.Summary.New)
}

// TestExportSiteDataDBError verifies that a DB error during re-projection is
// non-fatal (export continues, files are not written, stderr mentions the
// error) (issue #163 / AC-8).
func TestExportSiteDataDBError(t *testing.T) {
	t.Parallel()

	cfg := exportTestConfig(t, "true", "dev")

	runsRepo := &exportFakeIngestRunRepo{
		err: os.ErrNotExist, // simulate a DB error
	}

	var errOut bytes.Buffer
	r := NewRunner(
		WithOutput(&bytes.Buffer{}),
		WithErrorOutput(&errOut),
		WithLoadConfig(func(path string, _ config.Overrides) (config.Config, error) { return cfg, nil }),
		WithNewRepository(func(cfg config.StorageConfig) (domain.Repository, error) {
			return storage.NewMemoryRepository(), nil
		}),
		WithNewIngestRunRepo(func(cfg config.StorageConfig) (domain.IngestRunRepository, error) {
			return runsRepo, nil
		}),
		WithNewApp(func(opts []application.Option) *application.Application {
			return application.New(opts...)
		}),
		WithNewPublisher(func() *publish.LogPublisher {
			return publish.NewLogPublisher()
		}),
		WithNewFetcher(func(source domain.FeedSource, opts ...rss.Option) domain.FeedFetcher {
			return fastFetcher{entry: domain.FeedEntry{
				Title:       "Test story",
				Source:      source.ID,
				PublishedAt: time.Now().UTC().Add(-time.Hour),
				Summary:     "A test summary",
				URL:         "https://example.invalid/story-1",
				GUID:        "guid-1",
			}}
		}),
		WithNow(func() time.Time {
			return time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
		}),
	)

	result := r.RunExport(context.Background(), false, cfg.DataDir, "homepage.json", cfg.HugoContentDir)
	// Export should NOT fail on a site-data re-projection error.
	assert.NoError(t, result.Error)

	// No site-data files should be written.
	if _, err := os.Stat(cfg.HeartbeatFile); !os.IsNotExist(err) {
		t.Error("expected heartbeat.json to NOT exist on DB error")
	}

	// The stderr should mention the error.
	errStr := errOut.String()
	assert.True(t, strings.Contains(errStr, "site-data"), "stderr should mention site-data, got: %s", errStr)
}

// TestExportPublishSiteDataPartialRun verifies that export with a partial
// ingest run publishes site-data with partial status (issue #163 / AC-8).
func TestExportPublishSiteDataPartialRun(t *testing.T) {
	t.Parallel()

	cfg := exportTestConfig(t, "true", "dev")
	now := time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC)
	endedAt := now

	runsRepo := &exportFakeIngestRunRepo{
		lastAttempt: &domain.IngestRun{
			RunID:            "partial-run",
			Origin:           "github-backup",
			StartedAt:        now.Add(-5 * time.Minute),
			FinishedAt:       &endedAt,
			Status:           domain.IngestRunPartial,
			SourcesChecked:   4,
			SourcesSucceeded: 3,
			EntriesFetched:   8,
			StoriesCreated:   1,
			StoriesUpdated:   2,
		},
		lastSuccess: &domain.IngestRun{
			RunID:      "partial-run",
			Origin:     "github-backup",
			StartedAt:  now.Add(-5 * time.Minute),
			FinishedAt: &endedAt,
			Status:     domain.IngestRunPartial,
		},
	}

	r := exportTestRunner(t, cfg, runsRepo)
	result := r.RunExport(context.Background(), false, cfg.DataDir, "homepage.json", cfg.HugoContentDir)
	assert.NoError(t, result.Error)

	// Feeds should show partial status.
	feedsPath := cfg.FeedsFile
	data, err := os.ReadFile(feedsPath) //nolint:gosec
	require.NoError(t, err)
	var feeds domain.FeedHealth
	require.NoError(t, json.Unmarshal(data, &feeds))
	assert.Equal(t, domain.RunStatusPartial, feeds.RunStatus)
	assert.Equal(t, 1, feeds.Summary.Failed)
	assert.Equal(t, 8, feeds.Summary.Entries)
}
