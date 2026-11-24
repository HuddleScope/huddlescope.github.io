package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// IngestRunRepositoryTimeout bounds each repository operation so an unresponsive
// backend (e.g., a stalled Turso) surfaces as an error instead of blocking the
// caller (AC-3: fail closed).
const IngestRunRepositoryTimeout = 30 * time.Second

// IngestRunRepository implements domain.IngestRunRepository over an existing
// *sql.DB handle (driver-agnostic: modernc sqlite locally, libsql for Turso).
// The caller owns the handle and its migrations.
type IngestRunRepository struct {
	db *sql.DB
}

// NewIngestRunRepoFromDB wraps an existing *sql.DB handle. It does not open
// or migrate the database.
func NewIngestRunRepoFromDB(db *sql.DB) *IngestRunRepository {
	return &IngestRunRepository{db: db}
}

func opCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, IngestRunRepositoryTimeout)
}

// formatTime stores timestamps as UTC RFC3339 with second precision (the
// project-wide storage format: `YYYY-MM-DDTHH:MM:SSZ`, 20 chars).
func formatTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

func parseTime(s string) (time.Time, error) {
	return time.Parse("2006-01-02T15:04:05Z", s)
}

var ingestRunColumns = `run_id, origin, started_at, finished_at, status,
	sources_checked, sources_succeeded, entries_fetched, stories_created, stories_updated, error_summary`

// scanIngestRun scans one row of ingest_runs (column order = ingestRunColumns).
func scanIngestRun(row interface{ Scan(...any) error }) (domain.IngestRun, error) {
	var (
		run        domain.IngestRun
		startedAt  string
		finishedAt sql.NullString
		status     string
		checked    int
		succeeded  int
		fetched    int
		created    int
		updated    int
		errSummary string
	)
	err := row.Scan(&run.RunID, &run.Origin, &startedAt, &finishedAt, &status,
		&checked, &succeeded, &fetched, &created, &updated, &errSummary)
	if err != nil {
		return domain.IngestRun{}, err
	}
	t, err := parseTime(startedAt)
	if err != nil {
		return domain.IngestRun{}, fmt.Errorf("parse started_at %q: %w", startedAt, err)
	}
	st, err := domain.ParseIngestRunStatus(status)
	if err != nil {
		return domain.IngestRun{}, err
	}
	run.StartedAt = t
	run.Status = st
	run.SourcesChecked = checked
	run.SourcesSucceeded = succeeded
	run.EntriesFetched = fetched
	run.StoriesCreated = created
	run.StoriesUpdated = updated
	run.ErrorSummary = errSummary
	if finishedAt.Valid && finishedAt.String != "" {
		ft, err := parseTime(finishedAt.String)
		if err != nil {
			return domain.IngestRun{}, fmt.Errorf("parse finished_at %q: %w", finishedAt.String, err)
		}
		run.FinishedAt = &ft
	}
	return run, nil
}

// Create implements domain.IngestRunRepository. The status is forced to
// running: Create registers a run that just started (AC-6). INSERT OR REPLACE
// makes duplicate run_id registration idempotent.
func (r *IngestRunRepository) Create(ctx context.Context, run domain.IngestRun) error {
	ctx, cancel := opCtx(ctx)
	defer cancel()

	_, err := r.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO ingest_runs (`+ingestRunColumns+`)
		VALUES (?, ?, ?, NULL, 'running', ?, ?, ?, ?, ?, '')`,
		run.RunID, run.Origin, formatTime(run.StartedAt),
		run.SourcesChecked, run.SourcesSucceeded, run.EntriesFetched,
		run.StoriesCreated, run.StoriesUpdated)
	if err != nil {
		return fmt.Errorf("create ingest run %s: %w", run.RunID, err)
	}
	return nil
}

// Finish implements domain.IngestRunRepository. It finalizes the run row by
// run_id with the derived status, finished_at, counters, and sanitized error
// summary. If no matching row exists (e.g., the runner process died before
// Create persisted), the row is inserted in finished form so history is not
// lost.
func (r *IngestRunRepository) Finish(ctx context.Context, run domain.IngestRun) error {
	ctx, cancel := opCtx(ctx)
	defer cancel()

	var finishedAt any
	if run.FinishedAt != nil {
		finishedAt = formatTime(*run.FinishedAt)
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE ingest_runs
		SET status = ?, finished_at = ?, sources_checked = ?, sources_succeeded = ?,
			entries_fetched = ?, stories_created = ?, stories_updated = ?, error_summary = ?
		WHERE run_id = ?`,
		run.Status.String(), finishedAt, run.SourcesChecked, run.SourcesSucceeded,
		run.EntriesFetched, run.StoriesCreated, run.StoriesUpdated, run.ErrorSummary,
		run.RunID)
	if err != nil {
		return fmt.Errorf("finish ingest run %s: %w", run.RunID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("finish ingest run %s: %w", run.RunID, err)
	}
	if n == 0 {
		// No row to update: record the finished run instead of losing it.
		_, err = r.db.ExecContext(ctx, `
			INSERT OR REPLACE INTO ingest_runs (`+ingestRunColumns+`)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			run.RunID, run.Origin, formatTime(run.StartedAt), finishedAt, run.Status.String(),
			run.SourcesChecked, run.SourcesSucceeded, run.EntriesFetched,
			run.StoriesCreated, run.StoriesUpdated, run.ErrorSummary)
		if err != nil {
			return fmt.Errorf("finish ingest run %s (insert): %w", run.RunID, err)
		}
	}
	return nil
}

// LastSuccess implements domain.IngestRunRepository (AC-2): the most recent
// run with status ok/partial and a finished_at. Abandoned running rows and
// failed runs never qualify.
func (r *IngestRunRepository) LastSuccess(ctx context.Context) (*domain.IngestRun, bool, error) {
	ctx, cancel := opCtx(ctx)
	defer cancel()

	rows, err := r.db.QueryContext(ctx, `
		SELECT `+ingestRunColumns+`
		FROM ingest_runs
		WHERE status IN ('ok', 'partial') AND finished_at IS NOT NULL AND finished_at != ''
		ORDER BY finished_at DESC, started_at DESC
		LIMIT 1`)
	if err != nil {
		return nil, false, fmt.Errorf("query last success: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, false, fmt.Errorf("query last success: %w", err)
		}
		return nil, false, nil
	}
	run, err := scanIngestRun(rows)
	if err != nil {
		return nil, false, err
	}
	return &run, true, nil
}

// LastAttempt implements domain.IngestRunRepository: the most recent run of
// any status, including abandoned running rows (ordered by
// COALESCE(finished_at, started_at) DESC).
func (r *IngestRunRepository) LastAttempt(ctx context.Context) (*domain.IngestRun, bool, error) {
	ctx, cancel := opCtx(ctx)
	defer cancel()

	rows, err := r.db.QueryContext(ctx, `
		SELECT `+ingestRunColumns+`
		FROM ingest_runs
		ORDER BY COALESCE(finished_at, started_at) DESC, started_at DESC
		LIMIT 1`)
	if err != nil {
		return nil, false, fmt.Errorf("query last attempt: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, false, fmt.Errorf("query last attempt: %w", err)
		}
		return nil, false, nil
	}
	run, err := scanIngestRun(rows)
	if err != nil {
		return nil, false, err
	}
	return &run, true, nil
}

// Close satisfies the domain.IngestRunRepository contract. The repository
// does not own the handle (it was provided by the caller), so closing the
// shared handle here would break sibling repositories; only the reference is
// released.
func (r *IngestRunRepository) Close() error {
	return nil
}
