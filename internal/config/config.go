// Package config provides configuration loading and validation.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

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
}

// Load loads configuration from environment variables with optional file override.
func Load(configFile string) (Config, error) {
	cfg := Config{
		LogLevel:       "info",
		UserAgent:      "huddlescope/0.1",
		RequestTimeout: 30 * time.Second,
		HugoBaseURL:    "https://huddlescope.github.io/",
		StorageBackend: "memory",
	}

	// Override with config file if provided
	if configFile != "" {
		if err := loadEnvFile(configFile, &cfg); err != nil {
			return cfg, fmt.Errorf("failed to load config file %q: %w", configFile, err)
		}
	}

	// Override with environment variables
	cfg.loadFromEnv()

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
}

// loadEnvFile loads key=value pairs from a file.
func loadEnvFile(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Missing file is OK
			return nil
		}
		return err
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		line = trimComment(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid line (expected KEY=VALUE): %q", line)
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
			}
		case "HSC_HUGO_BASE_URL":
			cfg.HugoBaseURL = value
		case "HSC_STORAGE_BACKEND":
			cfg.StorageBackend = value
		case "TURSO_DATABASE_URL":
			cfg.TursoDatabaseURL = value
		case "TURSO_AUTH_TOKEN":
			cfg.TursoAuthToken = value
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

// Validate checks the configuration for validity.
func (c Config) Validate() error {
	if c.LogLevel != "" {
		switch c.LogLevel {
		case "debug", "info", "warn", "error":
		default:
			return fmt.Errorf("invalid log level %q; must be debug, info, warn, or error", c.LogLevel)
		}
	}
	return nil
}
