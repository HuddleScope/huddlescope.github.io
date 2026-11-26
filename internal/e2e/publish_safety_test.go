//go:build e2e

// Package e2e: publish safety tests (issue #250 / PUB-04). These exercise the
// last-known-good protection contract at the CLI level: the monotonic manifest
// stamp (AC3), the v2 manifest put with binding fields (AC4), and the
// secret-free diagnostic report (AC8). All tests are offline: they set
// HSC_STORAGE_BACKEND=sqlite-file with a temp path and never open Turso or the
// network.
package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// sqliteEnv returns env for an offline sqlite-file manifest store at dbPath.
func sqliteEnv(dbPath string) []string {
	return []string{
		"HSC_STORAGE_BACKEND=sqlite-file",
		"HSC_STORAGE_PATH=" + dbPath,
	}
}

// P-SAFE-1: Monotonic stamp. A late/delayed run with an OLDER candidate must
// not regress the stored manifest. The put is skipped with a warning, and the
// stored generatedAt is preserved (AC3).
func TestPublishManifest_PutMonotonic_LateRun_Skips(t *testing.T) {
	t.Parallel()
	cli := buildCLI(t)
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "storage.db")
	candidateNew := filepath.Join(tmpDir, "new.json")
	candidateOld := filepath.Join(tmpDir, "old.json")

	// Seed: store a manifest with generatedAt 2026-01-03 (the "current" deploy).
	writeSnapshot(t, candidateNew, 5, "2026-01-03T00:00:00Z")
	cmd := exec.Command(cli, "publish", "manifest", "put",
		"--candidate", candidateNew, "--commit", "abc123")
	cmd.Env = append(os.Environ(), sqliteEnv(dbPath)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("seed put: %v (%s)", err, out)
	}

	// Read back the stored generatedAt.
	cmd = exec.Command(cli, "publish", "manifest", "get", "--output", "json")
	cmd.Env = append(os.Environ(), sqliteEnv(dbPath)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("manifest get after seed: %v (%s)", err, out)
	}
	var stored struct {
		Found    bool `json:"found"`
		Manifest struct {
			GeneratedAt string `json:"generatedAt"`
		} `json:"manifest"`
	}
	if jerr := json.Unmarshal(out, &stored); jerr != nil {
		t.Fatalf("parse manifest get: %v (%s)", jerr, out)
	}
	if !stored.Found || stored.Manifest.GeneratedAt != "2026-01-03T00:00:00Z" {
		t.Fatalf("seed: found=%v generatedAt=%q, want found=true 2026-01-03T00:00:00Z", stored.Found, stored.Manifest.GeneratedAt)
	}

	// Late run: an OLDER candidate (2026-01-01) must be skipped (not regress).
	// The put exits 0 (success) with a warning, NOT an error (AC3).
	writeSnapshot(t, candidateOld, 5, "2026-01-01T00:00:00Z")
	cmd = exec.Command(cli, "publish", "manifest", "put",
		"--candidate", candidateOld, "--commit", "def456")
	cmd.Env = append(os.Environ(), sqliteEnv(dbPath)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("P-SAFE-1: late-run put should exit 0 (monotonic skip), got %v (%s)", err, out)
	} else {
		// The output should contain a skip warning.
		if !strings.Contains(string(out), "skipped") && !strings.Contains(string(out), "::warning:") {
			t.Errorf("P-SAFE-1: expected a skip warning in the output, got: %s", out)
		}
	}

	// The late run must NOT regress the stored generatedAt.
	cmd = exec.Command(cli, "publish", "manifest", "get", "--output", "json")
	cmd.Env = append(os.Environ(), sqliteEnv(dbPath)...)
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("manifest get after late run: %v (%s)", err, out)
	}
	stored = struct {
		Found    bool `json:"found"`
		Manifest struct {
			GeneratedAt string `json:"generatedAt"`
		} `json:"manifest"`
	}{}
	json.Unmarshal(out, &stored)
	if stored.Manifest.GeneratedAt != "2026-01-03T00:00:00Z" {
		t.Errorf("P-SAFE-1: late run regressed generatedAt to %q (want 2026-01-03T00:00:00Z)", stored.Manifest.GeneratedAt)
	}
}

// P-SAFE-2: v2 manifest put records the binding fields (AC4). A put with
// --snapshot-id, --last-ingest, --source-commit, --run-id stores all of them,
// and they round-trip through manifest get.
func TestPublishManifest_PutV2_BindingFields(t *testing.T) {
	t.Parallel()
	cli := buildCLI(t)
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "storage.db")
	candidate := filepath.Join(tmpDir, "homepage.json")
	writeSnapshot(t, candidate, 5, "2026-01-01T00:00:00Z")

	cmd := exec.Command(cli, "publish", "manifest", "put",
		"--candidate", candidate, "--commit", "abc123",
		"--snapshot-id", "aabbccdd00112233445566778899aabbccddeeff00112233445566778899aabb",
		"--last-ingest", "2026-01-01T00:00:00Z",
		"--source-commit", "src456",
		"--run-id", "12345")
	cmd.Env = append(os.Environ(), sqliteEnv(dbPath)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("v2 put: %v (%s)", err, out)
	}

	cmd = exec.Command(cli, "publish", "manifest", "get", "--output", "json")
	cmd.Env = append(os.Environ(), sqliteEnv(dbPath)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("manifest get: %v (%s)", err, out)
	}
	var m struct {
		Found    bool `json:"found"`
		Manifest struct {
			Version      string `json:"version"`
			SnapshotID   string `json:"snapshotId"`
			LastIngestAt string `json:"lastIngestAt"`
			SourceCommit string `json:"sourceCommit"`
			RunID        string `json:"runId"`
		} `json:"manifest"`
	}
	if jerr := json.Unmarshal(out, &m); jerr != nil {
		t.Fatalf("parse manifest get: %v (%s)", jerr, out)
	}
	if m.Manifest.Version != "2" {
		t.Errorf("version = %q, want 2", m.Manifest.Version)
	}
	if m.Manifest.SnapshotID == "" {
		t.Error("snapshotId empty after v2 put")
	}
	if m.Manifest.LastIngestAt != "2026-01-01T00:00:00Z" {
		t.Errorf("lastIngestAt = %q", m.Manifest.LastIngestAt)
	}
	if m.Manifest.SourceCommit != "src456" {
		t.Errorf("sourceCommit = %q, want src456", m.Manifest.SourceCommit)
	}
	if m.Manifest.RunID != "12345" {
		t.Errorf("runId = %q, want 12345", m.Manifest.RunID)
	}
}

// P-SAFE-3: The diagnostic report is secret-free (AC8). A report built from
// gate + verify + manifest inputs must NOT contain any DSN, token, or
// credential.
func TestPublishReport_SecretFree(t *testing.T) {
	t.Parallel()
	cli := buildCLI(t)
	tmpDir := t.TempDir()

	// A gate JSON file.
	gateJSON := `{"verdict":"allow","reason":"ok","candidate_stories":5,"deployed_stories":5}`
	gatePath := filepath.Join(tmpDir, "gate.json")
	if err := os.WriteFile(gatePath, []byte(gateJSON), 0o644); err != nil {
		t.Fatalf("write gate: %v", err)
	}
	// A verify JSON file.
	verifyJSON := `{"pass":true,"reason":"ok","story_cards":5,"checks":["ok"]}`
	verifyPath := filepath.Join(tmpDir, "verify.json")
	if err := os.WriteFile(verifyPath, []byte(verifyJSON), 0o644); err != nil {
		t.Fatalf("write verify: %v", err)
	}
	// A manifest JSON the report reads (v2 envelope).
	manifestJSON := `{"version":"2","deployedAt":"2026-01-01T00:00:00Z","generatedAt":"2026-01-01T00:00:00Z","commit":"abc123","contentSha256":"hash123","stories":5,"latest":3}`
	manifestPath := filepath.Join(tmpDir, "deployed.json")
	if err := os.WriteFile(manifestPath, []byte(manifestJSON), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	cmd := exec.Command(cli, "publish", "report",
		"--gate", gatePath,
		"--verify", verifyPath,
		"--manifest", manifestPath,
		"--run-id", "12345",
		"--commit", "abc123",
		"--snapshot-id", "aabbccdd00112233445566778899aabbccddeeff00112233445566778899aabb",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("report: %v (%s)", err, out)
	}

	// The report must be valid JSON.
	var r map[string]any
	if jerr := json.Unmarshal(out, &r); jerr != nil {
		t.Fatalf("report is not valid JSON: %v (output: %s)", jerr, out)
	}

	// The report must NOT contain any secret substrings.
	lower := strings.ToLower(string(out))
	for _, secret := range []string{
		"libsql://", "libsql-wss://", "password",
		"secret", "token", "apikey", "api_key", "dsn",
	} {
		if strings.Contains(lower, secret) {
			t.Errorf("P-SAFE-3: report contains secret substring %q", secret)
		}
	}
}
