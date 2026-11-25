package cli

import (
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/types"
)

// loadConfigFromFlags returns a LoadConfigFunc closure that resolves the
// effective configuration from the command's persistent flags (issue #158 /
// AC-1, AC-2). The runner calls `loadConfig(path, o)` with path="" and o a
// zero Overrides; this closure substitutes the real --config file path and the
// flag-built overrides (sources-file, teams-file, log-level, timeout) so the
// documented precedence chain (flags > env > file > defaults) is honored without
// changing the runner's call shape.
//
// Only flags the operator EXPLICITLY set (Changed) are applied as overrides, so
// a flag left at its default never clobbers an env value. Persistent flags are
// looked up on both the command's local set and its persistent set (the latter
// holds them before cobra merges them during Execute).
func loadConfigFromFlags(cmd *cobra.Command, d *Deps) types.LoadConfigFunc {
	return func(path string, _ config.Overrides) (config.Config, error) {
		cfgPath := ""
		if f := lookupFlag(cmd, "config"); f != nil && f.Changed {
			cfgPath = f.Value.String()
		}

		overrides := config.Overrides{}
		if f := lookupFlag(cmd, "sources-file"); f != nil && f.Changed {
			overrides.SourcesFile = f.Value.String()
		}
		if f := lookupFlag(cmd, "teams-file"); f != nil && f.Changed {
			overrides.TeamsFile = f.Value.String()
		}
		if f := lookupFlag(cmd, "log-level"); f != nil && f.Changed {
			overrides.LogLevel = f.Value.String()
		}
		if f := lookupFlag(cmd, "timeout"); f != nil && f.Changed {
			if f.Value.String() != "" {
				if td, err := time.ParseDuration(f.Value.String()); err == nil {
					overrides.Timeout = td
				}
			}
		}

		// The runner passes path="" (it does not know the --config value); the
		// authoritative config file path and overrides come from the flags.
		return d.LoadConfig(cfgPath, overrides)
	}
}

// lookupFlag finds a flag by name on the command's local flag set first, then
// on its persistent set. Persistent flags registered on the root are not part
// of a subcommand's local Flags() until cobra parses/merges them, so we check
// both to make the lookup robust for direct (non-Execute) calls.
func lookupFlag(cmd *cobra.Command, name string) *pflag.Flag {
	if f := cmd.Flags().Lookup(name); f != nil {
		return f
	}
	return cmd.PersistentFlags().Lookup(name)
}
