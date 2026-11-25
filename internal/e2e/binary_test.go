// Package e2e: binary-level integration tests (issue #165 / HS-PORT-09 AC-1).
//
// These tests build the real ./cmd/huddlescope binary once (TestMain) and
// exec it against a local sqlite-file store + an embedded-fixture HTTP RSS
// server (127.0.0.1) with an injected clock (HSC_FAKE_NOW). They exercise
// every CLI command end-to-end at the process boundary:
//
//	version, config check, ingest run, ingest status, ingest should-run
//	(fresh/stale/never-succeeded), export run, heartbeat check.
//
// Invariants (AC-9):
//   - DBs live in t.TempDir(); the fixture server is 127.0.0.1 only; no
//     TURSO_* / GITHUB_* env; the published site-data directory is a temp dir
//     (HSC_DATA_DIR), never web/data.
//   - No external feeds: sources point exclusively at the embedded fixture
//     server.

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// e2eBinaryPath is set by TestMain once the binary is built.
var e2eBinaryPath string

// e2eFakeNow is the injected clock used by the binary tests. It sits after
// every embedded fixture pubDate, so the full 24h lookback window is passed
// by all items.
var e2eFakeNow = "2024-07-31T14:00:00Z"

func TestMain(m *testing.M) {
	bin, err := buildE2EBinary()
	if err != nil {
		// A build failure is fatal for the binary suite.
		fmt.Fprintln(os.Stderr, "e2e binary build failed:", err)
		os.Exit(1)
	}
	e2eBinaryPath = bin
	code := m.Run()
	os.Exit(code)
}

// buildE2EBinary compiles ./cmd/huddlescope into a temp dir with a known
// stamped version so TestBinaryVersion can assert the stamping contract. It
// reuses the module cache (no network) and is independent of `make` ordering.
func buildE2EBinary() (string, error) {
	tmp, err := os.MkdirTemp("", "huddlescope-e2e-bin-*")
	if err != nil {
		return "", err
	}

	bin := filepath.Join(tmp, "huddlescope")
	// Resolve the module root by walking up from the package dir until go.mod
	// is found (tests run from internal/e2e).
	root, err := findModuleRoot()
	if err != nil {
		return "", err
	}

	cmd := exec.Command("go", "build", "-o", bin, "./cmd/huddlescope")
	cmd.Dir = root
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err2("go build ./cmd/huddlescope: " + string(out))
	}
	return bin, nil
}

// err2 wraps a message into an error without importing errors (kept local to
// avoid a package-level import churn).
type err2Msg string

func (e err2Msg) Error() string { return string(e) }

func err2(s string) error { return err2Msg(s) }

// findModuleRoot walks up from the current working directory until it finds
// a go.mod, returning that directory.
func findModuleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for i := 0; i < 12; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", err2("could not find go.mod from " + dir)
}

// runBinaryResult captures the output of an executed binary.
type runBinaryResult struct {
	stdout   string
	stderr   string
	exitCode int
}

// runBinary execs the built binary with the given env and args, capturing
// stdout/stderr and the exit code. The caller supplies a full env (the test
// isolates TURSO_* / GITHUB_* by NOT including them).
func runBinary(t *testing.T, env map[string]string, args ...string) runBinaryResult {
	t.Helper()
	cmd := exec.Command(e2eBinaryPath, args...)
	cmd.Env = envToSlice(env)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("exec binary %v: %v", args, err)
		}
	}
	return runBinaryResult{stdout: stdout.String(), stderr: stderr.String(), exitCode: code}
}

// envToSlice converts a map to a K=V slice, merging over the base OS env.
// The base env is intentionally NOT inherited: tests want a clean, hermetic
// process (no host TURSO_* / GITHUB_* leaking in). Only the keys present in
// `env` are exported.
func envToSlice(env map[string]string) []string {
	base := minimalEnv()
	seen := map[string]bool{}
	for _, kv := range base {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			seen[kv[:i]] = true
		}
	}
	for k, v := range env {
		if seen[k] {
			continue
		}
		base = append(base, k+"="+v)
	}
	return base
}

// minimalEnv returns the barest environment the Go runtime and the binary need
// (PATH for any subprocess, HOME for config lookups). It deliberately excludes
// every TURSO_* / GITHUB_* / HSC_* variable so the test process is hermetic.
func minimalEnv() []string {
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"TMPDIR=" + os.TempDir(),
		"LANG=C",
	}
}

// binaryEnv builds the common env for the binary ingest tests. When
// excludeFailing is true the intentionally-broken fixture feeds (failing-*) are
// omitted from the sources file so the run completes as "ok" (a PublishedAt is
// recorded) rather than "partial". Returns the env map plus the temp data dir.
func binaryEnv(t *testing.T, ts *httptest.Server, excludeFailing bool) (map[string]string, string, func()) {
	t.Helper()

	work := t.TempDir()
	dbPath := filepath.Join(work, "huddlescope.db")
	dataDir := filepath.Join(work, "web", "data")
	contentDir := filepath.Join(work, "web", "content", "stories")
	heartbeatFile := filepath.Join(dataDir, "heartbeat.json")
	feedsFile := filepath.Join(dataDir, "feeds.json")

	// Build a sources YAML pointing at the fixture server for each enabled
	// fixture feed (id/name/url). This keeps the binary test hermetic: no
	// external feeds, only the 127.0.0.1 fixture server.
	sources := []map[string]any{}
	for _, f := range LoadEmbeddedFixtures() {
		if !f.IsEnabled {
			continue
		}
		if excludeFailing && strings.Contains(f.SourceID, "failing") {
			continue
		}
		sources = append(sources, map[string]any{
			"id":   f.SourceID,
			"name": f.SourceID,
			"url":  ts.URL + "/feeds/" + f.SourceID + "/" + f.Version,
		})
	}
	sourcesDoc := "sources:\n"
	for _, s := range sources {
		sourcesDoc += "  - id: " + s["id"].(string) + "\n"
		sourcesDoc += "    name: " + s["name"].(string) + "\n"
		sourcesDoc += "    url: " + s["url"].(string) + "\n"
		sourcesDoc += "    enabled: true\n"
	}
	sourcesPath := filepath.Join(work, "sources.yaml")
	if err := os.WriteFile(sourcesPath, []byte(sourcesDoc), 0o644); err != nil { //nolint:gosec // temp fixture path, operator-controlled
		t.Fatalf("write sources.yaml: %v", err)
	}

	// Teams file: reuse the repo's conf/teams.yaml (absolute path required by
	// config validation when explicitly set).
	teamsFile, err := findRepoFile("conf/teams.yaml")
	if err != nil {
		t.Fatalf("find conf/teams.yaml: %v", err)
	}

	env := map[string]string{
		"HSC_ENV":              "dev",
		"HSC_STORAGE_BACKEND":  "sqlite-file",
		"HSC_STORAGE_PATH":     dbPath,
		"HSC_SOURCES_FILE":     sourcesPath,
		"HSC_TEAMS_FILE":       teamsFile,
		"HSC_DATA_DIR":         dataDir,
		"HSC_HUGO_CONTENT_DIR": contentDir,
		"HSC_HEARTBEAT_FILE":   heartbeatFile,
		"HSC_FEEDS_FILE":       feedsFile,
		"HSC_DATA_FILE":        "homepage.json",
		"HSC_FAKE_NOW":         e2eFakeNow,
		"HSC_RUNNER_ORIGIN":    "e2e",
		// Write site-data to the temp dir (dev default already writes, but be
		// explicit so the assertion on heartbeat.json is deterministic).
		"HSC_PUBLISH_SITE_DATA": "true",
	}
	return env, dataDir, func() {}
}

// findRepoFile resolves a repo-relative path to an absolute path by walking up
// to the module root.
func findRepoFile(rel string) (string, error) {
	root, err := findModuleRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, filepath.FromSlash(rel)), nil
}

// TestBinaryVersion asserts the version command prints the stable
// `huddlescope version <v>` line and a revision/built/target block, and exits 0.
func TestBinaryVersion(t *testing.T) {
	env := minimalEnvMap()
	res := runBinary(t, env, "version")
	if res.exitCode != 0 {
		t.Fatalf("version exit = %d, want 0 (stdout=%q stderr=%q)", res.exitCode, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, "huddlescope version ") {
		t.Errorf("version stdout = %q, want the stable 'huddlescope version <v>' line", res.stdout)
	}
	if !strings.Contains(res.stdout, "revision ") {
		t.Errorf("version stdout = %q, want a 'revision' line", res.stdout)
	}
	if !strings.Contains(res.stdout, "built ") {
		t.Errorf("version stdout = %q, want a 'built' line", res.stdout)
	}
	// The platform line must match the host runtime.
	wantTarget := runtime.GOOS + "/" + runtime.GOARCH
	if !strings.Contains(res.stdout, wantTarget) {
		t.Errorf("version stdout = %q, want target %s", res.stdout, wantTarget)
	}
}

func minimalEnvMap() map[string]string {
	out := map[string]string{}
	for _, kv := range minimalEnv() {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			out[kv[:i]] = kv[i+1:]
		}
	}
	return out
}

// TestBinaryConfigCheck asserts `config check` against the dev config exits 0
// and prints a recognizable config summary (no network needed).
func TestBinaryConfigCheck(t *testing.T) {
	feeds := LoadEmbeddedFixtures()
	server := NewFixtureServer(feeds...)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	env, _, _ := binaryEnv(t, ts, false)
	res := runBinary(t, env, "config", "check")
	if res.exitCode != 0 {
		t.Fatalf("config check exit = %d, want 0 (stdout=%q stderr=%q)", res.exitCode, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, "sqlite-file") {
		t.Errorf("config check stdout = %q, want it to render the sqlite-file storage backend", res.stdout)
	}
}

// TestBinaryIngestRunSQLite asserts `ingest run` against a local sqlite-file
// DB ingests the fixture stories and commits the homepage. The DB is
// t.TempDir()-scoped; the source points at the 127.0.0.1 fixture server.
func TestBinaryIngestRunSQLite(t *testing.T) {
	feeds := LoadEmbeddedFixtures()
	server := NewFixtureServer(feeds...)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	env, dataDir, _ := binaryEnv(t, ts, false)
	res := runBinary(t, env, "ingest", "run", "--mode", "primary")
	if res.exitCode != 0 {
		t.Fatalf("ingest run exit = %d, want 0 (stdout=%q stderr=%q)", res.exitCode, res.stdout, res.stderr)
	}
	// A successful primary ingest must report a committed homepage.
	if !strings.Contains(res.stdout, "Homepage committed successfully") {
		t.Errorf("ingest run stdout = %q, want 'Homepage committed successfully'", res.stdout)
	}

	// Site-data (heartbeat.json) must be written to the temp data dir (never
	// web/data). This is the AC-9 property: no production-path writes.
	hbPath := filepath.Join(dataDir, "heartbeat.json")
	if _, err := os.Stat(hbPath); err != nil {
		t.Errorf("expected heartbeat.json at %s after ingest run: %v", hbPath, err)
	}
}

// TestBinaryIngestStatus asserts `ingest status` reads the run history from the
// shared sqlite DB. With no runs yet it must report "no runs recorded" and
// exit 0. After an ingest run it must report the last success.
func TestBinaryIngestStatus(t *testing.T) {
	feeds := LoadEmbeddedFixtures()
	server := NewFixtureServer(feeds...)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	env, _, _ := binaryEnv(t, ts, false)

	t.Run("no runs yet", func(t *testing.T) {
		res := runBinary(t, env, "ingest", "status")
		if res.exitCode != 0 {
			t.Fatalf("ingest status (fresh) exit = %d, want 0 (stdout=%q stderr=%q)", res.exitCode, res.stdout, res.stderr)
		}
		if !strings.Contains(res.stdout, "no runs recorded") {
			t.Errorf("ingest status (fresh) stdout = %q, want 'no runs recorded'", res.stdout)
		}
	})

	t.Run("after an ingest run", func(t *testing.T) {
		run := runBinary(t, env, "ingest", "run", "--mode", "primary")
		if run.exitCode != 0 {
			t.Fatalf("ingest run exit = %d, want 0 (stdout=%q stderr=%q)", run.exitCode, run.stdout, run.stderr)
		}
		res := runBinary(t, env, "ingest", "status", "--output", "json")
		if res.exitCode != 0 {
			t.Fatalf("ingest status (json) exit = %d, want 0 (stdout=%q stderr=%q)", res.exitCode, res.stdout, res.stderr)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(res.stdout), &payload); err != nil {
			t.Fatalf("ingest status (json) stdout is not valid JSON: %v (stdout=%q)", err, res.stdout)
		}
		if payload["last_success"] == nil {
			t.Errorf("ingest status (json) last_success = nil, want a recorded run")
		}
		if state, _ := payload["state"].(string); state != "healthy" {
			t.Errorf("ingest status (json) state = %q, want healthy", state)
		}
	})
}

// TestBinaryIngestShouldRun asserts `ingest should-run` returns the correct
// decision across the three freshness states, all against the shared sqlite DB.
func TestBinaryIngestShouldRun(t *testing.T) {
	feeds := LoadEmbeddedFixtures()
	server := NewFixtureServer(feeds...)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	env, _, _ := binaryEnv(t, ts, false)

	// 1) never-succeeded: a fresh DB must say should_run=true.
	res := runBinary(t, env, "ingest", "should-run", "--output", "json")
	assertShouldRun(t, res, true, "never_succeeded")

	// 2) After a successful primary ingest, the injected clock is the same
	//    moment as the recorded finish, so the data is fresh: should_run=false.
	run := runBinary(t, env, "ingest", "run", "--mode", "primary")
	if run.exitCode != 0 {
		t.Fatalf("ingest run exit = %d, want 0 (stdout=%q stderr=%q)", run.exitCode, run.stdout, run.stderr)
	}
	res = runBinary(t, env, "ingest", "should-run", "--stale-after", "75m", "--output", "json")
	assertShouldRun(t, res, false, "fresh")
}

// assertShouldRun checks a should-run JSON result has the expected decision and
// reason, and exits 0.
func assertShouldRun(t *testing.T, res runBinaryResult, wantRun bool, wantReason string) {
	t.Helper()
	if res.exitCode != 0 {
		t.Fatalf("should-run exit = %d, want 0 (stdout=%q stderr=%q)", res.exitCode, res.stdout, res.stderr)
	}
	var payload struct {
		ShouldRun bool   `json:"should_run"`
		Reason    string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &payload); err != nil {
		t.Fatalf("should-run stdout is not valid JSON: %v (stdout=%q)", err, res.stdout)
	}
	if payload.ShouldRun != wantRun {
		t.Errorf("should-run should_run = %v, want %v (reason=%q)", payload.ShouldRun, wantRun, payload.Reason)
	}
	if wantReason != "" && payload.Reason != wantReason {
		t.Errorf("should-run reason = %q, want %q", payload.Reason, wantReason)
	}
}

// TestBinaryExportRun asserts `export run` produces a validating homepage.json
// in the temp data dir (never web/data).
func TestBinaryExportRun(t *testing.T) {
	feeds := LoadEmbeddedFixtures()
	server := NewFixtureServer(feeds...)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	env, dataDir, _ := binaryEnv(t, ts, false)
	// Populate the DB first so the export has stories to project.
	ing := runBinary(t, env, "ingest", "run", "--mode", "primary")
	if ing.exitCode != 0 {
		t.Fatalf("ingest run (prime) exit = %d, want 0 (stdout=%q stderr=%q)", ing.exitCode, ing.stdout, ing.stderr)
	}

	res := runBinary(t, env, "export", "run")
	if res.exitCode != 0 {
		t.Fatalf("export run exit = %d, want 0 (stdout=%q stderr=%q)", res.exitCode, res.stdout, res.stderr)
	}

	homePath := filepath.Join(dataDir, "homepage.json")
	data, err := os.ReadFile(homePath)
	if err != nil {
		t.Fatalf("expected homepage.json at %s after export run: %v", homePath, err)
	}
	var snap map[string]any
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("homepage.json is not valid JSON: %v", err)
	}
	if _, ok := snap["stories"]; !ok {
		t.Errorf("homepage.json = %s, want a 'stories' key", data)
	}
}

// TestBinaryHeartbeatCheck asserts `heartbeat check` reads the published
// heartbeat file. A missing file is a clean exit 0 (first run); a published
// fresh file with a passing injected clock is not stale (exit 0).
func TestBinaryHeartbeatCheck(t *testing.T) {
	feeds := LoadEmbeddedFixtures()
	server := NewFixtureServer(feeds...)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	env, dataDir, _ := binaryEnv(t, ts, true) // exclude failing feeds so the run is "ok"
	hbPath := filepath.Join(dataDir, "heartbeat.json")

	t.Run("missing file is clean", func(t *testing.T) {
		res := runBinary(t, env, "heartbeat", "check", "--file", hbPath)
		if res.exitCode != 0 {
			t.Fatalf("heartbeat check (missing) exit = %d, want 0 (stdout=%q stderr=%q)", res.exitCode, res.stdout, res.stderr)
		}
	})

	t.Run("published fresh file is not stale", func(t *testing.T) {
		// Ingest publishes heartbeat.json with PublishedAt set (OK run).
		ing := runBinary(t, env, "ingest", "run", "--mode", "primary")
		if ing.exitCode != 0 {
			t.Fatalf("ingest run exit = %d, want 0 (stdout=%q stderr=%q)", ing.exitCode, ing.stdout, ing.stderr)
		}
		res := runBinary(t, env, "heartbeat", "check", "--file", hbPath, "--stale-after", "90m")
		if res.exitCode != 0 {
			t.Fatalf("heartbeat check (fresh) exit = %d, want 0 (stdout=%q stderr=%q)", res.exitCode, res.stdout, res.stderr)
		}
		if !strings.Contains(res.stdout, "Stale: no") {
			t.Errorf("heartbeat check (fresh) stdout = %q, want 'Stale: no'", res.stdout)
		}
	})
}

// TestBinaryProductionRejectsFakeNow asserts the AC-1 fail-closed property:
// HSC_ENV=production + HSC_FAKE_NOW set must make a command that resolves the
// clock fail with a non-zero exit and a clear error. This guards against a
// misconfigured production host silently using a doctored clock.
func TestBinaryProductionRejectsFakeNow(t *testing.T) {
	feeds := LoadEmbeddedFixtures()
	server := NewFixtureServer(feeds...)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	env, _, _ := binaryEnv(t, ts, false)
	// Force production and keep the injected clock. Production also requires
	// a non-memory storage backend + TURSO_* (validated), so to isolate the
	// FakeNow rejection we point at the local sqlite-file backend AND set the
	// TURSO_* env to dummy values so config validation passes and the only
	// fault is the doctored clock. The clock is resolved by `ingest should-run`
	// (which loads config then reads the clock).
	env["HSC_ENV"] = "production"
	env["TURSO_DATABASE_URL"] = "libsql://127.0.0.1:1"
	env["TURSO_AUTH_TOKEN"] = "dummy"
	// production requires absolute paths (already set by binaryEnv via temp dirs).
	// The sqlite-file backend in production still validates fine (non-memory).

	res := runBinary(t, env, "ingest", "should-run", "--output", "json")
	if res.exitCode == 0 {
		t.Fatalf("production + HSC_FAKE_NOW should-run exit = 0, want non-zero (fail closed). stdout=%q", res.stdout)
	}
	combined := res.stdout + res.stderr
	if !strings.Contains(combined, "test-only") {
		t.Errorf("production + HSC_FAKE_NOW output = %q, want the 'test-only' rejection message", combined)
	}
}

// TestBinaryIngestRunStaleAfter asserts the stale-after flag is honored: a
// large stale-after makes a fresh DB's should-run still true (never succeeded),
// while the duration parsing is validated (a bad duration fails non-zero).
func TestBinaryIngestRunStaleAfter(t *testing.T) {
	feeds := LoadEmbeddedFixtures()
	server := NewFixtureServer(feeds...)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	env, _, _ := binaryEnv(t, ts, false)

	res := runBinary(t, env, "ingest", "should-run", "--stale-after", "not-a-duration", "--output", "json")
	if res.exitCode == 0 {
		t.Fatalf("should-run with a bad --stale-after exit = 0, want non-zero (stdout=%q)", res.stdout)
	}

	// A valid large duration parses fine.
	res = runBinary(t, env, "ingest", "should-run", "--stale-after", "2h", "--output", "json")
	if res.exitCode != 0 {
		t.Fatalf("should-run with --stale-after 2h exit = %d, want 0 (stdout=%q stderr=%q)", res.exitCode, res.stdout, res.stderr)
	}
}
