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
var manifestPutLong = `Store the deployed manifest in the durable store (issue #247 / PUB-01, v2 for
issue #250 / PUB-04).

Validates the candidate snapshot, builds the manifest (same fields as
` + "`publish stamp`" + `), and upserts the single manifest row. After a successful
Pages deploy, a workflow step calls this to record what was actually deployed.

Issue #250 / PUB-04: put is monotonic — it never lowers the stored manifest's
generatedAt. A candidate older than the stored manifest is a no-op with a
warning (exit 0, the stored manifest is unchanged), never a regression.
Optional binding flags (--snapshot-id, --last-ingest, --source-commit,
--run-id) record the exact validated artifact and run (manifest v2).

Exit codes:
  0  success (manifest stored OR monotonic no-op with a warning)
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
	cmd.AddCommand(newPublishVerifyHTMLCommand(d))
	cmd.AddCommand(newPublishReportCommand(d))

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
	cmd.AddCommand(newPublishManifestBootstrapCommand(d))
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
	cmd.Flags().String("snapshot-id", "", "Content-derived site-snapshot ID to bind (manifest v2, issue #250)")
	cmd.Flags().String("last-ingest", "", "Last successful ingest timestamp (RFC3339 or \"unknown\") (manifest v2, issue #250)")
	cmd.Flags().String("source-commit", "", "Source commit SHA of the validated artifact (manifest v2, issue #250)")
	cmd.Flags().String("run-id", "", "CI run identity (manifest v2, issue #250)")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		candidatePath, _ := cmd.Flags().GetString("candidate")
		commit, _ := cmd.Flags().GetString("commit")
		snapshotID, _ := cmd.Flags().GetString("snapshot-id")
		lastIngest, _ := cmd.Flags().GetString("last-ingest")
		sourceCommit, _ := cmd.Flags().GetString("source-commit")
		runID, _ := cmd.Flags().GetString("run-id")
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
		m := application.StampManifestV2(cand, commit, snapshotID, lastIngest, sourceCommit, runID)

		// Issue #250 / PUB-04 (AC3): monotonic put — never lower the stored
		// manifest's generatedAt. Read the stored manifest first; if the
		// candidate is older, skip with a warning (no upsert, exit 0).
		stored, serr := store.Get()
		if serr != nil {
			return NewExitError(1, fmt.Errorf("read stored manifest for monotonic check: %w", serr))
		}
		res := application.MonotonicStamp(stored, domain.DeployedManifest(m))
		if res.Skipped {
			out := struct {
				Skipped  bool                         `json:"skipped"`
				Warning  string                       `json:"warning,omitempty"`
				Manifest application.DeployedManifest `json:"manifest,omitempty"`
			}{Skipped: true, Warning: res.Warning, Manifest: res.Manifest}
			b, _ := json.Marshal(out)
			if res.Warning != "" {
				if _, werr := fmt.Fprintf(cmd.ErrOrStderr(), "::warning:%s\n", res.Warning); werr != nil {
					return NewExitError(1, fmt.Errorf("write warning: %w", werr))
				}
			}
			if _, werr := fmt.Fprintln(cmd.OutOrStdout(), string(b)); werr != nil {
				return NewExitError(1, fmt.Errorf("write manifest output: %w", werr))
			}
			return nil
		}
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

// newPublishManifestBootstrapCommand creates `publish manifest bootstrap`
// (issue #250 / PUB-04 AC5): the only path that can record a zero-story
// manifest, and only when no manifest is stored — or with an explicit,
// audited --acknowledge-override. After any non-empty successful publication,
// zero-over-N replacement is impossible via the gate (the bootstrap command
// is for the initial genuinely-empty state, not for clobbering a populated
// deployment).
func newPublishManifestBootstrapCommand(d *Deps) *cobra.Command {
	long := `Bootstrap the durable deployed manifest for the initial genuinely-empty
state (issue #250 / PUB-04 AC5).

A missing stored manifest with --require-manifest DENIES the gate (fail
closed). The bootstrap command is the explicit, audited path that seeds the
manifest for the first-ever (possibly zero-story) deployment. It refuses to
override an existing manifest unless --acknowledge-override is set, in which
case the --override-reason is recorded on the manifest (audited).

Exit codes:
  0  success (manifest bootstrapped OR override recorded)
  1  error (existing manifest without --acknowledge-override, invalid input,
      non-durable backend, or a DB write failure — fail closed)
`
	cmd := &cobra.Command{
		Use:           "bootstrap",
		Short:         "Bootstrap the durable deployed manifest (initial empty state, issue #250)",
		SilenceErrors: true,
		Long:          long,
		Annotations:   map[string]string{"commandType": "publish_manifest_bootstrap"},
	}
	cmd.Flags().String("candidate", "", "Path to candidate homepage.json (required)")
	cmd.Flags().String("commit", "", "Commit SHA to record in the manifest (required)")
	cmd.Flags().String("empty-reason", "", "Reason for a zero-story bootstrap (recorded when candidate has 0 stories)")
	cmd.Flags().Bool("acknowledge-override", false, "Override an existing manifest (audited; requires --override-reason)")
	cmd.Flags().String("override-reason", "", "Audited reason for the override (required with --acknowledge-override)")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		candidatePath, _ := cmd.Flags().GetString("candidate")
		commit, _ := cmd.Flags().GetString("commit")
		emptyReason, _ := cmd.Flags().GetString("empty-reason")
		ackOverride, _ := cmd.Flags().GetBool("acknowledge-override")
		overrideReason, _ := cmd.Flags().GetString("override-reason")
		if candidatePath == "" {
			return fmt.Errorf("--candidate is required")
		}
		if commit == "" {
			return fmt.Errorf("--commit is required")
		}
		if ackOverride && overrideReason == "" {
			return fmt.Errorf("--acknowledge-override requires --override-reason")
		}

		cfg, err := loadManifestConfig(cmd, d)
		if err != nil {
			return NewExitError(1, err)
		}
		store, err := d.NewManifestStore(cfg.Storage())
		if err != nil {
			return NewExitError(1, fmt.Errorf("open manifest store: %w", err))
		}
		defer func() { _ = store.Close() }() //nolint:errcheck

		cand, err := application.LoadCandidateSnapshot(candidatePath)
		if err != nil {
			return NewExitError(1, err)
		}
		m := application.StampManifest(cand, commit)
		if len(cand.Stories) == 0 && emptyReason != "" {
			m.OverrideReason = emptyReason
		}

		stored, serr := store.Get()
		if serr != nil {
			return NewExitError(1, fmt.Errorf("read stored manifest: %w", serr))
		}
		if stored != nil && stored.Version != "" {
			// An existing manifest is present.
			if !ackOverride {
				return NewExitError(1, fmt.Errorf("a deployed manifest already exists; bootstrap refuses to override without --acknowledge-override"))
			}
			// Audited override: record the reason and proceed.
			m.OverrideReason = overrideReason
		}
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

// verifyHTMLLong is the Long help text for `publish verify-html`.
var verifyHTMLLong = `Validate the rendered Hugo output against the candidate snapshot (issue #250 /
PUB-04 AC6). This is the rendered-HTML layer of last-known-good protection:
the publish gate compares JSON exports, while verify-html compares the rendered
web/public/index.html so a skeleton/zero-story render from a valid JSON export
(the #229 failure mode) is caught by the rendered artifact itself.

Checks (all fail closed): rendered index.html exists; story-card count is at
least the candidate story count and the last validated manifest minimum; the
"Data as of" freshness label equals the candidate generatedAt; no demo/fixture
leakage in any rendered HTML.

Exit codes:
  0  pass (rendered output may be uploaded to Pages)
  3  deny-equivalent (rendered output failed validation; do NOT upload)
  1  invalid input (missing/invalid candidate, manifest, or publicDir)

Output (JSON, single line): the VerifyResult {"pass": bool, "reason": "...",
"story_cards": N, "checks": [...]}
`

func newPublishVerifyHTMLCommand(d *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "verify-html",
		Short:         "Validate the rendered Hugo output against the candidate (issue #250)",
		SilenceErrors: true,
		SilenceUsage:  true,
		Long:          verifyHTMLLong,
		Annotations:   map[string]string{"commandType": "publish_verify_html"},
	}
	cmd.Flags().String("public", "", "Path to the rendered Hugo output dir (e.g. web/public) (required)")
	cmd.Flags().String("candidate", "", "Path to candidate homepage.json (required)")
	cmd.Flags().String("manifest", "", "Path to deployed manifest JSON (optional; empty = never deployed)")

	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		publicDir, _ := cmd.Flags().GetString("public")
		candidatePath, _ := cmd.Flags().GetString("candidate")
		manifestPath, _ := cmd.Flags().GetString("manifest")
		if publicDir == "" {
			return fmt.Errorf("--public is required")
		}
		if candidatePath == "" {
			return fmt.Errorf("--candidate is required")
		}

		cand, err := application.LoadCandidateSnapshot(candidatePath)
		if err != nil {
			return NewExitError(1, err)
		}
		var manifest *application.DeployedManifest
		if manifestPath != "" {
			m, merr := application.LoadDeployedManifest(manifestPath)
			if merr != nil {
				return NewExitError(1, merr)
			}
			manifest = m
		}

		res, verr := application.ValidateRenderedSite(publicDir, cand, manifest)
		if verr != nil {
			return NewExitError(1, verr)
		}
		b, _ := json.Marshal(res)
		if _, werr := fmt.Fprintln(cmd.OutOrStdout(), string(b)); werr != nil {
			return NewExitError(1, fmt.Errorf("write verify output: %w", werr))
		}
		if !res.Pass {
			// Deny-equivalent: the rendered output failed validation. Do NOT
			// upload to Pages (AC6/AC7).
			return NewExitError(3, fmt.Errorf("verify-html denied: %s", res.Reason))
		}
		return nil
	}
	return cmd
}

// reportLong is the Long help text for `publish report`.
var reportLong = `Emit a structured, secret-free run diagnostic (issue #250 / PUB-04 AC8).

Aggregates the run identity, the publish gate verdict, the rendered-site
verification result, and the manifest into a single JSON report. Inputs are
file paths (--gate, --verify, --manifest) plus run-identity flags. The report
is non-secret by construction: it contains only timestamps, hashes, counts,
verdicts, and run IDs — never a DSN, token, or credential.

Exit codes:
  0  success (report emitted)
  1  error (invalid input file)

Output (JSON): the report. With --output, the report is also written to that
path (so a workflow step can upload it as a diagnostic artifact).
`

// reportGate is the shape of the publish gate JSON the report reads.
type reportGate struct {
	Verdict          string `json:"verdict"`
	Reason           string `json:"reason"`
	CandidateStories int    `json:"candidate_stories"`
	DeployedStories  int    `json:"deployed_stories"`
}

// reportVerify is the shape of the verify-html JSON the report reads.
type reportVerify struct {
	Pass       bool     `json:"pass"`
	Reason     string   `json:"reason"`
	StoryCards int      `json:"story_cards"`
	Checks     []string `json:"checks"`
}

// publishReport is the structured secret-free diagnostic (issue #250 / PUB-04
// AC8). Field whitelist: only run identity, verdicts, counts, timestamps, and
// hashes — no DSN, token, or credential.
type publishReport struct {
	RunID        string                        `json:"run_id,omitempty"`
	RunNumber    string                        `json:"run_number,omitempty"`
	Commit       string                        `json:"commit,omitempty"`
	Gate         *reportGate                   `json:"gate,omitempty"`
	Verify       *reportVerify                 `json:"verify,omitempty"`
	Manifest     *application.DeployedManifest `json:"manifest,omitempty"`
	SnapshotID   string                        `json:"snapshot_id,omitempty"`
	LastIngestAt string                        `json:"last_ingest_at,omitempty"`
	SourceCommit string                        `json:"source_commit,omitempty"`
}

func newPublishReportCommand(d *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "report",
		Short:         "Emit a structured, secret-free run diagnostic (issue #250)",
		SilenceErrors: true,
		SilenceUsage:  true,
		Long:          reportLong,
		Annotations:   map[string]string{"commandType": "publish_report"},
	}
	cmd.Flags().String("gate", "", "Path to publish gate JSON output (optional)")
	cmd.Flags().String("verify", "", "Path to verify-html JSON output (optional)")
	cmd.Flags().String("manifest", "", "Path to deployed manifest JSON (optional)")
	cmd.Flags().String("run-id", "", "Run identity (e.g. github.run_id)")
	cmd.Flags().String("run-number", "", "Run number (e.g. github.run_number)")
	cmd.Flags().String("commit", "", "Commit SHA (e.g. github.sha)")
	cmd.Flags().String("snapshot-id", "", "Content-derived site-snapshot ID")
	cmd.Flags().String("last-ingest", "", "Last successful ingest timestamp")
	cmd.Flags().String("source-commit", "", "Source commit of the validated artifact")
	cmd.Flags().String("output", "", "Optional path to write the report JSON to")

	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		gatePath, _ := cmd.Flags().GetString("gate")
		verifyPath, _ := cmd.Flags().GetString("verify")
		manifestPath, _ := cmd.Flags().GetString("manifest")
		runID, _ := cmd.Flags().GetString("run-id")
		runNumber, _ := cmd.Flags().GetString("run-number")
		commit, _ := cmd.Flags().GetString("commit")
		snapshotID, _ := cmd.Flags().GetString("snapshot-id")
		lastIngest, _ := cmd.Flags().GetString("last-ingest")
		sourceCommit, _ := cmd.Flags().GetString("source-commit")
		outPath, _ := cmd.Flags().GetString("output")

		rep := publishReport{
			RunID:        runID,
			RunNumber:    runNumber,
			Commit:       commit,
			SnapshotID:   snapshotID,
			LastIngestAt: lastIngest,
			SourceCommit: sourceCommit,
		}

		if gatePath != "" {
			data, err := os.ReadFile(gatePath) // #nosec G304: gatePath is operator-provided
			if err != nil {
				return NewExitError(1, fmt.Errorf("read gate file: %w", err))
			}
			var g reportGate
			if err := json.Unmarshal(data, &g); err != nil {
				return NewExitError(1, fmt.Errorf("parse gate file: %w", err))
			}
			rep.Gate = &g
		}
		if verifyPath != "" {
			data, err := os.ReadFile(verifyPath) // #nosec G304: verifyPath is operator-provided
			if err != nil {
				return NewExitError(1, fmt.Errorf("read verify file: %w", err))
			}
			var v reportVerify
			if err := json.Unmarshal(data, &v); err != nil {
				return NewExitError(1, fmt.Errorf("parse verify file: %w", err))
			}
			rep.Verify = &v
		}
		if manifestPath != "" {
			m, err := application.LoadDeployedManifest(manifestPath)
			if err != nil {
				return NewExitError(1, fmt.Errorf("read manifest file: %w", err))
			}
			if m != nil {
				rep.Manifest = m
			}
		}

		b, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			return NewExitError(1, fmt.Errorf("marshal report: %w", err))
		}
		if _, werr := fmt.Fprintln(cmd.OutOrStdout(), string(b)); werr != nil {
			return NewExitError(1, fmt.Errorf("write report: %w", werr))
		}
		if outPath != "" {
			if werr := os.WriteFile(outPath, append(b, '\n'), 0o600); werr != nil {
				return NewExitError(1, fmt.Errorf("write report to %s: %w", outPath, werr))
			}
		}
		return nil
	}
	return cmd
}
