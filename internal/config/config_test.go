// Package config provides configuration loading and validation.
package config

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
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
	if cfg.Env != "dev" {
		t.Errorf("Env = %q; want %q", cfg.Env, "dev")
	}
	if len(cfg.Sources) != 0 {
		t.Errorf("Sources = %v; want empty", cfg.Sources)
	}
}

func TestLoadEmptyConfigFile(t *testing.T) {
	// Empty string = no file, should use defaults
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load('') error = %v; want nil", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q; want %q", cfg.LogLevel, "info")
	}
}

func TestLoadExplicitMissingFile(t *testing.T) {
	// Explicit non-existent file should return error
	_, err := Load("/nonexistent/.env")
	if err == nil {
		t.Fatalf("Load('/nonexistent/.env') error = nil; want error for missing explicit file")
	}
	if err.Error() == "" || !contains(err.Error(), "not found") {
		t.Errorf("Load('/nonexistent/.env') error = %v; want 'not found'", err)
	}
}

func TestLoadFromEnv(t *testing.T) {
	// Set env vars and load
	_ = os.Setenv("HSC_LOG_LEVEL", "debug")
	_ = os.Setenv("HSC_USER_AGENT", "test-agent/1.0")
	_ = os.Setenv("HSC_REQUEST_TIMEOUT", "60s")
	_ = os.Setenv("HSC_HUGO_BASE_URL", "http://localhost:1313")
	_ = os.Setenv("HSC_STORAGE_BACKEND", "turso")
	_ = os.Setenv("HSC_ENV", "production")
	_ = os.Setenv("TURSO_DATABASE_URL", "test-db")
	_ = os.Setenv("TURSO_AUTH_TOKEN", "test-token")
	defer func() {
		_ = os.Unsetenv("HSC_LOG_LEVEL")
		_ = os.Unsetenv("HSC_USER_AGENT")
		_ = os.Unsetenv("HSC_REQUEST_TIMEOUT")
		_ = os.Unsetenv("HSC_HUGO_BASE_URL")
		_ = os.Unsetenv("HSC_STORAGE_BACKEND")
		_ = os.Unsetenv("HSC_ENV")
		_ = os.Unsetenv("TURSO_DATABASE_URL")
		_ = os.Unsetenv("TURSO_AUTH_TOKEN")
	}()

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

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
	if cfg.Env != "production" {
		t.Errorf("Env = %q; want %q", cfg.Env, "production")
	}
	if cfg.TursoDatabaseURL != "test-db" {
		t.Errorf("TursoDatabaseURL = %q; want %q", cfg.TursoDatabaseURL, "test-db")
	}
	if cfg.TursoAuthToken != "test-token" {
		t.Errorf("TursoAuthToken = %q; want %q", cfg.TursoAuthToken, "test-token")
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
HSC_STORAGE_BACKEND=sqlite-file
HSC_ENV=dev
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
	if cfg.StorageBackend != "sqlite-file" {
		t.Errorf("StorageBackend = %q; want %q", cfg.StorageBackend, "sqlite-file")
	}
	if cfg.Env != "dev" {
		t.Errorf("Env = %q; want %q", cfg.Env, "dev")
	}
}

func TestLoadFileInvalidDuration(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")

	content := `HSC_REQUEST_TIMEOUT=invalid`
	if err := os.WriteFile(envFile, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(envFile)
	if err == nil {
		t.Fatalf("Load() error = nil; want invalid duration error")
	}
	if !contains(err.Error(), "invalid duration") {
		t.Errorf("Load() error = %v; want 'invalid duration'", err)
	}
}

func TestLoadFileInvalidLine(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")

	content := `HSC_LOG_LEVEL=warn
invalid line without equals
`
	if err := os.WriteFile(envFile, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(envFile)
	if err == nil {
		t.Fatalf("Load() error = nil; want invalid line error")
	}
	if !contains(err.Error(), "invalid line") {
		t.Errorf("Load() error = %v; want 'invalid line'", err)
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

	// Set env var override with valid log level
	_ = os.Setenv("HSC_LOG_LEVEL", "warn")
	defer func() { _ = os.Unsetenv("HSC_LOG_LEVEL") }()

	cfg, err := Load(envFile)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Env vars should override file
	if cfg.LogLevel != "warn" {
		t.Errorf("LogLevel = %q; want env override %q", cfg.LogLevel, "warn")
	}
	if cfg.UserAgent != "file_agent" {
		t.Errorf("UserAgent = %q; want file value %q", cfg.UserAgent, "file_agent")
	}
}

func TestLoadSources(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")

	content := `HSC_SOURCES=nfl-news,NFL News,http://example.com,300
`
	if err := os.WriteFile(envFile, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(envFile)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(cfg.Sources) != 1 {
		t.Fatalf("Sources = %v; want 1", len(cfg.Sources))
	}
	if cfg.Sources[0].ID != "nfl-news" {
		t.Errorf("Sources[0].ID = %q; want %q", cfg.Sources[0].ID, "nfl-news")
	}
	if cfg.Sources[0].Name != "NFL News" {
		t.Errorf("Sources[0].Name = %q; want %q", cfg.Sources[0].Name, "NFL News")
	}
	if cfg.Sources[0].URL != "http://example.com" {
		t.Errorf("Sources[0].URL = %q; want %q", cfg.Sources[0].URL, "http://example.com")
	}
	if cfg.Sources[0].FetchSeconds != 300 {
		t.Errorf("Sources[0].FetchSeconds = %d; want %d", cfg.Sources[0].FetchSeconds, 300)
	}
}

func TestLoadSourcesMultiple(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")

	content := `HSC_SOURCES=nfl-news,NFL News,http://example.com,300;nfl-updates,NFL Updates,http://other.com
`
	if err := os.WriteFile(envFile, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(envFile)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(cfg.Sources) != 2 {
		t.Fatalf("Sources = %v; want 2", len(cfg.Sources))
	}

	if cfg.Sources[0].ID != "nfl-news" {
		t.Errorf("Sources[0].ID = %q; want %q", cfg.Sources[0].ID, "nfl-news")
	}
	if cfg.Sources[1].ID != "nfl-updates" {
		t.Errorf("Sources[1].ID = %q; want %q", cfg.Sources[1].ID, "nfl-updates")
	}
	if cfg.Sources[1].FetchSeconds != 15*60 {
		t.Errorf("Sources[1].FetchSeconds = %d; want %d (default)", cfg.Sources[1].FetchSeconds, 15*60)
	}
}

func TestLoadSourcesDuplicateID(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")

	content := `HSC_SOURCES=nfl-news,News,http://a.com;nfl-news,Duplicate,http://b.com
`
	if err := os.WriteFile(envFile, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(envFile)
	if err == nil {
		t.Fatalf("Load() error = nil; want duplicate ID error")
	}
	if !contains(err.Error(), "duplicate") {
		t.Errorf("Load() error = %v; want 'duplicate'", err)
	}
}

func TestLoadSourcesInvalidURL(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")

	content := `HSC_SOURCES=nfl-news,News,not-a-url
`
	if err := os.WriteFile(envFile, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Load(envFile)
	if err == nil {
		t.Fatalf("Load() error = nil; want invalid URL error")
	}
	if !contains(err.Error(), "URL") {
		t.Errorf("Load() error = %v; want 'URL'", err)
	}
}

func TestLoadSourcesEnvOverride(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")

	content := `HSC_SOURCES=file-sources,File,http://file.com
`
	if err := os.WriteFile(envFile, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_ = os.Setenv("HSC_SOURCES", "env-sources,Env,http://env.com")
	defer func() { _ = os.Unsetenv("HSC_SOURCES") }()

	cfg, err := Load(envFile)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(cfg.Sources) != 1 || cfg.Sources[0].ID != "env-sources" {
		t.Errorf("Sources = %v; want env override", cfg.Sources)
	}
}

func TestParseSourcesRaw(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		errMatch  string
		wantCount int
	}{
		{"empty", "", "", 0},
		{"single", "id,name,http://url.com", "", 1},
		{"multiple", "id1,name1,http://a.com;id2,name2,http://b.com", "", 2},
		{"with-interval", "id,name,http://url.com,300", "", 1},
		{"whitespace", "  id  ,  name  ,  http://url.com  ,  300  ", "", 1},
		{"empty-id", ",name,http://url.com", "empty ID", 0},
		{"invalid-url", "id,name,not-a-url", "URL", 0},
		{"duplicate", "id,name,http://a.com;id,dup,http://b.com", "duplicate", 0},
		{"interval-parse", "id,name,http://url.com,invalid", "interval", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{}
			err := cfg.parseSourcesRaw(tt.raw)
			if tt.errMatch == "" {
				if err != nil {
					t.Errorf("parseSourcesRaw(%q) error = %v; want nil", tt.raw, err)
				}
				if len(cfg.Sources) != tt.wantCount {
					t.Errorf("len(Sources) = %d; want %d", len(cfg.Sources), tt.wantCount)
				}
			} else {
				if err == nil {
					t.Errorf("parseSourcesRaw(%q) error = nil; want error containing %q", tt.raw, tt.errMatch)
				} else if !contains(err.Error(), tt.errMatch) {
					t.Errorf("parseSourcesRaw(%q) error = %v; want error containing %q", tt.raw, err, tt.errMatch)
				}
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name     string
		cfg      Config
		valid    bool
		errMatch string
	}{
		{"valid-dev", Config{LogLevel: "info", UserAgent: "test/1", RequestTimeout: 30 * time.Second, HugoBaseURL: "https://example.com"}, true, ""},
		{"valid-production", Config{LogLevel: "info", UserAgent: "test/1", RequestTimeout: 30 * time.Second, HugoBaseURL: "https://example.com", StorageBackend: "turso", TursoDatabaseURL: "db", TursoAuthToken: "token", Env: "production"}, true, ""},
		{"invalid-log-level", Config{LogLevel: "invalid"}, false, "invalid log level"},
		{"empty-user-agent", Config{UserAgent: ""}, false, "UserAgent"},
		{"long-user-agent", Config{UserAgent: strings.Repeat("a", 513)}, false, "UserAgent too long"},
		{"invalid-timeout-zero", Config{RequestTimeout: 0}, false, "RequestTimeout"},
		{"invalid-timeout-negative", Config{RequestTimeout: -1 * time.Second}, false, "RequestTimeout"},
		{"invalid-hugo-url", Config{HugoBaseURL: "ftp://example.com"}, false, "HugoBaseURL"},
		{"invalid-storage-backend", Config{StorageBackend: "unknown"}, false, "StorageBackend"},
		{"production-missing-storage", Config{Env: "production", TursoDatabaseURL: "db", TursoAuthToken: "token"}, false, "StorageBackend"},
		{"production-missing-db", Config{StorageBackend: "turso", Env: "production", TursoAuthToken: "token"}, false, "TURSO_DATABASE_URL"},
		{"production-missing-token", Config{StorageBackend: "turso", Env: "production", TursoDatabaseURL: "db"}, false, "TURSO_AUTH_TOKEN"},
		{"sources-empty-id", Config{Sources: []Source{{Name: "n", URL: "http://x.com"}}}, false, "empty ID"},
		{"sources-empty-url", Config{Sources: []Source{{ID: "id", Name: "n"}}}, false, "empty URL"},
		{"sources-invalid-url", Config{Sources: []Source{{ID: "id", Name: "n", URL: "not-a-url"}}}, false, "URL"},
		{"sources-invalid-scheme", Config{Sources: []Source{{ID: "id", Name: "n", URL: "ftp://x.com"}}}, false, "http or https"},
		{"sources-negative-interval", Config{Sources: []Source{{ID: "id", Name: "n", URL: "http://x.com", FetchSeconds: -1}}}, false, "FetchSeconds"},
		{"invalid-env", Config{Env: "unknown"}, false, "env must be"},
		{"all-errors-collected", Config{LogLevel: "bad", UserAgent: "", RequestTimeout: 0, StorageBackend: "bad", Env: "production"}, false, "UserAgent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.valid && err != nil {
				t.Errorf("Validate() error = %v; want nil", err)
			}
			if !tt.valid && err == nil {
				t.Errorf("Validate() error = nil; want error containing %q", tt.errMatch)
			}
			if !tt.valid && err != nil && !contains(err.Error(), tt.errMatch) {
				t.Errorf("Validate() error = %v; want error containing %q", err, tt.errMatch)
			}
		})
	}
}

func TestValidateErrorsJoin(t *testing.T) {
	cfg := Config{
		LogLevel:       "invalid",
		UserAgent:      "",
		RequestTimeout: 0,
		StorageBackend: "unknown",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatalf("Validate() error = nil; want errors")
	}
	// Count occurrences of expected error strings
	errStr := err.Error()
	count := 0
	for _, substr := range []string{"log level", "UserAgent", "RequestTimeout", "StorageBackend"} {
		if contains(errStr, substr) {
			count++
		}
	}
	if count < 4 {
		t.Errorf("Validate() error = %v; should contain all 4 error parts", err)
	}
}

func TestMarshalJSON(t *testing.T) {
	cfg := Config{
		TursoAuthToken: "secret-token-123",
		LogLevel:       "info",
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	// Token should not appear in JSON output
	tokenVal := result["TursoAuthToken"]
	if tokenVal != nil && tokenVal != "" && tokenVal != "******" {
		t.Errorf("json.Marshal() TursoAuthToken = %v; should be redacted", tokenVal)
	}

	// Other fields should be present
	if result["LogLevel"] != "info" {
		t.Errorf("json.Marshal() LogLevel = %v; want %q", result["LogLevel"], "info")
	}
}

func TestMarshalJSONNoSecret(t *testing.T) {
	cfg := Config{
		TursoAuthToken: "my-secret-token",
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	// Check that the raw bytes don't contain the secret
	if bytes.Contains(data, []byte("my-secret-token")) {
		t.Errorf("json.Marshal() contains secret in output: %s", string(data))
	}
}

func TestRedacted(t *testing.T) {
	cfg := Config{
		TursoAuthToken:   "secret",
		TursoDatabaseURL: "https://db.example.com",
		LogLevel:         "debug",
	}
	redacted := cfg.Redacted()

	if redacted.TursoAuthToken != "***" {
		t.Errorf("Redacted().TursoAuthToken = %q; want %q", redacted.TursoAuthToken, "***")
	}
	if redacted.TursoDatabaseURL != "https://db.example.com" {
		t.Errorf("Redacted().TursoDatabaseURL = %q; want original", redacted.TursoDatabaseURL)
	}
	if redacted.LogLevel != "debug" {
		t.Errorf("Redacted().LogLevel = %q; want original", redacted.LogLevel)
	}

	// Original should be unchanged
	if cfg.TursoAuthToken != "secret" {
		t.Errorf("Original Config.TursoAuthToken = %q; should be unchanged", cfg.TursoAuthToken)
	}
}

func TestIdempotencyLoad(t *testing.T) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")

	content := `HSC_LOG_LEVEL=warn
HSC_USER_AGENT=test-agent
`
	if err := os.WriteFile(envFile, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg1, err1 := Load(envFile)
	cfg2, err2 := Load(envFile)

	if err1 != nil || err2 != nil {
		t.Fatalf("Load() errors: err1=%v, err2=%v", err1, err2)
	}

	if !reflect.DeepEqual(cfg1, cfg2) {
		t.Errorf("Load() not idempotent: cfg1 != cfg2")
	}
}

func TestIdempotencyValidate(t *testing.T) {
	cfg := Config{
		LogLevel:       "invalid",
		UserAgent:      "",
		RequestTimeout: 0,
	}
	err1 := cfg.Validate()
	err2 := cfg.Validate()

	if err1.Error() != err2.Error() {
		t.Errorf("Validate() not idempotent: err1=%v, err2=%v", err1, err2)
	}
}

func TestLoadFileMissing(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") error = %v; want nil for empty path", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q; want default %q", cfg.LogLevel, "info")
	}
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
		{"key=value#no-space", "key=value"},
	}

	for _, tt := range tests {
		result := trimComment(tt.input)
		if result != tt.expected {
			t.Errorf("trimComment(%q) = %q; want %q", tt.input, result, tt.expected)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestParseSourcesRawExtendedFields tests the extended field parsing.
func TestParseSourcesRawExtendedFields(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		errMatch  string
		wantCount int
		checks    func([]Source)
	}{
		{
			"basic only",
			"id,name,http://url.com",
			"", 1,
			func(srcs []Source) {
				if len(srcs) != 1 || srcs[0].ID != "id" {
					t.Errorf("basic: parsed incorrectly")
				}
			},
		},
		{
			"with interval",
			"id,name,http://url.com,300",
			"", 1,
			func(srcs []Source) {
				if srcs[0].FetchSeconds != 300 {
					t.Errorf("FetchSeconds = %d; want 300", srcs[0].FetchSeconds)
				}
			},
		},
		{
			"with all fields",
			"espn-nfl,ESPN NFL,https://espn.com/nfl/rss,300,1,true,ESPN,en,60,RSS syndication permitted,allowed",
			"", 1,
			func(srcs []Source) {
				src := srcs[0]
				if src.ID != "espn-nfl" {
					t.Errorf("ID = %s; want espn-nfl", src.ID)
				}
				if src.Publication != "ESPN" {
					t.Errorf("Publication = %s; want ESPN", src.Publication)
				}
				if src.Priority != 1 {
					t.Errorf("Priority = %d; want 1", src.Priority)
				}
				if !src.Enabled {
					t.Error("Enabled = false; want true")
				}
				if src.Language != "en" {
					t.Errorf("Language = %s; want en", src.Language)
				}
				if src.Timeout != 60 {
					t.Errorf("Timeout = %d; want 60", src.Timeout)
				}
				if src.License != "RSS syndication permitted" {
					t.Errorf("License = %s; want RSS syndication permitted", src.License)
				}
				if src.AcceptableUse != "allowed" {
					t.Errorf("AcceptableUse = %s; want allowed", src.AcceptableUse)
				}
			},
		},
		{
			"only required and enabled",
			"id,name,http://url.com,0,0,false",
			"", 1,
			func(srcs []Source) {
				if srcs[0].Enabled {
					t.Error("Enabled should be false")
				}
			},
		},
		{
			"invalid enabled value",
			"id,name,http://url.com,0,0,maybe", // enabled=maybe (invalid)
			"enabled", 0,
			func(srcs []Source) {},
		},
		{
			"invalid accept-use",
			"id,name,http://url.com,0,0,false,\"\",en,0,,invalid", // priority=0, enabled=false, pub="", lang=en, timeout=0, license="", accept-use=invalid
			"accept-use", 0,
			func(srcs []Source) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{}
			err := cfg.parseSourcesRaw(tt.raw)
			if tt.errMatch == "" {
				if err != nil {
					t.Errorf("parseSourcesRaw(%q) error = %v; want nil", tt.raw, err)
				}
				if len(cfg.Sources) != tt.wantCount {
					t.Errorf("len(Sources) = %d; want %d", len(cfg.Sources), tt.wantCount)
				}
				if tt.checks != nil {
					tt.checks(cfg.Sources)
				}
			} else {
				if err == nil {
					t.Errorf("parseSourcesRaw(%q) error = nil; want error containing %q", tt.raw, tt.errMatch)
				} else if !contains(err.Error(), tt.errMatch) {
					t.Errorf("parseSourcesRaw(%q) error = %v; want error containing %q", tt.raw, err, tt.errMatch)
				}
			}
		})
	}
}

// TestConfig_ToDomain tests the ToDomain mapping.
func TestConfig_ToDomain(t *testing.T) {
	cfg := Config{
		Sources: []Source{
			{
				ID:            "test",
				URL:           "https://example.com",
				Publication:   "Test Publication",
				Priority:      1,
				Enabled:       true,
				Language:      "en",
				Timeout:       60,
				License:       "Free to syndicate",
				AcceptableUse: "allowed",
			},
		},
	}

	domainSrcs := cfg.ToDomain()
	if len(domainSrcs) != 1 {
		t.Fatalf("ToDomain() returned %d sources; want 1", len(domainSrcs))
	}

	dom := domainSrcs[0]
	if dom.ID != "test" {
		t.Errorf("domain ID = %s; want test", dom.ID)
	}
	if dom.Publication != "Test Publication" {
		t.Errorf("domain Publication = %s; want Test Publication", dom.Publication)
	}
	if dom.Priority != 1 {
		t.Errorf("domain Priority = %d; want 1", dom.Priority)
	}
	if !dom.Enabled {
		t.Error("domain Enabled = false; want true")
	}
	if dom.Language != "en" {
		t.Errorf("domain Language = %s; want en", dom.Language)
	}
	if dom.Timeout != 60*time.Second {
		t.Errorf("domain Timeout = %v; want 60s", dom.Timeout)
	}
	if dom.License != "Free to syndicate" {
		t.Errorf("domain License = %s; want Free to syndicate", dom.License)
	}
	if dom.AcceptableUse != domain.AcceptanceAllowed {
		t.Errorf("domain AcceptableUse = %s; want allowed", dom.AcceptableUse.String())
	}
}
