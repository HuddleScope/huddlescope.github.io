package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage/sqlite"
	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

func TestNewRetentionService_MemoryNoop(t *testing.T) {
	svc, err := NewRetentionService(config.StorageConfig{Backend: "memory"})
	if err != nil {
		t.Fatalf("memory: %v", err)
	}
	defer func() { _ = svc.Close() }() //nolint:errcheck

	// Export must fail loudly (packages must be persisted; AC-2).
	if _, err := svc.ExportPackage(context.Background(), filepath.Join(t.TempDir(), "p.tar.gz"), domain.NewRetentionPolicy()); err == nil {
		t.Error("memory export should fail")
	} else if err != sqlite.ErrNotSupported {
		t.Errorf("memory export error: want ErrNotSupported, got %v", err)
	}
	// Apply is a safe no-op.
	if _, err := svc.Apply(context.Background(), domain.NewRetentionPolicy()); err != nil {
		t.Errorf("memory apply should be a no-op, got %v", err)
	}
	// Import must fail loudly.
	if _, err := svc.Import(context.Background(), "p.tar.gz"); err == nil {
		t.Error("memory import should fail")
	}
	// Status reports the memory backend.
	st, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("memory status: %v", err)
	}
	if st.Backend != "memory" {
		t.Errorf("backend: want memory, got %q", st.Backend)
	}
}

func TestNewRetentionService_SqliteFile(t *testing.T) {
	svc, err := NewRetentionService(config.StorageConfig{
		Backend: "sqlite-file",
		Path:    filepath.Join(t.TempDir(), "retention.db"),
	})
	if err != nil {
		t.Fatalf("sqlite-file: %v", err)
	}
	defer func() { _ = svc.Close() }() //nolint:errcheck

	st, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.Backend != "sqlite-file" {
		t.Errorf("backend: want sqlite-file, got %q", st.Backend)
	}
	if st.SchemaVersion != 11 {
		t.Errorf("schema: want 11, got %d", st.SchemaVersion)
	}
}

func TestNewRetentionService_UnknownBackend(t *testing.T) {
	_, err := NewRetentionService(config.StorageConfig{Backend: "bogus"})
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
	if !strings.Contains(err.Error(), "unknown backend") {
		t.Errorf("error: %q", err.Error())
	}
}

func TestNewRetentionService_TursoDSNParseFailsWithoutNetwork(t *testing.T) {
	// buildDSN rejects an empty database URL at construction time, before
	// any network access.
	_, err := NewRetentionService(config.StorageConfig{
		Backend:    "turso",
		TursoURL:   "",
		TursoToken: "",
	})
	if err == nil {
		t.Fatal("expected DSN parse error without network")
	}
}
