// Package cli: publish gate and manifest commands (issue #229).
//
// These commands are pure file operations (no DB, no secrets) used by the
// GitHub Actions workflows to make the publish decision and to stamp the
// deployed manifest after a successful deploy.
package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/HuddleScope/huddlescope.github.io/internal/application"
	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
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

// manifestGetLong is the Long help text for `publish manifest get`.
var manifestGetLong = `Read the deployed manifest from the durable store (issue #247 / PUB-01).

The manifest is the durable record of the last successful deployment that the
publish gate compares candidates against. With web/data/deployed.json moved
out of Git, the manifest now lives in the shared database (Turso in
production, sqlite-file locally).

Exit codes:
  0  success (manifest found OR absent; absent prints {"found":false})
  1  error (invalid input or a DB read failure — fail closed)

Output (JSON, single line):
  {"found": true, "manifest": {...}}   when a manifest is stored
  {"found": false}                    when no manifest is stored (never deployed)

When --output is set and a manifest is found, the manifest JSON is ALSO
written to that path (so a workflow step can feed it into ` + "`publish gate --manifest`" + `).
When no manifest is found, --output is NOT written.
`

// manifestPutLong is the Long help text for `publish manifest put`.
var manifestPutLong = `Store the deployed manifest in the durable store (issue #247 / PUB-01).

Validates the candidate snapshot, builds the manifest (same fields as
` + "`publish stamp`" + `), and upserts the single manifest row. After a successful
Pages deploy, a workflow step calls this to record what was actually deployed.

Exit codes:
  0  success
  1  error (invalid input, non-durable backend, or a DB write failure — fail closed)
`

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
	cmd.AddCommand(newPublishManifestCommand(d))

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
	cmd.Flags().Bool("require-manifest", false, "Fail closed when no manifest is present (production deploy paths; issue #247 AC-6)")
	cmd.Flags().String("output", "json", "Output format: json")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		candidatePath, _ := cmd.Flags().GetString("candidate")
		manifestPath, _ := cmd.Flags().GetString("manifest")
		requireManifest, _ := cmd.Flags().GetBool("require-manifest")
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

		// Evaluate the gate. --require-manifest (issue #247 AC-6): when no
		// manifest is present, deny (fail closed) instead of the plain
		// "never deployed" allow — this is what keeps a zero-story export from
		// deploying an empty site on a fresh (repo-less) checkout.
		var result application.GateResult
		if requireManifest && manifest == nil {
			result, err = application.EvaluatePublishGateRequired(cand)
		} else {
			result, err = application.EvaluatePublishGate(cand, manifest)
		}
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

// newPublishManifestCommand creates the `publish manifest` group (issue #247 /
// PUB-01): `get` and `put` subcommands that read or upsert the durable
// deployed-manifest store behind the publish gate.
func newPublishManifestCommand(d *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "manifest",
		Short:        "Read or store the durable deployed manifest (issue #247)",
		SilenceUsage: true,
		Annotations:  map[string]string{"commandType": "publish_manifest"},
		Long: `Durable deployed-manifest operations.

With web/data/deployed.json moved out of Git, the deployed manifest lives in
the shared database (Turso in production, sqlite-file locally). The publish
gate reads this manifest on every fresh checkout instead of a committed file.

  publish manifest get   Read the manifest (absent => {"found":false}, exit 0)
  publish manifest put   Store the manifest built from a candidate + commit`,
	}

	cmd.AddCommand(newPublishManifestGetCommand(d))
	cmd.AddCommand(newPublishManifestPutCommand(d))
	return cmd
}

// loadManifestConfig loads the storage configuration from flags/env/file.
func loadManifestConfig(cmd *cobra.Command, d *Deps) (config.Config, error) {
	cfg, err := loadConfigFromFlags(cmd, d)("", config.Overrides{})
	if err != nil {
		return config.Config{}, fmt.Errorf("load config: %w", err)
	}
	return cfg, nil
}

// newPublishManifestGetCommand creates `publish manifest get`.
func newPublishManifestGetCommand(d *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "get",
		Short:         "Read the durable deployed manifest",
		SilenceErrors: true,
		Long:          manifestGetLong,
		Annotations:   map[string]string{"commandType": "publish_manifest_get"},
	}
	cmd.Flags().String("output", "", "Optional path to write the manifest JSON to when found")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		outPath, _ := cmd.Flags().GetString("output")

		cfg, err := loadManifestConfig(cmd, d)
		if err != nil {
			return NewExitError(1, err)
		}
		store, err := d.NewManifestStore(cfg.Storage())
		if err != nil {
			// Fail closed: a backend we cannot open means we cannot read the
			// manifest; the gate must not proceed.
			return NewExitError(1, fmt.Errorf("open manifest store: %w", err))
		}
		defer func() { _ = store.Close() }() //nolint:errcheck

		m, err := store.Get()
		if err != nil {
			// DB read failure: fail closed (exit 1).
			return NewExitError(1, fmt.Errorf("read manifest: %w", err))
		}
		if m == nil {
			// Absent: never deployed. Print {"found":false}, do NOT write --output.
			b, _ := json.Marshal(struct {
				Found bool `json:"found"`
			}{Found: false})
			if _, werr := fmt.Fprintln(cmd.OutOrStdout(), string(b)); werr != nil {
				return NewExitError(1, fmt.Errorf("write output: %w", werr))
			}
			return nil
		}

		b, err := json.Marshal(struct {
			Found    bool                         `json:"found"`
			Manifest application.DeployedManifest `json:"manifest"`
		}{Found: true, Manifest: *m})
		if err != nil {
			return NewExitError(1, fmt.Errorf("marshal manifest: %w", err))
		}
		if _, werr := fmt.Fprintln(cmd.OutOrStdout(), string(b)); werr != nil {
			return NewExitError(1, fmt.Errorf("write output: %w", werr))
		}
		if outPath != "" {
			if werr := os.WriteFile(outPath, append(b, '\n'), 0o600); werr != nil {
				return NewExitError(1, fmt.Errorf("write manifest to %s: %w", outPath, werr))
			}
		}
		return nil
	}
	return cmd
}

// newPublishManifestPutCommand creates `publish manifest put`.
func newPublishManifestPutCommand(d *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "put",
		Short:         "Store the durable deployed manifest",
		SilenceErrors: true,
		Long:          manifestPutLong,
		Annotations:   map[string]string{"commandType": "publish_manifest_put"},
	}
	cmd.Flags().String("candidate", "", "Path to candidate homepage.json (required)")
	cmd.Flags().String("commit", "", "Commit SHA to record in the manifest (required)")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		candidatePath, _ := cmd.Flags().GetString("candidate")
		commit, _ := cmd.Flags().GetString("commit")
		if candidatePath == "" {
			return fmt.Errorf("--candidate is required")
		}
		if commit == "" {
			return fmt.Errorf("--commit is required")
		}

		cfg, err := loadManifestConfig(cmd, d)
		if err != nil {
			return NewExitError(1, err)
		}
		store, err := d.NewManifestStore(cfg.Storage())
		if err != nil {
			// Fail closed: a non-durable/unknown backend refuses put.
			return NewExitError(1, fmt.Errorf("open manifest store: %w", err))
		}
		defer func() { _ = store.Close() }() //nolint:errcheck

		cand, err := application.LoadCandidateSnapshot(candidatePath)
		if err != nil {
			return NewExitError(1, err)
		}
		m := application.StampManifest(cand, commit)
		if err := store.Put(domain.DeployedManifest(m)); err != nil {
			return NewExitError(1, fmt.Errorf("store manifest: %w", err))
		}
		b, _ := json.Marshal(m)
		if _, werr := fmt.Fprintln(cmd.OutOrStdout(), string(b)); werr != nil {
			return NewExitError(1, fmt.Errorf("write manifest output: %w", werr))
		}
		return nil
	}
	return cmd
}
