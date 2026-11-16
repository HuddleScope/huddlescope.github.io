package application

import (
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// BuildFeedHealth builds a FeedHealth snapshot from a RunRecord.
// It uses the feed registry to get full metadata (Name, Category) and the
// run record for per-source status and metrics.
func (a *Application) BuildFeedHealth(record domain.RunRecord, now time.Time) domain.FeedHealth {
	summary := domain.FeedHealthSummary{}
	feeds := make([]domain.FeedHealthFeed, 0, len(record.Sources))

	// Build a map of source metadata from the registry
	sourceMeta := make(map[string]domain.FeedSource)
	for _, src := range a.registry.All() {
		sourceMeta[src.ID] = src
	}

	// Build per-feed metrics
	for _, srcStatus := range record.Sources {
		// Get feed metadata (may not exist if registry was rebuilt)
		feed := domain.FeedHealthFeed{
			SourceID: srcStatus.SourceID,
			Category: sourceMeta[srcStatus.SourceID].Category,
		}

		// Determine status
		input := domain.DeriveFeedStatusInput{
			Enabled:        sourceMeta[srcStatus.SourceID].Enabled,
			CheckedAt:      srcStatus.FetchedAt,
			SucceededAt:    srcStatus.FetchedAt, // Use FetchedAt as success marker for now
			Errors:         srcStatus.Errors,
			EntriesTotal:   srcStatus.EntriesTotal,
			StoriesCreated: srcStatus.StoriesCreated,
			StoriesSkipped: srcStatus.StoriesSkipped,
		}
		feed.Status = domain.DeriveFeedStatus(input)

		// Update LastSucceeded based on status
		if feed.Status == domain.FeedStatusHealthy || feed.Status == domain.FeedStatusNoUpdates {
			feed.LastSucceeded = srcStatus.FetchedAt
		}

		// Populate other fields
		feed.Name = domain.SanitizeName(sourceMeta[srcStatus.SourceID].Name)
		feed.URL = domain.SanitizeURL(sourceMeta[srcStatus.SourceID].URL)
		feed.EntriesTotal = srcStatus.EntriesTotal
		feed.New = srcStatus.StoriesCreated
		feed.Duplicates = srcStatus.StoriesUpdated + srcStatus.StoriesSkipped
		feed.HTTPStatus = srcStatus.HTTPStatus
		if len(srcStatus.Errors) > 0 {
			feed.Error = domain.SanitizeFeedError(srcStatus.Errors[len(srcStatus.Errors)-1])
		}
		feed.Stale = domain.IsStale(sourceMeta[srcStatus.SourceID].Enabled, feed.LastSucceeded, now)

		feeds = append(feeds, feed)

		// Update summary
		summary.Configured++
		if sourceMeta[srcStatus.SourceID].Enabled {
			summary.Enabled++
		} else {
			summary.Disabled++
		}
		summary.Processed++
		switch feed.Status {
		case domain.FeedStatusFailed:
			summary.Failed++
		case domain.FeedStatusNoUpdates:
			summary.NoUpdates++
		case domain.FeedStatusHealthy:
			summary.Healthy++
		case domain.FeedStatusNotChecked:
			summary.NotChecked++
		}
		if feed.Stale {
			summary.Stale++
		}
		summary.Entries += srcStatus.EntriesFetched
		summary.New += srcStatus.StoriesCreated
		summary.Duplicates += srcStatus.StoriesUpdated + srcStatus.StoriesSkipped
	}

	return domain.FeedHealth{
		Version:     domain.FeedHealthVersion,
		GeneratedAt: now.UTC().Format(time.RFC3339),
		RunID:       record.RunID.String(),
		RunStatus:   record.Status,
		Duration:    0, // Will be set by caller
		Sources:     feeds,
		Summary:     summary,
	}
}
