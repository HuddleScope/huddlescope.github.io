package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
	"github.com/google/uuid"
)

// NewRepository opens or creates a database and returns a Repository.
// It does NOT run Migrate(); call Migrate() separately.
func NewRepository(dbPath string) (*Repository, error) {
	if dbPath == "" {
		dbPath = "file::memory:?cache=shared"
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	// Enable foreign keys per connection
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	// Apply migrations on open
	if _, err := Migrate(dbPath); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Repository{db: db}, nil
}

// NewRepositoryFromDB wraps an existing *sql.DB handle and runs migrations on it.
// Use this when you already have an open database handle (e.g., Turso).
// The handle is NOT closed by this function.
func NewRepositoryFromDB(db *sql.DB) (*Repository, error) {
	// Enable foreign keys per connection
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	// Apply migrations
	if _, err := MigrateDB(db); err != nil {
		return nil, err
	}
	return &Repository{db: db}, nil
}

// Repository implements domain.Repository using modernc.org/sqlite.
type Repository struct {
	db *sql.DB
}

// Upsert implements domain.Repository.Upsert.
func (r *Repository) Upsert(story domain.Story) (domain.UpsertStatus, error) {
	return Upsert(r.db, story)
}

// List implements domain.Repository.List.
func (r *Repository) List() ([]domain.Story, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	return ListWithCtx(ctx, r.db)
}

// Count implements domain.Repository.Count.
func (r *Repository) Count() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	return CountWithCtx(ctx, r.db)
}

// Migrate applies schema migrations.
func (r *Repository) Migrate() error {
	_, err := MigrateDB(r.db)
	return err
}

// Close closes the database connection.
func (r *Repository) Close() error {
	return r.db.Close()
}

// DB returns the underlying *sql.DB handle. It is exposed so that companion
// repositories (e.g. the correction-rule repository) can share the same
// connection without re-opening the database.
func (r *Repository) DB() *sql.DB {
	return r.db
}

// Upsert is a database/sql compatible Upsert implementation.
func Upsert(db *sql.DB, story domain.Story) (domain.UpsertStatus, error) {
	var upsertStatus domain.UpsertStatus

	// Check by story_key (story.ID)
	var existingID string
	var dedupeKey string
	err := db.QueryRow("SELECT story_id, dedupe_key FROM stories WHERE story_key = ?", story.ID).Scan(&existingID, &dedupeKey)
	if err == nil {
		// Found by story_key, check meaningful fields
		var title, summary, canonicalURL, publishedAt, guid, headline, firstSeenAt, updatedAt, displayTitleCol string
		var storyData []byte

		err = db.QueryRow(`
			SELECT title, summary, canonical_url, published_at, story_data, headline, guid,
			      first_seen_at, updated_at, display_title
			FROM stories
			WHERE story_key = ?
		`, story.ID).Scan(&title, &summary, &canonicalURL, &publishedAt, &storyData, &headline, &guid,
			&firstSeenAt, &updatedAt, &displayTitleCol)

		if err != nil {
			return domain.UpsertUnchanged, fmt.Errorf("read story: %w", err)
		}

		// Extract entities and article IDs from existing story_data if present
		var legacyEntities []string
		var legacyEntitiesJSON string
		err = db.QueryRow(`SELECT COALESCE(json_extract(story_data, '$.entities'), '[]') FROM stories WHERE story_key = ?`, story.ID).Scan(&legacyEntitiesJSON)
		if err == nil && legacyEntitiesJSON != "" {
			legacyEntities = []string{}
			_ = json.Unmarshal([]byte(legacyEntitiesJSON), &legacyEntities)
		}

		// Merge entities from legacy and current
		mergedEntities := mergeEntities(legacyEntities, story.Entities)

		// Determine if there are meaningful changes
		// (#201): the existing display_title column is loaded so that DisplayTitle
		// participates in unchanged-detection — a display-only re-ingestion still
		// counts as a change.
		existingDisplayTitle := storyDataDisplayTitle(storyData, displayTitleCol)
		otherStory := domain.Story{
			Title:        title,
			DisplayTitle: existingDisplayTitle,
			Summary:      summary,
			CanonicalURL: canonicalURL,
			PublishedAt:  parseTimestamp(publishedAt),
			GUID:         guid,
			Headline:     headline,
			Entities:     legacyEntities,
			FirstSeenAt:  parseTimestamp(firstSeenAt),
			UpdatedAt:    parseTimestamp(updatedAt),
		}

		// Check if merged entities differ from existing (normalized comparison)
		existingEntitiesNorm := normalizeEntities(legacyEntities)
		mergedEntitiesNorm := normalizeEntities(mergedEntities)
		entitiesChanged := !equalStringArrays(existingEntitiesNorm, mergedEntitiesNorm)

		if !otherStory.MeaningfulFieldsChanged(story) && !entitiesChanged {
			upsertStatus = domain.UpsertUnchanged
		} else {
			upsertStatus = domain.UpsertUpdated
		}

		// Upsert articles into articles table using ArticleData
		err = upsertArticles(db, story.ArticleData)
		if err != nil {
			return domain.UpsertUnchanged, fmt.Errorf("upsert articles: %w", err)
		}

		// Update story_articles links (use DB story_id, not story.ID)
		err = upsertStoryArticles(db, existingID, story.ArticleIDs)
		if err != nil {
			return domain.UpsertUnchanged, fmt.Errorf("upsert story_articles: %w", err)
		}

		// Update story record with merged entities
		mergedStory := story
		mergedStory.Entities = mergedEntities
		data, _ := mergedStory.MarshalJSON()
		displayTitle := story.DisplayTitle
		if strings.TrimSpace(displayTitle) == "" {
			displayTitle = story.Title
		}
		_, err = db.Exec(`UPDATE stories 
			SET title = ?, summary = ?, canonical_url = ?, published_at = ?, 
				story_data = ?, headline = ?, display_title = ?,
				first_seen_at = ?, updated_at = datetime('now')
			WHERE story_key = ?`,
			title, summary, canonicalURL,
			timestampToString(story.PublishedAt), string(data), headline,
			displayTitle,
			firstSeenAt, story.ID)
		if err != nil {
			return domain.UpsertUnchanged, fmt.Errorf("update story: %w", err)
		}

		return upsertStatus, nil
	}

	// Not found by story_key, try dedupe_key
	var dedupeFound bool
	err = db.QueryRow("SELECT 1 FROM stories WHERE dedupe_key = ? LIMIT 1", story.DedupeKey).Scan(&dedupeFound)
	if err == nil && dedupeFound {
		// Dedupe hit: update existing
		// Get existing story_id and entities for the deduped row
		var existingEntities string
		q := db.QueryRow(`SELECT story_id, COALESCE(json_extract(story_data, '$.entities'), '[]') FROM stories WHERE dedupe_key = ?`, story.DedupeKey)
		err = q.Scan(&existingID, &existingEntities)
		if err != nil {
			return domain.UpsertUnchanged, fmt.Errorf("read existing entities: %w", err)
		}

		var legacyEntities []string
		if existingEntities != "" {
			_ = json.Unmarshal([]byte(existingEntities), &legacyEntities)
		}

		result, err := db.Exec(`UPDATE stories 
			SET title = ?, summary = ?, canonical_url = ?, published_at = ?, 
				display_title = ?, updated_at = datetime('now')
			WHERE dedupe_key = ?`,
			story.Title, story.Summary, story.CanonicalURL,
			timestampToString(story.PublishedAt), storyDisplayTitle(story), story.DedupeKey)
		if err != nil {
			return domain.UpsertUnchanged, fmt.Errorf("dedupe update: %w", err)
		}
		affected, _ := result.RowsAffected()
		if affected > 0 {
			// Merge entities from legacy and current
			mergedEntities := mergeEntities(legacyEntities, story.Entities)

			// Upsert articles
			err = upsertArticles(db, story.ArticleData)
			if err != nil {
				return domain.UpsertUnchanged, fmt.Errorf("upsert articles: %w", err)
			}
			// Upsert story_articles (use dedupeKey path)
			err = upsertStoryArticles(db, existingID, story.ArticleIDs)
			if err != nil {
				return domain.UpsertUnchanged, fmt.Errorf("upsert story_articles: %w", err)
			}

			// Check if merged entities differ from existing (normalized comparison)
			legacyEntitiesNorm := normalizeEntities(legacyEntities)
			mergedEntitiesNorm := normalizeEntities(mergedEntities)
			entitiesChanged := !equalStringArrays(legacyEntitiesNorm, mergedEntitiesNorm)

			// Update story_data with merged entities
			mergedStory := story
			mergedStory.Entities = mergedEntities
			data, _ := mergedStory.MarshalJSON()
			_, err = db.Exec(`UPDATE stories SET story_data = ? WHERE dedupe_key = ?`, string(data), story.DedupeKey)
			if err != nil {
				return domain.UpsertUnchanged, fmt.Errorf("update story_data: %w", err)
			}

			// If only entities changed (same entities, different order), return unchanged
			if !entitiesChanged {
				return domain.UpsertUnchanged, nil
			}
			return domain.UpsertUpdated, nil
		}
	}

	// Insert new story
	uuid := uuid.New().String()
	nowStr := timestampToString(time.Now().UTC())
	data, _ := story.MarshalJSON()
	displayTitle := story.DisplayTitle
	if strings.TrimSpace(displayTitle) == "" {
		displayTitle = story.Title
	}
	result, err := db.Exec(`INSERT INTO stories 
		(story_id, story_key, title, summary, published_at, 
		 canonical_url, guid, story_data, dedupe_key, status, created_at, first_seen_at, updated_at, headline, display_title)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'active', ?, ?, ?, ?, ?)`,
		uuid, story.ID, story.Title, story.Summary,
		timestampToString(story.PublishedAt), story.CanonicalURL, story.GUID,
		string(data), story.DedupeKey, nowStr, nowStr, nowStr, story.Headline, displayTitle)
	if err != nil {
		return domain.UpsertUnchanged, fmt.Errorf("insert story: %w", err)
	}
	_, _ = result.LastInsertId()

	// Upsert articles using ArticleData
	err = upsertArticles(db, story.ArticleData)
	if err != nil {
		return domain.UpsertUnchanged, fmt.Errorf("upsert articles: %w", err)
	}

	// Insert story_articles links (use UUID of new story)
	err = upsertStoryArticles(db, uuid, story.ArticleIDs)
	if err != nil {
		return domain.UpsertUnchanged, fmt.Errorf("upsert story_articles: %w", err)
	}

	return domain.UpsertCreated, nil
}

// upsertArticles inserts or updates articles in the articles table.
func upsertArticles(db *sql.DB, articles []domain.Article) error {
	if len(articles) == 0 {
		return nil
	}

	// Use transaction for batch operations
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	for _, article := range articles {
		// Check if article exists; a missing row means a fresh insert
		var exists bool
		err := tx.QueryRow("SELECT 1 FROM articles WHERE article_id = ? LIMIT 1", article.ID).Scan(&exists)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			_ = tx.Rollback()
			return fmt.Errorf("check article: %w", err)
		}

		if exists {
			// Article exists, update with latest data (per HS-064: last writer wins)
			_, err = tx.Exec(`UPDATE articles SET 
				source_id = ?, url = ?, guid = ?, title = ?, display_title = ?, summary = ?, 
				author = ?, published_at = ?, first_seen_at = ?, entities = ?
				WHERE article_id = ?`,
				article.SourceID, article.URL, article.GUID, article.Title,
				articleDisplayTitle(article),
				article.Summary, article.Author, article.PublishedAt,
				article.FirstSeenAt,
				marshalEntities(article.Entities),
				article.ID)
			if err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("update article %s: %w", article.ID, err)
			}
		} else {
			// Insert new article
			_, err = tx.Exec(`INSERT INTO articles (
				article_id, source_id, url, guid, title, display_title, summary, author, published_at, first_seen_at, entities
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				article.ID, article.SourceID, article.URL, article.GUID, article.Title,
				articleDisplayTitle(article),
				article.Summary, article.Author, article.PublishedAt, article.FirstSeenAt,
				marshalEntities(article.Entities))

			if err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("insert article %s: %w", article.ID, err)
			}
		}
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// upsertStoryArticles inserts or updates story_articles links.
// Article IDs without a matching articles row are skipped: legacy story_data
// IDs may predate the articles table, and linking them would violate the
// foreign key.
func upsertStoryArticles(db *sql.DB, storyID string, articleIDs []string) error {
	if len(articleIDs) == 0 {
		return nil
	}

	// Delete existing links for this story
	_, err := db.Exec(`DELETE FROM story_articles WHERE story_id = ?`, storyID)
	if err != nil {
		return fmt.Errorf("delete existing story_articles: %w", err)
	}

	// Keep only article IDs that exist in the articles table
	placeholders := make([]string, len(articleIDs))
	args := make([]interface{}, len(articleIDs))
	for i, id := range articleIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	// #nosec G202: only static "?" placeholders are concatenated; IDs are bound as arguments
	rows, err := db.Query(`SELECT article_id FROM articles WHERE article_id IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return fmt.Errorf("check story_articles: %w", err)
	}
	existing := make(map[string]bool, len(articleIDs))
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan story_articles: %w", err)
		}
		existing[id] = true
	}
	_ = rows.Close()

	// Insert new links
	for _, articleID := range articleIDs {
		if articleID == "" || !existing[articleID] {
			continue
		}
		_, err := db.Exec(`INSERT OR IGNORE INTO story_articles (story_id, article_id) VALUES (?, ?)`,
			storyID, articleID)
		if err != nil {
			return fmt.Errorf("insert story_articles: %w", err)
		}
	}

	return nil
}

// List is a database/sql compatible List implementation.
func List(db *sql.DB) ([]domain.Story, error) {
	rows, err := db.Query(`SELECT story_data, display_title FROM stories ORDER BY published_at DESC, story_id ASC`)
	if err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("list: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var stories []domain.Story
	for rows.Next() {
		var data []byte
		var displayTitle string
		if err := rows.Scan(&data, &displayTitle); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		var s domain.Story
		if err := s.UnmarshalJSON(data); err != nil {
			return nil, fmt.Errorf("unmarshal: %w", err)
		}
		// Hydrate DisplayTitle from the column when the story_data JSON key is
		// absent (legacy rows persisted before #201) so pre-change rows keep
		// rendering as today (#201, AC-3).
		if strings.TrimSpace(s.DisplayTitle) == "" {
			s.DisplayTitle = displayTitle
		}
		stories = append(stories, s)
	}
	return stories, rows.Err()
}

// ListWithCtx is like List but accepts a context for timeout control.
func ListWithCtx(ctx context.Context, db *sql.DB) ([]domain.Story, error) {
	// Step 1: Get all story IDs and basic data
	rows, err := db.QueryContext(ctx, `SELECT story_id, story_data, display_title FROM stories ORDER BY published_at DESC, story_id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Collect story IDs and basic data
	type storyData struct {
		storyID      string
		storyData    []byte
		displayTitle string
	}
	var storyList []storyData
	for rows.Next() {
		var s storyData
		if err := rows.Scan(&s.storyID, &s.storyData, &s.displayTitle); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		storyList = append(storyList, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	// Step 2: Fetch article data in batches to avoid memory issues
	storyMap := make(map[string]*domain.Story)
	var storyIDs []string
	for _, s := range storyList {
		var story domain.Story
		if err := story.UnmarshalJSON(s.storyData); err != nil {
			return nil, fmt.Errorf("unmarshal: %w", err)
		}
		storyMap[s.storyID] = &story
		storyIDs = append(storyIDs, s.storyID)
	}

	// Batch process story_articles joins to get article data
	articlesByStory := make(map[string][]domain.Article)
	if len(storyIDs) > 0 {
		// Process in batches of 500 to avoid SQLite limitations
		const batchSize = 500
		for i := 0; i < len(storyIDs); i += batchSize {
			n := i + batchSize
			if n > len(storyIDs) {
				n = len(storyIDs)
			}
			batch := storyIDs[i:n]

			// Build placeholders for IN clause
			placeholders := make([]string, len(batch))
			args := make([]interface{}, len(batch))
			for j, id := range batch {
				placeholders[j] = "?"
				args[j] = id
			}
			// #nosec G202: only static "?" placeholders are concatenated; IDs are bound as arguments
			query := `SELECT sa.story_id, a.article_id, a.source_id, a.url, a.guid, a.title,
				a.display_title,
				a.summary, a.author, a.published_at, a.first_seen_at, a.entities
			FROM story_articles sa
			JOIN articles a ON a.article_id = sa.article_id
			WHERE sa.story_id IN (` + strings.Join(placeholders, ",") + `)
			ORDER BY a.published_at ASC, a.article_id ASC`

			batchRows, err := db.QueryContext(ctx, query, args...)
			if err != nil {
				return nil, fmt.Errorf("article query: %w", err)
			}

			for batchRows.Next() {
				var sa struct {
					storyID      string
					articleID    string
					sourceID     string
					url          string
					guid         string
					title        string
					displayTitle string
					summary      string
					author       string
					publishedAt  string
					firstSeenAt  string
					entities     string
				}
				if err := batchRows.Scan(&sa.storyID, &sa.articleID, &sa.sourceID, &sa.url, &sa.guid, &sa.title,
					&sa.displayTitle,
					&sa.summary, &sa.author, &sa.publishedAt, &sa.firstSeenAt, &sa.entities); err != nil {
					_ = batchRows.Close()
					return nil, fmt.Errorf("scan article: %w", err)
				}

				article := domain.Article{
					ID:           sa.articleID,
					SourceID:     sa.sourceID,
					URL:          sa.url,
					GUID:         sa.guid,
					Title:        sa.title,
					DisplayTitle: sa.displayTitle,
					Summary:      sa.summary,
					Author:       sa.author,
					PublishedAt:  sa.publishedAt,
					FirstSeenAt:  sa.firstSeenAt,
				}

				// Legacy rows: an empty display_title falls back to the normalized title
				// so the domain Display() helper and the hydrated article stay consistent (#201).
				if strings.TrimSpace(article.DisplayTitle) == "" {
					article.DisplayTitle = article.Title
				}

				// Parse entities JSON
				if sa.entities != "" {
					var entities []string
					err := json.Unmarshal([]byte(sa.entities), &entities)
					if err == nil {
						article.Entities = entities
					}
				}

				articlesByStory[sa.storyID] = append(articlesByStory[sa.storyID], article)
			}
			_ = batchRows.Close()
		}
	}

	// Step 3: Attach article data to stories and provide fallback for legacy stories
	var result []domain.Story
	for _, storyData := range storyList {
		story := storyMap[storyData.storyID]
		// Hydrate DisplayTitle from the display_title column when the story_data
		// JSON key is absent (legacy rows persisted before #201). The column was
		// backfilled with the normalized headline/title, so pre-change rows keep
		// rendering as today, while re-ingestion restores original casing (AC-3).
		if strings.TrimSpace(story.DisplayTitle) == "" {
			story.DisplayTitle = storyData.displayTitle
		}
		if articles, exists := articlesByStory[storyData.storyID]; exists {
			// We have articles from the new system
			story.ArticleData = articles
			// Populate ArticleIDs with sorted IDs from the articles
			articleIDs := make([]string, len(articles))
			for i, article := range articles {
				articleIDs[i] = article.ID
			}
			sort.Strings(articleIDs)
			story.ArticleIDs = articleIDs
		} else {
			// No articles found in new system - fallback to story_data ArticleIDs
			// This maintains backward compatibility for stories with no links
			var legacyStory domain.Story
			if err := legacyStory.UnmarshalJSON(storyData.storyData); err == nil {
				story.ArticleIDs = legacyStory.ArticleIDs
				// Keep ArticleData as nil for now (will be handled by the hydrator)
			}
		}
		result = append(result, *story)
	}

	return result, nil
}

// Count is a database/sql compatible Count implementation.
func Count(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM stories").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count: %w", err)
	}
	return count, nil
}

// CountWithCtx is like Count but accepts a context for timeout control.
func CountWithCtx(ctx context.Context, db *sql.DB) (int, error) {
	var count int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM stories").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count: %w", err)
	}
	return count, nil
}

// UpsertArticle adds or updates an Article in the database.
func UpsertArticle(db *sql.DB, article domain.Article) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO articles 
		(article_id, source_id, url, guid, title, display_title, summary, author, published_at, first_seen_at, entities)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		article.ID, article.SourceID, article.URL, article.GUID, article.Title,
		articleDisplayTitle(article),
		article.Summary, article.Author, article.PublishedAt, article.FirstSeenAt,
		marshalEntities(article.Entities))
	if err != nil {
		return fmt.Errorf("upsert article: %w", err)
	}
	return nil
}

// storyDisplayTitle returns the display title to persist for a story: the
// original feed casing (DisplayTitle) when set, otherwise the normalized Title
// (#201). This keeps the display_title column non-empty and consistent with the
// domain Display() fallback.
func storyDisplayTitle(story domain.Story) string {
	if d := strings.TrimSpace(story.DisplayTitle); d != "" {
		return story.DisplayTitle
	}
	return story.Title
}

// storyDataDisplayTitle extracts the display title persisted for a story from the
// story_data JSON (displayTitle key) when present, falling back to the
// display_title column value. This lets unchanged-detection (#201) compare the
// existing stored display title against the incoming DisplayTitle.
func storyDataDisplayTitle(storyData []byte, columnValue string) string {
	if len(storyData) > 0 {
		var tmp struct {
			DisplayTitle string `json:"displayTitle"`
		}
		if err := json.Unmarshal(storyData, &tmp); err == nil {
			if d := strings.TrimSpace(tmp.DisplayTitle); d != "" {
				return tmp.DisplayTitle
			}
		}
	}
	return columnValue
}

// articleDisplayTitle returns the display title to persist for an article: the
// original feed casing (DisplayTitle) when set, otherwise the normalized Title
// (#201).
func articleDisplayTitle(article domain.Article) string {
	if d := strings.TrimSpace(article.DisplayTitle); d != "" {
		return article.DisplayTitle
	}
	return article.Title
}

// Repository.UpsertArticle delegates to the package-level UpsertArticle function.
func (r *Repository) UpsertArticle(article domain.Article) error {
	return UpsertArticle(r.db, article)
}

// mergeEntities merges existing and new entities (union, deduplicated, sorted).
func mergeEntities(existing, new []string) []string {
	return normalizeEntities(concatArrays(existing, new))
}

// concatArrays concatenates two slices.
func concatArrays(a, b []string) []string {
	result := make([]string, 0, len(a)+len(b))
	result = append(result, a...)
	result = append(result, b...)
	return result
}

// normalizeEntities deduplicates and sorts an entity array.
func normalizeEntities(entities []string) []string {
	if entities == nil {
		return []string{}
	}
	seen := make(map[string]bool)
	result := []string{}

	for _, e := range entities {
		if !seen[e] {
			seen[e] = true
			result = append(result, e)
		}
	}

	sort.Strings(result)
	return result
}

// equalStringArrays compares two string arrays for equality.
func equalStringArrays(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]bool)
	for _, v := range a {
		seen[v] = true
	}
	for _, v := range b {
		if !seen[v] {
			return false
		}
	}
	return true
}

// timestampToString converts time.Time to ISO8601 string (20 chars).
func timestampToString(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

// parseTimestamp parses ISO8601 string to time.Time.
func parseTimestamp(s string) time.Time {
	t, _ := time.Parse("2006-01-02T15:04:05Z", s)
	return t
}

// marshalEntities converts a slice of strings to JSON or returns empty array
func marshalEntities(entities []string) string {
	if entities == nil {
		return "[]"
	}
	b, err := json.Marshal(entities)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// RuleRepository implements domain.CorrectionRuleRepository using a shared
// *sql.DB handle. It works against the same connection as the story repository
// (sqlite-file or turso via libsql) and reuses the 007_correction_rules table.
type RuleRepository struct {
	db *sql.DB
}

// NewRuleRepository wraps an existing *sql.DB handle. Migrations are NOT run
// here; call MigrateDB or NewRepository first so the 007 migration is applied.
func NewRuleRepository(db *sql.DB) *RuleRepository {
	return &RuleRepository{db: db}
}

// SaveRule implements domain.CorrectionRuleRepository.SaveRule.
func (r *RuleRepository) SaveRule(rule domain.CorrectionRule) error {
	if err := rule.Validate(); err != nil {
		return err
	}
	nowStr := timestampToString(time.Now().UTC())
	if rule.CreatedAt == "" {
		rule.CreatedAt = nowStr
	}
	articleIDsJSON := marshalEntities(rule.ArticleIDs)
	_, err := r.db.Exec(`INSERT INTO correction_rules 
		(id, kind, story_id, into_story_id, article_ids, created_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			kind = excluded.kind,
			story_id = excluded.story_id,
			into_story_id = excluded.into_story_id,
			article_ids = excluded.article_ids,
			created_at = excluded.created_at,
			created_by = excluded.created_by`,
		rule.ID, string(rule.Kind), rule.StoryID, rule.IntoStoryID,
		articleIDsJSON, rule.CreatedAt, rule.CreatedBy)
	if err != nil {
		return fmt.Errorf("save rule: %w", err)
	}
	return nil
}

// ListRules implements domain.CorrectionRuleRepository.ListRules in
// deterministic (ID-sorted) order.
func (r *RuleRepository) ListRules() ([]domain.CorrectionRule, error) {
	rows, err := r.db.Query(`SELECT id, kind, story_id, into_story_id, article_ids, created_at, created_by
		FROM correction_rules ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var rules []domain.CorrectionRule
	for rows.Next() {
		var rule domain.CorrectionRule
		var kind, articleIDsJSON string
		if err := rows.Scan(&rule.ID, &kind, &rule.StoryID, &rule.IntoStoryID, &articleIDsJSON, &rule.CreatedAt, &rule.CreatedBy); err != nil {
			return nil, fmt.Errorf("scan rule: %w", err)
		}
		rule.Kind = domain.CorrectionKind(kind)
		if articleIDsJSON != "" {
			_ = json.Unmarshal([]byte(articleIDsJSON), &rule.ArticleIDs)
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}
