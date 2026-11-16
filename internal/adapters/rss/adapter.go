// Package rss provides adapters for fetching RSS feeds.
package rss

import (
	"context"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// Fetcher interface for fetching entries from RSS feeds.
type Fetcher interface {
	Fetch(ctx context.Context, sourceID string) ([]domain.FeedEntry, error)
}

// StaticFetcher returns fixed entries for testing.
type StaticFetcher struct {
	entries []domain.FeedEntry
}

func NewStaticFetcher(entries []domain.FeedEntry) *StaticFetcher {
	return &StaticFetcher{entries: entries}
}

func (f *StaticFetcher) Fetch(ctx context.Context, sourceID string) ([]domain.FeedEntry, error) {
	return f.entries, nil
}
