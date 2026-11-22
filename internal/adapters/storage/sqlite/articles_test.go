package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

func article(id string, title string, entities ...string) domain.Article {
	return domain.Article{
		ID:          id,
		SourceID:    "src-1",
		Title:       title,
		Summary:     "Article summary " + id,
		URL:         "https://example.com/" + id,
		GUID:        "guid-" + id,
		PublishedAt: "2024-01-15T12:00:00Z",
		FirstSeenAt: "2024-01-15T12:00:00Z",
		Author:      "Author " + id,
		Entities:    entities,
	}
}

func TestUpsertPersistsArticlesAndLinks(t *testing.T) {
	repo, err := NewRepository("")
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	defer func() { _ = repo.Close() }()

	story := domain.Story{
		ID:          "story-with-articles",
		Title:       "Story with articles",
		Source:      "test-source",
		PublishedAt: now(),
		Summary:     "Story summary",
		URL:         "https://example.com/story",
		GUID:        "guid-story",
		DedupeKey:   "test-source|guid-story",
		Headline:    "Story headline",
		ArticleData: []domain.Article{
			article("a1", "First article", "nfl"),
			article("a2", "Second article"),
		},
		ArticleIDs: []string{"a1", "a2"},
	}

	status, err := repo.Upsert(story)
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	if status != domain.UpsertCreated {
		t.Fatalf("expected UpsertCreated, got %v", status)
	}

	// Articles are persisted in the articles table
	var a1Title, a1Author, a1Entities string
	if err := repo.db.QueryRow(`SELECT title, author, entities FROM articles WHERE article_id = 'a1'`).
		Scan(&a1Title, &a1Author, &a1Entities); err != nil {
		t.Fatalf("read article a1: %v", err)
	}
	if a1Title != "First article" || a1Author != "Author a1" || a1Entities != `["nfl"]` {
		t.Errorf("unexpected article a1: title=%q author=%q entities=%q", a1Title, a1Author, a1Entities)
	}
	var a2Entities string
	if err := repo.db.QueryRow(`SELECT entities FROM articles WHERE article_id = 'a2'`).Scan(&a2Entities); err != nil {
		t.Fatalf("read article a2: %v", err)
	}
	if a2Entities != "[]" {
		t.Errorf("article a2 with nil entities should store [], got %q", a2Entities)
	}

	// story_articles links use the internal story UUID
	var links int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM story_articles WHERE story_id = (SELECT story_id FROM stories WHERE story_key = 'story-with-articles')`).Scan(&links); err != nil {
		t.Fatalf("count links: %v", err)
	}
	if links != 2 {
		t.Errorf("expected 2 story_articles links, got %d", links)
	}

	// Second upsert: a1 changed, a2 dropped from the story, a3 added
	updated := story
	updated.Title = "Story with articles v2"
	updated.ArticleData = []domain.Article{
		article("a1", "First article v2", "nfl", "nba"),
		article("a3", "Third article"),
	}
	updated.ArticleIDs = []string{"a1", "a3"}

	status, err = repo.Upsert(updated)
	if err != nil {
		t.Fatalf("second Upsert() error = %v", err)
	}
	if status != domain.UpsertUpdated {
		t.Errorf("expected UpsertUpdated, got %v", status)
	}

	// Last writer wins for a1
	if err := repo.db.QueryRow(`SELECT title, entities FROM articles WHERE article_id = 'a1'`).Scan(&a1Title, &a1Entities); err != nil {
		t.Fatalf("re-read article a1: %v", err)
	}
	if a1Title != "First article v2" || a1Entities != `["nfl","nba"]` {
		t.Errorf("article a1 not updated: title=%q entities=%q", a1Title, a1Entities)
	}

	// a2 remains in articles (row is never deleted) but the link is gone
	var a2Rows, a2Links, a3Links int
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM articles WHERE article_id = 'a2'`).Scan(&a2Rows); err != nil {
		t.Fatalf("count a2 rows: %v", err)
	}
	if a2Rows != 1 {
		t.Errorf("article a2 row should persist, got %d rows", a2Rows)
	}
	selStory := `story_id = (SELECT story_id FROM stories WHERE story_key = 'story-with-articles')`
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM story_articles WHERE ` + selStory + ` AND article_id = 'a2'`).Scan(&a2Links); err != nil {
		t.Fatalf("count a2 links: %v", err)
	}
	if a2Links != 0 {
		t.Errorf("a2 link should be removed, got %d", a2Links)
	}
	if err := repo.db.QueryRow(`SELECT COUNT(*) FROM story_articles WHERE ` + selStory + ` AND article_id = 'a3'`).Scan(&a3Links); err != nil {
		t.Fatalf("count a3 links: %v", err)
	}
	if a3Links != 1 {
		t.Errorf("a3 link should exist, got %d", a3Links)
	}

	// List hydrates article data from the new tables
	stories, err := repo.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(stories) != 1 {
		t.Fatalf("expected 1 story, got %d", len(stories))
	}
	if len(stories[0].ArticleData) != 2 {
		t.Fatalf("expected 2 hydrated articles, got %d", len(stories[0].ArticleData))
	}
	if len(stories[0].ArticleIDs) != 2 || stories[0].ArticleIDs[0] != "a1" || stories[0].ArticleIDs[1] != "a3" {
		t.Errorf("unexpected hydrated ArticleIDs: %v", stories[0].ArticleIDs)
	}
	if stories[0].ArticleData[0].Title != "First article v2" {
		t.Errorf("hydrated a1 title = %q, want %q", stories[0].ArticleData[0].Title, "First article v2")
	}
	if len(stories[0].ArticleData[0].Entities) != 2 {
		t.Errorf("hydrated a1 entities = %v, want 2", stories[0].ArticleData[0].Entities)
	}
}

func TestUpsertWithoutArticles(t *testing.T) {
	repo, err := NewRepository("")
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	defer func() { _ = repo.Close() }()

	base := domain.Story{
		ID:          "story-no-articles",
		Title:       "Story without articles",
		Source:      "test-source",
		PublishedAt: now(),
		Summary:     "Summary",
		URL:         "https://example.com/no-articles",
		GUID:        "guid-no-articles",
		DedupeKey:   "test-source|guid-no-articles",
		ArticleIDs:  []string{"legacy-id"},
	}

	status, err := repo.Upsert(base)
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	if status != domain.UpsertCreated {
		t.Fatalf("expected UpsertCreated, got %v", status)
	}

	// Identical re-upsert without articles is unchanged
	status, err = repo.Upsert(base)
	if err != nil {
		t.Fatalf("second Upsert() error = %v", err)
	}
	if status != domain.UpsertUnchanged {
		t.Errorf("expected UpsertUnchanged, got %v", status)
	}

	// List falls back to story_data ArticleIDs when no links exist
	stories, err := repo.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(stories) != 1 {
		t.Fatalf("expected 1 story, got %d", len(stories))
	}
	if len(stories[0].ArticleIDs) != 1 || stories[0].ArticleIDs[0] != "legacy-id" {
		t.Errorf("expected legacy ArticleIDs fallback [legacy-id], got %v", stories[0].ArticleIDs)
	}
	if stories[0].ArticleData != nil {
		t.Errorf("ArticleData should be nil without links, got %v", stories[0].ArticleData)
	}
}

func TestUpsertArticle(t *testing.T) {
	repo, err := NewRepository("")
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	defer func() { _ = repo.Close() }()

	a := article("up-1", "Original title", "nfl")
	if err := repo.UpsertArticle(a); err != nil {
		t.Fatalf("UpsertArticle() error = %v", err)
	}
	var title string
	if err := repo.db.QueryRow(`SELECT title FROM articles WHERE article_id = 'up-1'`).Scan(&title); err != nil {
		t.Fatalf("read article: %v", err)
	}
	if title != "Original title" {
		t.Errorf("title = %q, want %q", title, "Original title")
	}

	// Replace: last writer wins; nil entities marshal to []
	a2 := a
	a2.Title = "Replaced title"
	a2.Author = "New author"
	a2.Entities = nil
	if err := UpsertArticle(repo.db, a2); err != nil {
		t.Fatalf("package UpsertArticle() error = %v", err)
	}
	var replacedTitle, author string
	if err := repo.db.QueryRow(`SELECT title, author FROM articles WHERE article_id = 'up-1'`).Scan(&replacedTitle, &author); err != nil {
		t.Fatalf("re-read article: %v", err)
	}
	if replacedTitle != "Replaced title" || author != "New author" {
		t.Errorf("article not replaced: title=%q author=%q", replacedTitle, author)
	}

	// Nil entities marshal to []
	var entities string
	if err := repo.db.QueryRow(`SELECT entities FROM articles WHERE article_id = 'up-1'`).Scan(&entities); err != nil {
		t.Fatalf("read entities: %v", err)
	}
	if entities != "[]" {
		t.Errorf("entities = %q, want []", entities)
	}
}

func TestListAndCountPackageFuncs(t *testing.T) {
	repo, err := NewRepository("")
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	defer func() { _ = repo.Close() }()

	for _, id := range []string{"s1", "s2"} {
		story := domain.Story{
			ID:          id,
			Title:       "Title " + id,
			Source:      "test-source",
			PublishedAt: now(),
			Summary:     "Summary " + id,
			GUID:        "guid-" + id,
			DedupeKey:   "test-source|guid-" + id,
			ArticleData: []domain.Article{article("art-"+id, "Article "+id)},
			ArticleIDs:  []string{"art-" + id},
		}
		if status, uerr := repo.Upsert(story); uerr != nil || status != domain.UpsertCreated {
			t.Fatalf("Upsert(%s) = %v, %v; want created, nil", id, status, uerr)
		}
	}

	stories, err := List(repo.db)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(stories) != 2 {
		t.Fatalf("List() returned %d stories, want 2", len(stories))
	}

	n, err := Count(repo.db)
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if n != 2 {
		t.Errorf("Count() = %d, want 2", n)
	}

	stories, err = ListWithCtx(context.Background(), repo.db)
	if err != nil {
		t.Fatalf("ListWithCtx() error = %v", err)
	}
	if len(stories) != 2 {
		t.Fatalf("ListWithCtx() returned %d stories, want 2", len(stories))
	}
	for _, s := range stories {
		if len(s.ArticleData) != 1 {
			t.Errorf("story %s: expected 1 hydrated article, got %d", s.ID, len(s.ArticleData))
		}
	}

	n, err = CountWithCtx(context.Background(), repo.db)
	if err != nil {
		t.Fatalf("CountWithCtx() error = %v", err)
	}
	if n != 2 {
		t.Errorf("CountWithCtx() = %d, want 2", n)
	}
}

func TestNewRepositoryFromDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "fromdb.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer func() { _ = db.Close() }()

	repo, err := NewRepositoryFromDB(db)
	if err != nil {
		t.Fatalf("NewRepositoryFromDB() error = %v", err)
	}
	defer func() { _ = repo.Close() }()

	story := domain.Story{
		ID:          "fromdb-story",
		Title:       "From DB story",
		Source:      "test-source",
		PublishedAt: now(),
		Summary:     "Summary",
		GUID:        "guid-fromdb",
		DedupeKey:   "test-source|guid-fromdb",
		ArticleData: []domain.Article{article("fromdb-art", "Article")},
		ArticleIDs:  []string{"fromdb-art"},
	}
	status, err := repo.Upsert(story)
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	if status != domain.UpsertCreated {
		t.Fatalf("expected UpsertCreated, got %v", status)
	}
	n, err := repo.Count()
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if n != 1 {
		t.Errorf("Count() = %d, want 1", n)
	}
}
