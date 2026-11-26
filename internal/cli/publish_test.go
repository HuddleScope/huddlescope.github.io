package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// writeCandidate writes a minimal valid candidate snapshot to a temp file.
func writeCandidate(t *testing.T, stories int, generatedAt string) string {
	t.Helper()
	s := &domain.Snapshot{
		Version:     domain.SnapshotVersion,
		GeneratedAt: generatedAt,
		Stories:     make([]domain.SnapshotStory, 0, stories),
		Latest:      []domain.SnapshotArticle{},
		Teams:       []domain.SnapshotTeam{},
	}
	for i := 0; i < stories; i++ {
		s.Stories = append(s.Stories, domain.SnapshotStory{
			ID:               string(rune('a'+i)) + string(rune('0'+i%10)),
			Title:            "Story " + string(rune('a'+i)),
			Summary:          "Summary " + string(rune('a'+i)),
			FirstPublishedAt: generatedAt,
			LastPublishedAt:  generatedAt,
			Score:            5.0,
			EvidenceLevel:    domain.EvidenceUnknown,
			EvidenceLevelCSS: "unknown",
			Publishers:       []domain.PublisherRef{{ID: "src", Name: "Source", URL: "https://example.com"}},
		})
	}
	b, _ := json.Marshal(s)
	path := filepath.Join(t.TempDir(), "homepage.json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write candidate: %v", err)
	}
	return path
}

// writeManifest writes a manifest to a temp file.
func writeManifest(t *testing.T, m *domain.Snapshot, commit string, stories int, generatedAt string) string {
	t.Helper()
	mf := struct {
		Version       string `json:"version"`
		DeployedAt    string `json:"deployedAt"`
		GeneratedAt   string `json:"generatedAt"`
		Commit        string `json:"commit"`
		ContentSHA256 string `json:"contentSha256"`
		Stories       int    `json:"stories"`
		Latest        int    `json:"latest"`
	}{
		Version:       "1",
		DeployedAt:    "2026-01-01T00:00:00Z",
		GeneratedAt:   generatedAt,
		Commit:        commit,
		ContentSHA256: "hash123",
		Stories:       stories,
		Latest:        3,
	}
	b, _ := json.Marshal(mf)
	path := filepath.Join(t.TempDir(), "deployed.json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return path
}

func TestPublishGateCommand_Allow(t *testing.T) {
	candPath := writeCandidate(t, 5, "2026-01-03T00:00:00Z")
	root := NewRootCommand()
	root.SilenceUsage = true
	root.SilenceErrors = true
	root.SetArgs([]string{"publish", "gate", "--candidate", candPath})
	err := root.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Check that the output contains "allow".
	// (We capture stdout via the command's out writer; for this test we just
	// verify no error was returned.)
}

func TestPublishGateCommand_Deny_ZeroOverN(t *testing.T) {
	candPath := writeCandidate(t, 0, "2026-01-02T00:00:00Z")
	manifestPath := writeManifest(t, nil, "abc123", 10, "2026-01-01T00:00:00Z")

	root := NewRootCommand()
	root.SilenceUsage = true
	root.SilenceErrors = true
	root.SetArgs([]string{"publish", "gate", "--candidate", candPath, "--manifest", manifestPath})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for deny verdict")
	}
	exitErr, ok := err.(*ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != 3 {
		t.Errorf("exit code = %d, want 3", exitErr.ExitCode())
	}
}

func TestPublishGateCommand_MissingCandidate(t *testing.T) {
	root := NewRootCommand()
	root.SilenceUsage = true
	root.SilenceErrors = true
	root.SetArgs([]string{"publish", "gate", "--candidate", "/nonexistent/homepage.json"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for missing candidate")
	}
	exitErr, ok := err.(*ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != 1 {
		t.Errorf("exit code = %d, want 1", exitErr.ExitCode())
	}
}

func TestPublishGateCommand_NoManifest(t *testing.T) {
	// No manifest → allow (first deploy).
	candPath := writeCandidate(t, 5, "2026-01-01T00:00:00Z")
	root := NewRootCommand()
	root.SilenceUsage = true
	root.SilenceErrors = true
	root.SetArgs([]string{"publish", "gate", "--candidate", candPath})
	err := root.Execute()
	if err != nil {
		t.Fatalf("unexpected error (no manifest should allow): %v", err)
	}
}

func TestPublishGateCommand_InvalidCandidate(t *testing.T) {
	// Invalid JSON.
	badPath := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(badPath, []byte("{invalid"), 0o644); err != nil {
		t.Fatalf("write bad json: %v", err)
	}
	root := NewRootCommand()
	root.SilenceUsage = true
	root.SilenceErrors = true
	root.SetArgs([]string{"publish", "gate", "--candidate", badPath})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for invalid candidate")
	}
	exitErr, ok := err.(*ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != 1 {
		t.Errorf("exit code = %d, want 1", exitErr.ExitCode())
	}
}

func TestPublishStampCommand_Success(t *testing.T) {
	candPath := writeCandidate(t, 5, "2026-01-01T00:00:00Z")
	root := NewRootCommand()
	root.SilenceUsage = true
	root.SilenceErrors = true
	root.SetArgs([]string{"publish", "stamp", "--candidate", candPath, "--commit", "abc123"})
	err := root.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPublishStampCommand_MissingCommit(t *testing.T) {
	candPath := writeCandidate(t, 5, "2026-01-01T00:00:00Z")
	root := NewRootCommand()
	root.SilenceUsage = true
	root.SilenceErrors = true
	root.SetArgs([]string{"publish", "stamp", "--candidate", candPath})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for missing --commit")
	}
}

func TestPublishGateCommand_NoSecretsInOutput(t *testing.T) {
	// AC-7: gate output must not contain secret patterns.
	candPath := writeCandidate(t, 5, "2026-01-01T00:00:00Z")

	// Capture stdout.
	var stdout strings.Builder
	root := NewRootCommand()
	root.SilenceUsage = true
	root.SilenceErrors = true
	root.SetOut(&stdout)
	root.SetArgs([]string{"publish", "gate", "--candidate", candPath})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()

	// Check for secret patterns (Turso tokens, DSNs, etc.).
	secretPatterns := []string{
		"turso",
		"TURSO",
		"token",
		"TOKEN",
		"password",
		"PASSWORD",
		"secret",
		"SECRET",
		"ghp_",
		"gho_",
		"Bearer ",
	}
	for _, p := range secretPatterns {
		if strings.Contains(output, p) {
			t.Errorf("gate output contains secret pattern %q: %s", p, output)
		}
	}

	// Verify the output is valid JSON.
	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("gate output is not valid JSON: %v", err)
	}
	if _, ok := result["verdict"]; !ok {
		t.Error("gate output missing verdict field")
	}
}
