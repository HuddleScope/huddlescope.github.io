package storage

import (
	"fmt"
	"sync"
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
	}
	status, err := repo.Upsert(s1)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	if status != domain.UpsertCreated {
		t.Fatalf("Expected UpsertCreated, got %s", status)
	}

	// Test Upsert update
	s1.Title = "Updated Title"
	status, err = repo.Upsert(s1)
	if err != nil {
		t.Fatalf("Upsert update failed: %v", err)
	}
	if status != domain.UpsertUpdated {
		t.Fatalf("Expected UpsertUpdated, got %s", status)
	}

	// Test Upsert unchanged
	status, err = repo.Upsert(s1)
	if err != nil {
		t.Fatalf("Upsert unchanged failed: %v", err)
	}
	if status != domain.UpsertUnchanged {
		t.Fatalf("Expected UpsertUnchanged, got %s", status)
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

func TestMemoryRepository_Idempotency(t *testing.T) {
	// Test AC1: Re-ingest of same entry returns Unchanged (not Created)
	repo := NewMemoryRepository()
	testTime := time.Now().UTC()

	// First upsert - should create
	story1 := domain.Story{
		ID:          "id-1",
		Title:       "Test Title",
		Source:      "test-feed",
		PublishedAt: testTime,
		Summary:     "Original summary",
	}
	status, err := repo.Upsert(story1)
	if err != nil {
		t.Fatalf("First Upsert failed: %v", err)
	}
	if status != domain.UpsertCreated {
		t.Fatalf("First Upsert: expected Created, got %s", status)
	}

	// Second upsert of same ID with same data - should be unchanged
	status, err = repo.Upsert(story1)
	if err != nil {
		t.Fatalf("Second Upsert failed: %v", err)
	}
	if status != domain.UpsertUnchanged {
		t.Fatalf("Second Upsert: expected Unchanged, got %s", status)
	}

	// Third upsert with same ID and data - should still be unchanged
	status, err = repo.Upsert(story1)
	if err != nil {
		t.Fatalf("Third Upsert failed: %v", err)
	}
	if status != domain.UpsertUnchanged {
		t.Fatalf("Third Upsert: expected Unchanged, got %s", status)
	}

	// Verify only 1 record exists
	count, err := repo.Count()
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Count = %d; want 1 (idempotent, no duplicates)", count)
	}
}

func TestMemoryRepository_Concurrency(t *testing.T) {
	// Test AC4: Concurrent Upsert does not create duplicates
	repo := NewMemoryRepository()
	numKeys := 100
	done := make(chan bool, numKeys*2)

	// Concurrent upsert of unique keys - should result in exactly numKeys records
	for i := 0; i < numKeys; i++ {
		go func(idx int) {
			story := domain.Story{
				ID:          fmt.Sprintf("id-%d", idx),
				Title:       fmt.Sprintf("Title %d", idx),
				Source:      "test-feed",
				PublishedAt: time.Now().UTC(),
				Summary:     fmt.Sprintf("Summary %d", idx),
			}
			_, err := repo.Upsert(story)
			if err != nil {
				t.Errorf("Concurrent Upsert(%d) failed: %v", idx, err)
			}
			done <- true
		}(i)
	}

	// Wait for all unique key upserts
	for i := 0; i < numKeys; i++ {
		<-done
	}

	// Verify no duplicates for unique keys
	count, err := repo.Count()
	if err != nil {
		t.Fatalf("Post-concurrency Count failed: %v", err)
	}
	if count != numKeys {
		t.Errorf("After %d concurrent Upsert calls with unique keys, Count = %d; want %d", numKeys, count, numKeys)
	}

	// Concurrent upsert of same key - should result in exactly 1 record
	repo2 := NewMemoryRepository()
	story := domain.Story{
		ID:          "same-id",
		Title:       "Same Title",
		Source:      "test-feed",
		PublishedAt: time.Now().UTC(),
		Summary:     "Same summary",
	}
	for i := 0; i < numKeys; i++ {
		go func() {
			_, err := repo2.Upsert(story)
			if err != nil {
				t.Errorf("Concurrent same-key Upsert failed: %v", err)
			}
			done <- true
		}()
	}

	// Wait for all same-key upserts
	for i := 0; i < numKeys; i++ {
		<-done
	}

	// Verify no duplicate for same key
	count, err = repo2.Count()
	if err != nil {
		t.Fatalf("Post-concurrency same-key Count failed: %v", err)
	}
	if count != 1 {
		t.Errorf("After %d concurrent Upsert calls with same key, Count = %d; want 1 (no duplicates)", numKeys, count)
	}
}

func TestMemoryRepository_ConcurrentUpdates(t *testing.T) {
	// Test AC4: Concurrent updates with meaningful changes correctly update StoriesUpdated
	repo := NewMemoryRepository()
	testTime := time.Now().UTC()

	// Create initial story
	story := domain.Story{
		ID:          "id-1",
		Title:       "Original",
		Source:      "test-feed",
		PublishedAt: testTime,
		Summary:     "Original summary",
	}
	status, err := repo.Upsert(story)
	if err != nil {
		t.Fatalf("Initial Upsert failed: %v", err)
	}
	if status != domain.UpsertCreated {
		t.Fatalf("Expected Created, got %s", status)
	}

	// Concurrent upserts with different summaries (meaningful changes)
	// Only one should get Updated status due to race, others Unchanged
	var updatedCount int
	var unchangedCount int
	numGoroutines := 20
	var mu sync.Mutex
	wg := sync.WaitGroup{}

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			newStory := story
			newStory.Summary = fmt.Sprintf("Updated %d", i)
			status, err := repo.Upsert(newStory)
			if err != nil {
				t.Errorf("Concurrent update failed: %v", err)
				return
			}
			mu.Lock()
			switch status {
			case domain.UpsertUpdated:
				updatedCount++
			case domain.UpsertUnchanged:
				unchangedCount++
			}
			mu.Unlock()
		}()
	}

	// Wait for all goroutines
	wg.Wait()

	// Count should be 1 (only one record)
	count, err := repo.Count()
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Count = %d; want 1 (no duplicates)", count)
	}

	// At least one should have been Updated
	if updatedCount < 1 {
		t.Errorf("No UpsertUpdated status recorded; at least one should have occurred")
	}
}
