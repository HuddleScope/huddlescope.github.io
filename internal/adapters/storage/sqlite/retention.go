package sqlite

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// RetentionWindow defines the number of days to keep ranking snapshots in the hot table.
const RetentionWindow = 90

// ArchiveBatchLimit defines the maximum rows to move in a single batch.
const ArchiveBatchLimit = 500

// StoragePolicy holds retention configuration.
type StoragePolicy struct {
	RetentionDays int
	BatchLimit    int
	FreeBytes     int64
}

func NewStoragePolicy() StoragePolicy {
	return StoragePolicy{
		RetentionDays: RetentionWindow,
		BatchLimit:    ArchiveBatchLimit,
		FreeBytes:     1_000_000_000, // 1 GB
	}
}

// SizeBudgetOK returns true if the current usage is within the free tier budget.
func (p StoragePolicy) SizeBudgetOK(usedBytes int64) bool {
	return usedBytes <= p.FreeBytes
}

// ArchiveOld moves rows older than maxAge from hot tables to archive tables in bounded batches.
// Returns the number of rows archived per table.
func ArchiveOld(dbPath string, maxAgeDays int) (map[string]int64, error) {
	if dbPath == "" {
		dbPath = "file::memory:?cache=shared"
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := Migrate(dbPath); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	results := make(map[string]int64)

	// Define hot→archive mappings with their age column
	tables := []struct {
		hot     string
		archive string
		ageCol  string
		pk      string
	}{
		{"articles", "articles_archive", "published_at", "article_id"},
		{"stories", "stories_archive", "published_at", "story_id"},
		{"ranking_snapshots", "ranking_snapshots_archive", "rank_date", "snapshot_id"},
	}

	for _, t := range tables {
		archived, err := archiveTableBatched(db, t.hot, t.archive, t.ageCol, t.pk, maxAgeDays)
		if err != nil {
			return nil, fmt.Errorf("archive %s: %w", t.hot, err)
		}
		results[t.hot] = archived
	}

	return results, nil
}

func archiveTableBatched(db *sql.DB, hot, archive, ageCol, pk string, maxAgeDays int) (int64, error) {
	totalArchived := int64(0)
	maxAgeParam := fmt.Sprintf("-%d days", maxAgeDays)

	// Get column list for the hot table (excluding pk for selection)
	cols, err := getTableColumns(db, hot)
	if err != nil {
		return 0, fmt.Errorf("get columns for %s: %w", hot, err)
	}
	if len(cols) == 0 {
		return 0, fmt.Errorf("no columns for table %s", hot)
	}

	for {
		var txn *sql.Tx
		txn, err := db.Begin()
		if err != nil {
			return totalArchived, fmt.Errorf("begin txn: %w", err)
		}

		// Build SELECT clause: select all columns except pk first, then pk
		var selectCols []string
		for _, c := range cols {
			if c != pk {
				selectCols = append(selectCols, quoteIdent(c))
			}
		}
		selectCols = append(selectCols, quoteIdent(pk)) // pk last
		selectColsStr := strings.Join(selectCols, ", ")

		// Select up to ArchiveBatchLimit old rows
		rows, err := txn.Query(fmt.Sprintf(
			`SELECT %s FROM %s WHERE date(%s) < date('now', ?) ORDER BY %s LIMIT ?`,
			selectColsStr, hot, ageCol, ageCol),
			maxAgeParam, ArchiveBatchLimit)
		if err != nil {
			_ = txn.Rollback()
			return totalArchived, fmt.Errorf("select old rows: %w", err)
		}

		// Scan each row into a map
		type rowData map[string]any
		var rowsToMove []rowData

		for rows.Next() {
			// Create slice of pointers for Scan
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

			// Build map
			row := make(rowData, len(selectCols))
			for i, c := range selectCols {
				// Remove quotes from column name
				cleanName := strings.Trim(c, `"`)
				row[cleanName] = vals[i]
			}
			rowsToMove = append(rowsToMove, row)
		}
		_ = rows.Close()

		if len(rowsToMove) == 0 {
			// No more rows to archive
			_ = txn.Commit()
			break
		}

		// Get destination columns (archive table columns + archived_at)
		destCols, err := getTableColumns(db, archive)
		if err != nil {
			_ = txn.Rollback()
			return totalArchived, fmt.Errorf("get archive columns: %w", err)
		}
		destCols = append(destCols, "archived_at")

		for _, row := range rowsToMove {
			// Build insert parameters
			params := make([]any, 0, len(destCols))
			for _, c := range destCols {
				if c == "archived_at" {
					params = append(params, timestampToString(time.Now().UTC()))
				} else {
					params = append(params, row[c])
				}
			}

			// Build VALUES placeholder list
			placeholders := strings.Repeat("?,", len(destCols))
			placeholders = strings.TrimSuffix(placeholders, ",")

			// Build column list for INSERT
			destColsStr := strings.Join(destCols, ", ")

			_, err = txn.Exec(fmt.Sprintf(
				"INSERT OR IGNORE INTO %s (%s) VALUES (%s)",
				archive, destColsStr, placeholders), params...)
			if err != nil {
				_ = txn.Rollback()
				return totalArchived, fmt.Errorf("insert archive: %w", err)
			}

			// Delete from hot table using pk
			pkVal := row[pk]
			_, err = txn.Exec(fmt.Sprintf("DELETE FROM %s WHERE %s = ?", hot, pk), pkVal)
			if err != nil {
				_ = txn.Rollback()
				return totalArchived, fmt.Errorf("delete hot: %w", err)
			}
		}

		if err := txn.Commit(); err != nil {
			return totalArchived, fmt.Errorf("commit batch: %w", err)
		}

		totalArchived += int64(len(rowsToMove))

		if int64(len(rowsToMove)) < ArchiveBatchLimit {
			// Last batch (fewer than limit means done)
			break
		}
	}

	return totalArchived, nil
}

// getTableColumns returns the list of column names for a table
func getTableColumns(db *sql.DB, table string) ([]string, error) {
	rows, err := db.Query("PRAGMA table_info(" + quoteIdent(table) + ")")
	if err != nil {
		return nil, fmt.Errorf("query table_info for %s: %w", table, err)
	}
	defer rows.Close()

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

// StorageEstimate returns the current database size in bytes.
func StorageEstimate(dbPath string) (int64, error) {
	if dbPath == "" {
		dbPath = "file::memory:?cache=shared"
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return 0, fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Use page_count × page_size for canonical DB size
	var pageCount, pageSize int64
	if err := db.QueryRow("PRAGMA page_count").Scan(&pageCount); err != nil {
		return 0, fmt.Errorf("page_count: %w", err)
	}
	if err := db.QueryRow("PRAGMA page_size").Scan(&pageSize); err != nil {
		return 0, fmt.Errorf("page_size: %w", err)
	}

	return pageCount * pageSize, nil
}

// ApplyRetention archives old data and checks storage budget.
// Returns true if budget is OK, false if exceeded.
func ApplyRetention(dbPath string) (bool, error) {
	if dbPath == "" {
		dbPath = "file::memory:?cache=shared"
	}

	policy := NewStoragePolicy()

	if _, err := Migrate(dbPath); err != nil {
		return false, fmt.Errorf("migrate: %w", err)
	}

	_, err := ArchiveOld(dbPath, policy.RetentionDays)
	if err != nil {
		return false, fmt.Errorf("archive: %w", err)
	}

	size, err := StorageEstimate(dbPath)
	if err != nil {
		return false, fmt.Errorf("estimate: %w", err)
	}

	return policy.SizeBudgetOK(size), nil
}
