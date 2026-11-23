package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/HuddleScope/huddlescope.github.io/internal/runner"
)

// NewConfigCommand creates the config command.
func NewConfigCommand(d *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Validate and display configuration",
		Long: `Validate and display the application configuration.

Examples:
  huddlescope config check              # Check and display configuration
  huddscope config check --output json  # Display as JSON`,
		Annotations: map[string]string{
			"commandType": "config",
		},
	}

	// Add subcommands
	cmd.AddCommand(NewConfigCheckCommand(d))

	return cmd
}

// NewConfigCheckCommand creates the config check subcommand.
func NewConfigCheckCommand(d *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check and display configuration",
		Long:  `Check and display the application configuration with redacted secrets.`,
		Annotations: map[string]string{
			"commandType": "config_check",
		},
	}

	// Add flags
	cmd.Flags().String("output", "text", "Output format (text/json)")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runConfigCheck(cmd, d)
	}

	return cmd
}

func runConfigCheck(cmd *cobra.Command, d *Deps) error {
	// Get flags
	outputFormat, _ := cmd.Flags().GetString("output")

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

	// Check config
	result := r.CheckConfig(cmd.Context(), outputFormat)

	// Return error with exit code
	if result.ExitCode != 0 {
		return NewExitError(result.ExitCode, fmt.Errorf("config check failed"))
	}

	return nil
}
