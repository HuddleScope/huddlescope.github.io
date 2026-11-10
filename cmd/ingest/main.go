package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

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
	dryRun := flag.Bool("dry-run", false, "Run ingress without processing")
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

	// Build application with adapters
	repo := storage.NewMemoryRepository()

	// Try to load registry from feeds seed data
	registry, registryErr := feeds.Load()
	if registryErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load feed registry: %v\n", registryErr)
	}

	var appOpts []application.Option
	appOpts = append(appOpts, application.WithRepository(repo))

	if registry != nil {
		// Use registry-driven ingestion
		// For now, map all sources to the same StaticFetcher (placeholder for HTTP fetcher)
		// Each source gets its own fetcher to enable per-source isolation
		sourceFetchers := make(map[string]domain.FeedFetcher)
		for _, src := range registry.All() {
			// For demo, use a fetcher that returns the source metadata as a single entry
			// In production, this would resolve a real HTTP fetcher
			feedEntries := make([]domain.FeedEntry, 0)
			// If this is a known enabled source, return some sample entries
			if src.Enabled {
				feedEntries = append(feedEntries, domain.FeedEntry{
					Title:       src.Publication + " Feed Entry",
					Source:      src.ID,
					PublishedAt: time.Now().UTC(),
					Summary:     "Sample feed entry from " + src.Publication,
					URL:         src.URL,
				})
			}
			sourceFetchers[src.ID] = rss.NewStaticFetcher(feedEntries)
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

	app := application.New(appOpts...)

	result := app.RunIngest(context.Background())

	// Output result
	if *dryRun {
		fmt.Println("Dry run complete")
	}

	fmt.Printf("Entries fetched: %d\n", result.EntriesFetched)
	fmt.Printf("Stories created: %d\n", result.StoriesCreated)
	fmt.Printf("Stories updated: %d\n", result.StoriesUpdated)
	fmt.Printf("Stories skipped: %d\n", result.StoriesSkipped)

	if len(result.Errors) > 0 {
		fmt.Printf("Errors: %v\n", result.Errors)
		os.Exit(2)
	}
}
