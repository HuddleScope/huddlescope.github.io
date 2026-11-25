package cli

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// NewRetentionCommand creates the `hs retention` command tree (issue #120):
//
//	hs retention run      export package then apply (--dry-run exports only)
//	hs retention export   export expiring rows to a package (no deletion)
//	hs retention apply    archive expiring rows (persist the package first)
//	hs retention import   restore a package into the hot tables
//	hs retention status   show retention state (size, budget, schema)
func NewRetentionCommand(d *Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "retention",
		Short: "Run retention: export expiring rows to a package, archive, restore",
		Long: `Retention pipeline (HS-065 / issue #120).

The retention window (default 90 days, HSC_RETENTION_DAYS) bounds the hot
tables articles, stories, and ranking_snapshots. Before rows are archived
the expiring rows are exported to a portable package
(huddlescope-retention-package/v1, .tar.gz) that must be persisted (in CI:
committed to archive/retention/) before ` + "`apply`" + ` runs. Import restores
a package idempotently.

Examples:
  huddlescope retention run                    # export + apply (local)
  huddlescope retention run --dry-run          # export only
  huddlescope retention export --output archive/retention
  huddlescope retention apply
  huddlescope retention import --package archive/retention/retention-20260101T000000Z.tar.gz
  huddlescope retention status`,
		Annotations: map[string]string{
			"commandType": "retention",
		},
	}

	cmd.AddCommand(NewRetentionRunCommand(d))
	cmd.AddCommand(NewRetentionExportCommand(d))
	cmd.AddCommand(NewRetentionApplyCommand(d))
	cmd.AddCommand(NewRetentionImportCommand(d))
	cmd.AddCommand(NewRetentionStatusCommand(d))

	return cmd
}

// retentionFlags holds the flags shared by retention subcommands.
type retentionFlags struct {
	window  int
	output  string
	dryRun  bool
	pkgPath string
}

func (f *retentionFlags) register(cmd *cobra.Command) {
	cmd.Flags().IntVar(&f.window, "window", 0, "Retention window in days (0 = use HSC_RETENTION_DAYS / default 90)")
	cmd.Flags().StringVar(&f.output, "output", "archive/retention", "Directory for exported packages")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "Export only; do not archive")
	cmd.Flags().StringVar(&f.pkgPath, "package", "", "Path to a retention package (import)")
}

// loadRetentionConfig loads configuration and resolves the retention policy.
// issue #158: resolved through the persistent flags so --config, --sources-file,
// --teams-file, --log-level, --timeout are honored.
func loadRetentionConfig(cmd *cobra.Command, d *Deps) (config.Config, domain.RetentionPolicy, error) {
	cfg, err := loadConfigFromFlags(cmd, d)("", config.Overrides{})
	if err != nil {
		return config.Config{}, domain.RetentionPolicy{}, fmt.Errorf("load config: %w", err)
	}

	pol := domain.NewRetentionPolicy()
	if cfg.RetentionWindowDays > 0 {
		pol.RetentionDays = cfg.RetentionWindowDays
	}
	pol.SourceBackend = cfg.StorageBackend
	return cfg, pol, nil
}

// applyWindowFlag overrides the policy window when --window is set.
func applyWindowFlag(f retentionFlags, pol *domain.RetentionPolicy) {
	if f.window > 0 {
		pol.RetentionDays = f.window
	}
}

// newRetentionService builds the backend service from the loaded config.
func newRetentionService(d *Deps, cfg config.Config) (domain.RetentionService, error) {
	return d.NewRetentionService(cfg.Storage())
}

// retentionPackagePath builds the canonical package file name inside outDir:
// retention-<UTC stamp>.tar.gz.
func retentionPackagePath(outDir string, now time.Time) string {
	stamp := now.UTC().Format("20060102T150405Z")
	return filepath.Join(outDir, "retention-"+stamp+".tar.gz")
}

// NewRetentionRunCommand creates `retention run` (export then apply).
func NewRetentionRunCommand(d *Deps) *cobra.Command {
	var f retentionFlags
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Export a retention package, then archive expiring rows",
		Long: `Run the retention pipeline locally: export expiring rows to a package
under --output, then archive them. Any export failure aborts before the
archive. In CI the upload (git commit) happens between export and apply.

With --dry-run only the export runs; nothing is archived.`,
		Annotations: map[string]string{"commandType": "retention_run"},
	}
	f.register(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return runRetentionRun(cmd, d, f)
	}
	return cmd
}

func runRetentionRun(cmd *cobra.Command, d *Deps, f retentionFlags) error {
	cfg, pol, err := loadRetentionConfig(cmd, d)
	if err != nil {
		return err
	}
	applyWindowFlag(f, &pol)
	if !cfg.RetentionEnabled {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "retention is disabled (HSC_RETENTION_ENABLED=false); nothing to do")
		return nil
	}

	svc, err := newRetentionService(d, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = svc.Close() }() //nolint:errcheck

	outPath := retentionPackagePath(f.output, d.Now())
	rep, err := svc.ExportPackage(cmd.Context(), outPath, pol)
	if err != nil {
		// Export failure aborts before apply (AC-2).
		return fmt.Errorf("retention export failed; apply aborted: %w", err)
	}
	if rep.Noop {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "retention run: no rows expired within %d days; nothing to do\n", pol.RetentionDays)
		return nil
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "retention run: package %s (%d rows: articles=%d stories=%d ranking_snapshots=%d)\n",
		rep.PackagePath, rep.TotalRows, rep.Rows["articles"], rep.Rows["stories"], rep.Rows["ranking_snapshots"])

	if f.dryRun {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "retention run: --dry-run set; skipping apply")
		return nil
	}

	report, err := svc.Apply(cmd.Context(), pol)
	if err != nil {
		return fmt.Errorf("retention apply failed: %w", err)
	}
	printApplyReport(cmd, report)
	return nil
}

// NewRetentionExportCommand creates `retention export`.
func NewRetentionExportCommand(d *Deps) *cobra.Command {
	var f retentionFlags
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export expiring rows to a portable package (no deletion)",
		Long: `Write all rows older than the retention window to a portable package
(huddlescope-retention-package/v1 .tar.gz) under --output. This command never
deletes anything. A run with no expiring rows writes nothing and exits 0.`,
		Annotations: map[string]string{"commandType": "retention_export"},
	}
	f.register(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return runRetentionExport(cmd, d, f)
	}
	return cmd
}

func runRetentionExport(cmd *cobra.Command, d *Deps, f retentionFlags) error {
	cfg, pol, err := loadRetentionConfig(cmd, d)
	if err != nil {
		return err
	}
	applyWindowFlag(f, &pol)
	if !cfg.RetentionEnabled {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "retention is disabled (HSC_RETENTION_ENABLED=false); nothing to do")
		return nil
	}

	svc, err := newRetentionService(d, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = svc.Close() }() //nolint:errcheck

	outPath := retentionPackagePath(f.output, d.Now())
	rep, err := svc.ExportPackage(cmd.Context(), outPath, pol)
	if err != nil {
		return err
	}
	if rep.Noop {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "retention export: no rows expired within %d days; no package written\n", pol.RetentionDays)
		return nil
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "retention export: %s (package_id=%s, %d rows: articles=%d stories=%d ranking_snapshots=%d)\n",
		rep.PackagePath, rep.PackageID, rep.TotalRows, rep.Rows["articles"], rep.Rows["stories"], rep.Rows["ranking_snapshots"])
	return nil
}

// NewRetentionApplyCommand creates `retention apply`.
func NewRetentionApplyCommand(d *Deps) *cobra.Command {
	var f retentionFlags
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Archive expiring rows into *_archive tables (persist the package first)",
		Long: `Archive rows older than the retention window into the *_archive mirrors in
bounded batches, then check the storage budget. Callers must have persisted
the exported package first (in CI: the git commit step).`,
		Annotations: map[string]string{"commandType": "retention_apply"},
	}
	f.register(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return runRetentionApply(cmd, d, f)
	}
	return cmd
}

func runRetentionApply(cmd *cobra.Command, d *Deps, f retentionFlags) error {
	cfg, pol, err := loadRetentionConfig(cmd, d)
	if err != nil {
		return err
	}
	applyWindowFlag(f, &pol)
	if !cfg.RetentionEnabled {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "retention is disabled (HSC_RETENTION_ENABLED=false); nothing to do")
		return nil
	}

	svc, err := newRetentionService(d, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = svc.Close() }() //nolint:errcheck

	report, err := svc.Apply(cmd.Context(), pol)
	if err != nil {
		return err
	}
	printApplyReport(cmd, report)
	return nil
}

func printApplyReport(cmd *cobra.Command, report domain.RetentionReport) {
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "retention apply: archived %d rows (articles=%d stories=%d ranking_snapshots=%d); db size %d bytes, budget ok=%v\n",
		report.TotalArchived, report.Archived["articles"], report.Archived["stories"], report.Archived["ranking_snapshots"],
		report.DBSizeBytes, report.BudgetOK)
	if !report.BudgetOK {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "warning: storage budget exceeded; review retention window or storage")
	}
}

// NewRetentionImportCommand creates `retention import`.
func NewRetentionImportCommand(d *Deps) *cobra.Command {
	var f retentionFlags
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Restore a retention package into the hot tables (idempotent)",
		Long: `Validate a retention package (format v1, whitelisted tables, statement
classifier, size cap) and restore its rows into the hot tables. Import is
idempotent: re-importing the same package does not duplicate rows.`,
		Annotations: map[string]string{"commandType": "retention_import"},
	}
	f.register(cmd)
	cmd.MarkFlagRequired("package") //nolint:errcheck
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if f.pkgPath == "" {
			return fmt.Errorf("--package is required")
		}
		return runRetentionImport(cmd, d, f)
	}
	return cmd
}

func runRetentionImport(cmd *cobra.Command, d *Deps, f retentionFlags) error {
	cfg, _, err := loadRetentionConfig(cmd, d)
	if err != nil {
		return err
	}

	svc, err := newRetentionService(d, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = svc.Close() }() //nolint:errcheck

	rep, err := svc.Import(cmd.Context(), f.pkgPath)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "retention import: %s restored (package_id=%s, %d rows: articles=%d stories=%d ranking_snapshots=%d)\n",
		rep.PackagePath, rep.PackageID, rep.TotalRows, rep.Rows["articles"], rep.Rows["stories"], rep.Rows["ranking_snapshots"])
	return nil
}

// NewRetentionStatusCommand creates `retention status`.
func NewRetentionStatusCommand(d *Deps) *cobra.Command {
	var f retentionFlags
	cmd := &cobra.Command{
		Use:         "status",
		Short:       "Show retention state (enabled, window, db size, budget, schema)",
		Annotations: map[string]string{"commandType": "retention_status"},
	}
	f.register(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return runRetentionStatus(cmd, d)
	}
	return cmd
}

func runRetentionStatus(cmd *cobra.Command, d *Deps) error {
	cfg, pol, err := loadRetentionConfig(cmd, d)
	if err != nil {
		return err
	}

	svc, err := newRetentionService(d, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = svc.Close() }() //nolint:errcheck

	st, err := svc.Status(cmd.Context())
	if err != nil {
		return err
	}
	st.Enabled = cfg.RetentionEnabled
	st.RetentionDays = pol.RetentionDays
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "retention status: backend=%s enabled=%v window=%d days schema_version=%d db_size=%d bytes budget_ok=%v\n",
		st.Backend, st.Enabled, st.RetentionDays, st.SchemaVersion, st.DBSizeBytes, st.BudgetOK)
	return nil
}
