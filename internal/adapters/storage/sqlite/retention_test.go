package sqlite

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestArchiveOld_Success(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create database with migrations
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close() //nolint:errcheck
	if _, err := Migrate(dbPath); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Seed rows
	now := time.Now().UTC()
	oldDate := now.AddDate(0, 0, -120).Format("2006-01-02T15:04:05Z")
	freshDate := now.Format("2006-01-02T15:04:05Z")

	_, err = db.Exec(`INSERT INTO stories (story_id, story_key, title, summary, published_at, status)
		VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), "old-1", "Old Story 1", "Old summary",
		oldDate, "active")
	if err != nil {
		t.Fatalf("insert old story: %v", err)
	}

	_, err = db.Exec(`INSERT INTO stories (story_id, story_key, title, summary, published_at, status)
		VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), "fresh-1", "Fresh Story 1", "Fresh summary",
		freshDate, "active")
	if err != nil {
		t.Fatalf("insert fresh story: %v", err)
	}

	// Verify pre-archive counts
	var count int
	db.QueryRow("SELECT COUNT(*) FROM stories").Scan(&count) //nolint:errcheck
	if count != 2 {
		t.Errorf("expected 2 stories pre-archive, got %d", count)
	}

	// Run archive
	counts, err := ArchiveOld(dbPath, RetentionWindow)
	if err != nil {
		t.Fatalf("ArchiveOld: %v", err)
	}
	if counts["stories"] != 1 {
		t.Errorf("expected 1 story archived, got %d", counts["stories"])
	}

	// Verify hot table has only fresh row
	db.QueryRow("SELECT COUNT(*) FROM stories").Scan(&count) //nolint:errcheck
	if count != 1 {
		t.Errorf("expected 1 story post-archive, got %d", count)
	}

	// Verify archived row exists
	var archivedID string
	db.QueryRow(`SELECT story_id FROM stories_archive WHERE story_key = ?`, "old-1").Scan(&archivedID) //nolint:errcheck
	if archivedID == "" {
		t.Error("expected archived row")
	}
}

func TestArchiveOld_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create database with migrations
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close() //nolint:errcheck
	if _, err := Migrate(dbPath); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Seed one old row
	now := time.Now().UTC()
	oldDate := now.AddDate(0, 0, -120).Format("2006-01-02T15:04:05Z")
	_, err = db.Exec(`INSERT INTO stories (story_id, story_key, title, summary, published_at, status)
		VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), "old-1", "Old Story 1", "Old summary",
		oldDate, "active")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// First run
	counts1, err := ArchiveOld(dbPath, RetentionWindow)
	if err != nil {
		t.Fatalf("ArchiveOld #1: %v", err)
	}
	if counts1["stories"] != 1 {
		t.Errorf("first run: expected 1 archived, got %d", counts1["stories"])
	}

	// Second run (should archive 0 rows since no old rows remain)
	counts2, err := ArchiveOld(dbPath, RetentionWindow)
	if err != nil {
		t.Fatalf("ArchiveOld #2: %v", err)
	}
	if counts2["stories"] != 0 {
		t.Errorf("second run: expected 0 archived (idempotent), got %d", counts2["stories"])
	}
}

func TestArchiveOld_EmptyDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Empty db, still apply migrations
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close() //nolint:errcheck
	if _, err := Migrate(dbPath); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	counts, err := ArchiveOld(dbPath, RetentionWindow)
	if err != nil {
		t.Fatalf("ArchiveOld on empty db: %v", err)
	}
	// Should return 0 for all tables
	for table, count := range counts {
		if count != 0 {
			t.Errorf("table %s: expected 0 archived, got %d", table, count)
		}
	}
}

func TestStorageEstimate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create database with migrations and seed data
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close() //nolint:errcheck
	if _, err := Migrate(dbPath); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now().UTC()
	for i := 0; i < 100; i++ {
		_, err := db.Exec(`INSERT INTO stories (story_id, story_key, title, summary, published_at, status)
			VALUES (?, ?, ?, ?, ?, ?)`,
			uuid.New().String(), "est-"+string(rune('A'+i%26))+string(rune('0'+i/26)), "Est Story "+string(rune('A'+i%26))+string(rune('0'+i/26)), "Est summary",
			timestampToString(now), "active")
		if err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}

	size, err := StorageEstimate(dbPath)
	if err != nil {
		t.Fatalf("StorageEstimate: %v", err)
	}
	if size <= 0 {
		t.Errorf("expected positive size, got %d", size)
	}
}

func TestApplyRetention_Success(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create database with migrations and seed data
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close() //nolint:errcheck
	if _, err := Migrate(dbPath); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now().UTC()
	oldDate := now.AddDate(0, 0, -120).Format("2006-01-02T15:04:05Z")
	_, err = db.Exec(`INSERT INTO stories (story_id, story_key, title, summary, published_at, status)
		VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), "old-1", "Old Story 1", "Old summary",
		oldDate, "active")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	budgetOK, err := ApplyRetention(dbPath)
	if err != nil {
		t.Fatalf("ApplyRetention: %v", err)
	}
	if !budgetOK {
		t.Error("expected budget OK after archiving (db small)")
	}

	// Verify hot row was archived
	var hotCount int
	db.QueryRow("SELECT COUNT(*) FROM stories").Scan(&hotCount) //nolint:errcheck
	if hotCount != 0 {
		t.Errorf("expected 0 hot rows, got %d", hotCount)
	}
}

func TestApplyRetention_DiskBudgetExceeded(t *testing.T) {
	policy := NewStoragePolicy()
	policy.FreeBytes = 1 // 1 byte -> definitely exceeded
	if policy.SizeBudgetOK(1_000_000_001) {
		t.Error("SizeBudgetOK should return false when exceeded")
	}
}

func TestArchiveOld_FileNotExists(t *testing.T) {
	_, err := ArchiveOld("/nonexistent/path/db.db", RetentionWindow)
	if err == nil {
		t.Error("expected error for non-existent db")
	}
}

func TestStorageEstimate_FileNotExists(t *testing.T) {
	_, err := StorageEstimate("/nonexistent/path/db.db")
	if err == nil {
		t.Error("expected error for non-existent db")
	}
}

func TestApplyRetention_FileNotExists(t *testing.T) {
	_, err := ApplyRetention("/nonexistent/path/db.db")
	if err == nil {
		t.Error("expected error for non-existent db")
	}
}
