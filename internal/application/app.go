package application

import (
	"context"
	"fmt"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/rss"
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

// ScoringResult represents the result of a scoring run (HS-012).
type ScoringResult struct {
	TotalGroups int
	Breakdowns  []domain.ScoreBreakdown
	Errors      []error
}

// GroupingResult represents the result of a grouping operation.
type GroupingResult struct {
	GroupsCreated   int
	GroupsMerged    int
	ArticlesGrouped int
	ArticlesSolo    int
	Errors          []error
}

// NewGroupingResult creates a new GroupingResult.
func NewGroupingResult() GroupingResult {
	return GroupingResult{
		GroupsCreated:   0,
		GroupsMerged:    0,
		ArticlesGrouped: 0,
		ArticlesSolo:    0,
		Errors:          nil,
	}
}

// Application holds the core business logic components.
type Application struct {
	repo     domain.Repository
	fetchers []domain.FeedFetcher
	// New for HS-006: registry-driven fetcher resolution
	registry   *domain.FeedRegistry
	fetcherMap map[string]domain.FeedFetcher // ID -> fetcher
	// New for HS-010: clustering engine
	clusteringEngine *domain.Clusterer
	// New for HS-011: team registry for entity extraction
	teamRegistry *domain.TeamRegistry
	// New for HS-012: scoring engine
	scorer *domain.Scorer
}

// Option configures the Application.
type Option func(*Application)

// WithRepository sets the storage repository.
func WithRepository(repo domain.Repository) Option {
	return func(a *Application) {
		a.repo = repo
	}
}

// WithFeedFetchers sets the feed fetchers (legacy; prefer WithRegistry).
func WithFeedFetchers(fetchers ...domain.FeedFetcher) Option {
	return func(a *Application) {
		a.fetchers = fetchers
	}
}

// WithRegistry sets the feed registry (HS-006).
func WithRegistry(registry *domain.FeedRegistry) Option {
	return func(a *Application) {
		a.registry = registry
	}
}

// WithFetcherResolver sets a function to resolve a FeedFetcher for a source ID.
func WithFetcherResolver(resolver func(sourceID string) domain.FeedFetcher) Option {
	return func(a *Application) {
	}
}

// WithFetcherMap sets a map of sourceID -> FeedFetcher (HS-006).
func WithFetcherMap(fetcherMap map[string]domain.FeedFetcher) Option {
	return func(a *Application) {
		a.fetcherMap = fetcherMap
	}
}

// WithClusterer sets the clustering engine (HS-010).
func WithClusterer(engine *domain.Clusterer) Option {
	return func(a *Application) {
		a.clusteringEngine = engine
	}
}

// WithScorer sets the scoring engine (HS-012).
func WithScorer(scorer *domain.Scorer) Option {
	return func(a *Application) {
		a.scorer = scorer
	}
}

// extractTeams extracts teams from story text and merges with existing entities.
func extractTeams(story domain.Story, registry *domain.TeamRegistry) domain.Story {
	if registry == nil {
		return story
	}

	text := story.Title + " " + story.Summary
	teamSlugs := registry.Extract(text)

	// Merge entities, avoiding duplicates
	existing := make(map[string]bool)
	for _, e := range story.Entities {
		existing[e] = true
	}
	for _, slug := range teamSlugs {
		if !existing[slug] {
			story.Entities = append(story.Entities, slug)
		}
	}

	return story
}

// WithTeamRegistry sets the team registry for entity extraction (HS-011).
func WithTeamRegistry(registry *domain.TeamRegistry) Option {
	return func(a *Application) {
		a.teamRegistry = registry
	}
}

// New creates a new Application instance.
func New(opts ...Option) *Application {
	app := &Application{
		fetcherMap: make(map[string]domain.FeedFetcher),
	}
	for _, opt := range opts {
		opt(app)
	}
	return app
}

// RunIngest fetches feeds and stores stories.
// For HS-006: registry-driven ingestion with per-source failure isolation.
func (a *Application) RunIngest(ctx context.Context) domain.IngestionResult {
	result := domain.IngestionResult{}

	if a.repo == nil {
		return domain.IngestionResult{
			StoriesSkipped: 0,
			Errors:         nil,
		}
	}

	if a.registry != nil {
		return a.runIngestWithRegistry(ctx, result)
	}

	if len(a.fetchers) == 0 {
		return domain.IngestionResult{
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

			// If a custom team registry is configured, re-extract teams
			// Extract teams if custom registry is configured
			story = extractTeams(story, a.teamRegistry)

			status, err := a.repo.Upsert(story)
			if err != nil {
				result.Errors = append(result.Errors, err)
				continue
			}

			switch status {
			case domain.UpsertCreated:
				result.StoriesCreated += 1
			case domain.UpsertUpdated:
				result.StoriesUpdated += 1
			case domain.UpsertUnchanged:
				result.StoriesSkipped += 1
			}
		}
	}

	return result
}

// runIngestWithRegistry implements HS-006 registry-driven ingestion.
func (a *Application) runIngestWithRegistry(ctx context.Context, result domain.IngestionResult) domain.IngestionResult {
	enabledSources := a.registry.Enabled()

	if len(enabledSources) == 0 {
		return domain.IngestionResult{
			EntriesFetched: 0,
			StoriesCreated: 0,
			StoriesUpdated: 0,
			StoriesSkipped: 0,
			Errors:         nil,
		}
	}

	for _, src := range enabledSources {
		fetcher, err := a.resolveFetcher(src.ID)
		if err != nil {
			a.registry.RecordStatus(src.ID, false, err)
			result.Errors = append(result.Errors, fmt.Errorf("failed to resolve fetcher for %s: %w", src.ID, err))
			continue
		}

		entries, err := fetcher.Fetch(src.ID)
		a.registry.RecordStatus(src.ID, err == nil, err)

		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("source %s: %w", src.ID, err))
			continue
		}

		result.EntriesFetched += len(entries)

		for _, entry := range entries {
			story := domain.Story{}.Canonicalize(entry)

			// If a custom team registry is configured, re-extract teams
			if a.teamRegistry != nil {
				text := story.Title + " " + story.Summary
				teamSlugs := a.teamRegistry.Extract(text)

				// Merge entities, avoiding duplicates
				existing := make(map[string]bool)
				for _, e := range story.Entities {
					existing[e] = true
				}
				for _, slug := range teamSlugs {
					if !existing[slug] {
						story.Entities = append(story.Entities, slug)
					}
				}
			}

			status, err := a.repo.Upsert(story)
			if err != nil {
				result.Errors = append(result.Errors, err)
				continue
			}

			switch status {
			case domain.UpsertCreated:
				result.StoriesCreated += 1
			case domain.UpsertUpdated:
				result.StoriesUpdated += 1
			case domain.UpsertUnchanged:
				result.StoriesSkipped += 1
			}
		}
	}

	return result
}

// RunGrouping performs cross-source story grouping (HS-010).
func (a *Application) RunGrouping(ctx context.Context) domain.GroupingResult {
	result := domain.NewGroupingResult()

	if a.clusteringEngine == nil {
		result.Errors = append(result.Errors, fmt.Errorf("clustering engine not configured"))
		return result
	}

	if a.repo == nil {
		result.Errors = append(result.Errors, fmt.Errorf("repository not configured"))
		return result
	}

	stories, err := a.repo.List()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("failed to list stories: %w", err))
		return result
	}

	articles := make([]domain.Article, 0, len(stories))
	for _, story := range stories {
		articles = append(articles, domain.Article{
			ID:          story.ID,
			SourceID:    story.Source,
			Title:       story.Title,
			Summary:     story.Summary,
			URL:         story.URL,
			PublishedAt: story.PublishedAt.UTC().Format("2006-01-02T15:04:05Z"),
			FirstSeenAt: story.FirstSeenAt.UTC().Format("2006-01-02T15:04:05Z"),
			Entities:    story.Entities,
		})
	}

	groups := a.clusteringEngine.Cluster(articles)

	// Persist grouping results back to database
	newStories := make([]domain.Story, 0, len(groups))
	for _, g := range groups {
		newStory := domain.NewStoryFromGroup(g)
		newStories = append(newStories, newStory)
	}

	// Upsert all grouped stories
	for _, story := range newStories {
		if _, err := a.repo.Upsert(story); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to upsert story %s: %w", story.ID, err))
		}
	}

	groupCount := 0
	for _, g := range groups {
		if g.Size() == 1 {
			result.ArticlesSolo += 1
		} else {
			result.ArticlesGrouped += g.Size()
		}
		groupCount++
	}
	result.GroupsCreated = groupCount

	return result
}

// resolveFetcher returns a FeedFetcher for the given source ID.
func (a *Application) resolveFetcher(sourceID string) (domain.FeedFetcher, error) {
	if fetcher, ok := a.fetcherMap[sourceID]; ok {
		return fetcher, nil
	}
	return rss.NewStaticFetcher([]domain.FeedEntry{}), nil
}

// RunExport exports stories to the configured backend.
func (a *Application) RunExport(ctx context.Context) ExportResult {
	result := ExportResult{}

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

	result.StoriesExported = len(stories)

	return result
}

// RunScoring performs scoring and ranking of stories (HS-012).
func (a *Application) RunScoring(ctx context.Context, now time.Time) ScoringResult {
	result := ScoringResult{}

	if a.scorer == nil {
		result.Errors = append(result.Errors, fmt.Errorf("scorer not configured"))
		return result
	}

	if a.repo == nil {
		result.Errors = append(result.Errors, fmt.Errorf("repository not configured"))
		return result
	}

	if a.clusteringEngine == nil {
		result.Errors = append(result.Errors, fmt.Errorf("clustering engine not configured; RunGrouping must be called first"))
		return result
	}

	// Re-run grouping to ensure we have fresh story groups
	groupingResult := a.RunGrouping(ctx)
	if len(groupingResult.Errors) > 0 {
		result.Errors = append(result.Errors, groupingResult.Errors...)
	}

	stories, err := a.repo.List()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("failed to list stories: %w", err))
		return result
	}

	// Rebuild story groups and score them
	articles := make([]domain.Article, 0, len(stories))
	for _, story := range stories {
		articles = append(articles, domain.Article{
			ID:          story.ID,
			SourceID:    story.Source,
			Title:       story.Title,
			Summary:     story.Summary,
			URL:         story.URL,
			PublishedAt: story.PublishedAt.UTC().Format("2006-01-02T15:04:05Z"),
			FirstSeenAt: story.FirstSeenAt.UTC().Format("2006-01-02T15:04:05Z"),
			Entities:    story.Entities,
		})
	}

	groups := a.clusteringEngine.Cluster(articles)

	// Score and rank groups
	for _, g := range groups {
		breakdown := a.scorer.WithTime(now).Score(g)
		result.Breakdowns = append(result.Breakdowns, breakdown)
	}

	result.TotalGroups = len(result.Breakdowns)

	return result
}
