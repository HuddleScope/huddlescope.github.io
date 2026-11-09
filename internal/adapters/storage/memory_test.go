package storage

import (
	"testing"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

func TestInMemoryRepository_Upsert_List_Count(t *testing.T) {
	repo := NewInMemoryRepository()

	// Initial state
	if count, err := repo.Count(); err != nil {
		t.Fatalf("Count() failed: %v", err)
	} else if count != 0 {
		t.Fatalf("Expected 0 stories, got %d", count)
	}

	// Upsert first story
	story1 := domain.Story{
		ID:     "story-1",
		Title:  "Story 1",
		Source: "test",
	}
	if err := repo.Upsert(story1); err != nil {
		t.Fatalf("Upsert(story1) failed: %v", err)
	}

	// Verify count
	if count, err := repo.Count(); err != nil {
		t.Fatalf("Count() failed: %v", err)
	} else if count != 1 {
		t.Fatalf("Expected 1 story, got %d", count)
	}

	// Upsert second story
	story2 := domain.Story{
		ID:     "story-2",
		Title:  "Story 2",
		Source: "test",
	}
	if err := repo.Upsert(story2); err != nil {
		t.Fatalf("Upsert(story2) failed: %v", err)
	}

	// Verify list and overwrite semantics
	story1Updated := domain.Story{
		ID:     "story-1",
		Title:  "Story 1 Updated",
		Source: "test",
	}
	if err := repo.Upsert(story1Updated); err != nil {
		t.Fatalf("Upsert(story1Updated) failed: %v", err)
	}

	list, err := repo.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	if len(list) != 2 {
		t.Fatalf("Expected 2 stories, got %d", len(list))
	}

	// Verify overwrite
	var found bool
	for _, s := range list {
		if s.ID == "story-1" && s.Title == "Story 1 Updated" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Overwrite semantics not applied: story-1 not updated")
	}

	// Verify Count after update
	if count, err := repo.Count(); err != nil {
		t.Fatalf("Count() after update failed: %v", err)
	} else if count != 2 {
		t.Fatalf("Expected 2 stories after update, got %d", count)
	}
}

func TestInMemoryRepository_Count(t *testing.T) {
	repo := NewInMemoryRepository()
	if count, err := repo.Count(); err != nil || count != 0 {
		t.Fatalf("Initial Count() wrong: %d, %v", count, err)
	}

	_ = repo.Upsert(domain.Story{ID: "1"})
	_ = repo.Upsert(domain.Story{ID: "2"})
	_ = repo.Upsert(domain.Story{ID: "3"})

	if count, err := repo.Count(); err != nil || count != 3 {
		t.Fatalf("Count after 3 stories wrong: %d, %v", count, err)
	}
}
