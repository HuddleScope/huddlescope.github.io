// Package config provides tests for teams configuration loading and registry wiring.
package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// writeTeamsYAML writes content to a temp file and returns its path.
func writeTeamsYAML(t *testing.T, content string) string {
	t.Helper()
	tmpfile, err := os.CreateTemp(t.TempDir(), "teams-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	_ = tmpfile.Close()
	return tmpfile.Name()
}

func TestToConfigTeam(t *testing.T) {
	yt := YAMLTeam{
		Slug:         "new-york-giants",
		Name:         "New York Giants",
		ShortName:    "Giants",
		Abbreviation: "NYG",
		Aliases:      []string{"Giants", "NY Giants"},
		Division:     "NFC East",
	}
	got := yt.toConfigTeam()
	//nolint:staticcheck // S1016: struct literal required for explicit field mapping
	want := Team{
		Slug:         yt.Slug,
		Name:         yt.Name,
		ShortName:    yt.ShortName,
		Abbreviation: yt.Abbreviation,
		Aliases:      yt.Aliases,
		Division:     yt.Division,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toConfigTeam() = %+v; want %+v", got, want)
	}
}

func TestLoadTeamsFromYAML(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		errMatch  string
		wantCount int
	}{
		{
			"single-team",
			`teams:
  - slug: ny-giants
    name: New York Giants
    short_name: Giants
    abbreviation: NYG
    aliases: [Giants, NY Giants]
    division: NFC East`,
			"", 1,
		},
		{
			"multiple-teams",
			`teams:
  - slug: ny-jets
    name: New York Jets
    short_name: Jets
    abbreviation: NYJ
  - slug: buffalo-bills
    name: Buffalo Bills
    short_name: Bills
    abbreviation: BUF`,
			"", 2,
		},
		{
			"missing-slug",
			`teams:
  - name: No Slug
    short_name: NS
    abbreviation: NS`,
			"missing required field 'slug'", 0,
		},
		{
			"missing-name",
			`teams:
  - slug: ny-giants
    short_name: Giants
    abbreviation: NYG`,
			"missing required field 'name'", 0,
		},
		{
			"missing-short-name",
			`teams:
  - slug: ny-giants
    name: New York Giants
    abbreviation: NYG`,
			"missing required field 'short_name'", 0,
		},
		{
			"missing-abbreviation",
			`teams:
  - slug: ny-giants
    name: New York Giants
    short_name: Giants`,
			"missing required field 'abbreviation'", 0,
		},
		{
			"duplicate-slug",
			`teams:
  - slug: ny-giants
    name: New York Giants
    short_name: Giants
    abbreviation: NYG
  - slug: ny-giants
    name: Duplicate
    short_name: Dup
    abbreviation: DUP`,
			"duplicate team slug", 0,
		},
		{
			"all-fields-preserved",
			`teams:
  - slug: sf-49ers
    name: San Francisco 49ers
    short_name: 49ers
    abbreviation: SF
    aliases: [49ers, SF 49ers, Niners]
    division: NFC West`,
			"", 1,
		},
		{
			"empty-teams",
			`teams: []`,
			"", 0,
		},
		{
			"empty-file",
			``,
			"", 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpfile := writeTeamsYAML(t, tt.yaml)
			cfg := Config{TeamsFile: tmpfile}
			got, err := cfg.loadTeamsFromYAML()

			if tt.errMatch != "" {
				if err == nil {
					t.Errorf("loadTeamsFromYAML() error = nil; want error containing %q", tt.errMatch)
				} else if !contains(err.Error(), tt.errMatch) {
					t.Errorf("loadTeamsFromYAML() error = %v; want error containing %q", err, tt.errMatch)
				}
				return
			}

			if err != nil {
				t.Fatalf("loadTeamsFromYAML() error = %v; want nil", err)
			}
			if len(got) != tt.wantCount {
				t.Fatalf("len(teams) = %d; want %d", len(got), tt.wantCount)
			}
		})
	}

	// Verify field preservation for the all-fields-preserved case
	tmpfile := writeTeamsYAML(t, `teams:
  - slug: sf-49ers
    name: San Francisco 49ers
    short_name: 49ers
    abbreviation: SF
    aliases: [49ers, SF 49ers, Niners]
    division: NFC West`)
	cfg := Config{TeamsFile: tmpfile}
	got, err := cfg.loadTeamsFromYAML()
	if err != nil {
		t.Fatalf("loadTeamsFromYAML() error = %v; want nil", err)
	}
	want := Team{
		Slug:         "sf-49ers",
		Name:         "San Francisco 49ers",
		ShortName:    "49ers",
		Abbreviation: "SF",
		Aliases:      []string{"49ers", "SF 49ers", "Niners"},
		Division:     "NFC West",
	}
	if !reflect.DeepEqual(got[0], want) {
		t.Errorf("teams[0] = %+v; want %+v", got[0], want)
	}
}

func TestLoadTeamsFromYAMLMissingFile(t *testing.T) {
	cfg := Config{TeamsFile: "/nonexistent/teams.yaml"}
	got, err := cfg.loadTeamsFromYAML()
	if err != nil {
		t.Fatalf("loadTeamsFromYAML() error = %v; want nil for missing file", err)
	}
	if got != nil {
		t.Errorf("loadTeamsFromYAML() = %v; want nil for missing file", got)
	}
}

func TestLoadTeamsFromYAMLInvalidYAML(t *testing.T) {
	tmpfile := writeTeamsYAML(t, `teams: [unclosed`)
	cfg := Config{TeamsFile: tmpfile}
	_, err := cfg.loadTeamsFromYAML()
	if err == nil {
		t.Fatal("loadTeamsFromYAML() error = nil; want parse error")
	}
	if !contains(err.Error(), "failed to parse teams file") {
		t.Errorf("loadTeamsFromYAML() error = %v; want 'failed to parse teams file'", err)
	}
}

func TestParseTeams(t *testing.T) {
	t.Run("already-parsed", func(t *testing.T) {
		cfg := Config{
			Teams: []Team{{Slug: "preset", Name: "Preset", ShortName: "P", Abbreviation: "PR"}},
		}
		if err := cfg.parseTeams(); err != nil {
			t.Fatalf("parseTeams() error = %v; want nil", err)
		}
		if len(cfg.Teams) != 1 || cfg.Teams[0].Slug != "preset" {
			t.Errorf("parseTeams() modified Teams = %v; want preset", cfg.Teams)
		}
	})

	t.Run("loads-from-yaml-file", func(t *testing.T) {
		tmpfile := writeTeamsYAML(t, `teams:
  - slug: ny-jets
    name: New York Jets
    short_name: Jets
    abbreviation: NYJ`)
		cfg := Config{TeamsFile: tmpfile}
		if err := cfg.parseTeams(); err != nil {
			t.Fatalf("parseTeams() error = %v; want nil", err)
		}
		if len(cfg.Teams) != 1 || cfg.Teams[0].Slug != "ny-jets" {
			t.Errorf("parseTeams() Teams = %v; want ny-jets", cfg.Teams)
		}
	})

	t.Run("missing-file-yields-empty", func(t *testing.T) {
		cfg := Config{TeamsFile: "/nonexistent/teams.yaml"}
		if err := cfg.parseTeams(); err != nil {
			t.Fatalf("parseTeams() error = %v; want nil", err)
		}
		if len(cfg.Teams) != 0 {
			t.Errorf("parseTeams() Teams = %v; want empty", cfg.Teams)
		}
	})

	t.Run("empty-file-yields-empty", func(t *testing.T) {
		tmpfile := writeTeamsYAML(t, "")
		cfg := Config{TeamsFile: tmpfile}
		if err := cfg.parseTeams(); err != nil {
			t.Fatalf("parseTeams() error = %v; want nil", err)
		}
		if len(cfg.Teams) != 0 {
			t.Errorf("parseTeams() Teams = %v; want empty", cfg.Teams)
		}
	})
}

func TestParseTeamsDefaultFilePath(t *testing.T) {
	_ = os.Setenv("HSC_TEAMS_FILE", "")
	defer func() { _ = os.Unsetenv("HSC_TEAMS_FILE") }()

	cfg := Config{}
	cfg.TeamsFile = ""
	// parseTeams only uses c.TeamsFile, not env directly; set default
	cfg.TeamsFile = defaultTeamsFile
	// Point to a nonexistent path so it yields empty without error
	cfg.TeamsFile = "/nonexistent/teams.yaml"
	if err := cfg.parseTeams(); err != nil {
		t.Fatalf("parseTeams() error = %v; want nil", err)
	}
	if len(cfg.Teams) != 0 {
		t.Errorf("parseTeams() Teams = %v; want empty", cfg.Teams)
	}
}

func TestConfigToDomainTeams(t *testing.T) {
	cfg := Config{
		Teams: []Team{
			{Slug: "ny-giants", Name: "New York Giants", ShortName: "Giants", Abbreviation: "NYG", Aliases: []string{"Giants"}},
			{Slug: "buffalo-bills", Name: "Buffalo Bills", ShortName: "Bills", Abbreviation: "BUF"},
		},
	}
	got := cfg.ToDomainTeams()
	if len(got) != 2 {
		t.Fatalf("ToDomainTeams() returned %d teams; want 2", len(got))
	}
	if got[0].Slug != "ny-giants" || got[0].Name != "New York Giants" || got[0].ShortName != "Giants" || got[0].Abbreviation != "NYG" {
		t.Errorf("ToDomainTeams()[0] = %+v; want ny-giants mapping", got[0])
	}
	if len(got[0].Aliases) != 1 || got[0].Aliases[0] != "Giants" {
		t.Errorf("ToDomainTeams()[0].Aliases = %v; want [Giants]", got[0].Aliases)
	}
	if got[1].Slug != "buffalo-bills" || got[1].Abbreviation != "BUF" {
		t.Errorf("ToDomainTeams()[1] = %+v; want buffalo-bills mapping", got[1])
	}
	if len(got[1].Aliases) != 0 {
		t.Errorf("ToDomainTeams()[1].Aliases = %v; want empty", got[1].Aliases)
	}
}

func TestConfigToDomainTeamsEmpty(t *testing.T) {
	cfg := Config{}
	got := cfg.ToDomainTeams()
	if len(got) != 0 {
		t.Errorf("ToDomainTeams() = %v; want empty", got)
	}
}

func TestConfigToDomainTeamsNilTeams(t *testing.T) {
	cfg := Config{Teams: nil}
	got := cfg.ToDomainTeams()
	if got == nil {
		t.Fatal("ToDomainTeams() = nil; want empty non-nil slice")
	}
	if len(got) != 0 {
		t.Errorf("ToDomainTeams() = %v; want empty", got)
	}
}

func TestTeamRegistry(t *testing.T) {
	cfg := Config{
		Teams: []Team{
			{Slug: "ny-giants", Name: "New York Giants", ShortName: "Giants", Abbreviation: "NYG"},
			{Slug: "buffalo-bills", Name: "Buffalo Bills", ShortName: "Bills", Abbreviation: "BUF"},
		},
	}
	reg, err := cfg.TeamRegistry()
	if err != nil {
		t.Fatalf("TeamRegistry() error = %v; want nil", err)
	}
	if reg == nil {
		t.Fatal("TeamRegistry() = nil; want registry")
	}
	if _, ok := reg.Get("ny-giants"); !ok {
		t.Error("TeamRegistry().Get('ny-giants') not found; want found")
	}
	if _, ok := reg.Get("buffalo-bills"); !ok {
		t.Error("TeamRegistry().Get('buffalo-bills') not found; want found")
	}
}

func TestTeamRegistryErrorOnEmptyTeams(t *testing.T) {
	cfg := Config{}
	reg, err := cfg.TeamRegistry()
	if err == nil {
		t.Fatal("TeamRegistry() with empty Teams did not error; want error")
	}
	if reg != nil {
		t.Errorf("TeamRegistry() reg = %v; want nil on error", reg)
	}
	if !contains(err.Error(), "teams are required") {
		t.Errorf("TeamRegistry() error = %v; want it to mention 'teams are required'", err)
	}
}

// baseValidConfig returns a Config that passes Validate with no teams or sources.
func baseValidConfig() Config {
	return Config{
		LogLevel:       "info",
		UserAgent:      "test/1",
		RequestTimeout: 30 * time.Second,
		HeartbeatFile:  "web/data/heartbeat.json",
	}
}

func TestValidateTeams(t *testing.T) {
	tests := []struct {
		name     string
		teams    []Team
		valid    bool
		errMatch string
	}{
		{
			"valid-teams",
			[]Team{
				{Slug: "a", Name: "Team A", ShortName: "A", Abbreviation: "TA"},
				{Slug: "b", Name: "Team B", ShortName: "B", Abbreviation: "TB"},
			},
			true, "",
		},
		{
			"empty-slug",
			[]Team{{Name: "No Slug", ShortName: "NS", Abbreviation: "NS"}},
			false, "empty slug",
		},
		{
			"empty-name",
			[]Team{{Slug: "a", ShortName: "A", Abbreviation: "TA"}},
			false, "empty name",
		},
		{
			"empty-short-name",
			[]Team{{Slug: "a", Name: "Team A", Abbreviation: "TA"}},
			false, "empty short_name",
		},
		{
			"empty-abbreviation",
			[]Team{{Slug: "a", Name: "Team A", ShortName: "A"}},
			false, "empty abbreviation",
		},
		{
			"duplicate-slug",
			[]Team{
				{Slug: "dup", Name: "First", ShortName: "F", Abbreviation: "F1"},
				{Slug: "dup", Name: "Second", ShortName: "S", Abbreviation: "S1"},
			},
			false, "duplicate team slug",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseValidConfig()
			cfg.Teams = tt.teams
			err := cfg.Validate()
			if tt.valid && err != nil {
				t.Errorf("Validate() error = %v; want nil", err)
			}
			if !tt.valid && err == nil {
				t.Errorf("Validate() error = nil; want error containing %q", tt.errMatch)
			}
			if !tt.valid && err != nil && !contains(err.Error(), tt.errMatch) {
				t.Errorf("Validate() error = %v; want error containing %q", err, tt.errMatch)
			}
		})
	}
}

func TestConfigToDomainAcceptanceUse(t *testing.T) {
	cfg := Config{
		Sources: []Source{
			{ID: "allowed-src", URL: "http://a.com", AcceptableUse: "allowed"},
			{ID: "restricted-src", URL: "http://b.com", AcceptableUse: "restricted"},
			{ID: "unknown-src", URL: "http://c.com", AcceptableUse: "bogus"},
		},
	}
	got := cfg.ToDomain()
	if len(got) != 3 {
		t.Fatalf("ToDomain() returned %d sources; want 3", len(got))
	}
	// allowed and restricted are covered by TestConfig_ToDomain; verify the default branch here
	if got[2].AcceptableUse != domain.AcceptanceUnknown {
		t.Errorf("ToDomain()[2].AcceptableUse = %v; want unknown for unrecognized value", got[2].AcceptableUse)
	}
}

func TestParseSourcesRawInvalidPriority(t *testing.T) {
	cfg := Config{}
	err := cfg.parseSourcesRaw("id,name,http://url.com,300,abc")
	if err == nil {
		t.Fatal("parseSourcesRaw() with invalid priority = nil error; want error")
	}
	if !contains(err.Error(), "invalid priority") {
		t.Errorf("parseSourcesRaw() error = %v; want 'invalid priority'", err)
	}
}

func TestParseSourcesRawInvalidTimeout(t *testing.T) {
	cfg := Config{}
	err := cfg.parseSourcesRaw("id,name,http://url.com,300,1,true,ESPN,en,abc")
	if err == nil {
		t.Fatal("parseSourcesRaw() with invalid timeout = nil error; want error")
	}
	if !contains(err.Error(), "invalid timeout") {
		t.Errorf("parseSourcesRaw() error = %v; want 'invalid timeout'", err)
	}
}

func TestParseSourcesRawInvalidFetchInterval(t *testing.T) {
	cfg := Config{}
	err := cfg.parseSourcesRaw("id,name,http://url.com,not-a-duration")
	if err == nil {
		t.Fatal("parseSourcesRaw() with invalid interval = nil error; want error")
	}
	if !contains(err.Error(), "invalid fetch interval") {
		t.Errorf("parseSourcesRaw() error = %v; want 'invalid fetch interval'", err)
	}
}

func TestValidateSourceErrorBranches(t *testing.T) {
	tests := []struct {
		name     string
		src      Source
		errMatch string
	}{
		{"empty-url", Source{ID: "id"}, "empty URL"},
		{"invalid-url", Source{ID: "id", URL: "not-a-url"}, "must use http or https"},
		{"invalid-scheme", Source{ID: "id", URL: "ftp://x.com"}, "http or https"},
		{"negative-fetch-seconds", Source{ID: "id", URL: "http://x.com", FetchSeconds: -1}, "FetchSeconds"},
		{"negative-priority", Source{ID: "id", URL: "http://x.com", Priority: -1}, "Priority"},
		{"negative-timeout", Source{ID: "id", URL: "http://x.com", Timeout: -1}, "Timeout"},
		{"invalid-acceptable-use", Source{ID: "id", URL: "http://x.com", AcceptableUse: "bogus"}, "AcceptableUse must be"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Sources: []Source{tt.src}}
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("Validate() error = nil; want error containing %q", tt.errMatch)
			}
			if !contains(err.Error(), tt.errMatch) {
				t.Errorf("Validate() error = %v; want error containing %q", err, tt.errMatch)
			}
		})
	}
}

func TestValidateDuplicateSourceID(t *testing.T) {
	cfg := Config{
		Sources: []Source{
			{ID: "dup", URL: "http://a.com"},
			{ID: "dup", URL: "http://b.com"},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil; want duplicate source ID error")
	}
	if !contains(err.Error(), "duplicate source ID") {
		t.Errorf("Validate() error = %v; want 'duplicate source ID'", err)
	}
}

func TestValidateProductionSuccess(t *testing.T) {
	cfg := Config{
		LogLevel:         "info",
		UserAgent:        "test/1",
		RequestTimeout:   30 * time.Second,
		HugoBaseURL:      "https://example.com",
		StorageBackend:   "turso",
		TursoDatabaseURL: "db",
		TursoAuthToken:   "token",
		Env:              "production",
		HeartbeatFile:    "web/data/heartbeat.json",
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error = %v; want nil for valid production config", err)
	}
}

func TestLoadFileSourcesRawError(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")
	content := "HSC_SOURCES=nfl-news,News,not-a-url"
	if err := os.WriteFile(envFile, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	_, err := Load(envFile)
	if err == nil {
		t.Fatal("Load() error = nil; want sources parse error")
	}
	if !contains(err.Error(), "invalid sources") {
		t.Errorf("Load() error = %v; want 'invalid sources'", err)
	}
}

func TestLoadTeamsFromYAMLFile(t *testing.T) {
	tmpDir := t.TempDir()
	teamsFile := filepath.Join(tmpDir, "teams.yaml")
	content := `teams:
  - slug: ny-giants
    name: New York Giants
    short_name: Giants
    abbreviation: NYG`
	if err := os.WriteFile(teamsFile, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cfg := Config{TeamsFile: teamsFile}
	if err := cfg.parseTeams(); err != nil {
		t.Fatalf("parseTeams() error = %v", err)
	}
	if len(cfg.Teams) != 1 || cfg.Teams[0].Slug != "ny-giants" {
		t.Errorf("Teams = %v; want ny-giants", cfg.Teams)
	}
}
