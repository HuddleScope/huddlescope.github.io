//go:build e2e

// Package e2e: publish gate end-to-end tests (issue #229 / AC-6).
//
// These tests exercise the publish gate and CLI in hermetic scenarios:
// S1: consecutive exports (content monotone, gate allows each)
// S2: empty export after real (gate denies, exit 3)
// S3: DB unreachable (export exits non-zero, on-disk file untouched)
// S4: stale candidate (gate denies)
// S5: retention + re-export without ingest (homepage valid, gate allows)
package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// buildCLI builds the huddlescope binary into a temp dir and returns its path.
func buildCLI(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	cmd := exec.Command("go", "build", "-o", filepath.Join(tmpDir, "huddlescope"), "./cmd/huddlescope")
	cmd.Dir = filepath.Join("..", "..")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	return filepath.Join(tmpDir, "huddlescope")
}

// writeSnapshot writes a minimal valid snapshot to a file.
func writeSnapshot(t *testing.T, path string, stories int, generatedAt string) {
	t.Helper()
	s := map[string]any{
		"version":     "1",
		"generatedAt": generatedAt,
		"stories":     make([]any, 0, stories),
		"latest":      []any{},
		"teams":       []any{},
	}
	for i := 0; i < stories; i++ {
		s["stories"] = append(s["stories"].([]any), map[string]any{
			"id":               string(rune('a'+i)) + string(rune('0'+i%10)),
			"title":            "Story " + string(rune('a'+i)),
			"summary":          "Summary " + string(rune('a'+i)),
			"firstPublishedAt": generatedAt,
			"lastPublishedAt":  generatedAt,
			"score":            5.0,
			"evidenceLevel":    "unknown",
			"evidenceLevelCSS": "unknown",
			"publishers":       []any{map[string]any{"id": "src", "name": "Source", "url": "https://example.com"}},
		})
	}
	b, _ := json.Marshal(s)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
}

// S1: Three consecutive exports with increasing story counts.
// The gate should allow each (newer generatedAt, more stories).
func TestPublishGate_S1_ConsecutiveExports(t *testing.T) {
	t.Parallel()
	cli := buildCLI(t)
	tmpDir := t.TempDir()
	candidate := filepath.Join(tmpDir, "homepage.json")
	manifest := filepath.Join(tmpDir, "deployed.json")

	// First export: 5 stories.
	writeSnapshot(t, candidate, 5, "2026-01-01T00:00:00Z")
	cmd := exec.Command(cli, "publish", "gate", "--candidate", candidate, "--manifest", manifest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gate (first): %v\n%s", err, out)
	}
	var r1 map[string]any
	json.Unmarshal(out, &r1)
	if r1["verdict"] != "allow" {
		t.Errorf("S1 first: verdict = %v, want allow", r1["verdict"])
	}

	// Stamp manifest from first export.
	stampCmd := exec.Command(cli, "publish", "stamp", "--candidate", candidate, "--commit", "abc123")
	stampOut, _ := stampCmd.CombinedOutput()
	os.WriteFile(manifest, stampOut, 0o644)

	// Second export: 6 stories, newer timestamp.
	writeSnapshot(t, candidate, 6, "2026-01-02T00:00:00Z")
	cmd = exec.Command(cli, "publish", "gate", "--candidate", candidate, "--manifest", manifest)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gate (second): %v\n%s", err, out)
	}
	var r2 map[string]any
	json.Unmarshal(out, &r2)
	if r2["verdict"] != "allow" {
		t.Errorf("S1 second: verdict = %v, want allow", r2["verdict"])
	}

	// Third export: 7 stories, newer timestamp.
	writeSnapshot(t, candidate, 7, "2026-01-03T00:00:00Z")
	cmd = exec.Command(cli, "publish", "gate", "--candidate", candidate, "--manifest", manifest)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gate (third): %v\n%s", err, out)
	}
	var r3 map[string]any
	json.Unmarshal(out, &r3)
	if r3["verdict"] != "allow" {
		t.Errorf("S1 third: verdict = %v, want allow", r3["verdict"])
	}
}

// S2: Empty export after a real one. Gate should deny (zero-over-N), exit 3.
func TestPublishGate_S2_EmptyExportAfterReal(t *testing.T) {
	t.Parallel()
	cli := buildCLI(t)
	tmpDir := t.TempDir()
	candidate := filepath.Join(tmpDir, "homepage.json")
	manifest := filepath.Join(tmpDir, "deployed.json")

	// First: 10 stories.
	writeSnapshot(t, candidate, 10, "2026-01-01T00:00:00Z")
	stampCmd := exec.Command(cli, "publish", "stamp", "--candidate", candidate, "--commit", "abc123")
	stampOut, _ := stampCmd.CombinedOutput()
	os.WriteFile(manifest, stampOut, 0o644)

	// Second: 0 stories (empty export).
	writeSnapshot(t, candidate, 0, "2026-01-02T00:00:00Z")
	cmd := exec.Command(cli, "publish", "gate", "--candidate", candidate, "--manifest", manifest)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("S2: expected error for zero-over-N")
	}
	// Exit code 3 (deny).
	if ee, ok := err.(*exec.ExitError); ok {
		if ee.ExitCode() != 3 {
			t.Errorf("S2: exit code = %d, want 3", ee.ExitCode())
		}
	}
	var r map[string]any
	json.Unmarshal(out, &r)
	if r["verdict"] != "deny" {
		t.Errorf("S2: verdict = %v, want deny", r["verdict"])
	}
}

// S3: Stale candidate (older generatedAt). Gate should deny.
func TestPublishGate_S3_StaleCandidate(t *testing.T) {
	t.Parallel()
	cli := buildCLI(t)
	tmpDir := t.TempDir()
	candidate := filepath.Join(tmpDir, "homepage.json")
	manifest := filepath.Join(tmpDir, "deployed.json")

	// Manifest: newer timestamp.
	writeSnapshot(t, candidate, 5, "2026-01-02T00:00:00Z")
	stampCmd := exec.Command(cli, "publish", "stamp", "--candidate", candidate, "--commit", "abc123")
	stampOut, _ := stampCmd.CombinedOutput()
	os.WriteFile(manifest, stampOut, 0o644)

	// Candidate: older timestamp.
	writeSnapshot(t, candidate, 5, "2026-01-01T00:00:00Z")
	cmd := exec.Command(cli, "publish", "gate", "--candidate", candidate, "--manifest", manifest)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("S3: expected error for stale candidate")
	}
	if ee, ok := err.(*exec.ExitError); ok {
		if ee.ExitCode() != 3 {
			t.Errorf("S3: exit code = %d, want 3", ee.ExitCode())
		}
	}
	var r map[string]any
	json.Unmarshal(out, &r)
	if r["verdict"] != "deny" {
		t.Errorf("S3: verdict = %v, want deny", r["verdict"])
	}
}

// S4: Identical content. Gate should skip-identical.
func TestPublishGate_S4_IdenticalContent(t *testing.T) {
	t.Parallel()
	cli := buildCLI(t)
	tmpDir := t.TempDir()
	candidate := filepath.Join(tmpDir, "homepage.json")
	manifest := filepath.Join(tmpDir, "deployed.json")

	// Stamp manifest from candidate.
	writeSnapshot(t, candidate, 5, "2026-01-01T00:00:00Z")
	stampCmd := exec.Command(cli, "publish", "stamp", "--candidate", candidate, "--commit", "abc123")
	stampOut, _ := stampCmd.CombinedOutput()
	os.WriteFile(manifest, stampOut, 0o644)

	// Same candidate (identical content).
	cmd := exec.Command(cli, "publish", "gate", "--candidate", candidate, "--manifest", manifest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("S4: unexpected error: %v\n%s", err, out)
	}
	var r map[string]any
	json.Unmarshal(out, &r)
	if r["verdict"] != "skip-identical" {
		t.Errorf("S4: verdict = %v, want skip-identical", r["verdict"])
	}
}

// S5: No manifest (never deployed). Gate should allow (first deploy).
func TestPublishGate_S5_NoManifest(t *testing.T) {
	t.Parallel()
	cli := buildCLI(t)
	tmpDir := t.TempDir()
	candidate := filepath.Join(tmpDir, "homepage.json")

	// No manifest file (never deployed).
	writeSnapshot(t, candidate, 5, "2026-01-01T00:00:00Z")
	cmd := exec.Command(cli, "publish", "gate", "--candidate", candidate)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("S5: unexpected error: %v\n%s", err, out)
	}
	var r map[string]any
	json.Unmarshal(out, &r)
	if r["verdict"] != "allow" {
		t.Errorf("S5: verdict = %v, want allow (no manifest)", r["verdict"])
	}
}

// S6: Skeleton render deny (issue #250 / PUB-04 AC6). A valid JSON export with
// 5 stories but a rendered index.html with 0 story cards must be denied by
// verify-html (exit 3). This is the #229 failure mode: the gate compares JSON,
// verify-html compares the rendered artifact.
func TestPublishVerifyHTML_S6_SkeletonRender_Deny(t *testing.T) {
	t.Parallel()
	cli := buildCLI(t)
	tmpDir := t.TempDir()

	// Candidate: 5 stories (a "valid" JSON export).
	candidate := filepath.Join(tmpDir, "homepage.json")
	writeSnapshot(t, candidate, 5, "2026-01-01T00:00:00Z")

	// Rendered site: an index.html with a freshness note but 0 story cards
	// (a skeleton render — the data did not make it into the HTML).
	publicDir := filepath.Join(tmpDir, "public")
	if err := os.MkdirAll(publicDir, 0o755); err != nil {
		t.Fatalf("mkdirt public: %v", err)
	}
	indexPath := filepath.Join(publicDir, "index.html")
	indexHTML := `<html><body><p class="update-note">Data as of ` +
		`<time datetime="2026-01-01T00:00:00Z">Jan 1</time></time></p>` +
		`<main><!-- no story cards rendered --></main></body></html>`
	if err := os.WriteFile(indexPath, []byte(indexHTML), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	cmd := exec.Command(cli, "publish", "verify-html", "--public", publicDir, "--candidate", candidate)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("S6: expected error for skeleton render (exit 3)")
	}
	if ee, ok := err.(*exec.ExitError); ok {
		if ee.ExitCode() != 3 {
			t.Errorf("S6: exit code = %d, want 3", ee.ExitCode())
		}
	}
	var r map[string]any
	if jerr := json.Unmarshal(out, &r); jerr != nil {
		t.Fatalf("S6: parse verify output: %v (output: %s)", jerr, out)
	}
	if r["pass"] != false {
		t.Errorf("S6: pass = %v, want false", r["pass"])
	}
}

// S7: Rendered pass (issue #250 / PUB-04 AC6). A rendered site with 5 story
// cards matching the candidate and a matching freshness label passes
// (exit 0).
func TestPublishVerifyHTML_S7_RenderedPass(t *testing.T) {
	t.Parallel()
	cli := buildCLI(t)
	tmpDir := t.TempDir()

	candidate := filepath.Join(tmpDir, "homepage.json")
	writeSnapshot(t, candidate, 5, "2026-01-01T00:00:00Z")

	publicDir := filepath.Join(tmpDir, "public")
	if err := os.MkdirAll(publicDir, 0o755); err != nil {
		t.Fatalf("mkdir public: %v", err)
	}
	indexPath := filepath.Join(publicDir, "index.html")
	var cards string
	for i := 0; i < 5; i++ {
		cards += `<article class="story-card"><h3>S</h3></article>`
	}
	indexHTML := `<html><body><p class="update-note">Data as of ` +
		`<time datetime="2026-01-01T00:00:00Z">Jan 1</time></time></p>` +
		`<main>` + cards + `</main></body></html>`
	if err := os.WriteFile(indexPath, []byte(indexHTML), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	cmd := exec.Command(cli, "publish", "verify-html", "--public", publicDir, "--candidate", candidate)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("S7: expected pass (exit 0), got %v (output: %s)", err, out)
	}
	var r map[string]any
	json.Unmarshal(out, &r)
	if r["pass"] != true {
		t.Errorf("S7: pass = %v, want true", r["pass"])
	}
}
