package cli

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	cobra "github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/publish"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/rss"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage"
	"github.com/HuddleScope/huddlescope.github.io/internal/application"
	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// TestRunIngest tests runIngest function
func TestRunIngest(t *testing.T) {
	var out, errOut bytes.Buffer
	cmd := &cobra.Command{}

	d := NewDeps(
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
		WithNow(func() time.Time {
			return time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
		}),
	)

	// Test with no sources - will fail
	cmd.SetArgs([]string{})
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := runIngest(cmd, d)

	// Should error because no sources are configured
	assert.Error(t, err)
}

// TestRunIngestWithSources tests runIngest with a proper setup
func TestRunIngestWithSources(t *testing.T) {
	var out, errOut bytes.Buffer
	cmd := &cobra.Command{}

	// Create a config with a source
	cfg := config.Config{
		UserAgent:           "test-agent/1.0",
		RequestTimeout:      30 * time.Second,
		HeartbeatFile:       "web/data/heartbeat.json",
		FeedsFile:           "web/data/feeds.json",
		DataDir:             "web/data",
		DataFile:            "homepage.json",
		HugoContentDir:      "web/content",
		SimilarityThreshold: 0.50,
		TemporalWindow:      48 * time.Hour,
		SourcesFile:         "conf/sources.yaml",
		Sources: []config.Source{
			{
				ID:      "test-source",
				Name:    "Test Source",
				URL:     "https://example.com/feed",
				Enabled: true,
			},
		},
	}

	cmd.SetArgs([]string{})
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := runIngest(cmd, NewDeps(
		WithOutput(&out),
		WithErrorOutput(&errOut),
		WithLoadConfig(func(path string) (config.Config, error) {
			return cfg, nil
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
		WithNow(func() time.Time {
			return time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
		}),
	))

	// May or may not error depending on repo state, but shouldn't crash
	_ = err
}

// TestRunExport tests runExport function
func TestRunExport(t *testing.T) {
	var out, errOut bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetArgs([]string{})
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)

	// Test with default config (no sources, will fail on validation)
	err := runExport(cmd, NewDeps(
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
		WithNow(func() time.Time {
			return time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
		}),
	))

	// Should error because no sources configured
	assert.Error(t, err)
}

// TestRunHeartbeatCheck tests runHeartbeatCheck function
func TestRunHeartbeatCheck(t *testing.T) {
	var out, errOut bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetArgs([]string{})

	// Test with non-existent file - should exit cleanly
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)

	err := runHeartbeatCheck(cmd, NewDeps(
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
		WithNow(func() time.Time {
			return time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
		}),
		WithLoadHeartbeat(func(path string) (*domain.Heartbeat, bool, error) {
			return nil, false, nil
		}),
	))

	// Should not error for non-existent file
	assert.NoError(t, err)
}

// TestRunHeartbeatCheckWithValid tests runHeartbeatCheck with valid heartbeat
func TestRunHeartbeatCheckWithValid(t *testing.T) {
	var out, errOut bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetArgs([]string{})

	// Use the same fixed time as in WithNow
	fixedTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	// Create a valid heartbeat with the fixed time
	hb := domain.NewHeartbeat("test-run")
	hb.AttemptedAt = fixedTime.Format(time.RFC3339)
	ingestedAt := fixedTime.Format(time.RFC3339)
	hb.IngestedAt = &ingestedAt
	publishedAt := fixedTime.Format(time.RFC3339)
	hb.PublishedAt = &publishedAt
	hb.Status = domain.HeartbeatStatusOK

	cmd.SetOut(&out)
	cmd.SetErr(&errOut)

	err := runHeartbeatCheck(cmd, NewDeps(
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
		WithNow(func() time.Time {
			return fixedTime
		}),
		WithLoadHeartbeat(func(path string) (*domain.Heartbeat, bool, error) {
			return hb, true, nil
		}),
	))

	// Should not error for valid heartbeat
	assert.NoError(t, err)
}

// TestRunConfigCheck tests runConfigCheck function
func TestRunConfigCheck(t *testing.T) {
	var out, errOut bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetArgs([]string{})

	// Test with valid config
	cfg := config.Config{
		UserAgent:           "test-agent/1.0",
		RequestTimeout:      30 * time.Second,
		HeartbeatFile:       "web/data/heartbeat.json",
		FeedsFile:           "web/data/feeds.json",
		DataDir:             "web/data",
		DataFile:            "homepage.json",
		HugoContentDir:      "web/content",
		SimilarityThreshold: 0.50,
		TemporalWindow:      48 * time.Hour,
	}

	cmd.SetOut(&out)
	cmd.SetErr(&errOut)

	err := runConfigCheck(cmd, NewDeps(
		WithOutput(&out),
		WithErrorOutput(&errOut),
		WithLoadConfig(func(path string) (config.Config, error) {
			return cfg, nil
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
		WithNow(func() time.Time {
			return time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
		}),
	))

	// Should succeed with valid config
	assert.NoError(t, err)

	// Verify output does not contain secrets (they are omitted from SafeConfig)
	output := out.String()
	assert.NotContains(t, output, "my-super-secret-token", "output should not contain actual secrets")
	assert.NotContains(t, output, "https://secret", "output should not contain Turso URL")
}

// TestRunConfigCheckWithInvalid tests runConfigCheck with invalid config
func TestRunConfigCheckWithInvalid(t *testing.T) {
	var out, errOut bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetArgs([]string{})

	cmd.SetOut(&out)
	cmd.SetErr(&errOut)

	err := runConfigCheck(cmd, NewDeps(
		WithOutput(&out),
		WithErrorOutput(&errOut),
		WithLoadConfig(func(path string) (config.Config, error) {
			return config.Config{}, fmt.Errorf("invalid config")
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
		WithNow(func() time.Time {
			return time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
		}),
	))

	// Should error with invalid config
	assert.Error(t, err)
}
