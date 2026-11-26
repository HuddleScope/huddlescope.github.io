package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writePackageWorkspace builds a minimal exported Hugo workspace for the
// export package/restore/validate CLI tests (issue #249).
func writePackageWorkspace(t *testing.T, root string) (dataDir, storiesDir string) {
	t.Helper()
	dataDir = filepath.Join(root, "web", "data")
	storiesDir = filepath.Join(root, "web", "content", "stories")
	require.NoError(t, os.MkdirAll(dataDir, 0o750))
	require.NoError(t, os.MkdirAll(storiesDir, 0o750))
	home := `{
	  "version": "1",
	  "generatedAt": "2026-11-26T12:00:00Z",
	  "stories": [
	    {
	      "id": "s1",
	      "title": "First story",
	      "summary": "A clean summary",
	      "firstPublishedAt": "2026-11-25T00:00:00Z",
	      "lastPublishedAt": "2026-11-25T01:00:00Z",
	      "score": 1,
	      "evidenceLevel": "confirmed",
	      "publishers": [{"id": "p1", "name": "P", "url": "https://news.example.org/a"}]
	    }
	  ],
	  "latest": [{"source_id": "p1", "source_name": "P", "title": "t", "url": "https://news.example.org/a", "published_at": "2026-11-25T01:00:00Z"}],
	  "teams": [{"slug": "team-a", "name": "A", "short_name": "A", "abbreviation": "A"}]
	}`
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "homepage.json"), []byte(home), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(storiesDir, "s1.md"), []byte("# First\n"), 0o644))
	return dataDir, storiesDir
}

func TestExportPackageCommand_JSONContract(t *testing.T) {
	var out strings.Builder
	root := t.TempDir()
	dataDir, storiesDir := writePackageWorkspace(t, root)
	outPath := filepath.Join(t.TempDir(), "site-snapshot")

	cmd := NewExportPackageCommand(nil)
	cmd.SetOut(&out)
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{
		"--data-dir", dataDir,
		"--content-dir", storiesDir,
		"--out", outPath,
		"--last-ingest", "2026-11-26T11:00:00Z",
		"--source-commit", "abc123",
		"--output", "json",
	})
	require.NoError(t, cmd.Execute())

	var payload struct {
		SnapshotID   string `json:"snapshotId"`
		Path         string `json:"path"`
		Stories      int    `json:"stories"`
		Latest       int    `json:"latest"`
		Teams        int    `json:"teams"`
		StoryPages   int    `json:"storyPages"`
		GeneratedAt  string `json:"generatedAt"`
		LastIngestAt string `json:"lastIngestAt"`
		SourceCommit string `json:"sourceCommit"`
	}
	require.NoError(t, json.Unmarshal([]byte(out.String()), &payload))
	assert.Len(t, payload.SnapshotID, 64)
	assert.Equal(t, outPath+".tar.gz", payload.Path)
	assert.Equal(t, 1, payload.Stories)
	assert.Equal(t, 1, payload.Latest)
	assert.Equal(t, 1, payload.Teams)
	assert.Equal(t, 1, payload.StoryPages)
	assert.Equal(t, "2026-11-26T12:00:00Z", payload.GeneratedAt)
	assert.Equal(t, "2026-11-26T11:00:00Z", payload.LastIngestAt)
	assert.Equal(t, "abc123", payload.SourceCommit)

	// The archive must exist on disk.
	_, err := os.Stat(payload.Path)
	assert.NoError(t, err)
}

func TestExportPackageCommand_RequiresOut(t *testing.T) {
	cmd := NewExportPackageCommand(nil)
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--out is required")
}

func TestExportPackageCommand_BadOutputFormat(t *testing.T) {
	cmd := NewExportPackageCommand(nil)
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"--out", "x", "--output", "yaml"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--output must be text or json")
}

func TestExportPackageCommand_TextOutput(t *testing.T) {
	var out strings.Builder
	root := t.TempDir()
	dataDir, storiesDir := writePackageWorkspace(t, root)

	cmd := NewExportPackageCommand(nil)
	cmd.SetOut(&out)
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"--data-dir", dataDir, "--content-dir", storiesDir, "--out", filepath.Join(t.TempDir(), "s")})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "snapshot_id=")
	assert.Contains(t, out.String(), "path=")
	assert.Contains(t, out.String(), "stories=1")
}

func TestExportValidateCommand_Valid(t *testing.T) {
	var out strings.Builder
	root := t.TempDir()
	dataDir, storiesDir := writePackageWorkspace(t, root)
	outPath := filepath.Join(t.TempDir(), "site-snapshot")

	require.NoError(t, runPackageForTest(t, dataDir, storiesDir, outPath))

	val := NewExportValidateCommand(nil)
	val.SetOut(&out)
	val.SetErr(&strings.Builder{})
	val.SetArgs([]string{"--artifact", outPath + ".tar.gz"})
	require.NoError(t, val.Execute())

	var payload struct {
		Valid      bool   `json:"valid"`
		SnapshotID string `json:"snapshotId"`
		Stories    int    `json:"stories"`
		Files      int    `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(out.String()), &payload))
	assert.True(t, payload.Valid)
	assert.Len(t, payload.SnapshotID, 64)
	assert.Equal(t, 1, payload.Stories)
	assert.Equal(t, 2, payload.Files) // homepage.json + s1.md
}

func runPackageForTest(t *testing.T, dataDir, storiesDir, outPath string) error {
	t.Helper()
	cmd := NewExportPackageCommand(nil)
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"--data-dir", dataDir, "--content-dir", storiesDir, "--out", outPath})
	return cmd.Execute()
}

func TestExportValidateCommand_InvalidArtifact(t *testing.T) {
	cmd := NewExportValidateCommand(nil)
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"--artifact", filepath.Join(t.TempDir(), "missing.tar.gz")})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validate snapshot")
}

func TestExportValidateCommand_RequiresArtifact(t *testing.T) {
	cmd := NewExportValidateCommand(nil)
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--artifact is required")
}

func TestExportRestoreCommand_RequiresForce(t *testing.T) {
	// --force must be enforced before any I/O: a missing archive and a
	// missing --force both error; the force error must come first.
	cmd := NewExportRestoreCommand(nil)
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"--artifact", filepath.Join(t.TempDir(), "x.tar.gz")})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--force is required")
}

func TestExportRestoreCommand_RequiresArtifact(t *testing.T) {
	cmd := NewExportRestoreCommand(nil)
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"--force"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--artifact is required")
}

func TestExportRestoreCommand_RoundTrip(t *testing.T) {
	var out strings.Builder
	root := t.TempDir()
	dataDir, storiesDir := writePackageWorkspace(t, root)
	outPath := filepath.Join(t.TempDir(), "site-snapshot")
	require.NoError(t, runPackageForTest(t, dataDir, storiesDir, outPath))

	dstRoot := t.TempDir()
	dstData := filepath.Join(dstRoot, "data")
	dstStories := filepath.Join(dstRoot, "stories")

	cmd := NewExportRestoreCommand(nil)
	cmd.SetOut(&out)
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{
		"--artifact", outPath + ".tar.gz",
		"--data-dir", dstData,
		"--content-dir", dstStories,
		"--force",
	})
	require.NoError(t, cmd.Execute())

	s := out.String()
	assert.Contains(t, s, "restored 2 files")
	assert.Contains(t, s, "content is as of 2026-11-26T12:00:00Z")
	assert.Contains(t, s, "snapshot_id=")

	src, err := os.ReadFile(filepath.Join(dataDir, "homepage.json"))
	require.NoError(t, err)
	dst, err := os.ReadFile(filepath.Join(dstData, "homepage.json"))
	require.NoError(t, err)
	assert.Equal(t, src, dst)
	_, err = os.Stat(filepath.Join(dstStories, "s1.md"))
	assert.NoError(t, err)
}

func TestExportRestoreCommand_CorruptedArtifactFails(t *testing.T) {
	root := t.TempDir()
	dataDir, storiesDir := writePackageWorkspace(t, root)
	outPath := filepath.Join(t.TempDir(), "site-snapshot")
	require.NoError(t, runPackageForTest(t, dataDir, storiesDir, outPath))

	archive := outPath + ".tar.gz"
	data, err := os.ReadFile(archive)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(archive, data[:len(data)/2], 0o644))

	dstData := filepath.Join(t.TempDir(), "data")
	cmd := NewExportRestoreCommand(nil)
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"--artifact", archive, "--data-dir", dstData, "--content-dir", filepath.Join(t.TempDir(), "stories"), "--force"})
	err = cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "restore snapshot")
	// Nothing written: the target data dir does not even exist.
	_, statErr := os.Stat(filepath.Join(dstData, "homepage.json"))
	assert.True(t, os.IsNotExist(statErr))
}

// TestExportCommandSubcommands registers all subcommands on export.
func TestExportCommandSubcommands(t *testing.T) {
	cmd := NewExportCommand(nil)
	var uses []string
	for _, sub := range cmd.Commands() {
		uses = append(uses, sub.Use)
	}
	assert.Contains(t, uses, "run")
	assert.Contains(t, uses, "package")
	assert.Contains(t, uses, "restore")
	assert.Contains(t, uses, "validate")
}
