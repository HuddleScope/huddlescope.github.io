package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/HuddleScope/huddlescope.github.io/internal/runner"
)

// NewExportCommand creates the export command.
func NewExportCommand(d *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export stories to Hugo site (generate homepage.json and story pages)",
		Long: `Export stories to the Hugo site.

This command generates homepage.json and story pages for the Hugo site.

Examples:
  huddlescope export run                          # Run the export pipeline
  huddscope export run --dry-run                # Preview without committing
  huddscope export run --data-dir web/data        # Custom data directory`,
		Annotations: map[string]string{
			"commandType": "export",
		},
	}

	// Add subcommands
	cmd.AddCommand(NewExportRunCommand(d))

	return cmd
}

// NewExportRunCommand creates the export run subcommand.
func NewExportRunCommand(d *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the export pipeline",
		Long:  `Run the export pipeline to generate homepage.json and story pages.`,
		Annotations: map[string]string{
			"commandType": "export_run",
		},
	}

	// Add flags
	cmd.Flags().Bool("dry-run", false, "Run export without committing")
	cmd.Flags().String("data-dir", "", "Output directory for snapshot JSON (empty: use config DataDir)")
	cmd.Flags().String("data-file", "", "Output filename for snapshot JSON (empty: use config DataFile)")
	cmd.Flags().String("content-dir", "", "Output directory for story pages (empty: use config HugoContentDir)")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runExport(cmd, d)
	}

	return cmd
}

func runExport(cmd *cobra.Command, d *Deps) error {
	// Get flags
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	dataDir, _ := cmd.Flags().GetString("data-dir")
	dataFile, _ := cmd.Flags().GetString("data-file")
	contentDir, _ := cmd.Flags().GetString("content-dir")

	// Create runner
	r := runner.NewRunner(
		runner.WithOutput(cmd.OutOrStdout()),
		runner.WithErrorOutput(cmd.ErrOrStderr()),
		runner.WithLoadConfig(loadConfigFromFlags(cmd, d)),
		runner.WithNewRepository(d.NewRepository),
		runner.WithNewApp(d.NewApp),
		runner.WithNewPublisher(d.NewPublisher),
		runner.WithNewFetcher(d.NewFetcher),
		runner.WithNow(d.Now),
		runner.WithLoadHeartbeat(d.LoadHeartbeat),
	)

	// Run export
	result := r.RunExport(cmd.Context(), dryRun, dataDir, dataFile, contentDir)

	if result.Error != nil {
		return result.Error
	}

	// Return error with exit code
	if result.ExitCode != 0 {
		return NewExitError(result.ExitCode, fmt.Errorf("export failed"))
	}

	return nil
}
