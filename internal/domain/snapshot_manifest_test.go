package domain

import (
	"strings"
	"testing"
)

func manifestFixture() Snapshot {
	return Snapshot{
		Version:     SnapshotVersion,
		GeneratedAt: "2026-11-26T12:00:00Z",
		Stories: []SnapshotStory{
			{
				ID:               "s1",
				Title:            "First story",
				Summary:          "A summary",
				FirstPublishedAt: "2026-11-25T00:00:00Z",
				LastPublishedAt:  "2026-11-25T01:00:00Z",
				EvidenceLevel:    EvidenceConfirmed,
				Publishers:       []PublisherRef{{ID: "p1", Name: "P", URL: "https://example.org/a"}},
			},
		},
		Latest: []SnapshotArticle{{SourceID: "p1", SourceName: "P", Title: "t", URL: "https://example.org/a", PublishedAt: "2026-11-25T01:00:00Z"}},
		Teams:  []SnapshotTeam{{Slug: "team-a", Name: "A", ShortName: "A", Abbreviation: "A"}},
	}
}

func TestSnapshotContentHash_StableAndExcludesGeneratedAt(t *testing.T) {
	a := manifestFixture()
	b := manifestFixture()
	b.GeneratedAt = "2026-11-26T13:30:00Z" // different time, same content
	if got := SnapshotContentHash(a); got != SnapshotContentHash(b) {
		t.Fatalf("content hash must exclude generatedAt: %q vs %q", got, SnapshotContentHash(b))
	}
	if !manifestIDRe.MatchString(SnapshotContentHash(a)) {
		t.Fatalf("content hash %q is not 64-char lowercase hex", SnapshotContentHash(a))
	}
	// Different content must change the hash.
	c := b
	c.Stories[0].Title = "Changed title"
	if SnapshotContentHash(a) == SnapshotContentHash(c) {
		t.Fatal("content hash must change when content changes")
	}
	// Nil vs empty slices are the same content.
	d := Snapshot{Version: SnapshotVersion}
	e := Snapshot{Version: SnapshotVersion, Stories: []SnapshotStory{}, Latest: []SnapshotArticle{}, Teams: []SnapshotTeam{}}
	if SnapshotContentHash(d) != SnapshotContentHash(e) {
		t.Fatal("nil slices and empty slices must hash identically")
	}
}

func TestBuildSnapshotManifest(t *testing.T) {
	s := manifestFixture()
	files := []ManifestFile{
		{Path: "data/homepage.json", SHA256: strings.Repeat("a", 64)},
		{Path: "content/stories/s1.md", SHA256: strings.Repeat("b", 64)},
	}
	m := BuildSnapshotManifest(s, files, 2, "2026-11-26T11:00:00Z", "abc123")

	if m.Version != SnapshotManifestVersion {
		t.Fatalf("version = %q", m.Version)
	}
	if m.SnapshotID != SnapshotContentHash(s) {
		t.Fatal("snapshotId must be the content hash")
	}
	if m.GeneratedAt != s.GeneratedAt {
		t.Fatalf("generatedAt = %q", m.GeneratedAt)
	}
	if m.Stories != 1 || m.Latest != 1 || m.Teams != 1 || m.StoryPages != 2 {
		t.Fatalf("counts = %+v", m)
	}
	if m.EmptyReason != "" {
		t.Fatalf("emptyReason must be empty for non-zero stories: %q", m.EmptyReason)
	}
	// Files must be sorted by path.
	if m.Files[0].Path != "content/stories/s1.md" || m.Files[1].Path != "data/homepage.json" {
		t.Fatalf("files not sorted: %+v", m.Files)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
}

func TestBuildSnapshotManifest_EmptyAndDefaults(t *testing.T) {
	s := Snapshot{
		Version:     SnapshotVersion,
		GeneratedAt: "2026-11-26T12:00:00Z",
		Stories:     []SnapshotStory{},
		Latest:      []SnapshotArticle{},
		Teams:       []SnapshotTeam{},
	}
	m := BuildSnapshotManifest(s, []ManifestFile{{Path: "data/homepage.json", SHA256: strings.Repeat("c", 64)}}, 0, "", "")
	if m.Stories != 0 {
		t.Fatalf("stories = %d", m.Stories)
	}
	if m.EmptyReason == "" {
		t.Fatal("zero stories must set emptyReason")
	}
	if m.LastIngestAt != "unknown" || m.SourceCommit != "unknown" {
		t.Fatalf("defaults not applied: %+v", m)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("empty manifest rejected: %v", err)
	}
}

func TestSnapshotManifestValidate(t *testing.T) {
	base := func() SnapshotManifest {
		return BuildSnapshotManifest(manifestFixture(),
			[]ManifestFile{{Path: "data/homepage.json", SHA256: strings.Repeat("a", 64)}}, 1, "2026-11-26T11:00:00Z", "abc123")
	}

	cases := []struct {
		name   string
		mutate func(*SnapshotManifest)
		want   string
	}{
		{"ok", func(*SnapshotManifest) {}, ""},
		{"schema mismatch v2", func(m *SnapshotManifest) { m.Version = "2" }, "version"},
		{"bad id", func(m *SnapshotManifest) { m.SnapshotID = "xyz" }, "snapshotId"},
		{"bad generatedAt", func(m *SnapshotManifest) { m.GeneratedAt = "not-a-time" }, "generatedAt"},
		{"bad lastIngestAt", func(m *SnapshotManifest) { m.LastIngestAt = "nope" }, "lastIngestAt"},
		{"empty sourceCommit", func(m *SnapshotManifest) { m.SourceCommit = "" }, "sourceCommit"},
		{"negative count", func(m *SnapshotManifest) { m.Stories = -1 }, "non-negative"},
		{"zero stories no reason", func(m *SnapshotManifest) { m.Stories = 0; m.EmptyReason = "" }, "emptyReason"},
		{"no files", func(m *SnapshotManifest) { m.Files = nil }, "no files"},
		{"empty path", func(m *SnapshotManifest) { m.Files[0].Path = "" }, "empty path"},
		{"absolute path", func(m *SnapshotManifest) { m.Files[0].Path = "/etc/passwd" }, "relative"},
		{"traversal path", func(m *SnapshotManifest) { m.Files[0].Path = "../evil" }, "relative"},
		{"duplicate path", func(m *SnapshotManifest) {
			m.Files = append(m.Files, ManifestFile{Path: "data/homepage.json", SHA256: strings.Repeat("d", 64)})
		}, "twice"},
		{"bad sha", func(m *SnapshotManifest) { m.Files[0].SHA256 = "zz" }, "sha256"},
		{"unsorted", func(m *SnapshotManifest) {
			m.Files = []ManifestFile{
				{Path: "data/homepage.json", SHA256: strings.Repeat("a", 64)},
				{Path: "content/stories/a.md", SHA256: strings.Repeat("b", 64)},
			}
		}, "sorted"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := base()
			tc.mutate(&m)
			err := m.Validate()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestIsSecretPatternExported(t *testing.T) {
	if !IsSecretPattern("token=abcdefghijklmnopqrstuvwxyz") {
		t.Fatal("assignment-style token must be flagged")
	}
	if !IsSecretPattern("Authorization Bearer abcdefghijklmnop1234") {
		t.Fatal("bearer token must be flagged")
	}
	if !IsSecretPattern("database_url=postgres://user:pass@host/db") {
		t.Fatal("dsn assignment must be flagged")
	}
	if IsSecretPattern("The NFL secret weapon was his running back") {
		t.Fatal("ordinary prose must not be flagged")
	}
	if IsSecretPattern("") {
		t.Fatal("empty string must not be flagged")
	}
}
