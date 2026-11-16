package sqlite

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestBackupToFile_Success(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "src.db")

	// Create empty db
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close() //nolint:errcheck
	if _, err := Migrate(dbPath); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	backupPath := filepath.Join(tmpDir, "backup.db")
	err = BackupToFile(dbPath, backupPath)
	if err != nil {
		t.Fatalf("BackupToFile: %v", err)
	}

	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("backup file should exist")
	}
}

func TestBackupToFile_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "src.db")

	// Create db with data
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close() //nolint:errcheck
	if _, err := Migrate(dbPath); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		_, err := db.Exec(`INSERT INTO stories (story_id, story_key, title, summary, published_at, status)
			VALUES (?, ?, ?, ?, ?, ?)`,
			uuid.New().String(), "rt-"+string(rune('A'+i%26))+string(rune('0'+i/26)), "RoundTrip Story "+string(rune('A'+i%26))+string(rune('0'+i/26)), "Round trip summary",
			timestampToString(now), "active")
		if err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}

	backupPath := filepath.Join(tmpDir, "backup.db")
	err = BackupToFile(dbPath, backupPath)
	if err != nil {
		t.Fatalf("BackupToFile: %v", err)
	}

	// Wipe source
	os.Remove(dbPath) //nolint:errcheck

	restorePath := filepath.Join(tmpDir, "restore.db")
	err = RestoreFromFile(backupPath, restorePath)
	if err != nil {
		t.Fatalf("RestoreFromFile: %v", err)
	}

	// Verify restored db
	db2, err := sql.Open("sqlite", restorePath)
	if err != nil {
		t.Fatalf("open restore db: %v", err)
	}
	defer db2.Close() //nolint:errcheck

	var count int
	db2.QueryRow("SELECT COUNT(*) FROM stories").Scan(&count) //nolint:errcheck
	if count != 5 {
		t.Errorf("expected 5 rows restored, got %d", count)
	}
}

func TestRestoreFromFile_NoBackup(t *testing.T) {
	tmpDir := t.TempDir()
	backupPath := filepath.Join(tmpDir, "nonexistent.db")
	dbPath := filepath.Join(tmpDir, "restore.db")

	err := RestoreFromFile(backupPath, dbPath)
	if err == nil {
		t.Error("expected error for missing backup")
	}
}

func TestDumpToFile_Success(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "src.db")

	// Create db
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close() //nolint:errcheck
	if _, err := Migrate(dbPath); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	dumpPath := filepath.Join(tmpDir, "dump.sql")
	err = DumpToFile(dbPath, dumpPath)
	if err != nil {
		t.Fatalf("DumpToFile: %v", err)
	}

	content, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("read dump: %v", err)
	}

	if len(content) == 0 {
		t.Error("dump file should not be empty")
	}

	dumpStr := string(content)
	if !strings.Contains(dumpStr, "BEGIN;") {
		t.Error("dump should start with BEGIN")
	}
	if !strings.Contains(dumpStr, "COMMIT;") {
		t.Error("dump should end with COMMIT")
	}
}

func TestDumpToFile_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "src.db")

	// Create db with data
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close() //nolint:errcheck
	if _, err := Migrate(dbPath); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now().UTC()
	_, err = db.Exec(`INSERT INTO stories (story_id, story_key, title, summary, published_at, status)
		VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), "dump-rt-1", "Dump RoundTrip 1", "Dump rt summary",
		timestampToString(now), "active")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	dumpPath := filepath.Join(tmpDir, "dump.sql")
	err = DumpToFile(dbPath, dumpPath)
	if err != nil {
		t.Fatalf("DumpToFile: %v", err)
	}

	// Restore
	restorePath := filepath.Join(tmpDir, "restore.db")
	db2, err := sql.Open("sqlite", restorePath)
	if err != nil {
		t.Fatalf("open restore db: %v", err)
	}
	defer db2.Close() //nolint:errcheck

	if _, err := Migrate(restorePath); err != nil {
		t.Fatalf("migrate on restore: %v", err)
	}

	dumpSQL, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("read dump: %v", err)
	}

	if _, err := db2.Exec(string(dumpSQL)); err != nil {
		t.Fatalf("exec dump: %v", err)
	}

	var count int64
	db2.QueryRow("SELECT COUNT(*) FROM stories").Scan(&count) //nolint:errcheck
	if count != 1 {
		t.Errorf("expected 1 row after restore, got %d", count)
	}
}

func TestDump_FileNotExists(t *testing.T) {
	_, err := Dump("/nonexistent/path/db.db")
	if err == nil {
		t.Error("expected error for non-existent db")
	}
}

func TestDumpToFile_FileNotExists(t *testing.T) {
	err := DumpToFile("/nonexistent/path/db.db", "/tmp/dump.sql")
	if err == nil {
		t.Error("expected error for non-existent db")
	}
}

func TestBackupToFile_FileNotExists(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "nonexistent.db")
	backupPath := filepath.Join(tmpDir, "backup.db")

	err := BackupToFile(dbPath, backupPath)
	if err == nil {
		t.Error("expected error for non-existent db")
	}
}

func TestBackupToFile_EmptyDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "empty.db")

	// Create empty db
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close() //nolint:errcheck
	if _, err := Migrate(dbPath); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	backupPath := filepath.Join(tmpDir, "backup.db")
	err = BackupToFile(dbPath, backupPath)
	if err != nil {
		t.Fatalf("BackupToFile on empty db: %v", err)
	}

	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("backup file should exist")
	}
}

func TestRestoreFromFile_EmptyBackup(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "empty.db")

	// Create empty db
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close() //nolint:errcheck
	if _, err := Migrate(dbPath); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	backupPath := filepath.Join(tmpDir, "backup.db")
	err = BackupToFile(dbPath, backupPath)
	if err != nil {
		t.Fatalf("BackupToFile: %v", err)
	}

	newDB := filepath.Join(tmpDir, "restore.db")
	if _, err := Migrate(newDB); err != nil {
		t.Fatalf("migrate on restore: %v", err)
	}
	err = RestoreFromFile(backupPath, newDB)
	if err != nil {
		t.Fatalf("RestoreFromFile: %v", err)
	}

	db2, err := sql.Open("sqlite", newDB)
	if err != nil {
		t.Fatalf("open restore db: %v", err)
	}
	defer db2.Close() //nolint:errcheck

	var count int
	db2.QueryRow("SELECT COUNT(*) FROM stories").Scan(&count) //nolint:errcheck
	if count != 0 {
		t.Errorf("expected 0 stories after restore, got %d", count)
	}
}
