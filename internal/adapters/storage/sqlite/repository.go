package sqlite

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
	"github.com/google/uuid"
)

// NewRepository opens or creates a database and returns a Repository.
// It does NOT run Migrate(); call Migrate() separately.
func NewRepository(dbPath string) (*Repository, error) {
	if dbPath == "" {
		dbPath = ":memory:"
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	// Enable foreign keys per connection
	db.Exec("PRAGMA foreign_keys = ON")
	return &Repository{db: db}, nil
}

// Repository implements domain.Repository using modernc.org/sqlite.
type Repository struct {
	db *sql.DB
}

// Upsert implements domain.Repository.Upsert.
func (r *Repository) Upsert(story domain.Story) (domain.UpsertStatus, error) {
	// Check by story_key (story.ID)
	var existingID string
	err := r.db.QueryRow("SELECT story_id FROM stories WHERE story_key = ?", story.ID).Scan(&existingID)
	if err == nil {
		// Found by story_key, check meaningful fields
		var title, summary, canonicalURL, publishedAt, guid string
		var storyData []byte
		err = r.db.QueryRow("SELECT title, summary, canonical_url, published_at, story_data FROM stories WHERE story_key = ?", story.ID).Scan(&title, &summary, &canonicalURL, &publishedAt, &storyData)
		if err != nil {
			return domain.UpsertUnchanged, fmt.Errorf("read story: %w", err)
		}
		var other domain.Story
		if err := other.UnmarshalJSON(storyData); err != nil {
			// Fallback to denorm fields
			other = domain.Story{
				Title:        title,
				Summary:      summary,
				CanonicalURL: canonicalURL,
				PublishedAt:  parseTimestamp(publishedAt),
				GUID:         guid,
			}
		}
		if !other.MeaningfulFieldsChanged(story) {
			return domain.UpsertUnchanged, nil
		}
		// Update
		data, _ := story.MarshalJSON()
		result, err := r.db.Exec(`UPDATE stories 
			SET title = ?, summary = ?, canonical_url = ?, published_at = ?, 
				story_data = ?, updated_at = datetime('now')
			WHERE story_key = ?`, story.Title, story.Summary, story.CanonicalURL, 
			timestampToString(story.PublishedAt), string(data), story.ID)
		if err != nil {
			return domain.UpsertUnchanged, fmt.Errorf("update: %w", err)
		}
		affected, _ := result.RowsAffected()
		if affected > 0 {
			return domain.UpsertUpdated, nil
		}
		return domain.UpsertUnchanged, nil
	}

	// Not found, try dedupe_key
	var dedupeFound bool
	err = r.db.QueryRow("SELECT 1 FROM stories WHERE dedupe_key = ? LIMIT 1", story.DedupeKey).Scan(&dedupeFound)
	if err == nil && dedupeFound {
		// Dedupe hit: update existing
		result, err := r.db.Exec(`UPDATE stories 
			SET title = ?, summary = ?, canonical_url = ?, published_at = ?, 
				updated_at = datetime('now')
			WHERE dedupe_key = ?`, story.Title, story.Summary, story.CanonicalURL,
			timestampToString(story.PublishedAt), story.DedupeKey)
		if err != nil {
			return domain.UpsertUnchanged, fmt.Errorf("dedupe update: %w", err)
		}
		affected, _ := result.RowsAffected()
		if affected > 0 {
			return domain.UpsertUpdated, nil
		}
	}

	// Insert new story
	data, _ := story.MarshalJSON()
	uuid := uuid.New().String()
	result, err := r.db.Exec(`INSERT INTO stories 
		(story_id, story_key, source_id, article_id, title, summary, published_at, 
		 canonical_url, guid, story_data, dedupe_key, status, created_at, first_seen_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'active', datetime('now'), datetime('now'), datetime('now'))`,
		uuid, story.ID, story.Source, story.ID, story.Title, story.Summary,
		timestampToString(story.PublishedAt), story.CanonicalURL, story.GUID,
		string(data), story.DedupeKey)
	if err != nil {
		return domain.UpsertUnchanged, fmt.Errorf("insert: %w", err)
	}
	_, _ = result.LastInsertId()
	return domain.UpsertCreated, nil
}

// List implements domain.Repository.List.
func (r *Repository) List() ([]domain.Story, error) {
	rows, err := r.db.Query(`SELECT story_data FROM stories ORDER BY published_at DESC, story_id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	defer rows.Close()
	var stories []domain.Story
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		var s domain.Story
		if err := s.UnmarshalJSON(data); err != nil {
			return nil, fmt.Errorf("unmarshal: %w", err)
		}
		stories = append(stories, s)
	}
	return stories, rows.Err()
}

// Count implements domain.Repository.Count.
func (r *Repository) Count() (int, error) {
	var count int
	err := r.db.QueryRow("SELECT COUNT(*) FROM stories").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count: %w", err)
	}
	return count, nil
}

// Migrate applies schema migrations.
func (r *Repository) Migrate() error {
	_, err := Migrate("")
	return err
}

// Close closes the database connection.
func (r *Repository) Close() error {
	return r.db.Close()
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