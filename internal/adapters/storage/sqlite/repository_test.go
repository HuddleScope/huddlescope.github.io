package sqlite

import (
	"database/sql"
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
	if len(versions) != 11 {
		t.Errorf("expected 11 migrations, got %d: %v", len(versions), versions)
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

func TestUpsertEntityChange(t *testing.T) {
	repo, err := NewRepository("")
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	defer func() { _ = repo.Close() }()

	if _, err := Migrate(""); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	// First upsert with one entity
	baseStory := domain.Story{
		ID:          "test-entity-1",
		Title:       "Test Story with Entity",
		Source:      "test-source",
		PublishedAt: now(),
		Summary:     "Test summary",
		URL:         "https://example.com/test",
		GUID:        "guid-entity-1",
		DedupeKey:   "test-source|guid-entity-1",
		Entities:    []string{"nfl"},
	}

	status, err := repo.Upsert(baseStory)
	if err != nil {
		t.Fatalf("first Upsert() error = %v", err)
	}
	if status != domain.UpsertCreated {
		t.Errorf("expected UpsertCreated, got %v", status)
	}

	// Second upsert with one new entity (same title, etc.)
	updatedStory := baseStory
	updatedStory.Entities = []string{"nfl", "nba"}

	status, err = repo.Upsert(updatedStory)
	if err != nil {
		t.Fatalf("second Upsert() error = %v", err)
	}
	if status != domain.UpsertUpdated {
		t.Errorf("expected UpsertUpdated for new entity, got %v", status)
	}

	// Verify entities persisted
	stories, err := repo.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(stories) != 1 {
		t.Fatalf("expected 1 story, got %d", len(stories))
	}

	// Verify both entities are present
	entities := stories[0].Entities
	if len(entities) != 2 {
		t.Errorf("expected 2 entities, got %d: %v", len(entities), entities)
	}
	entitySet := make(map[string]bool)
	for _, e := range entities {
		entitySet[e] = true
	}
	if !entitySet["nfl"] || !entitySet["nba"] {
		t.Errorf("expected entities [nfl, nba], got %v", entities)
	}
}

func TestUpsertEntityOrderUnchanged(t *testing.T) {
	repo, err := NewRepository("")
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	defer func() { _ = repo.Close() }()

	if _, err := Migrate(""); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	// First upsert with entities in specific order
	baseStory := domain.Story{
		ID:          "test-entity-order-1",
		Title:       "Test Story Entity Order",
		Source:      "test-source",
		PublishedAt: now(),
		Summary:     "Test summary",
		URL:         "https://example.com/order-test",
		GUID:        "guid-order-1",
		DedupeKey:   "test-source|guid-order-1",
		Entities:    []string{"nba", "nfl", "mlb"},
	}

	status, err := repo.Upsert(baseStory)
	if err != nil {
		t.Fatalf("first Upsert() error = %v", err)
	}
	if status != domain.UpsertCreated {
		t.Errorf("expected UpsertCreated, got %v", status)
	}

	// Second upsert with identical entities in different order
	reorderedStory := baseStory
	reorderedStory.Entities = []string{"mlb", "nba", "nfl"}

	status, err = repo.Upsert(reorderedStory)
	if err != nil {
		t.Fatalf("second Upsert() error = %v", err)
	}
	if status != domain.UpsertUnchanged {
		t.Errorf("expected UpsertUnchanged for same entities reordered, got %v", status)
	}

	// Verify list returns the same story (count 1)
	stories, err := repo.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(stories) != 1 {
		t.Fatalf("expected 1 story (reorder should not create duplicate), got %d", len(stories))
	}
}

func now() time.Time {
	return time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
}

// TestUpsertArticleDisplayTitle_RoundTrip (#201): an article upserted with a
// display title must persist it, and ListWithCtx must hydrate it back (AC-1, AC-3).
func TestUpsertArticleDisplayTitle_RoundTrip(t *testing.T) {
	repo, err := NewRepository("")
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	defer func() { _ = repo.Close() }()

	if _, err := Migrate(""); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	article := domain.Article{
		ID:           "art-1",
		SourceID:     "official-min",
		Title:        "chiefs sign rb james cook", // normalized identity
		DisplayTitle: "Chiefs sign RB James Cook", // original casing
		Summary:      "summary",
		URL:          "https://example.com/chiefs-cook",
		PublishedAt:  now().Format(time.RFC3339),
		FirstSeenAt:  now().Format(time.RFC3339),
	}
	if err := repo.UpsertArticle(article); err != nil {
		t.Fatalf("UpsertArticle() error = %v", err)
	}

	// Read the display_title column directly to verify persistence.
	var displayTitle string
	if err := repo.db.QueryRow(`SELECT display_title FROM articles WHERE article_id = ?`, "art-1").Scan(&displayTitle); err != nil {
		t.Fatalf("read display_title: %v", err)
	}
	if displayTitle != "Chiefs sign RB James Cook" {
		t.Errorf("articles.display_title = %q; want %q", displayTitle, "Chiefs sign RB James Cook")
	}

	// Also verify the normalized title column.
	var title string
	if err := repo.db.QueryRow(`SELECT title FROM articles WHERE article_id = ?`, "art-1").Scan(&title); err != nil {
		t.Fatalf("read title: %v", err)
	}
	if title != "chiefs sign rb james cook" {
		t.Errorf("articles.title = %q; want normalized", title)
	}
}

// TestUpsertStoryDisplayTitle_RoundTrip (#201): a story upserted with a display
// title must persist it in both the display_title column and the story_data JSON,
// and List must hydrate it back.
func TestUpsertStoryDisplayTitle_RoundTrip(t *testing.T) {
	repo, err := NewRepository("")
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	defer func() { _ = repo.Close() }()

	if _, err := Migrate(""); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	story := domain.Story{
		ID:           "story-display-1",
		Title:        "chiefs sign rb james cook",
		DisplayTitle: "Chiefs sign RB James Cook",
		Source:       "official-min",
		PublishedAt:  now(),
		Summary:      "summary",
		URL:          "https://example.com/chiefs-cook",
		GUID:         "guid-1",
		DedupeKey:    "official-min|guid-1",
	}
	if _, err := repo.Upsert(story); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	stories, err := repo.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(stories) != 1 {
		t.Fatalf("List() returned %d stories; want 1", len(stories))
	}
	if stories[0].DisplayTitle != "Chiefs sign RB James Cook" {
		t.Errorf("List()[0].DisplayTitle = %q; want %q", stories[0].DisplayTitle, "Chiefs sign RB James Cook")
	}
	if stories[0].Display() != "Chiefs sign RB James Cook" {
		t.Errorf("List()[0].Display() = %q; want display casing", stories[0].Display())
	}

	// Re-upsert identical story: Display() values match so unchanged detected.
	status, err := repo.Upsert(story)
	if err != nil {
		t.Fatalf("second Upsert() error = %v", err)
	}
	if status != domain.UpsertUnchanged {
		t.Errorf("second Upsert() status = %v; want UpsertUnchanged", status)
	}
}

// TestLegacyStoryDisplayTitleFallback (#201, AC-3): a story persisted before
// #201 (no display_title) must fall back to the normalized title when hydrated,
// and a re-ingestion that restores original casing must be detected as a change.
func TestLegacyStoryDisplayTitleFallback(t *testing.T) {
	repo, err := NewRepository("")
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	defer func() { _ = repo.Close() }()

	if _, err := Migrate(""); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	// Simulate a legacy row: display_title column NULL/empty, story_data without
	// displayTitle key. The migration 010 backfill should have populated the
	// column from the headline/title, but to exercise the legacy fallback we
	// directly set the column to empty (pre-#201 shape).
	story := domain.Story{
		ID:          "story-legacy-1",
		Title:       "chiefs sign rb james cook",
		Source:      "official-min",
		PublishedAt: now(),
		Summary:     "summary",
		URL:         "https://example.com/chiefs-cook",
		GUID:        "guid-legacy",
		DedupeKey:   "official-min|guid-legacy",
	}
	if _, err := repo.Upsert(story); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	// Clear the display_title column to simulate a pre-#201 row (the backfill
	// would normally have populated it). The story_data JSON has no displayTitle
	// key, so hydration must fall back to Title.
	if _, err := repo.db.Exec(`UPDATE stories SET display_title = '' WHERE story_key = ?`, story.ID); err != nil {
		t.Fatalf("clear display_title: %v", err)
	}

	stories, err := repo.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(stories) != 1 {
		t.Fatalf("List() returned %d stories; want 1", len(stories))
	}
	// Display() must fall back to the normalized Title.
	if stories[0].Display() != "chiefs sign rb james cook" {
		t.Errorf("legacy story Display() = %q; want normalized title fallback", stories[0].Display())
	}

	// Re-ingest with original casing restored: Display() now differs, so a
	// change must be detected (AC-3).
	reingested := story
	reingested.DisplayTitle = "Chiefs sign RB James Cook"
	status, err := repo.Upsert(reingested)
	if err != nil {
		t.Fatalf("re-ingest Upsert() error = %v", err)
	}
	if status != domain.UpsertUpdated {
		t.Errorf("re-ingest Upsert() status = %v; want UpsertUpdated (display restored)", status)
	}
}

// TestMigration010_BackfillDisplayTitle (#201): migration 010 backfills the
// display_title column from the existing normalized title/headline for rows that
// pre-date the migration, so legacy rows keep rendering as today.
func TestMigration010_BackfillDisplayTitle(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "backfill.db")
	defer func() { _ = os.Remove(dbPath) }()

	// Build a DB at schema 9 (pre-#201), insert a story, then apply 010.
	db, err := openDBForTest(t, dbPath)
	if err != nil {
		t.Fatalf("open DB: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Apply migrations 001-009 manually to reach schema 9 (pre-#201).
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')))`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	names, err := ListMigrations()
	if err != nil {
		t.Fatalf("ListMigrations: %v", err)
	}
	for _, name := range names {
		ver, _ := parseMigrationVersion(name)
		if ver > 9 {
			continue // skip 010 — the migration under test
		}
		raw, err := ReadMigration(name)
		if err != nil {
			t.Fatalf("ReadMigration %s: %v", name, err)
		}
		if _, err := db.Exec(string(raw)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, ver); err != nil {
			t.Fatalf("mark %d: %v", ver, err)
		}
	}

	// Insert a story at schema 9 with a known normalized title/headline.
	// story_id must satisfy the 36-char UUID CHECK constraint; use a fixed 36-char
	// UUID. The stories table at schema 9 has no `url` column (it uses
	// canonical_url).
	if _, err := db.Exec(`INSERT INTO stories (story_id, story_key, title, headline, summary, canonical_url, published_at, first_seen_at, updated_at, dedupe_key, story_data)
		VALUES ('aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee', 'key-1', 'chiefs sign rb james cook', 'chiefs sign rb james cook', 'summary', 'https://example.com/chiefs-cook', '2026-01-01T10:00:00Z', '2026-01-01T09:00:00Z', '2026-01-01T10:00:00Z', 'official-min|guid-1', '{}')`); err != nil {
		t.Fatalf("insert story: %v", err)
	}

	// Apply migration 010 (011 also applies since it is newer than the seeded
	// schema-9 state; this test is about the 010 backfill).
	versions, err := MigrateDB(db)
	if err != nil {
		t.Fatalf("MigrateDB: %v", err)
	}
	applied := map[int]bool{}
	for _, v := range versions {
		applied[v] = true
	}
	if !applied[10] {
		t.Fatalf("expected migration 10 applied, got %v", versions)
	}
	if !applied[11] {
		t.Fatalf("expected migration 11 applied, got %v", versions)
	}
	if len(versions) != 2 {
		t.Fatalf("expected migrations [10 11] applied, got %v", versions)
	}

	// The display_title column must be backfilled from the headline/title.
	var displayTitle string
	if err := db.QueryRow(`SELECT display_title FROM stories WHERE story_key = 'key-1'`).Scan(&displayTitle); err != nil {
		t.Fatalf("read backfilled display_title: %v", err)
	}
	if displayTitle != "chiefs sign rb james cook" {
		t.Errorf("backfilled display_title = %q; want normalized headline", displayTitle)
	}
}

// openDBForTest opens a sqlite DB at the given path and returns it.
func openDBForTest(t *testing.T, path string) (*sql.DB, error) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	return db, nil
}
