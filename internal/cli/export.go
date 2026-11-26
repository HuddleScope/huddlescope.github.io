package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/HuddleScope/huddlescope.github.io/internal/runner"
	"github.com/HuddleScope/huddlescope.github.io/internal/snapshot"
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
	cmd.AddCommand(
		NewExportRunCommand(d),
		NewExportPackageCommand(d),
		NewExportRestoreCommand(d),
		NewExportValidateCommand(d),
	)

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

// NewExportPackageCommand creates the export package subcommand (issue #249
// / PUB-03): bundle the already-written Hugo workspace files into a
// deterministic, validated .tar.gz artifact. Pure file I/O — no database.
func NewExportPackageCommand(d *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "package",
		Short: "Package the exported Hugo workspace into a validated site-snapshot archive",
		Long: `Package the already-written Hugo workspace files (web/data/*.json and
generated story pages) into a deterministic, validated .tar.gz snapshot
archive (issue #249 / PUB-03). The archive is validated after writing so a
writer bug is caught before upload.

Example:
  huddlescope export package --data-dir web/data --content-dir web/content/stories --out dist/site-snapshot --output json`,
		Annotations: map[string]string{
			"commandType": "export_package",
		},
	}

	cmd.Flags().String("data-dir", "web/data", "Directory containing homepage.json (+ optional feeds.json, heartbeat.json)")
	cmd.Flags().String("content-dir", "web/content/stories", "Directory containing generated story pages (*.md)")
	cmd.Flags().String("out", "", "Output path prefix; the archive is written to <out>.tar.gz (required)")
	cmd.Flags().String("last-ingest", "", "Last successful ingest timestamp (RFC3339; empty: unknown)")
	cmd.Flags().String("source-commit", "", "Source git commit SHA (empty: unknown)")
	cmd.Flags().StringP("output", "o", "text", "Output format: text|json")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runExportPackage(cmd, d)
	}
	return cmd
}

// runExportPackage implements `export package` (issue #249 / AC-1, AC-2).
func runExportPackage(cmd *cobra.Command, d *Deps) error {
	dataDir, _ := cmd.Flags().GetString("data-dir")
	contentDir, _ := cmd.Flags().GetString("content-dir")
	out, _ := cmd.Flags().GetString("out")
	lastIngest, _ := cmd.Flags().GetString("last-ingest")
	sourceCommit, _ := cmd.Flags().GetString("source-commit")
	output, _ := cmd.Flags().GetString("output")

	if out == "" {
		return fmt.Errorf("--out is required")
	}
	if output != "text" && output != "json" {
		return fmt.Errorf("--output must be text or json")
	}

	manifest, warnings, err := snapshot.Package(dataDir, contentDir, out, snapshot.PackageMeta{
		LastIngestAt: lastIngest,
		SourceCommit: sourceCommit,
	})
	if err != nil {
		return fmt.Errorf("package snapshot: %w", err)
	}

	archivePath := out + ".tar.gz"
	for _, w := range warnings {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
	}

	if output == "json" {
		payload := struct {
			SnapshotID   string `json:"snapshotId"`
			Path         string `json:"path"`
			Stories      int    `json:"stories"`
			Latest       int    `json:"latest"`
			Teams        int    `json:"teams"`
			StoryPages   int    `json:"storyPages"`
			GeneratedAt  string `json:"generatedAt"`
			LastIngestAt string `json:"lastIngestAt"`
			SourceCommit string `json:"sourceCommit"`
		}{
			SnapshotID:   manifest.SnapshotID,
			Path:         archivePath,
			Stories:      manifest.Stories,
			Latest:       manifest.Latest,
			Teams:        manifest.Teams,
			StoryPages:   manifest.StoryPages,
			GeneratedAt:  manifest.GeneratedAt,
			LastIngestAt: manifest.LastIngestAt,
			SourceCommit: manifest.SourceCommit,
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if err := enc.Encode(payload); err != nil {
			return fmt.Errorf("encode package result: %w", err)
		}
		return nil
	}

	outW := cmd.OutOrStdout()
	if _, err := fmt.Fprintf(outW, "snapshot_id=%s\n", manifest.SnapshotID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(outW, "path=%s\n", archivePath); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(outW, "stories=%d latest=%d teams=%d story_pages=%d\n",
		manifest.Stories, manifest.Latest, manifest.Teams, manifest.StoryPages); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(outW, "generated_at=%s last_ingest=%s source_commit=%s\n",
		manifest.GeneratedAt, manifest.LastIngestAt, manifest.SourceCommit); err != nil {
		return err
	}
	return nil
}

// NewExportRestoreCommand creates the export restore subcommand (issue #249
// / AC-4): rebuild the Hugo workspace from a validated snapshot artifact.
// --force is required: recovery is an explicit operator action.
func NewExportRestoreCommand(d *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore the Hugo workspace from a validated site-snapshot archive",
		Long: `Restore web/data and the generated story pages from a validated
site-snapshot .tar.gz artifact (manual recovery, issue #249 / AC-4).
The archive is fully validated before anything is written; the restored
content keeps its original freshness (no clock writes, no ingest).

--force is required: recovery is an explicit operator action.

Example:
  huddlescope export restore --artifact dist/site-snapshot.tar.gz --data-dir web/data --content-dir web/content/stories --force`,
		Annotations: map[string]string{
			"commandType": "export_restore",
		},
	}

	cmd.Flags().String("artifact", "", "Path to the site-snapshot .tar.gz artifact (required)")
	cmd.Flags().String("data-dir", "web/data", "Target directory for data files")
	cmd.Flags().String("content-dir", "web/content/stories", "Target directory for story pages")
	cmd.Flags().Bool("force", false, "Required: explicit operator intent to overwrite the workspace")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runExportRestore(cmd, d)
	}
	return cmd
}

// runExportRestore implements `export restore` (issue #249 / AC-4).
func runExportRestore(cmd *cobra.Command, d *Deps) error {
	artifact, _ := cmd.Flags().GetString("artifact")
	dataDir, _ := cmd.Flags().GetString("data-dir")
	contentDir, _ := cmd.Flags().GetString("content-dir")
	force, _ := cmd.Flags().GetBool("force")

	// --force is mandatory and checked before any I/O (AC-4).
	if !force {
		return fmt.Errorf("--force is required to restore over the workspace")
	}
	if artifact == "" {
		return fmt.Errorf("--artifact is required")
	}

	manifest, warnings, err := snapshot.Restore(artifact, dataDir, contentDir)
	if err != nil {
		return NewExitError(1, fmt.Errorf("restore snapshot: %w", err))
	}

	for _, w := range warnings {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
	}

	out := cmd.OutOrStdout()
	if _, err := fmt.Fprintf(out, "restored %d files; content is as of %s\n", len(manifest.Files), manifest.GeneratedAt); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "snapshot_id=%s\n", manifest.SnapshotID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "generated_at=%s last_ingest=%s source_commit=%s\n",
		manifest.GeneratedAt, manifest.LastIngestAt, manifest.SourceCommit); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "stories=%d latest=%d teams=%d story_pages=%d\n",
		manifest.Stories, manifest.Latest, manifest.Teams, manifest.StoryPages); err != nil {
		return err
	}
	return nil
}

// NewExportValidateCommand creates the export validate subcommand (issue
// #249 / AC-2): validate a snapshot archive without writing anything.
func NewExportValidateCommand(d *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a site-snapshot archive (fail-closed; writes nothing)",
		Long: `Validate a site-snapshot .tar.gz artifact (issue #249 / AC-2).
Runs the full fail-closed validation (integrity, manifest schema,
checksums, snapshot schema, count consistency, secret and demo-marker
scans, link safety) and prints a summary. Exit 0 on a valid archive,
1 on any validation failure.

Example:
  huddlescope export validate --artifact dist/site-snapshot.tar.gz`,
		Annotations: map[string]string{
			"commandType": "export_validate",
		},
	}

	cmd.Flags().String("artifact", "", "Path to the site-snapshot .tar.gz artifact (required)")
	cmd.Flags().StringP("output", "o", "json", "Output format: text|json")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runExportValidate(cmd, d)
	}
	return cmd
}

// runExportValidate implements `export validate` (issue #249 / AC-2).
func runExportValidate(cmd *cobra.Command, d *Deps) error {
	artifact, _ := cmd.Flags().GetString("artifact")
	output, _ := cmd.Flags().GetString("output")

	if artifact == "" {
		return fmt.Errorf("--artifact is required")
	}
	if output != "text" && output != "json" {
		return fmt.Errorf("--output must be text or json")
	}

	manifest, warnings, err := snapshot.Validate(artifact)
	if err != nil {
		return NewExitError(1, fmt.Errorf("validate snapshot: %w", err))
	}
	for _, w := range warnings {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", w)
	}

	if output == "json" {
		payload := struct {
			Valid      bool     `json:"valid"`
			SnapshotID string   `json:"snapshotId"`
			Stories    int      `json:"stories"`
			Latest     int      `json:"latest"`
			Teams      int      `json:"teams"`
			StoryPages int      `json:"storyPages"`
			Files      int      `json:"files"`
			Warnings   []string `json:"warnings"`
		}{
			Valid: true, SnapshotID: manifest.SnapshotID,
			Stories: manifest.Stories, Latest: manifest.Latest,
			Teams: manifest.Teams, StoryPages: manifest.StoryPages,
			Files: len(manifest.Files), Warnings: warnings,
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if err := enc.Encode(payload); err != nil {
			return fmt.Errorf("encode validate result: %w", err)
		}
		return nil
	}

	out := cmd.OutOrStdout()
	if _, err := fmt.Fprintln(out, "snapshot archive valid"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "snapshot_id=%s\n", manifest.SnapshotID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "stories=%d latest=%d teams=%d story_pages=%d files=%d\n",
		manifest.Stories, manifest.Latest, manifest.Teams, manifest.StoryPages, len(manifest.Files)); err != nil {
		return err
	}
	return nil
}
