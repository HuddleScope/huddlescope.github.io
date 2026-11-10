package sqlite

import (
	"testing"
)

// TestListMigrations verifies that migration files are discovered.
func TestListMigrations(t *testing.T) {
	migrations, err := ListMigrations()
	if err != nil {
		t.Fatalf("ListMigrations failed: %v", err)
	}
	if len(migrations) == 0 {
		t.Fatal("no migrations found")
	}
	t.Logf("found %d migrations: %v", len(migrations), migrations)
}

// TestReadMigration verifies reading a migration file.
func TestReadMigration(t *testing.T) {
	migrations, err := ListMigrations()
	if err != nil {
		t.Fatalf("ListMigrations failed: %v", err)
	}
	for _, m := range migrations {
		sql, err := ReadMigration(m)
		if err != nil {
			t.Errorf("read %s: %v", m, err)
			continue
		}
		if len(sql) == 0 {
			t.Errorf("empty migration: %s", m)
			continue
		}
		t.Logf("%s: %d bytes", m, len(sql))
	}
}

// TestMigrateNotImplemented checks that Migrate is not yet implemented.
func TestMigrateNotImplemented(t *testing.T) {
	_, err := Migrate(":memory:")
	if err == nil {
		t.Error("expected error from unimplemented Migrate")
	}
}
