package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repoRoot returns the path to the repository root relative to this test file
// (internal/cli -> ../../). The Dockerfile and workflows live at the root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Join(dir, "..", "..")
}

// TestDockerfileSafeguards (issue #161 AC-5) parses the Dockerfile text and
// asserts the structural invariants that make the runtime image minimal and
// safe: multi-stage, CGO_ENABLED=0 in the build stage, -trimpath, CA certs in
// the runtime stage, a non-root USER, the binary at /usr/local/bin/huddlescope,
// an ENTRYPOINT pointing at it, and no COPY of .env or credentials into any
// layer. The text-parse gate stands in for a Docker build in CI (no Docker on
// check runs).
func TestDockerfileSafeguards(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	require.NoError(t, err, "Dockerfile must exist at repo root")
	content := string(data)

	// Multi-stage: at least two FROM lines.
	fromCount := len(regexp.MustCompile(`(?im)^\s*FROM\s+`).FindAllString(content, -1))
	assert.GreaterOrEqual(t, fromCount, 2, "Dockerfile must be multi-stage (>=2 FROM)")

	// Build stage uses CGO_ENABLED=0 + -trimpath (static, reproducible).
	assert.Contains(t, content, "CGO_ENABLED=0", "build stage must set CGO_ENABLED=0")
	assert.Contains(t, content, "-trimpath", "build stage must use -trimpath")

	// Build stage uses the stamped ldflags (version/commit/built).
	assert.Contains(t, content, "cli.cliVersion=", "build stage must stamp cliVersion")
	assert.Contains(t, content, "cli.cliCommit=", "build stage must stamp cliCommit")
	assert.Contains(t, content, "cli.cliBuiltAt=", "build stage must stamp cliBuiltAt")

	// Runtime stage installs CA certs (Turso HTTPS).
	assert.Contains(t, content, "ca-certificates", "runtime stage must install ca-certificates")

	// Binary lands at /usr/local/bin/huddlescope.
	assert.Contains(t, content, "/usr/local/bin/huddlescope",
		"binary must be installed at /usr/local/bin/huddlescope")

	// Non-root USER. The line must set a numeric uid/gid or a non-root name;
	// root is uid 0, so the USER value must not be "0" or "root".
	var userLine string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "USER") {
			userLine = trimmed
			break
		}
	}
	require.NotEmpty(t, userLine, "Dockerfile must set a USER")
	userValue := strings.TrimPrefix(userLine, "USER")
	userValue = strings.TrimSpace(userValue)
	assert.NotEqual(t, "0", userValue, "USER must not be root (uid 0)")
	assert.NotEqual(t, "root", userValue, "USER must not be root")

	// ENTRYPOINT points at the installed binary.
	assert.Contains(t, content, `ENTRYPOINT ["/usr/local/bin/huddlescope"]`,
		"ENTRYPOINT must be the installed binary")

	// No COPY of .env or credential files into any layer. Only match actual
	// COPY *instruction* lines (after whitespace), not comments or prose: a
	// COPY whose arguments reference a .env or a credentials file.
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(?im)^\s*COPY\s+[^\n]*\.env`),
		regexp.MustCompile(`(?im)^\s*COPY\s+[^\n]*credentials`),
		regexp.MustCompile(`(?im)^\s*COPY\s+[^\n]*\.env\.`),
	} {
		assert.False(t, re.MatchString(content),
			"must not COPY any .env/credentials file into a layer (matched %s)", re)
	}
	assert.False(t,
		regexp.MustCompile(`(?im)^\s*COPY\s+--from=build\s+/src\b`).MatchString(content),
		"must not copy the whole source tree into the runtime image")
}

// TestNoImagePushWorkflow (issue #161 AC-6) is a codebase invariant: no GitHub
// workflow may push or log in to a container registry. Image publication stays
// manual/reviewed (the separate, unapproved #204 scope). If a workflow ever
// gains a `docker push` or `docker login`, this test fails the PR.
func TestNoImagePushWorkflow(t *testing.T) {
	root := repoRoot(t)
	wfDir := filepath.Join(root, ".github", "workflows")
	entries, err := os.ReadDir(wfDir)
	require.NoError(t, err, "workflows dir must exist")

	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			files = append(files, e.Name())
		} else if !e.IsDir() && strings.HasSuffix(e.Name(), ".yml") {
			files = append(files, e.Name())
		}
	}
	require.NotEmpty(t, files, "expected at least one workflow file")

	// Patterns that would publish or authenticate to a registry.
	pushPattern := regexp.MustCompile(`(?i)docker\s+push|kubectl\s+.*image|registry\.|gh\s+cr|docker\s+login`)
	for _, name := range files {
		data, err := os.ReadFile(filepath.Join(wfDir, name))
		require.NoError(t, err, "failed to read workflow %s", name)
		content := string(data)
		// Strip full-line comments before matching so a comment mentioning
		// `docker push` does not trip the invariant.
		stripped := stripFullLineComments(content)
		assert.NotRegexp(t, pushPattern, stripped,
			"workflow %s must not push or log in to a container registry (image publishing is #204 scope)", name)
	}
}

// stripFullLineComments removes lines whose first non-whitespace char is '#',
// so a comment that merely mentions a pattern does not trigger the invariant.
// Inline code is still inspected (a commented-out `run:` is still dead text,
// but we want to be conservative: only strip full-line comments).
func stripFullLineComments(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			lines[i] = ""
		}
	}
	return strings.Join(lines, "\n")
}
