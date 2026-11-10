// Package storage provides storage backends.
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

// MemoryRepository is an in-memory implementation of Repository for testing.
type MemoryRepository struct {
	stories map[string]domain.Story
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{stories: make(map[string]domain.Story)}
}

func (r *MemoryRepository) Upsert(story domain.Story) error {
	r.stories[story.ID] = story
	return nil
}

func (r *MemoryRepository) List() ([]domain.Story, error) {
	result := make([]domain.Story, 0, len(r.stories))
	for _, s := range r.stories {
		result = append(result, s)
	}
	return result, nil
}

func (r *MemoryRepository) Count() (int, error) {
	return len(r.stories), nil
}
