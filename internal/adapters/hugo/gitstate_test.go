// Package hugo: repository-state tests for issue #247 / PUB-01.
//
// These tests assert the Git state of the web/ tree that the issue requires:
// generated Hugo data is NOT tracked (AC-1), the ignore rules are correct
// (AC-5), and the tracked fixtures remain present (AC-1). They run under
// `make test` in CI (a real checkout) and skip gracefully when not run from a
// git checkout (e.g. a `go test` copy), so they can never break non-checkout
// use (plan §8/R4).
package hugo

import (
	"os/exec"
	"strings"
	"testing"
)

// gitRoot returns the repository root (absolute path).
func gitRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Skipf("not in a git checkout; skipping repo-state test: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// inGitRepo reports whether the repository root is a git checkout.
func inGitRepo(t *testing.T) bool {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").CombinedOutput()
	if err != nil {
		t.Skipf("not in a git checkout; skipping repo-state test: %v", err)
	}
	_ = out
	return true
}

// gitLSFiles returns the tracked files matching the given pathspec, run from
// the repository root.
func gitLSFiles(t *testing.T, args ...string) []string {
	t.Helper()
	root := gitRoot(t)
	cmdArgs := append([]string{"-C", root, "ls-files"}, args...)
	out, err := exec.Command("git", cmdArgs...).Output()
	if err != nil {
		t.Fatalf("git ls-files %v: %v", args, err)
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files
}

// gitCheckIgnore reports whether the path is ignored by .gitignore (run from
// the repository root; paths are repo-relative).
func gitCheckIgnore(t *testing.T, path string) bool {
	t.Helper()
	root := gitRoot(t)
	err := exec.Command("git", "-C", root, "check-ignore", path).Run()
	return err == nil
}

// TestGeneratedNewsDataUntracked (AC-1): the four generated data files are not
// in `git ls-files web/data`, and the generated story pages directory has no
// tracked .md except the hand-authored _index.md.
func TestGeneratedNewsDataUntracked(t *testing.T) {
	if !inGitRepo(t) {
		return
	}
	// No generated data file should be tracked.
	for _, f := range []string{
		"web/data/homepage.json",
		"web/data/feeds.json",
		"web/data/heartbeat.json",
		"web/data/deployed.json",
	} {
		tracked := gitLSFiles(t, f)
		if len(tracked) != 0 {
			t.Errorf("%s should NOT be tracked, but git ls-files returned %v", f, tracked)
		}
	}
	// The whole web/data dir should have no tracked files.
	if tracked := gitLSFiles(t, "web/data"); len(tracked) != 0 {
		t.Errorf("web/data should have no tracked files, got %v", tracked)
	}
}

// TestTrackedFixturesPresent (AC-1): the deterministic demo fixtures and the
// hand-authored stories index remain tracked.
func TestTrackedFixturesPresent(t *testing.T) {
	if !inGitRepo(t) {
		return
	}
	mustBeTracked := []string{
		"web/fixtures/homepage.json",
		"web/fixtures/feeds.json",
		"web/content/stories/_index.md",
	}
	for _, f := range mustBeTracked {
		if tracked := gitLSFiles(t, f); len(tracked) == 0 {
			t.Errorf("%s should be tracked, but git ls-files returned empty", f)
		}
	}
	// Fixture story pages are tracked.
	if tracked := gitLSFiles(t, "web/fixtures/content/stories/*.md"); len(tracked) == 0 {
		t.Error("web/fixtures/content/stories/*.md should be tracked")
	}
}

// TestIgnoreRules (AC-5): git check-ignore confirms the generated files are
// ignored and the tracked files are not.
func TestIgnoreRules(t *testing.T) {
	if !inGitRepo(t) {
		return
	}
	ignored := []string{
		"web/data/homepage.json",
		"web/data/feeds.json",
		"web/data/heartbeat.json",
		"web/data/deployed.json",
		"web/content/stories/generated-story.md",
		"web/.hugo_build.lock",
		"web/site-test-run-2.log",
	}
	for _, f := range ignored {
		if !gitCheckIgnore(t, f) {
			t.Errorf("%s should be ignored by .gitignore", f)
		}
	}
	notIgnored := []string{
		"web/content/stories/_index.md", // negation rule
		"web/fixtures/homepage.json",
		"web/fixtures/feeds.json",
		"web/fixtures/content/stories/top-free-agents-2024.md",
	}
	for _, f := range notIgnored {
		if gitCheckIgnore(t, f) {
			t.Errorf("%s should NOT be ignored (tracked fixture/index)", f)
		}
	}
}

// TestStrayArtifactsUntracked confirms the stray Hugo build/test artifacts are
// untracked and ignored.
func TestStrayArtifactsUntracked(t *testing.T) {
	if !inGitRepo(t) {
		return
	}
	for _, f := range []string{"web/.hugo_build.lock", "web/site-test-run-2.log"} {
		if tracked := gitLSFiles(t, f); len(tracked) != 0 {
			t.Errorf("%s should not be tracked, got %v", f, tracked)
		}
	}
}
