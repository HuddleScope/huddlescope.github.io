// Package feeds provides curated NFL feed sources for HuddleScope.
//
// To add a feed: append a new FeedSource to SeededSources.
// To remove a feed: delete its line from SeededSources.
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
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// SeededSources contains the initial curated list of NFL RSS/Atom feeds.
// Update this slice to add/remove feeds without code changes elsewhere.
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

// Load returns a FeedRegistry populated with SeededSources.
// The registry is pre-validated and ready for polling.
func Load() (*domain.FeedRegistry, error) {
	return domain.NewFeedRegistry(SeededSources...)
}

// EnabledCount returns the number of enabled seeds.
func EnabledCount() int {
	count := 0
	for _, s := range SeededSources {
		if s.Enabled {
			count++
		}
	}
	return count
}
