package main

import (
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

	// Check mode: print config and exit
	if *check {
		data, _ := json.MarshalIndent(cfg, "", "  ")
		fmt.Println(string(data))
		os.Exit(0)
	}

	// Build application with adapters
	repo := storage.NewInMemoryRepository()
	fetcher := rss.NewStaticFetcher([]domain.FeedEntry{
		{
			Title:       "Test Story",
			Source:      "test-feed",
			PublishedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Summary:     "Test summary",
			URL:         "https://example.com/test",
		},
	})

	app := application.New(
		application.WithRepository(repo),
		application.WithFeedFetchers(fetcher),
	)

	result := app.RunIngest(nil)

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
