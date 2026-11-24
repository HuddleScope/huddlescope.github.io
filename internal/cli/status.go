package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// openIngestRuns loads config and constructs the ingest-run repository for
// the freshness commands. The memory backend is rejected: a per-host volatile
// store cannot make a cross-host freshness decision (plan §1.8, AC-3).
func openIngestRuns(d *Deps, cfgPath string) (domain.IngestRunRepository, config.Config, error) {
	cfg, err := d.LoadConfig(cfgPath)
	if err != nil {
		return nil, config.Config{}, fmt.Errorf("load config: %w", err)
	}
	if cfg.StorageBackend == "memory" || cfg.StorageBackend == "" {
		return nil, cfg, fmt.Errorf("storage backend %q is not a shared source of truth; set HSC_STORAGE_BACKEND to turso or sqlite-file", cfg.StorageBackend)
	}
	if d.NewIngestRunRepo == nil {
		return nil, cfg, fmt.Errorf("ingest-run repository constructor is not configured")
	}
	repo, err := d.NewIngestRunRepo(cfg.Storage())
	if err != nil {
		return nil, cfg, fmt.Errorf("open ingest runs repository: %w", err)
	}
	return repo, cfg, nil
}

// formatRunJSON renders an IngestRun with stable JSON field names.
func formatRunJSON(run domain.IngestRun) map[string]any {
	out := map[string]any{
		"run_id":            run.RunID,
		"origin":            run.Origin,
		"started_at":        run.StartedAt.UTC().Format(time.RFC3339),
		"finished_at":       nil,
		"status":            run.Status.String(),
		"sources_checked":   run.SourcesChecked,
		"sources_succeeded": run.SourcesSucceeded,
		"entries_fetched":   run.EntriesFetched,
		"stories_created":   run.StoriesCreated,
		"stories_updated":   run.StoriesUpdated,
	}
	if run.FinishedAt != nil {
		out["finished_at"] = run.FinishedAt.UTC().Format(time.RFC3339)
	}
	return out
}

// runIngestStatus implements `ingest status` (AC-4). Exit 0 on a correct
// query (including zero runs); non-zero on DB errors with NO verdict printed.
func runIngestStatus(cmd *cobra.Command, d *Deps, output string) error {
	repo, cfg, err := openIngestRuns(d, "")
	if err != nil {
		return err
	}
	defer func() { _ = repo.Close() }() //nolint:errcheck

	ctx := cmd.Context()
	lastAttempt, _, aerr := repo.LastAttempt(ctx)
	if aerr != nil {
		return fmt.Errorf("query last attempt: %w", aerr)
	}
	lastSuccess, _, serr := repo.LastSuccess(ctx)
	if serr != nil {
		return fmt.Errorf("query last success: %w", serr)
	}

	now := d.Now()
	state := "unknown"
	var age time.Duration
	if lastSuccess != nil && lastSuccess.FinishedAt != nil {
		age = now.Sub(lastSuccess.FinishedAt.UTC())
		if age < 0 {
			age = 0 // clock-skew safe (AC-1)
		}
		state = "healthy"
	}
	if lastAttempt != nil && !lastAttempt.IsSuccess() && lastAttempt.Status != domain.IngestRunRunning {
		state = "degraded"
	}
	if lastAttempt != nil && lastAttempt.Status == domain.IngestRunRunning {
		if state == "unknown" {
			state = "running"
		}
	}

	var executionSource string
	if lastAttempt != nil {
		executionSource = lastAttempt.Origin
	} else if lastSuccess != nil {
		executionSource = lastSuccess.Origin
	}

	if output == "json" {
		payload := map[string]any{
			"last_attempt":     nil,
			"last_success":     nil,
			"age":              age.String(),
			"state":            state,
			"execution_source": executionSource,
			"storage_backend":  cfg.StorageBackend,
		}
		if lastAttempt != nil {
			payload["last_attempt"] = formatRunJSON(*lastAttempt)
		}
		if lastSuccess != nil {
			payload["last_success"] = formatRunJSON(*lastSuccess)
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if err := enc.Encode(payload); err != nil {
			return fmt.Errorf("encode status: %w", err)
		}
		return nil
	}

	out := cmd.OutOrStdout()
	if lastAttempt == nil && lastSuccess == nil {
		_, _ = fmt.Fprintln(out, "Ingestion status: no runs recorded yet (first install)")
		return nil
	}
	if lastAttempt != nil {
		la := *lastAttempt
		_, _ = fmt.Fprintf(out, "Last attempt:  status=%s origin=%s started=%s", la.Status.String(), la.Origin, la.StartedAt.UTC().Format(time.RFC3339))
		if la.FinishedAt != nil {
			_, _ = fmt.Fprintf(out, " finished=%s", la.FinishedAt.UTC().Format(time.RFC3339))
		}
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintf(out, "  sources=%d/%d entries_fetched=%d stories_created=%d stories_updated=%d\n",
			la.SourcesSucceeded, la.SourcesChecked, la.EntriesFetched, la.StoriesCreated, la.StoriesUpdated)
		if la.ErrorSummary != "" {
			_, _ = fmt.Fprintf(out, "  error: %s\n", la.ErrorSummary)
		}
	}
	if lastSuccess != nil {
		ls := *lastSuccess
		_, _ = fmt.Fprintf(out, "Last success:  status=%s finished=%s age=%s state=%s\n",
			ls.Status.String(), ls.FinishedAt.UTC().Format(time.RFC3339), age, state)
	} else {
		_, _ = fmt.Fprintln(out, "Last success:  (none)")
	}
	return nil
}

// shouldRunJSON is the stable JSON contract for `ingest should-run` (AC-5).
type shouldRunJSON struct {
	ShouldRun              bool    `json:"should_run"`
	Reason                 string  `json:"reason"`
	LastSuccessfulIngestAt *string `json:"last_successful_ingest_at"`
}

// runIngestShouldRun implements `ingest should-run` (AC-5). Exit 0 regardless
// of the true/false decision on a correct query; non-zero (no verdict) on DB
// errors so callers fail closed.
func runIngestShouldRun(cmd *cobra.Command, d *Deps, staleAfter time.Duration, output string) error {
	repo, _, err := openIngestRuns(d, "")
	if err != nil {
		return err
	}
	defer func() { _ = repo.Close() }() //nolint:errcheck

	lastSuccess, _, serr := repo.LastSuccess(cmd.Context())
	if serr != nil {
		return fmt.Errorf("query last success: %w", serr)
	}

	decision := domain.DecideShouldRun(lastSuccess, d.Now(), staleAfter)

	if output == "json" {
		payload := shouldRunJSON{ShouldRun: decision.ShouldRun, Reason: decision.Reason}
		if decision.LastSuccessfulAt != nil {
			s := decision.LastSuccessfulAt.UTC().Format(time.RFC3339)
			payload.LastSuccessfulIngestAt = &s
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if err := enc.Encode(payload); err != nil {
			return fmt.Errorf("encode should-run: %w", err)
		}
		return nil
	}

	out := cmd.OutOrStdout()
	yesNo := "NO"
	if decision.ShouldRun {
		yesNo = "YES"
	}
	if decision.LastSuccessfulAt != nil {
		_, _ = fmt.Fprintf(out, "Should run: %s (%s)\n", yesNo, decision.Reason)
		_, _ = fmt.Fprintf(out, "Last successful ingest at: %s (age %s, threshold %s)\n",
			decision.LastSuccessfulAt.UTC().Format(time.RFC3339), decision.Age, staleAfter)
	} else {
		_, _ = fmt.Fprintf(out, "Should run: %s (%s)\n", yesNo, decision.Reason)
	}
	return nil
}
