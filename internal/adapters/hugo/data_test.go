package hugo

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

func TestDataPublisher_SuccessPath(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")
	dataFile := "homepage.json"

	// Ensure directory exists
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("Failed to create data dir: %v", err)
	}

	p := NewDataPublisher(dataDir, dataFile)

	// Build a valid snapshot
	scorer := domain.NewScorer()
	resolver := domain.NewStaticSourceResolver(map[string]string{"test": "Test"})
	registry := domain.NewTeamRegistry()

	articles := []domain.Article{
		{ID: "a1", SourceID: "test", Title: "Story A", Summary: "Summary A", URL: "http://example.com/a", PublishedAt: "2024-01-01T10:00:00Z"},
	}
	groups := []domain.StoryGroup{
		{ID: "g1", Headline: "Story A", Articles: articles, FirstPublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC), LastPublishedAt: time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC)},
	}
	stories := []domain.Story{
		{ID: "g1", Title: "Story A", Source: "test", Summary: "Summary A", URL: "http://example.com/a", PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)},
	}

	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	snap, err := domain.BuildSnapshot(stories, groups, scorer, now, resolver, registry, 10)
	if err != nil {
		t.Fatalf("BuildSnapshot failed: %v", err)
	}

	// Publish
	result, err := p.Publish(context.Background(), snap)
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Check result
	if !result.Committed {
		t.Error("Committed should be true")
	}
	if !result.Changed {
		t.Error("Changed should be true (first write)")
	}
	if result.Version != domain.SnapshotVersion {
		t.Errorf("Version = %q; want %q", result.Version, domain.SnapshotVersion)
	}
	if result.Path == "" {
		t.Error("Path should not be empty after successful write")
	}

	// Verify file was created
	dataPath := filepath.Join(dataDir, dataFile)
	if _, err := os.Stat(dataPath); os.IsNotExist(err) {
		t.Fatalf("Data file not created at %s", dataPath)
	}
}

func TestDataPublisher_Idempotent(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")
	dataFile := "homepage.json"

	// Ensure directory exists
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("Failed to create data dir: %v", err)
	}

	p := NewDataPublisher(dataDir, dataFile)

	// Build a valid snapshot
	scorer := domain.NewScorer()
	resolver := domain.NewStaticSourceResolver(map[string]string{"test": "Test"})
	registry := domain.NewTeamRegistry()

	articles := []domain.Article{
		{ID: "a1", SourceID: "test", Title: "Story A", Summary: "Summary A", URL: "http://example.com/a", PublishedAt: "2024-01-01T10:00:00Z"},
	}
	groups := []domain.StoryGroup{
		{ID: "g1", Headline: "Story A", Articles: articles, FirstPublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC), LastPublishedAt: time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC)},
	}
	stories := []domain.Story{
		{ID: "g1", Title: "Story A", Source: "test", Summary: "Summary A", URL: "http://example.com/a", PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)},
	}

	// First publish
	now1 := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	snap1, err := domain.BuildSnapshot(stories, groups, scorer, now1, resolver, registry, 10)
	if err != nil {
		t.Fatalf("First BuildSnapshot failed: %v", err)
	}

	result1, err := p.Publish(context.Background(), snap1)
	if err != nil {
		t.Fatalf("First publish failed: %v", err)
	}
	if !result1.Changed {
		t.Error("First publish should show Changed=true")
	}

	// Second publish with same content (different generatedAt)
	now2 := time.Date(2024, 1, 16, 12, 0, 0, 0, time.UTC)
	snap2, err := domain.BuildSnapshot(stories, groups, scorer, now2, resolver, registry, 10)
	if err != nil {
		t.Fatalf("Second BuildSnapshot failed: %v", err)
	}

	// Debug: check content equality
	if snap1.ContentEqual(snap2) {
		t.Log("Content is equal")
	} else {
		t.Logf("Content differs: snap1.Stories=%v snap2.Stories=%v", snap1.Stories, snap2.Stories)
	}

	result2, err := p.Publish(context.Background(), snap2)
	if err != nil {
		t.Fatalf("Second publish failed: %v", err)
	}

	// Content should be equal, so Changed should be false
	if !result2.Committed {
		t.Error("Second publish should be committed")
	}
	if result2.Changed {
		t.Error("Second publish with identical content should show Changed=false (idempotent)")
	}
}

func TestDataPublisher_ContentChange(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")
	dataFile := "homepage.json"

	// Ensure directory exists
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("Failed to create data dir: %v", err)
	}

	p := NewDataPublisher(dataDir, dataFile)

	scorer := domain.NewScorer()
	resolver := domain.NewStaticSourceResolver(map[string]string{"test": "Test"})
	registry := domain.NewTeamRegistry()

	// First snapshot
	articles1 := []domain.Article{
		{ID: "a1", SourceID: "test", Title: "Story A", Summary: "Summary A", URL: "http://example.com/a", PublishedAt: "2024-01-01T10:00:00Z"},
	}
	groups1 := []domain.StoryGroup{
		{ID: "g1", Headline: "Story A", Articles: articles1, FirstPublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC), LastPublishedAt: time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC)},
	}
	stories1 := []domain.Story{
		{ID: "g1", Title: "Story A", Source: "test", Summary: "Summary A", URL: "http://example.com/a", PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)},
	}

	now1 := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	snap1, err := domain.BuildSnapshot(stories1, groups1, scorer, now1, resolver, registry, 10)
	if err != nil {
		t.Fatalf("First BuildSnapshot failed: %v", err)
	}

	result1, err := p.Publish(context.Background(), snap1)
	if err != nil {
		t.Fatalf("First publish failed: %v", err)
	}
	if !result1.Changed {
		t.Error("First publish should show Changed=true")
	}

	// Second snapshot with different content
	articles2 := []domain.Article{
		{ID: "a1", SourceID: "test", Title: "Story B", Summary: "Summary B", URL: "http://example.com/b", PublishedAt: "2024-01-01T10:00:00Z"},
	}
	groups2 := []domain.StoryGroup{
		{ID: "g1", Headline: "Story B", Articles: articles2, FirstPublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC), LastPublishedAt: time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC)},
	}
	stories2 := []domain.Story{
		{ID: "g1", Title: "Story B", Source: "test", Summary: "Summary B", URL: "http://example.com/b", PublishedAt: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)},
	}

	now2 := time.Date(2024, 1, 16, 12, 0, 0, 0, time.UTC)
	snap2, err := domain.BuildSnapshot(stories2, groups2, scorer, now2, resolver, registry, 10)
	if err != nil {
		t.Fatalf("Second BuildSnapshot failed: %v", err)
	}

	result2, err := p.Publish(context.Background(), snap2)
	if err != nil {
		t.Fatalf("Second publish failed: %v", err)
	}

	// Content differs, so Changed should be true
	if !result2.Changed {
		t.Error("Second publish with different content should show Changed=true")
	}
}

func TestDataPublisher_Noop(t *testing.T) {
	p := &NoopDataPublisher{}
	result, err := p.Publish(context.Background(), domain.Snapshot{})
	if err != nil {
		t.Fatalf("Noop Publish should not return error: %v", err)
	}
	if !result.Committed {
		t.Error("Noop should be committed")
	}
	if result.Changed {
		t.Error("Noop should not be changed")
	}
}
