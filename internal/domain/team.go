// Package domain contains core domain models and interfaces.
package domain

import (
	"regexp"
	"sort"
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

// aliasPattern holds a pre-compiled regex for an alias and its slug.
type aliasPattern struct {
	alias string
	re    *regexp.Regexp
	slug  string
}

// TeamRegistry holds a lookup map for all known team names/aliases to slugs.
type TeamRegistry struct {
	mu       sync.RWMutex
	bySlug   map[string]*Team
	byAlias  map[string]string // normalized alias → slug
	patterns []aliasPattern    // pre-compiled patterns for Extract
}

// compileAliasPattern compiles a word-boundary regex for an alias.
// Returns empty pattern if alias is empty after normalization.
func compileAliasPattern(alias string) aliasPattern {
	normalized := normalizeAlias(alias)
	if normalized == "" {
		return aliasPattern{}
	}
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(normalized) + `\b`)
	return aliasPattern{alias: normalized, re: re, slug: ""}
}

// buildPatterns builds the pre-compiled patterns slice from byAlias.
// It sorts aliases for deterministic construction order.
func (r *TeamRegistry) buildPatterns() {
	aliases := make([]string, 0, len(r.byAlias))
	for alias := range r.byAlias {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)

	r.patterns = make([]aliasPattern, 0, len(aliases))
	for _, alias := range aliases {
		p := compileAliasPattern(alias)
		if p.re != nil {
			p.slug = r.byAlias[alias]
			r.patterns = append(r.patterns, p)
		}
	}
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
	r.buildPatterns()
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
		normalized := normalizeAlias(alias)
		// Skip empty aliases (shouldn't happen in config, but defensive)
		if normalized != "" {
			r.byAlias[normalized] = t.Slug
		}
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
// The output is deterministic (sorted) and the regexes are compiled once at registry construction.
func (r *TeamRegistry) Extract(text string) []string {
	if text == "" {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	normalizedText := strings.ToLower(text)
	matchedSlugs := make(map[string]bool) // dedupe

	for _, p := range r.patterns {
		if p.re.MatchString(normalizedText) {
			matchedSlugs[p.slug] = true
		}
	}

	// Build sorted result
	result := make([]string, 0, len(matchedSlugs))
	for slug := range matchedSlugs {
		result = append(result, slug)
	}
	sort.Strings(result)
	return result
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
