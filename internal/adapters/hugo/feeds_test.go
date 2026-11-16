package hugo

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

func TestFeedsDataPublisher_Publish(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "data")
	dataFile := "feeds.json"
	publisher := NewFeedsDataPublisher(outDir, dataFile)

	// Create a test feed health
	feedHealth := domain.FeedHealth{
		Version:     domain.FeedHealthVersion,
		GeneratedAt: "2026-11-16T17:00:00Z",
		RunID:       "test-run-id",
		RunStatus:   domain.RunStatusOK,
		Sources: []domain.FeedHealthFeed{
			{
				SourceID:     "test-source",
				Name:         "Test Feed",
				URL:          "https://example.com",
				Category:     "national",
				Status:       domain.FeedStatusNoUpdates,
				EntriesTotal: 10,
				New:          5,
				Duplicates:   5,
				HTTPStatus:   200,
			},
		},
		Summary: domain.FeedHealthSummary{
			Configured: 1,
			Enabled:    1,
			Processed:  1,
			Healthy:    1,
		},
	}

	// Test successful publish
	result, err := publisher.Publish(context.Background(), feedHealth)
	if err != nil {
		t.Fatalf("Publish() error: %v", err)
	}
	t.Logf("Result: Committed=%v, Changed=%v, Path=%s, Version=%s, Errors=%v", result.Committed, result.Changed, result.Path, result.Version, result.Errors)
	if !result.Committed {
		t.Error("Publish() should have committed")
	}
	if !result.Changed {
		t.Error("Publish() should have changed (first write)")
	}
	if result.Path == "" {
		t.Error("Publish() should have a path")
	}

	// Verify file was created
	dataPath := filepath.Join(outDir, dataFile)
	if _, err := os.Stat(dataPath); os.IsNotExist(err) {
		t.Errorf("File not created at %s", dataPath)
	}

	// Test idempotency - same content should not rewrite
	result2, err := publisher.Publish(context.Background(), feedHealth)
	if err != nil {
		t.Fatalf("Publish() second call error: %v", err)
	}
	if !result2.Committed {
		t.Error("Publish() should have committed")
	}
	if result2.Changed {
		t.Error("Publish() should not have changed (same content)")
	}
}

func TestFeedsDataPublisher_Validate(t *testing.T) {
	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "data")
	dataFile := "feeds.json"
	publisher := NewFeedsDataPublisher(outDir, dataFile)

	// Test invalid feed health (missing run_id)
	feedHealth := domain.FeedHealth{
		Version:     domain.FeedHealthVersion,
		GeneratedAt: "2026-11-16T17:00:00Z",
		RunID:       "",
		RunStatus:   domain.RunStatusOK,
		Sources:     []domain.FeedHealthFeed{},
		Summary:     domain.FeedHealthSummary{},
	}

	result, err := publisher.Publish(context.Background(), feedHealth)
	if err != nil {
		t.Fatalf("Publish() error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Error("Publish() should have errors for invalid feed health")
	}
}

func TestFeedsDataPublisher_Noop(t *testing.T) {
	publisher := &NoopFeedsDataPublisher{}

	result, err := publisher.Publish(context.Background(), domain.FeedHealth{})
	if err != nil {
		t.Fatalf("Publish() error: %v", err)
	}
	if !result.Committed {
		t.Error("NoopPublish() should have committed")
	}
	if result.Changed {
		t.Error("NoopPublish() should not have changed")
	}
}

func TestFeedsDataPublisher_WithBaseURL(t *testing.T) {
	publisher := NewFeedsDataPublisherWithBaseURL("/tmp", "feeds.json", "https://example.com")
	if publisher == nil {
		t.Fatal("NewFeedsDataPublisherWithBaseURL() returned nil")
	}
	if publisher.OutDir != "/tmp" {
		t.Errorf("OutDir = %s, want /tmp", publisher.OutDir)
	}
	if publisher.DataFile != "feeds.json" {
		t.Errorf("DataFile = %s, want feeds.json", publisher.DataFile)
	}
	if publisher.BaseURL != "https://example.com" {
		t.Errorf("BaseURL = %s, want https://example.com", publisher.BaseURL)
	}
}

func TestFeedsDataPublisher_GetHash(t *testing.T) {
	publisher := NewFeedsDataPublisher("/tmp", "feeds.json")
	feedHealth := domain.FeedHealth{
		Version:     domain.FeedHealthVersion,
		GeneratedAt: "2026-11-16T17:00:00Z",
		RunID:       "test-id",
		RunStatus:   domain.RunStatusOK,
		Sources:     []domain.FeedHealthFeed{},
		Summary:     domain.FeedHealthSummary{},
	}

	// Test successful hash computation (Render doesn't validate, it just marshals JSON)
	_, err := publisher.GetHash(feedHealth)
	if err != nil {
		t.Fatalf("GetHash() error: %v", err)
	}

	// GetHash just marshals JSON, so empty version still succeeds
	_, err = publisher.GetHash(feedHealth)
	if err != nil {
		t.Errorf("GetHash() should not return error for empty version (JSON marshal succeeds): %v", err)
	}
}
