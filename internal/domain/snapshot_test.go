package domain

import (
	"strings"
	"testing"
	"time"
)

func TestBuildSnapshot_Empty(t *testing.T) {
	registry := NewTeamRegistry()
	scorer := NewScorer()
	resolver := NewStaticSourceResolver(map[string]string{"test": "Test"})

	snap, err := BuildSnapshot(
		[]Story{},
		[]StoryGroup{},
		scorer,
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		resolver,
		registry,
		10,
	)
	if err != nil {
		t.Fatalf("BuildSnapshot(empty) failed: %v", err)
	}

	if snap.Version != SnapshotVersion {
		t.Errorf("Version = %q; want %q", snap.Version, SnapshotVersion)
	}
	if len(snap.Stories) != 0 {
		t.Errorf("Stories length = %d; want 0", len(snap.Stories))
	}
	if len(snap.Latest) != 0 {
		t.Errorf("Latest length = %d; want 0", len(snap.Latest))
	}
	if len(snap.Teams) != 0 {
		t.Errorf("Teams length = %d; want 0", len(snap.Teams))
	}
}

func TestBuildSnapshot_Determinism(t *testing.T) {
	registry := NewTeamRegistry()
	scorer := NewScorer()
	resolver := NewStaticSourceResolver(map[string]string{"test": "Test"})

	articles := []Article{
		{ID: "a1", SourceID: "test", Title: "Story A", Summary: "Summary A", URL: "http://example.com/a", PublishedAt: "2024-01-01T10:00:00Z"},
		{ID: "a2", SourceID: "test", Title: "Story B", Summary: "Summary B", URL: "http://example.com/b", PublishedAt: "2024-01-02T10:00:00Z"},
	}

	groups := []StoryGroup{
		{ID: "g1", Headline: "Story A", Articles: articles, FirstPublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC), LastPublishedAt: time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC)},
	}

	stories := []Story{
		{ID: "g1", Title: "Story A", Source: "test", Summary: "Summary A", URL: "http://example.com/a", PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)},
	}

	// Build twice and compare bytes
	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	snap1, err1 := BuildSnapshot(stories, groups, scorer, now, resolver, registry, 10)
	if err1 != nil {
		t.Fatalf("first build failed: %v", err1)
	}
	b1, err := snap1.Render()
	if err != nil {
		t.Fatalf("first render failed: %v", err)
	}

	// Second build
	snap2, err2 := BuildSnapshot(stories, groups, scorer, now, resolver, registry, 10)
	if err2 != nil {
		t.Fatalf("second build failed: %v", err2)
	}
	b2, err := snap2.Render()
	if err != nil {
		t.Fatalf("second render failed: %v", err)
	}

	// Content should be identical (ignore generatedAt which differs)
	if string(b1) != string(b2) {
		t.Errorf("Content differs between builds")
		t.Logf("First:\n%s", string(b1))
		t.Logf("Second:\n%s", string(b2))
	}
}

func TestBuildSnapshot_SortOrder(t *testing.T) {
	registry := NewTeamRegistry()
	scorer := NewScorer()
	resolver := NewStaticSourceResolver(map[string]string{
		"espn": "ESPN",
		"nfl":  "NFL.com",
	})

	articles := []Article{
		{ID: "a1", SourceID: "nfl", Title: "NFL Story", Summary: "NFL summary", URL: "http://nfl.com/a", PublishedAt: "2024-01-01T10:00:00Z"},
		{ID: "a2", SourceID: "espn", Title: "ESPN Story", Summary: "ESPN summary", URL: "http://espn.com/a", PublishedAt: "2024-01-01T11:00:00Z"},
	}

	groups := []StoryGroup{
		{ID: "g1", Headline: "NFL Story", Articles: articles, FirstPublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC), LastPublishedAt: time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC)},
	}

	stories := []Story{
		{ID: "g1", Title: "NFL Story", Source: "nfl", Summary: "NFL summary", URL: "http://nfl.com/a", PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)},
	}

	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	snap, err := BuildSnapshot(stories, groups, scorer, now, resolver, registry, 10)
	if err != nil {
		t.Fatalf("BuildSnapshot failed: %v", err)
	}

	// Publishers should be sorted by ID
	if len(snap.Stories) == 0 {
		t.Fatal("no stories")
	}
	if len(snap.Stories[0].Publishers) != 2 {
		t.Fatalf("expected 2 publishers, got %d", len(snap.Stories[0].Publishers))
	}
	if snap.Stories[0].Publishers[0].ID != "espn" {
		t.Errorf("publishers not sorted by ID: first is %s", snap.Stories[0].Publishers[0].ID)
	}
	if snap.Stories[0].Publishers[1].ID != "nfl" {
		t.Errorf("publishers not sorted by ID: second is %s", snap.Stories[0].Publishers[1].ID)
	}
}

func TestBuildSnapshot_Latest(t *testing.T) {
	registry := NewTeamRegistry()
	scorer := NewScorer()
	resolver := NewStaticSourceResolver(map[string]string{"test": "Test"})

	stories := []Story{
		{ID: "s1", Title: "Old Story", Source: "test", Summary: "Old", URL: "http://example.com/old", PublishedAt: time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC)},
		{ID: "s2", Title: "Recent Story", Source: "test", Summary: "Recent", URL: "http://example.com/recent", PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)},
		{ID: "s3", Title: "Newest Story", Source: "test", Summary: "Newest", URL: "http://example.com/newest", PublishedAt: time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)},
		{ID: "s4", Title: "No URL", Source: "test", Summary: "No URL", URL: "", PublishedAt: time.Date(2024, 2, 1, 10, 0, 0, 0, time.UTC)},
	}

	now := time.Date(2024, 12, 1, 12, 0, 0, 0, time.UTC)
	snap, err := BuildSnapshot(stories, nil, scorer, now, resolver, registry, 3)
	if err != nil {
		t.Fatalf("BuildSnapshot failed: %v", err)
	}

	if len(snap.Latest) != 3 {
		t.Errorf("Latest count = %d; want 3", len(snap.Latest))
	}

	// Should be sorted by publishedAt desc; s4 (no URL) should be skipped
	if len(snap.Latest) > 0 && snap.Latest[0].Title != "Newest Story" {
		t.Errorf("First latest = %s; want Newest Story", snap.Latest[0].Title)
	}
	if len(snap.Latest) > 1 && snap.Latest[1].Title != "Recent Story" {
		t.Errorf("Second latest = %s; want Recent Story", snap.Latest[1].Title)
	}
}

func TestBuildSnapshot_Teams(t *testing.T) {
	scorer := NewScorer()
	resolver := NewStaticSourceResolver(map[string]string{"test": "Test"})
	registry := NewTeamRegistry(Team{
		Slug:         "giants",
		Name:         "New York Giants",
		ShortName:    "Giants",
		Abbreviation: "NYG",
		Aliases:      []string{"Big Blue", "Giant"},
	})

	stories := []Story{
		{ID: "s1", Title: "Giants win", Source: "test", Summary: "Giants won", URL: "http://example.com", PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)},
	}

	now := time.Date(2024, 12, 1, 12, 0, 0, 0, time.UTC)
	snap, err := BuildSnapshot(stories, nil, scorer, now, resolver, registry, 10)
	if err != nil {
		t.Fatalf("BuildSnapshot failed: %v", err)
	}

	if len(snap.Teams) != 1 {
		t.Fatalf("Teams count = %d; want 1", len(snap.Teams))
	}
	if snap.Teams[0].Slug != "giants" {
		t.Errorf("Team slug = %s; want giants", snap.Teams[0].Slug)
	}
}

func TestSnapshot_Validate_Success(t *testing.T) {
	snap := Snapshot{
		Version: SnapshotVersion,
		Stories: []SnapshotStory{
			{
				ID:               "s1",
				Title:            "Title",
				Summary:          "Summary",
				FirstPublishedAt: "2024-01-01T00:00:00Z",
				LastPublishedAt:  "2024-01-02T00:00:00Z",
				Score:            80.5,
				EvidenceLevel:    EvidenceConfirmed,
				Publishers: []PublisherRef{
					{ID: "src1", Name: "Source 1", URL: "http://example.com/1"},
				},
			},
		},
		Latest: []SnapshotArticle{
			{SourceID: "src1", SourceName: "Source 1", Title: "Latest", URL: "http://example.com/latest", PublishedAt: "2024-01-03T00:00:00Z"},
		},
		Teams: []SnapshotTeam{
			{Slug: "giants", Name: "New York Giants", ShortName: "Giants", Abbreviation: "NYG"},
		},
	}

	if err := snap.Validate(); err != nil {
		t.Fatalf("Validate() returned error: %v", err)
	}
}

func TestSnapshot_Validate_Version(t *testing.T) {
	snap := Snapshot{
		Version: "invalid",
		Stories: []SnapshotStory{{ID: "s1", Title: "T", Summary: "S", FirstPublishedAt: "2024-01-01T00:00:00Z", LastPublishedAt: "2024-01-02T00:00:00Z", Score: 80, EvidenceLevel: EvidenceConfirmed, Publishers: []PublisherRef{{ID: "src1", Name: "S1", URL: "http://ex.com"}}}},
		Latest:  []SnapshotArticle{{SourceID: "src1", SourceName: "S1", Title: "L", URL: "http://ex.com", PublishedAt: "2024-01-03T00:00:00Z"}},
		Teams:   []SnapshotTeam{{Slug: "giants", Name: "Giants", ShortName: "Giants", Abbreviation: "NYG"}},
	}

	err := snap.Validate()
	if err == nil {
		t.Fatal("Validate() should fail on bad version")
	}
	if !strings.Contains(err.Error(), "invalid version") {
		t.Errorf("Error message should mention 'invalid version': %v", err)
	}
}

func TestSnapshot_Validate_SecretLeak(t *testing.T) {
	tests := []struct {
		name    string
		summary string
		wantErr bool
	}{
		{"no secret", "Standard summary text", false},
		{"password found", "Summary with password123 in text", true},
		{"token found", "Summary with secret_token here", true},
		{"api_key found", "Summary with api_key: abc123", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snap := Snapshot{
				Version: SnapshotVersion,
				Stories: []SnapshotStory{{ID: "s1", Title: "Title", Summary: tt.summary, FirstPublishedAt: "2024-01-01T00:00:00Z", LastPublishedAt: "2024-01-02T00:00:00Z", Score: 80, EvidenceLevel: EvidenceConfirmed, Publishers: []PublisherRef{{ID: "src1", Name: "S1", URL: "http://ex.com"}}}},
				Latest:  []SnapshotArticle{{SourceID: "src1", SourceName: "S1", Title: "L", URL: "http://ex.com", PublishedAt: "2024-01-03T00:00:00Z"}},
				Teams:   []SnapshotTeam{{Slug: "giants", Name: "Giants", ShortName: "Giants", Abbreviation: "NYG"}},
			}

			err := snap.Validate()
			if tt.wantErr && err == nil {
				t.Errorf("Validate() should have found secret leak")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Validate() unexpectedly returned error: %v", err)
			}
		})
	}
}

func TestSnapshot_Validate_BodyTooLong(t *testing.T) {
	// Summary > 1024 chars is flagged as likely containing body content
	longSummary := strings.Repeat("word ", 210) // ~1050 chars, over 1024 threshold

	snap := Snapshot{
		Version: SnapshotVersion,
		Stories: []SnapshotStory{
			{
				ID:               "s1",
				Title:            "Title",
				Summary:          longSummary,
				FirstPublishedAt: "2024-01-01T00:00:00Z",
				LastPublishedAt:  "2024-01-02T00:00:00Z",
				Score:            80.5,
				EvidenceLevel:    EvidenceConfirmed,
				Publishers: []PublisherRef{
					{ID: "src1", Name: "Source 1", URL: "http://example.com/1"},
				},
			},
		},
		Latest: []SnapshotArticle{
			{SourceID: "src1", SourceName: "Source 1", Title: "Latest", URL: "http://example.com/latest", PublishedAt: "2024-01-03T00:00:00Z"},
		},
		Teams: []SnapshotTeam{
			{Slug: "giants", Name: "New York Giants", ShortName: "Giants", Abbreviation: "NYG"},
		},
	}

	err := snap.Validate()
	if err == nil {
		t.Fatal("Validate() should fail on long summary (body content)")
	}
	if !strings.Contains(err.Error(), "summary too long") {
		t.Errorf("Error should mention 'summary too long': %v", err)
	}
}

func TestSnapshot_Validate_EmptyRequired(t *testing.T) {
	tests := []struct {
		name     string
		modify   func(*Snapshot)
		field    string
		initSnap func() Snapshot
	}{
		{"empty stories", func(s *Snapshot) { s.Stories = nil }, "stories", func() Snapshot {
			return Snapshot{
				Version: SnapshotVersion,
				Stories: []SnapshotStory{{ID: "s1", Title: "T", Summary: "S", FirstPublishedAt: "2024-01-01T00:00:00Z", LastPublishedAt: "2024-01-02T00:00:00Z", Score: 80, EvidenceLevel: EvidenceConfirmed, Publishers: []PublisherRef{{ID: "src1", Name: "S1", URL: "http://ex.com"}}}},
				Latest:  []SnapshotArticle{{SourceID: "src1", SourceName: "S1", Title: "L", URL: "http://ex.com", PublishedAt: "2024-01-03T00:00:00Z"}},
				Teams:   []SnapshotTeam{{Slug: "giants", Name: "Giants", ShortName: "Giants", Abbreviation: "NYG"}},
			}
		}},
		{"empty latest", func(s *Snapshot) { s.Latest = nil }, "latest", func() Snapshot {
			return Snapshot{
				Version: SnapshotVersion,
				Stories: []SnapshotStory{{ID: "s1", Title: "T", Summary: "S", FirstPublishedAt: "2024-01-01T00:00:00Z", LastPublishedAt: "2024-01-02T00:00:00Z", Score: 80, EvidenceLevel: EvidenceConfirmed, Publishers: []PublisherRef{{ID: "src1", Name: "S1", URL: "http://ex.com"}}}},
				Latest:  []SnapshotArticle{{SourceID: "src1", SourceName: "S1", Title: "L", URL: "http://ex.com", PublishedAt: "2024-01-03T00:00:00Z"}},
				Teams:   []SnapshotTeam{{Slug: "giants", Name: "Giants", ShortName: "Giants", Abbreviation: "NYG"}},
			}
		}},
		{"empty teams", func(s *Snapshot) { s.Teams = nil }, "teams", func() Snapshot {
			return Snapshot{
				Version: SnapshotVersion,
				Stories: []SnapshotStory{{ID: "s1", Title: "T", Summary: "S", FirstPublishedAt: "2024-01-01T00:00:00Z", LastPublishedAt: "2024-01-02T00:00:00Z", Score: 80, EvidenceLevel: EvidenceConfirmed, Publishers: []PublisherRef{{ID: "src1", Name: "S1", URL: "http://ex.com"}}}},
				Latest:  []SnapshotArticle{{SourceID: "src1", SourceName: "S1", Title: "L", URL: "http://ex.com", PublishedAt: "2024-01-03T00:00:00Z"}},
				Teams:   []SnapshotTeam{{Slug: "giants", Name: "Giants", ShortName: "Giants", Abbreviation: "NYG"}},
			}
		}},
		{"empty story ID", func(s *Snapshot) { s.Stories[0].ID = "" }, "story ID", func() Snapshot {
			return Snapshot{
				Version: SnapshotVersion,
				Stories: []SnapshotStory{{ID: "s1", Title: "T", Summary: "S", FirstPublishedAt: "2024-01-01T00:00:00Z", LastPublishedAt: "2024-01-02T00:00:00Z", Score: 80, EvidenceLevel: EvidenceConfirmed, Publishers: []PublisherRef{{ID: "src1", Name: "S1", URL: "http://ex.com"}}}},
				Latest:  []SnapshotArticle{{SourceID: "src1", SourceName: "S1", Title: "L", URL: "http://ex.com", PublishedAt: "2024-01-03T00:00:00Z"}},
				Teams:   []SnapshotTeam{{Slug: "giants", Name: "Giants", ShortName: "Giants", Abbreviation: "NYG"}},
			}
		}},
		{"empty story title", func(s *Snapshot) { s.Stories[0].Title = "" }, "story title", func() Snapshot {
			return Snapshot{
				Version: SnapshotVersion,
				Stories: []SnapshotStory{{ID: "s1", Title: "T", Summary: "S", FirstPublishedAt: "2024-01-01T00:00:00Z", LastPublishedAt: "2024-01-02T00:00:00Z", Score: 80, EvidenceLevel: EvidenceConfirmed, Publishers: []PublisherRef{{ID: "src1", Name: "S1", URL: "http://ex.com"}}}},
				Latest:  []SnapshotArticle{{SourceID: "src1", SourceName: "S1", Title: "L", URL: "http://ex.com", PublishedAt: "2024-01-03T00:00:00Z"}},
				Teams:   []SnapshotTeam{{Slug: "giants", Name: "Giants", ShortName: "Giants", Abbreviation: "NYG"}},
			}
		}},
		{"empty story summary", func(s *Snapshot) { s.Stories[0].Summary = "" }, "story summary", func() Snapshot {
			return Snapshot{
				Version: SnapshotVersion,
				Stories: []SnapshotStory{{ID: "s1", Title: "T", Summary: "S", FirstPublishedAt: "2024-01-01T00:00:00Z", LastPublishedAt: "2024-01-02T00:00:00Z", Score: 80, EvidenceLevel: EvidenceConfirmed, Publishers: []PublisherRef{{ID: "src1", Name: "S1", URL: "http://ex.com"}}}},
				Latest:  []SnapshotArticle{{SourceID: "src1", SourceName: "S1", Title: "L", URL: "http://ex.com", PublishedAt: "2024-01-03T00:00:00Z"}},
				Teams:   []SnapshotTeam{{Slug: "giants", Name: "Giants", ShortName: "Giants", Abbreviation: "NYG"}},
			}
		}},
		{"empty publisher ID", func(s *Snapshot) { s.Stories[0].Publishers[0].ID = "" }, "publisher ID", func() Snapshot {
			return Snapshot{
				Version: SnapshotVersion,
				Stories: []SnapshotStory{{ID: "s1", Title: "T", Summary: "S", FirstPublishedAt: "2024-01-01T00:00:00Z", LastPublishedAt: "2024-01-02T00:00:00Z", Score: 80, EvidenceLevel: EvidenceConfirmed, Publishers: []PublisherRef{{ID: "src1", Name: "S1", URL: "http://ex.com"}}}},
				Latest:  []SnapshotArticle{{SourceID: "src1", SourceName: "S1", Title: "L", URL: "http://ex.com", PublishedAt: "2024-01-03T00:00:00Z"}},
				Teams:   []SnapshotTeam{{Slug: "giants", Name: "Giants", ShortName: "Giants", Abbreviation: "NYG"}},
			}
		}},
		{"empty publisher URL", func(s *Snapshot) { s.Stories[0].Publishers[0].URL = "" }, "publisher URL", func() Snapshot {
			return Snapshot{
				Version: SnapshotVersion,
				Stories: []SnapshotStory{{ID: "s1", Title: "T", Summary: "S", FirstPublishedAt: "2024-01-01T00:00:00Z", LastPublishedAt: "2024-01-02T00:00:00Z", Score: 80, EvidenceLevel: EvidenceConfirmed, Publishers: []PublisherRef{{ID: "src1", Name: "S1", URL: "http://ex.com"}}}},
				Latest:  []SnapshotArticle{{SourceID: "src1", SourceName: "S1", Title: "L", URL: "http://ex.com", PublishedAt: "2024-01-03T00:00:00Z"}},
				Teams:   []SnapshotTeam{{Slug: "giants", Name: "Giants", ShortName: "Giants", Abbreviation: "NYG"}},
			}
		}},
		{"empty latest URL", func(s *Snapshot) { s.Latest[0].URL = "" }, "latest URL", func() Snapshot {
			return Snapshot{
				Version: SnapshotVersion,
				Stories: []SnapshotStory{{ID: "s1", Title: "T", Summary: "S", FirstPublishedAt: "2024-01-01T00:00:00Z", LastPublishedAt: "2024-01-02T00:00:00Z", Score: 80, EvidenceLevel: EvidenceConfirmed, Publishers: []PublisherRef{{ID: "src1", Name: "S1", URL: "http://ex.com"}}}},
				Latest:  []SnapshotArticle{{SourceID: "src1", SourceName: "S1", Title: "L", URL: "http://ex.com", PublishedAt: "2024-01-03T00:00:00Z"}},
				Teams:   []SnapshotTeam{{Slug: "giants", Name: "Giants", ShortName: "Giants", Abbreviation: "NYG"}},
			}
		}},
		{"empty team slug", func(s *Snapshot) { s.Teams[0].Slug = "" }, "team slug", func() Snapshot {
			return Snapshot{
				Version: SnapshotVersion,
				Stories: []SnapshotStory{{ID: "s1", Title: "T", Summary: "S", FirstPublishedAt: "2024-01-01T00:00:00Z", LastPublishedAt: "2024-01-02T00:00:00Z", Score: 80, EvidenceLevel: EvidenceConfirmed, Publishers: []PublisherRef{{ID: "src1", Name: "S1", URL: "http://ex.com"}}}},
				Latest:  []SnapshotArticle{{SourceID: "src1", SourceName: "S1", Title: "L", URL: "http://ex.com", PublishedAt: "2024-01-03T00:00:00Z"}},
				Teams:   []SnapshotTeam{{Slug: "giants", Name: "Giants", ShortName: "Giants", Abbreviation: "NYG"}},
			}
		}},
		{"empty team name", func(s *Snapshot) { s.Teams[0].Name = "" }, "team name", func() Snapshot {
			return Snapshot{
				Version: SnapshotVersion,
				Stories: []SnapshotStory{{ID: "s1", Title: "T", Summary: "S", FirstPublishedAt: "2024-01-01T00:00:00Z", LastPublishedAt: "2024-01-02T00:00:00Z", Score: 80, EvidenceLevel: EvidenceConfirmed, Publishers: []PublisherRef{{ID: "src1", Name: "S1", URL: "http://ex.com"}}}},
				Latest:  []SnapshotArticle{{SourceID: "src1", SourceName: "S1", Title: "L", URL: "http://ex.com", PublishedAt: "2024-01-03T00:00:00Z"}},
				Teams:   []SnapshotTeam{{Slug: "giants", Name: "Giants", ShortName: "Giants", Abbreviation: "NYG"}},
			}
		}},
		{"empty team short", func(s *Snapshot) { s.Teams[0].ShortName = "" }, "team short_name", func() Snapshot {
			return Snapshot{
				Version: SnapshotVersion,
				Stories: []SnapshotStory{{ID: "s1", Title: "T", Summary: "S", FirstPublishedAt: "2024-01-01T00:00:00Z", LastPublishedAt: "2024-01-02T00:00:00Z", Score: 80, EvidenceLevel: EvidenceConfirmed, Publishers: []PublisherRef{{ID: "src1", Name: "S1", URL: "http://ex.com"}}}},
				Latest:  []SnapshotArticle{{SourceID: "src1", SourceName: "S1", Title: "L", URL: "http://ex.com", PublishedAt: "2024-01-03T00:00:00Z"}},
				Teams:   []SnapshotTeam{{Slug: "giants", Name: "Giants", ShortName: "Giants", Abbreviation: "NYG"}},
			}
		}},
		{"empty team abbr", func(s *Snapshot) { s.Teams[0].Abbreviation = "" }, "team abbreviation", func() Snapshot {
			return Snapshot{
				Version: SnapshotVersion,
				Stories: []SnapshotStory{{ID: "s1", Title: "T", Summary: "S", FirstPublishedAt: "2024-01-01T00:00:00Z", LastPublishedAt: "2024-01-02T00:00:00Z", Score: 80, EvidenceLevel: EvidenceConfirmed, Publishers: []PublisherRef{{ID: "src1", Name: "S1", URL: "http://ex.com"}}}},
				Latest:  []SnapshotArticle{{SourceID: "src1", SourceName: "S1", Title: "L", URL: "http://ex.com", PublishedAt: "2024-01-03T00:00:00Z"}},
				Teams:   []SnapshotTeam{{Slug: "giants", Name: "Giants", ShortName: "Giants", Abbreviation: "NYG"}},
			}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snap := tt.initSnap()
			tt.modify(&snap)
			err := snap.Validate()
			if err == nil {
				t.Errorf("Validate() should fail on %s", tt.field)
			}
		})
	}
}

func TestContentEqual(t *testing.T) {
	scorer := NewScorer().WithTime(time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC))
	resolver := NewStaticSourceResolver(map[string]string{"test": "Test"})
	registry := NewTeamRegistry()

	articles := []Article{
		{ID: "a1", SourceID: "test", Title: "Story A", Summary: "Summary A", URL: "http://example.com/a", PublishedAt: "2024-01-01T10:00:00Z"},
	}
	groups := []StoryGroup{
		{ID: "g1", Headline: "Story A", Articles: articles, FirstPublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC), LastPublishedAt: time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC)},
	}
	stories := []Story{
		{ID: "g1", Title: "Story A", Source: "test", Summary: "Summary A", URL: "http://example.com/a", PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)},
	}

	// Build two snapshots with same now but different generatedAt (only generatedAt differs)
	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	snap1, err1 := BuildSnapshot(stories, groups, scorer, now, resolver, registry, 10)
	if err1 != nil {
		t.Fatalf("first build failed: %v", err1)
	}
	snap2, err2 := BuildSnapshot(stories, groups, scorer, now, resolver, registry, 10)
	if err2 != nil {
		t.Fatalf("second build failed: %v", err2)
	}

	// Content should be equal despite different generatedAt
	b1, _ := snap1.Render()
	b2, _ := snap2.Render()
	t.Log("snap1:", string(b1))
	t.Log("snap2:", string(b2))
	if !snap1.ContentEqual(snap2) {
		t.Error("ContentEqual() should return true when only generatedAt differs")
	}

	// Different content should return false
	snap2.Stories[0].Title = "Different Title"
	if snap1.ContentEqual(snap2) {
		t.Error("ContentEqual() should return false when content differs")
	}
}
