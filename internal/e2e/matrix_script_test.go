//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// scriptPath resolves scripts/e2e-matrix.sh relative to the package dir
// (go test runs with cwd = package dir).
func scriptPath(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	p := cwd
	for i := 0; i < 5; i++ {
		candidate := filepath.Join(p, "scripts", "e2e-matrix.sh")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
	t.Fatalf("scripts/e2e-matrix.sh not found walking up from %s", cwd)
	return ""
}

// TestE2EMatrixScriptOfflineGuard is a regression test for the CI failure on
// issue #165: the offline guard in scripts/e2e-matrix.sh previously FAILED when
// GITHUB_REPOSITORY was set, but GitHub Actions sets GITHUB_REPOSITORY (and
// friends) as non-secret repo context on every job, so the required "Build"
// check could never pass in a hosted run. The guard must instead SCRUB these
// vars from the matrix env and proceed; the suites keep the matrix offline via
// their own hermetic process env + t.TempDir() DBs + 127.0.0.1 fixture server.
func TestE2EMatrixScriptOfflineGuard(t *testing.T) {
	script := scriptPath(t)

	t.Setenv("TURSO_DATABASE_URL", "libsql://example.turso.io")
	t.Setenv("TURSO_AUTH_TOKEN", "not-a-real-token")
	t.Setenv("GITHUB_TOKEN", "not-a-real-token")
	t.Setenv("GITHUB_REPOSITORY", "HuddleScope/huddlescope.github.io")
	t.Setenv("E2E_MATRIX_SELFTEST", "1")

	cmd := exec.Command("bash", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("e2e-matrix.sh self-test failed with GH Actions-style env: %v\noutput:\n%s", err, out)
	}
	text := string(out)
	for _, want := range []string{
		"scrubbed from matrix env: TURSO_DATABASE_URL TURSO_AUTH_TOKEN GITHUB_TOKEN GITHUB_REPOSITORY",
		"e2e-matrix self-test: PASS (env scrubbed)",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("expected self-test output to contain %q, got:\n%s", want, text)
		}
	}
}

// TestE2EMatrixScriptOfflineGuardCleanEnv verifies the scrub is a no-op when
// none of the guarded vars are set (self-test still passes, no scrub line).
func TestE2EMatrixScriptOfflineGuardCleanEnv(t *testing.T) {
	script := scriptPath(t)

	t.Setenv("TURSO_DATABASE_URL", "")
	t.Setenv("TURSO_AUTH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GITHUB_REPOSITORY", "")
	t.Setenv("E2E_MATRIX_SELFTEST", "1")

	cmd := exec.Command("bash", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("e2e-matrix.sh self-test failed with clean env: %v\noutput:\n%s", err, out)
	}
	text := string(out)
	if strings.Contains(text, "scrubbed from matrix env") {
		t.Errorf("expected no scrub line with clean env, got:\n%s", text)
	}
	if !strings.Contains(text, "e2e-matrix self-test: PASS (env scrubbed)") {
		t.Errorf("expected self-test PASS line, got:\n%s", text)
	}
}
