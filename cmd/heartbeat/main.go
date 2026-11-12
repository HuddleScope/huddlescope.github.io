package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/heartbeat"
)

const defaultStaleAfter = 90 * time.Minute

func main() {
	file := flag.String("file", "web/data/heartbeat.json", "Path to heartbeat JSON file")
	staleAfter := flag.Duration("stale-after", defaultStaleAfter, "Duration after which a heartbeat is considered stale")
	flag.Parse()

	// Load heartbeat
	hb, found, err := heartbeat.Load(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading heartbeat: %v\n", err)
		os.Exit(1)
	}
	if !found {
		fmt.Fprintf(os.Stderr, "Heartbeat file not found: %s\n", *file)
		os.Exit(1)
	}

	// Validate
	if err := hb.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Heartbeat validation error: %v\n", err)
		os.Exit(1)
	}

	// Check staleness
	now := time.Now().UTC()
	isStale := hb.IsStale(now, *staleAfter)

	// Output summary
	fmt.Printf("Heartbeat status:\n")
	fmt.Printf("  Version: %s\n", hb.Version)
	fmt.Printf("  Run ID: %s\n", hb.RunID)
	fmt.Printf("  Status: %s\n", hb.Status)
	fmt.Printf("  Attempted at: %s\n", hb.AttemptedAt)
	if hb.IngestedAt != nil {
		fmt.Printf("  Ingested at: %s\n", *hb.IngestedAt)
	} else {
		fmt.Printf("  Ingested at: (none)\n")
	}
	if hb.PublishedAt != nil {
		fmt.Printf("  Published at: %s\n", *hb.PublishedAt)
	} else {
		fmt.Printf("  Published at: (none)\n")
	}
	fmt.Printf("  Sources checked: %d\n", hb.SourcesChecked)
	fmt.Printf("  Sources failed: %d\n", hb.SourcesFailed)
	fmt.Printf("  Errors: %d\n", len(hb.Errors))
	if len(hb.Errors) > 0 {
		fmt.Printf("  Errors (first 5):\n")
		for i, e := range hb.Errors {
			if i >= 5 {
				break
			}
			fmt.Printf("    %s\n", e)
		}
	}

	fmt.Printf("  Stale threshold: %v\n", *staleAfter)
	if isStale {
		fmt.Printf("  Stale: YES (publication not fresh)\n")
	} else {
		fmt.Printf("  Stale: no\n")
	}

	// Exit codes
	if !found {
		os.Exit(1)
	}
	if err := hb.Validate(); err != nil {
		os.Exit(1)
	}
	if isStale {
		os.Exit(1)
	}

	os.Exit(0)
}