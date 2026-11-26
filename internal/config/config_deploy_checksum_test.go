package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadEnvFile_IgnoresDeployChecksumKey (issue #202 R5) asserts that the
// env-file loader ignores HSC_INSTALLED_SHA256. scripts/deploy.sh appends this
// key to the installed env file to record the on-disk binary checksum
// (AC-5/AC-16); the deploy breaks if a future strict-mode change makes the
// loader reject unknown keys. This test pins the permissive contract.
func TestLoadEnvFile_IgnoresDeployChecksumKey(t *testing.T) {
	content := "HSC_ENV=production\nHSC_INSTALLED_SHA256=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n"
	path := filepath.Join(t.TempDir(), "huddlescope.env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	var cfg Config
	if err := loadEnvFile(path, &cfg); err != nil {
		t.Fatalf("loadEnvFile must not reject the deploy checksum key (permissive loader), got %v", err)
	}
	if cfg.Env != "production" {
		t.Errorf("Env: want production, got %q", cfg.Env)
	}
}
