package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
// issue #158 (HS-PORT-02): portable configuration & runtime behavior tests.
// ---------------------------------------------------------------------------

// portableTestRunner wires a runner with a successful single-source pipeline
// (mirroring leaseTestRunner) but with a caller-supplied config and captured
// out/err buffers so tests can assert on the memory warning, site-data gating,
// and the JSON run log.
func portableTestRunner(t *testing.T, cfg config.Config, out, err *bytes.Buffer) *Runner {
	t.Helper()
	return NewRunner(
		WithOutput(out),
		WithErrorOutput(err),
		WithLoadConfig(func(path string, _ config.Overrides) (config.Config, error) { return cfg, nil }),
		WithNewRepository(func(config.StorageConfig) (domain.Repository, error) {
			return storage.NewMemoryRepository(), nil
		}),
		WithNewRuleRepo(func(config.StorageConfig) (domain.CorrectionRuleRepository, error) {
			return nil, fmt.Errorf("no rules in test")
		}),
		WithNewIngestRunRepo(func(config.StorageConfig) (domain.IngestRunRepository, error) {
			return &fakeIngestRunRepo{}, nil
		}),
		WithNewLeaseRepo(func(config.StorageConfig) (domain.LeaseRepository, error) {
			// Memory backend: no lease available (degraded path), proceed.
			return nil, storage.ErrLeaseUnavailable
		}),
		WithNewApp(func(opts []application.Option) *application.Application {
			return application.New(opts...)
		}),
		WithNewPublisher(func() *publish.LogPublisher {
			return publish.NewLogPublisher()
		}),
		WithNewFetcher(func(source domain.FeedSource, _ ...rss.Option) domain.FeedFetcher {
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

// portableTestConfig builds a valid single-source config with temp paths and
// the caller-specified Env/PublishSiteData/JSONLog.
func portableTestConfig(t *testing.T, env, publishSiteData, jsonLog string) config.Config {
	t.Helper()
	tmp := t.TempDir()
	return config.Config{
		Env:                 env,
		UserAgent:           "test/0.1",
		RequestTimeout:      30 * time.Second,
		LookbackWindow:      24 * time.Hour,
		SimilarityThreshold: 0.75,
		TemporalWindow:      48 * time.Hour,
		PublishSiteData:     publishSiteData,
		JSONLog:             jsonLog,
		HeartbeatFile:       filepath.Join(tmp, "heartbeat.json"),
		FeedsFile:           filepath.Join(tmp, "feeds.json"),
		DataDir:             tmp,
		DataFile:            "homepage.json",
		HugoContentDir:      filepath.Join(tmp, "content"),
		StorageBackend:      "memory",
		Teams:               []config.Team{{Slug: "nfc", Name: "Test Team", ShortName: "TST", Abbreviation: "TST"}},
		Sources:             []config.Source{{ID: "src-1", Name: "Test Feed", URL: "https://example.invalid/feed.xml", Enabled: true}},
	}
}

func fileExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return true
}

// TestRunIngestMemoryWarning verifies the one-line warning is emitted to stderr
// when the storage backend is memory or sqlite-file (issue #158 / AC-4).
func TestRunIngestMemoryWarning(t *testing.T) {
	cfg := portableTestConfig(t, "dev", "auto", "false")
	cfg.StorageBackend = "memory"
	var out, errBuf bytes.Buffer
	r := portableTestRunner(t, cfg, &out, &errBuf)
	res := r.RunIngest(context.Background(), "primary", 24*time.Hour, false)
	if res.Error != nil {
		t.Fatalf("RunIngest unexpected error: %v", res.Error)
	}
	if !strings.Contains(errBuf.String(), "not shared/persistent") {
		t.Errorf("stderr = %q, want memory-backend warning", errBuf.String())
	}
}

// TestRunIngestMemoryWarningSQLiteFile verifies the warning also fires for the
// sqlite-file backend (issue #158 / AC-4).
func TestRunIngestMemoryWarningSQLiteFile(t *testing.T) {
	cfg := portableTestConfig(t, "dev", "auto", "false")
	cfg.StorageBackend = "sqlite-file"
	var out, errBuf bytes.Buffer
	r := portableTestRunner(t, cfg, &out, &errBuf)
	_ = r.RunIngest(context.Background(), "primary", 24*time.Hour, false)
	if !strings.Contains(errBuf.String(), "not shared/persistent") {
		t.Errorf("stderr = %q, want sqlite-file warning", errBuf.String())
	}
}

// TestRunIngestNoMemoryWarningForTurso verifies no warning is emitted for the
// turso backend (the shared/persistent case).
func TestRunIngestNoMemoryWarningForTurso(t *testing.T) {
	cfg := portableTestConfig(t, "dev", "auto", "false")
	cfg.StorageBackend = "turso"
	var out, errBuf bytes.Buffer
	r := portableTestRunner(t, cfg, &out, &errBuf)
	_ = r.RunIngest(context.Background(), "primary", 24*time.Hour, false)
	if strings.Contains(errBuf.String(), "not shared/persistent") {
		t.Errorf("stderr = %q, want NO memory warning for turso", errBuf.String())
	}
}

// TestRunIngestSiteDataSkippedInProductionDefault verifies that with the
// default (auto) publish mode in production, the site-data files are NOT
// written (issue #158 / AC-3, AC-5).
func TestRunIngestSiteDataSkippedInProductionDefault(t *testing.T) {
	cfg := portableTestConfig(t, "production", "", "false")
	// Production requires turso; the test uses memory for the repo, but the
	// gating decision depends only on Env+PublishSiteData, so set turso to
	// satisfy Validate.
	cfg.StorageBackend = "turso"
	cfg.TursoDatabaseURL = "libsql://example.com"
	cfg.TursoAuthToken = "token"
	heartbeatPath := cfg.HeartbeatFile
	feedsPath := cfg.FeedsFile

	var out, errBuf bytes.Buffer
	r := portableTestRunner(t, cfg, &out, &errBuf)
	res := r.RunIngest(context.Background(), "primary", 24*time.Hour, false)
	if res.Error != nil {
		t.Fatalf("RunIngest unexpected error: %v (stderr=%q)", res.Error, errBuf.String())
	}
	if fileExists(t, heartbeatPath) {
		t.Errorf("heartbeat.json was written in production default, want skipped")
	}
	if fileExists(t, feedsPath) {
		t.Errorf("feeds.json was written in production default, want skipped")
	}
	if !strings.Contains(errBuf.String(), "site-data writes disabled") {
		t.Errorf("stderr = %q, want site-data-disabled note", errBuf.String())
	}
}

// TestRunIngestSiteDataWrittenWhenPublishTrue verifies that
// HSC_PUBLISH_SITE_DATA=true writes the site-data files even in production
// (issue #158 / AC-3). This is the GitHub Actions path (refresh-news sets
// HSC_PUBLISH_SITE_DATA=true so the committed files keep updating).
func TestRunIngestSiteDataWrittenWhenPublishTrue(t *testing.T) {
	cfg := portableTestConfig(t, "production", "true", "false")
	cfg.StorageBackend = "turso"
	cfg.TursoDatabaseURL = "libsql://example.com"
	cfg.TursoAuthToken = "token"
	heartbeatPath := cfg.HeartbeatFile

	var out, errBuf bytes.Buffer
	r := portableTestRunner(t, cfg, &out, &errBuf)
	res := r.RunIngest(context.Background(), "primary", 24*time.Hour, false)
	if res.Error != nil {
		t.Fatalf("RunIngest unexpected error: %v (stderr=%q)", res.Error, errBuf.String())
	}
	// heartbeat.json is the reliable site-data indicator (the feeds publisher
	// additionally validates feed-health content); its presence proves the gate
	// opened and the write happened.
	if !fileExists(t, heartbeatPath) {
		t.Errorf("heartbeat.json was NOT written with HSC_PUBLISH_SITE_DATA=true in production")
	}
}

// TestRunIngestSiteDataWrittenInDevDefault verifies that with the default
// (auto) mode in dev, the site-data files ARE written (issue #158 / AC-3) so
// normal dev usage keeps working.
func TestRunIngestSiteDataWrittenInDevDefault(t *testing.T) {
	cfg := portableTestConfig(t, "dev", "", "false")
	heartbeatPath := cfg.HeartbeatFile

	var out, errBuf bytes.Buffer
	r := portableTestRunner(t, cfg, &out, &errBuf)
	res := r.RunIngest(context.Background(), "primary", 24*time.Hour, false)
	if res.Error != nil {
		t.Fatalf("RunIngest unexpected error: %v (stderr=%q)", res.Error, errBuf.String())
	}
	if !fileExists(t, heartbeatPath) {
		t.Errorf("heartbeat.json was NOT written in dev default, want written")
	}
}

// TestRunIngestJSONLogEnabled verifies that with HSC_JSON_LOG=true, a single
// JSON line is emitted to stderr (issue #158 / AC-8).
func TestRunIngestJSONLogEnabled(t *testing.T) {
	cfg := portableTestConfig(t, "dev", "auto", "true")
	cfg.RunnerOrigin = "vps"
	var out, errBuf bytes.Buffer
	r := portableTestRunner(t, cfg, &out, &errBuf)
	res := r.RunIngest(context.Background(), "primary", 24*time.Hour, false)
	if res.Error != nil {
		t.Fatalf("RunIngest unexpected error: %v (stderr=%q)", res.Error, errBuf.String())
	}
	stderr := errBuf.String()
	// Find a line that is valid JSON and contains run_id.
	var found bool
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			continue
		}
		if _, ok := m["run_id"]; ok {
			found = true
			// Verify the secret-free field set.
			for _, k := range []string{"run_id", "origin", "status", "exit_code"} {
				if _, ok := m[k]; !ok {
					t.Errorf("JSON log line missing key %q: %s", k, line)
				}
			}
			if m["origin"] != "vps" {
				t.Errorf("JSON log origin = %v, want vps", m["origin"])
			}
		}
	}
	if !found {
		t.Errorf("stderr = %q, want a JSON run-log line with run_id", stderr)
	}
}

// TestRunIngestJSONLogDisabled verifies no JSON line is emitted when
// HSC_JSON_LOG is not set (issue #158 / AC-8).
func TestRunIngestJSONLogDisabled(t *testing.T) {
	cfg := portableTestConfig(t, "dev", "auto", "")
	var out, errBuf bytes.Buffer
	r := portableTestRunner(t, cfg, &out, &errBuf)
	_ = r.RunIngest(context.Background(), "primary", 24*time.Hour, false)
	stderr := errBuf.String()
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "{") {
			var m map[string]any
			if err := json.Unmarshal([]byte(line), &m); err == nil {
				if _, ok := m["run_id"]; ok {
					t.Errorf("JSON run-log line emitted despite HSC_JSON_LOG unset: %s", line)
				}
			}
		}
	}
}

// TestBuildJSONRunLogNoSecrets verifies the JSON run-log payload never contains
// secret fields (issue #158 / AC-8 + Security).
func TestBuildJSONRunLogNoSecrets(t *testing.T) {
	now := time.Now().UTC()
	rec := &domain.RunRecord{
		RunID:          domain.NewRunID(),
		Status:         domain.RunStatusOK,
		StartedAt:      now,
		Committed:      true,
		EntriesFetched: 5,
		StoriesCreated: 2,
		Errors:         []error{fmt.Errorf("feed A failed")},
	}
	b, err := buildJSONRunLog(rec, "vps", 0)
	if err != nil {
		t.Fatalf("buildJSONRunLog: %v", err)
	}
	s := string(b)
	// Field whitelist check.
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	allowed := map[string]bool{
		"time": true, "run_id": true, "origin": true, "command": true,
		"status": true, "exit_code": true, "committed": true,
		"entries_fetched": true, "stories_created": true, "stories_updated": true,
		"stories_skipped": true, "errors": true,
	}
	for k := range m {
		if !allowed[k] {
			t.Errorf("JSON log contains disallowed key %q", k)
		}
	}
	// Secret check: none of these substrings should appear.
	for _, secret := range []string{"TURSO_AUTH_TOKEN", "token", "dsn", "auth", "password", "secret"} {
		if strings.Contains(strings.ToLower(s), secret) {
			t.Errorf("JSON log contains secret-like substring %q: %s", secret, s)
		}
	}
}
