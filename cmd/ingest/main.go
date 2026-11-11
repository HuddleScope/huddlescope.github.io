package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/publish"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/rss"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage"
	"github.com/HuddleScope/huddlescope.github.io/internal/application"
	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
	"github.com/HuddleScope/huddlescope.github.io/internal/feeds"
)

func main() {
	configFile := flag.String("config", "", "Path to config file (.env format)")
	check := flag.Bool("check", false, "Show config and exit")
	flag.Parse()

	cfg, err := config.Load(*configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Validation error: %v\n", err)
		os.Exit(1)
	}

	// Check mode: print redacted config and exit
	if *check {
		data, _ := json.MarshalIndent(cfg.Redacted(), "", "  ")
		fmt.Println(string(data))
		os.Exit(0)
	}

	// Build publisher
	publisher := publish.NewLogPublisher()

	// Build repository
	repo := storage.NewMemoryRepository()

	// Try to load registry from feeds seed data
	registry, registryErr := feeds.Load()
	if registryErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load feed registry: %v\n", registryErr)
	}

	// Load teams from config file
	teamRegistry := cfg.TeamRegistry()
	fmt.Printf("Loaded %d teams from config\n", len(cfg.Teams))

	// Build application
	var appOpts []application.Option
	appOpts = append(appOpts, application.WithRepository(repo))
	appOpts = append(appOpts, application.WithPublisher(publisher))
	appOpts = append(appOpts, application.WithLookback(cfg.LookbackWindow))

	if registry != nil {
		// Use registry-driven ingestion with real HTTP fetchers
		rateLimiter := rss.NewHostRateLimiter(1 * time.Second)
		etagCache := rss.NewETagCache()

		sourceFetchers := make(map[string]domain.FeedFetcher)
		for _, src := range registry.Enabled() {
			fetcher := rss.NewHTTPFetcher(src,
				rss.WithUserAgent(cfg.UserAgent),
				rss.WithDefaultTimeout(cfg.RequestTimeout),
				rss.WithRateLimiter(rateLimiter),
				rss.WithETagCache(etagCache),
			)
			sourceFetchers[src.ID] = fetcher
		}
		appOpts = append(appOpts, application.WithRegistry(registry))
		appOpts = append(appOpts, application.WithFetcherMap(sourceFetchers))
	} else {
		// Fallback to legacy static fetcher
		fetcher := rss.NewStaticFetcher([]domain.FeedEntry{
			{
				Title:       "Test Story",
				Source:      "test-feed",
				PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				Summary:     "Test summary",
				URL:         "https://example.com/test",
			},
		})
		appOpts = append(appOpts, application.WithFeedFetchers(fetcher))
	}

	// Add clusterer and scorer (required for pipeline)
	appOpts = append(appOpts, application.WithClusterer(domain.NewClusterer()))
	appOpts = append(appOpts, application.WithScorer(domain.NewScorer()))

	// Add team registry for entity extraction
	appOpts = append(appOpts, application.WithTeamRegistry(teamRegistry))

	app := application.New(appOpts...)

	// Build timestamp for pipeline
	now := time.Now().UTC()

	// Run pipeline
	result := app.RunPipeline(context.Background(), now)

	// Output result summary
	fmt.Printf("Run ID: %s\n", result.RunID)
	fmt.Printf("Started at: %s\n", result.StartedAt.Format(time.RFC3339))
	if result.EndedAt != nil {
		fmt.Printf("Ended at: %s\n", result.EndedAt.Format(time.RFC3339))
	}
	fmt.Printf("Status: %s\n", result.Status)

	if result.LastPublishedAt != nil {
		fmt.Printf("Last published entry: %s\n", result.LastPublishedAt.Format(time.RFC3339))
	}

	fmt.Printf("Sources checked: %d\n", len(result.Sources))
	fmt.Printf("Entries fetched: %d\n", result.EntriesFetched)
	fmt.Printf("Stories created: %d\n", result.StoriesCreated)
	fmt.Printf("Stories updated: %d\n", result.StoriesUpdated)
	fmt.Printf("Stories skipped: %d\n", result.StoriesSkipped)

	if result.Homepage != nil {
		fmt.Printf("Stories in homepage: %d\n", len(result.Homepage.RankedStories))
	}

	fmt.Printf("Groups created: %d\n", result.GroupsCreated)
	fmt.Printf("Articles grouped: %d\n", result.ArticlesGrouped)
	fmt.Printf("Articles solo: %d\n", result.ArticlesSolo)

	if result.Homepage != nil && result.Homepage.Notes != nil {
		fmt.Println("Homepage scoring notes:")
		for _, note := range result.Homepage.Notes {
			fmt.Printf("  - %s\n", note)
		}
	}

	if len(result.Errors) > 0 {
		fmt.Printf("Errors: %v\n", result.Errors)
	}

	// Exit with appropriate code
	if result.Status.IsCritical() {
		os.Exit(result.Status.ExitCode())
	}

	// If we get here, run completed (OK or Partial)
	if result.Committed {
		fmt.Println("Homepage committed successfully")
	}
}
