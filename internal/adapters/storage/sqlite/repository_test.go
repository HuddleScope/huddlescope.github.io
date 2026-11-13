package sqlite

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

func TestMigrateSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	defer func() { _ = os.Remove(dbPath) }()

	versions, err := Migrate(dbPath)
	if err != nil {
		t.Fatalf("Migrate() error = %v; want nil", err)
	}
	if len(versions) == 0 {
		t.Fatal("Migrate() returned empty versions")
	}
	if len(versions) != 3 {
		t.Errorf("expected 3 migrations, got %d: %v", len(versions), versions)
	}
}

func TestMigrateIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	defer func() { _ = os.Remove(dbPath) }()

	if _, err := Migrate(dbPath); err != nil {
		t.Fatalf("first Migrate() error = %v", err)
	}

	// Second run should be idempotent (no new migrations applied)
	if _, err := Migrate(dbPath); err != nil {
		t.Fatalf("second Migrate() error = %v; want nil (idempotent)", err)
	}
}

func TestMigrateEmptyPath(t *testing.T) {
	versions, err := Migrate("")
	if err != nil {
		t.Fatalf("Migrate(\"\") error = %v; want nil (in-memory)", err)
	}
	if len(versions) == 0 {
		t.Fatal("Migrate(\"\") returned empty versions")
	}
}

func TestNewRepositorySuccess(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	defer func() { _ = os.Remove(dbPath) }()

	if _, err := Migrate(dbPath); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	repo, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("NewRepository() error = %v; want nil", err)
	}
	defer func() { _ = repo.Close() }()

	if repo == nil {
		t.Fatal("NewRepository() returned nil")
	}
}

func TestNewRepositoryEmptyPath(t *testing.T) {
	repo, err := NewRepository("")
	if err != nil {
		t.Fatalf("NewRepository(\"\") error = %v; want nil (in-memory)", err)
	}
	defer func() { _ = repo.Close() }()
	if repo == nil {
		t.Fatal("NewRepository(\"\") returned nil")
	}
}

func TestUpsertCreated(t *testing.T) {
	repo, err := NewRepository("")
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	defer func() { _ = repo.Close() }()

	if _, err := Migrate(""); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	story := domain.Story{
		ID:          "test-1",
		Title:       "Test Story",
		Source:      "test-source",
		PublishedAt: now(),
		Summary:     "Test summary",
		URL:         "https://example.com/1",
		GUID:        "guid-1",
		DedupeKey:   "test|guid1",
	}

	_, err = repo.Upsert(story)
	if err != nil {
		t.Fatalf("Upsert() error = %v; want nil", err)
	}

	count, err := repo.Count()
	if err != nil {
		t.Fatalf("Count() error = %v; want nil", err)
	}
	if count != 1 {
		t.Errorf("Count() = %d; want 1", count)
	}
}

func TestUpsertUpdated(t *testing.T) {
	repo, err := NewRepository("")
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	defer func() { _ = repo.Close() }()

	if _, err := Migrate(""); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	story := domain.Story{
		ID:          "test-1",
		Title:       "Original Title",
		Source:      "test-source",
		PublishedAt: now(),
		Summary:     "Original summary",
		URL:         "https://example.com/1",
		GUID:        "guid-1",
		DedupeKey:   "test|guid1",
	}

	_, err = repo.Upsert(story)
	if err != nil {
		t.Fatalf("first Upsert() error = %v", err)
	}

	story.Title = "Updated Title"
	_, err = repo.Upsert(story)
	if err != nil {
		t.Fatalf("second Upsert() error = %v", err)
	}
}

func TestUpsertUnchanged(t *testing.T) {
	repo, err := NewRepository("")
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	defer func() { _ = repo.Close() }()

	if _, err := Migrate(""); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	story := domain.Story{
		ID:          "test-1",
		Title:       "Original Title",
		Source:      "test-source",
		PublishedAt: now(),
		Summary:     "Original summary",
		URL:         "https://example.com/1",
		GUID:        "guid-1",
		DedupeKey:   "test|guid1",
	}

	_, err = repo.Upsert(story)
	if err != nil {
		t.Fatalf("first Upsert() error = %v", err)
	}

	// Second upsert with no change
	_, err = repo.Upsert(story)
	if err != nil {
		t.Fatalf("second Upsert() error = %v", err)
	}
}

func TestList(t *testing.T) {
	repo, err := NewRepository("")
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	defer func() { _ = repo.Close() }()

	if _, err := Migrate(""); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	// Insert 3 stories
	for i := 0; i < 3; i++ {
		story := domain.Story{
			ID:          "test-" + string(rune('1'+i)),
			Title:       "Story " + string(rune('1'+i)),
			Source:      "test-source",
			PublishedAt: time.Date(2024, 1, i+1, 0, 0, 0, 0, time.UTC),
			Summary:     "Summary " + string(rune('1'+i)),
			URL:         "https://example.com/" + string(rune('1'+i)),
			GUID:        "guid-" + string(rune('1'+i)),
			DedupeKey:   "test|guid" + string(rune('1'+i)),
		}
		if _, err := repo.Upsert(story); err != nil {
			t.Fatalf("Upsert() error = %v", err)
		}
	}

	stories, err := repo.List()
	if err != nil {
		t.Fatalf("List() error = %v; want nil", err)
	}
	if len(stories) != 3 {
		t.Errorf("List() returned %d stories; want 3", len(stories))
	}

	// Check ordering (descending by published_at)
	for i := 0; i < len(stories)-1; i++ {
		if stories[i].PublishedAt.Before(stories[i+1].PublishedAt) {
			t.Errorf("List() not ordered by published_at DESC at index %d", i)
		}
	}
}

func TestCount(t *testing.T) {
	repo, err := NewRepository("")
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	defer func() { _ = repo.Close() }()

	if _, err := Migrate(""); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	count, err := repo.Count()
	if err != nil {
		t.Fatalf("Count() error = %v; want nil", err)
	}
	if count != 0 {
		t.Errorf("Count() = %d; want 0", count)
	}

	story := domain.Story{
		ID:          "test-1",
		Title:       "Test Story",
		Source:      "test-source",
		PublishedAt: now(),
		Summary:     "Test summary",
		URL:         "https://example.com/1",
		GUID:        "guid-1",
		DedupeKey:   "test|guid1",
	}
	if _, err := repo.Upsert(story); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	count, err = repo.Count()
	if err != nil {
		t.Fatalf("Count() after insert error = %v; want nil", err)
	}
	if count != 1 {
		t.Errorf("Count() after insert = %d; want 1", count)
	}
}

func TestRestartPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	defer func() { _ = os.Remove(dbPath) }()

	repo1, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("first NewRepository() error = %v", err)
	}
	if err := repo1.Migrate(); err != nil {
		t.Fatalf("first Migrate() error = %v", err)
	}

	story := domain.Story{
		ID:          "test-1",
		Title:       "Test Story",
		Source:      "test-source",
		PublishedAt: now(),
		Summary:     "Test summary",
		URL:         "https://example.com/1",
		GUID:        "guid-1",
		DedupeKey:   "test|guid1",
	}
	if _, err := repo1.Upsert(story); err != nil {
		t.Fatalf("first Upsert() error = %v", err)
	}
	_ = repo1.Close()

	repo2, err := NewRepository(dbPath)
	if err != nil {
		t.Fatalf("second NewRepository() error = %v", err)
	}
	if err := repo2.Migrate(); err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}
	defer func() { _ = repo2.Close() }()

	count, err := repo2.Count()
	if err != nil {
		t.Fatalf("Count() after restart error = %v; want nil", err)
	}
	if count != 1 {
		t.Errorf("Count() after restart = %d; want 1 (persistence failed)", count)
	}

	stories, err := repo2.List()
	if err != nil {
		t.Fatalf("List() after restart error = %v; want nil", err)
	}
	if len(stories) != 1 {
		t.Fatalf("List() after restart returned %d stories; want 1", len(stories))
	}
	if stories[0].Title != "Test Story" {
		t.Errorf("story title after restart = %q; want %q", stories[0].Title, "Test Story")
	}
}

func TestDedupeKeyUpsert(t *testing.T) {
	repo, err := NewRepository("")
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	defer func() { _ = repo.Close() }()

	if _, err := Migrate(""); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	story1 := domain.Story{
		ID:          "story-1",
		Title:       "Story 1",
		Source:      "source-1",
		PublishedAt: now(),
		Summary:     "Summary 1",
		URL:         "https://example.com/1",
		GUID:        "guid-1",
		DedupeKey:   "source-1|guid1",
	}
	_, err = repo.Upsert(story1)
	if err != nil {
		t.Fatalf("first Upsert() error = %v", err)
	}

	story2 := domain.Story{
		ID:          "story-2",
		Title:       "Story 1 Updated",
		Source:      "source-1",
		PublishedAt: now(),
		Summary:     "Summary 1 Updated",
		URL:         "https://example.com/1",
		GUID:        "guid-1",
		DedupeKey:   "source-1|guid1",
	}
	_, err = repo.Upsert(story2)
	if err != nil {
		t.Fatalf("second Upsert() error = %v", err)
	}

	count, err := repo.Count()
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 1 {
		t.Errorf("Count() = %d; want 1 (dedupe failed)", count)
	}
}

func now() time.Time {
	return time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
}
