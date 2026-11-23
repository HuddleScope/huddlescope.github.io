package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/HuddleScope/huddlescope.github.io/internal/runner"
)

// NewHeartbeatCommand creates the heartbeat command.
func NewHeartbeatCommand(d *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "heartbeat",
		Short: "Check heartbeat health and staleness",
		Long: `Check the health and staleness of the heartbeat file.

Examples:
  huddlescope heartbeat check                           # Check heartbeat
  huddscope heartbeat check --file web/data/heartbeat.json  # Specify file path`,
		Annotations: map[string]string{
			"commandType": "heartbeat",
		},
	}

	// Add subcommands
	cmd.AddCommand(NewHeartbeatCheckCommand(d))

	return cmd
}

// NewHeartbeatCheckCommand creates the heartbeat check subcommand.
func NewHeartbeatCheckCommand(d *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check heartbeat health",
		Long:  `Check the health and staleness of the heartbeat file.`,
		Annotations: map[string]string{
			"commandType": "heartbeat_check",
		},
	}

	// Add flags
	cmd.Flags().String("file", "web/data/heartbeat.json", "Path to heartbeat JSON file")
	cmd.Flags().Duration("stale-after", 90*time.Minute, "Duration after which a heartbeat is considered stale")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runHeartbeatCheck(cmd, d)
	}

	return cmd
}

func runHeartbeatCheck(cmd *cobra.Command, d *Deps) error {
	// Get flags
	filePath, _ := cmd.Flags().GetString("file")
	staleAfter, _ := cmd.Flags().GetDuration("stale-after")

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

	// Check heartbeat
	result := r.CheckHeartbeat(cmd.Context(), filePath, staleAfter)

	// Return error with exit code
	if result.ExitCode != 0 {
		return NewExitError(result.ExitCode, fmt.Errorf("heartbeat check failed"))
	}

	return nil
}
