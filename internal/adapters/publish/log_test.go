package publish

import (
	"context"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

func TestLogPublisher_PublishSuccess(t *testing.T) {
	p := NewLogPublisher()

	homepage := domain.Homepage{
		RankedStories: []domain.Story{
			{ID: "1", Title: "Story 1", Source: "test"},
			{ID: "2", Title: "Story 2", Source: "test"},
		},
		Notes: []string{"note1", "note2"},
	}

	err := p.Publish(context.Background(), homepage)
	if err != nil {
		t.Errorf("Publish() returned error: %v", err)
	}

	if p.Count() != 1 {
		t.Errorf("Count() = %d, want 1", p.Count())
	}

	if !p.committed {
		t.Error("committed should be true after successful publish")
	}
}

func TestLogPublisher_FailOnce(t *testing.T) {
	p := NewLogPublisher()
	p.SetFailOnce()

	homepage := domain.Homepage{
		RankedStories: []domain.Story{{ID: "1", Title: "Story 1", Source: "test"}},
	}

	err := p.Publish(context.Background(), homepage)
	if err == nil {
		t.Error("Publish() should return error when failOnce is set")
	}

	// Second call should succeed (failOnce consumed)
	err = p.Publish(context.Background(), homepage)
	if err != nil {
		t.Errorf("Second Publish() should succeed: %v", err)
	}

	if p.Count() != 2 {
		t.Errorf("Count() = %d, want 2", p.Count())
	}
}

func TestLogPublisher_Reset(t *testing.T) {
	p := NewLogPublisher()
	p.SetFailOnce()

	homepage := domain.Homepage{
		RankedStories: []domain.Story{{ID: "1", Title: "Story 1", Source: "test"}},
	}

	// First fails
	_ = p.Publish(context.Background(), homepage)

	// Reset and verify state
	p.Reset()

	if p.failOnce {
		t.Error("failOnce should be false after Reset()")
	}

	if p.committed {
		t.Error("committed should be false after Reset()")
	}

	if p.count != 0 {
		t.Errorf("count should be 0 after Reset(), got %d", p.count)
	}
}

func TestLogPublisher_NoStories(t *testing.T) {
	p := NewLogPublisher()

	homepage := domain.Homepage{
		RankedStories: []domain.Story{},
		Notes:         []string{},
	}

	err := p.Publish(context.Background(), homepage)
	if err != nil {
		t.Errorf("Publish() with empty stories returned error: %v", err)
	}

	if p.Count() != 1 {
		t.Errorf("Count() = %d, want 1", p.Count())
	}
}

func TestLogPublisher_Timeout(t *testing.T) {
	p := NewLogPublisher()

	homepage := domain.Homepage{
		RankedStories: []domain.Story{{ID: "1", Title: "Story 1", Source: "test"}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Context shouldn't be cancelled immediately
	err := p.Publish(ctx, homepage)
	if err != nil {
		t.Errorf("Publish() with valid context returned error: %v", err)
	}
}
