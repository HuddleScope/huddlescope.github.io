package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

const (
	cliName    = "huddlescope"
	cliVersion = "0.3.0"
)

// NewRootCommand creates the root Cobra command.
func NewRootCommand(opts ...Option) *cobra.Command {
	d := NewDeps(opts...)
	if err := d.Validate(); err != nil {
		panic(fmt.Sprintf("CLI dependency configuration error: %v", err))
	}

	rootCmd := &cobra.Command{
		Use:   cliName,
		Short: "HuddleScope unified CLI for ingest, export, and heartbeat operations",
		Long: `HuddleScope CLI - Unified tool for news aggregation, homepage generation, and system health monitoring.

Usage:
  huddlescope [command] [flags]

Available Commands:
  ingest      Run the ingestion pipeline (fetch feeds, group stories, score and rank)
  export      Export stories to Hugo site (generate homepage.json and story pages)
  heartbeat   Check heartbeat health and staleness
  retention   Run retention: export expiring rows to a package, archive, restore
  config      Validate and display configuration
  version     Show CLI version information

Global Flags:
      --config PATH           Path to config file (.env format)
      --sources-file PATH     Absolute path to a YAML sources file
      --teams-file PATH       Absolute path to a YAML teams file
      --log-level LEVEL       Log level: debug, info, warn, error (default: info)
      --timeout DUR           Request timeout (e.g., 30s, 1m)

Configuration precedence (highest wins):
  1. CLI flags (--config, --sources-file, --teams-file, --log-level, --timeout)
  2. Environment variables (HSC_*, TURSO_*)
  3. --config file values (KEY=VALUE lines)
  4. Built-in defaults

Sources/teams files are expected to be ABSOLUTE paths when set explicitly
(e.g., /etc/huddlescope/sources.yaml). Relative paths are rejected for input
files in all envs and for output paths in production (see docs/portable-deployment.md).

For more information, visit https://huddlescope.github.io/
`,
		Annotations: map[string]string{
			"commandType": "root",
		},
	}

	// Add persistent flags
	rootCmd.PersistentFlags().String("config", "", "Path to config file (.env format)")
	rootCmd.PersistentFlags().String("sources-file", "", "Absolute path to a YAML sources file (wins over HSC_SOURCES_FILE and --config file)")
	rootCmd.PersistentFlags().String("teams-file", "", "Absolute path to a YAML teams file (wins over HSC_TEAMS_FILE and --config file)")
	rootCmd.PersistentFlags().String("log-level", "info", "Log level: debug, info, warn, error")
	rootCmd.PersistentFlags().String("timeout", "", "Request timeout (e.g., 30s, 1m)")

	// Add commands
	rootCmd.AddCommand(NewIngestCommand(d))
	rootCmd.AddCommand(NewExportCommand(d))
	rootCmd.AddCommand(NewHeartbeatCommand(d))
	rootCmd.AddCommand(NewRetentionCommand(d))
	rootCmd.AddCommand(NewConfigCommand(d))
	rootCmd.AddCommand(NewVersionCommand(d))

	return rootCmd
}

// Execute executes the root command.
func Execute(opts ...Option) error {
	rootCmd := NewRootCommand(opts...)
	return rootCmd.Execute()
}

// ExecuteC executes the root command with arguments.
func ExecuteC(args []string, opts ...Option) error {
	rootCmd := NewRootCommand(opts...)
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}
