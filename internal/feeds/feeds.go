// Package feeds provides curated NFL feed sources for HuddleScope.
//
// Sources are loaded from conf/sources.yaml at startup.
// To add/remove sources, edit conf/sources.yaml instead of this file.
// No other code changes are needed.
//
// Field notes:
//   - ID: stable business key (e.g. "espn-nfl", "nfl-com")
//   - URL: public RSS/Atom feed URL (http or https)
//   - Publication: publisher name for display
//   - Priority: lower number polled first; tiebreak by ID
//   - Enabled: false to skip polling entirely
//   - Language: BCP-47 tag (e.g. "en", "es")
//   - Timeout: per-source HTTP timeout (0 = global default)
//   - License: known license or acceptable-use notes (free text)
//   - AcceptableUse: whether syndication/scraping is permitted
//
// Important: Never hardcode credentials. Never fetch full paywalled content.
// Feeds with AcceptableUse=Restricted must be Enabled=false to avoid access attempts.
package feeds

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// SeededSources contains the initial curated list of NFL RSS/Atom feeds.
// This is kept for backward compatibility if conf/sources.yaml is not found.
var SeededSources = []domain.FeedSource{
	{
		ID:            "espn-nfl",
		URL:           "https://www.espn.com/nfl/rss/",
		Publication:   "ESPN NFL",
		Priority:      1,
		Enabled:       true,
		Language:      "en",
		License:       "RSS syndication permitted; no credentials required",
		AcceptableUse: domain.AcceptanceAllowed,
	},
	{
		ID:            "espn-fantasy",
		URL:           "https://www.espn.com/fantasy/football/rss/",
		Publication:   "ESPN Fantasy",
		Priority:      2,
		Enabled:       true,
		Language:      "en",
		License:       "RSS syndication permitted; fantasy content only",
		AcceptableUse: domain.AcceptanceAllowed,
	},
	{
		ID:            "cbs-nfl",
		URL:           "https://www.cbssports.com/nfl/rss/",
		Publication:   "CBS Sports NFL",
		Priority:      1,
		Enabled:       true,
		Language:      "en",
		License:       "RSS syndication permitted",
		AcceptableUse: domain.AcceptanceAllowed,
	},
	{
		ID:            "fox-nfl",
		URL:           "https://www.foxsports.com/nfl/rss",
		Publication:   "FOX Sports NFL",
		Priority:      2,
		Enabled:       true,
		Language:      "en",
		License:       "RSS syndication permitted",
		AcceptableUse: domain.AcceptanceAllowed,
	},
	{
		ID:            "nfl-com",
		URL:           "https://www.nfl.com/news/rss/",
		Publication:   "NFL.com News",
		Priority:      1,
		Enabled:       true,
		Language:      "en",
		License:       "Official NFL feed; syndication permitted",
		AcceptableUse: domain.AcceptanceAllowed,
	},
	{
		ID:            "nfl-com-boxscore",
		URL:           "https://www.nfl.com/scoreboard/rss/",
		Publication:   "NFL.com Boxscore",
		Priority:      3,
		Enabled:       true,
		Language:      "en",
		License:       "Official NFL scores feed",
		AcceptableUse: domain.AcceptanceAllowed,
	},
	{
		ID:            "bleacher-report-nfl",
		URL:           "https://bleacherreport.com/nfl/rss",
		Publication:   "Bleacher Report NFL",
		Priority:      2,
		Enabled:       true,
		Language:      "en",
		License:       "RSS syndication permitted",
		AcceptableUse: domain.AcceptanceAllowed,
	},
	{
		ID:            "sports-illustrated",
		URL:           "https://www.si.com/nfl/rss",
		Publication:   "Sports Illustrated NFL",
		Priority:      3,
		Enabled:       true,
		Language:      "en",
		License:       "RSS syndication permitted",
		AcceptableUse: domain.AcceptanceAllowed,
	},
	{
		ID:            "usa-today-nfl",
		URL:           "https://www.usatoday.com/rss-sports/nfl/",
		Publication:   "USA Today Sports NFL",
		Priority:      3,
		Enabled:       true,
		Language:      "en",
		License:       "RSS syndication permitted",
		AcceptableUse: domain.AcceptanceAllowed,
	},
	{
		ID:            "the-score",
		URL:           "https://www.thescore.com/nfl/rss",
		Publication:   "The Score NFL",
		Priority:      4,
		Enabled:       true,
		Language:      "en",
		License:       "RSS syndication permitted",
		AcceptableUse: domain.AcceptanceAllowed,
	},
	{
		ID:            "the-athletic-nfl",
		URL:           "https://theathletic.com/nfl/rss/",
		Publication:   "The Athletic NFL",
		Priority:      5,
		Enabled:       false, // paywalled content; restricted
		Language:      "en",
		License:       "Restricted: paywalled subscription required. Do not attempt to fetch full articles.",
		AcceptableUse: domain.AcceptanceRestricted,
	},
	{
		ID:            "reddit-r-nfl",
		URL:           "https://www.reddit.com/r/nfl/.rss",
		Publication:   "Reddit r/nfl",
		Priority:      6,
		Enabled:       true,
		Language:      "en",
		License:       "Community feed via Reddit RSS API",
		AcceptableUse: domain.AcceptanceAllowed,
	},
}

// YAMLSource represents a source as defined in the YAML file.
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

// YAMLConfig represents the root of the sources YAML file.
type YAMLConfig struct {
	Sources []YAMLSource `yaml:"sources"`
}

// LoadFrom loads sources from the given file path.
// Returns nil registry if file doesn't exist or is empty.
func LoadFrom(path string) (*domain.FeedRegistry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read sources file %q: %w", path, err)
	}

	var cfg YAMLConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse sources file %q: %w", path, err)
	}

	if len(cfg.Sources) == 0 {
		return nil, nil
	}

	feeds := make([]domain.FeedSource, 0, len(cfg.Sources))
	for _, ys := range cfg.Sources {
		fs := domain.FeedSource{
			ID:          ys.ID,
			URL:         ys.URL,
			Publication: ys.Publication,
			Priority:    ys.Priority,
			Enabled:     ys.Enabled,
			Language:    ys.Language,
			Timeout:     time.Duration(ys.Timeout) * time.Second,
			License:     ys.License,
		}
		switch ys.AcceptableUse {
		case "allowed":
			fs.AcceptableUse = domain.AcceptanceAllowed
		case "restricted":
			fs.AcceptableUse = domain.AcceptanceRestricted
		default:
			fs.AcceptableUse = domain.AcceptanceUnknown
		}
		feeds = append(feeds, fs)
	}

	return domain.NewFeedRegistry(feeds...)
}

// Load returns a FeedRegistry populated with sources from conf/sources.yaml.
// Falls back to SeededSources if the YAML file is not found or empty.
// The registry is pre-validated and ready for polling.
func Load() (*domain.FeedRegistry, error) {
	// Try to load from YAML file
	// Try conf/sources.yaml relative to current directory first
	data, err := os.ReadFile("conf/sources.yaml")
	if err != nil {
		// If that fails, try relative to parent directory (for tests)
		data, err = os.ReadFile("../../conf/sources.yaml")
		if err != nil {
			// Fall back to SeededSources
			return domain.NewFeedRegistry(SeededSources...)
		}
	}

	var cfg YAMLConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse sources.yaml: %w", err)
	}

	if len(cfg.Sources) > 0 {
		// Convert YAML sources to domain.FeedSource
		feeds := make([]domain.FeedSource, 0, len(cfg.Sources))
		for _, ys := range cfg.Sources {
			fs := domain.FeedSource{
				ID:          ys.ID,
				URL:         ys.URL,
				Publication: ys.Publication,
				Priority:    ys.Priority,
				Enabled:     ys.Enabled,
				Language:    ys.Language,
				Timeout:     time.Duration(ys.Timeout) * time.Second,
				License:     ys.License,
			}
			switch ys.AcceptableUse {
			case "allowed":
				fs.AcceptableUse = domain.AcceptanceAllowed
			case "restricted":
				fs.AcceptableUse = domain.AcceptanceRestricted
			default:
				fs.AcceptableUse = domain.AcceptanceUnknown
			}
			feeds = append(feeds, fs)
		}

		return domain.NewFeedRegistry(feeds...)
	}

	// Fall back to SeededSources
	return domain.NewFeedRegistry(SeededSources...)
}

// EnabledCount returns the number of enabled seeds from the loaded sources.
func EnabledCount() int {
	registry, err := Load()
	if err != nil {
		return 0
	}
	return registry.CountEnabled()
}
