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

import "gopkg.in/yaml.v3"

import "github.com/HuddleScope/huddlescope.github.io/internal/domain"

// Source represents a news source or feed.
type Source struct {
	ID           string
	Name         string
	URL          string
	FetchSeconds int
	// Additional registry fields (HS-006)
	Publication   string
	Priority      int
	Enabled       bool
	Language      string
	Timeout       int // seconds
	License       string
	AcceptableUse string
}

// YAMLTeam represents the structure of a team in YAML config files.
type YAMLTeam struct {
	Slug         string   `yaml:"slug"`
	Name         string   `yaml:"name"`
	ShortName    string   `yaml:"short_name"`
	Abbreviation string   `yaml:"abbreviation"`
	Aliases      []string `yaml:"aliases,omitempty"`
	Division     string   `yaml:"division,omitempty"`
}

// toConfigTeam converts YAMLTeam to config Team.
//
//nolint:staticcheck // S1016: struct literal required for explicit field mapping
func (yt YAMLTeam) toConfigTeam() Team {
	return Team{
		Slug:         yt.Slug,
		Name:         yt.Name,
		ShortName:    yt.ShortName,
		Abbreviation: yt.Abbreviation,
		Aliases:      yt.Aliases,
		Division:     yt.Division,
	}
}

// Team represents an NFL team with all its identifying information.
type Team struct {
	Slug         string   // Stable URL slug (e.g., "new-york-giants")
	Name         string   // Full legal name (e.g., "New York Giants")
	ShortName    string   // Short name (e.g., "Giants")
	Abbreviation string   // Official abbreviation (e.g., "NYG")
	Aliases      []string // Common alternative names and nicknames
	Division     string   // NFL division (optional)
}

// Config holds the application configuration.
type Config struct {
	ConfigFile       string
	SourcesFile      string // Path to YAML sources file (optional)
	TeamsFile        string // Path to YAML teams file (optional)
	LogLevel         string
	UserAgent        string
	RequestTimeout   time.Duration
	HugoBaseURL      string
	StorageBackend   string
	TursoDatabaseURL string
	TursoAuthToken   string
	Sources          []Source
	Teams            []Team
	Env              string
	// New for HS-013: lookback window
	LookbackWindow time.Duration
}

const (
	defaultLogLevel       = "info"
	defaultUserAgent      = "huddlescope/0.1"
	defaultRequestTimeout = 30 * time.Second
	defaultHugoBaseURL    = "https://huddlescope.github.io/"
	defaultStorageBackend = "memory"
	defaultTeamsFile      = "conf/teams.yaml"
	defaultEnv            = "dev"
	defaultLookbackWindow = 24 * time.Hour
)

// Load loads configuration from environment variables with optional file override.
func Load(configFile string) (Config, error) {
	cfg := Config{
		LogLevel:       defaultLogLevel,
		UserAgent:      defaultUserAgent,
		TeamsFile:      defaultTeamsFile,
		RequestTimeout: defaultRequestTimeout,
		HugoBaseURL:    defaultHugoBaseURL,
		StorageBackend: defaultStorageBackend,
		Env:            defaultEnv,
		LookbackWindow: defaultLookbackWindow,
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

	// Parse teams after all env vars are loaded
	if err := cfg.parseTeams(); err != nil {
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
	if v := os.Getenv("HSC_SOURCES_FILE"); v != "" {
		c.SourcesFile = v
	}
	if v := os.Getenv("HSC_TEAMS_FILE"); v != "" {
		c.TeamsFile = v
	}
	if v := os.Getenv("HSC_LOOKBACK"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.LookbackWindow = d
		}
	}
}

// parseSources parses sources from the parsed Sources field (set by loadFromEnv).
// Sources are loaded in this priority order:
//  1. HSC_SOURCES environment variable (highest priority)
//  2. Sources file (from SourcesFile or default conf/sources.yaml)
//
// If neither provides sources, c.Sources stays empty; the ingest command fails
// fast in that case (see cmd/ingest). Export does not consume sources.
//
// A missing or empty sources file is not an error. A file that exists but fails
// validation (duplicate id, bad URL, etc.) returns that error here so the user
// sees the real cause instead of a misleading "no sources configured" message.
func (c *Config) parseSources() error {
	if len(c.Sources) > 0 {
		return nil // already parsed
	}

	// Try HSC_SOURCES_FILE if no env sources
	if c.SourcesFile == "" {
		c.SourcesFile = "conf/sources.yaml"
	}

	// Fall back to env var first (highest priority)
	if v := os.Getenv("HSC_SOURCES"); v != "" {
		if err := c.parseSourcesRaw(v); err != nil {
			return err
		}
		// Don't load from file if env var is set
		return nil
	}

	// Load from the configured YAML file. A missing or empty file yields no
	// sources (ingest fails fast); a malformed file surfaces its error.
	sources, err := c.loadSourcesFromYAML()
	if err != nil {
		return err
	}
	c.Sources = sources

	return nil
}

// parseTeams parses teams from the Teams field (set by loadFromEnv).
// Teams are loaded in this priority order:
//  1. HSC_TEAMS_FILE environment variable (highest priority)
//  2. Teams file (from TeamsFile or default conf/teams.yaml)
//  3. Default teams from SeedNFLTeams (lowest priority)
func (c *Config) parseTeams() error {
	if len(c.Teams) > 0 {
		return nil // already parsed
	}

	// Try HSC_TEAMS_FILE if no env teams
	if c.TeamsFile == "" {
		c.TeamsFile = defaultTeamsFile
	}

	// Try loading from YAML file
	if teams, err := c.loadTeamsFromYAML(); err == nil && len(teams) > 0 {
		c.Teams = teams
		return nil
	}

	return nil
}

// parseSourcesRaw parses HSC_SOURCES environment value into Sources.
// Format: "id,name,url[,interval][,priority][,enabled][,publication][,language][,timeout][,license][,accept-use];..."
// e.g., "nfl-news,NFL News,http://example.com,300,1,true,ESPN,en,60,allowed"
// Optional fields after URL are parsed positionally; omitted fields use defaults.
// Defaults: interval=900, priority=0, enabled=true, publication="", language="en", timeout=0, license="", accept-use="unknown"
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
		priority := 0
		enabled := true
		publication := ""
		language := "en"
		timeout := 0
		license := ""
		acceptableUse := "unknown"

		// Parse optional fields
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

		if len(parts) >= 5 {
			if v, err := fmt.Sscanf(strings.TrimSpace(parts[4]), "%d", &priority); err == nil && v == 1 && priority >= 0 {
				// priority parsed
			} else {
				return fmt.Errorf("source entry %d invalid priority: %q", i+1, parts[4])
			}
		}

		if len(parts) >= 6 {
			if strings.ToLower(strings.TrimSpace(parts[5])) == "false" {
				enabled = false
			} else if strings.ToLower(strings.TrimSpace(parts[5])) != "true" {
				return fmt.Errorf("source entry %d invalid enabled value: %q (expected true/false)", i+1, parts[5])
			}
		}

		if len(parts) >= 7 {
			publication = strings.TrimSpace(parts[6])
		}

		if len(parts) >= 8 {
			language = strings.TrimSpace(parts[7])
		}

		if len(parts) >= 9 {
			if v, err := fmt.Sscanf(strings.TrimSpace(parts[8]), "%d", &timeout); err == nil && v == 1 && timeout >= 0 {
				// timeout parsed (seconds)
			} else {
				return fmt.Errorf("source entry %d invalid timeout: %q", i+1, parts[8])
			}
		}

		if len(parts) >= 10 {
			license = strings.TrimSpace(parts[9])
		}

		if len(parts) >= 11 {
			acceptableUse = strings.TrimSpace(parts[10])
			if acceptableUse != "allowed" && acceptableUse != "restricted" && acceptableUse != "unknown" {
				return fmt.Errorf("source entry %d invalid accept-use value: %q (expected allowed/restricted/unknown)", i+1, parts[10])
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
			ID:            id,
			Name:          name,
			URL:           urlStr,
			FetchSeconds:  fetchSeconds,
			Publication:   publication,
			Priority:      priority,
			Enabled:       enabled,
			Language:      language,
			Timeout:       timeout,
			License:       license,
			AcceptableUse: acceptableUse,
		})
	}

	return nil
}

// loadSourcesFromYAML loads sources from a YAML file.
// Returns empty slice if file doesn't exist or is empty.
func (c *Config) loadSourcesFromYAML() ([]Source, error) {
	data, err := os.ReadFile(c.SourcesFile)
	if err != nil {
		if os.IsNotExist(err) {
			// File not found - return empty slice to allow fallback
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read sources file %q: %w", c.SourcesFile, err)
	}

	type YAMLSource struct {
		ID            string `yaml:"id"`
		Name          string `yaml:"name"`
		URL           string `yaml:"url"`
		Category      string `yaml:"category,omitempty"`
		Enabled       bool   `yaml:"enabled,omitempty"`
		Verification  string `yaml:"verification,omitempty"`
		Publication   string `yaml:"publication,omitempty"`
		Priority      int    `yaml:"priority,omitempty"`
		Team          string `yaml:"team,omitempty"`
		Language      string `yaml:"language,omitempty"`
		Timeout       int    `yaml:"timeout,omitempty"`
		License       string `yaml:"license,omitempty"`
		AcceptableUse string `yaml:"acceptable_use,omitempty"`
	}

	type YAMLConfig struct {
		Sources []YAMLSource `yaml:"sources"`
	}

	var cfg YAMLConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse sources file %q: %w", c.SourcesFile, err)
	}

	if len(cfg.Sources) == 0 {
		return nil, nil
	}

	// Validate and convert to Source
	sources := make([]Source, 0, len(cfg.Sources))
	seenIDs := make(map[string]bool)

	for i, ys := range cfg.Sources {
		if ys.ID == "" {
			return nil, fmt.Errorf("source %d: missing required field 'id'", i+1)
		}
		if ys.URL == "" {
			return nil, fmt.Errorf("source %d (%s): missing required field 'url'", i+1, ys.ID)
		}

		if seenIDs[ys.ID] {
			return nil, fmt.Errorf("duplicate source ID %q", ys.ID)
		}
		seenIDs[ys.ID] = true

		// Validate URL scheme
		u, err := url.Parse(ys.URL)
		if err != nil {
			return nil, fmt.Errorf("source %d (%s): invalid URL %q: %v", i+1, ys.ID, ys.URL, err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return nil, fmt.Errorf("source %d (%s): URL must use http or https, got %s", i+1, ys.ID, u.Scheme)
		}

		// Validate AcceptableUse if provided
		if ys.AcceptableUse != "" && ys.AcceptableUse != "allowed" && ys.AcceptableUse != "restricted" && ys.AcceptableUse != "unknown" {
			return nil, fmt.Errorf("source %d (%s): acceptable_use must be allowed/restricted/unknown, got %s", i+1, ys.ID, ys.AcceptableUse)
		}

		sources = append(sources, Source{
			ID:            ys.ID,
			Name:          ys.Name,
			URL:           ys.URL,
			FetchSeconds:  15 * 60, // default 15 minutes
			Publication:   ys.Publication,
			Priority:      ys.Priority,
			Enabled:       ys.Enabled,
			Language:      ys.Language,
			Timeout:       ys.Timeout,
			License:       ys.License,
			AcceptableUse: ys.AcceptableUse,
		})
	}

	return sources, nil
}

// loadTeamsFromYAML loads teams from a YAML file.
// Returns empty slice if file doesn't exist or is empty.
func (c *Config) loadTeamsFromYAML() ([]Team, error) {
	data, err := os.ReadFile(c.TeamsFile)
	if err != nil {
		if os.IsNotExist(err) {
			// File not found - return empty slice to allow fallback
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read teams file %q: %w", c.TeamsFile, err)
	}

	type YAMLConfig struct {
		Teams []YAMLTeam `yaml:"teams"`
	}

	var cfg YAMLConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse teams file %q: %w", c.TeamsFile, err)
	}

	if len(cfg.Teams) == 0 {
		return nil, nil
	}

	// Validate and convert to Team
	teams := make([]Team, 0, len(cfg.Teams))
	seenSlugs := make(map[string]bool)

	for i, yt := range cfg.Teams {
		if yt.Slug == "" {
			return nil, fmt.Errorf("team %d: missing required field 'slug'", i+1)
		}
		if yt.Name == "" {
			return nil, fmt.Errorf("team %d (%s): missing required field 'name'", i+1, yt.Slug)
		}
		if yt.ShortName == "" {
			return nil, fmt.Errorf("team %d (%s): missing required field 'short_name'", i+1, yt.Slug)
		}
		if yt.Abbreviation == "" {
			return nil, fmt.Errorf("team %d (%s): missing required field 'abbreviation'", i+1, yt.Slug)
		}

		if seenSlugs[yt.Slug] {
			return nil, fmt.Errorf("duplicate team slug %q", yt.Slug)
		}
		seenSlugs[yt.Slug] = true

		teams = append(teams, yt.toConfigTeam())
	}

	return teams, nil
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

	// Validate LookbackWindow
	if c.LookbackWindow < 0 {
		errs = append(errs, fmt.Errorf("LookbackWindow must be non-negative; got %v", c.LookbackWindow))
	}

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

		if src.Priority < 0 {
			errs = append(errs, fmt.Errorf("source %d (%s) Priority must be >= 0", i+1, src.ID))
		}

		if src.Timeout < 0 {
			errs = append(errs, fmt.Errorf("source %d (%s) Timeout must be >= 0", i+1, src.ID))
		}

		if src.AcceptableUse != "" && src.AcceptableUse != "allowed" && src.AcceptableUse != "restricted" && src.AcceptableUse != "unknown" {
			errs = append(errs, fmt.Errorf("source %d (%s) AcceptableUse must be allowed/restricted/unknown; got %s", i+1, src.ID, src.AcceptableUse))
		}
	}

	// Validate Teams
	teamSlugs := make(map[string]bool)
	for i, team := range c.Teams {
		if team.Slug == "" {
			errs = append(errs, fmt.Errorf("team %d has empty slug", i+1))
		}
		if team.Name == "" {
			errs = append(errs, fmt.Errorf("team %d (%s) has empty name", i+1, team.Slug))
		}
		if team.ShortName == "" {
			errs = append(errs, fmt.Errorf("team %d (%s) has empty short_name", i+1, team.Slug))
		}
		if team.Abbreviation == "" {
			errs = append(errs, fmt.Errorf("team %d (%s) has empty abbreviation", i+1, team.Slug))
		}
		if teamSlugs[team.Slug] {
			errs = append(errs, fmt.Errorf("duplicate team slug %q", team.Slug))
		}
		teamSlugs[team.Slug] = true
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
// Uses value receiver to ensure json.Marshal() works with both cfg and &cfg.
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
		Teams          []Team   `json:"Teams"`
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
		Teams:          c.Teams,
	}
	return json.Marshal(safe)
}

// ToDomain converts config Sources to domain FeedSources.
// This is used to wire the registry when the application starts.
func (c Config) ToDomain() []domain.FeedSource {
	sources := make([]domain.FeedSource, 0, len(c.Sources))
	for _, s := range c.Sources {
		fs := domain.FeedSource{
			ID:          s.ID,
			URL:         s.URL,
			Publication: s.Publication,
			Priority:    s.Priority,
			Enabled:     s.Enabled,
			Language:    s.Language,
			Timeout:     time.Duration(s.Timeout) * time.Second,
			License:     s.License,
		}
		switch s.AcceptableUse {
		case "allowed":
			fs.AcceptableUse = domain.AcceptanceAllowed
		case "restricted":
			fs.AcceptableUse = domain.AcceptanceRestricted
		default:
			fs.AcceptableUse = domain.AcceptanceUnknown
		}
		sources = append(sources, fs)
	}
	return sources
}

// ToDomainTeams converts config Teams to domain.Team.
// This is used to wire the team registry when the application starts.
func (c Config) ToDomainTeams() []domain.Team {
	teams := make([]domain.Team, 0, len(c.Teams))
	for _, t := range c.Teams {
		dt := domain.Team{
			Slug:         t.Slug,
			Name:         t.Name,
			ShortName:    t.ShortName,
			Abbreviation: t.Abbreviation,
			Aliases:      t.Aliases,
		}
		teams = append(teams, dt)
	}
	return teams
}

// TeamRegistry returns a TeamRegistry initialized with the teams from config.
// It is a required configuration and the app will fail if teams cannot be loaded.
func (c Config) TeamRegistry() *domain.TeamRegistry {
	if len(c.Teams) == 0 {
		panic("teams configuration is required but could not be loaded")
	}
	return domain.NewTeamRegistryFromSlice(c.ToDomainTeams())
}

// SourceRegistry returns the FeedRegistry built from the configured sources.
// It mirrors TeamRegistry: source fields are validated when the configuration
// is loaded, so a build failure here indicates an invariant violation and
// panics. When no sources are configured it returns an empty (non-nil)
// registry; callers that require at least one source must check the count.
func (c Config) SourceRegistry() *domain.FeedRegistry {
	registry, err := domain.NewFeedRegistry(c.ToDomain()...)
	if err != nil {
		panic(fmt.Sprintf("failed to build feed registry from configured sources: %v", err))
	}
	return registry
}
