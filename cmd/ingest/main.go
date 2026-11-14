package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/heartbeat"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/publish"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/rss"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage"
	"github.com/HuddleScope/huddlescope.github.io/internal/application"
	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
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

	// Build repository based on storage backend
	repo, err := storage.NewRepository(storage.RepositoryOptions{
		Backend: cfg.StorageBackend,
		Path:    cfg.StoragePath,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating repository: %v\n", err)
		os.Exit(1)
	}

	// Build the feed registry from the configured sources. This is the single
	// source of truth: conf/sources.yaml or HSC_SOURCES, both handled by config.
	registry := cfg.SourceRegistry()

	// Fail fast when no sources are configured.
	if registry.Count() == 0 {
		fmt.Fprintln(os.Stderr, "Error: no feed sources are configured. Provide a sources file (HSC_SOURCES_FILE or conf/sources.yaml) or set HSC_SOURCES.")
		os.Exit(1)
	}

	// Load teams from config file
	teamRegistry, err := cfg.TeamRegistry()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Loaded %d sources, %d teams from config\n", registry.Count(), len(cfg.Teams))

	// Build application
	var appOpts []application.Option
	appOpts = append(appOpts, application.WithRepository(repo))
	appOpts = append(appOpts, application.WithPublisher(publisher))
	appOpts = append(appOpts, application.WithLookback(cfg.LookbackWindow))

	// Registry-driven ingestion with real HTTP fetchers
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

	// Publish heartbeat (best-effort: failures don't affect run exit)
	heartbeatPath := os.Getenv("HSC_HEARTBEAT_FILE")
	if heartbeatPath == "" {
		heartbeatPath = "web/data/heartbeat.json"
	}
	if hbPub := heartbeat.NewFilePublisher(heartbeatPath); hbPub != nil {
		if hb := domain.NewHeartbeat(result.RunID.String()); hb != nil {
			hb.FromRunRecord(result, *result.EndedAt)
			if err := hbPub.Publish(context.Background(), hb); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to publish heartbeat: %v\n", err)
			}
		}
	}

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
