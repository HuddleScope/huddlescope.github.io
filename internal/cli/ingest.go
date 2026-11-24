package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/HuddleScope/huddlescope.github.io/internal/runner"
)

// NewIngestCommand creates the ingest command.
func NewIngestCommand(d *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Run the ingestion pipeline (fetch feeds, group stories, score and rank)",
		Long: `Run the ingestion pipeline for HuddleScope.

This command fetches RSS feeds, groups related stories, scores them,
and generates a ranked homepage.

Examples:
  huddlescope ingest run                    # Run the full ingestion pipeline
  huddscope ingest run --mode primary     # Run in primary mode
  huddscope ingest run --stale-after 75m  # Set stale threshold
  huddscope ingest run --force              # Force run even if stale`,
		Annotations: map[string]string{
			"commandType": "ingest",
		},
	}

	// Add subcommands
	cmd.AddCommand(NewIngestRunCommand(d))
	cmd.AddCommand(NewIngestStatusCommand(d))
	cmd.AddCommand(NewIngestShouldRunCommand(d))

	return cmd
}

// NewIngestRunCommand creates the ingest run subcommand.
func NewIngestRunCommand(d *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the ingestion pipeline",
		Long:  `Run the ingestion pipeline to fetch feeds, group stories, score and rank them.`,
		Annotations: map[string]string{
			"commandType": "ingest_run",
		},
	}

	// Add flags
	cmd.Flags().String("mode", "primary", "Mode of operation (primary/backup)")
	cmd.Flags().String("stale-after", "75m", "Duration after which a heartbeat is considered stale")
	cmd.Flags().Bool("force", false, "Force run even if stale")

	// Mark flags as hidden (no-op for contract completeness)
	_ = cmd.Flags().MarkHidden("mode")
	_ = cmd.Flags().MarkHidden("stale-after")
	_ = cmd.Flags().MarkHidden("force")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runIngest(cmd, d)
	}

	return cmd
}

// NewIngestStatusCommand creates the ingest status subcommand (issue #159).
// Reads run-level ingest history from the shared database (Turso).
func NewIngestStatusCommand(d *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show ingestion status from the shared run history",
		Long: `Show run-level ingestion status from the shared database (issue #159):
last attempt, last successful run, age, state, counters, and execution source.

This is a DB-sourced query: the memory backend cannot serve it (per-host
volatile store) and the command exits non-zero in that case. On any database
error the command exits non-zero WITHOUT printing a verdict (fail closed).`,
		Annotations: map[string]string{
			"commandType": "ingest_status",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			output, _ := cmd.Flags().GetString("output")
			return runIngestStatus(cmd, d, output)
		},
	}

	cmd.Flags().String("output", "text", "Output format (text/json)")
	return cmd
}

// NewIngestShouldRunCommand creates the ingest should-run subcommand (issue #159).
// Stable JSON contract: {"should_run":bool,"reason":"fresh|stale|never_succeeded",
// "last_successful_ingest_at":"RFC3339Z"|null}.
func NewIngestShouldRunCommand(d *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "should-run",
		Short: "Check if ingestion should run based on shared run history",
		Long: `Decide whether a backup ingest should run, based purely on the shared
database's last successful ingest (issue #159).

Exit codes: 0 on a correct query (regardless of the true/false decision);
non-zero on database errors, in which case NO verdict is printed (callers
must fail closed and never treat an error as "run").`,
		Annotations: map[string]string{
			"commandType": "ingest_should_run",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			staleAfterStr, _ := cmd.Flags().GetString("stale-after")
			output, _ := cmd.Flags().GetString("output")
			staleAfter := 75 * time.Minute
			if staleAfterStr != "" {
				parsed, err := time.ParseDuration(staleAfterStr)
				if err != nil {
					return fmt.Errorf("invalid stale-after duration: %w", err)
				}
				staleAfter = parsed
			}
			return runIngestShouldRun(cmd, d, staleAfter, output)
		},
	}

	cmd.Flags().String("stale-after", "75m", "Duration after which a heartbeat is considered stale")
	cmd.Flags().String("output", "text", "Output format (text/json)")
	return cmd
}

func runIngest(cmd *cobra.Command, d *Deps) error {
	// Get flags
	_, _ = cmd.Flags().GetString("mode") // no-op for now
	staleAfterStr, _ := cmd.Flags().GetString("stale-after")
	force, _ := cmd.Flags().GetBool("force")

	// Parse staleAfter
	staleAfter := 75 * time.Minute
	if staleAfterStr != "" {
		parsed, err := time.ParseDuration(staleAfterStr)
		if err != nil {
			return fmt.Errorf("invalid stale-after duration: %w", err)
		}
		staleAfter = parsed
	}

	_ = force // no-op for now

	// Create runner
	r := runner.NewRunner(
		runner.WithOutput(cmd.OutOrStdout()),
		runner.WithErrorOutput(cmd.ErrOrStderr()),
		runner.WithLoadConfig(d.LoadConfig),
		runner.WithNewRepository(d.NewRepository),
		runner.WithNewApp(d.NewApp),
		runner.WithNewPublisher(d.NewPublisher),
		runner.WithNewFetcher(d.NewFetcher),
		runner.WithNow(d.Now),
		runner.WithLoadHeartbeat(d.LoadHeartbeat),
	)

	// Run ingest
	result := r.RunIngest(cmd.Context(), "primary", staleAfter, force)

	if result.Error != nil {
		return result.Error
	}

	// Print summary
	_, _ = fmt.Fprint(cmd.OutOrStdout(), result.Summary)

	// Return error with exit code
	if result.ExitCode != 0 {
		return NewExitError(result.ExitCode, fmt.Errorf("ingest failed"))
	}

	return nil
}
