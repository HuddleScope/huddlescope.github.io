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

// NewIngestStatusCommand creates the ingest status subcommand.
func NewIngestStatusCommand(d *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show ingestion status (stub for HS-PORT-03)",
		Long:  `Show ingestion status. This is a stub command for HS-PORT-03.`,
		Annotations: map[string]string{
			"commandType": "ingest_status",
			"deprecated":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Ingestion status: stub for HS-PORT-03")
			return nil
		},
	}

	cmd.Flags().String("output", "text", "Output format (text/json)")
	return cmd
}

// NewIngestShouldRunCommand creates the ingest should-run subcommand.
func NewIngestShouldRunCommand(d *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "should-run",
		Short: "Check if ingestion should run (stub for HS-PORT-03)",
		Long:  `Check if ingestion should run. This is a stub command for HS-PORT-03.`,
		Annotations: map[string]string{
			"commandType": "ingest_should_run",
			"deprecated":  "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "true")
			return nil
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
