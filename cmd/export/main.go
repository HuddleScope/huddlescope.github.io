package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/heartbeat"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/hugo"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage"
	"github.com/HuddleScope/huddlescope.github.io/internal/application"
	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

func main() {
	configFile := flag.String("config", "", "Path to config file (.env format)")
	check := flag.Bool("check", false, "Show config and exit")
	dryRun := flag.Bool("dry-run", false, "Run export without processing")
	dataDir := flag.String("data-dir", "", "Output directory for snapshot JSON (empty: use config DataDir)")
	dataFile := flag.String("data-file", "", "Output filename for snapshot JSON (empty: use config DataFile)")
	contentDir := flag.String("content-dir", "", "Output directory for story pages (empty: use config HugoContentDir)")
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

	// Resolve snapshot output paths: explicit flag wins, otherwise config
	resolvedDataDir := *dataDir
	if resolvedDataDir == "" {
		resolvedDataDir = cfg.DataDir
	}
	resolvedDataFile := *dataFile
	if resolvedDataFile == "" {
		resolvedDataFile = cfg.DataFile
	}

	// Resolve story page output path: explicit flag wins, otherwise config
	resolvedContentDir := *contentDir
	if resolvedContentDir == "" {
		resolvedContentDir = cfg.HugoContentDir
	}

	// Build repository
	repo, err := storage.NewRepository(cfg.Storage())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating repository: %v\n", err)
		os.Exit(1)
	}

	// Load teams from config file
	teamRegistry, err := cfg.TeamRegistry()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Build clusterer and scorer (fixed time for determinism in tests)
	clustering := domain.NewClusterer()
	scorer := domain.NewScorer().WithTime(time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC))

	// Build story page publisher (HS-061): writes into the Hugo content directory
	storyPagePublisher := hugo.NewPublisher(resolvedContentDir, cfg.HugoBaseURL)

	// Build snapshot publisher (writes to Hugo data directory)
	fullDataPath := filepath.Join(resolvedDataDir, resolvedDataFile)
	snapshotPublisher := hugo.NewDataPublisher(resolvedDataDir, resolvedDataFile)

	// Build application
	app := application.New(
		application.WithRepository(repo),
		application.WithTeamRegistry(teamRegistry),
		application.WithClusterer(clustering),
		application.WithScorer(scorer),
		application.WithSnapshotPublisher(snapshotPublisher),
		application.WithStoryPagePublisher(storyPagePublisher),
	)

	// Run snapshot export
	if *dryRun {
		result := app.RunSnapshotPreview(context.Background(), time.Now())
		// Dry run: output result but skip write and heartbeat update
		fmt.Printf("Snapshot preview complete\n")
		fmt.Printf("  Committed: %v\n", result.PubResult.Committed)
		fmt.Printf("  Changed: %v\n", result.PubResult.Changed)
		fmt.Printf("  Path: %v\n", result.PubResult.Path)
		fmt.Printf("  Version: %v\n", result.PubResult.Version)
		fmt.Printf("  Stories built: %d\n", result.StoriesBuilt)

		if len(result.PubResult.Errors) > 0 {
			fmt.Printf("  Errors: %v\n", result.PubResult.Errors)
		}

		if len(result.Errors) > 0 {
			fmt.Printf("Application errors: %v\n", result.Errors)
			os.Exit(2)
		}

		fmt.Printf("Dry run preview complete\n")
		os.Exit(0)
	}

	result := app.RunSnapshot(context.Background(), time.Now())

	// Update heartbeat with publish result (best-effort)
	heartbeatPath := cfg.HeartbeatFile
	hb, found, err := heartbeat.Load(heartbeatPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load heartbeat for update: %v\n", err)
		found = false
	}
	if !found {
		hb = domain.NewHeartbeat("export-run")
		hb.IngestedAt = nil
	}
	hb.MergePublication(result.PubResult, time.Now())
	if err := hb.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: heartbeat validation failed: %v\n", err)
	} else {
		hbPub := heartbeat.NewFilePublisher(heartbeatPath)
		if err := hbPub.Publish(context.Background(), hb); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to update heartbeat: %v\n", err)
		}
	}

	// Output result
	fmt.Printf("Snapshot export complete\n")
	fmt.Printf("  Committed: %v\n", result.PubResult.Committed)
	fmt.Printf("  Changed: %v\n", result.PubResult.Changed)
	fmt.Printf("  Path: %v\n", result.PubResult.Path)
	fmt.Printf("  Version: %v\n", result.PubResult.Version)
	fmt.Printf("  Stories built: %d\n", result.StoriesBuilt)

	if len(result.PubResult.Errors) > 0 {
		fmt.Printf("  Errors: %v\n", result.PubResult.Errors)
	}

	if len(result.Errors) > 0 {
		fmt.Printf("Application errors: %v\n", result.Errors)
		os.Exit(2)
	}

	// Exit non-zero if snapshot not committed
	if !result.PubResult.Committed {
		fmt.Printf("Snapshot was not committed\n")
		os.Exit(2)
	}

	fmt.Printf("Exported snapshot to %s\n", fullDataPath)
}
