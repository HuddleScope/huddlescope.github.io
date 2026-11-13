package sqlite

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

const schemaMigrationsTable = "schema_migrations"

// Migrate applies forward-only migrations from the embedded filesystem.
// It returns the list of applied migration numbers (in order).
// This is idempotent: running Migrate multiple times has the same effect.
func Migrate(dbPath string) ([]int, error) {
	if dbPath == "" {
		dbPath = ":memory:"
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	// Enable foreign keys per connection
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	// Create schema_migrations table if not exists
	if _, err := db.Exec(fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (strftime('%%Y-%%m-%%dT%%H:%%M:%%SZ','now'))
		)`, schemaMigrationsTable)); err != nil {
		return nil, fmt.Errorf("create schema_migrations: %w", err)
	}

	// Get applied migrations
	applied, err := getAppliedMigrations(db)
	if err != nil {
		return nil, err
	}

	// Get available migrations
	available, err := ListMigrations()
	if err != nil {
		return nil, err
	}

	var appliedVersions []int
	var txn *sql.Tx
	txn, err = db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin txn: %w", err)
	}
	defer func() {
		if txn != nil {
			_ = txn.Rollback()
		}
	}()

	for _, m := range available {
		ver, err := parseMigrationVersion(m)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", m, err)
		}

		if _, ok := applied[ver]; ok {
			continue
		}

		sqlBytes, err := ReadMigration(m)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", m, err)
		}
		sqlStr := string(sqlBytes)

		if _, err := txn.Exec(sqlStr); err != nil {
			return nil, fmt.Errorf("exec %s: %w", m, err)
		}

		if _, err := txn.Exec(fmt.Sprintf("INSERT INTO %s (version) VALUES (?)", schemaMigrationsTable), ver); err != nil {
			return nil, fmt.Errorf("record %s: %w", m, err)
		}

		appliedVersions = append(appliedVersions, ver)
	}

	if err := txn.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	txn = nil

	return appliedVersions, nil
}

func getAppliedMigrations(db *sql.DB) (map[int]bool, error) {
	rows, err := db.Query(fmt.Sprintf("SELECT version FROM %s ORDER BY version", schemaMigrationsTable))
	if err != nil {
		return nil, fmt.Errorf("query schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var ver int
		if err := rows.Scan(&ver); err != nil {
			return nil, fmt.Errorf("scan version: %w", err)
		}
		applied[ver] = true
	}
	return applied, rows.Err()
}

func parseMigrationVersion(filename string) (int, error) {
	name := strings.TrimSuffix(filename, ".sql")
	parts := strings.Split(name, "_")
	if len(parts) < 1 {
		return 0, fmt.Errorf("invalid filename format: %s", filename)
	}
	numStr := strings.TrimLeft(parts[0], "0")
	if numStr == "" {
		numStr = "0"
	}
	var ver int
	_, err := fmt.Sscanf(numStr, "%d", &ver)
	if err != nil {
		return 0, fmt.Errorf("parse version from %s: %w", filename, err)
	}
	return ver, nil
}

// Migration represents a single migration file.
type Migration struct {
	Number int
	SQL    string
}

// ListMigrations returns the list of available migration file names.
func ListMigrations() ([]string, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// ReadMigration returns the SQL content of a specific migration file.
func ReadMigration(filename string) ([]byte, error) {
	return migrationFS.ReadFile(filepath.Join("migrations", filename))
}
