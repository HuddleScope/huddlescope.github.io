// Package cli: publish gate and manifest commands (issue #229).
//
// These commands are pure file operations (no DB, no secrets) used by the
// GitHub Actions workflows to make the publish decision and to stamp the
// deployed manifest after a successful deploy.
package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/HuddleScope/huddlescope.github.io/internal/application"
)

// gateLong is the Long help text for `publish gate`.
var gateLong = `Evaluate the publish gate by comparing a candidate snapshot against the
last deployed manifest.

Exit codes:
  0  allow or skip-identical (deploy may proceed)
  3  deny (retain last-good; do not deploy)
  1  invalid input (missing/invalid candidate or manifest)

Output (JSON, single line):
  {
    "verdict": "allow" | "skip-identical" | "deny",
    "reason": "...",
    "candidate_stories": N,
    "deployed_stories": N,
    "candidate_generated_at": "RFC3339",
    "deployed_generated_at": "RFC3339",
    "content_sha256": "hex",
    "warning": "..."   // optional
  }`

// stampLong is the Long help text for `publish stamp`.
var stampLong = `Create the deployed manifest from a candidate snapshot and commit SHA.

Output (JSON, single line): the manifest.

Exit codes:
  0  success
  1  invalid input`

// NewPublishCommand creates the publish command group.
func NewPublishCommand(d *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Publish gate and manifest operations (offline, no DB)",
		Long: `Publish gate and manifest operations.

These commands are pure file operations (no DB, no secrets) used by the
GitHub Actions workflows to make the publish decision and to stamp the
deployed manifest after a successful deploy.

  publish gate   Evaluate the publish gate (candidate vs manifest)
  publish stamp  Stamp the deployed manifest from a candidate snapshot`,
		Annotations: map[string]string{
			"commandType": "publish",
		},
	}

	cmd.AddCommand(newPublishGateCommand(d))
	cmd.AddCommand(newPublishStampCommand(d))

	return cmd
}

func newPublishGateCommand(d *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "gate",
		Short:         "Evaluate the publish gate (candidate vs deployed manifest)",
		SilenceErrors: true,
		SilenceUsage:  true,
		Long:          gateLong,
		Annotations: map[string]string{
			"commandType": "publish_gate",
		},
	}

	cmd.Flags().String("candidate", "", "Path to candidate homepage.json (required)")
	cmd.Flags().String("manifest", "", "Path to deployed manifest JSON (empty: never deployed)")
	cmd.Flags().String("output", "json", "Output format: json")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		candidatePath, _ := cmd.Flags().GetString("candidate")
		manifestPath, _ := cmd.Flags().GetString("manifest")
		output, _ := cmd.Flags().GetString("output")

		if candidatePath == "" {
			return fmt.Errorf("--candidate is required")
		}
		if output != "json" {
			return fmt.Errorf("unsupported output format %q (only json)", output)
		}

		// Load candidate (fail closed: missing/invalid -> exit 1).
		cand, err := application.LoadCandidateSnapshot(candidatePath)
		if err != nil {
			return NewExitError(1, err)
		}

		// Load manifest (missing -> nil, never deployed).
		var manifest *application.DeployedManifest
		if manifestPath != "" {
			m, merr := application.LoadDeployedManifest(manifestPath)
			if merr != nil {
				return NewExitError(1, merr)
			}
			manifest = m
		}

		// Evaluate the gate.
		result, err := application.EvaluatePublishGate(cand, manifest)
		if err != nil {
			return NewExitError(1, err)
		}

		// Emit JSON to stdout (the single source of the verdict for the
		// workflow). SilenceErrors/SilenceUsage keep the deny path's stderr
		// clean so the JSON is the only machine-readable output.
		b, _ := json.Marshal(result)
		if _, werr := fmt.Fprintln(cmd.OutOrStdout(), string(b)); werr != nil {
			return NewExitError(1, fmt.Errorf("write gate output: %w", werr))
		}

		// Exit code: 0 allow/skip-identical, 3 deny.
		if result.Verdict == application.GateVerdictDeny {
			return NewExitError(3, fmt.Errorf("publish gate denied: %s", result.Reason))
		}
		return nil
	}

	return cmd
}

func newPublishStampCommand(d *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "stamp",
		Short:         "Stamp the deployed manifest from a candidate snapshot",
		SilenceErrors: true,
		SilenceUsage:  true,
		Long:          stampLong,
		Annotations: map[string]string{
			"commandType": "publish_stamp",
		},
	}

	cmd.Flags().String("candidate", "", "Path to candidate homepage.json (required)")
	cmd.Flags().String("commit", "", "Commit SHA to record in the manifest (required)")
	cmd.Flags().String("output", "json", "Output format: json")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		candidatePath, _ := cmd.Flags().GetString("candidate")
		commit, _ := cmd.Flags().GetString("commit")
		output, _ := cmd.Flags().GetString("output")

		if candidatePath == "" {
			return fmt.Errorf("--candidate is required")
		}
		if commit == "" {
			return fmt.Errorf("--commit is required")
		}
		if output != "json" {
			return fmt.Errorf("unsupported output format %q (only json)", output)
		}

		// Load candidate (fail closed).
		cand, err := application.LoadCandidateSnapshot(candidatePath)
		if err != nil {
			return NewExitError(1, err)
		}

		// Stamp the manifest.
		m := application.StampManifest(cand, commit)
		b, _ := json.Marshal(m)
		if _, werr := fmt.Fprintln(cmd.OutOrStdout(), string(b)); werr != nil {
			return NewExitError(1, fmt.Errorf("write manifest output: %w", werr))
		}

		return nil
	}

	return cmd
}
