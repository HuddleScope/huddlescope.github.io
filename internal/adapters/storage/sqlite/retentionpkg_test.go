package sqlite

import (
	"archive/tar"
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

func TestExportPackage_OnlyExpiringRows(t *testing.T) {
	db := newRetentionTestDB(t)
	old := oldDateStr(t, 120)
	fresh := time.Now().UTC().Format(time.RFC3339)
	if _, err := seedStory(db, "old-s", "Old story", old); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := seedStory(db, "fresh-s", "Fresh story", fresh); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := seedArticle(db, "a123456789abcdef", "src-a", "Old article", old); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := seedSnapshot(db, "2026-01-01", "nyg", 1); err != nil {
		t.Fatalf("seed: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "pkg.tar.gz")
	rep, err := exportForTest(db, outPath, domain.NewRetentionPolicy())
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if rep.Noop {
		t.Fatal("expected a non-noop export")
	}
	if rep.Rows["stories"] != 1 || rep.Rows["articles"] != 1 || rep.Rows["ranking_snapshots"] != 1 {
		t.Errorf("rows: %v", rep.Rows)
	}
	if rep.SchemaVersion != 8 {
		t.Errorf("schema version: want 8, got %d", rep.SchemaVersion)
	}
	if rep.TotalRows != 3 {
		t.Errorf("total rows: want 3, got %d", rep.TotalRows)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("package file: %v", err)
	}
}

func TestExportPackage_NoopWhenFresh(t *testing.T) {
	db := newRetentionTestDB(t)
	fresh := time.Now().UTC().Format(time.RFC3339)
	if _, err := seedStory(db, "fresh-s", "Fresh story", fresh); err != nil {
		t.Fatalf("seed: %v", err)
	}

	outDir := t.TempDir()
	outPath := filepath.Join(outDir, "pkg.tar.gz")
	rep, err := exportForTest(db, outPath, domain.NewRetentionPolicy())
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if !rep.Noop {
		t.Error("expected noop for fresh data")
	}
	entries, _ := os.ReadDir(outDir)
	if len(entries) != 0 {
		t.Errorf("no files expected for noop, got %d", len(entries))
	}
}

func TestImportExportRoundTrip(t *testing.T) {
	db := newRetentionTestDB(t)
	old := oldDateStr(t, 120)

	cases := []struct{ key, title string }{
		{"uni-s", "Ünïcödé & “quotes”"},
		{"nl-s", "line one\nline two"},
		{"nul-s", ""},
	}
	for _, c := range cases {
		if _, err := seedStory(db, c.key, c.title, old); err != nil {
			t.Fatalf("seed %s: %v", c.key, err)
		}
	}
	if err := seedArticle(db, "a123456789abcdef", "src-a", "article with 'quotes'", old); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := seedSnapshot(db, "2026-01-01", "nyg", 1); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// story_data JSON with embedded quotes/newlines (valid JSON must survive)
	const wantJSON = `{"note":"he said \"hi\"\nline2","n":null,"nums":[1,2,3]}`
	if _, err := db.Exec(`UPDATE stories SET story_data = ? WHERE story_key = ?`, wantJSON, "uni-s"); err != nil {
		t.Fatalf("set story_data: %v", err)
	}

	before := hotTableFingerprint(t, db)

	outPath := filepath.Join(t.TempDir(), "pkg.tar.gz")
	rep, err := exportForTest(db, outPath, domain.NewRetentionPolicy())
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if rep.TotalRows != 5 {
		t.Errorf("exported rows: want 5, got %d", rep.TotalRows)
	}

	// Simulate apply: delete links, then hot rows.
	for _, table := range []string{"story_articles", "articles", "stories", "ranking_snapshots"} {
		if _, err := db.Exec("DELETE FROM " + table); err != nil {
			t.Fatalf("delete %s: %v", table, err)
		}
	}

	rep2, err := importForTest(db, outPath)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if rep2.TotalRows != 5 {
		t.Errorf("imported rows: want 5, got %d", rep2.TotalRows)
	}
	if rep2.PackageID != rep.PackageID {
		t.Errorf("package id mismatch: %q vs %q", rep2.PackageID, rep.PackageID)
	}

	after := hotTableFingerprint(t, db)
	if before != after {
		t.Errorf("round-trip mismatch:\nbefore=%q\nafter=%q", before, after)
	}

	var got string
	if err := db.QueryRow(`SELECT story_data FROM stories WHERE story_key = ?`, "uni-s").Scan(&got); err != nil {
		t.Fatalf("scan story_data: %v", err)
	}
	if got != wantJSON {
		t.Errorf("story_data: want %q, got %q", wantJSON, got)
	}
}

func TestImportIdempotent(t *testing.T) {
	db := newRetentionTestDB(t)
	old := oldDateStr(t, 120)
	if _, err := seedStory(db, "old-s", "Old story", old); err != nil {
		t.Fatalf("seed: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "pkg.tar.gz")
	if _, err := exportForTest(db, outPath, domain.NewRetentionPolicy()); err != nil {
		t.Fatalf("export: %v", err)
	}
	if _, err := db.Exec("DELETE FROM stories"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	for i := 0; i < 2; i++ {
		if _, err := importForTest(db, outPath); err != nil {
			t.Fatalf("import %d: %v", i, err)
		}
	}
	if n := countTable(t, db, "stories"); n != 1 {
		t.Errorf("stories after double import: want 1, got %d", n)
	}
}

func TestImportFKSafe(t *testing.T) {
	db := newRetentionTestDB(t)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	old := oldDateStr(t, 120)
	fresh := time.Now().UTC().Format(time.RFC3339)

	oldStoryID, err := seedStory(db, "old-s", "Old story", old)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := seedArticle(db, "a123456789abcdef", "src-a", "Old article", old); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO story_articles (story_id, article_id) VALUES (?, ?)`, oldStoryID, "a123456789abcdef"); err != nil {
		t.Fatalf("link: %v", err)
	}

	if _, err := seedStory(db, "new-s", "New story", fresh); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := seedArticle(db, "b123456789abcdef", "src-b", "New article", fresh); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO story_articles (story_id, article_id)
		SELECT story_id, 'b123456789abcdef' FROM stories WHERE story_key = 'new-s'`); err != nil {
		t.Fatalf("link: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "pkg.tar.gz")
	if _, err := exportForTest(db, outPath, domain.NewRetentionPolicy()); err != nil {
		t.Fatalf("export: %v", err)
	}

	// Simulate apply with FK ON: delete the old story (cascades its links),
	// then the old article. Fresh rows untouched.
	if _, err := db.Exec(`DELETE FROM stories WHERE story_id = ?`, oldStoryID); err != nil {
		t.Fatalf("delete old story: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM articles WHERE article_id = 'a123456789abcdef'`); err != nil {
		t.Fatalf("delete old article: %v", err)
	}

	if _, err := importForTest(db, outPath); err != nil {
		t.Fatalf("import with FK on: %v", err)
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM story_articles WHERE article_id = 'b123456789abcdef'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("fresh article links: want 1, got %d", n)
	}
	// The old story and article are back.
	if n := countTable(t, db, "stories"); n != 2 {
		t.Errorf("stories: want 2, got %d", n)
	}
	if n := countTable(t, db, "articles"); n != 2 {
		t.Errorf("articles: want 2, got %d", n)
	}
}

func TestImportRejectsMaliciousPackage(t *testing.T) {
	db := newRetentionTestDB(t)
	tmp := t.TempDir()

	baseManifest := manifest{
		Format: RetentionPackageFormat, Version: 1, PackageID: "p1",
		GeneratedAt: "2026-01-01T00:00:00Z", SchemaVersion: 7,
		RetentionDays: 90, SourceBackend: "sqlite-file", BatchLimit: 500,
		ExpiresBefore: "2025-10-02T00:00:00Z",
	}
	emptyTables := func() []manifestTable {
		return []manifestTable{{Name: "articles", ColumnCount: 6, RowCount: 0, SQLFile: "tables/articles.sql"}}
	}
	emptyFiles := map[string]string{"tables/articles.sql": ""}

	tests := []struct {
		name    string
		build   func(path string) error
		wantErr string
	}{
		{
			name: "extra table in manifest",
			build: func(path string) error {
				m := baseManifest
				m.Tables = []manifestTable{
					{Name: "articles", ColumnCount: 6, RowCount: 0, SQLFile: "tables/articles.sql"},
					{Name: "feeds", ColumnCount: 4, RowCount: 0, SQLFile: "tables/feeds.sql"},
				}
				return writeTestPackage(path, m, map[string]string{
					"tables/articles.sql": "", "tables/feeds.sql": "",
				})
			},
			wantErr: "not in the retention whitelist",
		},
		{
			name: "DROP statement",
			build: func(path string) error {
				m := baseManifest
				m.Tables = emptyTables()
				return writeTestPackage(path, m, map[string]string{
					"tables/articles.sql": "DROP TABLE articles",
				})
			},
			wantErr: "statement not allowed",
		},
		{
			name: "wrong format string",
			build: func(path string) error {
				m := baseManifest
				m.Format = "evil-format"
				return writeTestPackage(path, m, emptyFiles)
			},
			wantErr: "unexpected format",
		},
		{
			name: "row count mismatch",
			build: func(path string) error {
				m := baseManifest
				m.Tables = []manifestTable{{Name: "articles", ColumnCount: 6, RowCount: 5, SQLFile: "tables/articles.sql"}}
				return writeTestPackage(path, m, map[string]string{
					"tables/articles.sql": "INSERT OR REPLACE INTO articles (article_id) VALUES ('a')",
				})
			},
			wantErr: "does not match 1 INSERT statements",
		},
		{
			name: "path traversal member",
			build: func(path string) error {
				m := baseManifest
				m.Tables = emptyTables()
				return writeTestPackage(path, m, map[string]string{
					"tables/articles.sql": "",
					"../evil.sql":         "DROP TABLE x",
				})
			},
			wantErr: "path traversal",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pkgPath := filepath.Join(tmp, "pkg-"+tc.name+".tar.gz")
			if err := tc.build(pkgPath); err != nil {
				t.Fatalf("build package: %v", err)
			}
			_, err := importForTest(db, pkgPath)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestImportValidEmptyPackage(t *testing.T) {
	db := newRetentionTestDB(t)
	pkgPath := filepath.Join(t.TempDir(), "empty.tar.gz")
	m := manifest{
		Format: RetentionPackageFormat, Version: 1, PackageID: "p1",
		GeneratedAt: "2026-01-01T00:00:00Z", SchemaVersion: 7,
		RetentionDays: 90, SourceBackend: "sqlite-file", BatchLimit: 500,
		ExpiresBefore: "2025-10-02T00:00:00Z",
		Tables:        []manifestTable{{Name: "articles", ColumnCount: 6, RowCount: 0, SQLFile: "tables/articles.sql"}},
	}
	if err := writeTestPackage(pkgPath, m, map[string]string{"tables/articles.sql": ""}); err != nil {
		t.Fatalf("build: %v", err)
	}
	rep, err := importForTest(db, pkgPath)
	if err != nil {
		t.Fatalf("valid empty package should import: %v", err)
	}
	if rep.TotalRows != 0 {
		t.Errorf("total rows: want 0, got %d", rep.TotalRows)
	}
}

// --- test helpers ------------------------------------------------------------

func exportForTest(db *sql.DB, outPath string, pol domain.RetentionPolicy) (domain.PackageReport, error) {
	rows, manifest, err := ExportPackageDB(db, outPath, pol.RetentionDays, pol.BatchLimit, pol.SourceBackend)
	if err != nil {
		return domain.PackageReport{}, err
	}
	total := int64(0)
	for _, n := range rows {
		total += n
	}
	report := domain.PackageReport{
		PackageID:     manifest.PackageID,
		GeneratedAt:   manifest.GeneratedAt,
		SchemaVersion: manifest.SchemaVersion,
		RetentionDays: manifest.RetentionDays,
		Rows:          rows,
		TotalRows:     total,
		Noop:          total == 0,
	}
	if !report.Noop {
		report.PackagePath = outPath
	}
	return report, nil
}

func importForTest(db *sql.DB, pkgPath string) (domain.PackageReport, error) {
	rows, manifest, err := ImportPackageDB(db, pkgPath)
	if err != nil {
		return domain.PackageReport{}, err
	}
	total := int64(0)
	for _, n := range rows {
		total += n
	}
	return domain.PackageReport{
		PackagePath:   pkgPath,
		PackageID:     manifest.PackageID,
		GeneratedAt:   manifest.GeneratedAt,
		SchemaVersion: manifest.SchemaVersion,
		RetentionDays: manifest.RetentionDays,
		Rows:          rows,
		TotalRows:     total,
	}, nil
}

// hotTableFingerprint returns a stable string of all rows in the hot tables,
// for before/after comparisons.
func hotTableFingerprint(t *testing.T, db *sql.DB) string {
	t.Helper()
	var sb strings.Builder
	for _, table := range []string{"stories", "articles", "ranking_snapshots"} {
		cols, err := getTableColumns(db, table)
		if err != nil {
			t.Fatalf("cols %s: %v", table, err)
		}
		rows, err := db.Query("SELECT " + strings.Join(cols, ",") + " FROM " + table + " ORDER BY rowid")
		if err != nil {
			t.Fatalf("query %s: %v", table, err)
		}
		for rows.Next() {
			values := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range values {
				ptrs[i] = &values[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				_ = rows.Close()
				t.Fatalf("scan %s: %v", table, err)
			}
			for _, v := range values {
				switch x := v.(type) {
				case []byte:
					sb.WriteString(string(x))
				default:
					fmt.Fprintf(&sb, "%v", x)
				}
				sb.WriteByte('|')
			}
			sb.WriteByte('\n')
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			t.Fatalf("rows %s: %v", table, err)
		}
		_ = rows.Close()
	}
	return sb.String()
}

// writeTestPackage builds a .tar.gz retention package with the given manifest
// and file contents (path -> content). Used to craft adversarial packages.
func writeTestPackage(path string, m manifest, files map[string]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck
	gz := gzip.NewWriter(f)
	defer gz.Close() //nolint:errcheck
	tw := tar.NewWriter(gz)

	manifestBytes, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if err := tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0o644, Size: int64(len(manifestBytes))}); err != nil {
		return err
	}
	if _, err := tw.Write(manifestBytes); err != nil {
		return err
	}
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}); err != nil {
			return err
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			return err
		}
	}
	return tw.Close()
}
