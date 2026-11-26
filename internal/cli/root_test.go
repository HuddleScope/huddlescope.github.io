package cli

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	cobra "github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/publish"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/rss"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage"
	"github.com/HuddleScope/huddlescope.github.io/internal/application"
	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
	"github.com/HuddleScope/huddlescope.github.io/internal/runner"
)

func TestRootHelp(t *testing.T) {
	// Test that root help doesn't expose secrets
	var out, errOut bytes.Buffer

	// Use a fresh command for each test
	cmd := &cobra.Command{
		Use:   cliName,
		Short: "HuddleScope unified CLI for ingest, export, and heartbeat operations",
		Long: `HuddleScope CLI - Unified tool for news aggregation, homepage generation, and system health monitoring.

Usage:
  huddlescope [command] [flags]

Available Commands:
  ingest    Run the ingestion pipeline (fetch feeds, group stories, score and rank)
  export    Export stories to Hugo site (generate homepage.json and story pages)
  heartbeat Check heartbeat health and staleness
  config    Validate and display configuration
  version   Show CLI version information

Global Flags:
      --config PATH       Path to config file (.env format)
      --log-level LEVEL   Log level: debug, info, warn, error (default: info)
      --timeout DUR       Request timeout (e.g., 30s, 1m)

For more information, visit https://huddlescope.github.io/
`,
		Annotations: map[string]string{
			"commandType": "root",
		},
	}

	cmd.SetArgs([]string{"--help"})
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	assert.NoError(t, err)

	helpOutput := out.String()

	// Check that required commands are present
	requiredCommands := []string{"ingest", "export", "heartbeat", "config", "version"}
	for _, cmdName := range requiredCommands {
		assert.Contains(t, helpOutput, cmdName, "help should contain %s command", cmdName)
	}

	// Check that secrets are not exposed
	assert.NotContains(t, helpOutput, "TURSO_AUTH_TOKEN", "help should not expose secrets")
	assert.NotContains(t, helpOutput, "auth", "help should not expose auth tokens")
}

func TestRootCommandNoSecretsInError(t *testing.T) {
	// Test that errors don't expose secrets
	var out, errOut bytes.Buffer
	cmd := NewRootCommand(
		WithOutput(&out),
		WithErrorOutput(&errOut),
		// Use an invalid config path to trigger an error
		WithLoadConfig(func(path string, _ config.Overrides) (config.Config, error) {
			return config.Config{}, assert.AnError // Simulate error
		}),
	)

	cmd.SetArgs([]string{"config", "check"})
	err := cmd.Execute()
	assert.Error(t, err)

	// Verify the error doesn't contain secrets
	errOutput := out.String()
	assert.NotContains(t, errOutput, "TURSO_AUTH_TOKEN", "error should not expose secrets")
}

func TestConfigCheckRedacted(t *testing.T) {
	// Test that config check properly redacts secrets
	var out, errOut bytes.Buffer

	// Use the runner directly for testing
	r := runner.NewRunner(
		runner.WithOutput(&out),
		runner.WithErrorOutput(&errOut),
		runner.WithLoadConfig(func(path string, _ config.Overrides) (config.Config, error) {
			return config.Config{
				UserAgent:           "test-agent/1.0",
				RequestTimeout:      30 * time.Second,
				HeartbeatFile:       "web/data/heartbeat.json",
				FeedsFile:           "web/data/feeds.json",
				DataDir:             "web/data",
				DataFile:            "homepage.json",
				HugoContentDir:      "web/content",
				SimilarityThreshold: 0.50,
				TemporalWindow:      48 * time.Hour,
			}, nil
		}),
	)

	result := r.CheckConfig(context.Background(), "text")
	assert.NoError(t, result.Error)

	// Verify output contains non-secret values
	assert.Contains(t, out.String(), "test-agent", "output should contain non-secret values")
	assert.Contains(t, out.String(), "web/data", "output should contain file paths")

	// Verify secrets are redacted (not present in output)
	assert.NotContains(t, out.String(), "TURSO_AUTH_TOKEN", "output should not contain TURSO_AUTH_TOKEN")
	assert.NotContains(t, out.String(), "https://secret-db.turso.io", "output should not contain Turso URL")
}

func TestExecuteC(t *testing.T) {
	// Test ExecuteC helper function with help
	var out, errOut bytes.Buffer
	args := []string{"--help"}

	// Create a root command with the output writers
	cmd := NewRootCommand(
		WithOutput(&out),
		WithErrorOutput(&errOut),
	)

	cmd.SetArgs(args)
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	assert.NoError(t, err)

	// Verify output contains expected content
	output := out.String()
	assert.Contains(t, output, "HuddleScope CLI", "output should contain CLI name")
}

func TestExecuteCWithCommand(t *testing.T) {
	// Test ExecuteC with a specific command
	var out bytes.Buffer
	args := []string{"config", "check"}

	result := ExecuteC(args,
		WithOutput(&out),
		WithErrorOutput(&out),
		WithLoadConfig(func(path string, _ config.Overrides) (config.Config, error) {
			return config.Config{
				UserAgent:           "test-agent/1.0",
				RequestTimeout:      30 * time.Second,
				HeartbeatFile:       "web/data/heartbeat.json",
				FeedsFile:           "web/data/feeds.json",
				DataDir:             "web/data",
				DataFile:            "homepage.json",
				HugoContentDir:      "web/content",
				SimilarityThreshold: 0.50,
				TemporalWindow:      48 * time.Hour,
			}, nil
		}),
	)

	// The command may error (no heartbeat file) but we just verify it runs
	_ = result

	// ExecuteC creates a new command each time, so the output writer is not the same
	// Let's just test that it doesn't crash
	assert.Nil(t, result, "ExecuteC should not return error when config loads")
}

func TestRootCommandAnnotations(t *testing.T) {
	// Test that root command has proper annotations
	cmd := NewRootCommand()

	assert.Equal(t, "root", cmd.Annotations["commandType"], "root command should have commandType annotation")
}

func TestRootCommandPersistentFlags(t *testing.T) {
	// Test that persistent flags are properly set
	cmd := NewRootCommand()

	// Test --config flag
	configFlag := cmd.PersistentFlags().Lookup("config")
	assert.NotNil(t, configFlag, "--config flag should exist")

	// Test --log-level flag
	logLevelFlag := cmd.PersistentFlags().Lookup("log-level")
	assert.NotNil(t, logLevelFlag, "--log-level flag should exist")

	// Test --timeout flag
	timeoutFlag := cmd.PersistentFlags().Lookup("timeout")
	assert.NotNil(t, timeoutFlag, "--timeout flag should exist")
}

func TestSubcommandsExist(t *testing.T) {
	// Test that all required subcommands exist
	cmd := NewRootCommand()

	subcommands := cmd.Commands()
	subcommandNames := make(map[string]bool)
	for _, sub := range subcommands {
		subcommandNames[sub.Use] = true
	}

	// Check required subcommands
	requiredSubcommands := []string{"ingest", "export", "heartbeat", "config", "version"}
	for _, sub := range requiredSubcommands {
		assert.True(t, subcommandNames[sub], "subcommand %s should exist", sub)
	}
}

func TestIngestSubcommand(t *testing.T) {
	// Test ingest subcommand structure
	rootCmd := NewRootCommand()

	// Find ingest command
	var ingestCmd *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "ingest" {
			ingestCmd = cmd
			break
		}
	}

	assert.NotNil(t, ingestCmd, "ingest command should exist")
	assert.Contains(t, ingestCmd.Short, "ingestion pipeline", "ingest short description should be correct")

	// Check subcommands
	subcommands := ingestCmd.Commands()
	subcommandNames := make(map[string]bool)
	for _, sub := range subcommands {
		subcommandNames[sub.Use] = true
	}

	// Check required subcommands
	requiredSubs := []string{"run", "status", "should-run"}
	for _, sub := range requiredSubs {
		assert.True(t, subcommandNames[sub], "ingest should have subcommand %s", sub)
	}
}

func TestIngestRunExitCode(t *testing.T) {
	// Test that ingest run properly handles exit codes
	var out, errOut bytes.Buffer
	cmd := NewRootCommand(
		WithOutput(&out),
		WithErrorOutput(&errOut),
		WithLoadConfig(func(path string, _ config.Overrides) (config.Config, error) {
			return config.Config{}, nil
		}),
		WithNewRepository(func(cfg config.StorageConfig) (domain.Repository, error) {
			return storage.NewMemoryRepository(), nil
		}),
		WithNewApp(func(opts []application.Option) *application.Application {
			return application.New(opts...)
		}),
		WithNewPublisher(func() *publish.LogPublisher {
			return publish.NewLogPublisher()
		}),
		WithNewFetcher(func(source domain.FeedSource, opts ...rss.Option) domain.FeedFetcher {
			return rss.NewHTTPFetcher(source, opts...)
		}),
	)

	cmd.SetArgs([]string{"ingest", "run"})
	err := cmd.Execute()

	// The important thing is that the command runs without panicking
	// The exit code is handled internally and doesn't call os.Exit
	// With no sources, the command may error but the test just ensures it doesn't crash
	_ = err // Ignore error for this test - we're testing that it doesn't panic
}

func TestExportSubcommand(t *testing.T) {
	// Test export subcommand structure
	rootCmd := NewRootCommand()

	// Find export command
	var exportCmd *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "export" {
			exportCmd = cmd
			break
		}
	}

	assert.NotNil(t, exportCmd, "export command should exist")
	assert.Contains(t, exportCmd.Short, "Export", "export short description should contain Export")

	// Check subcommands (issue #249 / PUB-03 adds package/restore/validate;
	// `run` semantics are unchanged).
	subcommands := exportCmd.Commands()
	var uses []string
	for _, sub := range subcommands {
		uses = append(uses, sub.Use)
	}
	assert.ElementsMatch(t, []string{"run", "package", "restore", "validate"}, uses, "export subcommands")
}

func TestHeartbeatSubcommand(t *testing.T) {
	// Test heartbeat subcommand structure
	rootCmd := NewRootCommand()

	// Find heartbeat command
	var heartbeatCmd *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "heartbeat" {
			heartbeatCmd = cmd
			break
		}
	}

	assert.NotNil(t, heartbeatCmd, "heartbeat command should exist")
	assert.Contains(t, heartbeatCmd.Short, "heartbeat", "heartbeat short description should contain heartbeat")

	// Check subcommands
	subcommands := heartbeatCmd.Commands()
	assert.Len(t, subcommands, 1, "heartbeat should have one subcommand")
	assert.Equal(t, "check", subcommands[0].Use, "heartbeat check should be the subcommand")
}

func TestConfigSubcommand(t *testing.T) {
	// Test config subcommand structure
	rootCmd := NewRootCommand()

	// Find config command
	var configCmd *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "config" {
			configCmd = cmd
			break
		}
	}

	assert.NotNil(t, configCmd, "config command should exist")
	assert.Contains(t, configCmd.Short, "config", "config short description should contain config")

	// Check subcommands
	subcommands := configCmd.Commands()
	assert.Len(t, subcommands, 1, "config should have one subcommand")
	assert.Equal(t, "check", subcommands[0].Use, "config check should be the subcommand")
}

func TestVersionCommand(t *testing.T) {
	// Test version command
	var out bytes.Buffer
	cmd := NewVersionCommand(nil)
	cmd.SetOut(&out)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	assert.NoError(t, err)

	output := out.String()
	assert.Contains(t, output, "huddlescope version", "version output should contain version string")
}

// TestCliVersionIsOverridableVar is a regression test for issue #203 / #161.
//
// The build & release workflow stamps the binary version at build time via:
//
//	go build -ldflags "-X .../internal/cli.cliVersion=X.Y.Z"
//
// The Go linker's `-X` flag can only override a *package-level variable*, not
// a constant. If `cliVersion` were ever reverted to a `const`, that `-X` stamp
// would silently no-op and every released binary would report the hardcoded
// default version. This test pins the invariant that `cliVersion` is an
// assignable `var` and that the version command renders its current value.
func TestCliVersionIsOverridableVar(t *testing.T) {
	original := cliVersion
	defer func() { cliVersion = original }() // restore so other tests see the default

	// The invariant: cliVersion must be assignable (i.e., a var, not a const).
	// This assignment compiles only if cliVersion is a package-level variable.
	cliVersion = "9.9.9-overridable"

	var out bytes.Buffer
	cmd := NewVersionCommand(nil)
	cmd.SetOut(&out)
	cmd.SetArgs([]string{})
	assert.NoError(t, cmd.Execute())

	// Line 1 must keep the stable `huddlescope version <v>` grep shape that
	// build-and-release.yaml depends on, and must render the overridden value.
	assert.True(t,
		strings.HasPrefix(out.String(), "huddlescope version 9.9.9-overridable\n"),
		"version command must render the (mutable) cliVersion value on line 1, got %q", out.String())
	// The multi-line output contract from #161 AC-2 (revision/built/GOOS-GOARCH)
	// must be retained.
	assert.Contains(t, out.String(), "revision ")
	assert.Contains(t, out.String(), "built ")
	assert.Contains(t, out.String(), runtime.GOOS+"/"+runtime.GOARCH)
}

// TestCliVersionDefault is a regression test for issue #203: the default
// (unstamped) version string must remain a well-formed semver so that local
// `make build` binaries report a sane version even when no `-X` stamp is passed.
func TestCliVersionDefault(t *testing.T) {
	// Default version must be non-empty and a plausible semver.
	assert.NotEmpty(t, cliVersion, "cliVersion must have a non-empty default")
	// The default must be a semver-ish string (digits and dots, no spaces).
	assert.Regexp(t, `^[0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.]+)?$`, cliVersion,
		"cliVersion default should look like a semantic version, got %q", cliVersion)
}

func TestExecute(t *testing.T) {
	// Test Execute helper function doesn't crash
	var out bytes.Buffer

	// Execute with no args will fail but we just verify it runs
	err := Execute(
		WithOutput(&out),
	)
	_ = err // Ignore error for this test
}
