package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

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
	dataDir := flag.String("data-dir", "web/data", "Output directory for snapshot JSON")
	dataFile := flag.String("data-file", "homepage.json", "Output filename for snapshot JSON")
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

	// Build repository (memory for now; Turso via HS-040 when ready)
	repo := storage.NewMemoryRepository()

	// Load teams from config file
	teamRegistry := cfg.TeamRegistry()

	// Build clusterer and scorer (fixed time for determinism in tests)
	clustering := domain.NewClusterer()
	scorer := domain.NewScorer().WithTime(time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC))

	// Build snapshot publisher (writes to Hugo data directory)
	fullDataPath := filepath.Join(*dataDir, *dataFile)
	snapshotPublisher := hugo.NewDataPublisher(*dataDir, *dataFile)

	// Build application
	app := application.New(
		application.WithRepository(repo),
		application.WithTeamRegistry(teamRegistry),
		application.WithClusterer(clustering),
		application.WithScorer(scorer),
		application.WithSnapshotPublisher(snapshotPublisher),
	)

	// Run snapshot export
	result := app.RunSnapshot(context.Background(), time.Now())

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

	// Exit non-zero if dry-run enabled but nothing was done
	if *dryRun && !result.PubResult.Changed {
		fmt.Println("Dry run complete - no changes")
		os.Exit(0)
	}

	fmt.Printf("Exported snapshot to %s\n", fullDataPath)
}
