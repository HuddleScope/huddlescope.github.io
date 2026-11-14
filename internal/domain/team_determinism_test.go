package domain

import (
	"sort"
	"testing"
)

// TestTeamRegistry_Extract_DeterministicOrder tests that Extract returns
// the same sorted order across repeated calls.
func TestTeamRegistry_Extract_DeterministicOrder(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)
	// Use a text that matches 9 distinct teams
	text := "The Giants and Patriots and Cowboys and Bears and Lions and Chiefs and Eagles and Rams and 49ers played"

	// Run 100 times and assert identical output each time
	var lastResult []string
	for i := 0; i < 100; i++ {
		result := registry.Extract(text)
		if i == 0 {
			lastResult = result
		} else {
			if len(result) != len(lastResult) {
				t.Errorf("Extract count mismatch on run %d: got %d, expected %d", i+1, len(result), len(lastResult))
			}
			for j := range result {
				if result[j] != lastResult[j] {
					t.Errorf("Extract order mismatch on run %d at index %d: got %q, expected %q", i+1, j, result[j], lastResult[j])
				}
			}
		}
	}

	// Assert sorted order
	if !sort.StringsAreSorted(lastResult) {
		t.Errorf("Extract result not sorted: %v", lastResult)
	}
}

// TestTeamRegistry_Extract_EmptyAliasGuard tests that an empty alias
// does not produce a degenerate pattern that matches every word.
func TestTeamRegistry_Extract_EmptyAliasGuard(t *testing.T) {
	// Create a registry with a team that has an empty alias (should be skipped)
	registry := NewTeamRegistry(
		Team{Slug: "dummy-team", Name: "Dummy Team", ShortName: "Dummy", Abbreviation: "DT", Aliases: []string{"", "  ", "dummy"}},
	)
	text := "The quick brown fox jumps"
	result := registry.Extract(text)
	if len(result) != 0 {
		t.Errorf("Extract(%q) returned %v for non-team text; expected empty (empty aliases should be skipped)", text, result)
	}
}

// TestTeamRegistry_Extract_EmptyText tests that Extract on empty text returns nil.
func TestTeamRegistry_Extract_EmptyText(t *testing.T) {
	registry := NewTeamRegistry(testTeams...)
	result := registry.Extract("")
	if result != nil {
		t.Errorf("Extract(\"\") = %v, expected nil", result)
	}
	// Second call should also be nil (idempotent)
	result2 := registry.Extract("")
	if result2 != nil {
		t.Errorf("Extract(\"\") (second call) = %v, expected nil", result2)
	}
}

// TestStoryCanonicalizeNoTeamExtraction tests that Canonicalize does NOT
// extract teams from the entry (dead DefaultTeamRegistry() removal).
func TestStoryCanonicalizeNoTeamExtraction(t *testing.T) {
	fe := FeedEntry{
		Title:    "The New York Giants won",
		Source:   "test-source",
		Summary:  "The Giants beat the Patriots",
		Entities: []string{"original-entity-1", "original-entity-2"},
	}

	story := Story{}.Canonicalize(fe)

	// Entities should be a copy of entry.Entities (no extraction)
	expected := []string{"original-entity-1", "original-entity-2"}
	if len(story.Entities) != len(expected) {
		t.Errorf("Canonicalize Entities length mismatch: got %d, expected %d", len(story.Entities), len(expected))
	}
	for i, e := range expected {
		if story.Entities[i] != e {
			t.Errorf("Canonicalize Entities[%d]: got %q, expected %q", i, story.Entities[i], e)
		}
	}

	// Verify non-aliasing: mutating the original FeedEntry.Entities doesn't affect story
	fe.Entities[0] = "modified"
	if story.Entities[0] == "modified" {
		t.Error("CanonicalizeEntities aliased entry.Entities; should be a copy")
	}
}
