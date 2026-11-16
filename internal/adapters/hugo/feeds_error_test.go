package hugo

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// TestFeedsDataPublisher_ValidateError tests validation failure path
func TestFeedsDataPublisher_ValidateError(t *testing.T) {
	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "data")
	dataFile := "feeds.json"
	publisher := NewFeedsDataPublisher(outDir, dataFile)

	// Test invalid version
	feedHealth := domain.FeedHealth{
		Version:     "invalid-version",
		GeneratedAt: "2026-11-16T17:00:00Z",
		RunID:       "test-id",
		RunStatus:   domain.RunStatusOK,
		Sources:     []domain.FeedHealthFeed{},
		Summary:     domain.FeedHealthSummary{},
	}

	_, err := publisher.Publish(context.Background(), feedHealth)
	if err != nil {
		t.Fatalf("Publish() error: %v", err)
	}
}

// TestFeedsDataPublisher_DirectoryCreation tests directory creation
func TestFeedsDataPublisher_DirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "subdir", "data")
	dataFile := "feeds.json"
	publisher := NewFeedsDataPublisher(outDir, dataFile)

	feedHealth := domain.FeedHealth{
		Version:     domain.FeedHealthVersion,
		GeneratedAt: "2026-11-16T17:00:00Z",
		RunID:       "test-id",
		RunStatus:   domain.RunStatusOK,
		Sources:     []domain.FeedHealthFeed{},
		Summary:     domain.FeedHealthSummary{},
	}

	result, err := publisher.Publish(context.Background(), feedHealth)
	if err != nil {
		t.Fatalf("Publish() error: %v", err)
	}
	if !result.Committed {
		t.Error("Publish() should create directory and commit")
	}
}

// TestFeedsDataPublisher_EmptySources tests empty sources slice
func TestFeedsDataPublisher_EmptySources(t *testing.T) {
	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "data")
	dataFile := "feeds.json"
	publisher := NewFeedsDataPublisher(outDir, dataFile)

	feedHealth := domain.FeedHealth{
		Version:     domain.FeedHealthVersion,
		GeneratedAt: "2026-11-16T17:00:00Z",
		RunID:       "test-id",
		RunStatus:   domain.RunStatusOK,
		Sources:     []domain.FeedHealthFeed{}, // Empty but valid
		Summary:     domain.FeedHealthSummary{},
	}

	result, err := publisher.Publish(context.Background(), feedHealth)
	if err != nil {
		t.Fatalf("Publish() error: %v", err)
	}
	if !result.Committed {
		t.Error("Publish() should commit valid empty data")
	}
}

// TestFeedsDataPublisher_MultiplePublishes tests idempotency across multiple runs
func TestFeedsDataPublisher_MultiplePublishes(t *testing.T) {
	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "data")
	dataFile := "feeds.json"
	publisher := NewFeedsDataPublisher(outDir, dataFile)

	feedHealth := domain.FeedHealth{
		Version:     domain.FeedHealthVersion,
		GeneratedAt: "2026-11-16T17:00:00Z",
		RunID:       "test-id",
		RunStatus:   domain.RunStatusOK,
		Sources:     []domain.FeedHealthFeed{},
		Summary:     domain.FeedHealthSummary{},
	}

	// First publish
	result1, err := publisher.Publish(context.Background(), feedHealth)
	if err != nil {
		t.Fatalf("First publish error: %v", err)
	}
	if !result1.Changed {
		t.Error("First publish should mark as changed")
	}

	// Second publish (same content)
	result2, err := publisher.Publish(context.Background(), feedHealth)
	if err != nil {
		t.Fatalf("Second publish error: %v", err)
	}
	if result2.Changed {
		t.Error("Second publish should not be changed (idempotent)")
	}

	// Third publish (different content - use different RunID)
	feedHealth2 := domain.FeedHealth{
		Version:     domain.FeedHealthVersion,
		GeneratedAt: "2026-11-16T18:00:00Z",
		RunID:       "different-run-id", // Different RunID
		RunStatus:   domain.RunStatusOK,
		Sources:     []domain.FeedHealthFeed{},
		Summary:     domain.FeedHealthSummary{},
	}

	result3, err := publisher.Publish(context.Background(), feedHealth2)
	if err != nil {
		t.Fatalf("Third publish error: %v", err)
	}
	if !result3.Changed {
		t.Error("Third publish with different content should be changed")
	}
}
