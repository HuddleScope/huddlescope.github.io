package application

import (
	"context"
	"fmt"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// StoryPagePublisher defines the interface for publishing story detail pages.
type StoryPagePublisher interface {
	Publish(ctx context.Context, page domain.StoryDetailPage) error
}

// DefaultStoryPagePublisher is a no-op publisher for offline/dev environments.
type DefaultStoryPagePublisher struct{}

func (p *DefaultStoryPagePublisher) Publish(ctx context.Context, page domain.StoryDetailPage) error {
	// No-op: in production, this would commit to Hugo or CMS
	return nil
}

// StoryPageResult represents the result of a story page build/run.
type StoryPageResult struct {
	PagesBuilt     int
	PagesPublished int
	FailedStories  int
	Errors         []error
}

// RunStoryPages builds and publishes story detail pages for all clustered stories.
// It uses the clustering engine (from HS-010) to group articles and the StoryPagePublisher
// to write pages. Articles without valid URLs are skipped gracefully (AC4).
func (a *Application) RunStoryPages(ctx context.Context, now time.Time, publisher StoryPagePublisher) StoryPageResult {
	result := StoryPageResult{}

	if a.repo == nil {
		return StoryPageResult{
			FailedStories: 0,
			Errors:        []error{fmt.Errorf("repository not configured")},
		}
	}

	if a.clusteringEngine == nil {
		return StoryPageResult{
			FailedStories: 0,
			Errors:        []error{fmt.Errorf("clustering engine not configured; RunGrouping must be called first")},
		}
	}

	// If no publisher provided, use no-op
	if publisher == nil {
		publisher = &DefaultStoryPagePublisher{}
	}

	// List stories
	stories, err := a.repo.List()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("failed to list stories: %w", err))
		result.FailedStories = 1
		return result
	}

	// Convert stories to articles
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

	// Cluster to form story groups
	groups := a.clusteringEngine.Cluster(articles)
	result.PagesBuilt = len(groups)

	// Build and publish each story page
	for _, group := range groups {
		// Build the story detail page
		// Use a simple resolver (in production, this would come from config)
		resolver := domain.NewStaticSourceResolver(map[string]string{
			group.Articles[0].SourceID: group.Articles[0].SourceID,
		})

		page, err := domain.BuildStoryDetailPage(group, resolver, now)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("build failed for story group %s: %w", group.ID, err))
			result.FailedStories++
			continue
		}

		// Publish the page
		if err := publisher.Publish(ctx, page); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("publish failed for story %s: %w", page.StoryID, err))
			result.FailedStories++
			continue
		}

		result.PagesPublished++
	}

	return result
}