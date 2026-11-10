package sqlite

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Migrate applies forward-only migrations from the embedded filesystem.
// It returns the list of applied migration numbers (in order).
// This function is currently a placeholder; full implementation requires
// a database connection and proper transaction handling.
func Migrate(dbPath string) ([]int, error) {
	// TODO: implement with modernc.org/sqlite and transaction control
	return nil, fmt.Errorf("not implemented")
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
