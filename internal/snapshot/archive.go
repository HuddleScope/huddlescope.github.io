// Package snapshot packages, validates and restores versioned Hugo site
// snapshots as workflow artifacts (issue #249 / PUB-03).
//
// An archive is a deterministic .tar.gz containing exactly:
//
//	manifest.json                  versioned manifest (schema pinned to "1")
//	data/homepage.json             the exported snapshot (required)
//	data/feeds.json                site data (optional)
//	data/heartbeat.json            site data (optional)
//	content/stories/*.md           generated story pages
//
// Determinism: members are sorted, tar headers carry a fixed ModTime (the
// snapshot's original generatedAt), zero uid/gid, and empty user/group
// names, and the gzip header is left at Go's fixed defaults — so identical
// workspace files produce byte-identical archives on any OS. Pure file I/O:
// no database access, fully offline-testable.
package snapshot

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// DemoMarkers mirrors the demo-marker grep in
// .github/workflows/refresh-news.yml; a guard test in internal/cli pins the
// workflow list against this constant so the two cannot drift.
var DemoMarkers = []string{
	"example.com",
	"seed-run-",
	"top-free-agents-2024",
	"pro-bowl-2025-format",
	"preseason-week-1-overview",
}

// MaxArchiveBytes caps the total uncompressed size of a site-snapshot
// archive (64 MiB). It is a var so tests can lower it.
var MaxArchiveBytes int64 = 64 << 20

// Archive member names (whitelist enforced on write and read).
const (
	ManifestMember  = "manifest.json"
	homepageMember  = "data/homepage.json"
	feedsMember     = "data/feeds.json"
	heartbeatMember = "data/heartbeat.json"
	dataPrefix      = "data/"
	storiesPrefix   = "content/stories/"
)

// knownDataMembers is the exact set of data/*.json files an archive may hold
// (other workspace files such as deployed.json never enter the archive).
var knownDataMembers = map[string]bool{
	homepageMember:  true,
	feedsMember:     true,
	heartbeatMember: true,
}

// PackageMeta carries caller-provided freshness metadata. Values come from
// the CLI (workflow flags/env), never from a database read inside this
// package.
type PackageMeta struct {
	LastIngestAt string // RFC3339 UTC or "" (→ "unknown")
	SourceCommit string // git SHA or "" (→ "unknown")
}

// member is one archive member (path + bytes).
type member struct {
	path string
	data []byte
}

// Package bundles the already-written Hugo workspace files into a
// deterministic .tar.gz at outPath+".tar.gz". dataDir must contain
// homepage.json (required; feeds.json and heartbeat.json are optional);
// storiesDir contains the generated story pages (*.md).
//
// The generatedAt used for the manifest and the fixed tar ModTime is taken
// from the workspace homepage.json content itself, so identical workspace
// files produce byte-identical archives (AC-1). The archive is validated
// after writing so a writer bug is caught before upload (AC-2).
func Package(dataDir, storiesDir, outPath string, meta PackageMeta) (domain.SnapshotManifest, []string, error) {
	var files []domain.ManifestFile
	var members []member
	var warnings []string

	// Data members: homepage.json required, feeds/heartbeat optional.
	dataFiles := []struct {
		name     string
		required bool
	}{
		{"homepage.json", true},
		{"feeds.json", false},
		{"heartbeat.json", false},
	}
	for _, df := range dataFiles {
		path := filepath.Join(dataDir, df.name)
		data, err := os.ReadFile(path) // #nosec G304: df.name is a fixed whitelist (homepage/feeds/heartbeat.json), operator-controlled dataDir
		if err != nil {
			if os.IsNotExist(err) {
				if df.required {
					return domain.SnapshotManifest{}, nil, fmt.Errorf("read %s: %w", path, err)
				}
				warnings = append(warnings, "optional data file missing: "+df.name)
				continue
			}
			return domain.SnapshotManifest{}, nil, fmt.Errorf("read %s: %w", path, err)
		}
		mpath := dataPrefix + df.name
		members = append(members, member{path: mpath, data: data})
		files = append(files, domain.ManifestFile{Path: mpath, SHA256: sha256Hex(data)})
	}

	// Story pages: every regular *.md file in storiesDir, sorted by name.
	storyPages := 0
	if storiesDir != "" {
		entries, err := os.ReadDir(storiesDir)
		if err != nil {
			if os.IsNotExist(err) {
				warnings = append(warnings, "stories directory missing: "+storiesDir)
			} else {
				return domain.SnapshotManifest{}, nil, fmt.Errorf("read stories dir: %w", err)
			}
		} else {
			var names []string
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
					continue
				}
				names = append(names, e.Name())
			}
			sort.Strings(names)
			for _, n := range names {
				data, err := os.ReadFile(filepath.Join(storiesDir, n)) // #nosec G304: n is an entry name from os.ReadDir of storiesDir
				if err != nil {
					return domain.SnapshotManifest{}, nil, fmt.Errorf("read story page %s: %w", n, err)
				}
				mpath := storiesPrefix + n
				members = append(members, member{path: mpath, data: data})
				files = append(files, domain.ManifestFile{Path: mpath, SHA256: sha256Hex(data)})
				storyPages++
			}
		}
	}

	// Parse + validate the workspace snapshot (the generatedAt source).
	hp, err := memberData(members, homepageMember)
	if err != nil {
		return domain.SnapshotManifest{}, nil, err
	}
	var snap domain.Snapshot
	if err := json.Unmarshal(hp, &snap); err != nil {
		return domain.SnapshotManifest{}, nil, fmt.Errorf("parse %s: %w", homepageMember, err)
	}
	if err := snap.Validate(); err != nil {
		return domain.SnapshotManifest{}, nil, fmt.Errorf("validate workspace snapshot: %w", err)
	}

	manifest := domain.BuildSnapshotManifest(snap, files, storyPages, meta.LastIngestAt, meta.SourceCommit)
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return domain.SnapshotManifest{}, nil, fmt.Errorf("marshal manifest: %w", err)
	}

	members = append([]member{{path: ManifestMember, data: manifestBytes}}, members...)
	sortMembers(members)

	if err := writeArchive(outPath+".tar.gz", members, tarModTime(snap.GeneratedAt)); err != nil {
		return domain.SnapshotManifest{}, nil, fmt.Errorf("write archive: %w", err)
	}

	// Validate the archive we just wrote: a writer bug must be caught here,
	// before any upload.
	if _, _, err := Validate(outPath + ".tar.gz"); err != nil {
		_ = os.Remove(outPath + ".tar.gz")
		return domain.SnapshotManifest{}, nil, fmt.Errorf("archive failed post-write validation: %w", err)
	}
	return manifest, warnings, nil
}

// Validate reads an archive and enforces every integrity check, fail-closed
// (AC-2/AC-4). Any anomaly — truncated data, off-whitelist or path-traversal
// members, symlinks, oversized payloads, schema mismatch, checksum mismatch,
// mixed generations, secret patterns, unsafe links, or demo markers — is a
// hard error. Warnings (optional files missing) do not fail validation.
func Validate(archivePath string) (domain.SnapshotManifest, []string, error) {
	members, warnings, err := readArchiveMembers(archivePath)
	if err != nil {
		return domain.SnapshotManifest{}, nil, err
	}

	var manifestBytes, snapData []byte
	for _, m := range members {
		switch m.path {
		case ManifestMember:
			manifestBytes = m.data
		case homepageMember:
			snapData = m.data
		}
	}
	if manifestBytes == nil {
		return domain.SnapshotManifest{}, warnings, fmt.Errorf("missing required member %s", ManifestMember)
	}
	if snapData == nil {
		return domain.SnapshotManifest{}, warnings, fmt.Errorf("missing required member %s", homepageMember)
	}

	var manifest domain.SnapshotManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return domain.SnapshotManifest{}, warnings, fmt.Errorf("parse manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return domain.SnapshotManifest{}, warnings, fmt.Errorf("manifest schema: %w", err)
	}

	// Per-file checksums (checksum mismatch).
	byPath := make(map[string][]byte, len(members))
	for _, m := range members {
		byPath[m.path] = m.data
	}
	for _, f := range manifest.Files {
		data, ok := byPath[f.Path]
		if !ok {
			return domain.SnapshotManifest{}, warnings, fmt.Errorf("manifest declares %q but the archive member is missing", f.Path)
		}
		if sha256Hex(data) != f.SHA256 {
			return domain.SnapshotManifest{}, warnings, fmt.Errorf("checksum mismatch for %s", f.Path)
		}
	}
	for _, m := range members {
		if m.path == ManifestMember {
			continue
		}
		declared := false
		for _, f := range manifest.Files {
			if f.Path == m.path {
				declared = true
				break
			}
		}
		if !declared {
			return domain.SnapshotManifest{}, warnings, fmt.Errorf("archive member %s is not declared in the manifest", m.path)
		}
	}

	// Snapshot parse + schema (invalid JSON in a data member).
	var snap domain.Snapshot
	if err := json.Unmarshal(snapData, &snap); err != nil {
		return domain.SnapshotManifest{}, warnings, fmt.Errorf("parse %s: %w", homepageMember, err)
	}
	if err := snap.Validate(); err != nil {
		return domain.SnapshotManifest{}, warnings, fmt.Errorf("snapshot schema: %w", err)
	}

	// Mixed generations: the manifest ID must match the actual content hash.
	if manifest.SnapshotID != domain.SnapshotContentHash(snap) {
		return domain.SnapshotManifest{}, warnings, fmt.Errorf("mixed generations: manifest snapshotId does not match data/homepage.json content hash")
	}

	// Count consistency (manifest vs files; non-empty rule).
	if manifest.Stories != len(snap.Stories) || manifest.Latest != len(snap.Latest) || manifest.Teams != len(snap.Teams) {
		return domain.SnapshotManifest{}, warnings, fmt.Errorf("count mismatch: manifest (stories=%d latest=%d teams=%d) vs data file (stories=%d latest=%d teams=%d)",
			manifest.Stories, manifest.Latest, manifest.Teams, len(snap.Stories), len(snap.Latest), len(snap.Teams))
	}
	if manifest.StoryPages != countStoryPages(members) {
		return domain.SnapshotManifest{}, warnings, fmt.Errorf("story page count mismatch: manifest %d vs archive %d",
			manifest.StoryPages, countStoryPages(members))
	}
	if manifest.Stories > 0 && len(snap.Stories) == 0 {
		return domain.SnapshotManifest{}, warnings, fmt.Errorf("manifest declares %d stories but data file has none", manifest.Stories)
	}

	// Secret scan over every text member (secret leakage defense).
	for _, m := range members {
		if domain.IsSecretPattern(string(m.data)) {
			return domain.SnapshotManifest{}, warnings, fmt.Errorf("member %s appears to contain secrets", m.path)
		}
	}

	// Link safety: every URL in the snapshot must be http(s).
	for i, story := range snap.Stories {
		for j, pub := range story.Publishers {
			if err := checkURL("story %d publisher %d", pub.URL, i, j); err != nil {
				return domain.SnapshotManifest{}, warnings, err
			}
		}
	}
	for j, art := range snap.Latest {
		if err := checkURL("latest article %d", art.URL, j); err != nil {
			return domain.SnapshotManifest{}, warnings, err
		}
	}

	// Demo-marker scan (mirrors the workflow grep).
	for _, m := range members {
		text := string(m.data)
		for _, marker := range DemoMarkers {
			if strings.Contains(text, marker) {
				return domain.SnapshotManifest{}, warnings, fmt.Errorf("member %s contains demo marker %q", m.path, marker)
			}
		}
	}

	return manifest, warnings, nil
}

// Restore validates the archive (hard error before any write, AC-4), then
// extracts it to a staging directory and atomically promotes the data and
// story files into dataDir / storiesDir. Generated story pages (*.md with
// the huddlescope generator marker) that are not in the archive are removed
// as stale; hand-authored pages, _index.md and fixtures are never touched.
// No timestamps are rewritten: the restored content keeps its original
// freshness (no clock writes, no ingest, no fabricated freshness).
func Restore(archivePath, dataDir, storiesDir string) (domain.SnapshotManifest, []string, error) {
	manifest, warnings, err := Validate(archivePath)
	if err != nil {
		return domain.SnapshotManifest{}, warnings, err
	}

	members, _, err := readArchiveMembers(archivePath)
	if err != nil {
		return domain.SnapshotManifest{}, warnings, err
	}

	staging := dataDir + ".staging"
	if err := os.RemoveAll(staging); err != nil {
		return domain.SnapshotManifest{}, nil, fmt.Errorf("remove staging: %w", err)
	}
	if err := os.MkdirAll(staging, 0o700); err != nil {
		return domain.SnapshotManifest{}, nil, fmt.Errorf("create staging: %w", err)
	}
	defer func() { _ = os.RemoveAll(staging) }() //nolint:errcheck

	// Extract to staging with path-traversal guards (defense in depth over
	// the validation that already ran).
	for _, m := range members {
		if m.path == ManifestMember {
			continue // the manifest is not promoted into the workspace
		}
		dest := filepath.Join(staging, filepath.FromSlash(m.path))
		if !strings.HasPrefix(dest, filepath.Clean(staging)+string(filepath.Separator)) {
			return domain.SnapshotManifest{}, nil, fmt.Errorf("member %s escapes the staging directory", m.path)
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
			return domain.SnapshotManifest{}, nil, fmt.Errorf("mkdir %s: %w", filepath.Dir(dest), err)
		}
		if err := os.WriteFile(dest, m.data, 0o644); err != nil { // #nosec G306: restored snapshot content is public static site content and must stay world-readable
			return domain.SnapshotManifest{}, nil, fmt.Errorf("stage %s: %w", m.path, err)
		}
	}

	// Promote atomically (tmp + rename) into the workspace.
	for _, m := range members {
		if m.path == ManifestMember {
			continue
		}
		var dest string
		switch {
		case strings.HasPrefix(m.path, dataPrefix):
			dest = filepath.Join(dataDir, filepath.Base(m.path))
		case strings.HasPrefix(m.path, storiesPrefix):
			dest = filepath.Join(storiesDir, filepath.Base(m.path))
		default:
			return domain.SnapshotManifest{}, nil, fmt.Errorf("member %s has no promotion target", m.path)
		}
		if err := promoteFile(filepath.Join(staging, filepath.FromSlash(m.path)), dest); err != nil {
			return domain.SnapshotManifest{}, nil, err
		}
	}

	// Remove stale generated story pages not in the archive (preserve
	// hand-authored pages: only files carrying the generator marker go).
	if storiesDir != "" {
		if err := removeStaleStoryPages(storiesDir, members); err != nil {
			return domain.SnapshotManifest{}, nil, err
		}
	}

	return manifest, warnings, nil
}

// removeStaleStoryPages deletes *.md files in storiesDir that are not part
// of the archive and carry the huddlescope generator marker. Everything else
// (_index.md, hand-authored pages, non-.md files) is preserved.
func removeStaleStoryPages(storiesDir string, members []member) error {
	inArchive := make(map[string]bool, len(members))
	for _, m := range members {
		if strings.HasPrefix(m.path, storiesPrefix) {
			inArchive[filepath.Base(m.path)] = true
		}
	}
	entries, err := os.ReadDir(storiesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read stories dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || inArchive[e.Name()] {
			continue
		}
		path := filepath.Join(storiesDir, e.Name())
		data, err := os.ReadFile(path) // #nosec G304: e.Name() comes from os.ReadDir of storiesDir
		if err != nil {
			return fmt.Errorf("read stale page %s: %w", e.Name(), err)
		}
		if !hasGeneratorMarker(data) {
			continue // hand-authored or fixture page; keep
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove stale page %s: %w", e.Name(), err)
		}
	}
	return nil
}

// hasGeneratorMarker reports whether the YAML front-matter block of a
// Markdown document contains the huddlescope generator marker line (same
// semantics as the Hugo publisher's stale cleanup).
func hasGeneratorMarker(data []byte) bool {
	text := string(data)
	if !strings.HasPrefix(text, "---\n") {
		return false
	}
	rest := text[4:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return false
	}
	for _, line := range strings.Split(rest[:end], "\n") {
		if strings.TrimSpace(line) == "generator: huddlescope" {
			return true
		}
	}
	return false
}

// promoteFile copies src to dest atomically (tmp + rename, 0644).
func promoteFile(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(dest), err)
	}
	tmp := dest + ".tmp"
	data, err := os.ReadFile(src) // #nosec G304: src is the snapshot staging directory created by this process
	if err != nil {
		return fmt.Errorf("read staged %s: %w", src, err)
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil { // #nosec G306,G703: dest/tmp derive from validated, whitelisted archive member paths (no traversal possible) and content is public static site content
		return fmt.Errorf("temp write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s -> %s: %w", tmp, dest, err)
	}
	return nil
}

// readArchiveMembers reads a .tar.gz, enforcing the member whitelist
// (manifest.json + data/*.json + content/stories/*.md), no path traversal,
// no non-regular files, and the size cap. Warnings report optional members
// that are missing.
func readArchiveMembers(archivePath string) ([]member, []string, error) {
	f, err := os.Open(archivePath) // #nosec G304: archivePath is the operator-provided artifact path
	if err != nil {
		return nil, nil, fmt.Errorf("open archive: %w", err)
	}
	defer f.Close() //nolint:errcheck

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, nil, fmt.Errorf("gzip open: %w", err)
	}
	defer gz.Close() //nolint:errcheck

	tr := tar.NewReader(gz)
	var members []member
	var total int64
	seen := make(map[string]bool)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("tar next (truncated or corrupted archive?): %w", err)
		}
		name := hdr.Name
		if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "..") || strings.HasSuffix(name, "/") {
			return nil, nil, fmt.Errorf("tar member name %q is not a safe relative path", name)
		}
		if hdr.Typeflag != tar.TypeReg {
			return nil, nil, fmt.Errorf("tar member %q is not a regular file (symlinks and other types are rejected)", name)
		}
		if !isWhitelistedMember(name) {
			return nil, nil, fmt.Errorf("unexpected tar member %q", name)
		}
		if seen[name] {
			return nil, nil, fmt.Errorf("duplicate tar member %q", name)
		}
		seen[name] = true

		size := hdr.Size
		if size < 0 {
			return nil, nil, fmt.Errorf("tar member %q has unknown size", name)
		}
		total += size
		if total > MaxArchiveBytes {
			return nil, nil, fmt.Errorf("archive exceeds size cap of %d bytes", MaxArchiveBytes)
		}
		data, err := io.ReadAll(io.LimitReader(tr, size+1))
		if err != nil {
			return nil, nil, fmt.Errorf("read member %q: %w", name, err)
		}
		if int64(len(data)) != size {
			return nil, nil, fmt.Errorf("member %q size mismatch: header %d, got %d", name, size, len(data))
		}
		members = append(members, member{path: name, data: data})
	}

	if len(members) == 0 {
		return nil, nil, fmt.Errorf("archive contains no members")
	}
	var warnings []string
	if !seen[feedsMember] {
		warnings = append(warnings, "optional member missing: "+feedsMember)
	}
	if !seen[heartbeatMember] {
		warnings = append(warnings, "optional member missing: "+heartbeatMember)
	}
	return members, warnings, nil
}

// isWhitelistedMember reports whether name is an allowed archive member.
func isWhitelistedMember(name string) bool {
	if name == ManifestMember {
		return true
	}
	if strings.HasPrefix(name, dataPrefix) {
		return knownDataMembers[name]
	}
	if strings.HasPrefix(name, storiesPrefix) {
		base := strings.TrimPrefix(name, storiesPrefix)
		return strings.HasSuffix(base, ".md") && !strings.Contains(base, "/")
	}
	return false
}

func countStoryPages(members []member) int {
	n := 0
	for _, m := range members {
		if strings.HasPrefix(m.path, storiesPrefix) {
			n++
		}
	}
	return n
}

func memberData(members []member, path string) ([]byte, error) {
	for _, m := range members {
		if m.path == path {
			return m.data, nil
		}
	}
	return nil, fmt.Errorf("missing required member %s", path)
}

// checkURL enforces http(s) links only (rejects file:, data:, gopher:, ...).
func checkURL(label string, url string, args ...any) error {
	if url == "" {
		return fmt.Errorf(label+" has an empty URL", args...)
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return fmt.Errorf(label+" URL must be http:// or https:// (got %q)", append(args, url)...)
	}
	return nil
}

// writeArchive writes members into a deterministic .tar.gz at outPath via a
// temp file + atomic rename (0600). The temp file is removed on any error
// so no partial archive survives an interrupted export (AC-2).
func writeArchive(outPath string, members []member, modTime time.Time) error {
	dir := filepath.Dir(outPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create dir %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".site-snapshot-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp: %w", err)
	}

	gz := gzip.NewWriter(tmp)
	tw := tar.NewWriter(gz)

	for _, m := range members {
		hdr := &tar.Header{
			Name:    m.path,
			Mode:    0o644,
			Size:    int64(len(m.data)),
			ModTime: modTime,
			Uid:     0,
			Gid:     0,
			Uname:   "",
			Gname:   "",
		}
		if err := tw.WriteHeader(hdr); err != nil {
			_ = tw.Close()
			_ = gz.Close()
			_ = tmp.Close()
			return fmt.Errorf("write header %s: %w", m.path, err)
		}
		if _, err := tw.Write(m.data); err != nil {
			_ = tw.Close()
			_ = gz.Close()
			_ = tmp.Close()
			return fmt.Errorf("write member %s: %w", m.path, err)
		}
	}
	if err := tw.Close(); err != nil {
		_ = gz.Close()
		_ = tmp.Close()
		return fmt.Errorf("close tar: %w", err)
	}
	if err := gz.Close(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("close gzip: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpName, outPath); err != nil {
		return fmt.Errorf("rename temp to %s: %w", outPath, err)
	}
	tmpName = ""
	return nil
}

// tarModTime converts the snapshot generatedAt into the fixed tar ModTime;
// an unparseable value falls back to the Unix epoch so output stays stable.
func tarModTime(createdAt string) time.Time {
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return time.Unix(0, 0).UTC()
	}
	return t.UTC()
}

func sortMembers(members []member) {
	sort.Slice(members, func(i, j int) bool { return members[i].path < members[j].path })
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
