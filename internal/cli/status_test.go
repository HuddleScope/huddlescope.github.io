package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// fakeIngestRunRepo implements domain.IngestRunRepository for CLI tests.
type fakeIngestRunRepo struct {
	lastSuccess *domain.IngestRun
	lastAttempt *domain.IngestRun
	successErr  error
	attemptErr  error
	closed      bool
}

func (f *fakeIngestRunRepo) Create(context.Context, domain.IngestRun) error { return nil }
func (f *fakeIngestRunRepo) Finish(context.Context, domain.IngestRun) error { return nil }
func (f *fakeIngestRunRepo) LastSuccess(context.Context) (*domain.IngestRun, bool, error) {
	if f.successErr != nil {
		return nil, false, f.successErr
	}
	if f.lastSuccess == nil {
		return nil, false, nil
	}
	return f.lastSuccess, true, nil
}
func (f *fakeIngestRunRepo) LastAttempt(context.Context) (*domain.IngestRun, bool, error) {
	if f.attemptErr != nil {
		return nil, false, f.attemptErr
	}
	if f.lastAttempt == nil {
		return nil, false, nil
	}
	return f.lastAttempt, true, nil
}
func (f *fakeIngestRunRepo) Close() error { f.closed = true; return nil }

// execIngest builds the ingest command tree and runs the given sub-args,
// capturing stdout and stderr.
func execIngest(t *testing.T, d *Deps, args ...string) (string, string, error) {
	t.Helper()
	root := NewIngestCommand(d)
	root.SilenceUsage = true
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		c.SetOut(out)
		c.SetErr(errOut)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), errOut.String(), err
}

func depsWithRuns(cfg config.Config, repo domain.IngestRunRepository, now time.Time) *Deps {
	d := NewDeps()
	d.LoadConfig = func(string, config.Overrides) (config.Config, error) { return cfg, nil }
	d.NewIngestRunRepo = func(config.StorageConfig) (domain.IngestRunRepository, error) {
		return repo, nil
	}
	d.Now = func() time.Time { return now }
	return d
}

func TestIngestShouldRun_NeverSucceeded(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	repo := &fakeIngestRunRepo{} // empty
	cfg := config.Config{StorageBackend: "sqlite-file"}
	d := depsWithRuns(cfg, repo, now)

	stdout, _, err := execIngest(t, d, "should-run", "--output", "json")
	if err != nil {
		t.Fatalf("expected exit 0, got %v", err)
	}
	var payload shouldRunJSON
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("unmarshal: %v (%q)", err, stdout)
	}
	if !payload.ShouldRun || payload.Reason != domain.ReasonNeverSucceeded || payload.LastSuccessfulIngestAt != nil {
		t.Errorf("payload = %+v", payload)
	}
}

func TestIngestShouldRun_FreshJSON(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	last := now.Add(-10 * time.Minute)
	repo := &fakeIngestRunRepo{lastSuccess: &domain.IngestRun{
		RunID: "r1", Status: domain.IngestRunOK, FinishedAt: &last,
	}}
	d := depsWithRuns(config.Config{StorageBackend: "turso"}, repo, now)

	stdout, _, err := execIngest(t, d, "should-run", "--output", "json")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	var payload shouldRunJSON
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.ShouldRun || payload.Reason != domain.ReasonFresh {
		t.Errorf("payload = %+v", payload)
	}
	if payload.LastSuccessfulIngestAt == nil || *payload.LastSuccessfulIngestAt != last.Format(time.RFC3339) {
		t.Errorf("last_successful_ingest_at = %v", payload.LastSuccessfulIngestAt)
	}
}

func TestIngestShouldRun_StaleAndZeroThreshold(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	last := now.Add(-2 * time.Hour)
	repo := &fakeIngestRunRepo{lastSuccess: &domain.IngestRun{
		RunID: "r1", Status: domain.IngestRunPartial, FinishedAt: &last,
	}}
	d := depsWithRuns(config.Config{StorageBackend: "turso"}, repo, now)

	// Default 75m threshold -> stale.
	stdout, _, err := execIngest(t, d, "should-run", "--output", "json")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	var payload shouldRunJSON
	_ = json.Unmarshal([]byte(stdout), &payload)
	if !payload.ShouldRun || payload.Reason != domain.ReasonStale {
		t.Errorf("stale: payload = %+v", payload)
	}

	// --stale-after 0s forces stale even for a recent success.
	recent := now.Add(-1 * time.Minute)
	repo.lastSuccess = &domain.IngestRun{RunID: "r2", Status: domain.IngestRunOK, FinishedAt: &recent}
	stdout, _, err = execIngest(t, d, "should-run", "--stale-after", "0s", "--output", "json")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	_ = json.Unmarshal([]byte(stdout), &payload)
	if !payload.ShouldRun || payload.Reason != domain.ReasonStale {
		t.Errorf("zero-threshold: payload = %+v", payload)
	}
}

func TestIngestShouldRun_TextFormat(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	last := now.Add(-10 * time.Minute)
	repo := &fakeIngestRunRepo{lastSuccess: &domain.IngestRun{
		RunID: "r1", Status: domain.IngestRunOK, FinishedAt: &last,
	}}
	d := depsWithRuns(config.Config{StorageBackend: "turso"}, repo, now)

	stdout, _, err := execIngest(t, d, "should-run", "--output", "text")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(stdout, "Should run: NO (fresh)") {
		t.Errorf("text = %q, want 'Should run: NO (fresh)'", stdout)
	}
}

// TestIngestShouldRun_UnreachableDB_FailClosed (AC-3): a DB error means
// exit != 0, NO verdict on stdout, error present.
func TestIngestShouldRun_UnreachableDB_FailClosed(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	repo := &fakeIngestRunRepo{successErr: errUnreachableDB}
	d := depsWithRuns(config.Config{StorageBackend: "turso"}, repo, now)

	stdout, _, err := execIngest(t, d, "should-run", "--output", "json")
	if err == nil {
		t.Fatal("expected non-zero exit on DB error")
	}
	if strings.Contains(stdout, "stale") || strings.Contains(stdout, "fresh") || strings.Contains(stdout, "should_run") {
		t.Errorf("stdout leaked a verdict on DB error: %q", stdout)
	}
	if stdout != "" {
		t.Errorf("stdout should be empty on DB error: %q", stdout)
	}
}

var errUnreachableDB = &unreachableDBError{msg: "connection refused"}

type unreachableDBError struct{ msg string }

func (e *unreachableDBError) Error() string { return "query last success: " + e.msg }

// TestIngestStatus_MemoryBackend_FailsClosed (AC-3): memory backend ->
// exit != 0 with a "not a shared source of truth" message.
func TestIngestStatus_MemoryBackend_FailsClosed(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	d := depsWithRuns(config.Config{StorageBackend: "memory"}, &fakeIngestRunRepo{}, now)

	stdout, _, err := execIngest(t, d, "status", "--output", "json")
	if err == nil {
		t.Fatal("expected non-zero exit for memory backend")
	}
	if !strings.Contains(err.Error(), "not a shared source of truth") {
		t.Errorf("error = %q, want 'not a shared source of truth'", err.Error())
	}
	if stdout != "" {
		t.Errorf("stdout should be empty: %q", stdout)
	}
}

func TestIngestStatus_JSON(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	last := now.Add(-1 * time.Hour)
	repo := &fakeIngestRunRepo{
		lastSuccess: &domain.IngestRun{
			RunID: "r1", Origin: "github-backup", StartedAt: last.Add(-5 * time.Minute),
			FinishedAt: &last, Status: domain.IngestRunOK,
			SourcesChecked: 4, SourcesSucceeded: 4, EntriesFetched: 20,
			StoriesCreated: 10, StoriesUpdated: 2,
		},
		lastAttempt: &domain.IngestRun{
			RunID: "r1", Origin: "github-backup", StartedAt: last.Add(-5 * time.Minute),
			FinishedAt: &last, Status: domain.IngestRunOK,
			SourcesChecked: 4, SourcesSucceeded: 4, EntriesFetched: 20,
		},
	}
	d := depsWithRuns(config.Config{StorageBackend: "turso"}, repo, now)

	stdout, _, err := execIngest(t, d, "status", "--output", "json")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("unmarshal: %v (%q)", err, stdout)
	}
	for _, key := range []string{"last_attempt", "last_success", "age", "state", "execution_source"} {
		if _, ok := payload[key]; !ok {
			t.Errorf("missing key %q in status JSON", key)
		}
	}
	if payload["state"] != "healthy" {
		t.Errorf("state = %v, want healthy", payload["state"])
	}
	if payload["execution_source"] != "github-backup" {
		t.Errorf("execution_source = %v", payload["execution_source"])
	}
	if payload["age"] == "" {
		t.Error("age should be set")
	}
}

func TestIngestStatus_TextAndEmpty(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	// Empty DB -> no runs recorded, exit 0.
	d := depsWithRuns(config.Config{StorageBackend: "sqlite-file"}, &fakeIngestRunRepo{}, now)
	stdout, _, err := execIngest(t, d, "status", "--output", "text")
	if err != nil {
		t.Fatalf("empty status err: %v", err)
	}
	if !strings.Contains(stdout, "no runs recorded") {
		t.Errorf("empty text = %q", stdout)
	}

	// With a success -> text renders it.
	last := now.Add(-30 * time.Minute)
	repo := &fakeIngestRunRepo{
		lastSuccess: &domain.IngestRun{RunID: "r1", Origin: "vps", StartedAt: last.Add(-5 * time.Minute), FinishedAt: &last, Status: domain.IngestRunOK},
		lastAttempt: &domain.IngestRun{RunID: "r1", Origin: "vps", StartedAt: last.Add(-5 * time.Minute), FinishedAt: &last, Status: domain.IngestRunOK, SourcesChecked: 2, SourcesSucceeded: 2, EntriesFetched: 8},
	}
	d = depsWithRuns(config.Config{StorageBackend: "turso"}, repo, now)
	stdout, _, err = execIngest(t, d, "status", "--output", "text")
	if err != nil {
		t.Fatalf("status err: %v", err)
	}
	if !strings.Contains(stdout, "Last success:") || !strings.Contains(stdout, "status=ok") {
		t.Errorf("text = %q", stdout)
	}
}

// TestIngestStatus_DBError (AC-4): DB error -> exit != 0, no verdict.
func TestIngestStatus_DBError(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	repo := &fakeIngestRunRepo{attemptErr: errUnreachableDB}
	d := depsWithRuns(config.Config{StorageBackend: "turso"}, repo, now)

	stdout, _, err := execIngest(t, d, "status", "--output", "json")
	if err == nil {
		t.Fatal("expected non-zero exit on DB error")
	}
	if stdout != "" {
		t.Errorf("stdout should be empty on DB error: %q", stdout)
	}
}

// Ensure the cobra import is used even if the tree-shaking removes execIngest
// in some build tags.
var _ = cobra.Command{}
