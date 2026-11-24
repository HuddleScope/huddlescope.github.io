package sqlite

import (
	"archive/tar"
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Retention package format v1 (issue #120 / HS-065, plan §4).
//
// A package is a .tar.gz containing exactly:
//
//	manifest.json                 self-describing manifest (format pinned to v1)
//	tables/articles.sql           INSERT OR REPLACE statements for expiring rows
//	tables/stories.sql            (same)
//	tables/ranking_snapshots.sql  (same)
//
// Only the three whitelisted hot tables ever enter a package — never
// sources, tokens, correction rules, or link tables. Each .sql file holds
// bounded (500-row) transactions:
//
//	BEGIN TRANSACTION;
//	INSERT OR REPLACE INTO <table> (cols...) VALUES (...);
//	...
//	COMMIT;
//
// INSERT OR REPLACE with stable primary keys makes import idempotent and the
// export→import round-trip exact. The package is portable (no absolute paths,
// no foreign keys) so it can be committed to Git and imported on another
// machine or a fresh DB.

// RetentionPackageFormat is the format string of manifest.json (version is
// carried separately as manifest.version, pinned to 1).
const RetentionPackageFormat = "huddlescope-retention-package"

// maxPackageBytes caps the total uncompressed size of a retention package
// (100 MB). It is a var so tests can lower it.
var maxPackageBytes int64 = 100 << 20

// manifest is the manifest.json payload of a retention package (v1).
type manifest struct {
	Format        string          `json:"format"`
	Version       int             `json:"version"`
	PackageID     string          `json:"package_id"`
	GeneratedAt   string          `json:"generated_at"`
	SchemaVersion int             `json:"schema_version"`
	RetentionDays int             `json:"retention_days"`
	SourceBackend string          `json:"source_backend"`
	BatchLimit    int             `json:"batch_limit"`
	ExpiresBefore string          `json:"expires_before"`
	Tables        []manifestTable `json:"tables"`
}

// manifestTable describes one exported table member.
type manifestTable struct {
	Name        string `json:"name"`
	ColumnCount int    `json:"column_count"`
	RowCount    int    `json:"row_count"`
	SQLFile     string `json:"sql_file"`
}

// ExportPackageDB writes all rows older than retentionDays (identical
// predicate to ArchiveOldDB) into a portable package at outPath. It performs
// NO deletion. If no rows expire it writes nothing (Noop) and reports zero
// rows. The file is written via a temp file + atomic rename with 0600
// permissions (plan §4).
func ExportPackageDB(db *sql.DB, outPath string, retentionDays, batchLimit int, sourceBackend string) (map[string]int64, manifest, error) {
	if retentionDays <= 0 {
		return nil, manifest{}, fmt.Errorf("retentionDays must be positive; got %d", retentionDays)
	}
	if batchLimit <= 0 {
		batchLimit = ArchiveBatchLimit
	}

	schemaVersion, err := SchemaVersionDB(db)
	if err != nil {
		return nil, manifest{}, err
	}

	maxAgeParam := fmt.Sprintf("-%d days", retentionDays)

	tables := make([]manifestTable, 0, len(retentionTables))
	fileData := make(map[string][]string, len(retentionTables)) // table -> statements
	rowsPerTable := make(map[string]int64, len(retentionTables))
	var totalRows int64

	for _, t := range retentionTables {
		cols, err := getTableColumns(db, t.hot)
		if err != nil {
			return nil, manifest{}, fmt.Errorf("columns for %s: %w", t.hot, err)
		}
		colsQuoted := make([]string, len(cols))
		for i, c := range cols {
			colsQuoted[i] = quoteIdent(c)
		}

		rows, err := db.Query(fmt.Sprintf(
			"SELECT %s FROM %s WHERE date(%s) < date('now', ?)",
			strings.Join(colsQuoted, ", "), t.hot, t.ageCol),
			maxAgeParam)
		if err != nil {
			return nil, manifest{}, fmt.Errorf("select %s: %w", t.hot, err)
		}

		var stmts []string
		rowCount := 0
		batch := 0
		batchOpen := false
		closeBatch := func() error {
			if batchOpen {
				stmts = append(stmts, "COMMIT;")
				batchOpen = false
			}
			return nil
		}

		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(vals))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				_ = rows.Close()
				return nil, manifest{}, fmt.Errorf("scan %s: %w", t.hot, err)
			}
			if !batchOpen {
				stmts = append(stmts, "BEGIN TRANSACTION;")
				batchOpen = true
			}
			stmts = append(stmts, fmt.Sprintf("INSERT OR REPLACE INTO %s (%s) VALUES %s;",
				quoteIdent(t.hot), strings.Join(colsQuoted, ", "), valuesTuple(vals)))
			rowCount++
			batch++
			if batch >= batchLimit {
				if err := closeBatch(); err != nil {
					_ = rows.Close()
					return nil, manifest{}, err
				}
				batch = 0
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, manifest{}, fmt.Errorf("rows %s: %w", t.hot, err)
		}
		_ = rows.Close()
		if err := closeBatch(); err != nil {
			return nil, manifest{}, err
		}

		fileData[t.hot] = stmts
		rowsPerTable[t.hot] = int64(rowCount)
		totalRows += int64(rowCount)
		tables = append(tables, manifestTable{
			Name:        t.hot,
			ColumnCount: len(cols),
			RowCount:    rowCount,
			SQLFile:     "tables/" + t.hot + ".sql",
		})
	}

	m := manifest{
		Format:        RetentionPackageFormat,
		Version:       1,
		PackageID:     uuid.New().String(),
		GeneratedAt:   timestampToString(time.Now().UTC()),
		SchemaVersion: schemaVersion,
		RetentionDays: retentionDays,
		SourceBackend: sourceBackend,
		BatchLimit:    batchLimit,
		ExpiresBefore: timestampToString(time.Now().UTC().AddDate(0, 0, -retentionDays)),
		Tables:        tables,
	}

	if totalRows == 0 {
		// Noop: nothing expired — write nothing.
		return rowsPerTable, m, nil
	}

	if err := writePackage(outPath, m, fileData); err != nil {
		return nil, manifest{}, fmt.Errorf("write package: %w", err)
	}
	return rowsPerTable, m, nil
}

// writePackage writes manifest.json + tables/<table>.sql into a .tar.gz at
// outPath via a temp file + atomic rename (0600).
func writePackage(outPath string, m manifest, fileData map[string][]string) error {
	dir := filepath.Dir(outPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("create dir %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".retention-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp: %w", err)
	}

	gz := gzip.NewWriter(tmp)
	tw := tar.NewWriter(gz)

	manifestBytes, err := json.Marshal(m)
	if err != nil {
		_ = tw.Close()
		_ = gz.Close()
		_ = tmp.Close()
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0o644, Size: int64(len(manifestBytes))}); err != nil {
		_ = tw.Close()
		_ = gz.Close()
		_ = tmp.Close()
		return fmt.Errorf("write manifest header: %w", err)
	}
	if _, err := tw.Write(manifestBytes); err != nil {
		_ = tw.Close()
		_ = gz.Close()
		_ = tmp.Close()
		return fmt.Errorf("write manifest: %w", err)
	}

	for _, t := range retentionTables {
		data := strings.Join(fileData[t.hot], "\n")
		name := "tables/" + t.hot + ".sql"
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))}); err != nil {
			_ = tw.Close()
			_ = gz.Close()
			_ = tmp.Close()
			return fmt.Errorf("write %s header: %w", name, err)
		}
		if _, err := tw.Write([]byte(data)); err != nil {
			_ = tw.Close()
			_ = gz.Close()
			_ = tmp.Close()
			return fmt.Errorf("write %s: %w", name, err)
		}
	}

	if err := tw.Close(); err != nil {
		_ = gz.Close()
		_ = tmp.Close()
		return fmt.Errorf("close tar: %w", err)
	}
	if err := gz.Close(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("close gzip: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpName, outPath); err != nil {
		return fmt.Errorf("rename temp to %s: %w", outPath, err)
	}
	tmpName = ""
	return nil
}

// readPackageMembers reads and parses the package members of a .tar.gz:
// manifest.json plus tables/*.sql, enforcing the member whitelist
// (manifest.json + exactly the whitelisted table .sql files), no path
// traversal, and the size cap.
func readPackageMembers(pkgPath string) (manifest, map[string][]string, error) {
	f, err := os.Open(pkgPath)
	if err != nil {
		return manifest{}, nil, fmt.Errorf("open package: %w", err)
	}
	defer f.Close() //nolint:errcheck

	gz, err := gzip.NewReader(f)
	if err != nil {
		return manifest{}, nil, fmt.Errorf("gzip open: %w", err)
	}
	defer gz.Close() //nolint:errcheck

	tr := tar.NewReader(gz)
	var manifestBytes []byte
	tableSQL := make(map[string][]string)
	var total int64

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return manifest{}, nil, fmt.Errorf("tar next: %w", err)
		}
		name := hdr.Name
		if strings.Contains(name, "..") {
			return manifest{}, nil, fmt.Errorf("tar member name %q contains path traversal", name)
		}

		if name == "manifest.json" {
			if manifestBytes != nil {
				return manifest{}, nil, fmt.Errorf("duplicate manifest.json member")
			}
			manifestBytes, err = readMemberLimited(tr, hdr, maxPackageBytes, &total)
			if err != nil {
				return manifest{}, nil, err
			}
			continue
		}

		if !strings.HasPrefix(name, "tables/") || !strings.HasSuffix(name, ".sql") {
			return manifest{}, nil, fmt.Errorf("unexpected tar member %q", name)
		}
		tableName := strings.TrimSuffix(strings.TrimPrefix(name, "tables/"), ".sql")
		if !isRetentionHotTable(tableName) {
			return manifest{}, nil, fmt.Errorf("table %q is not in the retention whitelist", tableName)
		}
		if _, exists := tableSQL[tableName]; exists {
			return manifest{}, nil, fmt.Errorf("duplicate table member %q", name)
		}
		data, err := readMemberLimited(tr, hdr, maxPackageBytes, &total)
		if err != nil {
			return manifest{}, nil, err
		}
		tableSQL[tableName] = splitSQLStatements(string(data))
	}

	if manifestBytes == nil {
		return manifest{}, nil, fmt.Errorf("missing manifest.json member")
	}
	var m manifest
	if err := json.Unmarshal(manifestBytes, &m); err != nil {
		return manifest{}, nil, fmt.Errorf("parse manifest: %w", err)
	}
	return m, tableSQL, nil
}

// readMemberLimited reads one tar member, enforcing the package size cap.
func readMemberLimited(tr *tar.Reader, hdr *tar.Header, capBytes int64, total *int64) ([]byte, error) {
	size := hdr.Size
	if size < 0 {
		return nil, fmt.Errorf("tar member %q with unknown size", hdr.Name)
	}
	*total += size
	if *total > capBytes {
		return nil, fmt.Errorf("package exceeds size cap of %d bytes", capBytes)
	}
	data, err := io.ReadAll(io.LimitReader(tr, size+1))
	if err != nil {
		return nil, fmt.Errorf("read member %q: %w", hdr.Name, err)
	}
	if int64(len(data)) != size {
		return nil, fmt.Errorf("member %q size mismatch: header %d, got %d", hdr.Name, size, len(data))
	}
	return data, nil
}

// isRetentionHotTable reports whether name is one of the hot tables in the
// retention whitelist (the three tables retention applies to).
func isRetentionHotTable(name string) bool {
	for _, t := range retentionTables {
		if t.hot == name {
			return true
		}
	}
	return false
}

// splitSQLStatements splits a .sql script into individual statements,
// handling single-quoted strings (including ” escapes) and double-quoted
// identifiers.
func splitSQLStatements(script string) []string {
	var stmts []string
	var cur strings.Builder
	inSingle, inDouble := false, false
	runes := []rune(script)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		switch {
		case inSingle:
			cur.WriteRune(c)
			if c == '\'' {
				if i+1 < len(runes) && runes[i+1] == '\'' {
					cur.WriteRune('\'')
					i++
				} else {
					inSingle = false
				}
			}
		case inDouble:
			cur.WriteRune(c)
			if c == '"' {
				inDouble = false
			}
		case c == '\'':
			inSingle = true
			cur.WriteRune(c)
		case c == '"':
			inDouble = true
			cur.WriteRune(c)
		case c == ';':
			s := strings.TrimSpace(cur.String())
			if s != "" {
				stmts = append(stmts, s)
			}
			cur.Reset()
		default:
			cur.WriteRune(c)
		}
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		stmts = append(stmts, s)
	}
	return stmts
}

// validatePackageForImport checks the manifest format/version, the table
// whitelist, per-file row counts, and classifies every statement (only
// INSERT OR REPLACE / BEGIN / COMMIT allowed; DROP/DELETE/UPDATE/ALTER/CREATE
// rejected; no semicolon-stacked statements; no statement targeting a table
// outside the whitelist).
func validatePackageForImport(m manifest, tableSQL map[string][]string) error {
	if m.Format != RetentionPackageFormat {
		return fmt.Errorf("unexpected format %q (want %q)", m.Format, RetentionPackageFormat)
	}
	if m.Version != 1 {
		return fmt.Errorf("unexpected manifest version %d (want 1)", m.Version)
	}
	if len(m.Tables) == 0 {
		return fmt.Errorf("manifest declares no tables")
	}
	seen := make(map[string]bool, len(m.Tables))
	for _, tb := range m.Tables {
		if seen[tb.Name] {
			return fmt.Errorf("duplicate table %q in manifest", tb.Name)
		}
		seen[tb.Name] = true
		if !isRetentionHotTable(tb.Name) {
			return fmt.Errorf("table %q is not in the retention whitelist", tb.Name)
		}
		statements, ok := tableSQL[tb.Name]
		if !ok {
			return fmt.Errorf("manifest declares table %q but its .sql member is missing", tb.Name)
		}
		if n := countInserts(statements); n != tb.RowCount {
			return fmt.Errorf("table %q: manifest row_count %d does not match %d INSERT statements", tb.Name, tb.RowCount, n)
		}
		for _, stmt := range statements {
			if err := classifySQLStatement(stmt, tb.Name); err != nil {
				return err
			}
		}
	}
	// Every .sql member must be declared in the manifest.
	for name := range tableSQL {
		if !seen[name] {
			return fmt.Errorf("table %q has a .sql member but is not declared in the manifest", name)
		}
	}
	return nil
}

// countInserts counts INSERT OR REPLACE statements in a statement list.
func countInserts(stmts []string) int {
	n := 0
	for _, s := range stmts {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(s)), "INSERT OR REPLACE INTO ") {
			n++
		}
	}
	return n
}

// classifySQLStatement enforces the import statement whitelist
// (plan §4/§7): only INSERT OR REPLACE INTO <whitelisted table> with a
// single-statement VALUES clause; BEGIN/COMMIT transaction markers.
// Semicolon-stacked statements cannot reach this function (splitSQLStatements
// already splits on ';').
func classifySQLStatement(stmt, table string) error {
	upper := strings.ToUpper(strings.TrimSpace(stmt))
	switch {
	case strings.HasPrefix(upper, "BEGIN"):
		return nil
	case strings.HasPrefix(upper, "COMMIT"):
		return nil
	case strings.HasPrefix(upper, "INSERT OR REPLACE INTO "):
		// Target table must match the file's declared table.
		rest := strings.TrimSpace(strings.TrimPrefix(upper, "INSERT OR REPLACE INTO "))
		target := strings.Fields(rest)
		if len(target) == 0 {
			return fmt.Errorf("malformed INSERT statement in %s", table)
		}
		tName := strings.Trim(target[0], "`\"[]")
		if !strings.EqualFold(tName, table) {
			return fmt.Errorf("statement in %s.sql targets table %q", table, tName)
		}
		if !strings.Contains(upper, "VALUES") {
			return fmt.Errorf("INSERT in %s must use VALUES", table)
		}
		return nil
	default:
		return fmt.Errorf("statement not allowed in retention import (only INSERT OR REPLACE / BEGIN / COMMIT): %q",
			truncateForError(stmt))
	}
}

// truncateForError shortens a statement for safe inclusion in an error
// message.
func truncateForError(s string) string {
	if len(s) > 80 {
		return s[:80] + "…"
	}
	return s
}

// ImportPackageDB validates a package and executes its INSERT OR REPLACE
// statements in a single transaction. Returns the per-table row counts and
// the manifest.
func ImportPackageDB(db *sql.DB, pkgPath string) (map[string]int64, manifest, error) {
	m, tableSQL, err := readPackageMembers(pkgPath)
	if err != nil {
		return nil, manifest{}, err
	}
	if err := validatePackageForImport(m, tableSQL); err != nil {
		return nil, manifest{}, err
	}

	txn, err := db.Begin()
	if err != nil {
		return nil, manifest{}, fmt.Errorf("begin import txn: %w", err)
	}
	rowsPerTable := make(map[string]int64)
	for table, stmts := range tableSQL {
		for _, stmt := range stmts {
			upper := strings.ToUpper(strings.TrimSpace(stmt))
			// The file carries BEGIN/COMMIT markers; our driver-level txn
			// already owns the transaction, so skip them on execution.
			if strings.HasPrefix(upper, "BEGIN") || strings.HasPrefix(upper, "COMMIT") {
				continue
			}
			if _, err := txn.Exec(stmt); err != nil {
				_ = txn.Rollback()
				return nil, manifest{}, fmt.Errorf("import %s: %w", table, err)
			}
		}
		rowsPerTable[table] = int64(countInserts(stmts))
	}
	if err := txn.Commit(); err != nil {
		_ = txn.Rollback()
		return nil, manifest{}, fmt.Errorf("commit import: %w", err)
	}
	return rowsPerTable, m, nil
}
