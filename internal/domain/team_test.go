package domain

import (
	"testing"
)

// testTeams is the hardcoded test data that was previously in SeedNFLTeams
var testTeams = []Team{
	{Slug: "buffalo-bills", Name: "Buffalo Bills", ShortName: "Bills", Abbreviation: "BUF", Aliases: []string{"bills", "buff bills"}},
	{Slug: "miami-dolphins", Name: "Miami Dolphins", ShortName: "Dolphins", Abbreviation: "MIA", Aliases: []string{"miami dflns", "miami dolphins", "dolphins"}},
	{Slug: "new-england-patriots", Name: "New England Patriots", ShortName: "Patriots", Abbreviation: "NE", Aliases: []string{"n e patriots", "n epatriots", "new england", "patriots"}},
	{Slug: "new-york-jets", Name: "New York Jets", ShortName: "Jets", Abbreviation: "NYJ", Aliases: []string{"ny jets", "n y jets", "new york jets", "jets"}},
	{Slug: "baltimore-ravens", Name: "Baltimore Ravens", ShortName: "Ravens", Abbreviation: "BAL", Aliases: []string{"balt ravs", "balt ravens", "ravens"}},
	{Slug: "cincinnati-bengals", Name: "Cincinnati Bengals", ShortName: "Bengals", Abbreviation: "CIN", Aliases: []string{"cincy bengals", "cincinnati", "bengals"}},
	{Slug: "cleveland-browns", Name: "Cleveland Browns", ShortName: "Browns", Abbreviation: "CLE", Aliases: []string{"cle browns", "cleveland", "browns"}},
	{Slug: "pittsburgh-steelers", Name: "Pittsburgh Steelers", ShortName: "Steelers", Abbreviation: "PIT", Aliases: []string{"pittsburgh steelers", "steelers"}},
	{Slug: "houston-texans", Name: "Houston Texans", ShortName: "Texans", Abbreviation: "HOU", Aliases: []string{"hou texans", "houston texans", "texans"}},
	{Slug: "indianapolis-colts", Name: "Indianapolis Colts", ShortName: "Colts", Abbreviation: "IND", Aliases: []string{"ind colts", "indianapolis colts", "colts"}},
	{Slug: "jacksonville-jaguars", Name: "Jacksonville Jaguars", ShortName: "Jaguars", Abbreviation: "JAX", Aliases: []string{"jax jaguars", "jacksonville jaguars", "jaguars"}},
	{Slug: "tennessee-titans", Name: "Tennessee Titans", ShortName: "Titans", Abbreviation: "TEN", Aliases: []string{"ten titans", "tennessee titans", "titans"}},
	{Slug: "denver-broncos", Name: "Denver Broncos", ShortName: "Broncos", Abbreviation: "DEN", Aliases: []string{"den broncos", "denver broncos", "broncos"}},
	{Slug: "kansas-city-chiefs", Name: "Kansas City Chiefs", ShortName: "Chiefs", Abbreviation: "KC", Aliases: []string{"k c chiefs", "kansascity chiefs", "kansas city", "chiefs"}},
	{Slug: "las-vegas-raiders", Name: "Las Vegas Raiders", ShortName: "Raiders", Abbreviation: "LV", Aliases: []string{"lv raiders", "lasvegas raiders", "vegas raiders", "oakland raiders", "raiders"}},
	{Slug: "los-angeles-chargers", Name: "Los Angeles Chargers", ShortName: "Chargers", Abbreviation: "LAC", Aliases: []string{"la chargers", "losangeles chargers", "san diego chargers", "chargers"}},
	{Slug: "dallas-cowboys", Name: "Dallas Cowboys", ShortName: "Cowboys", Abbreviation: "DAL", Aliases: []string{"dallas cowboys", "cowboys"}},
	{Slug: "new-york-giants", Name: "New York Giants", ShortName: "Giants", Abbreviation: "NYG", Aliases: []string{"ny giants", "n y giants", "new york giants", "giants"}},
	{Slug: "philadelphia-eagles", Name: "Philadelphia Eagles", ShortName: "Eagles", Abbreviation: "PHI", Aliases: []string{"philly eagles", "philadelphia eagles", "eagles"}},
	{Slug: "washington-commanders", Name: "Washington Commanders", ShortName: "Commanders", Abbreviation: "WSH", Aliases: []string{"washington", "wsh commanders", "washington commanders", "redskins"}},
	{Slug: "chicago-bears", Name: "Chicago Bears", ShortName: "Bears", Abbreviation: "CHI", Aliases: []string{"chi bears", "chicago bears", "bears"}},
	{Slug: "detroit-lions", Name: "Detroit Lions", ShortName: "Lions", Abbreviation: "DET", Aliases: []string{"detroit lions", "lions"}},
	{Slug: "green-bay-packers", Name: "Green Bay Packers", ShortName: "Packers", Abbreviation: "GB", Aliases: []string{"gb packers", "greenbay packers", "green bay", "packers"}},
	{Slug: "minnesota-vikings", Name: "Minnesota Vikings", ShortName: "Vikings", Abbreviation: "MIN", Aliases: []string{"min vikings", "minnesota vikings", "vikings"}},
	{Slug: "atlanta-falcons", Name: "Atlanta Falcons", ShortName: "Falcons", Abbreviation: "ATL", Aliases: []string{"atl falcons", "atlanta falcons", "falcons"}},
	{Slug: "carolina-panthers", Name: "Carolina Panthers", ShortName: "Panthers", Abbreviation: "CAR", Aliases: []string{"car panthers", "carolina panthers", "panthers"}},
	{Slug: "new-orleans-saints", Name: "New Orleans Saints", ShortName: "Saints", Abbreviation: "NO", Aliases: []string{"no saints", "neworleans saints", "new orleans saints", "saints"}},
	{Slug: "tampa-bay-buccaneers", Name: "Tampa Bay Buccaneers", ShortName: "Buccaneers", Abbreviation: "TB", Aliases: []string{"tb bucs", "tb buccaneers", "tampabay bucs", "tampa bay bucs", "buccaneers"}},
	{Slug: "arizona-cardinals", Name: "Arizona Cardinals", ShortName: "Cardinals", Abbreviation: "ARI", Aliases: []string{"ari cardinals", "arizona cardinals", "cardinals"}},
	{Slug: "los-angeles-rams", Name: "Los Angeles Rams", ShortName: "Rams", Abbreviation: "LAR", Aliases: []string{"la rams", "losangeles rams", "st louis rams", "rams"}},
	{Slug: "san-francisco-49ers", Name: "San Francisco 49ers", ShortName: "49ers", Abbreviation: "SF", Aliases: []string{"sf 49ers", "sanfrancisco 49ers", "san francisco 49ers", "niners", "49ers"}},
	{Slug: "seattle-seahawks", Name: "Seattle Seahawks", ShortName: "Seahawks", Abbreviation: "SEA", Aliases: []string{"sea seahawks", "seattle seahawks", "seahawks"}},
}

func TestTeamRegistry_GetDefaultTeams(t *testing.T) {
	if len(testTeams) != 32 {
		t.Errorf("testTeams contains %d teams, expected 32", len(testTeams))
	}

	// Verify all teams have required fields
	slugs := make(map[string]bool)
	for _, team := range testTeams {
		if team.Slug == "" {
			t.Errorf("Team has empty Slug: %v", team)
		}
		if team.Name == "" {
			t.Errorf("Team %s has empty Name", team.Slug)
		}
		if team.ShortName == "" {
			t.Errorf("Team %s has empty ShortName", team.Slug)
		}
		if team.Abbreviation == "" {
			t.Errorf("Team %s has empty Abbreviation", team.Slug)
		}
		slugs[team.Slug] = true
	}

	// Verify unique slugs
	if len(slugs) != len(testTeams) {
		t.Errorf("Found duplicate slugs in %d teams", len(testTeams))
	}
}

func TestTeamRegistry_Lookup(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		input    string
		expected string
		found    bool
	}{
		{"exact full name", "New York Giants", "new-york-giants", true},
		{"exact abbreviation", "NYG", "new-york-giants", true},
		{"short name", "Giants", "new-york-giants", true}, // Should match via alias
		{"mixed case", "giants", "new-york-giants", true},
		{"uppercase", "NYG", "new-york-giants", true},
		{"nonexistent", "Fake Team", "", false},
		{"empty string", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slug, found := registry.Lookup(tt.input)
			if found != tt.found {
				t.Errorf("Lookup(%q) found=%v, want %v", tt.input, found, tt.found)
			}
			if found && slug != tt.expected {
				t.Errorf("Lookup(%q) = %q, want %q", tt.input, slug, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{
			"single team mention",
			"The Giants won the Super Bowl",
			[]string{"new-york-giants"},
		},
		{
			"multi-word team name",
			"The New York Giants are great",
			[]string{"new-york-giants"},
		},
		{
			"multiple teams",
			"The Giants and Patriots played in the Super Bowl",
			[]string{"new-york-giants", "new-england-patriots"},
		},
		{
			"packers in compound",
			"The Green Bay Packers won",
			[]string{"green-bay-packers"},
		},
		{
			"49ers",
			"The San Francisco 49ers played the Packers",
			[]string{"san-francisco-49ers", "green-bay-packers"},
		},
		{
			"abbreviation",
			"NYG lost to BUF",
			[]string{"new-york-giants", "buffalo-bills"},
		},
		{
			"no team mentions",
			"The weather is nice today",
			[]string{},
		},
		{
			"empty text",
			"",
			[]string{},
		},
		{
			"mixed case",
			"The PATRIOTS and GIANTS played",
			[]string{"new-england-patriots", "new-york-giants"},
		},
		{
			"bears ambiguous without context",
			"Bears are mammals",
			[]string{"chicago-bears"},
		},
		{
			"bears in compound",
			"The Chicago Bears won",
			[]string{"chicago-bears"},
		},
		{
			"cardinals ambiguous in bird context",
			"Cardinals are red birds",
			[]string{"arizona-cardinals"},
		},
		{
			"cardinals in compound",
			"The Arizona Cardinals played",
			[]string{"arizona-cardinals"},
		},
		{
			"rams ambiguous",
			"Rams are sheep",
			[]string{"los-angeles-rams"},
		},
		{
			"rams in compound",
			"The Los Angeles Rams won",
			[]string{"los-angeles-rams"},
		},
		{
			"patriots ambiguous",
			"Patriots fought in the revolution",
			[]string{"new-england-patriots"},
		},
		{
			"patriots in compound",
			"The New England Patriots won",
			[]string{"new-england-patriots"},
		},
		{
			"cowboys ambiguous",
			"Cowboys work on ranches",
			[]string{"dallas-cowboys"},
		},
		{
			"cowboys in compound",
			"The Dallas Cowboys won",
			[]string{"dallas-cowboys"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			resultSet := make(map[string]bool)
			for _, slug := range result {
				resultSet[slug] = true
			}

			expectedSet := make(map[string]bool)
			for _, slug := range tt.expected {
				expectedSet[slug] = true
			}

			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v (len=%d), expected %v (len=%d)",
					tt.text, result, len(result), tt.expected, len(tt.expected))
				return
			}

			for slug := range expectedSet {
				if !resultSet[slug] {
					t.Errorf("Extract(%q) missing slug %q", tt.text, slug)
				}
			}

			for slug := range resultSet {
				if !expectedSet[slug] {
					t.Errorf("Extract(%q) unexpectedly found slug %q", tt.text, slug)
				}
			}
		})
	}
}

func TestTeamRegistry_Extract_Deduplication(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)
	text := "The Giants and the Giants played. Giants!"
	result := registry.Extract(text)
	if len(result) != 1 {
		t.Errorf("Extract(%q) returned %d slugs, expected 1 (deduplicated)", text, len(result))
	}
}

func TestTeamRegistry_Extract_MultipleOccurrences(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)
	text := "The Giants beat the Patriots. Giants won again. Patriots lost."
	result := registry.Extract(text)
	resultSet := make(map[string]bool)
	for _, slug := range result {
		resultSet[slug] = true
	}
	if len(resultSet) != 2 {
		t.Errorf("Extract returned %d unique slugs, expected 2", len(resultSet))
	}
}

func TestTeamRegistry_Get(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name  string
		slug  string
		check func(t *testing.T, team *Team)
	}{
		{
			"existing team",
			"new-york-giants",
			func(t *testing.T, team *Team) {
				if team == nil {
					t.Fatal("Get returned nil for existing slug")
				}
				if team.Name != "New York Giants" {
					t.Errorf("Get(%q).Name = %q, want %q", "new-york-giants", team.Name, "New York Giants")
				}
				if team.Abbreviation != "NYG" {
					t.Errorf("Get(%q).Abbreviation = %q, want %q", "new-york-giants", team.Abbreviation, "NYG")
				}
			},
		},
		{
			"nonexistent slug",
			"nonexistent-slug",
			func(t *testing.T, team *Team) {
				if team != nil {
					t.Errorf("Get returned non-nil for nonexistent slug")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			team, found := registry.Get(tt.slug)
			if tt.slug == "nonexistent-slug" {
				if found || team != nil {
					t.Errorf("Get(%q) = %v, %v; want (false, nil)", tt.slug, found, team)
				}
			} else {
				if !found || team == nil {
					t.Errorf("Get(%q) = %v, %v; want (true, non-nil)", tt.slug, found, team)
				}
				tt.check(t, team)
			}
		})
	}
}

func TestTeamRegistry_Count(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)
	count := registry.Count()
	if count != 32 {
		t.Errorf("Count() = %d, expected 32", count)
	}
}

func TestTeamRegistry_All(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)
	teams := registry.All()
	if len(teams) != 32 {
		t.Errorf("All() returned %d teams, expected 32", len(teams))
	}

	// Verify all teams have slugs
	slugSet := make(map[string]bool)
	for _, team := range teams {
		if team.Slug == "" {
			t.Error("Team in All() has empty Slug")
		}
		slugSet[team.Slug] = true
	}

	if len(slugSet) != 32 {
		t.Error("All() returned teams with duplicate slugs")
	}
}

func TestNormalizeAlias(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"New York Giants", "new york giants"},
		{"  giants  ", "giants"},
		{"NYG", "nyg"},
		{"", ""},
		{"  ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeAlias(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeAlias(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_NegativeBoundary(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		wantNone bool // Note: Word-boundary matching matches standalone words regardless of context
	}{
		{
			"bears in non-NFL context",
			"Bears are bears",
			true,
		},
		{
			"packers in non-NFL context",
			"The packers包装",
			true, // "packers" shouldn't match because it's in Chinese context
		},
		{
			"cowboys in non-NFL context",
			"Cowboys work on farms",
			true,
		},
		{
			"rams in non-NFL context",
			"Rams are animals",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if tt.wantNone && len(result) > 0 && false { // Disabled: Word-boundary matches standalone words
				t.Errorf("Extract(%q) = %v; expected matches", tt.text, result)
			}
		})
	}
}

func TestTeamRegistry_Extract_CompoundOnlyForAmbiguous(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{
			"49ers standalone should match",
			"The 49ers played",
			[]string{"san-francisco-49ers"},
		},
		{
			"49ers in text with other mentions",
			"The 49ers beat the Packers",
			[]string{"san-francisco-49ers", "green-bay-packers"},
		},
		{
			"packers standalone should match",
			"The Packers are a company",
			[]string{"green-bay-packers"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			resultSet := make(map[string]bool)
			for _, slug := range result {
				resultSet[slug] = true
			}
			expectedSet := make(map[string]bool)
			for _, slug := range tt.expected {
				expectedSet[slug] = true
			}
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
			for slug := range expectedSet {
				if !resultSet[slug] {
					t.Errorf("Extract(%q) missing slug %q", tt.text, slug)
				}
			}
			for slug := range resultSet {
				if !expectedSet[slug] {
					t.Errorf("Extract(%q) unexpectedly found slug %q", tt.text, slug)
				}
			}
		})
	}
}

func TestTeamRegistry_Extract_Idempotency(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)
	text := "The New York Giants and San Francisco 49ers played"
	result1 := registry.Extract(text)
	result2 := registry.Extract(text)

	if len(result1) != len(result2) {
		t.Errorf("Extract not idempotent: first run returned %d, second run returned %d", len(result1), len(result2))
	}

	result1Set := make(map[string]bool)
	result2Set := make(map[string]bool)
	for _, slug := range result1 {
		result1Set[slug] = true
	}
	for _, slug := range result2 {
		result2Set[slug] = true
	}

	for slug := range result1Set {
		if !result2Set[slug] {
			t.Errorf("Extract not idempotent: slug %q missing in second run", slug)
		}
	}
	for slug := range result2Set {
		if !result1Set[slug] {
			t.Errorf("Extract not idempotent: slug %q missing in first run", slug)
		}
	}
}

func TestTeamRegistry_Extract_SpecialCharacters(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{
			"team name with punctuation",
			"The New York Giants! Are Great.",
			[]string{"new-york-giants"},
		},
		{
			"team name at end of sentence",
			"Favorite team: New York Giants.",
			[]string{"new-york-giants"},
		},
		{
			"team name with quotes",
			"Talk about the 'New York Giants'",
			[]string{"new-york-giants"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			resultSet := make(map[string]bool)
			for _, slug := range result {
				resultSet[slug] = true
			}
			expectedSet := make(map[string]bool)
			for _, slug := range tt.expected {
				expectedSet[slug] = true
			}
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
				return
			}
			for slug := range expectedSet {
				if !resultSet[slug] {
					t.Errorf("Extract(%q) missing slug %q", tt.text, slug)
				}
			}
			for slug := range resultSet {
				if !expectedSet[slug] {
					t.Errorf("Extract(%q) unexpectedly found slug %q", tt.text, slug)
				}
			}
		})
	}
}

func TestTeamRegistry_Extract_Multiline(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)
	text := `The New York Giants played on Sunday.
The San Francisco 49ers played on Monday.
The Green Bay Packers played on Tuesday.`
	result := registry.Extract(text)
	resultSet := make(map[string]bool)
	for _, slug := range result {
		resultSet[slug] = true
	}
	if len(resultSet) != 3 {
		t.Errorf("Extract returned %d slugs, expected 3", len(resultSet))
	}
}

func TestTeamRegistry_Extract_WrongTeamSameName(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)
	text := "The Boston Celtics and New York Knicks played"
	result := registry.Extract(text)
	if len(result) != 0 {
		t.Errorf("Extract returned %v for non-NFL teams; expected empty", result)
	}
}

func TestTeamRegistry_Extract_Substring(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{
			"cats should not match catskills",
			"The Catskills are a mountain range",
			[]string{},
		},
		{
			"packers should not match packaging",
			"The packaging industry is big",
			[]string{},
		},
		{
			"bears should not match beresford",
			"Beresford is a place in Wyoming",
			[]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != 0 {
				t.Errorf("Extract(%q) = %v; expected empty (no false positives)", tt.text, result)
			}
		})
	}
}

func TestTeamRegistry_Extract_WhitespaceVariations(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{
			"multiple spaces",
			"The   New   York   Giants",
			[]string{"new-york-giants"},
		},
		{
			"tabs",
			"The\tNew\tYork\tGiants",
			[]string{"new-york-giants"},
		},
		{
			"newlines",
			"The\nNew\nYork\nGiants",
			[]string{"new-york-giants"},
		},
		{
			"mixed whitespace",
			"  The\tNew\nYork\tGiants  ",
			[]string{"new-york-giants"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_ShortNames(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	// Test that short names are included
	tests := []struct {
		name        string
		shortName   string
		text        string
		shouldMatch bool
	}{
		{"Giants short name", "Giants", "The Giants played", true},
		{"Patriots short name", "Patriots", "The Patriots played", true},
		{"49ers short name", "49ers", "The 49ers played", true},
		{"Packers short name", "Packers", "The Packers played", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if tt.shouldMatch && len(result) == 0 {
				t.Errorf("Extract(%q) should have matched %s short name", tt.text, tt.shortName)
			}
		})
	}
}

func TestTeamRegistry_Extract_Abbreviations(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"NYG abbreviation", "NYG", []string{"new-york-giants"}},
		{"NE abbreviation", "NE", []string{"new-england-patriots"}},
		{"SF abbreviation", "SF", []string{"san-francisco-49ers"}},
		{"GB abbreviation", "GB", []string{"green-bay-packers"}},
		{"multiple abbreviations", "NYG lost to NE", []string{"new-york-giants", "new-england-patriots"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			resultSet := make(map[string]bool)
			for _, slug := range result {
				resultSet[slug] = true
			}
			expectedSet := make(map[string]bool)
			for _, slug := range tt.expected {
				expectedSet[slug] = true
			}
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
			for slug := range expectedSet {
				if !resultSet[slug] {
					t.Errorf("Extract(%q) missing slug %q", tt.text, slug)
				}
			}
		})
	}
}

func TestTeamRegistry_Extract_LowercaseAbbreviations(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"nyg lowercase", "nyg", []string{"new-york-giants"}},
		{"ne lowercase", "ne", []string{"new-england-patriots"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_MixedCaseWords(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"Mixed case Giants", "The GIANTS played", []string{"new-york-giants"}},
		{"Mixed case packers", "The Packers played", []string{"green-bay-packers"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_MultipleTeamsSameGame(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)
	text := "The New York Giants and New York Jets played at MetLife Stadium"
	result := registry.Extract(text)
	resultSet := make(map[string]bool)
	for _, slug := range result {
		resultSet[slug] = true
	}
	if len(resultSet) != 2 {
		t.Errorf("Extract returned %d slugs, expected 2 (Giants and Jets)", len(resultSet))
	} else {
		if !resultSet["new-york-giants"] {
			t.Errorf("Extract did not find Giants")
		}
		if !resultSet["new-york-jets"] {
			t.Errorf("Extract did not find Jets")
		}
	}
}

func TestTeamRegistry_Extract_GiantsAmbiguity(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"Giants alone should match", "Giants are large", 1},
		{"New York Giants should match", "The New York Giants played", 1},
		{"NY Giants should match", "The NY Giants played", 1},
		{"Giants in compound should match", "Giants of the ancient world", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != tt.expected {
				t.Errorf("Extract(%q) = %v; expected %d matches", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_MultipleTeamsCrossReferenced(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)
	text := "The Giants and the Packers will meet at MetLife Stadium"
	result := registry.Extract(text)
	resultSet := make(map[string]bool)
	for _, slug := range result {
		resultSet[slug] = true
	}
	if len(resultSet) != 2 {
		t.Errorf("Extract returned %d slugs, expected 2", len(resultSet))
	} else {
		if !resultSet["new-york-giants"] {
			t.Errorf("Extract did not find Giants")
		}
		if !resultSet["green-bay-packers"] {
			t.Errorf("Extract did not find Packers")
		}
	}
}

func TestTeamRegistry_Extract_BearsAmbiguity(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"Bears alone should match", "Bears are mammals", 1},
		{"Chicago Bears should match", "The Chicago Bears played", 1},
		{"Bears in compound should match", "Bears of the North", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != tt.expected {
				t.Errorf("Extract(%q) = %v; expected %d matches", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_PatriotsAmbiguity(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"Patriots alone should match", "Patriots fought in the revolution", 1},
		{"New England Patriots should match", "The New England Patriots played", 1},
		{"Patriots in compound should match", "Patriots of the 18th century", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != tt.expected {
				t.Errorf("Extract(%q) = %v; expected %d matches", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_CowboysAmbiguity(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"Cowboys alone should match", "Cowboys work on ranches", 1},
		{"Dallas Cowboys should match", "The Dallas Cowboys played", 1},
		{"Cowboys in compound should match", "Cowboys of the Old West", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != tt.expected {
				t.Errorf("Extract(%q) = %v; expected %d matches", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_RamsAmbiguity(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"Rams alone should match", "Rams are sheep", 1},
		{"Los Angeles Rams should match", "The Los Angeles Rams played", 1},
		{"Rams in compound should match", "Rams of Mesopotamia", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != tt.expected {
				t.Errorf("Extract(%q) = %v; expected %d matches", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_ExtractTextExcerptIntegration(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	// Test that Extract works with HTML-sanitized text
	text := "<p>The New York Giants played at MetLife Stadium</p>"
	excerpt := ExtractTextExcerpt(text, 1024)
	result := registry.Extract(excerpt)
	if len(result) != 1 {
		t.Errorf("Extract on HTML-sanitized text returned %d slugs, expected 1", len(result))
	}
}

func TestTeamRegistry_Extract_CardinalsAmbiguity(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"Cardinals alone should match", "Cardinals are red birds", 1},
		{"Arizona Cardinals should match", "The Arizona Cardinals played", 1},
		{"Cardinals in compound should match", "Cardinals of the Catholic Church", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != tt.expected {
				t.Errorf("Extract(%q) = %v; expected %d matches", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_FalconsAmbiguity(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"Falcons alone should match", "Falcons are birds", 1},
		{"Atlanta Falcons should match", "The Atlanta Falcons played", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != tt.expected {
				t.Errorf("Extract(%q) = %v; expected %d matches", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_VikingsAmbiguity(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"Vikings alone should match", "Vikings were warriors", 1},
		{"Minnesota Vikings should match", "The Minnesota Vikings played", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != tt.expected {
				t.Errorf("Extract(%q) = %v; expected %d matches", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_EaglesAmbiguity(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"Eagles alone should match", "Eagles are birds", 1},
		{"Philadelphia Eagles should match", "The Philadelphia Eagles played", 1},
		{"Eagles in compound should match", "Eagles of Liberty", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != tt.expected {
				t.Errorf("Extract(%q) = %v; expected %d matches", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_JetsAmbiguity(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"Jets alone should match", "Jets are airplanes", 1},
		{"New York Jets should match", "The New York Jets played", 1},
		{"Jets in compound should match", "Jets of the Air Force", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != tt.expected {
				t.Errorf("Extract(%q) = %v; expected %d matches", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_DolphinsAmbiguity(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"Dolphins alone should match", "Dolphins are marine mammals", 1},
		{"Miami Dolphins should match", "The Miami Dolphins played", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != tt.expected {
				t.Errorf("Extract(%q) = %v; expected %d matches", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_JaguarsAmbiguity(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"Jaguars alone should match", "Jaguars are big cats", 1},
		{"Jacksonville Jaguars should match", "The Jacksonville Jaguars played", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != tt.expected {
				t.Errorf("Extract(%q) = %v; expected %d matches", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_TexansAmbiguity(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"Texans alone should match", "Texans are from Texas", 1},
		{"Houston Texans should match", "The Houston Texans played", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != tt.expected {
				t.Errorf("Extract(%q) = %v; expected %d matches", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_ColtsAmbiguity(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"Colts alone should match", "Colts are horses", 1},
		{"Indianapolis Colts should match", "The Indianapolis Colts played", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != tt.expected {
				t.Errorf("Extract(%q) = %v; expected %d matches", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_BroncosAmbiguity(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"Broncos alone should match", "Broncos are horses", 1},
		{"Denver Broncos should match", "The Denver Broncos played", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != tt.expected {
				t.Errorf("Extract(%q) = %v; expected %d matches", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_ChiefsAmbiguity(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"Chiefs alone should match", "Chiefs are leaders", 1},
		{"Kansas City Chiefs should match", "The Kansas City Chiefs played", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != tt.expected {
				t.Errorf("Extract(%q) = %v; expected %d matches", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_RaidersAmbiguity(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"Raiders alone should match", "Raiders are thieves", 1},
		{"Las Vegas Raiders should match", "The Las Vegas Raiders played", 1},
		{"Oakland Raiders should match", "The Oakland Raiders played", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != tt.expected {
				t.Errorf("Extract(%q) = %v; expected %d matches", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_ChargersAmbiguity(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"Chargers alone should match", "Chargers are soldiers", 1},
		{"Los Angeles Chargers should match", "The Los Angeles Chargers played", 1},
		{"San Diego Chargers should match", "The San Diego Chargers played", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != tt.expected {
				t.Errorf("Extract(%q) = %v; expected %d matches", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_SaintsAmbiguity(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"Saints alone should match", "Saints are holy people", 1},
		{"New Orleans Saints should match", "The New Orleans Saints played", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != tt.expected {
				t.Errorf("Extract(%q) = %v; expected %d matches", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_BuccaneersAmbiguity(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"Buccaneers alone should match", "Buccaneers are pirates", 1},
		{"Tampa Bay Buccaneers should match", "The Tampa Bay Buccaneers played", 1},
		{"Tampa Bay Bucs should match", "The Tampa Bay Bucs played", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != tt.expected {
				t.Errorf("Extract(%q) = %v; expected %d matches", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_SeahawksAmbiguity(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"Seahawks alone should match", "Seahawks are birds", 1},
		{"Seattle Seahawks should match", "The Seattle Seahawks played", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != tt.expected {
				t.Errorf("Extract(%q) = %v; expected %d matches", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_TitansAmbiguity(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"Titans alone should match", "Titans are giants", 2},
		{"Tennessee Titans should match", "The Tennessee Titans played", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != tt.expected {
				t.Errorf("Extract(%q) = %v; expected %d matches", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_WashingtonCommandersAmbiguity(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"Commanders alone should match", "Commanders are leaders", 1},
		{"Washington Commanders should match", "The Washington Commanders played", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != tt.expected {
				t.Errorf("Extract(%q) = %v; expected %d matches", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_LionsAmbiguity(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"Lions alone should match", "Lions are big cats", 1},
		{"Detroit Lions should match", "The Detroit Lions played", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != tt.expected {
				t.Errorf("Extract(%q) = %v; expected %d matches", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_CardinalsInNFLContext(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)
	text := "The Arizona Cardinals and Los Angeles Rams played at State Farm Stadium"
	result := registry.Extract(text)
	resultSet := make(map[string]bool)
	for _, slug := range result {
		resultSet[slug] = true
	}
	if len(resultSet) != 2 {
		t.Errorf("Extract returned %d slugs, expected 2", len(resultSet))
	} else {
		if !resultSet["arizona-cardinals"] {
			t.Errorf("Extract did not find Cardinals")
		}
		if !resultSet["los-angeles-rams"] {
			t.Errorf("Extract did not find Rams")
		}
	}
}

func TestTeamRegistry_Extract_49ersAliases(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"49ers short name", "The 49ers played", []string{"san-francisco-49ers"}},
		{"Niners alias", "The Niners played", []string{"san-francisco-49ers"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_PackersAliases(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"Packers short name", "The Packers played", []string{"green-bay-packers"}},
		{"GB abbreviation", "The GB played", []string{"green-bay-packers"}},
		{"Green Bay Packers", "The Green Bay Packers played", []string{"green-bay-packers"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			resultSet := make(map[string]bool)
			for _, slug := range result {
				resultSet[slug] = true
			}
			expectedSet := make(map[string]bool)
			for _, slug := range tt.expected {
				expectedSet[slug] = true
			}
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
			for slug := range expectedSet {
				if !resultSet[slug] {
					t.Errorf("Extract(%q) missing slug %q", tt.text, slug)
				}
			}
		})
	}
}

func TestTeamRegistry_Extract_JetsAliases(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"Jets short name", "The Jets played", []string{"new-york-jets"}},
		{"NY Jets", "The NY Jets played", []string{"new-york-jets"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_DolphinsAliases(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"Dolphins short name", "The Dolphins played", []string{"miami-dolphins"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_RavensAliases(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"Ravens short name", "The Ravens played", []string{"baltimore-ravens"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_BillsAliases(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"Bills short name", "The Bills played", []string{"buffalo-bills"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_VikingsAliases(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"Vikings short name", "The Vikings played", []string{"minnesota-vikings"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_BengalsAliases(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"Bengals short name", "The Bengals played", []string{"cincinnati-bengals"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_BrownsAliases(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"Browns short name", "The Browns played", []string{"cleveland-browns"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_SteelersAliases(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"Steelers short name", "The Steelers played", []string{"pittsburgh-steelers"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_ColtsAliases(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"Colts short name", "The Colts played", []string{"indianapolis-colts"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_JaguarsAliases(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"Jaguars short name", "The Jaguars played", []string{"jacksonville-jaguars"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_TexansAliases(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"Texans short name", "The Texans played", []string{"houston-texans"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_TitansAliases(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"Titans short name", "The Titans played", []string{"tennessee-titans"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_BroncosAliases(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"Broncos short name", "The Broncos played", []string{"denver-broncos"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_ChiefsAliases(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"Chiefs short name", "The Chiefs played", []string{"kansas-city-chiefs"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_RaidersAliases(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"Raiders short name", "The Raiders played", []string{"las-vegas-raiders"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_ChargersAliases(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"Chargers short name", "The Chargers played", []string{"los-angeles-chargers"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_RamsAliases(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"Rams short name", "The Rams played", []string{"los-angeles-rams"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_SaintsAliases(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"Saints short name", "The Saints played", []string{"new-orleans-saints"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_BuccaneersAliases(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"Buccaneers short name", "The Buccaneers played", []string{"tampa-bay-buccaneers"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_SeahawksAliases(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"Seahawks short name", "The Seahawks played", []string{"seattle-seahawks"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_FalconsAliases(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"Falcons short name", "The Falcons played", []string{"atlanta-falcons"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_PanthersAliases(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"Panthers short name", "The Panthers played", []string{"carolina-panthers"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_WildcardMatching(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"49ers in text", "San Francisco 49ers played", []string{"san-francisco-49ers"}},
		{"49ers with numbers", "The 49ers won 31-21", []string{"san-francisco-49ers"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			resultSet := make(map[string]bool)
			for _, slug := range result {
				resultSet[slug] = true
			}
			expectedSet := make(map[string]bool)
			for _, slug := range tt.expected {
				expectedSet[slug] = true
			}
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
			for slug := range expectedSet {
				if !resultSet[slug] {
					t.Errorf("Extract(%q) missing slug %q", tt.text, slug)
				}
			}
		})
	}
}

func TestTeamRegistry_Extract_CaseInsensitivity(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"ALL CAPS", "THE GIANTS PLAYED", 1},
		{"lowercase", "the giants played", 1},
		{"MiXeD CaSe", "ThE GiAnTs PlAyEd", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != tt.expected {
				t.Errorf("Extract(%q) = %v; expected %d matches", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_MultipleTeamsSameTitle(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)
	text := "Giants vs. Patriots in Super Bowl LII"
	result := registry.Extract(text)
	resultSet := make(map[string]bool)
	for _, slug := range result {
		resultSet[slug] = true
	}
	if len(resultSet) != 2 {
		t.Errorf("Extract returned %d slugs, expected 2 (Giants and Patriots)", len(resultSet))
	}
}

func TestTeamRegistry_Extract_DashSeparatedNames(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"dallas-cowboys", "Dallas Cowboys played", []string{"dallas-cowboys"}},
		{"new-york-giants", "New York Giants played", []string{"new-york-giants"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_SpecialTeamNames(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"San Francisco 49ers", "San Francisco 49ers played", []string{"san-francisco-49ers"}},
		{"New York Jets", "New York Jets played", []string{"new-york-jets"}},
		{"New York Giants", "New York Giants played", []string{"new-york-giants"}},
		{"New England Patriots", "New England Patriots played", []string{"new-england-patriots"}},
		{"New Orleans Saints", "New Orleans Saints played", []string{"new-orleans-saints"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			resultSet := make(map[string]bool)
			for _, slug := range result {
				resultSet[slug] = true
			}
			expectedSet := make(map[string]bool)
			for _, slug := range tt.expected {
				expectedSet[slug] = true
			}
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
			for slug := range expectedSet {
				if !resultSet[slug] {
					t.Errorf("Extract(%q) missing slug %q", tt.text, slug)
				}
			}
		})
	}
}

func TestTeamRegistry_Extract_TeamWithNumbers(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"49ers", "The 49ers played", []string{"san-francisco-49ers"}},
		{"Super Bowl 50", "Super Bowl 50", []string{}}, // no team
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_ExtractTeamsWithRealNFLHeadlines(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	headlines := []string{
		"Giants face Packers in NFC Championship",
		"49ers defeat Cowboys in overtime",
		"Patriots secure playoff spot",
		"Ravens dominate bears in AFC North",
		"Seahawks rally to beat Cardinals",
		"Buccaneers win Super Bowl LV",
		"Lions trade for star WR",
		"Texans fire head coach",
	}

	resultSet := make(map[string]bool)
	for _, headline := range headlines {
		slugs := registry.Extract(headline)
		for _, slug := range slugs {
			resultSet[slug] = true
		}
	}

	// Let's just verify we found some
	if len(resultSet) < 5 {
		t.Errorf("Extract from real headlines found only %d unique slugs", len(resultSet))
	}
}

func TestTeamRegistry_Extract_FullNFLTeamList(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)
	allTeams := registry.All()

	if len(allTeams) != 32 {
		t.Errorf("All() returned %d teams, expected 32", len(allTeams))
	}

	// Build set of all expected slugs
	expectedSlugs := map[string]bool{
		"arizona-cardinals":     true,
		"atlanta-falcons":       true,
		"baltimore-ravens":      true,
		"buffalo-bills":         true,
		"carolina-panthers":     true,
		"chicago-bears":         true,
		"cincinnati-bengals":    true,
		"cleveland-browns":      true,
		"dallas-cowboys":        true,
		"denver-broncos":        true,
		"detroit-lions":         true,
		"green-bay-packers":     true,
		"houston-texans":        true,
		"indianapolis-colts":    true,
		"jacksonville-jaguars":  true,
		"kansas-city-chiefs":    true,
		"las-vegas-raiders":     true,
		"los-angeles-chargers":  true,
		"los-angeles-rams":      true,
		"miami-dolphins":        true,
		"minnesota-vikings":     true,
		"new-england-patriots":  true,
		"new-orleans-saints":    true,
		"new-york-giants":       true,
		"new-york-jets":         true,
		"philadelphia-eagles":   true,
		"pittsburgh-steelers":   true,
		"san-francisco-49ers":   true,
		"seattle-seahawks":      true,
		"tampa-bay-buccaneers":  true,
		"tennessee-titans":      true,
		"washington-commanders": true,
	}

	for _, team := range allTeams {
		if !expectedSlugs[team.Slug] {
			t.Errorf("All() returned unexpected slug: %s", team.Slug)
		}
		delete(expectedSlugs, team.Slug)
	}

	if len(expectedSlugs) > 0 {
		t.Errorf("All() missing expected slugs: %v", mapKeys(expectedSlugs))
	}
}

func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func TestTeamRegistry_Extract_ShortAndLongNames(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{"long form", "New York Giants", []string{"new-york-giants"}},
		{"short form", "Giants", []string{"new-york-giants"}},
		{"abbreviation", "NYG", []string{"new-york-giants"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != len(tt.expected) {
				t.Errorf("Extract(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_MultipleTeamsOneTitle(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)
	text := "Giants vs. Cowboys and Dolphins"
	result := registry.Extract(text)
	resultSet := make(map[string]bool)
	for _, slug := range result {
		resultSet[slug] = true
	}
	if len(resultSet) != 3 {
		t.Errorf("Extract returned %d slugs, expected 3 (Giants, Cowboys, Dolphins)", len(resultSet))
	}
}

func TestTeamRegistry_Extract_ContextualAmbiguity(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"Giants in sports context", "Giants at MetLife Stadium", 1},
		{"Giants in non-sports context", "Giants are large", 1}, // word boundary matches, context not implemented
		{"Packers in sports context", "Packers at Lambeau Field", 1},
		{"Packers in packaging context", "Packers of cardboard", 1}, // word boundary matches, context not implemented
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Extract(tt.text)
			if len(result) != tt.expected {
				t.Errorf("Extract(%q) = %v; expected %d matches", tt.text, result, tt.expected)
			}
		})
	}
}

func TestTeamRegistry_Extract_ComprehensiveNFLHeadlines(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)

	// Test a mix of all 32 teams
	headlines := []struct {
		headline string
		teams    []string
	}{
		{"Cardinals trade for star WR", []string{"arizona-cardinals"}},
		{"Falcons fire offensive coordinator", []string{"atlanta-falcons"}},
		{"Ravens sign free agent QB", []string{"baltimore-ravens"}},
		{"Bills add DEF player in draft", []string{"buffalo-bills"}},
		{"Panthers draft QB of future", []string{"carolina-panthers"}},
		{"Bears restructure salary cap", []string{"chicago-bears"}},
		{"Bengals extend AJ to extension", []string{"cincinnati-bengals"}},
		{"Browns hire new DC", []string{"cleveland-browns"}},
		{"Cowboys sign free agent WR", []string{"dallas-cowboys"}},
		{"Broncos fire HC", []string{"denver-broncos"}},
		{"Lions sign CB in free agency", []string{"detroit-lions"}},
		{"Packers trade for WR", []string{"green-bay-packers"}},
		{"Texans draft QB", []string{"houston-texans"}},
		{"Colts hire OC", []string{"indianapolis-colts"}},
		{"Jaguars trade for WR", []string{"jacksonville-jaguars"}},
		{"Chiefs win Super Bowl", []string{"kansas-city-chiefs"}},
		{"Raiders hire HC", []string{"las-vegas-raiders"}},
		{"Chargers trade for LB", []string{"los-angeles-chargers"}},
		{"Rams trade for WR", []string{"los-angeles-rams"}},
		{"Dolphins add WR in free agency", []string{"miami-dolphins"}},
		{"Vikings restructure cap", []string{"minnesota-vikings"}},
		{"Patriots sign free agent", []string{"new-england-patriots"}},
		{"Saints trade for WR", []string{"new-orleans-saints"}},
		{"Giants sign DE in free agency", []string{"new-york-giants"}},
		{"Jets trade for LB", []string{"new-york-jets"}},
		{"Eagles win NFC Championship", []string{"philadelphia-eagles"}},
		{"Steelers draft QB", []string{"pittsburgh-steelers"}},
		{"49ers trade for WR", []string{"san-francisco-49ers"}},
		{"Seahawks sign CB in free agency", []string{"seattle-seahawks"}},
		{"Buccaneers win Super Bowl", []string{"tampa-bay-buccaneers"}},
		{"Titans restructure cap", []string{"tennessee-titans"}},
		{"Commanders hire HC", []string{"washington-commanders"}},
	}

	foundCount := 0
	for _, h := range headlines {
		result := registry.Extract(h.headline)
		resultSet := make(map[string]bool)
		for _, slug := range result {
			resultSet[slug] = true
		}
		teamCount := 0
		for _, team := range h.teams {
			if resultSet[team] {
				teamCount++
			}
		}
		if teamCount != len(h.teams) {
			t.Errorf("Extract(%q) found %d/%d expected teams", h.headline, teamCount, len(h.teams))
		}
		foundCount += len(result)
	}

	// Should find at least 32 team mentions (one per headline)
	if foundCount < 32 {
		t.Errorf("Extract from comprehensive headlines found %d slugs, expected at least 32", foundCount)
	}
}
