package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/heartbeat"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/publish"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/rss"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage"
	"github.com/HuddleScope/huddlescope.github.io/internal/application"
	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// TestRunIngestWithNoSources tests that ingest fails fast with no sources
func TestRunIngestWithNoSources(t *testing.T) {
	var out, errOut bytes.Buffer
	r := NewRunner(
		WithOutput(&out),
		WithErrorOutput(&errOut),
		WithLoadConfig(func(path string) (config.Config, error) {
			return config.Config{}, nil
		}),
		WithNewRepository(func(cfg config.StorageConfig) (domain.Repository, error) {
			return storage.NewMemoryRepository(), nil
		}),
		WithNewApp(func(opts []application.Option) *application.Application {
			return application.New(opts...)
		}),
		WithNewPublisher(func() *publish.LogPublisher {
			return publish.NewLogPublisher()
		}),
		WithNewFetcher(func(source domain.FeedSource, opts ...rss.Option) domain.FeedFetcher {
			return rss.NewHTTPFetcher(source, opts...)
		}),
	)

	result := r.RunIngest(context.Background(), "primary", 75*time.Minute, false)
	assert.NotNil(t, result)
	assert.Error(t, result.Error)
	assert.Equal(t, 1, result.ExitCode)
}

// TestRunIngestConfigValidationError tests config validation failure
func TestRunIngestConfigValidationError(t *testing.T) {
	var out, errOut bytes.Buffer
	r := NewRunner(
		WithOutput(&out),
		WithErrorOutput(&errOut),
		WithLoadConfig(func(path string) (config.Config, error) {
			return config.Config{}, fmt.Errorf("invalid config")
		}),
	)

	result := r.RunIngest(context.Background(), "primary", 75*time.Minute, false)
	assert.NotNil(t, result)
	assert.Error(t, result.Error)
	assert.Equal(t, 1, result.ExitCode)
}

// TestRunIngestRepositoryError tests repository creation failure
func TestRunIngestRepositoryError(t *testing.T) {
	var out, errOut bytes.Buffer
	r := NewRunner(
		WithOutput(&out),
		WithErrorOutput(&errOut),
		WithLoadConfig(func(path string) (config.Config, error) {
			return config.Config{}, nil
		}),
		WithNewRepository(func(cfg config.StorageConfig) (domain.Repository, error) {
			return nil, fmt.Errorf("repository error")
		}),
	)

	result := r.RunIngest(context.Background(), "primary", 75*time.Minute, false)
	assert.NotNil(t, result)
	assert.Error(t, result.Error)
	assert.Equal(t, 1, result.ExitCode)
}

// TestRunIngestTeamsError tests teams loading failure
func TestRunIngestTeamsError(t *testing.T) {
	var out, errOut bytes.Buffer
	r := NewRunner(
		WithOutput(&out),
		WithErrorOutput(&errOut),
		WithLoadConfig(func(path string) (config.Config, error) {
			return config.Config{
				Sources: []config.Source{},
			}, nil
		}),
		WithNewRepository(func(cfg config.StorageConfig) (domain.Repository, error) {
			return storage.NewMemoryRepository(), nil
		}),
		WithNewApp(func(opts []application.Option) *application.Application {
			return application.New(opts...)
		}),
		WithNewPublisher(func() *publish.LogPublisher {
			return publish.NewLogPublisher()
		}),
		WithNewFetcher(func(source domain.FeedSource, opts ...rss.Option) domain.FeedFetcher {
			return rss.NewHTTPFetcher(source, opts...)
		}),
	)

	result := r.RunIngest(context.Background(), "primary", 75*time.Minute, false)
	assert.NotNil(t, result)
	assert.Error(t, result.Error)
	assert.Equal(t, 1, result.ExitCode)
}

// TestRunExportWithNoSources tests export with no sources
func TestRunExportWithNoSources(t *testing.T) {
	var out, errOut bytes.Buffer
	r := NewRunner(
		WithOutput(&out),
		WithErrorOutput(&errOut),
		WithLoadConfig(func(path string) (config.Config, error) {
			return config.Config{
				Sources: []config.Source{},
			}, nil
		}),
		WithNewRepository(func(cfg config.StorageConfig) (domain.Repository, error) {
			return storage.NewMemoryRepository(), nil
		}),
		WithNewApp(func(opts []application.Option) *application.Application {
			return application.New(opts...)
		}),
		WithNewPublisher(func() *publish.LogPublisher {
			return publish.NewLogPublisher()
		}),
		WithNewFetcher(func(source domain.FeedSource, opts ...rss.Option) domain.FeedFetcher {
			return rss.NewHTTPFetcher(source, opts...)
		}),
	)

	result := r.RunExport(context.Background(), false, "", "", "")
	assert.NotNil(t, result)
	assert.NotNil(t, result.ExitCode)
}

// TestRunExportConfigValidationError tests config validation failure in export
func TestRunExportConfigValidationError(t *testing.T) {
	var out, errOut bytes.Buffer
	r := NewRunner(
		WithOutput(&out),
		WithErrorOutput(&errOut),
		WithLoadConfig(func(path string) (config.Config, error) {
			return config.Config{}, fmt.Errorf("invalid config")
		}),
	)

	result := r.RunExport(context.Background(), false, "", "", "")
	assert.NotNil(t, result)
	assert.Error(t, result.Error)
	assert.Equal(t, 1, result.ExitCode)
}

// TestRunExportDryRunValidationFailure tests dry-run with validation error
func TestRunExportDryRunValidationFailure(t *testing.T) {
	var out, errOut bytes.Buffer
	r := NewRunner(
		WithOutput(&out),
		WithErrorOutput(&errOut),
		WithLoadConfig(func(path string) (config.Config, error) {
			return config.Config{}, fmt.Errorf("invalid config")
		}),
	)

	result := r.RunExport(context.Background(), true, "", "", "")
	assert.NotNil(t, result)
	assert.Error(t, result.Error)
	assert.Equal(t, 1, result.ExitCode)
}

// TestRunExportRunFailure tests export run with error
func TestRunExportRunFailure(t *testing.T) {
	var out, errOut bytes.Buffer
	r := NewRunner(
		WithOutput(&out),
		WithErrorOutput(&errOut),
		WithLoadConfig(func(path string) (config.Config, error) {
			return config.Config{}, fmt.Errorf("load config error")
		}),
	)

	result := r.RunExport(context.Background(), false, "", "", "")
	assert.NotNil(t, result)
	assert.Error(t, result.Error)
	assert.Equal(t, 1, result.ExitCode)
}

// TestRunExportRepositoryError tests repository creation failure in export
func TestRunExportRepositoryError(t *testing.T) {
	var out, errOut bytes.Buffer
	r := NewRunner(
		WithOutput(&out),
		WithErrorOutput(&errOut),
		WithLoadConfig(func(path string) (config.Config, error) {
			return config.Config{
				Sources: []config.Source{},
			}, nil
		}),
		WithNewRepository(func(cfg config.StorageConfig) (domain.Repository, error) {
			return nil, fmt.Errorf("repository error")
		}),
		WithNewApp(func(opts []application.Option) *application.Application {
			return application.New(opts...)
		}),
		WithNewPublisher(func() *publish.LogPublisher {
			return publish.NewLogPublisher()
		}),
		WithNewFetcher(func(source domain.FeedSource, opts ...rss.Option) domain.FeedFetcher {
			return rss.NewHTTPFetcher(source, opts...)
		}),
	)

	result := r.RunExport(context.Background(), false, "", "", "")
	assert.NotNil(t, result)
	assert.Error(t, result.Error)
	assert.Equal(t, 1, result.ExitCode)
}

// TestCheckHeartbeatNotFound tests heartbeat check with missing file
func TestCheckHeartbeatNotFound(t *testing.T) {
	var out, errOut bytes.Buffer
	r := NewRunner(
		WithOutput(&out),
		WithErrorOutput(&errOut),
	)

	result := r.CheckHeartbeat(context.Background(), "nonexistent.json", 90*time.Minute)
	assert.NotNil(t, result)
	assert.NoError(t, result.Error)
	assert.Equal(t, 0, result.ExitCode)
}

// TestCheckHeartbeatInvalidJSON tests heartbeat check with invalid JSON
func TestCheckHeartbeatInvalidJSON(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "runner-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	heartbeatPath := filepath.Join(tmpDir, "heartbeat.json")
	require.NoError(t, os.WriteFile(heartbeatPath, []byte("invalid json{"), 0644))

	var out, errOut bytes.Buffer
	r := NewRunner(
		WithOutput(&out),
		WithErrorOutput(&errOut),
	)

	result := r.CheckHeartbeat(context.Background(), heartbeatPath, 90*time.Minute)
	assert.NotNil(t, result)
	assert.Error(t, result.Error)
	assert.Equal(t, 1, result.ExitCode)
}

// TestCheckHeartbeatStaleness tests heartbeat staleness detection
func TestCheckHeartbeatStaleness(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "runner-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	heartbeatPath := filepath.Join(tmpDir, "heartbeat.json")
	hb := domain.NewHeartbeat("test-run")
	hb.AttemptedAt = time.Now().Add(-2 * time.Hour).Format(time.RFC3339)
	hb.IngestedAt = &hb.AttemptedAt
	hb.PublishedAt = &hb.AttemptedAt
	hb.Status = domain.HeartbeatStatusOK

	publisher := heartbeat.NewFilePublisher(heartbeatPath)
	require.NoError(t, publisher.Publish(context.Background(), hb))

	var out, errOut bytes.Buffer
	r := NewRunner(
		WithOutput(&out),
		WithErrorOutput(&errOut),
	)

	result := r.CheckHeartbeat(context.Background(), heartbeatPath, 5*time.Minute)
	assert.NotNil(t, result)
	assert.Error(t, result.Error)
	assert.Equal(t, 1, result.ExitCode)
	assert.True(t, result.Stale)
}

// TestCheckHeartbeatSummaryOutput tests that heartbeat check prints summary (legacy format)
func TestCheckHeartbeatSummaryOutput(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "runner-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	heartbeatPath := filepath.Join(tmpDir, "heartbeat.json")
	hb := domain.NewHeartbeat("test-run-123")
	now := time.Now().UTC()
	hb.AttemptedAt = now.Format(time.RFC3339)
	ingestedAt := now.Add(-1 * time.Minute).Format(time.RFC3339)
	hb.IngestedAt = &ingestedAt
	hb.PublishedAt = nil // No published_at - will be stale
	hb.Status = domain.HeartbeatStatusOK
	hb.SourcesChecked = 10
	hb.SourcesFailed = 1
	hb.Errors = []string{"error 1", "error 2"}

	publisher := heartbeat.NewFilePublisher(heartbeatPath)
	require.NoError(t, publisher.Publish(context.Background(), hb))

	var out, errOut bytes.Buffer
	r := NewRunner(
		WithOutput(&out),
		WithErrorOutput(&errOut),
		WithNow(func() time.Time {
			return now
		}),
	)

	result := r.CheckHeartbeat(context.Background(), heartbeatPath, 90*time.Minute)
	assert.NotNil(t, result)
	// Heartbeat is stale because PublishedAt is nil
	assert.True(t, result.Stale)
	assert.Equal(t, 1, result.ExitCode)

	// Verify summary output (legacy format)
	output := out.String()
	assert.Contains(t, output, "Heartbeat status:")
	assert.Contains(t, output, "Version: 1")
	assert.Contains(t, output, "Run ID: test-run-123")
	assert.Contains(t, output, "Status: ok")
	assert.Contains(t, output, "Attempted at:")
	assert.Contains(t, output, "Ingested at:")
	assert.Contains(t, output, "Published at: (none)")
	assert.Contains(t, output, "Sources checked: 10")
	assert.Contains(t, output, "Sources failed: 1")
	assert.Contains(t, output, "Errors: 2")
	assert.Contains(t, output, "Stale threshold: 1h30m0s")
	assert.Contains(t, output, "Stale: YES")
}

// TestCheckHeartbeatFresh tests that a fresh heartbeat (with published_at) passes
func TestCheckHeartbeatFresh(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "runner-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	heartbeatPath := filepath.Join(tmpDir, "heartbeat.json")
	hb := domain.NewHeartbeat("test-run-456")
	now := time.Now().UTC()
	hb.AttemptedAt = now.Format(time.RFC3339)
	publishedAt := now.Add(-1 * time.Minute).Format(time.RFC3339)
	hb.PublishedAt = &publishedAt
	hb.Status = domain.HeartbeatStatusOK

	publisher := heartbeat.NewFilePublisher(heartbeatPath)
	require.NoError(t, publisher.Publish(context.Background(), hb))

	var out, errOut bytes.Buffer
	r := NewRunner(
		WithOutput(&out),
		WithErrorOutput(&errOut),
		WithNow(func() time.Time {
			return now
		}),
	)

	result := r.CheckHeartbeat(context.Background(), heartbeatPath, 90*time.Minute)
	assert.NotNil(t, result)
	assert.NoError(t, result.Error)
	assert.False(t, result.Stale)
	assert.Equal(t, 0, result.ExitCode)

	// Verify summary output shows "Stale: no"
	output := out.String()
	assert.Contains(t, output, "Stale: no")
}

// TestCheckConfigTextOutput tests config check with text output
func TestCheckConfigTextOutput(t *testing.T) {
	var out, errOut bytes.Buffer
	r := NewRunner(
		WithOutput(&out),
		WithErrorOutput(&errOut),
		WithLoadConfig(func(path string) (config.Config, error) {
			return config.Config{
				UserAgent:           "test-agent/1.0",
				RequestTimeout:      30 * time.Second,
				HeartbeatFile:       "web/data/heartbeat.json",
				FeedsFile:           "web/data/feeds.json",
				DataDir:             "web/data",
				DataFile:            "homepage.json",
				HugoContentDir:      "web/content",
				SimilarityThreshold: 0.50,
				TemporalWindow:      48 * time.Hour,
			}, nil
		}),
	)

	result := r.CheckConfig(context.Background(), "text")
	assert.NotNil(t, result)
	assert.NoError(t, result.Error)
	assert.Equal(t, 0, result.ExitCode)
	assert.NotEmpty(t, out.String())
}

// TestCheckConfigJSONOutput tests config check with JSON output
func TestCheckConfigJSONOutput(t *testing.T) {
	var out, errOut bytes.Buffer
	r := NewRunner(
		WithOutput(&out),
		WithErrorOutput(&errOut),
		WithLoadConfig(func(path string) (config.Config, error) {
			return config.Config{
				UserAgent:           "test-agent/1.0",
				RequestTimeout:      30 * time.Second,
				HeartbeatFile:       "web/data/heartbeat.json",
				FeedsFile:           "web/data/feeds.json",
				DataDir:             "web/data",
				DataFile:            "homepage.json",
				HugoContentDir:      "web/content",
				SimilarityThreshold: 0.50,
				TemporalWindow:      48 * time.Hour,
			}, nil
		}),
	)

	result := r.CheckConfig(context.Background(), "json")
	assert.NotNil(t, result)
	assert.NoError(t, result.Error)
	assert.Equal(t, 0, result.ExitCode)

	output := out.String()
	assert.NotEmpty(t, output)
	var parsed map[string]interface{}
	err := json.Unmarshal([]byte(output), &parsed)
	assert.NoError(t, err)
}

// TestCheckConfigValidationError tests config validation failure in config check
func TestCheckConfigValidationError(t *testing.T) {
	var out, errOut bytes.Buffer
	r := NewRunner(
		WithOutput(&out),
		WithErrorOutput(&errOut),
		WithLoadConfig(func(path string) (config.Config, error) {
			return config.Config{}, fmt.Errorf("invalid config")
		}),
	)

	result := r.CheckConfig(context.Background(), "text")
	assert.NotNil(t, result)
	assert.Error(t, result.Error)
	assert.Equal(t, 1, result.ExitCode)
}

// TestShowVersion tests version display
func TestShowVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	r := NewRunner(
		WithOutput(&out),
		WithErrorOutput(&errOut),
	)

	result := r.ShowVersion(context.Background())
	assert.NotNil(t, result)
	assert.NoError(t, result.Error)
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "0.3.0", result.Version)

	output := out.String()
	assert.Contains(t, output, "huddlescope version")
	assert.Contains(t, output, "0.3.0")
}

// TestRunIngestResultFields tests RunIngestResult fields
func TestRunIngestResultFields(t *testing.T) {
	result := &RunIngestResult{
		RunResult: RunResult{
			ExitCode: 0,
			Error:    nil,
		},
	}
	assert.NotNil(t, result.RunResult)
	assert.Nil(t, result.Result)
	assert.Empty(t, result.Summary)
}

// TestRunExportResultFields tests RunExportResult fields
func TestRunExportResultFields(t *testing.T) {
	result := &RunExportResult{
		RunResult: RunResult{
			ExitCode: 0,
			Error:    nil,
		},
	}
	assert.NotNil(t, result.RunResult)
	assert.Equal(t, application.SnapshotResult{}, result.SnapshotResult)
}

// TestCheckHeartbeatResultFields tests CheckHeartbeatResult fields
func TestCheckHeartbeatResultFields(t *testing.T) {
	result := &CheckHeartbeatResult{
		RunResult: RunResult{
			ExitCode: 0,
			Error:    nil,
		},
	}
	assert.NotNil(t, result.RunResult)
}

// TestCheckConfigResultFields tests CheckConfigResult fields
func TestCheckConfigResultFields(t *testing.T) {
	result := &CheckConfigResult{
		RunResult: RunResult{
			ExitCode: 0,
			Error:    nil,
		},
	}
	assert.NotNil(t, result.RunResult)
}

// TestVersionResultFields tests VersionResult fields
func TestVersionResultFields(t *testing.T) {
	result := &VersionResult{
		RunResult: RunResult{
			ExitCode: 0,
			Error:    nil,
		},
	}
	assert.NotNil(t, result.RunResult)
}

// TestRunnerDefaultDependencies tests default dependencies
func TestRunnerDefaultDependencies(t *testing.T) {
	r := NewRunner()
	assert.NotNil(t, r)
	assert.NotNil(t, r.loadConfig)
	assert.NotNil(t, r.newRepository)
	assert.NotNil(t, r.newApp)
	assert.NotNil(t, r.newPublisher)
	assert.NotNil(t, r.newFetcher)
	assert.NotNil(t, r.now)
	assert.NotNil(t, r.loadHeartbeat)
}

// TestRunnerOutputWriters tests output writer injection
func TestRunnerOutputWriters(t *testing.T) {
	var out, errOut bytes.Buffer
	r := NewRunner(
		WithOutput(&out),
		WithErrorOutput(&errOut),
	)
	assert.Equal(t, &out, r.out)
	assert.Equal(t, &errOut, r.err)
}

// TestBuildIngestSummaryComplete tests buildIngestSummary with complete data
func TestBuildIngestSummaryComplete(t *testing.T) {
	runRecord := &domain.RunRecord{
		RunID:           domain.RunID("test-run-id-123"),
		StartedAt:       time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		EndedAt:         func() *time.Time { t := time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC); return &t }(),
		Status:          domain.RunStatusOK,
		LastPublishedAt: func() *time.Time { t := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC); return &t }(),
		Sources:         []domain.SourceRunStatus{},
		EntriesFetched:  50,
		StoriesCreated:  10,
		StoriesUpdated:  5,
		StoriesSkipped:  35,
		Homepage: &domain.Homepage{
			RankedStories: []domain.Story{
				{ID: "story1", Title: "Story 1", Summary: "Summary 1", PublishedAt: time.Now()},
			},
			Notes: []string{"Note 1"},
		},
		GroupsCreated:   3,
		ArticlesGrouped: 15,
		ArticlesSolo:    20,
		Committed:       true,
		Errors:          []error{},
	}

	summary := buildIngestSummary(runRecord)
	assert.Contains(t, summary, "Run ID:")
	assert.Contains(t, summary, "Started at:")
	assert.Contains(t, summary, "Ended at:")
	assert.Contains(t, summary, "Status:")
	assert.Contains(t, summary, "Homepage committed successfully")
}

// TestBuildIngestSummaryNoCommitted tests buildIngestSummary when not committed
func TestBuildIngestSummaryNoCommitted(t *testing.T) {
	runRecord := &domain.RunRecord{
		RunID:           domain.RunID("test-run-id-123"),
		StartedAt:       time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		EndedAt:         func() *time.Time { t := time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC); return &t }(),
		Status:          domain.RunStatusPartial,
		LastPublishedAt: func() *time.Time { t := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC); return &t }(),
		Sources:         []domain.SourceRunStatus{},
		EntriesFetched:  50,
		StoriesCreated:  10,
		StoriesUpdated:  5,
		StoriesSkipped:  35,
		Homepage: &domain.Homepage{
			RankedStories: []domain.Story{
				{ID: "story1", Title: "Story 1", Summary: "Summary 1", PublishedAt: time.Now()},
			},
			Notes: []string{"Note 1"},
		},
		GroupsCreated:   3,
		ArticlesGrouped: 15,
		ArticlesSolo:    20,
		Committed:       false,
		Errors:          []error{},
	}

	summary := buildIngestSummary(runRecord)
	assert.Contains(t, summary, "Run ID:")
	assert.Contains(t, summary, "Status:")
	assert.NotContains(t, summary, "Homepage committed successfully")
}

// TestBuildIngestSummaryWithErrors tests buildIngestSummary with errors
func TestBuildIngestSummaryWithErrors(t *testing.T) {
	runRecord := &domain.RunRecord{
		RunID:           domain.RunID("test-run-id-123"),
		StartedAt:       time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		EndedAt:         func() *time.Time { t := time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC); return &t }(),
		Status:          domain.RunStatusPartial,
		LastPublishedAt: func() *time.Time { t := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC); return &t }(),
		Sources:         []domain.SourceRunStatus{},
		EntriesFetched:  50,
		StoriesCreated:  10,
		StoriesUpdated:  5,
		StoriesSkipped:  35,
		Homepage: &domain.Homepage{
			RankedStories: []domain.Story{
				{ID: "story1", Title: "Story 1", Summary: "Summary 1", PublishedAt: time.Now()},
			},
			Notes: []string{"Note 1"},
		},
		GroupsCreated:   3,
		ArticlesGrouped: 15,
		ArticlesSolo:    20,
		Committed:       false,
		Errors:          []error{fmt.Errorf("error 1"), fmt.Errorf("error 2")},
	}

	summary := buildIngestSummary(runRecord)
	assert.Contains(t, summary, "Errors:")
	assert.Contains(t, summary, "error 1")
	assert.Contains(t, summary, "error 2")
}

// TestRunIngestContextCancelled tests ingest with cancelled context
func TestRunIngestContextCancelled(t *testing.T) {
	var out, errOut bytes.Buffer
	r := NewRunner(
		WithOutput(&out),
		WithErrorOutput(&errOut),
		WithLoadConfig(func(path string) (config.Config, error) {
			return config.Config{}, nil
		}),
		WithNewRepository(func(cfg config.StorageConfig) (domain.Repository, error) {
			return storage.NewMemoryRepository(), nil
		}),
		WithNewApp(func(opts []application.Option) *application.Application {
			return application.New(opts...)
		}),
		WithNewPublisher(func() *publish.LogPublisher {
			return publish.NewLogPublisher()
		}),
		WithNewFetcher(func(source domain.FeedSource, opts ...rss.Option) domain.FeedFetcher {
			return rss.NewHTTPFetcher(source, opts...)
		}),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := r.RunIngest(ctx, "primary", 75*time.Minute, false)
	assert.NotNil(t, result)
	assert.Error(t, result.Error)
}

// TestRunExportContextCancelled tests export with cancelled context
func TestRunExportContextCancelled(t *testing.T) {
	var out, errOut bytes.Buffer
	r := NewRunner(
		WithOutput(&out),
		WithErrorOutput(&errOut),
		WithLoadConfig(func(path string) (config.Config, error) {
			return config.Config{}, nil
		}),
		WithNewRepository(func(cfg config.StorageConfig) (domain.Repository, error) {
			return storage.NewMemoryRepository(), nil
		}),
		WithNewApp(func(opts []application.Option) *application.Application {
			return application.New(opts...)
		}),
		WithNewPublisher(func() *publish.LogPublisher {
			return publish.NewLogPublisher()
		}),
		WithNewFetcher(func(source domain.FeedSource, opts ...rss.Option) domain.FeedFetcher {
			return rss.NewHTTPFetcher(source, opts...)
		}),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := r.RunExport(ctx, false, "", "", "")
	assert.NotNil(t, result)
}

// TestRunnerWithAllOptions tests runner with all options
func TestRunnerWithAllOptions(t *testing.T) {
	var out, errOut bytes.Buffer
	customFetcher := rss.NewHTTPFetcher(domain.FeedSource{ID: "test"}, rss.WithUserAgent("custom"))

	r := NewRunner(
		WithOutput(&out),
		WithErrorOutput(&errOut),
		WithLoadConfig(func(path string) (config.Config, error) {
			return config.Config{}, nil
		}),
		WithNewRepository(func(cfg config.StorageConfig) (domain.Repository, error) {
			return storage.NewMemoryRepository(), nil
		}),
		WithNewApp(func(opts []application.Option) *application.Application {
			return application.New(opts...)
		}),
		WithNewPublisher(func() *publish.LogPublisher {
			return publish.NewLogPublisher()
		}),
		WithNewFetcher(func(source domain.FeedSource, opts ...rss.Option) domain.FeedFetcher {
			return customFetcher
		}),
		WithNow(func() time.Time {
			return time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
		}),
		WithLoadHeartbeat(func(path string) (*domain.Heartbeat, bool, error) {
			return nil, false, nil
		}),
	)

	assert.NotNil(t, r)
	assert.Equal(t, &out, r.out)
	assert.Equal(t, &errOut, r.err)
	assert.NotNil(t, r.loadConfig)
	assert.NotNil(t, r.newRepository)
	assert.NotNil(t, r.newApp)
	assert.NotNil(t, r.newPublisher)
	assert.NotNil(t, r.newFetcher)
	assert.NotNil(t, r.now)
	assert.NotNil(t, r.loadHeartbeat)
}
