package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/publish"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/rss"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage"
	"github.com/HuddleScope/huddlescope.github.io/internal/application"
	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// ingestRunCmd builds the real `ingest run` command (with its flags) so the
// mode/stale-after/force validation is exercised through the actual flag
// wiring (issue #160).
func ingestRunCmd(d *Deps) *cobra.Command {
	return NewIngestRunCommand(d)
}

// depsForIngestCLI wires a minimal deps set that reaches the mode/stale-after
// validation before any real pipeline work.
func depsForIngestCLI(out, errOut *bytes.Buffer) *Deps {
	return NewDeps(
		WithOutput(out),
		WithErrorOutput(errOut),
		WithLoadConfig(func(path string) (config.Config, error) {
			// Empty config: validation fails after mode/stale parse, which is
			// all we need to assert on the parse errors.
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
}

// TestIngestRun_InvalidMode (issue #160): mode must be primary|backup.
func TestIngestRun_InvalidMode(t *testing.T) {
	var out, errOut bytes.Buffer
	d := depsForIngestCLI(&out, &errOut)
	cmd := ingestRunCmd(d)
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	// Set an invalid mode and execute.
	_ = cmd.Flags().Set("mode", "bogus")
	err := runIngest(cmd, d)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid mode")
}

// TestIngestRun_InvalidStaleAfter (issue #160): a malformed --stale-after is a
// parse error, not a silent fallback.
func TestIngestRun_InvalidStaleAfter(t *testing.T) {
	var out, errOut bytes.Buffer
	d := depsForIngestCLI(&out, &errOut)
	cmd := ingestRunCmd(d)
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	_ = cmd.Flags().Set("stale-after", "not-a-duration")
	err := runIngest(cmd, d)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid stale-after duration")
}

// TestIngestRun_DefaultModeIsValid (issue #160): the default mode (primary) and
// default stale-after parse cleanly; the only failure is the (expected) empty
// config, proving the parse path itself did not error.
func TestIngestRun_DefaultModeIsValid(t *testing.T) {
	var out, errOut bytes.Buffer
	d := depsForIngestCLI(&out, &errOut)
	cmd := ingestRunCmd(d)
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := runIngest(cmd, d)
	// The empty config fails validation ("failed to load config" or a config
	// validation error), but NOT the mode/stale parse.
	if err == nil {
		t.Log("no error (config may have been valid)")
		return
	}
	assert.NotContains(t, err.Error(), "invalid mode")
	assert.NotContains(t, err.Error(), "invalid stale-after")
}

// TestIngestRunCommand_FlagsRegistered (issue #160): the previously-hidden
// flags are now live and registered on the command.
func TestIngestRunCommand_FlagsRegistered(t *testing.T) {
	var out, errOut bytes.Buffer
	d := depsForIngestCLI(&out, &errOut)
	cmd := ingestRunCmd(d)
	assert.True(t, cmd.Flags().Lookup("mode") != nil, "--mode flag must be registered")
	assert.True(t, cmd.Flags().Lookup("stale-after") != nil, "--stale-after flag must be registered")
	assert.True(t, cmd.Flags().Lookup("force") != nil, "--force flag must be registered")
	// Default mode is primary (the safe default).
	assert.Equal(t, "primary", cmd.Flags().Lookup("mode").DefValue)
}
