package storage

import (
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// Repository interface for story storage.
type Repository interface {
	Upsert(story domain.Story) error
	List() ([]domain.Story, error)
	Count() (int, error)
}

// FeedFetcher interface for fetching entries from feeds.
type FeedFetcher interface {
	Fetch(sourceID string) ([]domain.FeedEntry, error)
}

// InMemoryRepository is an in-memory implementation of Repository for testing.
type InMemoryRepository struct {
	stories map[string]domain.Story
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{stories: make(map[string]domain.Story)}
}

func (r *InMemoryRepository) Upsert(story domain.Story) error {
	r.stories[story.ID] = story
	return nil
}

func (r *InMemoryRepository) List() ([]domain.Story, error) {
	result := make([]domain.Story, 0, len(r.stories))
	for _, s := range r.stories {
		result = append(result, s)
	}
	return result, nil
}

func (r *InMemoryRepository) Count() (int, error) {
	return len(r.stories), nil
}
