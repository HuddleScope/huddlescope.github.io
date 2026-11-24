package storage

import (
	"context"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

func TestMemoryIngestRunRepository(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryIngestRunRepository()
	defer func() { _ = repo.Close() }()

	// Empty: no attempts, no success.
	if _, ok, err := repo.LastAttempt(ctx); err != nil || ok {
		t.Errorf("LastAttempt on empty = ok=%v err=%v, want false/nil", ok, err)
	}
	if _, ok, err := repo.LastSuccess(ctx); err != nil || ok {
		t.Errorf("LastSuccess on empty = ok=%v err=%v, want false/nil", ok, err)
	}

	started := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	finished := started.Add(5 * time.Minute)
	run := domain.IngestRun{RunID: "mem-1", Origin: "local", StartedAt: started}

	// Create -> running.
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}
	attempt, ok, err := repo.LastAttempt(ctx)
	if err != nil || !ok || attempt.Status != domain.IngestRunRunning {
		t.Errorf("after create: %v ok=%v err=%v", attempt, ok, err)
	}

	// Finish -> ok.
	run.Status = domain.IngestRunOK
	run.FinishedAt = &finished
	run.SourcesChecked = 2
	run.SourcesSucceeded = 2
	if err := repo.Finish(ctx, run); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	success, ok, err := repo.LastSuccess(ctx)
	if err != nil || !ok || success.Status != domain.IngestRunOK {
		t.Errorf("after finish: %v ok=%v err=%v", success, ok, err)
	}
}

func TestMemoryIngestRunRepository_FailedNotSuccess(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryIngestRunRepository()
	defer func() { _ = repo.Close() }()

	finished := time.Now().UTC()
	run := domain.IngestRun{RunID: "mem-f", Origin: "local", StartedAt: finished, Status: domain.IngestRunFailed, FinishedAt: &finished}
	if err := repo.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.Finish(ctx, run); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if _, ok, _ := repo.LastSuccess(ctx); ok {
		t.Error("failed run must not be a success")
	}
}

func TestNewIngestRunRepository_Factory(t *testing.T) {
	// memory backend
	repo, err := NewIngestRunRepository(config.StorageConfig{Backend: "memory"})
	if err != nil {
		t.Fatalf("NewIngestRunRepository(memory): %v", err)
	}
	if _, ok := repo.(*MemoryIngestRunRepository); !ok {
		t.Errorf("memory backend returned %T, want *MemoryIngestRunRepository", repo)
	}
	defer func() { _ = repo.Close() }()

	// turso backend with a file: DSN (no network)
	tmpfile := t.TempDir() + "/ingest.db"
	repo, err = NewIngestRunRepository(config.StorageConfig{Backend: "turso", TursoURL: "file:" + tmpfile})
	if err != nil {
		t.Fatalf("NewIngestRunRepository(turso file:): %v", err)
	}
	defer func() { _ = repo.Close() }()

	// sqlite-file backend
	dbPath := t.TempDir() + "/ingest-file.db"
	repo, err = NewIngestRunRepository(config.StorageConfig{Backend: "sqlite-file", Path: dbPath})
	if err != nil {
		t.Fatalf("NewIngestRunRepository(sqlite-file): %v", err)
	}
	defer func() { _ = repo.Close() }()

	// unknown backend
	if _, err := NewIngestRunRepository(config.StorageConfig{Backend: "redis"}); err == nil {
		t.Error("expected error for unknown backend")
	}
}
