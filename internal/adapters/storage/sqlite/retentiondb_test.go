package sqlite

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

// newRetentionTestDB opens a temp-file sqlite database with migrations
// applied and returns the handle (closed by the caller).
func newRetentionTestDB(t *testing.T) *sql.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := MigrateDB(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func oldDateStr(t *testing.T, daysAgo int) string {
	t.Helper()
	return timestampToString(time.Now().UTC().AddDate(0, 0, -daysAgo))
}

func seedStory(db *sql.DB, key, title, pubAt string) (string, error) {
	id := uuid.New().String()
	_, err := db.Exec(`INSERT INTO stories (story_id, story_key, title, summary, published_at, status)
		VALUES (?, ?, ?, ?, ?, ?)`, id, key, title, "summary", pubAt, "active")
	return id, err
}

func seedArticle(db *sql.DB, articleID, sourceID, title, pubAt string) error {
	_, err := db.Exec(`INSERT INTO articles (article_id, source_id, url, title, published_at)
		VALUES (?, ?, ?, ?, ?)`, articleID, sourceID, "https://example.com/"+articleID, title, pubAt)
	return err
}

func seedSnapshot(db *sql.DB, rankDate, teamID string, rank int) error {
	_, err := db.Exec(`INSERT INTO ranking_snapshots (snapshot_id, rank_date, team_id, rank, metric, value)
		VALUES (?, ?, ?, ?, 'win_rate', 0.5)`, uuid.New().String(), rankDate, teamID, rank)
	return err
}

func seedSnapshotFor(db *sql.DB, rankDate, teamID string) error {
	_, err := db.Exec(`INSERT INTO ranking_snapshots (snapshot_id, rank_date, team_id, rank, metric, value)
		VALUES (?, ?, ?, 1, 'win_rate', 0.5)`, uuid.New().String(), rankDate, teamID)
	return err
}

func countTable(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestArchiveOldDB_AllTables(t *testing.T) {
	db := newRetentionTestDB(t)
	old := oldDateStr(t, 120)
	fresh := timestampToString(time.Now().UTC())

	if _, err := seedStory(db, "old-s", "Old story", old); err != nil {
		t.Fatalf("seed story: %v", err)
	}
	if _, err := seedStory(db, "fresh-s", "Fresh story", fresh); err != nil {
		t.Fatalf("seed story: %v", err)
	}
	if err := seedArticle(db, "a123456789abcdef", "src-a", "Old article", old); err != nil {
		t.Fatalf("seed article: %v", err)
	}
	if err := seedArticle(db, "b123456789abcdef", "src-b", "Fresh article", fresh); err != nil {
		t.Fatalf("seed fresh article: %v", err)
	}
	if err := seedSnapshot(db, "2026-01-01", "nyg", 1); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	counts, err := ArchiveOldDB(db, RetentionWindow, ArchiveBatchLimit)
	if err != nil {
		t.Fatalf("ArchiveOldDB: %v", err)
	}
	if counts["stories"] != 1 {
		t.Errorf("stories archived: want 1, got %d", counts["stories"])
	}
	if counts["articles"] != 1 {
		t.Errorf("articles archived: want 1, got %d", counts["articles"])
	}
	if counts["ranking_snapshots"] != 1 {
		t.Errorf("snapshots archived: want 1, got %d", counts["ranking_snapshots"])
	}

	if n := countTable(t, db, "stories"); n != 1 {
		t.Errorf("hot stories: want 1, got %d", n)
	}
	if n := countTable(t, db, "articles"); n != 1 {
		t.Errorf("hot articles: want 1, got %d", n)
	}
	if n := countTable(t, db, "ranking_snapshots"); n != 0 {
		t.Errorf("hot snapshots: want 0, got %d", n)
	}
	if n := countTable(t, db, "stories_archive"); n != 1 {
		t.Errorf("stories_archive: want 1, got %d", n)
	}
	if n := countTable(t, db, "articles_archive"); n != 1 {
		t.Errorf("articles_archive: want 1, got %d", n)
	}
	if n := countTable(t, db, "ranking_snapshots_archive"); n != 1 {
		t.Errorf("ranking_snapshots_archive: want 1, got %d", n)
	}

	// Second run: idempotent, archives 0
	counts2, err := ArchiveOldDB(db, RetentionWindow, ArchiveBatchLimit)
	if err != nil {
		t.Fatalf("ArchiveOldDB #2: %v", err)
	}
	for table, n := range counts2 {
		if n != 0 {
			t.Errorf("second run %s: want 0 archived, got %d", table, n)
		}
	}
}

func TestArchiveOldDB_BoundedBatches(t *testing.T) {
	db := newRetentionTestDB(t)
	const total = 1200 // > 2 batches at the default 500 limit

	for i := 0; i < total; i++ {
		// Spread across distinct rank_dates/teams so the UNIQUE
		// (rank_date, metric, team_id) constraint holds.
		day := 200 + i%60 // 200..259 days ago
		date := time.Now().UTC().AddDate(0, 0, -day).Format("2006-01-02")
		if err := seedSnapshotFor(db, date, fmt.Sprintf("team-%d", i)); err != nil {
			t.Fatalf("seed snapshot %d: %v", i, err)
		}
	}
	if n := countTable(t, db, "ranking_snapshots"); n != total {
		t.Fatalf("seeded snapshots: want %d, got %d", total, n)
	}

	counts, err := ArchiveOldDB(db, RetentionWindow, ArchiveBatchLimit)
	if err != nil {
		t.Fatalf("ArchiveOldDB: %v", err)
	}
	if counts["ranking_snapshots"] != int64(total) {
		t.Errorf("archived: want %d, got %d", total, counts["ranking_snapshots"])
	}
	if n := countTable(t, db, "ranking_snapshots"); n != 0 {
		t.Errorf("hot snapshots: want 0, got %d", n)
	}
	if n := countTable(t, db, "ranking_snapshots_archive"); n != total {
		t.Errorf("archive snapshots: want %d, got %d", total, n)
	}
}

func TestStorageEstimateDB(t *testing.T) {
	db := newRetentionTestDB(t)
	size, err := StorageEstimateDB(db)
	if err != nil {
		t.Fatalf("StorageEstimateDB: %v", err)
	}
	if size <= 0 {
		t.Errorf("expected positive size, got %d", size)
	}
}

func TestSchemaVersionDB(t *testing.T) {
	db := newRetentionTestDB(t)
	ver, err := SchemaVersionDB(db)
	if err != nil {
		t.Fatalf("SchemaVersionDB: %v", err)
	}
	if ver != 12 {
		t.Errorf("schema version: want 12, got %d", ver)
	}
}

func TestApplyRetentionDB(t *testing.T) {
	db := newRetentionTestDB(t)
	if _, err := seedStory(db, "old-s", "Old story", oldDateStr(t, 120)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	ok, err := ApplyRetentionDB(db)
	if err != nil {
		t.Fatalf("ApplyRetentionDB: %v", err)
	}
	if !ok {
		t.Error("budget should be OK for a tiny db")
	}
	if n := countTable(t, db, "stories"); n != 0 {
		t.Errorf("hot stories after apply: want 0, got %d", n)
	}
}

func TestApplyRetentionPolicyDB_BudgetExceeded(t *testing.T) {
	db := newRetentionTestDB(t)
	policy := NewStoragePolicy()
	policy.FreeBytes = 1 // 1 byte -> any real db exceeds it
	ok, err := applyRetentionPolicyDB(db, policy)
	if err != nil {
		t.Fatalf("applyRetentionPolicyDB: %v", err)
	}
	if ok {
		t.Error("budget should be exceeded with FreeBytes=1")
	}
}
