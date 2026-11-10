// Package storage provides storage backends.
package storage

import (
	"sync"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// Repository interface for story storage.
type Repository interface {
	Upsert(story domain.Story) (domain.UpsertStatus, error)
	List() ([]domain.Story, error)
	Count() (int, error)
}

// MemoryRepository is an in-memory implementation of Repository for testing.
type MemoryRepository struct {
	mu      sync.RWMutex
	stories map[string]domain.Story
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{stories: make(map[string]domain.Story)}
}

func (r *MemoryRepository) Upsert(story domain.Story) (domain.UpsertStatus, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, exists := r.stories[story.ID]
	if !exists {
		// New story: set FirstSeenAt to now
		story.FirstSeenAt = time.Now().UTC()
		r.stories[story.ID] = story
		return domain.UpsertCreated, nil
	}

	// Existing story: check if meaningful fields changed
	if existing.MeaningfulFieldsChanged(story) {
		// Update fields but preserve FirstSeenAt
		story.FirstSeenAt = existing.FirstSeenAt
		r.stories[story.ID] = story
		return domain.UpsertUpdated, nil
	}

	// No meaningful change
	return domain.UpsertUnchanged, nil
}

func (r *MemoryRepository) List() ([]domain.Story, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]domain.Story, 0, len(r.stories))
	for _, s := range r.stories {
		result = append(result, s)
	}
	return result, nil
}

func (r *MemoryRepository) Count() (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.stories), nil
}
