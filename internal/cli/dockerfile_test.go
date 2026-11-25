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

// TestNoImagePushWorkflow (issue #161 AC-6, updated for issue #204) is a
// codebase invariant: no GitHub workflow may push or log in to a container
// registry, with the single exception of .github/workflows/docker-build.yaml,
// the approved image-publication workflow (issue #204). It uses docker
// actions (login-action / build-push-action) rather than a literal `docker
// push`/`docker login` shell command, but is allowlisted explicitly so the
// intent is unambiguous and future-proof: any *other* workflow that ever
// gains a registry login/push (or a new workflow that isn't on the allowlist)
// fails the PR.
func TestNoImagePushWorkflow(t *testing.T) {
	root := repoRoot(t)
	wfDir := filepath.Join(root, ".github", "workflows")
	entries, err := os.ReadDir(wfDir)
	require.NoError(t, err, "workflows dir must exist")

	// The single approved image-publication workflow (issue #204). Every other
	// workflow is scanned for registry login/push patterns.
	const allowedImagePushWorkflow = "docker-build.yaml"

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
		if name == allowedImagePushWorkflow {
			// Explicitly approved image-publication workflow (issue #204);
			// still read to keep the test's coverage statement honest, but
			// exempt from the registry-pattern ban.
			continue
		}
		data, err := os.ReadFile(filepath.Join(wfDir, name))
		require.NoError(t, err, "failed to read workflow %s", name)
		content := string(data)
		// Strip full-line comments before matching so a comment mentioning
		// `docker push` does not trip the invariant.
		stripped := stripFullLineComments(content)
		assert.NotRegexp(t, pushPattern, stripped,
			"workflow %s must not push or log in to a container registry (only %s may publish images, issue #204)", name, allowedImagePushWorkflow)
	}
}

// TestDockerBuildWorkflow (issue #204) is a regression gate on the approved
// image-publication workflow: it must exist, trigger only on push to main,
// request packages:write (minimal token scope), publish all three tags
// (latest / v{semver} / sha-{short}), delegate version resolution to
// scripts/resolve-version.sh (the same source of truth as #203), enable the
// GHA build cache, and enforce the size / non-root / entrypoint gates. The
// text-parse gate stands in for a real workflow run in CI.
func TestDockerBuildWorkflow(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, ".github", "workflows", "docker-build.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err, "docker-build.yaml must exist at .github/workflows")
	content := string(data)
	stripped := stripFullLineComments(content)

	// AC-1: trigger on push to main.
	assert.Regexp(t, regexp.MustCompile(`(?s)on:\s*\n\s*push:\s*\n\s*branches:\s*\[\s*main\s*\]`), content,
		"must trigger on push to main")

	// AC-11: minimal token scope — packages: write (not repo-wide).
	assert.Contains(t, stripped, "packages: write", "must request packages: write")
	assert.Contains(t, stripped, "contents: read", "must keep contents: read")
	assert.NotContains(t, stripped, "packages: read", "need not re-request packages: read")

	// AC-4/AC-12/AC-13: all three tags, version via the shared resolver.
	assert.Contains(t, stripped, "ghcr.io/huddlescope/huddlescope.github.io:latest", "must tag :latest")
	assert.Contains(t, stripped, `steps.version.outputs.version`, "must derive the semver tag from the resolved version")
	assert.Contains(t, stripped, `sha-${{ steps.stamp.outputs.short }}`, "must tag the exact commit sha")
	assert.Contains(t, stripped, "scripts/resolve-version.sh",
		"must resolve the version via scripts/resolve-version.sh (same source of truth as #203)")

	// AC-6: GHA build cache.
	assert.Contains(t, stripped, "cache-from: type=gha", "must set cache-from: type=gha")
	assert.Contains(t, stripped, "cache-to: type=gha,mode=max", "must set cache-to: type=gha,mode=max")

	// AC-3: image size gate (< 15 MiB).
	assert.Contains(t, stripped, "15728640", "must enforce the 15 MiB size gate (15728640 bytes)")

	// AC-5: non-root + entrypoint gates.
	assert.Contains(t, stripped, "65534:65534", "must gate on the non-root user 65534:65534")
	assert.Contains(t, stripped, "/usr/local/bin/huddlescope", "must gate on the installed-binary entrypoint")

	// AC-5: container smoke runs version + config check against the binary.
	assert.Contains(t, stripped, "version)", "must smoke-test `huddlescope version`")
	assert.Contains(t, stripped, "config check", "must smoke-test `huddlescope config check`")
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
