package snapshot

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// --- fixtures -------------------------------------------------------------

func validSnapshotJSON(t *testing.T, generatedAt string) []byte {
	t.Helper()
	s := domain.Snapshot{
		Version:     domain.SnapshotVersion,
		GeneratedAt: generatedAt,
		Stories: []domain.SnapshotStory{
			{
				ID:               "s1",
				Title:            "First story",
				Summary:          "A clean summary",
				FirstPublishedAt: "2026-11-25T00:00:00Z",
				LastPublishedAt:  "2026-11-25T01:00:00Z",
				EvidenceLevel:    domain.EvidenceConfirmed,
				Publishers:       []domain.PublisherRef{{ID: "p1", Name: "P", URL: "https://news.example.org/a"}},
			},
			{
				ID:               "s2",
				Title:            "Second story",
				Summary:          "Another clean summary",
				FirstPublishedAt: "2026-11-25T02:00:00Z",
				LastPublishedAt:  "2026-11-25T03:00:00Z",
				EvidenceLevel:    domain.EvidenceUnverified,
				Publishers:       []domain.PublisherRef{{ID: "p2", Name: "Q", URL: "https://news.example.org/b"}},
			},
		},
		Latest: []domain.SnapshotArticle{{SourceID: "p1", SourceName: "P", Title: "t", URL: "https://news.example.org/a", PublishedAt: "2026-11-25T01:00:00Z"}},
		Teams:  []domain.SnapshotTeam{{Slug: "team-a", Name: "A", ShortName: "A", Abbreviation: "A"}},
	}
	b, err := s.Render()
	require.NoError(t, err)
	return b
}

func emptySnapshotJSON(t *testing.T) []byte {
	t.Helper()
	s := domain.Snapshot{
		Version:     domain.SnapshotVersion,
		GeneratedAt: "2026-11-26T12:00:00Z",
		Stories:     []domain.SnapshotStory{},
		Latest:      []domain.SnapshotArticle{},
		Teams:       []domain.SnapshotTeam{},
	}
	b, err := s.Render()
	require.NoError(t, err)
	return b
}

// writeWorkspace builds a minimal Hugo workspace (dataDir + storiesDir).
func writeWorkspace(t *testing.T, root string, generatedAt string, extraPages map[string]string) (string, string) {
	t.Helper()
	dataDir := filepath.Join(root, "web", "data")
	storiesDir := filepath.Join(root, "web", "content", "stories")
	require.NoError(t, os.MkdirAll(dataDir, 0o750))
	require.NoError(t, os.MkdirAll(storiesDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "homepage.json"), validSnapshotJSON(t, generatedAt), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "feeds.json"), []byte(`{"version":"1","feeds":[]}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "heartbeat.json"), []byte(`{"version":"1","lastRun":""}`), 0o644))
	pages := map[string]string{
		"s1.md": "---\ntitle: \"First\"\ngenerator: huddlescope\n---\n\n# First\n",
		"s2.md": "---\ntitle: \"Second\"\ngenerator: huddlescope\n---\n\n# Second\n",
	}
	for k, v := range extraPages {
		pages[k] = v
	}
	for name, content := range pages {
		require.NoError(t, os.WriteFile(filepath.Join(storiesDir, name), []byte(content), 0o644))
	}
	return dataDir, storiesDir
}

// buildArchive writes a raw .tar.gz with the given members (unordered).
func buildArchive(t *testing.T, out string, members map[string][]byte, types map[string]byte) {
	t.Helper()
	f, err := os.Create(out)
	require.NoError(t, err)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for name, data := range members {
		typeflag := byte(tar.TypeReg)
		if tf, ok := types[name]; ok {
			typeflag = tf
		}
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data)), Typeflag: typeflag}))
		if typeflag == byte(tar.TypeReg) {
			_, err := tw.Write(data)
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	require.NoError(t, f.Close())
}

// readMembers reads back the raw members of an archive (test helper).
func readMembers(t *testing.T, path string) map[string][]byte {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	require.NoError(t, err)
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	out := map[string][]byte{}
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		var buf []byte
		buf, err = io.ReadAll(tr)
		require.NoError(t, err)
		out[hdr.Name] = buf
	}
	return out
}

// --- Package --------------------------------------------------------------

func TestPackage_DeterministicAndByteIdentical(t *testing.T) {
	root := t.TempDir()
	dataDir, storiesDir := writeWorkspace(t, root, "2026-11-26T12:00:00Z", nil)
	out1 := filepath.Join(t.TempDir(), "site-snapshot")
	out2 := filepath.Join(t.TempDir(), "site-snapshot")

	m1, warnings, err := Package(dataDir, storiesDir, out1, PackageMeta{LastIngestAt: "2026-11-26T11:00:00Z", SourceCommit: "abc123"})
	require.NoError(t, err)
	require.Empty(t, warnings)

	b1, err := os.ReadFile(out1 + ".tar.gz")
	require.NoError(t, err)

	// Second packaging of the identical workspace: byte-identical archive,
	// same snapshotId.
	m2, _, err := Package(dataDir, storiesDir, out2, PackageMeta{LastIngestAt: "2026-11-26T11:00:00Z", SourceCommit: "abc123"})
	require.NoError(t, err)
	b2, err := os.ReadFile(out2 + ".tar.gz")
	require.NoError(t, err)

	assert.Equal(t, b1, b2, "identical inputs must produce byte-identical archives")
	assert.Equal(t, m1.SnapshotID, m2.SnapshotID)
	assert.Equal(t, 2, m1.Stories)
	assert.Equal(t, 2, m1.StoryPages)
	assert.Equal(t, 1, m1.Latest)
	assert.Equal(t, 1, m1.Teams)
	assert.NotEmpty(t, m1.SnapshotID)
	assert.Empty(t, m1.EmptyReason)

	// Same content, different generatedAt: same snapshotId (AC-1).
	root2 := t.TempDir()
	dataDir2, storiesDir2 := writeWorkspace(t, root2, "2026-11-26T13:30:00Z", nil)
	out3 := filepath.Join(t.TempDir(), "site-snapshot")
	m3, _, err := Package(dataDir2, storiesDir2, out3, PackageMeta{})
	require.NoError(t, err)
	assert.Equal(t, m1.SnapshotID, m3.SnapshotID, "snapshotId must be content-derived (generatedAt excluded)")

	// No temp files may survive.
	for _, dir := range []string{filepath.Dir(out1), filepath.Dir(out2), filepath.Dir(out3)} {
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		for _, e := range entries {
			assert.False(t, strings.HasSuffix(e.Name(), ".tmp"), "temp file %s survived", e.Name())
		}
	}
}

func TestPackage_EmptyWorkspace(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "web", "data")
	require.NoError(t, os.MkdirAll(dataDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "homepage.json"), emptySnapshotJSON(t), 0o644))

	out := filepath.Join(t.TempDir(), "site-snapshot")
	m, _, err := Package(dataDir, "", out, PackageMeta{})
	require.NoError(t, err)
	assert.Equal(t, 0, m.Stories)
	assert.NotEmpty(t, m.EmptyReason, "zero stories must carry an emptyReason")
	_, _, err = Validate(out + ".tar.gz")
	assert.NoError(t, err, "empty archive must validate")
}

func TestPackage_MissingHomepage(t *testing.T) {
	dataDir := t.TempDir()
	storiesDir := t.TempDir()
	_, _, err := Package(dataDir, storiesDir, filepath.Join(t.TempDir(), "s"), PackageMeta{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "homepage.json")
}

func TestPackage_InvalidWorkspaceSnapshot(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	require.NoError(t, os.MkdirAll(dataDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "homepage.json"), []byte(`{not json`), 0o644))
	_, _, err := Package(dataDir, "", filepath.Join(t.TempDir(), "s"), PackageMeta{})
	assert.Error(t, err)
}

// --- Validate -------------------------------------------------------------

func validArchive(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dataDir, storiesDir := writeWorkspace(t, root, "2026-11-26T12:00:00Z", nil)
	out := filepath.Join(t.TempDir(), "site-snapshot")
	_, _, err := Package(dataDir, storiesDir, out, PackageMeta{})
	require.NoError(t, err)
	return out + ".tar.gz"
}

func TestValidate_Valid(t *testing.T) {
	archive := validArchive(t)
	m, warnings, err := Validate(archive)
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Equal(t, domain.SnapshotManifestVersion, m.Version)
	assert.Equal(t, 2, m.Stories)
	assert.Equal(t, 2, m.StoryPages)
}

func TestValidate_WarningsForOptionalMembers(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	storiesDir := filepath.Join(root, "stories")
	require.NoError(t, os.MkdirAll(dataDir, 0o750))
	require.NoError(t, os.MkdirAll(storiesDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "homepage.json"), validSnapshotJSON(t, "2026-11-26T12:00:00Z"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(storiesDir, "s1.md"), []byte("# First\n"), 0o644))

	out := filepath.Join(t.TempDir(), "s")
	_, warnings, err := Package(dataDir, storiesDir, out, PackageMeta{})
	require.NoError(t, err)
	joined := strings.Join(warnings, " ")
	assert.Contains(t, joined, "feeds.json")
	assert.Contains(t, joined, "heartbeat.json")

	_, vwarnings, err := Validate(out + ".tar.gz")
	require.NoError(t, err)
	assert.NotEmpty(t, vwarnings)
}

func TestValidate_TruncatedArchive(t *testing.T) {
	archive := validArchive(t)
	data, err := os.ReadFile(archive)
	require.NoError(t, err)
	corrupted := filepath.Join(t.TempDir(), "truncated.tar.gz")
	require.NoError(t, os.WriteFile(corrupted, data[:len(data)/2], 0o644))
	_, _, err = Validate(corrupted)
	assert.Error(t, err)
}

func TestValidate_InvalidJSONHomepage(t *testing.T) {
	archive := validArchive(t)
	members := readMembers(t, archive)
	broken := []byte(`{broken`)
	members["data/homepage.json"] = broken
	// Declare every member with the real sha so the checksum/membership
	// checks pass and the failure is the JSON parse itself.
	var files []domain.ManifestFile
	for path, data := range members {
		if path == ManifestMember {
			continue
		}
		files = append(files, domain.ManifestFile{Path: path, SHA256: sha256Hex(data)})
	}
	emptySnap := domain.Snapshot{
		Version: domain.SnapshotVersion, GeneratedAt: "2026-11-26T12:00:00Z",
		Stories: []domain.SnapshotStory{}, Latest: []domain.SnapshotArticle{}, Teams: []domain.SnapshotTeam{},
	}
	manifest := domain.BuildSnapshotManifest(emptySnap, files, 2, "", "")
	mb, err := json.Marshal(manifest)
	require.NoError(t, err)
	members["manifest.json"] = mb
	buildArchive(t, archive, members, nil)
	_, _, err = Validate(archive)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

func TestValidate_ChecksumMismatch(t *testing.T) {
	archive := validArchive(t)
	members := readMembers(t, archive)
	// Corrupt the member bytes; the manifest sha no longer matches.
	members["data/feeds.json"] = []byte(`{"version":"1","feeds":[]}` + "x")
	buildArchive(t, archive, members, nil)
	_, _, err := Validate(archive)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum mismatch")
}

func TestValidate_MixedGenerations(t *testing.T) {
	archive := validArchive(t)
	members := readMembers(t, archive)
	// Replace homepage.json with different content than the manifest ID was
	// computed from (mixed generations across two exports).
	members["data/homepage.json"] = validSnapshotJSON(t, "2026-11-26T12:00:00Z")
	var manifest domain.SnapshotManifest
	require.NoError(t, json.Unmarshal(members["manifest.json"], &manifest))
	manifest.SnapshotID = strings.Repeat("0", 64)
	b, err := json.Marshal(manifest)
	require.NoError(t, err)
	members["manifest.json"] = b
	buildArchive(t, archive, members, nil)
	_, _, err = Validate(archive)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mixed generations")
}

func TestValidate_SchemaMismatch(t *testing.T) {
	archive := validArchive(t)
	members := readMembers(t, archive)
	var manifest domain.SnapshotManifest
	require.NoError(t, json.Unmarshal(members["manifest.json"], &manifest))
	manifest.Version = "2"
	b, err := json.Marshal(manifest)
	require.NoError(t, err)
	members["manifest.json"] = b
	buildArchive(t, archive, members, nil)
	_, _, err = Validate(archive)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version")
}

func TestValidate_CountMismatchManifestVsFile(t *testing.T) {
	archive := validArchive(t)
	members := readMembers(t, archive)
	var manifest domain.SnapshotManifest
	require.NoError(t, json.Unmarshal(members["manifest.json"], &manifest))
	manifest.Stories = 5 // manifest says 5, data file says 2
	b, err := json.Marshal(manifest)
	require.NoError(t, err)
	members["manifest.json"] = b
	buildArchive(t, archive, members, nil)
	_, _, err = Validate(archive)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "count mismatch")
}

func TestValidate_NonEmptyRule(t *testing.T) {
	// Manifest declares stories, data file has none: never silently empty.
	empty := emptySnapshotJSON(t)
	var snapEmpty domain.Snapshot
	require.NoError(t, json.Unmarshal(empty, &snapEmpty))
	manifest := domain.BuildSnapshotManifest(domain.Snapshot{
		Version: domain.SnapshotVersion, GeneratedAt: "2026-11-26T12:00:00Z",
		Stories: []domain.SnapshotStory{{ID: "x", Title: "t", Summary: "s",
			FirstPublishedAt: "2026-11-25T00:00:00Z", LastPublishedAt: "2026-11-25T00:00:00Z",
			Publishers: []domain.PublisherRef{{ID: "p", Name: "P", URL: "https://x.org/a"}}}},
		Latest: []domain.SnapshotArticle{}, Teams: []domain.SnapshotTeam{},
	}, nil, 0, "", "")
	_ = snapEmpty
	mb, err := json.Marshal(manifest)
	require.NoError(t, err)
	archive := filepath.Join(t.TempDir(), "a.tar.gz")
	buildArchive(t, archive, map[string][]byte{
		"manifest.json":         mb,
		"data/homepage.json":    empty,
		"content/stories/s1.md": []byte("# x\n"),
	}, nil)
	_, _, err = Validate(archive)
	require.Error(t, err)
}

func TestValidate_MissingRequiredMembers(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "a.tar.gz")
	buildArchive(t, archive, map[string][]byte{
		"manifest.json": []byte(`{}`),
		"feeds.json":    []byte(`{}`), // wrong path (no data/ prefix)
	}, nil)
	_, _, err := Validate(archive)
	require.Error(t, err)
}

func TestValidate_PathTraversalMember(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "a.tar.gz")
	buildArchive(t, archive, map[string][]byte{
		"manifest.json": []byte(`{}`),
		"../evil.txt":   []byte("bad"),
	}, nil)
	_, _, err := Validate(archive)
	require.Error(t, err)
}

func TestValidate_SymlinkMember(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "a.tar.gz")
	buildArchive(t, archive,
		map[string][]byte{"manifest.json": []byte(`{}`), "data/homepage.json": []byte(`{}`)},
		map[string]byte{"data/homepage.json": tar.TypeSymlink})
	_, _, err := Validate(archive)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a regular file")
}

func TestValidate_UnexpectedDataMember(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "a.tar.gz")
	buildArchive(t, archive, map[string][]byte{
		"manifest.json":          []byte(`{}`),
		"data/deployed.json":     []byte(`{}`),
		"content/stories/a/b.md": []byte("nested"),
	}, nil)
	_, _, err := Validate(archive)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected tar member")
}

func TestValidate_DuplicateMember(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "a.tar.gz")
	// buildArchive uses a map (unique keys); write the duplicate manually.
	f, err := os.Create(archive)
	require.NoError(t, err)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for i := 0; i < 2; i++ {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0o644, Size: 2}))
		_, _ = tw.Write([]byte(`{}`))
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	require.NoError(t, f.Close())
	_, _, err = Validate(archive)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")
}

// consistentManifest rebuilds manifest.json from the given members so the
// manifest stays checksum-consistent (lets tests target a later validation
// stage without tripping the checksum check first).
func consistentManifest(t *testing.T, members map[string][]byte) []byte {
	t.Helper()
	var snap domain.Snapshot
	require.NoError(t, json.Unmarshal(members["data/homepage.json"], &snap))
	var files []domain.ManifestFile
	storyPages := 0
	for path, data := range members {
		if path == ManifestMember {
			continue
		}
		files = append(files, domain.ManifestFile{Path: path, SHA256: sha256Hex(data)})
		if strings.HasPrefix(path, storiesPrefix) {
			storyPages++
		}
	}
	m := domain.BuildSnapshotManifest(snap, files, storyPages, "2026-11-26T11:00:00Z", "abc123")
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return b
}

func TestValidate_SecretLeakage(t *testing.T) {
	archive := validArchive(t)
	members := readMembers(t, archive)
	members["content/stories/s1.md"] = []byte("# First\n\ndatabase_url=postgres://user:pass@host/db\n")
	members["manifest.json"] = consistentManifest(t, members)
	buildArchive(t, archive, members, nil)
	_, _, err := Validate(archive)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "secrets")
}

func TestValidate_FileURL(t *testing.T) {
	archive := validArchive(t)
	members := readMembers(t, archive)
	hp := `{"version":"1","generatedAt":"2026-11-26T12:00:00Z",
	"stories":[{"id":"s1","title":"T","summary":"S","firstPublishedAt":"2026-11-25T00:00:00Z","lastPublishedAt":"2026-11-25T00:00:00Z","score":1,"evidenceLevel":"confirmed","publishers":[{"id":"p1","name":"P","url":"file:///etc/passwd"}]}],
	"latest":[],"teams":[]}`
	members["data/homepage.json"] = []byte(hp)
	members["manifest.json"] = consistentManifest(t, members)
	buildArchive(t, archive, members, nil)
	_, _, verr := Validate(archive)
	require.Error(t, verr)
	assert.Contains(t, verr.Error(), "http://")
}

func TestValidate_DemoMarker(t *testing.T) {
	archive := validArchive(t)
	members := readMembers(t, archive)
	members["content/stories/s2.md"] = []byte("# Second\n\nsee seed-run-01 for details\n")
	members["manifest.json"] = consistentManifest(t, members)
	buildArchive(t, archive, members, nil)
	_, _, err := Validate(archive)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "demo marker")
}

func TestValidate_SizeCap(t *testing.T) {
	archive := validArchive(t) // package first at the default cap
	old := MaxArchiveBytes
	MaxArchiveBytes = 10
	defer func() { MaxArchiveBytes = old }()
	_, _, err := Validate(archive)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "size cap")
}

func TestValidate_NotAnArchive(t *testing.T) {
	p := filepath.Join(t.TempDir(), "plain.txt")
	require.NoError(t, os.WriteFile(p, []byte("hello"), 0o644))
	_, _, err := Validate(p)
	assert.Error(t, err)
}

func TestValidate_MissingFile(t *testing.T) {
	_, _, err := Validate(filepath.Join(t.TempDir(), "nope.tar.gz"))
	assert.Error(t, err)
}

// --- Restore --------------------------------------------------------------

func TestRestore_ValidRoundTrip(t *testing.T) {
	root := t.TempDir()
	srcData, srcStories := writeWorkspace(t, root, "2026-11-26T12:00:00Z", nil)
	out := filepath.Join(t.TempDir(), "site-snapshot")
	_, _, err := Package(srcData, srcStories, out, PackageMeta{})
	require.NoError(t, err)
	archive := out + ".tar.gz"

	// Target workspace: different content + a stale generated page + a
	// hand-authored page that must be preserved.
	dstRoot := t.TempDir()
	dstData := filepath.Join(dstRoot, "data")
	dstStories := filepath.Join(dstRoot, "stories")
	require.NoError(t, os.MkdirAll(dstData, 0o750))
	require.NoError(t, os.MkdirAll(dstStories, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dstData, "homepage.json"), []byte(`{"version":"0","old":true}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dstStories, "stale-page.md"), []byte("---\ntitle: \"Stale\"\ngenerator: huddlescope\n---\n\n# Stale\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dstStories, "manual.md"), []byte("# Hand authored, no marker\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dstStories, "_index.md"), []byte("# Index\n"), 0o644))

	m, _, err := Restore(archive, dstData, dstStories)
	require.NoError(t, err)
	assert.NotEmpty(t, m.SnapshotID)
	assert.Equal(t, "2026-11-26T12:00:00Z", m.GeneratedAt, "original freshness must be reported")

	// Restored files are byte-equal to the source.
	for _, name := range []string{"homepage.json", "feeds.json", "heartbeat.json"} {
		src, err := os.ReadFile(filepath.Join(srcData, name))
		require.NoError(t, err)
		dst, err := os.ReadFile(filepath.Join(dstData, name))
		require.NoError(t, err)
		assert.Equal(t, src, dst, name)
	}
	for _, name := range []string{"s1.md", "s2.md"} {
		src, err := os.ReadFile(filepath.Join(srcStories, name))
		require.NoError(t, err)
		dst, err := os.ReadFile(filepath.Join(dstStories, name))
		require.NoError(t, err)
		assert.Equal(t, src, dst, name)
	}

	// Stale generated page removed; hand-authored + index preserved.
	_, err = os.Stat(filepath.Join(dstStories, "stale-page.md"))
	assert.True(t, os.IsNotExist(err), "stale generated page must be removed")
	for _, keep := range []string{"manual.md", "_index.md"} {
		_, err := os.Stat(filepath.Join(dstStories, keep))
		assert.NoError(t, err, "%s must be preserved", keep)
	}

	// No staging dir or tmp files left behind.
	entries, err := os.ReadDir(dstRoot)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotEqual(t, "data.staging", e.Name())
		assert.False(t, strings.HasSuffix(e.Name(), ".tmp"))
	}
}

func TestRestore_CorruptedArchiveTouchesNothing(t *testing.T) {
	archive := validArchive(t)
	data, err := os.ReadFile(archive)
	require.NoError(t, err)
	bad := filepath.Join(t.TempDir(), "bad.tar.gz")
	require.NoError(t, os.WriteFile(bad, data[:len(data)/3], 0o644))

	dstData := filepath.Join(t.TempDir(), "data")
	dstStories := filepath.Join(t.TempDir(), "stories")
	require.NoError(t, os.MkdirAll(dstData, 0o750))
	require.NoError(t, os.MkdirAll(dstStories, 0o750))
	marker := filepath.Join(dstData, "existing.json")
	require.NoError(t, os.WriteFile(marker, []byte(`{"before":true}`), 0o644))

	_, _, err = Restore(bad, dstData, dstStories)
	require.Error(t, err)

	// Zero files touched: existing content intact, nothing new, no staging.
	data2, err := os.ReadFile(marker)
	require.NoError(t, err)
	assert.Equal(t, []byte(`{"before":true}`), data2)
	entries, err := os.ReadDir(filepath.Dir(marker))
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, strings.HasSuffix(e.Name(), ".tmp"), "%s must not be left behind", e.Name())
		assert.NotEqual(t, "data.staging", e.Name())
	}
	entries, err = os.ReadDir(dstStories)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestRestore_CreatesMissingDirectories(t *testing.T) {
	archive := validArchive(t)
	dstData := filepath.Join(t.TempDir(), "web", "data") // does not exist yet
	dstStories := filepath.Join(t.TempDir(), "web", "content", "stories")
	m, _, err := Restore(archive, dstData, dstStories)
	require.NoError(t, err)
	assert.NotEmpty(t, m.SnapshotID)
	_, err = os.Stat(filepath.Join(dstData, "homepage.json"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(dstStories, "s1.md"))
	assert.NoError(t, err)
}
