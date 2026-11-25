// Package config provides configuration loading and validation.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
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
	Category      string // Content category (e.g., "national", "all-sports", "team-official")
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
	StoragePath      string // Path for sqlite-file backend
	Sources          []Source
	Teams            []Team
	Env              string
	// New for HS-013: lookback window
	LookbackWindow time.Duration
	// New for HS-021: heartbeat file path
	HeartbeatFile string
	// New for HS-056: Hugo snapshot file paths
	FeedsFile string // Path to feed health JSON output (production: web/data/feeds.json)
	DataDir   string // Hugo data output directory (production: web/data)
	DataFile  string // Homepage snapshot filename (production: homepage.json)
	// New for HS-061: Hugo content directory for generated story pages
	HugoContentDir string // Hugo content directory (production: web/content)
	// New for HS-024: clustering threshold and window
	SimilarityThreshold float64
	TemporalWindow      time.Duration
	// New for issue #25: minimum entity-set affinity for a scored merge.
	EntityAffinityMin float64
	// New for issue #120 (HS-065): retention policy for hot tables.
	RetentionEnabled    bool
	RetentionWindowDays int
	// New for issue #159 (HS-PORT-03): execution origin recorded on ingest
	// run rows (e.g., raspberry, vps, github-backup). Default "local".
	RunnerOrigin string
	// New for issue #158 (HS-PORT-02): site-data publish control. Values:
	// "auto" (default: dev writes files, production skips), "true", "false".
	PublishSiteData string
	// New for issue #158 (HS-PORT-02): opt-in single-line JSON run log.
	// "true"/"false" (default false). Non-secret; included in redacted output.
	JSONLog string
}

// StorageConfig carries the storage-backend options for repository
// construction. New backend options are added here and to
// storage.NewRepository only; call sites are unaffected.
type StorageConfig struct {
	Backend    string
	Path       string
	TursoURL   string
	TursoToken string
}

// Storage returns the storage configuration for repository construction.
func (c Config) Storage() StorageConfig {
	return StorageConfig{
		Backend:    c.StorageBackend,
		Path:       c.StoragePath,
		TursoURL:   c.TursoDatabaseURL,
		TursoToken: c.TursoAuthToken,
	}
}

const (
	defaultLogLevel       = "info"
	defaultUserAgent      = "huddlescope/0.1"
	defaultRequestTimeout = 30 * time.Second
	defaultHugoBaseURL    = "https://huddlescope.github.io/"
	defaultStorageBackend = "memory"
	defaultStoragePath    = "data/huddlescope.db"
	defaultTeamsFile      = "conf/teams.yaml"
	defaultEnv            = "dev"
	defaultLookbackWindow = 24 * time.Hour
	defaultHeartbeatFile  = "web/data/heartbeat.json"
	defaultFeedsFile      = "web/data/feeds.json"
	defaultDataDir        = "web/data"
	defaultDataFile       = "homepage.json"
	defaultHugoContentDir = "web/content"
	// New for HS-024: clustering defaults
	defaultSimilarityThreshold = 0.50
	defaultTemporalWindow      = 48 * time.Hour
	// New for issue #25: default minimum entity-set affinity.
	defaultEntityAffinityMin = domain.DefaultEntityAffinityMin
	// New for issue #120 (HS-065): retention defaults.
	defaultRetentionEnabled    = true
	defaultRetentionWindowDays = domain.DefaultRetentionDays
	// New for issue #159 (HS-PORT-03): default execution origin.
	defaultRunnerOrigin = "local"
)

// Overrides carries CLI-flag values that must win over environment variables
// and the --config file (issue #158 / AC-1, AC-2). Zero values mean "unset" and
// fall through to the next precedence level. The documented precedence chain is
// flags > environment > --config file > defaults.
type Overrides struct {
	SourcesFile string
	TeamsFile   string
	Timeout     time.Duration
	LogLevel    string
}

// Load loads configuration from environment variables with optional file override.
// It is equivalent to LoadWith(configFile, Overrides{}).
func Load(configFile string) (Config, error) {
	return LoadWith(configFile, Overrides{})
}

// LoadWith loads configuration applying the documented precedence chain
// (issue #158): defaults -> --config file -> environment -> flag overrides.
// The overrides are applied LAST so a CLI flag always wins over the environment
// and the file. This is how the root --config/--sources-file/--teams-file/
// --timeout/--log-level persistent flags reach config loading without changing
// the runner's `loadConfig("")` call shape.
func LoadWith(configFile string, o Overrides) (Config, error) {
	cfg := Config{
		LogLevel:       defaultLogLevel,
		UserAgent:      defaultUserAgent,
		TeamsFile:      defaultTeamsFile,
		RequestTimeout: defaultRequestTimeout,
		HugoBaseURL:    defaultHugoBaseURL,
		StorageBackend: defaultStorageBackend,
		StoragePath:    defaultStoragePath,
		Env:            defaultEnv,
		LookbackWindow: defaultLookbackWindow,
		HeartbeatFile:  defaultHeartbeatFile,
		FeedsFile:      defaultFeedsFile,
		DataDir:        defaultDataDir,
		DataFile:       defaultDataFile,
		HugoContentDir: defaultHugoContentDir,
		// New for HS-024
		SimilarityThreshold: defaultSimilarityThreshold,
		TemporalWindow:      defaultTemporalWindow,
		EntityAffinityMin:   defaultEntityAffinityMin,
		// New for issue #120
		RetentionEnabled:    defaultRetentionEnabled,
		RetentionWindowDays: defaultRetentionWindowDays,
		RunnerOrigin:        defaultRunnerOrigin,
	}

	// Override with config file if provided
	if configFile != "" {
		if err := loadEnvFile(configFile, &cfg); err != nil {
			return cfg, fmt.Errorf("failed to load config file %q: %w", configFile, err)
		}
	}

	// Override with environment variables
	cfg.loadFromEnv()

	// Apply CLI flag overrides last (highest precedence; issue #158 / AC-1).
	cfg.applyOverrides(o)

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
	if v := os.Getenv("HSC_STORAGE_PATH"); v != "" {
		c.StoragePath = v
	}
	if v := os.Getenv("TURSO_DATABASE_URL"); v != "" {
		c.TursoDatabaseURL = v
	}
	if v := os.Getenv("TURSO_AUTH_TOKEN"); v != "" {
		c.TursoAuthToken = v
	}
	if v := os.Getenv("HSC_SOURCES"); v != "" {
		// Clear sources first to allow env to override file;
		// parsing and validation happens in parseSources().
		c.Sources = nil
	}
	if v := os.Getenv("HSC_ENV"); v != "" {
		c.Env = v
	}
	if v := os.Getenv("HSC_HEARTBEAT_FILE"); v != "" {
		c.HeartbeatFile = v
	}
	if v := os.Getenv("HSC_FEEDS_FILE"); v != "" {
		c.FeedsFile = v
	}
	if v := os.Getenv("HSC_DATA_DIR"); v != "" {
		c.DataDir = v
	}
	if v := os.Getenv("HSC_HUGO_CONTENT_DIR"); v != "" {
		c.HugoContentDir = v
	}
	if v := os.Getenv("HSC_DATA_FILE"); v != "" {
		c.DataFile = v
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
	// New for HS-024
	if v := os.Getenv("HSC_SIMILARITY_THRESHOLD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			c.SimilarityThreshold = f
		}
	}
	if v := os.Getenv("HSC_CLUSTER_WINDOW"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.TemporalWindow = d
		}
	}
	// New for issue #25
	if v := os.Getenv("HSC_CLUSTER_ENTITY_AFFINITY"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			c.EntityAffinityMin = f
		}
	}
	// New for issue #120 (HS-065)
	if v := os.Getenv("HSC_RETENTION_ENABLED"); v != "" {
		switch strings.ToLower(v) {
		case "true", "1", "yes", "on":
			c.RetentionEnabled = true
		case "false", "0", "no", "off":
			c.RetentionEnabled = false
		}
	}
	if v := os.Getenv("HSC_RETENTION_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.RetentionWindowDays = n
		}
	}
	// New for issue #159 (HS-PORT-03): execution origin for ingest run rows.
	if v := os.Getenv("HSC_RUNNER_ORIGIN"); v != "" {
		c.RunnerOrigin = sanitizeRunnerOrigin(v)
	}
	// New for issue #158 (HS-PORT-02): site-data publish control + JSON run log.
	if v := os.Getenv("HSC_PUBLISH_SITE_DATA"); v != "" {
		c.PublishSiteData = strings.ToLower(v)
	}
	if v := os.Getenv("HSC_JSON_LOG"); v != "" {
		c.JSONLog = strings.ToLower(v)
	}
}

// applyOverrides applies CLI flag overrides (issue #158 / AC-1, AC-2). These are
// the highest-precedence values and are applied after file+env so a flag always
// wins. Zero-valued fields are ignored (they mean "not set on the command line").
func (c *Config) applyOverrides(o Overrides) {
	if o.SourcesFile != "" {
		c.SourcesFile = o.SourcesFile
	}
	if o.TeamsFile != "" {
		c.TeamsFile = o.TeamsFile
	}
	if o.Timeout > 0 {
		c.RequestTimeout = o.Timeout
	}
	if o.LogLevel != "" {
		c.LogLevel = o.LogLevel
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

	// env HSC_SOURCES takes priority over file sources
	if v := os.Getenv("HSC_SOURCES"); v != "" {
		if err := c.parseSourcesRaw(v); err != nil {
			return fmt.Errorf("invalid HSC_SOURCES: %w", err)
		}
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
//
// Missing or empty files yield no teams; callers must check and handle that case.
// Malformed files return an error that Load propagates.
func (c *Config) parseTeams() error {
	if len(c.Teams) > 0 {
		return nil // already parsed
	}

	// Try HSC_TEAMS_FILE if no env teams
	if c.TeamsFile == "" {
		c.TeamsFile = defaultTeamsFile
	}

	// Try loading from YAML file
	if teams, err := c.loadTeamsFromYAML(); err != nil {
		// malformed file → propagate error
		return err
	} else if len(teams) > 0 {
		c.Teams = teams
		return nil
	}
	// missing or empty file → no error, callers must handle empty teams
	return nil
}

// parseSourcesRaw parses HSC_SOURCES environment value into Sources.
// It returns a slice of sources parsed successfully; if the raw string is
// invalid, the returned slice is nil and an error describes the cause.
// This method is safe to call multiple times — it always starts fresh.
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
		case "HSC_STORAGE_PATH":
			cfg.StoragePath = value
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
		case "HSC_HEARTBEAT_FILE":
			cfg.HeartbeatFile = value
		case "HSC_FEEDS_FILE":
			cfg.FeedsFile = value
		case "HSC_DATA_DIR":
			cfg.DataDir = value
		case "HSC_HUGO_CONTENT_DIR":
			cfg.HugoContentDir = value
		case "HSC_DATA_FILE":
			cfg.DataFile = value
		case "HSC_SIMILARITY_THRESHOLD":
			if f, err := strconv.ParseFloat(value, 64); err == nil {
				cfg.SimilarityThreshold = f
			} else {
				return fmt.Errorf("line %d: invalid similarity threshold %q: %v", i+1, value, err)
			}
		case "HSC_CLUSTER_WINDOW":
			if d, err := time.ParseDuration(value); err == nil {
				cfg.TemporalWindow = d
			} else {
				return fmt.Errorf("line %d: invalid cluster window duration %q: %v", i+1, value, err)
			}
		case "HSC_CLUSTER_ENTITY_AFFINITY":
			if f, err := strconv.ParseFloat(value, 64); err == nil {
				cfg.EntityAffinityMin = f
			} else {
				return fmt.Errorf("line %d: invalid entity affinity %q: %v", i+1, value, err)
			}
		case "HSC_RETENTION_ENABLED":
			switch strings.ToLower(value) {
			case "true", "1", "yes", "on":
				cfg.RetentionEnabled = true
			case "false", "0", "no", "off":
				cfg.RetentionEnabled = false
			default:
				return fmt.Errorf("line %d: invalid HSC_RETENTION_ENABLED %q (expected true/false)", i+1, value)
			}
		case "HSC_RETENTION_DAYS":
			n, err := strconv.Atoi(value)
			if err != nil || n <= 0 {
				return fmt.Errorf("line %d: invalid HSC_RETENTION_DAYS %q (expected positive integer)", i+1, value)
			}
			cfg.RetentionWindowDays = n
		case "HSC_RUNNER_ORIGIN":
			cfg.RunnerOrigin = sanitizeRunnerOrigin(value)
		case "HSC_SOURCES_FILE":
			cfg.SourcesFile = value
		case "HSC_TEAMS_FILE":
			cfg.TeamsFile = value
		case "HSC_PUBLISH_SITE_DATA":
			cfg.PublishSiteData = strings.ToLower(value)
		case "HSC_JSON_LOG":
			cfg.JSONLog = strings.ToLower(value)
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

// sanitizeRunnerOrigin normalizes the HSC_RUNNER_ORIGIN value to the allowed
// charset [a-z0-9-] (max 64 chars) so it is safe to store and log. An empty
// result keeps the previous/default value.
func sanitizeRunnerOrigin(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	var b strings.Builder
	for _, r := range v {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
		if b.Len() >= 64 {
			return b.String()
		}
	}
	return b.String()
}

// ShouldPublishSiteData reports whether ingest should write the site-data files
// (web/data/heartbeat.json + web/data/feeds.json). HSC_PUBLISH_SITE_DATA accepts
// "auto" (default), "true", or "false". "auto" writes files in dev/test and skips
// them in production, so a repo-less VPS/Raspberry host does not create web/data
// (issue #158 / AC-3, AC-5). GitHub Actions sets HSC_PUBLISH_SITE_DATA=true so
// refresh-news keeps the committed files fresh.
func (c Config) ShouldPublishSiteData() bool {
	switch strings.ToLower(c.PublishSiteData) {
	case "true":
		return true
	case "false":
		return false
	default: // "auto" (or empty) — env-dependent
		return c.Env != "production"
	}
}

// ShouldJSONLog reports whether the optional single-line JSON run log is enabled.
// Defaults to false (issue #158 / AC-8).
func (c Config) ShouldJSONLog() bool {
	switch c.JSONLog {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}

// Validate checks the configuration for validity and returns all errors.
func (c Config) Validate() error {
	var errs []error

	// Validate LookbackWindow
	if c.LookbackWindow < 0 {
		errs = append(errs, fmt.Errorf("LookbackWindow must be non-negative; got %v", c.LookbackWindow))
	}

	// Validate SimilarityThreshold (AC-2: 0 < threshold <= 1)
	if c.SimilarityThreshold <= 0 || c.SimilarityThreshold > 1 {
		errs = append(errs, fmt.Errorf("SimilarityThreshold must be in (0, 1]; got %v", c.SimilarityThreshold))
	}

	// Validate TemporalWindow (AC-2: window > 0)
	if c.TemporalWindow <= 0 {
		errs = append(errs, errors.New("TemporalWindow must be positive"))
	}

	// Validate EntityAffinityMin (issue #25: when explicitly set, 0 < affinity <= 1).
	// A zero value means "unset" and is accepted; the domain falls back to
	// DefaultEntityAffinityMin. This keeps Config literals that omit the field valid.
	if c.EntityAffinityMin != 0 && (c.EntityAffinityMin < 0 || c.EntityAffinityMin > 1) {
		errs = append(errs, fmt.Errorf("EntityAffinityMin must be in (0, 1]; got %v", c.EntityAffinityMin))
	}

	// Validate RetentionWindowDays (issue #120: when explicitly set, must be
	// non-negative). Zero means "unset" and is accepted; the retention service
	// falls back to domain.DefaultRetentionDays. This keeps Config literals
	// that omit the field valid.
	if c.RetentionWindowDays < 0 {
		errs = append(errs, fmt.Errorf("RetentionWindowDays must be non-negative; got %d", c.RetentionWindowDays))
	}

	// Validate HeartbeatFile
	if c.HeartbeatFile == "" {
		errs = append(errs, errors.New("HeartbeatFile must not be empty"))
	}

	// Validate FeedsFile
	if c.FeedsFile == "" {
		errs = append(errs, errors.New("FeedsFile must not be empty"))
	}

	// Validate DataDir
	if c.DataDir == "" {
		errs = append(errs, errors.New("DataDir must not be empty"))
	}

	// Validate HugoContentDir
	if c.HugoContentDir == "" {
		errs = append(errs, errors.New("HugoContentDir must not be empty"))
	}

	// Validate DataFile
	if c.DataFile == "" {
		errs = append(errs, errors.New("DataFile must not be empty"))
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

	// Path-independence validation (issue #158 / AC-3). Explicitly configured
	// relative paths are rejected for input files in all envs, and for output
	// paths in production. The built-in repo-relative defaults remain valid so
	// documented dev usage (relative conf/*.yaml, web/data) keeps working.
	if relativeExplicit(c.SourcesFile, "conf/sources.yaml") {
		errs = append(errs, fmt.Errorf("HSC_SOURCES_FILE must be an absolute path when explicitly set; got %q", c.SourcesFile))
	}
	if relativeExplicit(c.TeamsFile, defaultTeamsFile) {
		errs = append(errs, fmt.Errorf("HSC_TEAMS_FILE must be an absolute path when explicitly set; got %q", c.TeamsFile))
	}
	if c.Env == "production" {
		if relativeExplicit(c.HeartbeatFile, defaultHeartbeatFile) {
			errs = append(errs, fmt.Errorf("HSC_HEARTBEAT_FILE must be an absolute path in production; got %q", c.HeartbeatFile))
		}
		if relativeExplicit(c.FeedsFile, defaultFeedsFile) {
			errs = append(errs, fmt.Errorf("HSC_FEEDS_FILE must be an absolute path in production; got %q", c.FeedsFile))
		}
		if relativeExplicit(c.DataDir, defaultDataDir) {
			errs = append(errs, fmt.Errorf("HSC_DATA_DIR must be an absolute path in production; got %q", c.DataDir))
		}
		if relativeExplicit(c.HugoContentDir, defaultHugoContentDir) {
			errs = append(errs, fmt.Errorf("HSC_HUGO_CONTENT_DIR must be an absolute path in production; got %q", c.HugoContentDir))
		}
		if relativeExplicit(c.StoragePath, defaultStoragePath) {
			errs = append(errs, fmt.Errorf("HSC_STORAGE_PATH must be an absolute path in production; got %q", c.StoragePath))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// relativeExplicit reports whether a configured path is a non-default relative
// path (issue #158 / AC-3). An empty value (unset) and the exact built-in default
// are both legal (the default is repo-relative and documented for dev). Only a
// value the operator explicitly set that is relative returns true.
func relativeExplicit(path, defaultPath string) bool {
	if path == "" || path == defaultPath {
		return false
	}
	return !filepath.IsAbs(path)
}

// Redacted returns a copy of Config with secrets replaced by "***".
func (c Config) Redacted() Config {
	c.TursoAuthToken = "***"
	c.HeartbeatFile = "***"
	return c
}

// MarshalJSON implements json.Marshaler to never include secrets.
// Uses value receiver to ensure json.Marshal() works with both cfg and &cfg.
func (c Config) MarshalJSON() ([]byte, error) {
	type SafeConfig struct {
		ConfigFile          string   `json:"ConfigFile"`
		LogLevel            string   `json:"LogLevel"`
		UserAgent           string   `json:"UserAgent"`
		RequestTimeout      int64    `json:"RequestTimeout"`
		HugoBaseURL         string   `json:"HugoBaseURL"`
		StorageBackend      string   `json:"StorageBackend"`
		Env                 string   `json:"Env"`
		Sources             []Source `json:"Sources"`
		Teams               []Team   `json:"Teams"`
		HeartbeatFile       string   `json:"HeartbeatFile"`
		FeedsFile           string   `json:"FeedsFile"`
		DataDir             string   `json:"DataDir"`
		DataFile            string   `json:"DataFile"`
		HugoContentDir      string   `json:"HugoContentDir"`
		RetentionEnabled    bool     `json:"RetentionEnabled"`
		RetentionWindowDays int      `json:"RetentionWindowDays"`
		PublishSiteData     string   `json:"PublishSiteData"`
		JSONLog             string   `json:"JSONLog"`
	}
	safe := SafeConfig{
		ConfigFile:          c.ConfigFile,
		LogLevel:            c.LogLevel,
		UserAgent:           c.UserAgent,
		RequestTimeout:      c.RequestTimeout.Nanoseconds(),
		HugoBaseURL:         c.HugoBaseURL,
		StorageBackend:      c.StorageBackend,
		Env:                 c.Env,
		Sources:             c.Sources,
		Teams:               c.Teams,
		HeartbeatFile:       c.HeartbeatFile,
		FeedsFile:           c.FeedsFile,
		DataDir:             c.DataDir,
		DataFile:            c.DataFile,
		HugoContentDir:      c.HugoContentDir,
		RetentionEnabled:    c.RetentionEnabled,
		RetentionWindowDays: c.RetentionWindowDays,
		PublishSiteData:     c.PublishSiteData,
		JSONLog:             c.JSONLog,
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
			Name:        s.Name,
			Category:    s.Category,
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
func (c Config) TeamRegistry() (*domain.TeamRegistry, error) {
	if len(c.Teams) == 0 {
		return nil, fmt.Errorf("teams are required but no teams were loaded (teams file: %q)", c.TeamsFile)
	}
	return domain.NewTeamRegistryFromSlice(c.ToDomainTeams()), nil
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

// ClustererSettings returns a domain.Clusterer configured with the similarity threshold
// and temporal window from this Config. Returns a default clusterer with 0.50 threshold
// and 48h window if validation fails (caller should have already validated via Validate()).
func (c Config) ClustererSettings() *domain.Clusterer {
	// If values failed validation, return defaults to allow graceful degradation
	if err := c.Validate(); err != nil {
		return domain.NewClusterer()
	}
	affinityMin := c.EntityAffinityMin
	if affinityMin <= 0 {
		affinityMin = domain.DefaultEntityAffinityMin
	}
	return &domain.Clusterer{
		SimilarityThreshold: c.SimilarityThreshold,
		TemporalWindow:      c.TemporalWindow,
		EntityAffinityMin:   affinityMin,
	}
}
