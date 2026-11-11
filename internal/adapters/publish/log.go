package publish

import (
	"context"
	"fmt"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// LogPublisher is a concrete Publisher that logs ranked headlines.
// In production, this would commit to Hugo or a CMS.
type LogPublisher struct {
	failOnce  bool
	committed bool
	count     int
}

// Publish implements domain.Publisher.
func (p *LogPublisher) Publish(ctx context.Context, homepage domain.Homepage) error {
	p.count++

	if p.failOnce && !p.committed {
		p.failOnce = false // consume fail flag
		return fmt.Errorf("publish failed (simulated)")
	}

	p.committed = true

	// Log the homepage
	fmt.Printf("Publishing homepage (run at %s):\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Printf("Total ranked stories: %d\n", len(homepage.RankedStories))
	if len(homepage.Notes) > 0 {
		fmt.Println("Scoring notes:")
		for _, note := range homepage.Notes {
			fmt.Printf("  - %s\n", note)
		}
	}

	for i, story := range homepage.RankedStories {
		fmt.Printf("  %2d. %s (%s, score: %.2f)\n", i+1, story.Title, story.Source, 100.0)
	}

	return nil
}

// Count returns the number of times Publish was called.
func (p *LogPublisher) Count() int {
	return p.count
}

// SetFailOnce sets a flag to fail on the next Publish call.
func (p *LogPublisher) SetFailOnce() {
	p.failOnce = true
}

// Reset resets the publisher state.
func (p *LogPublisher) Reset() {
	p.failOnce = false
	p.committed = false
	p.count = 0
}

// NewLogPublisher creates a new LogPublisher.
func NewLogPublisher() *LogPublisher {
	return &LogPublisher{}
}
