package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HuddleScope/huddlescope.github.io/internal/config"
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

// fakeManifestStore is an in-memory domain.DeployedManifestStore for CLI tests
// (issue #247 / PUB-01). It records Put calls so tests can assert on them.
type fakeManifestStore struct {
	m      *domain.DeployedManifest
	putM   *domain.DeployedManifest
	getErr error
	putErr error
}

func (f *fakeManifestStore) Get() (*domain.DeployedManifest, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.m, nil
}

func (f *fakeManifestStore) Put(m domain.DeployedManifest) error {
	if f.putErr != nil {
		return f.putErr
	}
	mm := m
	f.m = &mm
	f.putM = &m
	return nil
}

func (f *fakeManifestStore) Close() error { return nil }

// fakeManifestStoreConstructor returns a NewManifestStoreFunc that always returns
// the same store (ignoring cfg).
func fakeManifestStoreConstructor(s *fakeManifestStore) NewManifestStoreFunc {
	return func(_ config.StorageConfig) (domain.DeployedManifestStore, error) {
		return s, nil
	}
}

func TestPublishManifestGetCommand_Absent(t *testing.T) {
	store := &fakeManifestStore{} // empty (nil m)
	candPath := writeCandidate(t, 1, "2026-01-01T00:00:00Z")
	_ = candPath
	var stdout strings.Builder
	root := NewRootCommand(WithNewManifestStore(fakeManifestStoreConstructor(store)))
	root.SilenceUsage = true
	root.SilenceErrors = true
	root.SetOut(&stdout)
	root.SetArgs([]string{"publish", "manifest", "get"})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got struct {
		Found bool `json:"found"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &got); err != nil {
		t.Fatalf("output not JSON: %v (%q)", err, stdout.String())
	}
	if got.Found {
		t.Error("expected found=false for empty store")
	}
}

func TestPublishManifestGetCommand_Found_WritesOutputFile(t *testing.T) {
	m := domain.DeployedManifest{
		Version:       domain.ManifestVersion,
		DeployedAt:    "2026-01-01T00:00:00Z",
		GeneratedAt:   "2026-01-01T00:00:00Z",
		Commit:        "abc123",
		ContentSHA256: "hash123",
		Stories:       5,
	}
	store := &fakeManifestStore{m: &m}
	outPath := filepath.Join(t.TempDir(), "deployed.json")
	var stdout strings.Builder
	root := NewRootCommand(WithNewManifestStore(fakeManifestStoreConstructor(store)))
	root.SilenceUsage = true
	root.SilenceErrors = true
	root.SetOut(&stdout)
	root.SetArgs([]string{"publish", "manifest", "get", "--output", outPath})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got struct {
		Found    bool                    `json:"found"`
		Manifest domain.DeployedManifest `json:"manifest"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &got); err != nil {
		t.Fatalf("output not JSON: %v", err)
	}
	if !got.Found {
		t.Error("expected found=true")
	}
	if got.Manifest.Commit != "abc123" || got.Manifest.Stories != 5 {
		t.Errorf("manifest mismatch: %+v", got.Manifest)
	}
	// The --output file must contain the manifest.
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read --output file: %v", err)
	}
	var fileManifest domain.DeployedManifest
	// The file contains {"found":true,"manifest":{...}} — parse the manifest.
	var wrapper struct {
		Manifest domain.DeployedManifest `json:"manifest"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		t.Fatalf("--output file not JSON: %v", err)
	}
	if wrapper.Manifest.Commit != "abc123" {
		t.Errorf("--output manifest mismatch: %+v", wrapper.Manifest)
	}
	_ = fileManifest
}

func TestPublishManifestGetCommand_DBError_FailsClosed(t *testing.T) {
	store := &fakeManifestStore{getErr: errBoom}
	var stdout strings.Builder
	root := NewRootCommand(WithNewManifestStore(fakeManifestStoreConstructor(store)))
	root.SilenceUsage = true
	root.SilenceErrors = true
	root.SetOut(&stdout)
	root.SetArgs([]string{"publish", "manifest", "get"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for DB read failure")
	}
	exitErr, ok := err.(*ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != 1 {
		t.Errorf("exit code = %d, want 1", exitErr.ExitCode())
	}
}

func TestPublishManifestPutCommand_Success(t *testing.T) {
	store := &fakeManifestStore{}
	candPath := writeCandidate(t, 4, "2026-01-01T00:00:00Z")
	var stdout strings.Builder
	root := NewRootCommand(WithNewManifestStore(fakeManifestStoreConstructor(store)))
	root.SilenceUsage = true
	root.SilenceErrors = true
	root.SetOut(&stdout)
	root.SetArgs([]string{"publish", "manifest", "put", "--candidate", candPath, "--commit", "def456"})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.putM == nil {
		t.Fatal("expected Put to be called")
	}
	if store.putM.Commit != "def456" {
		t.Errorf("put commit = %q, want def456", store.putM.Commit)
	}
	if store.putM.Stories != 4 {
		t.Errorf("put stories = %d, want 4", store.putM.Stories)
	}
	if store.putM.ContentSHA256 == "" {
		t.Error("put contentSha256 should be set")
	}
}

func TestPublishManifestPutCommand_MissingFlags(t *testing.T) {
	store := &fakeManifestStore{}
	root := NewRootCommand(WithNewManifestStore(fakeManifestStoreConstructor(store)))
	root.SilenceUsage = true
	root.SilenceErrors = true
	root.SetArgs([]string{"publish", "manifest", "put"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected error for missing --candidate/--commit")
	}
}

func TestPublishGateCommand_RequireManifest_NoManifest_Deny(t *testing.T) {
	// --require-manifest with no manifest present → deny (fail closed), even
	// though the candidate has stories (issue #247 / AC-6).
	candPath := writeCandidate(t, 5, "2026-01-01T00:00:00Z")
	var stdout strings.Builder
	root := NewRootCommand()
	root.SilenceUsage = true
	root.SilenceErrors = true
	root.SetOut(&stdout)
	root.SetArgs([]string{"publish", "gate", "--candidate", candPath, "--require-manifest"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected deny error for --require-manifest with no manifest")
	}
	exitErr, ok := err.(*ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != 3 {
		t.Errorf("exit code = %d, want 3 (deny)", exitErr.ExitCode())
	}
	// The JSON verdict should be deny.
	var got struct {
		Verdict string `json:"verdict"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &got); err != nil {
		t.Fatalf("output not JSON: %v", err)
	}
	if got.Verdict != "deny" {
		t.Errorf("verdict = %q, want deny", got.Verdict)
	}
}

// errBoom is a non-nil error for fail-closed tests.
var errBoom = &boomErr{}

type boomErr struct{}

func (boomErr) Error() string { return "boom" }
