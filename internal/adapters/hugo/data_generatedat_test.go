package hugo

// HS-056 (Q3): regression tests for generatedAt honesty in the snapshot
// writers. The writers must never backdate or forge timestamps:
//   - a content change (e.g. N stories -> 0) rewrites the file with the new
//     snapshot's genuine generation timestamp;
//   - a content-equal publish leaves the existing file untouched, preserving
//     its original generatedAt.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

const (
	oldTimestamp = "2026-01-01T00:00:00Z"
	newTimestamp = "2026-01-02T00:00:00Z"
)

func oneStorySnapshot() domain.Snapshot {
	return domain.Snapshot{
		Version:     domain.SnapshotVersion,
		GeneratedAt: oldTimestamp,
		Stories: []domain.SnapshotStory{
			{
				ID:               "real-story-1",
				Title:            "A real story",
				Summary:          "Short original summary.",
				FirstPublishedAt: "2024-01-01T00:00:00Z",
				LastPublishedAt:  "2024-01-02T00:00:00Z",
				Score:            80.0,
				EvidenceLevel:    domain.EvidenceConfirmed,
				Publishers: []domain.PublisherRef{
					{ID: "nfl-com", Name: "NFL.com", URL: "http://example.com/story"},
				},
			},
		},
		Latest: []domain.SnapshotArticle{},
		Teams:  []domain.SnapshotTeam{},
	}
}

func zeroStorySnapshot() domain.Snapshot {
	return domain.Snapshot{
		Version:     domain.SnapshotVersion,
		GeneratedAt: newTimestamp,
		Stories:     []domain.SnapshotStory{},
		Latest:      []domain.SnapshotArticle{},
		Teams:       []domain.SnapshotTeam{},
	}
}

func readFileJSON(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return data
}

func TestDataPublisher_ZeroStoryPublishRewritesWithFreshTimestamp(t *testing.T) {
	dataDir := t.TempDir()
	dataFile := "homepage.json"
	dataPath := filepath.Join(dataDir, dataFile)
	p := NewDataPublisher(dataDir, dataFile)

	// Seed the file with a 1-story snapshot carrying the old timestamp.
	res, err := p.Publish(context.Background(), oneStorySnapshot())
	if err != nil {
		t.Fatalf("seed publish failed: %v", err)
	}
	if !res.Changed {
		t.Fatal("seed publish should have written the file (Changed=true)")
	}
	var seeded domain.Snapshot
	if err := json.Unmarshal(readFileJSON(t, dataPath), &seeded); err != nil {
		t.Fatalf("seeded file does not parse: %v", err)
	}
	if seeded.GeneratedAt != oldTimestamp || len(seeded.Stories) != 1 {
		t.Fatalf("seeded file = generatedAt %q, %d stories; want %q, 1", seeded.GeneratedAt, len(seeded.Stories), oldTimestamp)
	}

	// Publish a zero-story snapshot with a new (real) generation timestamp.
	res, err = p.Publish(context.Background(), zeroStorySnapshot())
	if err != nil {
		t.Fatalf("zero-story publish failed: %v", err)
	}
	if !res.Committed {
		t.Error("zero-story publish should commit (Committed=true)")
	}
	if !res.Changed {
		t.Error("content changed (1 story -> 0); Changed should be true")
	}
	if res.PublishedAt != newTimestamp {
		t.Errorf("PublishedAt = %q; want the fresh snapshot timestamp %q", res.PublishedAt, newTimestamp)
	}

	var updated domain.Snapshot
	if err := json.Unmarshal(readFileJSON(t, dataPath), &updated); err != nil {
		t.Fatalf("updated file does not parse: %v", err)
	}
	if len(updated.Stories) != 0 {
		t.Errorf("updated file has %d stories; want 0", len(updated.Stories))
	}
	if updated.GeneratedAt != newTimestamp {
		t.Errorf("updated file generatedAt = %q; want the fresh timestamp %q (no stale/forged timestamp)", updated.GeneratedAt, newTimestamp)
	}
}

func TestDataPublisher_ContentEqualPreservesExistingTimestamp(t *testing.T) {
	dataDir := t.TempDir()
	dataFile := "homepage.json"
	dataPath := filepath.Join(dataDir, dataFile)
	p := NewDataPublisher(dataDir, dataFile)

	// First publish with the old timestamp.
	res, err := p.Publish(context.Background(), oneStorySnapshot())
	if err != nil {
		t.Fatalf("first publish failed: %v", err)
	}
	if !res.Changed {
		t.Fatal("first publish should write the file (Changed=true)")
	}

	// Re-publish byte-identical content (excluding generatedAt) with a new
	// wall-clock timestamp: nothing new happened, so the file must not be
	// rewritten and must keep the original generatedAt.
	same := oneStorySnapshot()
	same.GeneratedAt = newTimestamp
	before := readFileJSON(t, dataPath)
	res, err = p.Publish(context.Background(), same)
	if err != nil {
		t.Fatalf("second publish failed: %v", err)
	}
	if !res.Committed {
		t.Error("content-equal publish should commit (Committed=true)")
	}
	if res.Changed {
		t.Error("content equal; Changed should be false (no rewrite)")
	}
	if res.PublishedAt != oldTimestamp {
		t.Errorf("PublishedAt = %q; want the preserved original %q", res.PublishedAt, oldTimestamp)
	}
	after := readFileJSON(t, dataPath)
	if string(before) != string(after) {
		t.Error("content-equal publish rewrote the file; it must be preserved byte-for-byte")
	}
}

func oneFeedHealth() domain.FeedHealth {
	when := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return domain.FeedHealth{
		Version:     domain.FeedHealthVersion,
		GeneratedAt: oldTimestamp,
		RunID:       "run-1",
		RunStatus:   domain.RunStatusOK,
		Duration:    5 * time.Second,
		Sources: []domain.FeedHealthFeed{
			{
				SourceID:     "espn-nfl",
				Name:         "ESPN NFL",
				URL:          "http://example.com/rss",
				Status:       domain.FeedStatusNoUpdates,
				LastChecked:  &when,
				EntriesTotal: 10,
				HTTPStatus:   200,
			},
		},
		Summary: domain.FeedHealthSummary{Configured: 1, Enabled: 1, Processed: 1, NoUpdates: 1, Entries: 10},
	}
}

func zeroFeedHealth() domain.FeedHealth {
	fh := oneFeedHealth()
	fh.GeneratedAt = newTimestamp
	fh.Sources = []domain.FeedHealthFeed{}
	fh.Summary = domain.FeedHealthSummary{}
	return fh
}

func TestFeedsDataPublisher_ZeroFeedPublishRewritesWithFreshTimestamp(t *testing.T) {
	dataDir := t.TempDir()
	dataFile := "feeds.json"
	dataPath := filepath.Join(dataDir, dataFile)
	p := NewFeedsDataPublisher(dataDir, dataFile)

	res, err := p.Publish(context.Background(), oneFeedHealth())
	if err != nil {
		t.Fatalf("seed publish failed: %v", err)
	}
	if !res.Changed {
		t.Fatal("seed publish should have written the file (Changed=true)")
	}
	var seeded domain.FeedHealth
	if err := json.Unmarshal(readFileJSON(t, dataPath), &seeded); err != nil {
		t.Fatalf("seeded file does not parse: %v", err)
	}
	if seeded.GeneratedAt != oldTimestamp || len(seeded.Sources) != 1 {
		t.Fatalf("seeded file = generatedAt %q, %d feeds; want %q, 1", seeded.GeneratedAt, len(seeded.Sources), oldTimestamp)
	}

	res, err = p.Publish(context.Background(), zeroFeedHealth())
	if err != nil {
		t.Fatalf("zero-feed publish failed: %v", err)
	}
	if !res.Committed {
		t.Error("zero-feed publish should commit (Committed=true)")
	}
	if !res.Changed {
		t.Error("content changed (1 feed -> 0); Changed should be true")
	}
	if res.PublishedAt != newTimestamp {
		t.Errorf("PublishedAt = %q; want the fresh snapshot timestamp %q", res.PublishedAt, newTimestamp)
	}

	var updated domain.FeedHealth
	if err := json.Unmarshal(readFileJSON(t, dataPath), &updated); err != nil {
		t.Fatalf("updated file does not parse: %v", err)
	}
	if len(updated.Sources) != 0 {
		t.Errorf("updated file has %d feeds; want 0", len(updated.Sources))
	}
	if updated.GeneratedAt != newTimestamp {
		t.Errorf("updated file generatedAt = %q; want the fresh timestamp %q (no stale/forged timestamp)", updated.GeneratedAt, newTimestamp)
	}
}

func TestFeedsDataPublisher_ContentEqualPreservesExistingTimestamp(t *testing.T) {
	dataDir := t.TempDir()
	dataFile := "feeds.json"
	dataPath := filepath.Join(dataDir, dataFile)
	p := NewFeedsDataPublisher(dataDir, dataFile)

	res, err := p.Publish(context.Background(), oneFeedHealth())
	if err != nil {
		t.Fatalf("first publish failed: %v", err)
	}
	if !res.Changed {
		t.Fatal("first publish should write the file (Changed=true)")
	}

	same := oneFeedHealth()
	same.GeneratedAt = newTimestamp
	before := readFileJSON(t, dataPath)
	res, err = p.Publish(context.Background(), same)
	if err != nil {
		t.Fatalf("second publish failed: %v", err)
	}
	if !res.Committed {
		t.Error("content-equal publish should commit (Committed=true)")
	}
	if res.Changed {
		t.Error("content equal; Changed should be false (no rewrite)")
	}
	if res.PublishedAt != oldTimestamp {
		t.Errorf("PublishedAt = %q; want the preserved original %q", res.PublishedAt, oldTimestamp)
	}
	after := readFileJSON(t, dataPath)
	if string(before) != string(after) {
		t.Error("content-equal publish rewrote the file; it must be preserved byte-for-byte")
	}
}
