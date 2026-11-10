package sqlite

import (
	"fmt"
	"os"
)

// BackupToFile creates a backup of the database to a file.
func BackupToFile(dbPath, backupPath string) error {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fmt.Errorf("database file not found: %s", dbPath)
	}
	// TODO: implement with PRAGMA backup_to_file
	return fmt.Errorf("not implemented")
}

// RestoreFromFile restores a database from a backup file.
func RestoreFromFile(backupPath, dbPath string) error {
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup file not found: %s", backupPath)
	}
	// TODO: implement restore from dump
	return fmt.Errorf("not implemented")
}

// Dump returns the full SQL dump of the database.
func Dump(dbPath string) (string, error) {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return "", fmt.Errorf("database file not found: %s", dbPath)
	}
	// TODO: implement using PRAGMA integrity_check or Turso shell
	return "", fmt.Errorf("not implemented")
}

// DumpToFile writes the SQL dump to a file.
func DumpToFile(dbPath, dumpPath string) error {
	dump, err := Dump(dbPath)
	if err != nil {
		return err
	}
	return os.WriteFile(dumpPath, []byte(dump), 0600)
}
