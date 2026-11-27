package sqlite

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
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

// TestArchiveOldDB_WindowBoundary_RefreshedRowNotDeleted (issue #252 / AC-3):
// a row refreshed (published_at -> now) between export and apply no longer
// matches the window and must NOT be deleted; a genuinely old row is
// archived before its delete (both tables consistent in the same pass).
func TestArchiveOldDB_WindowBoundary_RefreshedRowNotDeleted(t *testing.T) {
	db := newRetentionTestDB(t)
	old := oldDateStr(t, 120)
	if _, err := seedStory(db, "old-s", "Old story", old); err != nil {
		t.Fatalf("seed old: %v", err)
	}
	refreshedID, err := seedStory(db, "refreshed-s", "Refreshed story", old)
	if err != nil {
		t.Fatalf("seed refreshed: %v", err)
	}

	// Simulate a refresh (e.g., by a concurrent ingest/export) that moves the
	// row back into the window before apply runs.
	if _, err := db.Exec(`UPDATE stories SET published_at = ? WHERE story_id = ?`,
		timestampToString(time.Now().UTC()), refreshedID); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	counts, err := ArchiveOldDB(db, RetentionWindow, ArchiveBatchLimit)
	if err != nil {
		t.Fatalf("ArchiveOldDB: %v", err)
	}
	if counts["stories"] != 1 {
		t.Errorf("stories archived: want 1 (only the genuinely old row), got %d", counts["stories"])
	}
	// The refreshed row survives in hot; the old one is in archive.
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM stories WHERE story_key = 'refreshed-s'`).Scan(&n); err != nil {
		t.Fatalf("count refreshed: %v", err)
	}
	if n != 1 {
		t.Errorf("refreshed story must survive (exit the window), hot stories for refreshed-s: want 1, got %d", n)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM stories WHERE story_key = 'old-s'`).Scan(&n); err != nil {
		t.Fatalf("count old: %v", err)
	}
	if n != 0 {
		t.Errorf("old story must leave hot after apply, got %d", n)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM stories_archive WHERE story_key = 'old-s'`).Scan(&n); err != nil {
		t.Fatalf("count archive: %v", err)
	}
	if n != 1 {
		t.Errorf("old story must be in archive before/with its delete, got %d", n)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM stories_archive WHERE story_key = 'refreshed-s'`).Scan(&n); err != nil {
		t.Fatalf("count refreshed archive: %v", err)
	}
	if n != 0 {
		t.Errorf("refreshed story must NOT be archived, got %d", n)
	}
}

// TestArchiveOldDB_ArchiveBeforeDeleteInvariant (issue #252 / AC-3):
// every deleted hot row is present in its *_archive mirror afterwards, and
// hot + archive counts are complementary (no row is lost or duplicated).
func TestArchiveOldDB_ArchiveBeforeDeleteInvariant(t *testing.T) {
	db := newRetentionTestDB(t)
	// Seed 3 old + 2 fresh stories, 2 old + 1 fresh articles, 1 old + 1 fresh snapshot.
	for i, key := range []string{"o1", "o2", "o3"} {
		if _, err := seedStory(db, key, "Old "+key, oldDateStr(t, 100+i*10)); err != nil {
			t.Fatalf("seed story %s: %v", key, err)
		}
	}
	for _, key := range []string{"f1", "f2"} {
		if _, err := seedStory(db, key, "Fresh "+key, timestampToString(time.Now().UTC())); err != nil {
			t.Fatalf("seed fresh story %s: %v", key, err)
		}
	}
	if err := seedArticle(db, "a123456789abcdef", "src-a", "Old A", oldDateStr(t, 100)); err != nil {
		t.Fatalf("seed article: %v", err)
	}
	if err := seedArticle(db, "b123456789abcdef", "src-b", "Old B", oldDateStr(t, 110)); err != nil {
		t.Fatalf("seed article: %v", err)
	}
	if err := seedArticle(db, "c123456789abcdef", "src-c", "Fresh C", timestampToString(time.Now().UTC())); err != nil {
		t.Fatalf("seed fresh article: %v", err)
	}
	if err := seedSnapshot(db, oldDateStr(t, 100)[:10], "nyg", 1); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	if err := seedSnapshot(db, time.Now().UTC().Format("2006-01-02"), "nyg", 2); err != nil {
		t.Fatalf("seed fresh snapshot: %v", err)
	}

	counts, err := ArchiveOldDB(db, RetentionWindow, ArchiveBatchLimit)
	if err != nil {
		t.Fatalf("ArchiveOldDB: %v", err)
	}
	if counts["stories"] != 3 || counts["articles"] != 2 || counts["ranking_snapshots"] != 1 {
		t.Errorf("archived counts wrong: %v", counts)
	}
	// Invariant: hot + archive == total seeded, for each table.
	want := map[string]int{"stories": 5, "articles": 3, "ranking_snapshots": 2}
	for table, total := range want {
		var hot, arc int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&hot); err != nil {
			t.Fatalf("count hot %s: %v", table, err)
		}
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table + `_archive`).Scan(&arc); err != nil {
			t.Fatalf("count archive %s: %v", table, err)
		}
		if hot+arc != total {
			t.Errorf("%s: hot(%d)+archive(%d) = %d, want %d (no loss, no duplication)", table, hot, arc, hot+arc, total)
		}
	}
}

// TestExportApplyImportRoundTrip (issue #252 / AC-4): representative dataset
// round-trip — seed in-window and expired rows per hot table, export, apply,
// then import the package into a FRESH database and verify row counts match.
// Re-apply archives 0 (idempotent); double import duplicates nothing.
func TestExportApplyImportRoundTrip(t *testing.T) {
	db := newRetentionTestDB(t)
	old := oldDateStr(t, 120)
	fresh := timestampToString(time.Now().UTC())

	// Representative seed: 2 expired + 1 fresh per hot table.
	for _, key := range []string{"old-a", "old-b"} {
		if _, err := seedStory(db, key, "Story "+key, old); err != nil {
			t.Fatalf("seed story: %v", err)
		}
	}
	if _, err := seedStory(db, "new-c", "Story new-c", fresh); err != nil {
		t.Fatalf("seed story: %v", err)
	}
	if err := seedArticle(db, "a123456789abcdef", "src-a", "Old article", old); err != nil {
		t.Fatalf("seed article: %v", err)
	}
	if err := seedArticle(db, "b123456789abcdef", "src-b", "Old article 2", oldDateStr(t, 110)); err != nil {
		t.Fatalf("seed article: %v", err)
	}
	if err := seedArticle(db, "c123456789abcdef", "src-c", "Fresh article", fresh); err != nil {
		t.Fatalf("seed article: %v", err)
	}
	if err := seedSnapshot(db, "2025-01-01", "nyg", 1); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	if err := seedSnapshot(db, time.Now().UTC().Format("2006-01-02"), "nyg", 2); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	// 1) Export: captures the expired rows only (2 stories, 2 articles, 1 snapshot).
	outPath := filepath.Join(t.TempDir(), "pkg.tar.gz")
	rep, err := exportForTest(db, outPath, domain.NewRetentionPolicy())
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if rep.Rows["stories"] != 2 || rep.Rows["articles"] != 2 || rep.Rows["ranking_snapshots"] != 1 {
		t.Errorf("export rows: %v (want stories=2 articles=2 ranking_snapshots=1)", rep.Rows)
	}
	if rep.TotalRows != 5 {
		t.Errorf("export total: want 5, got %d", rep.TotalRows)
	}

	// 2) Apply: archive the expired rows; fresh rows stay hot.
	counts, err := ArchiveOldDB(db, RetentionWindow, ArchiveBatchLimit)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if counts["stories"] != 2 || counts["articles"] != 2 || counts["ranking_snapshots"] != 1 {
		t.Errorf("archived: %v", counts)
	}
	if n := countTable(t, db, "stories"); n != 1 {
		t.Errorf("hot stories after apply: want 1 (fresh), got %d", n)
	}
	if n := countTable(t, db, "stories_archive"); n != 2 {
		t.Errorf("stories_archive after apply: want 2, got %d", n)
	}

	// 3) Import the package into a FRESH database; row counts must match.
	freshDB := newRetentionTestDB(t)
	rep2, err := importForTest(freshDB, outPath)
	if err != nil {
		t.Fatalf("import into fresh db: %v", err)
	}
	if rep2.TotalRows != 5 {
		t.Errorf("imported rows: want 5, got %d", rep2.TotalRows)
	}
	if n := countTable(t, freshDB, "stories"); n != 2 {
		t.Errorf("fresh-db stories: want 2, got %d", n)
	}
	if n := countTable(t, freshDB, "articles"); n != 2 {
		t.Errorf("fresh-db articles: want 2, got %d", n)
	}
	if n := countTable(t, freshDB, "ranking_snapshots"); n != 1 {
		t.Errorf("fresh-db snapshots: want 1, got %d", n)
	}

	// 4) Re-apply on the original db: idempotent (archives 0).
	counts2, err := ArchiveOldDB(db, RetentionWindow, ArchiveBatchLimit)
	if err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	for table, n := range counts2 {
		if n != 0 {
			t.Errorf("re-apply %s: want 0 archived, got %d", table, n)
		}
	}

	// 5) Double import on the fresh db: duplicates nothing.
	if _, err := importForTest(freshDB, outPath); err != nil {
		t.Fatalf("double import: %v", err)
	}
	if n := countTable(t, freshDB, "stories"); n != 2 {
		t.Errorf("fresh-db stories after double import: want 2, got %d", n)
	}
	if n := countTable(t, freshDB, "articles"); n != 2 {
		t.Errorf("fresh-db articles after double import: want 2, got %d", n)
	}
}
