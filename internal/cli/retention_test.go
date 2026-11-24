package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// fakeRetentionService records calls for CLI wiring tests.
type fakeRetentionService struct {
	exported  bool
	applied   bool
	imported  bool
	statused  bool
	exportRep domain.PackageReport
	importRep domain.PackageReport
}

func (f *fakeRetentionService) ExportPackage(_ context.Context, _ string, _ domain.RetentionPolicy) (domain.PackageReport, error) {
	f.exported = true
	return f.exportRep, nil
}

func (f *fakeRetentionService) Apply(_ context.Context, _ domain.RetentionPolicy) (domain.RetentionReport, error) {
	f.applied = true
	return domain.RetentionReport{BudgetOK: true}, nil
}

func (f *fakeRetentionService) Import(_ context.Context, _ string) (domain.PackageReport, error) {
	f.imported = true
	return f.importRep, nil
}

func (f *fakeRetentionService) Status(_ context.Context) (domain.RetentionStatus, error) {
	f.statused = true
	return domain.RetentionStatus{Backend: "fake", SchemaVersion: 7, BudgetOK: true}, nil
}

func (f *fakeRetentionService) Close() error { return nil }

// execRetention builds the retention command tree, captures all stdout into
// a buffer, and executes it with the given args.
func execRetention(t *testing.T, d *Deps, args ...string) (string, error) {
	t.Helper()
	root := NewRetentionCommand(d)
	buf := &bytes.Buffer{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		c.SetOut(buf)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

func TestNewRetentionCommand_Wiring(t *testing.T) {
	d := NewDeps()
	cmd := NewRetentionCommand(d)
	if cmd.Name() != "retention" {
		t.Fatalf("name: want retention, got %q", cmd.Name())
	}
	subNames := map[string]bool{}
	for _, sub := range cmd.Commands() {
		subNames[sub.Name()] = true
	}
	for _, want := range []string{"run", "export", "apply", "import", "status"} {
		if !subNames[want] {
			t.Errorf("missing subcommand %q (have %v)", want, subNames)
		}
	}
}

func TestRetentionRun_Disabled(t *testing.T) {
	d := NewDeps()
	d.LoadConfig = func(path string) (config.Config, error) {
		cfg := config.Config{}
		cfg.RetentionEnabled = false
		return cfg, nil
	}
	out, err := execRetention(t, d, "run")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "disabled") {
		t.Errorf("expected disabled message, got %q", out)
	}
}

func TestRetentionRun_DryRunExportsOnly(t *testing.T) {
	fake := &fakeRetentionService{
		exportRep: domain.PackageReport{Noop: false, TotalRows: 3, PackagePath: "archive/retention/x.tar.gz"},
	}
	d := NewDeps(WithNewRetentionService(func(cfg config.StorageConfig) (domain.RetentionService, error) {
		return fake, nil
	}))
	out, err := execRetention(t, d, "run", "--dry-run")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !fake.exported {
		t.Error("export should have been called")
	}
	if fake.applied {
		t.Error("apply must NOT be called with --dry-run")
	}
	if !strings.Contains(out, "package") {
		t.Errorf("expected package mention, got %q", out)
	}
}

func TestRetentionRun_ExportFailureAbortsApply(t *testing.T) {
	// A service whose export fails must prevent apply from running.
	failing := &failingExportService{}
	d := NewDeps(WithNewRetentionService(func(cfg config.StorageConfig) (domain.RetentionService, error) {
		return failing, nil
	}))
	_, err := execRetention(t, d, "run")
	if err == nil {
		t.Fatal("run should fail when export fails")
	}
	if failing.applied {
		t.Error("apply must NOT run after export failure")
	}
	if !strings.Contains(err.Error(), "export failed") {
		t.Errorf("error should mention export failure, got %q", err.Error())
	}
}

type failingExportService struct{ applied bool }

func (f *failingExportService) ExportPackage(_ context.Context, _ string, _ domain.RetentionPolicy) (domain.PackageReport, error) {
	return domain.PackageReport{}, errExport
}
func (f *failingExportService) Apply(_ context.Context, _ domain.RetentionPolicy) (domain.RetentionReport, error) {
	f.applied = true
	return domain.RetentionReport{}, nil
}
func (f *failingExportService) Import(_ context.Context, _ string) (domain.PackageReport, error) {
	return domain.PackageReport{}, nil
}
func (f *failingExportService) Status(_ context.Context) (domain.RetentionStatus, error) {
	return domain.RetentionStatus{}, nil
}
func (f *failingExportService) Close() error { return nil }

var errExport = &exportFailedErr{}

type exportFailedErr struct{}

func (e *exportFailedErr) Error() string { return "boom" }

func TestRetentionExport_Noop(t *testing.T) {
	fake := &fakeRetentionService{exportRep: domain.PackageReport{Noop: true}}
	d := NewDeps(WithNewRetentionService(func(cfg config.StorageConfig) (domain.RetentionService, error) {
		return fake, nil
	}))
	out, err := execRetention(t, d, "export")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if !strings.Contains(out, "no rows expired") {
		t.Errorf("expected noop message, got %q", out)
	}
}

func TestRetentionImport_RequiresPackageFlag(t *testing.T) {
	d := NewDeps()
	_, err := execRetention(t, d, "import")
	if err == nil {
		t.Error("import without --package should fail")
	}
}

func TestRetentionImport_WithPackage(t *testing.T) {
	fake := &fakeRetentionService{
		importRep: domain.PackageReport{TotalRows: 2, PackageID: "p1"},
	}
	d := NewDeps(WithNewRetentionService(func(cfg config.StorageConfig) (domain.RetentionService, error) {
		return fake, nil
	}))
	out, err := execRetention(t, d, "import", "--package", "archive/retention/p.tar.gz")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if !fake.imported {
		t.Error("import should have been called")
	}
	if !strings.Contains(out, "restored") {
		t.Errorf("expected restored message, got %q", out)
	}
}

func TestRetentionStatus_PrintsStatus(t *testing.T) {
	fake := &fakeRetentionService{}
	d := NewDeps(WithNewRetentionService(func(cfg config.StorageConfig) (domain.RetentionService, error) {
		return fake, nil
	}))
	out, err := execRetention(t, d, "status")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !fake.statused {
		t.Error("status should have been called")
	}
	if !strings.Contains(out, "backend=fake") {
		t.Errorf("expected backend=fake, got %q", out)
	}
}
