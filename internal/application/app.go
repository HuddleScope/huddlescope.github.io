package application

import (
	"context"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// IngestionResult represents the result of an ingestion run.
type IngestionResult struct {
	EntriesFetched int
	StoriesCreated int
	StoriesUpdated int
	StoriesSkipped int
	Errors         []error
}

// ExportResult represents the result of an export run.
type ExportResult struct {
	StoriesExported int
	Errors          []error
}

// Application holds the core business logic components.
type Application struct {
	repo     domain.Repository
	fetchers []domain.FeedFetcher
}

// Option configures the Application.
type Option func(*Application)

// WithRepository sets the storage repository.
func WithRepository(repo domain.Repository) Option {
	return func(a *Application) {
		a.repo = repo
	}
}

// WithFeedFetchers sets the feed fetchers.
func WithFeedFetchers(fetchers ...domain.FeedFetcher) Option {
	return func(a *Application) {
		a.fetchers = fetchers
	}
}

// New creates a new Application instance.
func New(opts ...Option) *Application {
	app := &Application{}
	for _, opt := range opts {
		opt(app)
	}
	return app
}

// RunIngest fetches feeds and stores stories.
func (a *Application) RunIngest(ctx context.Context) IngestionResult {
	result := IngestionResult{}

	// If no repository is configured, return "no work"
	if a.repo == nil {
		return IngestionResult{
			StoriesSkipped: 0,
			Errors:         nil,
		}
	}

	// If no fetchers are configured, return "no work"
	if len(a.fetchers) == 0 {
		return IngestionResult{
			StoriesSkipped: 0,
			Errors:         nil,
		}
	}

	for _, fetcher := range a.fetchers {
		entries, err := fetcher.Fetch("default")
		if err != nil {
			result.Errors = append(result.Errors, err)
			continue
		}
		result.EntriesFetched += len(entries)

		for _, entry := range entries {
			story := domain.Story{}.Canonicalize(entry)

			// Try to upsert
			err := a.repo.Upsert(story)
			if err != nil {
				result.Errors = append(result.Errors, err)
				continue
			}

			result.StoriesCreated += 1
			result.StoriesUpdated += 0
			result.StoriesSkipped += 0
		}
	}

	return result
}

// RunExport exports stories to the configured backend.
func (a *Application) RunExport(ctx context.Context) ExportResult {
	result := ExportResult{}

	// If no repository is configured, return "no work"
	if a.repo == nil {
		return ExportResult{
			StoriesExported: 0,
			Errors:          nil,
		}
	}

	stories, err := a.repo.List()
	if err != nil {
		result.Errors = append(result.Errors, err)
		return result
	}

	// In this placeholder, just count stories
	result.StoriesExported = len(stories)

	return result
}
