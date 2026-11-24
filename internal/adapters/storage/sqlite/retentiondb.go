package sqlite

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// retentionTables maps hot tables to their archive mirrors and the column
// that carries the row age. It is the fixed retention scope: nothing outside
// this list is ever archived or packaged (issue #120).
var retentionTables = []struct {
	hot     string
	archive string
	ageCol  string
	pk      string
}{
	{"articles", "articles_archive", "published_at", "article_id"},
	{"stories", "stories_archive", "published_at", "story_id"},
	{"ranking_snapshots", "ranking_snapshots_archive", "rank_date", "snapshot_id"},
}

// ArchiveOldDB moves rows older than maxAgeDays from the hot tables to their
// archive mirrors in bounded batches of batchLimit rows. It operates on an
// open *sql.DB handle so it works on both the modernc sqlite driver and the
// libsql (Turso) driver. Returns the number of rows archived per hot table.
func ArchiveOldDB(db *sql.DB, maxAgeDays, batchLimit int) (map[string]int64, error) {
	results := make(map[string]int64)
	for _, t := range retentionTables {
		archived, err := archiveTableBatchedDB(db, t.hot, t.archive, t.ageCol, t.pk, maxAgeDays, batchLimit)
		if err != nil {
			return nil, fmt.Errorf("archive %s: %w", t.hot, err)
		}
		results[t.hot] = archived
	}
	return results, nil
}

func archiveTableBatchedDB(db *sql.DB, hot, archive, ageCol, pk string, maxAgeDays, batchLimit int) (int64, error) {
	totalArchived := int64(0)
	maxAgeParam := fmt.Sprintf("-%d days", maxAgeDays)

	cols, err := getTableColumns(db, hot)
	if err != nil {
		return 0, fmt.Errorf("get columns for %s: %w", hot, err)
	}
	if len(cols) == 0 {
		return 0, fmt.Errorf("no columns for table %s", hot)
	}

	for {
		txn, err := db.Begin()
		if err != nil {
			return totalArchived, fmt.Errorf("begin txn: %w", err)
		}

		// Select up to batchLimit old rows (pk last, deterministic order)
		var selectCols []string
		var plainCols []string
		for _, c := range cols {
			if c != pk {
				selectCols = append(selectCols, quoteIdent(c))
				plainCols = append(plainCols, c)
			}
		}
		selectCols = append(selectCols, quoteIdent(pk))
		plainCols = append(plainCols, pk)
		selectColsStr := joinQuoted(selectCols)

		rows, err := txn.Query(fmt.Sprintf(
			`SELECT %s FROM %s WHERE date(%s) < date('now', ?) ORDER BY %s LIMIT ?`,
			selectColsStr, hot, ageCol, ageCol),
			maxAgeParam, batchLimit)
		if err != nil {
			_ = txn.Rollback()
			return totalArchived, fmt.Errorf("select old rows: %w", err)
		}

		type rowData map[string]any
		var rowsToMove []rowData
		for rows.Next() {
			ptrs := make([]any, len(selectCols))
			vals := make([]any, len(selectCols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				_ = rows.Close()
				_ = txn.Rollback()
				return totalArchived, fmt.Errorf("scan row: %w", err)
			}
			row := make(rowData, len(plainCols))
			for i, c := range plainCols {
				row[c] = vals[i]
			}
			rowsToMove = append(rowsToMove, row)
		}
		_ = rows.Close()

		if len(rowsToMove) == 0 {
			_ = txn.Commit()
			break
		}

		destCols, err := getTableColumns(db, archive)
		if err != nil {
			_ = txn.Rollback()
			return totalArchived, fmt.Errorf("get archive columns for %s: %w", archive, err)
		}
		destCols = append(destCols, "archived_at")
		destColsStr := strings.Join(destCols, ", ")
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(destCols)), ",")

		for _, row := range rowsToMove {
			params := make([]any, 0, len(destCols))
			for _, c := range destCols {
				if c == "archived_at" {
					params = append(params, timestampToString(time.Now().UTC()))
				} else {
					params = append(params, row[c])
				}
			}
			if _, err := txn.Exec(fmt.Sprintf(
				"INSERT OR IGNORE INTO %s (%s) VALUES (%s)",
				archive, destColsStr, placeholders), params...); err != nil {
				_ = txn.Rollback()
				return totalArchived, fmt.Errorf("insert archive: %w", err)
			}
			pkVal := row[pk]
			if _, err := txn.Exec(fmt.Sprintf("DELETE FROM %s WHERE %s = ?", hot, pk), pkVal); err != nil {
				_ = txn.Rollback()
				return totalArchived, fmt.Errorf("delete hot: %w", err)
			}
		}

		if err := txn.Commit(); err != nil {
			return totalArchived, fmt.Errorf("commit batch: %w", err)
		}

		totalArchived += int64(len(rowsToMove))
		if int64(len(rowsToMove)) < int64(batchLimit) {
			break
		}
	}

	return totalArchived, nil
}

// joinQuoted joins already-quoted identifiers with ", ".
func joinQuoted(cols []string) string {
	return strings.Join(cols, ", ")
}

// getTableColumns returns the list of column names for a table.
func getTableColumns(db *sql.DB, table string) ([]string, error) {
	rows, err := db.Query("PRAGMA table_info(" + quoteIdent(table) + ")")
	if err != nil {
		return nil, fmt.Errorf("query table_info for %s: %w", table, err)
	}
	defer rows.Close() //nolint:errcheck

	var cols []string
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return nil, fmt.Errorf("scan table_info for %s: %w", table, err)
		}
		cols = append(cols, name)
	}
	return cols, rows.Err()
}

// StorageEstimateDB returns the current database size in bytes
// (page_count × page_size) for an open handle.
func StorageEstimateDB(db *sql.DB) (int64, error) {
	var pageCount, pageSize int64
	if err := db.QueryRow("PRAGMA page_count").Scan(&pageCount); err != nil {
		return 0, fmt.Errorf("page_count: %w", err)
	}
	if err := db.QueryRow("PRAGMA page_size").Scan(&pageSize); err != nil {
		return 0, fmt.Errorf("page_size: %w", err)
	}
	return pageCount * pageSize, nil
}

// SchemaVersionDB returns the highest applied migration version (0 for an
// un-migrated database).
func SchemaVersionDB(db *sql.DB) (int, error) {
	var ver int
	err := db.QueryRow(fmt.Sprintf("SELECT COALESCE(MAX(version), 0) FROM %s", schemaMigrationsTable)).Scan(&ver)
	if err != nil {
		return 0, fmt.Errorf("schema_migrations: %w", err)
	}
	return ver, nil
}

// ApplyRetentionDB archives old data with the default StoragePolicy and checks
// the storage budget. Returns true if the budget is OK, false if exceeded.
func ApplyRetentionDB(db *sql.DB) (bool, error) {
	return applyRetentionPolicyDB(db, NewStoragePolicy())
}

// applyRetentionPolicyDB archives old data according to the given policy and
// checks the storage budget. Returns true if the budget is OK, false if
// exceeded.
func applyRetentionPolicyDB(db *sql.DB, policy StoragePolicy) (bool, error) {
	_, err := ArchiveOldDB(db, policy.RetentionDays, policy.BatchLimit)
	if err != nil {
		return false, fmt.Errorf("archive: %w", err)
	}
	size, err := StorageEstimateDB(db)
	if err != nil {
		return false, fmt.Errorf("estimate: %w", err)
	}
	return policy.SizeBudgetOK(size), nil
}
