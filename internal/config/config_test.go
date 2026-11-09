package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q; want %q", cfg.LogLevel, "info")
	}
	if cfg.UserAgent != "huddlescope/0.1" {
		t.Errorf("UserAgent = %q; want %q", cfg.UserAgent, "huddlescope/0.1")
	}
	if cfg.RequestTimeout != 30*time.Second {
		t.Errorf("RequestTimeout = %v; want %v", cfg.RequestTimeout, 30*time.Second)
	}
	if cfg.HugoBaseURL != "https://huddlescope.github.io/" {
		t.Errorf("HugoBaseURL = %q; want %q", cfg.HugoBaseURL, "https://huddlescope.github.io/")
	}
	if cfg.StorageBackend != "memory" {
		t.Errorf("StorageBackend = %q; want %q", cfg.StorageBackend, "memory")
	}
}

func TestLoadFromEnv(t *testing.T) {
	// Set env vars and load
	os.Setenv("HSC_LOG_LEVEL", "debug")
	os.Setenv("HSC_USER_AGENT", "test-agent/1.0")
	os.Setenv("HSC_REQUEST_TIMEOUT", "60s")
	os.Setenv("HSC_HUGO_BASE_URL", "http://localhost:1313")
	os.Setenv("HSC_STORAGE_BACKEND", "turso")
	defer func() {
		os.Unsetenv("HSC_LOG_LEVEL")
		os.Unsetenv("HSC_USER_AGENT")
		os.Unsetenv("HSC_REQUEST_TIMEOUT")
		os.Unsetenv("HSC_HUGO_BASE_URL")
		os.Unsetenv("HSC_STORAGE_BACKEND")
	}()

	cfg := Config{
		LogLevel:       "info",
		UserAgent:      "huddlescope/0.1",
		RequestTimeout: 30 * time.Second,
		HugoBaseURL:    "https://huddlescope.github.io/",
		StorageBackend: "memory",
	}
	cfg.loadFromEnv()

	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q; want %q", cfg.LogLevel, "debug")
	}
	if cfg.UserAgent != "test-agent/1.0" {
		t.Errorf("UserAgent = %q; want %q", cfg.UserAgent, "test-agent/1.0")
	}
	if cfg.RequestTimeout != 60*time.Second {
		t.Errorf("RequestTimeout = %v; want %v", cfg.RequestTimeout, 60*time.Second)
	}
	if cfg.HugoBaseURL != "http://localhost:1313" {
		t.Errorf("HugoBaseURL = %q; want %q", cfg.HugoBaseURL, "http://localhost:1313")
	}
	if cfg.StorageBackend != "turso" {
		t.Errorf("StorageBackend = %q; want %q", cfg.StorageBackend, "turso")
	}
}

func TestLoadFile(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")

	content := `# Test environment file
HSC_LOG_LEVEL=warn
HSC_USER_AGENT=file-agent/1.0
HSC_REQUEST_TIMEOUT=45s
HSC_HUGO_BASE_URL=http://example.com
HSC_STORAGE_BACKEND=file
`
	if err := os.WriteFile(envFile, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(envFile)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.LogLevel != "warn" {
		t.Errorf("LogLevel = %q; want %q", cfg.LogLevel, "warn")
	}
	if cfg.UserAgent != "file-agent/1.0" {
		t.Errorf("UserAgent = %q; want %q", cfg.UserAgent, "file-agent/1.0")
	}
	if cfg.RequestTimeout != 45*time.Second {
		t.Errorf("RequestTimeout = %v; want %v", cfg.RequestTimeout, 45*time.Second)
	}
	if cfg.HugoBaseURL != "http://example.com" {
		t.Errorf("HugoBaseURL = %q; want %q", cfg.HugoBaseURL, "http://example.com")
	}
	if cfg.StorageBackend != "file" {
		t.Errorf("StorageBackend = %q; want %q", cfg.StorageBackend, "file")
	}
}

func TestLoadFileMissing(t *testing.T) {
	cfg, err := Load("/nonexistent/.env")
	if err != nil {
		t.Fatalf("Load() error = %v; want nil for missing file", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q; want default %q", cfg.LogLevel, "info")
	}
}

func TestLoadFromEnvPrecedence(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")

	content := `HSC_LOG_LEVEL=file_level
HSC_USER_AGENT=file_agent
`
	if err := os.WriteFile(envFile, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Set env var override
	os.Setenv("HSC_LOG_LEVEL", "env_level")
	defer os.Unsetenv("HSC_LOG_LEVEL")

	cfg, err := Load(envFile)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Env vars should override file
	if cfg.LogLevel != "env_level" {
		t.Errorf("LogLevel = %q; want env override %q", cfg.LogLevel, "env_level")
	}
	if cfg.UserAgent != "file_agent" {
		t.Errorf("UserAgent = %q; want file value %q", cfg.UserAgent, "file_agent")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		level  string
		valid  bool
		errStr string
	}{
		{"debug", true, ""},
		{"info", true, ""},
		{"warn", true, ""},
		{"error", true, ""},
		{"invalid", false, "invalid log level"},
		{"", true, ""}, // empty is OK
	}

	for _, tt := range tests {
		cfg := Config{LogLevel: tt.level}
		err := cfg.Validate()
		if tt.valid && err != nil {
			t.Errorf("Validate(%q) error = %v; want nil", tt.level, err)
		}
		if !tt.valid && err == nil {
			t.Errorf("Validate(%q) error = nil; want error containing %q", tt.level, tt.errStr)
		}
		if !tt.valid && err != nil && !contains(err.Error(), tt.errStr) {
			t.Errorf("Validate(%q) error = %v; want error containing %q", tt.level, err, tt.errStr)
		}
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestTrimComment(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"key=value", "key=value"},
		{"key=value # comment", "key=value"},
		{"# full comment", ""},
		{"  key=value  # comment  ", "key=value"},
	}

	for _, tt := range tests {
		result := trimComment(tt.input)
		if result != tt.expected {
			t.Errorf("trimComment(%q) = %q; want %q", tt.input, result, tt.expected)
		}
	}
}
