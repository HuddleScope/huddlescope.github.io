package storage

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

func TestMemoryRepositoryUpsert(t *testing.T) {
	repo := NewMemoryRepository()

	// Create a new story
	story := domain.Story{
		ID:          "test-id",
		Title:       "Test Story",
		Source:      "test-source",
		PublishedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Summary:     "Test summary",
		URL:         "https://example.com/test",
		GUID:        "guid-123",
		DedupeKey:   "test-source|guid-123",
	}

	status, err := repo.Upsert(story)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	if status != domain.UpsertCreated {
		t.Errorf("expected UpsertCreated, got %v", status)
	}

	// Upsert same story (unchanged)
	status, err = repo.Upsert(story)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	if status != domain.UpsertUnchanged {
		t.Errorf("expected UpsertUnchanged, got %v", status)
	}

	// Update meaningful field
	story.Title = "Updated Title"
	status, err = repo.Upsert(story)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	if status != domain.UpsertUpdated {
		t.Errorf("expected UpsertUpdated, got %v", status)
	}

	// Verify FirstSeenAt preserved
	list, err := repo.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 story, got %d", len(list))
	}
	if list[0].FirstSeenAt.IsZero() {
		t.Error("FirstSeenAt should not be zero")
	}
}

func TestMemoryRepositoryUpsertWithDedupeKey(t *testing.T) {
	repo := NewMemoryRepository()

	// First story with GUID
	story1 := domain.Story{
		ID:          "id1",
		Title:       "Story 1",
		Source:      "source1",
		PublishedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Summary:     "Summary 1",
		URL:         "https://example.com/1",
		GUID:        "guid-1",
		DedupeKey:   "source1|guid-1",
	}

	status, err := repo.Upsert(story1)
	if err != nil {
		t.Fatalf("First upsert failed: %v", err)
	}
	if status != domain.UpsertCreated {
		t.Errorf("expected UpsertCreated, got %v", status)
	}

	// Second story with same GUID but different ID (simulates duplicate with same GUID)
	story2 := domain.Story{
		ID:          "id2", // different ID but same dedupe key
		Title:       "Story 1 Updated",
		Source:      "source1",
		PublishedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Summary:     "Updated summary 1",
		URL:         "https://example.com/1",
		GUID:        "guid-1",
		DedupeKey:   "source1|guid-1",
	}

	status, err = repo.Upsert(story2)
	if err != nil {
		t.Fatalf("Second upsert failed: %v", err)
	}

	// Should be UpsertUpdated due to dedupe-key match
	if status != domain.UpsertUpdated {
		t.Errorf("expected UpsertUpdated due to dedupe-key match, got %v", status)
	}

	// Verify count is still 1
	list, err := repo.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 story (deduped), got %d", len(list))
	}

	// Verify the story has the updated title
	if list[0].Title != "Story 1 Updated" {
		t.Errorf("expected updated title, got %s", list[0].Title)
	}

	// Verify FirstSeenAt preserved from first insert
	if list[0].FirstSeenAt.IsZero() {
		t.Error("FirstSeenAt should be preserved from first insert")
	}
}

func TestMemoryRepositoryCount(t *testing.T) {
	repo := NewMemoryRepository()

	// Insert multiple stories
	for i := 0; i < 5; i++ {
		story := domain.Story{
			ID:          fmt.Sprintf("id%d", i),
			Title:       fmt.Sprintf("Story %d", i),
			Source:      "source1",
			PublishedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			Summary:     fmt.Sprintf("Summary %d", i),
			URL:         fmt.Sprintf("https://example.com/%d", i),
			GUID:        fmt.Sprintf("guid-%d", i),
			DedupeKey:   fmt.Sprintf("source1|guid-%d", i),
		}
		_, err := repo.Upsert(story)
		if err != nil {
			t.Fatalf("Upsert %d failed: %v", i, err)
		}
	}

	count, err := repo.Count()
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 5 {
		t.Errorf("expected count 5, got %d", count)
	}

	// Upsert same story (unchanged)
	story := domain.Story{
		ID:          "id0",
		Title:       "Story 0",
		Source:      "source1",
		PublishedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Summary:     "Summary 0",
		URL:         "https://example.com/0",
		GUID:        "guid-0",
		DedupeKey:   "source1|guid-0",
	}
	_, err = repo.Upsert(story)
	if err != nil {
		t.Fatalf("Second upsert failed: %v", err)
	}

	count, err = repo.Count()
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 5 {
		t.Errorf("count should remain 5 after unchanged upsert, got %d", count)
	}
}

func TestMemoryRepositoryUpsertEntityChange(t *testing.T) {
	repo := NewMemoryRepository()

	// First upsert with one entity
	baseStory := domain.Story{
		ID:          "test-entity-1",
		Title:       "Test Story with Entity",
		Source:      "test-source",
		PublishedAt: time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
		Summary:     "Test summary",
		URL:         "https://example.com/test",
		GUID:        "guid-entity-1",
		DedupeKey:   "test-source|guid-entity-1",
		Entities:    []string{"nfl"},
	}

	status, err := repo.Upsert(baseStory)
	if err != nil {
		t.Fatalf("First Upsert failed: %v", err)
	}
	if status != domain.UpsertCreated {
		t.Errorf("expected UpsertCreated, got %v", status)
	}

	// Second upsert with one new entity (same title, etc.)
	updatedStory := baseStory
	updatedStory.Entities = []string{"nfl", "nba"}

	status, err = repo.Upsert(updatedStory)
	if err != nil {
		t.Fatalf("Second Upsert failed: %v", err)
	}
	if status != domain.UpsertUpdated {
		t.Errorf("expected UpsertUpdated for new entity, got %v", status)
	}

	// Verify entities persisted
	list, err := repo.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 story, got %d", len(list))
	}

	// Verify both entities are present
	entities := list[0].Entities
	if len(entities) != 2 {
		t.Errorf("expected 2 entities, got %d: %v", len(entities), entities)
	}
	entitySet := make(map[string]bool)
	for _, e := range entities {
		entitySet[e] = true
	}
	if !entitySet["nfl"] || !entitySet["nba"] {
		t.Errorf("expected entities [nfl, nba], got %v", entities)
	}
}

func TestMemoryRepositoryUpsertEntityOrderUnchanged(t *testing.T) {
	repo := NewMemoryRepository()

	// First upsert with entities in specific order
	baseStory := domain.Story{
		ID:          "test-entity-order-1",
		Title:       "Test Story Entity Order",
		Source:      "test-source",
		PublishedAt: time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
		Summary:     "Test summary",
		URL:         "https://example.com/order-test",
		GUID:        "guid-order-1",
		DedupeKey:   "test-source|guid-order-1",
		Entities:    []string{"nba", "nfl", "mlb"},
	}

	status, err := repo.Upsert(baseStory)
	if err != nil {
		t.Fatalf("First Upsert failed: %v", err)
	}
	if status != domain.UpsertCreated {
		t.Errorf("expected UpsertCreated, got %v", status)
	}

	// Second upsert with identical entities in different order
	reorderedStory := baseStory
	reorderedStory.Entities = []string{"mlb", "nba", "nfl"}

	status, err = repo.Upsert(reorderedStory)
	if err != nil {
		t.Fatalf("Second Upsert failed: %v", err)
	}
	if status != domain.UpsertUnchanged {
		t.Errorf("expected UpsertUnchanged for same entities reordered, got %v", status)
	}

	// Verify list returns the same story (count 1)
	list, err := repo.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 story (reorder should not create duplicate), got %d", len(list))
	}
}

// TestMemoryRepositoryListDeterministicOrder is a regression test for the
// issue #165 flake: List() used to return map iteration order, which is
// randomized by the runtime. Snapshot builders tie-break on list order, so a
// nondeterministic List() made ContentEqual flaky and reported spurious
// "changed" publishes. List() must always return ID-sorted order.
func TestMemoryRepositoryListDeterministicOrder(t *testing.T) {
	repo := NewMemoryRepository()

	// Insert with IDs in an order that differs from sorted order, using
	// shuffled keys to force map bucket diversity.
	ids := []string{"zulu", "alpha", "mike", "kilo", "bravo", "echo", "hotel", "golf", "delta", "charlie"}
	for _, id := range ids {
		story := domain.Story{
			ID:          id,
			Title:       "T " + id,
			Source:      "src-" + id,
			PublishedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			Summary:     "S " + id,
			URL:         "https://example.com/" + id,
			GUID:        id,
			DedupeKey:   "src-" + id + "|" + id,
		}
		if _, err := repo.Upsert(story); err != nil {
			t.Fatalf("Upsert %s failed: %v", id, err)
		}
	}

	// Repeated List() calls must all be identical and sorted by ID.
	want := append([]string(nil), ids...)
	sort.Strings(want)

	for i := 0; i < 50; i++ {
		stories, err := repo.List()
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(stories) != len(ids) {
			t.Fatalf("List returned %d stories, want %d", len(stories), len(ids))
		}
		for j, s := range stories {
			if s.ID != want[j] {
				t.Fatalf("List()[%d].ID = %q, want %q (order must be deterministic ID-sorted)", j, s.ID, want[j])
			}
		}
	}
}
