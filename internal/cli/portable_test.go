package cli

import (
	"os"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/config"
)

// TestRootCommandHasPersistentFileFlags verifies the new --sources-file and
// --teams-file persistent flags exist on the root command (issue #158 / AC-1,
// AC-2).
func TestRootCommandHasPersistentFileFlags(t *testing.T) {
	cmd := NewRootCommand()
	for _, name := range []string{"config", "sources-file", "teams-file", "log-level", "timeout"} {
		if cmd.PersistentFlags().Lookup(name) == nil {
			t.Errorf("persistent flag --%s not found", name)
		}
	}
}

// TestLoadConfigFromFlagsSourcesFileWinsOverEnv verifies the documented
// precedence: the --sources-file flag wins over HSC_SOURCES_FILE env
// (issue #158 / AC-1).
func TestLoadConfigFromFlagsSourcesFileWinsOverEnv(t *testing.T) {
	t.Setenv("HSC_SOURCES_FILE", "/env/sources.yaml")
	d := NewDeps()
	cmd := NewRootCommand()
	// Simulate the operator setting --sources-file on the command line.
	if err := cmd.PersistentFlags().Set("sources-file", "/flag/sources.yaml"); err != nil {
		t.Fatalf("set sources-file flag: %v", err)
	}
	cfg, err := loadConfigFromFlags(cmd, d)("", config.Overrides{})
	if err != nil {
		t.Fatalf("loadConfigFromFlags: %v", err)
	}
	if cfg.SourcesFile != "/flag/sources.yaml" {
		t.Errorf("SourcesFile = %q, want /flag/sources.yaml (flag should win over env)", cfg.SourcesFile)
	}
}

// TestLoadConfigFromFlagsTeamsFileWinsOverEnv verifies the --teams-file flag
// wins over HSC_TEAMS_FILE env (issue #158 / AC-2).
func TestLoadConfigFromFlagsTeamsFileWinsOverEnv(t *testing.T) {
	t.Setenv("HSC_TEAMS_FILE", "/env/teams.yaml")
	d := NewDeps()
	cmd := NewRootCommand()
	if err := cmd.PersistentFlags().Set("teams-file", "/flag/teams.yaml"); err != nil {
		t.Fatalf("set teams-file flag: %v", err)
	}
	cfg, err := loadConfigFromFlags(cmd, d)("", config.Overrides{})
	if err != nil {
		t.Fatalf("loadConfigFromFlags: %v", err)
	}
	if cfg.TeamsFile != "/flag/teams.yaml" {
		t.Errorf("TeamsFile = %q, want /flag/teams.yaml (flag should win over env)", cfg.TeamsFile)
	}
}

// TestLoadConfigFromFlagsEnvFallsThroughWhenFlagUnset verifies that when the
// flag is NOT set, the env value is used (issue #158 / AC-1 fallback).
func TestLoadConfigFromFlagsEnvFallsThroughWhenFlagUnset(t *testing.T) {
	t.Setenv("HSC_SOURCES_FILE", "/env/sources.yaml")
	d := NewDeps()
	cmd := NewRootCommand()
	// Flag not set; env should be used.
	cfg, err := loadConfigFromFlags(cmd, d)("", config.Overrides{})
	if err != nil {
		t.Fatalf("loadConfigFromFlags: %v", err)
	}
	if cfg.SourcesFile != "/env/sources.yaml" {
		t.Errorf("SourcesFile = %q, want /env/sources.yaml (env fallback)", cfg.SourcesFile)
	}
}

// TestLoadConfigFromFlagsConfigFileFlag verifies the --config persistent flag
// is honored by loadConfigFromFlags (issue #158 / AC-1).
func TestLoadConfigFromFlagsConfigFileFlag(t *testing.T) {
	// Use a real temp config file.
	tmpe := t.TempDir()
	cfgPath := tmpe + "/test.env"
	if err := os.WriteFile(cfgPath, []byte("HSC_SOURCES_FILE=/fromfile/sources.yaml\n"), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	d := NewDeps()
	cmd := NewRootCommand()
	if err := cmd.PersistentFlags().Set("config", cfgPath); err != nil {
		t.Fatalf("set config flag: %v", err)
	}
	cfg, err := loadConfigFromFlags(cmd, d)("", config.Overrides{})
	if err != nil {
		t.Fatalf("loadConfigFromFlags: %v", err)
	}
	if cfg.SourcesFile != "/fromfile/sources.yaml" {
		t.Errorf("SourcesFile = %q, want /fromfile/sources.yaml (from --config file)", cfg.SourcesFile)
	}
}

// TestLoadConfigFromFlagsTimeoutAndLogLevel verifies the --timeout and
// --log-level persistent flags are honored (issue #158 / AC-1).
func TestLoadConfigFromFlagsTimeoutAndLogLevel(t *testing.T) {
	d := NewDeps()
	cmd := NewRootCommand()
	if err := cmd.PersistentFlags().Set("timeout", "45s"); err != nil {
		t.Fatalf("set timeout flag: %v", err)
	}
	if err := cmd.PersistentFlags().Set("log-level", "warn"); err != nil {
		t.Fatalf("set log-level flag: %v", err)
	}
	cfg, err := loadConfigFromFlags(cmd, d)("", config.Overrides{})
	if err != nil {
		t.Fatalf("loadConfigFromFlags: %v", err)
	}
	if cfg.RequestTimeout != 45*time.Second {
		t.Errorf("RequestTimeout = %v, want 45s", cfg.RequestTimeout)
	}
	if cfg.LogLevel != "warn" {
		t.Errorf("LogLevel = %q, want warn", cfg.LogLevel)
	}
}
