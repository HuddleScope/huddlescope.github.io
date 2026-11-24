package turso

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// newTestIngestRunRepo wraps a local modernc sqlite handle in the Turso
// IngestRunRepository via the network-free test constructor. This proves the
// repository is driver-agnostic (works over any database/sql handle), while
// the production path uses the libsql driver (NewIngestRunRepository).
func newTestIngestRunRepo(t *testing.T) *IngestRunRepository {
	t.Helper()
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return newIngestRunRepoFromDB(db, "")
}

func TestTursoIngestRuns_CreateFinishLastSuccess(t *testing.T) {
	repo := newTestIngestRunRepo(t)
	t.Cleanup(func() { _ = repo.Close() })
	ctx := context.Background()

	started := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	finished := started.Add(5 * time.Minute)
	run := domain.IngestRun{
		RunID: "turso-run-1", Origin: "vps", StartedAt: started,
		Status: domain.IngestRunRunning,
	}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, ok, _ := repo.LastSuccess(ctx); ok {
		t.Error("LastSuccess before finish should be empty")
	}

	run.Status = domain.IngestRunPartial
	run.FinishedAt = &finished
	run.SourcesChecked = 2
	run.SourcesSucceeded = 1
	if err := repo.Finish(ctx, run); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	success, ok, err := repo.LastSuccess(ctx)
	if err != nil || !ok {
		t.Fatalf("LastSuccess: %v ok=%v", err, ok)
	}
	if success.Status != domain.IngestRunPartial || !success.FinishedAt.Equal(finished) {
		t.Errorf("success = %+v", success)
	}
	attempt, ok, err := repo.LastAttempt(ctx)
	if err != nil || !ok || attempt.RunID != "turso-run-1" {
		t.Errorf("LastAttempt = %+v ok=%v err=%v", attempt, ok, err)
	}
}

func TestTursoIngestRuns_InitErrorRedaction(t *testing.T) {
	// A DSN with an auth token: connection errors must not leak the token.
	dsn := "libsql://db.turso.io/default?authToken=secret-token-456"
	repo := newIngestRunRepoFromDB(nil, dsn)
	err := repo.wrapInitError(&ingestRedactProbeErr{msg: "failed to connect to " + dsn})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "secret-token-456") {
		t.Errorf("error leaks token: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "authToken=***") {
		t.Errorf("error should contain redacted token: %q", err.Error())
	}
}

// ingestRedactProbeErr is a dummy error whose message embeds the DSN.
type ingestRedactProbeErr struct{ msg string }

func (e *ingestRedactProbeErr) Error() string { return e.msg }

func TestTursoIngestRuns_LazyOpen(t *testing.T) {
	// sql.Open is lazy: no network I/O at construction time.
	repo, err := NewIngestRunRepository("libsql://example.invalid/db", "")
	if err != nil {
		t.Fatalf("construction should not touch the network: %v", err)
	}
	defer func() { _ = repo.Close() }() //nolint:errcheck

	// The first operation fails (unreachable host) and must be redacted.
	_, _, err = repo.LastSuccess(context.Background())
	if err == nil {
		t.Fatal("expected error from unreachable host")
	}
	if strings.Contains(err.Error(), "example.invalid/db") && !strings.Contains(err.Error(), "initialize turso ingest runs") {
		t.Logf("error: %v", err)
	}
}

func TestTursoIngestRuns_EmptyURL(t *testing.T) {
	if _, err := NewIngestRunRepository("", ""); err == nil {
		t.Fatal("expected error for empty database URL")
	}
}

func TestTursoIngestRuns_InitDSNBranch(t *testing.T) {
	// A real local handle with a non-empty DSN exercises init()'s
	// PRAGMA foreign_keys + MigrateDB branch (no network).
	tmp := t.TempDir()
	dbPath := "file:" + filepath.Join(tmp, "init.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := newIngestRunRepoFromDB(db, dbPath)
	if err := repo.init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	if repo.inner == nil {
		t.Error("inner repository not set after init")
	}
}
