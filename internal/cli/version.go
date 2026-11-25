package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// NewVersionCommand creates the version command.
//
// Output (multi-line, issue #161 AC-2) — the first line keeps the exact
// `huddlescope version <v>` shape so that `build-and-release.yaml`'s
// `grep -q <version>` verification and any downstream consumers keep working
// unchanged:
//
//	huddlescope version 0.3.0
//	revision <cliCommit>
//	built <cliBuiltAt>
//	linux/amd64
func NewVersionCommand(d *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show CLI version information",
		Long:  `Show the huddlescope CLI version, revision, build time, and target platform.`,
		Annotations: map[string]string{
			"commandType": "version",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Line 1: stable `huddlescope version <v>` for grep compat.
			// Lines 2-4: revision, deterministic build stamp, and target
			// platform (from runtime, so cross-compiled artifacts report the
			// architecture they were built for — AC-4).
			_, err := fmt.Fprintf(cmd.OutOrStdout(),
				"huddlescope version %s\nrevision %s\nbuilt %s\n%s/%s\n",
				cliVersion, cliCommit, cliBuiltAt, runtime.GOOS, runtime.GOARCH)
			if err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to write version output: %v\n", err)
			}
			return nil
		},
	}

	return cmd
}
