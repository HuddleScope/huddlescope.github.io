// Package domain contains core domain models and interfaces.
package domain

import (
	"regexp"
	"strings"
	"sync"
)

// Team represents an NFL team with all its identifying information.
type Team struct {
	Slug         string   // Stable URL slug (e.g., "new-york-giants")
	Name         string   // Full legal name (e.g., "New York Giants")
	ShortName    string   // Short name (e.g., "Giants")
	Abbreviation string   // Official abbreviation (e.g., "NYG")
	Aliases      []string // Common alternative names and nicknames
}

// TeamRegistry holds a lookup map for all known team names/aliases to slugs.
type TeamRegistry struct {
	mu      sync.RWMutex
	bySlug  map[string]*Team
	byAlias map[string]string // normalized alias → slug
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


// DefaultTeamRegistry returns an empty registry.
// DEPRECATED: Teams should be loaded from config via Config.TeamRegistry().
// This function is kept for backward compatibility and testing only.
// DO NOT USE IN PRODUCTION - teams must be loaded from conf/teams.yaml
func DefaultTeamRegistry() *TeamRegistry {
	return NewTeamRegistry()
}

// NewTeamRegistryFromSlice creates a TeamRegistry from a slice of Team.
func NewTeamRegistryFromSlice(teams []Team) *TeamRegistry {
	return NewTeamRegistry(teams...)
}
