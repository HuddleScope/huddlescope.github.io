// Package domain contains core domain models and interfaces.
package domain

import (
	"regexp"
	"strings"
	"sync"
)

// Team represents an NFL team with all its identifying information.
type Team struct {
	Slug           string   // Stable URL slug (e.g., "new-york-giants")
	Name           string   // Full legal name (e.g., "New York Giants")
	ShortName      string   // Short name (e.g., "Giants")
	Abbreviation   string   // Official abbreviation (e.g., "NYG")
	Aliases        []string // Common alternative names and nicknames
}

// TeamRegistry holds a lookup map for all known team names/aliases to slugs.
type TeamRegistry struct {
	mu       sync.RWMutex
	bySlug   map[string]*Team
	byAlias  map[string]string // normalized alias → slug
}

// NewTeamRegistry creates a new registry from a slice of Team.
func NewTeamRegistry(teams ...Team) *TeamRegistry {
	r := &TeamRegistry{
		bySlug:  make(map[string]*Team),
		byAlias: make(map[string]string),
	}
	for i := range teams {
		r.addTeam(&teams[i])
	}
	return r
}

// addTeam inserts a team into the registry.
func (r *TeamRegistry) addTeam(t *Team) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Register by slug
	r.bySlug[t.Slug] = t

	// Register name (full name) as alias
	r.byAlias[normalizeAlias(t.Name)] = t.Slug

	// Register short name if distinct from full name
	if t.ShortName != t.Name {
		r.byAlias[normalizeAlias(t.ShortName)] = t.Slug
	}

	// Register abbreviation
	r.byAlias[normalizeAlias(t.Abbreviation)] = t.Slug

	// Register aliases
	for _, alias := range t.Aliases {
		r.byAlias[normalizeAlias(alias)] = t.Slug
	}
}

// normalizeAlias normalizes an alias for lookup (lowercase, trimmed).
func normalizeAlias(alias string) string {
	return strings.TrimSpace(strings.ToLower(alias))
}

// Lookup returns the team slug for a given name/alias, and a boolean indicating if found.
func (r *TeamRegistry) Lookup(alias string) (slug string, found bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	slug, found = r.byAlias[normalizeAlias(alias)]
	return slug, found
}

// Get returns the full Team for a given slug.
func (r *TeamRegistry) Get(slug string) (*Team, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, found := r.bySlug[slug]
	return t, found
}

// Extract finds all team mentions in the given text and returns their slugs.
// It matches team names using word boundaries.
func (r *TeamRegistry) Extract(text string) []string {
	if text == "" {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var matches []string
	matchedSlugs := make(map[string]bool) // dedupe

	// Scan for all known aliases using word boundaries
	for alias, slug := range r.byAlias {
		if r.matchText(alias, text, matchedSlugs) {
			matches = append(matches, slug)
		}
	}

	return matches
}

// matchText checks if the alias appears in the text with word boundaries.
func (r *TeamRegistry) matchText(alias, text string, matchedSlugs map[string]bool) bool {
	normalizedText := strings.ToLower(text)
	normalizedAlias := normalizeAlias(alias)

	// Use word boundary regex to match standalone tokens
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(normalizedAlias) + `\b`)

	// Check if the alias matches
	if re.MatchString(normalizedText) {
		// Find the actual slug for this alias
		slug, _ := r.byAlias[normalizedAlias]
		if slug != "" && !matchedSlugs[slug] {
			matchedSlugs[slug] = true
			return true
		}
	}
	return false
}

// All returns all teams in the registry (copy).
func (r *TeamRegistry) All() []Team {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := len(r.bySlug)
	result := make([]Team, 0, count)
	for _, t := range r.bySlug {
		result = append(result, *t)
	}
	return result
}

// Count returns the number of teams in the registry.
func (r *TeamRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.bySlug)
}

// ---NFL Teams Data---

// SeedNFLTeams returns all 32 current NFL teams.
func SeedNFLTeams() []Team {
	return []Team{
		// AFC East
		{Slug: "baltimore-ravens", Name: "Baltimore Ravens", ShortName: "Ravens", Abbreviation: "BAL", Aliases: []string{"balt ravs", "balt ravens", "ravens"}},
		{Slug: "buffalo-bills", Name: "Buffalo Bills", ShortName: "Bills", Abbreviation: "BUF", Aliases: []string{"bills", "buff bills"}},
		{Slug: "miami-dolphins", Name: "Miami Dolphins", ShortName: "Dolphins", Abbreviation: "MIA", Aliases: []string{"miami dflns", "miami dolphins", "dolphins"}},
		{Slug: "new-england-patriots", Name: "New England Patriots", ShortName: "Patriots", Abbreviation: "NE", Aliases: []string{"n e patriots", "n epatriots", "new england", "patriots"}},

		// AFC North
		{Slug: "cincinnati-bengals", Name: "Cincinnati Bengals", ShortName: "Bengals", Abbreviation: "CIN", Aliases: []string{"cincy bengals", "cincinnati", "bengals"}},
		{Slug: "cleveland-browns", Name: "Cleveland Browns", ShortName: "Browns", Abbreviation: "CLE", Aliases: []string{"cle browns", "cleveland", "browns"}},
		{Slug: "pittsburgh-steelers", Name: "Pittsburgh Steelers", ShortName: "Steelers", Abbreviation: "PIT", Aliases: []string{"pittsburgh steelers", "steelers"}},
		{Slug: "tennessee-titans", Name: "Tennessee Titans", ShortName: "Titans", Abbreviation: "TEN", Aliases: []string{"ten titans", "tennessee titans", "titans"}}, // AFC South
		{Slug: "houston-texans", Name: "Houston Texans", ShortName: "Texans", Abbreviation: "HOU", Aliases: []string{"hou texans", "houston texans", "texans"}},
		{Slug: "indianapolis-colts", Name: "Indianapolis Colts", ShortName: "Colts", Abbreviation: "IND", Aliases: []string{"ind colts", "indianapolis colts", "colts"}},
		{Slug: "jacksonville-jaguars", Name: "Jacksonville Jaguars", ShortName: "Jaguars", Abbreviation: "JAX", Aliases: []string{"jax jaguars", "jacksonville jaguars", "jaguars"}},

		// AFC West
		{Slug: "denver-broncos", Name: "Denver Broncos", ShortName: "Broncos", Abbreviation: "DEN", Aliases: []string{"den broncos", "denver broncos", "broncos"}},
		{Slug: "kansas-city-chiefs", Name: "Kansas City Chiefs", ShortName: "Chiefs", Abbreviation: "KC", Aliases: []string{"k c chiefs", "kansascity chiefs", "kansas city", "chiefs"}},
		{Slug: "las-vegas-raiders", Name: "Las Vegas Raiders", ShortName: "Raiders", Abbreviation: "LV", Aliases: []string{"lv raiders", "lasvegas raiders", "vegas raiders", "oakland raiders", "raiders"}},
		{Slug: "los-angeles-chargers", Name: "Los Angeles Chargers", ShortName: "Chargers", Abbreviation: "LAC", Aliases: []string{"la chargers", "losangeles chargers", "san diego chargers", "chargers"}},

		// NFC East
		{Slug: "dallas-cowboys", Name: "Dallas Cowboys", ShortName: "Cowboys", Abbreviation: "DAL", Aliases: []string{"dallas cowboys", "cowboys"}},
		{Slug: "new-york-giants", Name: "New York Giants", ShortName: "Giants", Abbreviation: "NYG", Aliases: []string{"ny giants", "n y giants", "new york giants", "giants"}},
		{Slug: "new-york-jets", Name: "New York Jets", ShortName: "Jets", Abbreviation: "NYJ", Aliases: []string{"ny jets", "n y jets", "new york jets", "jets"}},
		{Slug: "philadelphia-eagles", Name: "Philadelphia Eagles", ShortName: "Eagles", Abbreviation: "PHI", Aliases: []string{"philly eagles", "philadelphia eagles", "eagles"}},
		{Slug: "washington-commanders", Name: "Washington Commanders", ShortName: "Commanders", Abbreviation: "WSH", Aliases: []string{"washington", "wsh commanders", "washington commanders", "redskins"}},

		// NFC North
		{Slug: "chicago-bears", Name: "Chicago Bears", ShortName: "Bears", Abbreviation: "CHI", Aliases: []string{"chi bears", "chicago bears", "bears"}},
		{Slug: "detroit-lions", Name: "Detroit Lions", ShortName: "Lions", Abbreviation: "DET", Aliases: []string{"detroit lions", "lions"}},
		{Slug: "green-bay-packers", Name: "Green Bay Packers", ShortName: "Packers", Abbreviation: "GB", Aliases: []string{"gb packers", "greenbay packers", "green bay", "packers"}},
		{Slug: "minnesota-vikings", Name: "Minnesota Vikings", ShortName: "Vikings", Abbreviation: "MIN", Aliases: []string{"min vikings", "minnesota vikings", "vikings"}},

		// NFC South
		{Slug: "atlanta-falcons", Name: "Atlanta Falcons", ShortName: "Falcons", Abbreviation: "ATL", Aliases: []string{"atl falcons", "atlanta falcons", "falcons"}},
		{Slug: "carolina-panthers", Name: "Carolina Panthers", ShortName: "Panthers", Abbreviation: "CAR", Aliases: []string{"car panthers", "carolina panthers", "panthers"}},
		{Slug: "new-orleans-saints", Name: "New Orleans Saints", ShortName: "Saints", Abbreviation: "NO", Aliases: []string{"no saints", "neworleans saints", "new orleans saints", "saints"}},
		{Slug: "tampa-bay-buccaneers", Name: "Tampa Bay Buccaneers", ShortName: "Buccaneers", Abbreviation: "TB", Aliases: []string{"tb bucs", "tb buccaneers", "tampabay bucs", "tampa bay bucs", "buccaneers"}},

		// NFC West
		{Slug: "arizona-cardinals", Name: "Arizona Cardinals", ShortName: "Cardinals", Abbreviation: "ARI", Aliases: []string{"ari cardinals", "arizona cardinals", "cardinals"}},
		{Slug: "los-angeles-rams", Name: "Los Angeles Rams", ShortName: "Rams", Abbreviation: "LAR", Aliases: []string{"la rams", "losangeles rams", "st louis rams", "rams"}},
		{Slug: "san-francisco-49ers", Name: "San Francisco 49ers", ShortName: "49ers", Abbreviation: "SF", Aliases: []string{"sf 49ers", "sanfrancisco 49ers", "san francisco 49ers", "niners", "49ers"}},
		{Slug: "seattle-seahawks", Name: "Seattle Seahawks", ShortName: "Seahawks", Abbreviation: "SEA", Aliases: []string{"sea seahawks", "seattle seahawks", "seahawks"}},
	}
}

// DefaultTeamRegistry returns a pre-built registry with all 32 NFL teams.
func DefaultTeamRegistry() *TeamRegistry {
	return NewTeamRegistry(SeedNFLTeams()...)
}