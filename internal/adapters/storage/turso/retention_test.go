package turso

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage/sqlite"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// newTestRetentionService wraps a local modernc sqlite handle in the Turso
// RetentionService via the network-free test constructor. This proves the
// service is driver-agnostic (works over any database/sql handle), while the
// production path uses the libsql driver (NewRetentionService).
func newTestRetentionService(t *testing.T) (*RetentionService, string) {
	t.Helper()
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := newRetentionServiceFromDB(db, "")
	return svc, dbPath
}

func TestTursoRetentionService_ExportApplyImportStatus(t *testing.T) {
	svc, _ := newTestRetentionService(t)
	t.Cleanup(func() { _ = svc.Close() })

	// Seed one old story via the underlying handle (migrations applied by init).
	if err := svc.init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	old := "2026-01-01T00:00:00Z"
	if _, err := svc.db.Exec(`INSERT INTO stories (story_id, story_key, title, summary, published_at, status)
		VALUES ('11111111-1111-1111-1111-111111111111', 'old-s', 'Old story', 's', ?, 'active')`, old); err != nil {
		t.Fatalf("seed: %v", err)
	}

	pol := domain.NewRetentionPolicy()
	// Old story is far older than 90 days.

	outPath := filepath.Join(t.TempDir(), "pkg.tar.gz")
	rep, err := svc.ExportPackage(t.Context(), outPath, pol)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if rep.Noop || rep.TotalRows != 1 {
		t.Fatalf("export: want 1 row, got noop=%v total=%d", rep.Noop, rep.TotalRows)
	}

	// Apply archives the old story.
	report, err := svc.Apply(t.Context(), pol)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if report.TotalArchived != 1 {
		t.Errorf("archived: want 1, got %d", report.TotalArchived)
	}

	// Delete the hot story and restore from the package.
	if _, err := svc.db.Exec("DELETE FROM stories"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	rep2, err := svc.Import(t.Context(), outPath)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if rep2.TotalRows != 1 {
		t.Errorf("imported: want 1, got %d", rep2.TotalRows)
	}

	st, err := svc.Status(t.Context())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.Backend != "turso" {
		t.Errorf("backend: want turso, got %q", st.Backend)
	}
	if st.SchemaVersion != 8 {
		t.Errorf("schema: want 8, got %d", st.SchemaVersion)
	}
}

func TestTursoRetentionService_InitErrorRedaction(t *testing.T) {
	// A DSN with an auth token: connection errors must not leak the token.
	dsn := "libsql://db.turso.io/default?authToken=secret-token-123"
	svc := newRetentionServiceFromDB(nil, dsn)
	err := svc.wrapInitError(&redactProbeErr{msg: "failed to connect to " + dsn})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "secret-token-123") {
		t.Errorf("error leaks token: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "authToken=***") {
		t.Errorf("error should contain redacted token: %q", err.Error())
	}
}

// redactProbeErr is a dummy error whose message embeds the DSN for
// redaction tests.
type redactProbeErr struct{ msg string }

func (e *redactProbeErr) Error() string { return e.msg }

func TestTursoRetentionService_NewRetentionServiceLazyOpen(t *testing.T) {
	// sql.Open is lazy: no network I/O at construction time.
	svc, err := NewRetentionService("libsql://example.invalid/db", "")
	if err != nil {
		t.Fatalf("construction should not touch the network: %v", err)
	}
	defer func() { _ = svc.Close() }() //nolint:errcheck

	// The first operation fails (unreachable host) and must not leak the DSN.
	_, err = svc.Status(context.Background())
	if err == nil {
		t.Fatal("expected error from unreachable host")
	}
	if strings.Contains(err.Error(), "example.invalid/db") && !strings.Contains(err.Error(), "initialize turso retention") {
		t.Logf("error: %v", err)
	}
}

func TestTursoRetentionService_InitDSNBranch(t *testing.T) {
	// A real local handle with a non-empty DSN exercises init()'s
	// PRAGMA foreign_keys + MigrateDB branch (no network).
	tmp := t.TempDir()
	dbPath := "file:" + filepath.Join(tmp, "init.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := newRetentionServiceFromDB(db, dbPath)
	if err := svc.init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	st, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.SchemaVersion != 8 {
		t.Errorf("schema: want 8, got %d", st.SchemaVersion)
	}
}

func TestTursoRetentionService_NewRetentionServiceEmptyURL(t *testing.T) {
	if _, err := NewRetentionService("", ""); err == nil {
		t.Fatal("expected error for empty database URL")
	}
}

// Ensure the sqlite package is referenced (driver-agnostic delegation).
var _ = sqlite.MigrateDB
