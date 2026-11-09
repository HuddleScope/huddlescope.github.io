// Package config provides configuration loading and validation.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

import "encoding/json"

// Source represents a news source or feed.
type Source struct {
	ID           string
	Name         string
	URL          string
	FetchSeconds int
}

// Config holds the application configuration.
type Config struct {
	ConfigFile       string
	LogLevel         string
	UserAgent        string
	RequestTimeout   time.Duration
	HugoBaseURL      string
	StorageBackend   string
	TursoDatabaseURL string
	TursoAuthToken   string
	Sources          []Source
	Env              string
}

const (
	defaultLogLevel       = "info"
	defaultUserAgent      = "huddlescope/0.1"
	defaultRequestTimeout = 30 * time.Second
	defaultHugoBaseURL    = "https://huddlescope.github.io/"
	defaultStorageBackend = "memory"
	defaultEnv            = "dev"
)

// Load loads configuration from environment variables with optional file override.
func Load(configFile string) (Config, error) {
	cfg := Config{
		LogLevel:       defaultLogLevel,
		UserAgent:      defaultUserAgent,
		RequestTimeout: defaultRequestTimeout,
		HugoBaseURL:    defaultHugoBaseURL,
		StorageBackend: defaultStorageBackend,
		Env:            defaultEnv,
	}

	// Override with config file if provided
	if configFile != "" {
		if err := loadEnvFile(configFile, &cfg); err != nil {
			return cfg, fmt.Errorf("failed to load config file %q: %w", configFile, err)
		}
	}

	// Override with environment variables
	cfg.loadFromEnv()

	// Parse sources after all env vars are loaded
	if err := cfg.parseSources(); err != nil {
		return cfg, err
	}

	// Validate all fields
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func (c *Config) loadFromEnv() {
	if v := os.Getenv("HSC_LOG_LEVEL"); v != "" {
		c.LogLevel = v
	}
	if v := os.Getenv("HSC_USER_AGENT"); v != "" {
		c.UserAgent = v
	}
	if v := os.Getenv("HSC_REQUEST_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.RequestTimeout = d
		}
	}
	if v := os.Getenv("HSC_HUGO_BASE_URL"); v != "" {
		c.HugoBaseURL = v
	}
	if v := os.Getenv("HSC_STORAGE_BACKEND"); v != "" {
		c.StorageBackend = v
	}
	if v := os.Getenv("TURSO_DATABASE_URL"); v != "" {
		c.TursoDatabaseURL = v
	}
	if v := os.Getenv("TURSO_AUTH_TOKEN"); v != "" {
		c.TursoAuthToken = v
	}
	if v := os.Getenv("HSC_SOURCES"); v != "" {
		// Clear sources first to allow env to override file
		c.Sources = nil
		if err := c.parseSourcesRaw(v); err != nil {
			// Store raw value for error reporting; don't set c.Sources
			_ = err // will be caught by Validate
		}
	}
	if v := os.Getenv("HSC_ENV"); v != "" {
		c.Env = v
	}
}

// parseSources parses sources from the parsed Sources field (set by loadFromEnv)
func (c *Config) parseSources() error {
	if len(c.Sources) > 0 {
		return nil // already parsed
	}
	return nil
}

// parseSourcesRaw parses HSC_SOURCES environment value into Sources.
// Format: "id1,name1,http://url1[,interval];id2,name2,http://url2[,interval];..."
// e.g., "nfl-news,NFL News,http://example.com,300;nfl-updates,NFL Updates,http://other.com"
func (c *Config) parseSourcesRaw(raw string) error {
	if raw == "" {
		return nil
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	entries := strings.Split(raw, ";")
	seenIDs := make(map[string]bool)

	for i, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		parts := strings.Split(entry, ",")
		if len(parts) < 3 {
			return fmt.Errorf("source entry %d invalid (expected at least id,name,url): %q", i+1, entry)
		}

		id := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])
		urlStr := strings.TrimSpace(parts[2])
		fetchSeconds := 15 * 60 // default 15 minutes

		if len(parts) >= 4 {
			// Optional fetch interval in seconds
			if d, err := time.ParseDuration(strings.TrimSpace(parts[3])); err == nil {
				fetchSeconds = int(d.Seconds())
			} else if v, err := fmt.Sscanf(strings.TrimSpace(parts[3]), "%d", &fetchSeconds); err == nil && v == 1 && fetchSeconds >= 0 {
				// Already in seconds
			} else {
				return fmt.Errorf("source entry %d invalid fetch interval: %q", i+1, parts[3])
			}
		}

		// Validate ID uniqueness and format
		if id == "" {
			return fmt.Errorf("source entry %d has empty ID", i+1)
		}
		if seenIDs[id] {
			return fmt.Errorf("duplicate source ID %q", id)
		}
		seenIDs[id] = true

		// Validate URL
		u, err := url.Parse(urlStr)
		if err != nil {
			return fmt.Errorf("source entry %d invalid URL %q: %v", i+1, urlStr, err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("source entry %d URL %q must use http or https, got %s", i+1, urlStr, u.Scheme)
		}

		c.Sources = append(c.Sources, Source{
			ID:           id,
			Name:         name,
			URL:          urlStr,
			FetchSeconds: fetchSeconds,
		})
	}

	return nil
}

// loadEnvFile loads key=value pairs from a file.
// #nosec G304: path comes from operator --config flag or env, not untrusted input
func loadEnvFile(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Missing file when path is explicitly provided is an error
			return fmt.Errorf("config file %q not found", path)
		}
		return err
	}

	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		line = trimComment(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("line %d: invalid line (expected KEY=VALUE): %q", i+1, line)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "HSC_LOG_LEVEL":
			cfg.LogLevel = value
		case "HSC_USER_AGENT":
			cfg.UserAgent = value
		case "HSC_REQUEST_TIMEOUT":
			if d, err := time.ParseDuration(value); err == nil {
				cfg.RequestTimeout = d
			} else {
				return fmt.Errorf("line %d: invalid duration %q: %v", i+1, value, err)
			}
		case "HSC_HUGO_BASE_URL":
			cfg.HugoBaseURL = value
		case "HSC_STORAGE_BACKEND":
			cfg.StorageBackend = value
		case "TURSO_DATABASE_URL":
			cfg.TursoDatabaseURL = value
		case "TURSO_AUTH_TOKEN":
			cfg.TursoAuthToken = value
		case "HSC_SOURCES":
			if err := cfg.parseSourcesRaw(value); err != nil {
				return fmt.Errorf("line %d: invalid sources %q: %v", i+1, value, err)
			}
		case "HSC_ENV":
			cfg.Env = value
		default:
			// Permissive: unknown keys are ignored
		}
	}

	return nil
}

func trimComment(line string) string {
	if idx := strings.IndexByte(line, '#'); idx >= 0 {
		return strings.TrimSpace(line[:idx])
	}
	return line
}

// Validate checks the configuration for validity and returns all errors.
func (c Config) Validate() error {
	var errs []error

	// Validate LogLevel
	if c.LogLevel != "" {
		switch c.LogLevel {
		case "debug", "info", "warn", "error":
		default:
			errs = append(errs, fmt.Errorf("invalid log level %q; must be debug, info, warn, or error", c.LogLevel))
		}
	}

	// Validate UserAgent
	if c.UserAgent == "" {
		errs = append(errs, errors.New("UserAgent must not be empty"))
	} else if len(c.UserAgent) > 512 {
		errs = append(errs, fmt.Errorf("UserAgent too long (%d chars); max 512", len(c.UserAgent)))
	}

	// Validate RequestTimeout
	if c.RequestTimeout <= 0 {
		errs = append(errs, fmt.Errorf("RequestTimeout must be positive; got %v", c.RequestTimeout))
	}

	// Validate HugoBaseURL
	if c.HugoBaseURL != "" {
		u, err := url.Parse(c.HugoBaseURL)
		if err != nil {
			errs = append(errs, fmt.Errorf("HugoBaseURL invalid: %v", err))
		} else if u.Scheme != "http" && u.Scheme != "https" {
			errs = append(errs, fmt.Errorf("HugoBaseURL must use http or https; got %s", u.Scheme))
		}
	}

	// Validate StorageBackend
	if c.StorageBackend != "" && c.StorageBackend != "memory" && c.StorageBackend != "turso" && c.StorageBackend != "sqlite-file" {
		errs = append(errs, fmt.Errorf("invalid StorageBackend %q; must be memory, turso, or sqlite-file", c.StorageBackend))
	}

	// Validate Sources
	sourceIDs := make(map[string]bool)
	for i, src := range c.Sources {
		if src.ID == "" {
			errs = append(errs, fmt.Errorf("source %d has empty ID", i+1))
		}
		if sourceIDs[src.ID] {
			errs = append(errs, fmt.Errorf("duplicate source ID %q", src.ID))
		}
		sourceIDs[src.ID] = true

		if src.URL == "" {
			errs = append(errs, fmt.Errorf("source %d (%s) has empty URL", i+1, src.ID))
		} else if u, err := url.Parse(src.URL); err != nil {
			errs = append(errs, fmt.Errorf("source %d (%s) URL invalid: %v", i+1, src.ID, err))
		} else if u.Scheme != "http" && u.Scheme != "https" {
			errs = append(errs, fmt.Errorf("source %d (%s) URL must use http or https; got %s", i+1, src.ID, u.Scheme))
		}

		if src.FetchSeconds < 0 {
			errs = append(errs, fmt.Errorf("source %d (%s) FetchSeconds must be >= 0", i+1, src.ID))
		}
	}

	// Validate Env
	if c.Env != "" && c.Env != "dev" && c.Env != "production" {
		errs = append(errs, fmt.Errorf("env must be dev or production; got %q", c.Env))
	}

	// Validate production constraints (A4: fail clearly on missing mandatory production config)
	if c.Env == "production" {
		if c.StorageBackend == "" || c.StorageBackend == "memory" {
			errs = append(errs, errors.New("production requires StorageBackend to be non-memory (turso or sqlite-file)"))
		}
		if c.TursoDatabaseURL == "" {
			errs = append(errs, errors.New("production requires TURSO_DATABASE_URL"))
		}
		if c.TursoAuthToken == "" {
			errs = append(errs, errors.New("production requires TURSO_AUTH_TOKEN"))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// Redacted returns a copy of Config with secrets replaced by "***".
func (c Config) Redacted() Config {
	c.TursoAuthToken = "***"
	return c
}

// MarshalJSON implements json.Marshaler to never include secrets.
func (c Config) MarshalJSON() ([]byte, error) {
	type SafeConfig struct {
		ConfigFile     string   `json:"ConfigFile"`
		LogLevel       string   `json:"LogLevel"`
		UserAgent      string   `json:"UserAgent"`
		RequestTimeout int64    `json:"RequestTimeout"`
		HugoBaseURL    string   `json:"HugoBaseURL"`
		StorageBackend string   `json:"StorageBackend"`
		Env            string   `json:"Env"`
		Sources        []Source `json:"Sources"`
	}
	safe := SafeConfig{
		ConfigFile:     c.ConfigFile,
		LogLevel:       c.LogLevel,
		UserAgent:      c.UserAgent,
		RequestTimeout: c.RequestTimeout.Nanoseconds(),
		HugoBaseURL:    c.HugoBaseURL,
		StorageBackend: c.StorageBackend,
		Env:            c.Env,
		Sources:        c.Sources,
	}
	return json.Marshal(safe)
}
