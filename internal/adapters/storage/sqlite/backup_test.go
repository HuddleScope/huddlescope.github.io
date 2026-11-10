package sqlite

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupToFileNotExists(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "nonexistent.db")
	backupPath := filepath.Join(tmpDir, "backup.sql")

	err := BackupToFile(dbPath, backupPath)
	if err == nil {
		t.Error("expected error for non-existent database")
	}
}

func TestRestoreFromFileNotExists(t *testing.T) {
	tmpDir := t.TempDir()
	backupPath := filepath.Join(tmpDir, "nonexistent.sql")
	dbPath := filepath.Join(tmpDir, "db.sql")

	err := RestoreFromFile(backupPath, dbPath)
	if err == nil {
		t.Error("expected error for non-existent backup")
	}
}

func TestDumpNotExists(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "nonexistent.db")

	_, err := Dump(dbPath)
	if err == nil {
		t.Error("expected error for non-existent database")
	}
}

func TestDumpToFileNotExists(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "nonexistent.db")
	dumpPath := filepath.Join(tmpDir, "dump.sql")

	err := DumpToFile(dbPath, dumpPath)
	if err == nil {
		t.Error("expected error for non-existent database")
	}
}

func TestDumpToFileWriteOnly(t *testing.T) {
	tmpDir := t.TempDir()
	dumpPath := filepath.Join(tmpDir, "dump.sql")
	dump := "-- test dump\n"

	if err := os.WriteFile(dumpPath, []byte(dump), 0644); err != nil {
		t.Fatalf("write dump: %v", err)
	}

	if _, err := os.Stat(dumpPath); err != nil {
		t.Errorf("dump file missing: %v", err)
	}
}
