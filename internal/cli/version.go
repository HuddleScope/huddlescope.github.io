package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewVersionCommand creates the version command.
func NewVersionCommand(d *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show CLI version information",
		Long:  `Show the huddlescope CLI version.`,
		Annotations: map[string]string{
			"commandType": "version",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Print version
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "huddlescope version %s\n", cliVersion)
			if err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to write version output: %v\n", err)
			}
			return nil
		},
	}

	return cmd
}
