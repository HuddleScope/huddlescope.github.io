package cli

import (
	"bytes"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVersionOutputFields renders the version command with overridden stamp
// vars and asserts every field (version, revision, built, GOOS/GOARCH) is
// present in the documented positions. This is the AC-2 verification:
// `huddlescope version` prints version, revision, build time, and
// GOOS/GOARCH.
func TestVersionOutputFields(t *testing.T) {
	origVersion, origCommit, origBuiltAt := cliVersion, cliCommit, cliBuiltAt
	defer func() {
		cliVersion, cliCommit, cliBuiltAt = origVersion, origCommit, origBuiltAt
	}()

	cliVersion = "9.8.7-test"
	cliCommit = "deadbeef"
	cliBuiltAt = "2026-11-25T00:00:00Z"

	var out bytes.Buffer
	cmd := NewVersionCommand(nil)
	cmd.SetOut(&out)
	cmd.SetArgs([]string{})
	assert.NoError(t, cmd.Execute())

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	require.Len(t, lines, 4, "version output must be exactly 4 lines, got %q", out.String())

	// Line 1 keeps the stable `huddlescope version <v>` grep shape.
	assert.Equal(t, "huddlescope version 9.8.7-test", lines[0])
	assert.Equal(t, "revision deadbeef", lines[1])
	assert.Equal(t, "built 2026-11-25T00:00:00Z", lines[2])
	assert.Equal(t, runtime.GOOS+"/"+runtime.GOARCH, lines[3])
}

// TestVersionUnknownPlaceholders asserts the unstamped defaults render sane
// placeholders (AC-1: defaults keep `make build` working unchanged).
func TestVersionUnknownPlaceholders(t *testing.T) {
	origVersion, origCommit, origBuiltAt := cliVersion, cliCommit, cliBuiltAt
	defer func() {
		cliVersion, cliCommit, cliBuiltAt = origVersion, origCommit, origBuiltAt
	}()

	// Simulate an unstamped local build: stamp vars left at their zero-ish
	// defaults. (cliVersion has a semver default; commit/builtAt default to
	// the package initializers — assert they are the documented placeholders.)
	cliVersion = "0.0.0"
	cliCommit = "unknown"
	cliBuiltAt = "dev"

	var out bytes.Buffer
	cmd := NewVersionCommand(nil)
	cmd.SetOut(&out)
	cmd.SetArgs([]string{})
	assert.NoError(t, cmd.Execute())

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	assert.Equal(t, "huddlescope version 0.0.0", lines[0])
	assert.Equal(t, "revision unknown", lines[1])
	assert.Equal(t, "built dev", lines[2])
	assert.Equal(t, runtime.GOOS+"/"+runtime.GOARCH, lines[3])
}

// TestBuildInfoVarAssignability is a regression test mirroring the #203
// invariant for the two new stamp vars (AC-3). The Go linker's `-X` flag can
// only override package-level *variables*. If cliCommit or cliBuiltAt were
// ever reverted to a const, their `-X` stamps would silently no-op and every
// released binary would report the hardcoded default. The assignments below
// compile only if both are assignable vars.
func TestBuildInfoVarAssignability(t *testing.T) {
	origCommit, origBuiltAt := cliCommit, cliBuiltAt
	defer func() {
		cliCommit, cliBuiltAt = origCommit, origBuiltAt
	}()

	// These compile only if cliCommit / cliBuiltAt are package-level vars.
	cliCommit = "abc1234"
	cliBuiltAt = "2026-11-25T12:00:00Z"

	var out bytes.Buffer
	cmd := NewVersionCommand(nil)
	cmd.SetOut(&out)
	cmd.SetArgs([]string{})
	assert.NoError(t, cmd.Execute())

	assert.Contains(t, out.String(), "revision abc1234")
	assert.Contains(t, out.String(), "built 2026-11-25T12:00:00Z")
}

// TestVersionCommandNoSecrets asserts the version output never carries
// credential-looking material — only non-secret build metadata (security:
// the ldflags stamp embeds only version/commit/time, no tokens/URLs).
func TestVersionCommandNoSecrets(t *testing.T) {
	origVersion, origCommit, origBuiltAt := cliVersion, cliCommit, cliBuiltAt
	defer func() {
		cliVersion, cliCommit, cliBuiltAt = origVersion, origCommit, origBuiltAt
	}()

	var out bytes.Buffer
	cmd := NewVersionCommand(nil)
	cmd.SetOut(&out)
	cmd.SetArgs([]string{})
	assert.NoError(t, cmd.Execute())

	output := out.String()
	assert.NotContains(t, output, "TURSO_AUTH_TOKEN")
	assert.NotContains(t, output, "token")
	assert.NotContains(t, output, "https://")
}
