package storage

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// MemoryIngestRunRepository is a per-host, mutex-guarded in-memory
// implementation of domain.IngestRunRepository. It exists for the `memory`
// backend and unit tests; freshness commands over it must fail closed (a
// volatile per-host store cannot make a cross-host decision, plan §1.8).
type MemoryIngestRunRepository struct {
	mu   sync.RWMutex
	runs map[string]domain.IngestRun
}

// NewMemoryIngestRunRepository creates an empty in-memory run store.
func NewMemoryIngestRunRepository() *MemoryIngestRunRepository {
	return &MemoryIngestRunRepository{runs: make(map[string]domain.IngestRun)}
}

// Create implements domain.IngestRunRepository (status forced to running).
func (r *MemoryIngestRunRepository) Create(_ context.Context, run domain.IngestRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copied := run
	copied.Status = domain.IngestRunRunning
	copied.FinishedAt = nil
	copied.ErrorSummary = ""
	r.runs[run.RunID] = copied
	return nil
}

// Finish implements domain.IngestRunRepository (upsert by run_id).
func (r *MemoryIngestRunRepository) Finish(_ context.Context, run domain.IngestRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[run.RunID] = run
	return nil
}

// LastSuccess implements domain.IngestRunRepository (AC-2): most recent run
// with status ok/partial and a FinishedAt.
func (r *MemoryIngestRunRepository) LastSuccess(_ context.Context) (*domain.IngestRun, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var best *domain.IngestRun
	for i := range r.runs {
		run := r.runs[i]
		if !run.IsSuccess() || run.FinishedAt == nil {
			continue
		}
		if best == nil || run.FinishedAt.After(*best.FinishedAt) {
			c := run
			best = &c
		}
	}
	if best == nil {
		return nil, false, nil
	}
	return best, true, nil
}

// LastAttempt implements domain.IngestRunRepository: most recent run of any
// status by COALESCE(FinishedAt, StartedAt).
func (r *MemoryIngestRunRepository) LastAttempt(_ context.Context) (*domain.IngestRun, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.runs) == 0 {
		return nil, false, nil
	}
	keys := make([]string, 0, len(r.runs))
	for k := range r.runs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var best domain.IngestRun
	var bestTS time.Time
	first := true
	for _, k := range keys {
		run := r.runs[k]
		ts := run.StartedAt
		if run.FinishedAt != nil && run.FinishedAt.After(ts) {
			ts = *run.FinishedAt
		}
		if first || ts.After(bestTS) {
			best = run
			bestTS = ts
			first = false
		}
	}
	c := best
	return &c, true, nil
}

// Close implements domain.IngestRunRepository (nothing to release).
func (r *MemoryIngestRunRepository) Close() error {
	return nil
}
