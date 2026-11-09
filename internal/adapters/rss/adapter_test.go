package rss

import (
	"testing"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

func TestStaticFetcher_Fetch(t *testing.T) {
	entries := []domain.FeedEntry{
		{Title: "Entry 1", Source: "test-feed"},
		{Title: "Entry 2", Source: "test-feed"},
	}

	fetcher := NewStaticFetcher(entries)

	result, err := fetcher.Fetch("default")
	if err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(result))
	}

	if result[0].Title != "Entry 1" || result[1].Title != "Entry 2" {
		t.Errorf("Entries not returned correctly: got %v", result)
	}

	// Test idempotency
	result2, err := fetcher.Fetch("default")
	if err != nil {
		t.Fatalf("Second Fetch() returned error: %v", err)
	}

	if len(result2) != 2 {
		t.Fatalf("Expected 2 entries on second call, got %d", len(result2))
	}
}
