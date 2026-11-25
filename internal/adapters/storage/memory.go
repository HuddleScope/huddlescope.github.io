// Package storage provides storage backends.
package storage

import (
	"sort"
	"sync"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// Repository interface for story storage.
type Repository interface {
	Upsert(story domain.Story) (domain.UpsertStatus, error)
	UpsertArticle(article domain.Article) error
	List() ([]domain.Story, error)
	Count() (int, error)
}

// MemoryRepository is an in-memory implementation of Repository for testing.
type MemoryRepository struct {
	mu      sync.RWMutex
	stories map[string]domain.Story
	dedupe  map[string]string // dedupeKey -> storyID
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		stories: make(map[string]domain.Story),
		dedupe:  make(map[string]string),
	}
}

func (r *MemoryRepository) Upsert(story domain.Story) (domain.UpsertStatus, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, exists := r.stories[story.ID]
	if !exists {
		// ID not found; try dedupe key match
		dedupeKey := story.DedupeKey
		if dedupeKey != "" {
			if existingID, ok := r.dedupe[dedupeKey]; ok {
				// Dedupe match: update existing story (merge)
				existing, exists := r.stories[existingID]
				if !exists {
					// Should not happen; log and create new
					story.FirstSeenAt = time.Now().UTC()
					r.stories[story.ID] = story
					return domain.UpsertCreated, nil
				}

				// Merge: preserve ID and FirstSeenAt, take meaningful fields
				story.FirstSeenAt = existing.FirstSeenAt
				story.UpdatedAt = time.Now().UTC()
				r.stories[existingID] = story
				return domain.UpsertUpdated, nil
			}
		}

		// New story: set FirstSeenAt to now
		story.FirstSeenAt = time.Now().UTC()
		r.stories[story.ID] = story

		// Index by dedupe key if available
		if story.DedupeKey != "" {
			r.dedupe[story.DedupeKey] = story.ID
		}

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
	// Map iteration order is randomized; sort by ID so callers always see a
	// deterministic order (downstream snapshot/cluster tie-breaks depend on it).
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (r *MemoryRepository) Count() (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.stories), nil
}

// UpsertArticle adds or updates an Article in the memory repository.
// This is a no-op for pure memory storage - articles are not persisted.
func (r *MemoryRepository) UpsertArticle(article domain.Article) error {
	// No-op: Memory repository doesn't persist articles
	// This exists to satisfy the domain.Repository interface
	return nil
}

// MemoryRuleRepository is an in-memory implementation of
// domain.CorrectionRuleRepository for testing and the memory backend.
type MemoryRuleRepository struct {
	mu    sync.RWMutex
	rules map[string]domain.CorrectionRule
}

// NewMemoryRuleRepository returns an empty in-memory rule repository.
func NewMemoryRuleRepository() *MemoryRuleRepository {
	return &MemoryRuleRepository{
		rules: make(map[string]domain.CorrectionRule),
	}
}

// SaveRule implements domain.CorrectionRuleRepository.SaveRule.
func (r *MemoryRuleRepository) SaveRule(rule domain.CorrectionRule) error {
	if err := rule.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rules[rule.ID] = rule
	return nil
}

// ListRules implements domain.CorrectionRuleRepository.ListRules. It returns
// rules in deterministic (ID-sorted) order.
func (r *MemoryRuleRepository) ListRules() ([]domain.CorrectionRule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]domain.CorrectionRule, 0, len(r.rules))
	for _, rule := range r.rules {
		result = append(result, rule)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}
