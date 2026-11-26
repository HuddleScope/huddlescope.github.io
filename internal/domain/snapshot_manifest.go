package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// SnapshotManifestVersion is the schema version of the snapshot-archive
// manifest (manifest.json inside a site-snapshot artifact), pinned to "1"
// (issue #249 / PUB-03). A mismatched version is a hard validation error so
// mixed-generation archives are rejected.
const SnapshotManifestVersion = "1"

// ManifestFile is one archived member: a relative archive path plus the
// lowercase hex sha256 of its bytes.
type ManifestFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// SnapshotManifest is the self-describing manifest of a versioned Hugo
// snapshot archive (issue #249 / PUB-03). It links the exported content to
// its export/build/deployment runs: content-derived snapshot ID (stable per
// database state), original freshness timestamps, counts, per-file
// checksums, and the reason for a legitimate empty result.
type SnapshotManifest struct {
	Version      string         `json:"version"`
	SnapshotID   string         `json:"snapshotId"`
	GeneratedAt  string         `json:"generatedAt"`  // RFC3339 UTC (original, from the snapshot)
	LastIngestAt string         `json:"lastIngestAt"` // RFC3339 UTC or "unknown"
	SourceCommit string         `json:"sourceCommit"` // git SHA or "unknown"
	Stories      int            `json:"stories"`
	Latest       int            `json:"latest"`
	Teams        int            `json:"teams"`
	StoryPages   int            `json:"storyPages"`
	EmptyReason  string         `json:"emptyReason,omitempty"` // set when Stories == 0
	Files        []ManifestFile `json:"files"`                 // sorted by Path
}

var manifestIDRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// snapshotIDUnknown is the placeholder used when a freshness source is
// unavailable (fresh install, local run without git).
const snapshotIDUnknown = "unknown"

// SnapshotContentHash computes the content-derived hash of a snapshot:
// sha256 over the compact JSON of the snapshot with GeneratedAt excluded —
// the same comparison surface as ContentEqual. The hash is reproducible for
// the same database state while generatedAt stays fresh.
func SnapshotContentHash(s Snapshot) string {
	n := s
	n.GeneratedAt = ""
	if n.Stories == nil {
		n.Stories = []SnapshotStory{}
	}
	if n.Latest == nil {
		n.Latest = []SnapshotArticle{}
	}
	if n.Teams == nil {
		n.Teams = []SnapshotTeam{}
	}
	b, err := json.Marshal(n)
	if err != nil {
		// A plain struct of strings/floats/slices cannot fail to marshal;
		// fail closed with an empty (invalid) hash so validation rejects it.
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// BuildSnapshotManifest constructs the archive manifest from a parsed
// snapshot, the archived member checksums, and caller-provided freshness
// metadata. Files are sorted by path; empty lastIngestAt/sourceCommit become
// "unknown"; stories == 0 always carries a non-empty emptyReason.
func BuildSnapshotManifest(s Snapshot, files []ManifestFile, storyPages int, lastIngestAt, sourceCommit string) SnapshotManifest {
	sorted := make([]ManifestFile, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })

	if lastIngestAt == "" {
		lastIngestAt = snapshotIDUnknown
	}
	if sourceCommit == "" {
		sourceCommit = snapshotIDUnknown
	}
	emptyReason := ""
	if len(s.Stories) == 0 {
		emptyReason = "no stories in lookback"
	}
	return SnapshotManifest{
		Version:      SnapshotManifestVersion,
		SnapshotID:   SnapshotContentHash(s),
		GeneratedAt:  s.GeneratedAt,
		LastIngestAt: lastIngestAt,
		SourceCommit: sourceCommit,
		Stories:      len(s.Stories),
		Latest:       len(s.Latest),
		Teams:        len(s.Teams),
		StoryPages:   storyPages,
		EmptyReason:  emptyReason,
		Files:        sorted,
	}
}

// Validate checks the manifest schema (fail-closed): pinned version,
// well-formed snapshot ID, parseable timestamps, non-negative counts, the
// empty-reason rule, and a sorted, relative, unique, non-empty file list.
func (m SnapshotManifest) Validate() error {
	if m.Version != SnapshotManifestVersion {
		return fmt.Errorf("manifest version %q; must be %q", m.Version, SnapshotManifestVersion)
	}
	if !manifestIDRe.MatchString(m.SnapshotID) {
		return fmt.Errorf("manifest snapshotId %q is not a 64-char lowercase sha256 hex", m.SnapshotID)
	}
	if _, err := time.Parse(time.RFC3339, m.GeneratedAt); err != nil {
		return fmt.Errorf("manifest generatedAt %q is not RFC3339: %v", m.GeneratedAt, err)
	}
	if m.LastIngestAt != snapshotIDUnknown {
		if _, err := time.Parse(time.RFC3339, m.LastIngestAt); err != nil {
			return fmt.Errorf("manifest lastIngestAt %q is neither RFC3339 nor %q", m.LastIngestAt, snapshotIDUnknown)
		}
	}
	if m.SourceCommit == "" {
		return fmt.Errorf("manifest sourceCommit must not be empty")
	}
	if m.Stories < 0 || m.Latest < 0 || m.Teams < 0 || m.StoryPages < 0 {
		return fmt.Errorf("manifest counts must be non-negative (stories=%d latest=%d teams=%d storyPages=%d)",
			m.Stories, m.Latest, m.Teams, m.StoryPages)
	}
	if m.Stories == 0 && m.EmptyReason == "" {
		return fmt.Errorf("manifest has zero stories but no emptyReason")
	}
	if len(m.Files) == 0 {
		return fmt.Errorf("manifest declares no files")
	}
	seen := make(map[string]bool, len(m.Files))
	for i, f := range m.Files {
		if f.Path == "" {
			return fmt.Errorf("manifest file %d has an empty path", i)
		}
		if strings.HasPrefix(f.Path, "/") || strings.Contains(f.Path, "..") {
			return fmt.Errorf("manifest file %d path %q is not a relative archive path", i, f.Path)
		}
		if seen[f.Path] {
			return fmt.Errorf("manifest file path %q is declared twice", f.Path)
		}
		seen[f.Path] = true
		if !manifestIDRe.MatchString(f.SHA256) {
			return fmt.Errorf("manifest file %q sha256 %q is not 64-char lowercase hex", f.Path, f.SHA256)
		}
		if i > 0 && m.Files[i-1].Path >= f.Path {
			return fmt.Errorf("manifest files are not sorted by path at index %d (%q after %q)", i, f.Path, m.Files[i-1].Path)
		}
	}
	return nil
}

// IsSecretPattern exposes the feed-health secret detector to other packages
// (snapshot archive validation, issue #249). The private implementation
// stays in feedhealth.go.
func IsSecretPattern(s string) bool {
	return isSecretPattern(s)
}
