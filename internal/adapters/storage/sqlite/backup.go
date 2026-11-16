package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
)

// BackupToFile creates a backup of the database to a file.
// For backup, we use a two-step approach: close source, copy file, reopen source.
// This provides a consistent snapshot (assuming no other writers).
func BackupToFile(dbPath, backupPath string) error {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fmt.Errorf("database file not found: %s", dbPath)
	}

	// Verify db is valid by opening and running a simple query
	srcDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open source db: %w", err)
	}
	var dummy int
	if err := srcDB.QueryRow("SELECT 1").Scan(&dummy); err != nil {
		_ = srcDB.Close()
		return fmt.Errorf("validate source db: %w", err)
	}
	_ = srcDB.Close()

	// Copy the file (SQLite file copy provides a consistent snapshot if db is closed)
	if err := copyFile(dbPath, backupPath); err != nil {
		return fmt.Errorf("copy file: %w", err)
	}

	// Verify backup was created
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup file was not created")
	}

	return nil
}

func copyFile(src, dst string) error {
	srcData, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}

	if err := os.WriteFile(dst, srcData, 0644); err != nil {
		return fmt.Errorf("write destination: %w", err)
	}

	return nil
}

// RestoreFromFile restores a database from a backup file.
func RestoreFromFile(backupPath, dbPath string) error {
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup file not found: %s", backupPath)
	}

	// Copy backup to db path (overwrites existing)
	if err := copyFile(backupPath, dbPath); err != nil {
		return fmt.Errorf("copy backup to db: %w", err)
	}

	// Open and run migrations
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	if _, err := Migrate(dbPath); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	// Verify integrity
	var ic string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&ic); err != nil {
		return fmt.Errorf("integrity_check: %w", err)
	}
	if ic != "ok" {
		return fmt.Errorf("integrity_check failed: %s", ic)
	}

	return nil
}

// Dump returns a full SQL dump of the database as a string.
func Dump(dbPath string) (string, error) {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return "", fmt.Errorf("database file not found: %s", dbPath)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return "", fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	var b strings.Builder
	b.WriteString("BEGIN;\n")

	// Write schema (tables only, skip indexes and internal tables) excluding sqlite_% and schema_migrations
	rows, err := db.Query(`SELECT name, sql FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name != 'schema_migrations' ORDER BY name`)
	if err != nil {
		return "", fmt.Errorf("query master: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name, sqlText string
		if err := rows.Scan(&name, &sqlText); err != nil {
			_ = rows.Close()
			return "", fmt.Errorf("scan master: %w", err)
		}
		// Skip archive tables from schema — they are auto-created by migration
		if strings.HasSuffix(name, "_archive") {
			continue
		}
		// Drop table if exists before creating (for restore idempotency)
		fmt.Fprintf(&b, "DROP TABLE IF EXISTS %s;\n", quoteIdent(name))
		b.WriteString(sqlText)
		if !strings.HasSuffix(strings.TrimSpace(sqlText), ";") {
			b.WriteString(";")
		}
		b.WriteString("\n")

		// Write data for tables
		dataRows, err := db.Query(fmt.Sprintf("SELECT * FROM %s", quoteIdent(name)))
		if err != nil {
			_ = rows.Close()
			return "", fmt.Errorf("query data %s: %w", name, err)
		}

		cols, _ := dataRows.Columns()
		for dataRows.Next() {
			var vals []any
			if err := scanRowToAnySlice(dataRows, len(cols), &vals); err != nil {
				_ = rows.Close()
				_ = dataRows.Close()
				return "", fmt.Errorf("scan data %s: %w", name, err)
			}
			// Format as VALUES tuple
			fmt.Fprintf(&b, "INSERT INTO %s VALUES %s;\n", quoteIdent(name), valuesTuple(vals))
		}
		_ = dataRows.Close()
	}
	_ = rows.Close()

	b.WriteString("COMMIT;\n")

	return b.String(), nil
}

func scanRowToAnySlice(rows *sql.Rows, cols int, vals *[]any) error {
	*vals = make([]any, cols)
	ptrs := make([]any, cols)
	for i := range *vals {
		ptrs[i] = &(*vals)[i]
	}
	return rows.Scan(ptrs...)
}

func valuesTuple(vals []any) string {
	var b strings.Builder
	b.WriteString("(")
	for i, v := range vals {
		if i > 0 {
			b.WriteString(", ")
		}
		if v == nil {
			b.WriteString("NULL")
		} else {
			fmt.Fprintf(&b, "%v", quoteValue(v))
		}
	}
	b.WriteString(")")
	return b.String()
}

func quoteValue(v any) string {
	switch t := v.(type) {
	case nil:
		return "NULL"
	case string:
		return quoteString(t)
	case []byte:
		return quoteString(string(t))
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v)
	case float32, float64:
		return fmt.Sprintf("%v", v)
	case bool:
		if v.(bool) {
			return "1"
		}
		return "0"
	default:
		return fmt.Sprintf("%v", v)
	}
}

func quoteString(s string) string {
	// Escape single quotes by doubling them
	s = strings.ReplaceAll(s, "'", "''")
	return "'" + s + "'"
}

func quoteIdent(s string) string {
	// Escape double quotes by doubling them
	s = strings.ReplaceAll(s, `"`, `""`)
	return `"` + s + `"`
}

// DumpToFile writes the SQL dump to a file.
func DumpToFile(dbPath, dumpPath string) error {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fmt.Errorf("database file not found: %s", dbPath)
	}

	dump, err := Dump(dbPath)
	if err != nil {
		return err
	}

	if err := os.WriteFile(dumpPath, []byte(dump), 0644); err != nil {
		return fmt.Errorf("write dump file: %w", err)
	}

	return nil
}
