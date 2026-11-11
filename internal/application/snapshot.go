package application

import (
	"context"
	"fmt"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// SnapshotResult captures the outcome of a snapshot run.
type SnapshotResult struct {
	Snapshot     domain.Snapshot
	PubResult    domain.PublicationResult
	StoriesBuilt int
	Errors       []error
}

// RunSnapshot builds a versioned snapshot and publishes it.
func (a *Application) RunSnapshot(ctx context.Context, now time.Time) SnapshotResult {
	result := SnapshotResult{}

	if a.repo == nil {
		result.Errors = append(result.Errors, fmt.Errorf("repository not configured"))
		return result
	}

	if a.clusteringEngine == nil {
		result.Errors = append(result.Errors, fmt.Errorf("clustering engine not configured"))
		return result
	}

	if a.scorer == nil {
		result.Errors = append(result.Errors, fmt.Errorf("scorer not configured"))
		return result
	}

	// Build the snapshot
	snapshot, err := a.buildSnapshot(now)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("build snapshot failed: %w", err))
		result.PubResult = domain.PublicationResult{
			Committed:   false,
			Changed:     false,
			Path:        "",
			Version:     domain.SnapshotVersion,
			PublishedAt: now.UTC().Format(time.RFC3339),
			Errors:      []error{err},
		}
		return result
	}
	result.Snapshot = snapshot
	result.StoriesBuilt = len(snapshot.Stories)

	// Publish using the default publisher (no-op for offline/dev)
	publisher := &domain.DefaultSnapshotPublisher{}

	// Publish
	pubResult, err := publisher.Publish(ctx, snapshot)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("publish failed: %w", err))
		pubResult = domain.PublicationResult{
			Committed:   false,
			Changed:     false,
			Path:        "",
			Version:     domain.SnapshotVersion,
			PublishedAt: now.UTC().Format(time.RFC3339),
			Errors:      []error{err},
		}
	}
	result.PubResult = pubResult

	return result
}

// buildSnapshot constructs the domain.Snapshot from stories and scoring.
func (a *Application) buildSnapshot(now time.Time) (domain.Snapshot, error) {
	// List stories
	stories, err := a.repo.List()
	if err != nil {
		return domain.Snapshot{}, fmt.Errorf("list stories: %w", err)
	}

	if len(stories) == 0 {
		// Empty snapshot
		return domain.Snapshot{
			Version:     domain.SnapshotVersion,
			GeneratedAt: now.UTC().Format(time.RFC3339),
			Stories:     []domain.SnapshotStory{},
			Latest:      []domain.SnapshotArticle{},
			Teams:       []domain.SnapshotTeam{},
		}, nil
	}

	// Convert to Articles for clustering
	articles := a.storiesToArticles(stories)

	// Cluster
	groups := a.clusteringEngine.Cluster(articles)

	// Debug: print clustering results
	if len(groups) == 0 {
		return domain.Snapshot{}, fmt.Errorf("clustering produced no groups from %d stories", len(stories))
	}

	// Debug: print cluster info
	for i, g := range groups {
		fmt.Printf("DEBUG cluster %d: ID=%s, articles=%d, headline=%s\n", i, g.ID, len(g.Articles), g.Headline)
	}

	// Build the snapshot
	snapshot, err := domain.BuildSnapshot(
		stories,
		groups,
		a.scorer,
		now,
		a.sourceNameResolver(),
		a.teamRegistry,
		10, // latest limit
	)
	if err != nil {
		return domain.Snapshot{}, fmt.Errorf("build snapshot domain: %w", err)
	}

	// Validate
	if err := snapshot.Validate(); err != nil {
		return domain.Snapshot{}, fmt.Errorf("snapshot validation failed: stories=%v (nil? %v) latest=%v (nil? %v) teams=%v (nil? %v): %w",
			snapshot.Stories, snapshot.Stories == nil,
			snapshot.Latest, snapshot.Latest == nil,
			snapshot.Teams, snapshot.Teams == nil,
			err)
	}

	return snapshot, nil
}

// sourceNameResolver returns a SourceNameResolver for this application.
func (a *Application) sourceNameResolver() domain.SourceNameResolver {
	// For now, return a static resolver with the source IDs as names.
	// In production, this would map source IDs to display names from config.
	resolver := domain.NewStaticSourceResolver(map[string]string{})
	if a.registry != nil {
		for _, src := range a.registry.Enabled() {
			// Use Publication as name if available, otherwise use ID
			name := src.ID
			if src.Publication != "" {
				name = src.Publication
			}
			resolver.AddMapping(src.ID, name)
		}
	}
	return resolver
}

// storiesToArticles converts domain.Story to domain.Article.
func (a *Application) storiesToArticles(stories []domain.Story) []domain.Article {
	var articles []domain.Article
	for _, s := range stories {
		articles = append(articles, domain.Article{
			ID:          s.ID,
			SourceID:    s.Source,
			Title:       s.Title,
			Summary:     s.Summary,
			URL:         s.URL,
			PublishedAt: s.PublishedAt.UTC().Format(time.RFC3339),
			FirstSeenAt: s.FirstSeenAt.UTC().Format(time.RFC3339),
			Entities:    s.Entities,
		})
	}
	return articles
}
