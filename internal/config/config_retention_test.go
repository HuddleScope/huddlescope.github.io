package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRetentionEnvParsing(t *testing.T) {
	t.Run("enabled true and days", func(t *testing.T) {
		t.Setenv("HSC_RETENTION_ENABLED", "true")
		t.Setenv("HSC_RETENTION_DAYS", "45")
		cfg, err := Load("")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if !cfg.RetentionEnabled {
			t.Error("RetentionEnabled: want true")
		}
		if cfg.RetentionWindowDays != 45 {
			t.Errorf("RetentionWindowDays: want 45, got %d", cfg.RetentionWindowDays)
		}
	})

	t.Run("disabled with off", func(t *testing.T) {
		t.Setenv("HSC_RETENTION_ENABLED", "off")
		cfg, err := Load("")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.RetentionEnabled {
			t.Error("RetentionEnabled: want false")
		}
	})

	t.Run("invalid days ignored", func(t *testing.T) {
		t.Setenv("HSC_RETENTION_DAYS", "not-a-number")
		cfg, err := Load("")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.RetentionWindowDays != 90 {
			t.Errorf("invalid HSC_RETENTION_DAYS should be ignored, got %d", cfg.RetentionWindowDays)
		}
	})
}

func TestRetentionValidation(t *testing.T) {
	t.Run("negative window invalid", func(t *testing.T) {
		cfg, err := Load("")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		cfg.RetentionWindowDays = -5
		if err := cfg.Validate(); err == nil {
			t.Error("negative RetentionWindowDays should fail validation")
		}
	})
	t.Run("zero window accepted (unset)", func(t *testing.T) {
		cfg, err := Load("")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		cfg.RetentionWindowDays = 0
		if err := cfg.Validate(); err != nil {
			t.Errorf("zero RetentionWindowDays should be accepted, got %v", err)
		}
	})
}

func TestLoadEnvFile_Retention(t *testing.T) {
	content := "HSC_RETENTION_ENABLED=false\nHSC_RETENTION_DAYS=30\n"
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	cfg := Config{RetentionWindowDays: 90}
	if err := loadEnvFile(path, &cfg); err != nil {
		t.Fatalf("loadEnvFile: %v", err)
	}
	if cfg.RetentionEnabled {
		t.Error("RetentionEnabled: want false from file")
	}
	if cfg.RetentionWindowDays != 30 {
		t.Errorf("RetentionWindowDays: want 30, got %d", cfg.RetentionWindowDays)
	}
}
