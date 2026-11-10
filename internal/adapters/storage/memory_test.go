package storage

import (
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

func TestMemoryRepository(t *testing.T) {
	repo := NewMemoryRepository()

	// Test Upsert
	testTime := time.Now().UTC()
	s1 := domain.Story{
		ID:          "test-1",
		Title:       "Test Story 1",
		Source:      "test-feed",
		PublishedAt: testTime,
		Summary:     "Test summary 1",
		FirstSeenAt: testTime,
	}
	if err := repo.Upsert(s1); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	// Test Upsert update
	s1.Title = "Updated Title"
	if err := repo.Upsert(s1); err != nil {
		t.Fatalf("Upsert update failed: %v", err)
	}

	// Test List
	stories, err := repo.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(stories) != 1 {
		t.Errorf("Expected 1 story, got %d", len(stories))
	}
	if stories[0].Title != "Updated Title" {
		t.Errorf("Expected updated title, got %s", stories[0].Title)
	}

	// Test Count
	count, err := repo.Count()
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected count 1, got %d", count)
	}
}

func TestMemoryRepository_Empty(t *testing.T) {
	repo := NewMemoryRepository()

	stories, err := repo.List()
	if err != nil {
		t.Fatalf("List on empty repo failed: %v", err)
	}
	if len(stories) != 0 {
		t.Errorf("Expected 0 stories, got %d", len(stories))
	}

	count, err := repo.Count()
	if err != nil {
		t.Fatalf("Count on empty repo failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected count 0, got %d", count)
	}
}
