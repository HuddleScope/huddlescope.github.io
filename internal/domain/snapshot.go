// Package domain defines the exported snapshot contract for Hugo.
package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// SnapshotVersion is the current version of the export format.
const SnapshotVersion = "1"

// Snapshot represents a deterministic, versioned export of stories and metadata.
type Snapshot struct {
	Version     string            `json:"version"`
	GeneratedAt string            `json:"generatedAt"`
	Stories     []SnapshotStory   `json:"stories"`
	Latest      []SnapshotArticle `json:"latest"`
	Teams       []SnapshotTeam    `json:"teams"`
}

// SnapshotStory is a canonical representation of a story for the homepage.
type SnapshotStory struct {
	ID               string         `json:"id"`
	Title            string         `json:"title"`
	Summary          string         `json:"summary"`
	FirstPublishedAt string         `json:"firstPublishedAt"`
	LastPublishedAt  string         `json:"lastPublishedAt"`
	Score            float64        `json:"score"`
	EvidenceLevel    EvidenceLevel  `json:"evidenceLevel"`
	Publishers       []PublisherRef `json:"publishers"`
}

// SnapshotArticle represents a single article (no body, no full content).
type SnapshotArticle struct {
	SourceID    string `json:"source_id"`
	SourceName  string `json:"source_name"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	PublishedAt string `json:"published_at"`
}

// SnapshotTeam represents a team with minimal metadata.
type SnapshotTeam struct {
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	ShortName    string `json:"short_name"`
	Abbreviation string `json:"abbreviation"`
}

// PublisherRef is a minimal reference to a publisher/source.
type PublisherRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// BuildSnapshot constructs a deterministic snapshot from stories and registry.
// It clusters stories, scores them, collects latest articles, and hydrates team metadata.
func BuildSnapshot(
	stories []Story,
	clusters []StoryGroup,
	scorer *Scorer,
	now time.Time,
	sourceNameResolver SourceNameResolver,
	teamRegistry *TeamRegistry,
	latestLimit int,
) (Snapshot, error) {
	if len(stories) == 0 {
		return Snapshot{
			Version:     SnapshotVersion,
			GeneratedAt: now.UTC().Format(time.RFC3339),
			Stories:     []SnapshotStory{},
			Latest:      []SnapshotArticle{},
			Teams:       []SnapshotTeam{},
		}, nil
	}

	// Build stories
	snapshotStories, err := buildStories(stories, clusters, scorer, now, sourceNameResolver)
	if err != nil {
		return Snapshot{}, fmt.Errorf("build stories: %w", err)
	}

	// Collect latest articles (from stories' ArticleIDs)
	latestArticles, err := buildLatestArticles(stories, latestLimit)
	if err != nil {
		return Snapshot{}, fmt.Errorf("build latest: %w", err)
	}

	// Collect unique teams referenced
	teams := buildTeams(teamRegistry)

	return Snapshot{
		Version:     SnapshotVersion,
		GeneratedAt: now.UTC().Format(time.RFC3339),
		Stories:     snapshotStories,
		Latest:      latestArticles,
		Teams:       teams,
	}, nil
}

func buildStories(stories []Story, clusters []StoryGroup, scorer *Scorer, now time.Time, resolver SourceNameResolver) ([]SnapshotStory, error) {
	if len(clusters) == 0 {
		return []SnapshotStory{}, nil
	}

	if len(stories) == 0 {
		return nil, fmt.Errorf("cannot build stories: no stories provided")
	}

	// Build snapshot stories from clusters
	ss := make([]SnapshotStory, 0)
	for _, g := range clusters {
		// Skip empty clusters (should be filtered earlier, but defend here)
		if len(g.Articles) == 0 {
			continue
		}

		// Collect publishers from articles
		publishers := buildPublishers(g.Articles, resolver)

		// Get score
		var score float64
		var evidence EvidenceLevel
		if len(g.Articles) > 0 {
			sb := scorer.WithTime(now).Score(g)
			score = sb.TotalScore
			evidence = sb.EvidenceLevel
		}

		ss = append(ss, SnapshotStory{
			ID:               g.ID,
			Title:            g.Headline,
			Summary:          g.Articles[0].Summary, // Use first article's summary
			FirstPublishedAt: g.FirstPublishedAt.UTC().Format(time.RFC3339),
			LastPublishedAt:  g.LastPublishedAt.UTC().Format(time.RFC3339),
			Score:            score,
			EvidenceLevel:    evidence,
			Publishers:       publishers,
		})
	}

	// Sort by ID for determinism
	sort.Slice(ss, func(i, j int) bool {
		return ss[i].ID < ss[j].ID
	})

	return ss, nil
}

func buildPublishers(articles []Article, resolver SourceNameResolver) []PublisherRef {
	seen := make(map[string]bool)
	var refs []PublisherRef

	for _, art := range articles {
		id := NormalizeSourceID(art.SourceID)
		if seen[id] {
			continue
		}
		seen[id] = true
		refs = append(refs, PublisherRef{
			ID:   id,
			Name: resolver.Resolve(art.SourceID),
			URL:  art.URL,
		})
	}

	// Sort by ID for determinism
	sort.Slice(refs, func(i, j int) bool {
		return refs[i].ID < refs[j].ID
	})

	return refs
}

func buildLatestArticles(stories []Story, limit int) ([]SnapshotArticle, error) {
	if len(stories) == 0 {
		return nil, nil
	}

	// Collect articles with valid URLs
	var articles []SnapshotArticle
	for _, s := range stories {
		if strings.TrimSpace(s.URL) == "" {
			continue
		}
		articles = append(articles, SnapshotArticle{
			SourceID:    NormalizeSourceID(s.Source),
			SourceName:  s.Source, // use raw source ID as name when no resolver
			Title:       s.Title,
			URL:         s.URL,
			PublishedAt: s.PublishedAt.UTC().Format(time.RFC3339),
		})
	}

	// Sort by publishedAt desc, then ID desc for determinism
	sort.Slice(articles, func(i, j int) bool {
		pi, err := articles[i].ParsePublishedAt()
		if err != nil {
			return false
		}
		pj, err := articles[j].ParsePublishedAt()
		if err != nil {
			return true
		}
		if pi.Equal(pj) {
			return articles[i].SourceID > articles[j].SourceID
		}
		return pi.After(pj)
	})

	// Limit
	if limit > 0 && len(articles) > limit {
		articles = articles[:limit]
	}

	return articles, nil
}

func (a SnapshotArticle) ParsePublishedAt() (time.Time, error) {
	return time.Parse(time.RFC3339, a.PublishedAt)
}

func buildTeams(registry *TeamRegistry) []SnapshotTeam {
	if registry == nil {
		return nil
	}

	teams := registry.All()
	result := make([]SnapshotTeam, 0)
	for _, t := range teams {
		result = append(result, SnapshotTeam{
			Slug:         t.Slug,
			Name:         t.Name,
			ShortName:    t.ShortName,
			Abbreviation: t.Abbreviation,
		})
	}

	// Sort by slug for determinism
	sort.Slice(result, func(i, j int) bool {
		return result[i].Slug < result[j].Slug
	})

	return result
}

// Render produces canonical JSON with stable field order.
func (s Snapshot) Render() ([]byte, error) {
	// Struct field order ensures stable output; MarshalIndent adds indentation
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	return b, nil
}

// Validate checks the snapshot for required fields and security constraints.
func (s Snapshot) Validate() error {
	// Check version
	if s.Version != SnapshotVersion {
		return fmt.Errorf("invalid version %q; must be %q", s.Version, SnapshotVersion)
	}

	// Check required fields
	if s.Stories == nil {
		return fmt.Errorf("stories must not be nil")
	}
	if s.Latest == nil {
		return fmt.Errorf("latest must not be nil")
	}
	if s.Teams == nil {
		return fmt.Errorf("teams must not be nil")
	}

	// Validate each story
	for i, story := range s.Stories {
		if story.ID == "" {
			return fmt.Errorf("story %d has empty ID", i)
		}
		if story.Title == "" {
			return fmt.Errorf("story %d (%s) has empty title", i, story.ID)
		}
		if story.Summary == "" {
			return fmt.Errorf("story %d (%s) has empty summary", i, story.ID)
		}
		// Check for potential secret leaks in summary (defense in depth)
		if isSecretPattern(story.Summary) {
			return fmt.Errorf("story %d (%s) summary appears to contain secrets", i, story.ID)
		}
		// Check for body content (should only have summary)
		if len(story.Summary) > 1024 {
			return fmt.Errorf("story %d (%s) summary too long (%d chars); likely contains body content", i, story.ID, len(story.Summary))
		}
	}

	// Validate each publisher ref
	for storyIdx, story := range s.Stories {
		for j, pub := range story.Publishers {
			if pub.ID == "" {
				return fmt.Errorf("story %d publisher %d has empty ID", storyIdx, j)
			}
			if pub.URL == "" {
				return fmt.Errorf("story %d publisher %d has empty URL", storyIdx, j)
			}
		}
	}

	// Validate each latest article
	for j, art := range s.Latest {
		if art.URL == "" {
			return fmt.Errorf("latest article %d has empty URL", j)
		}
	}

	// Validate each team
	for k, team := range s.Teams {
		if team.Slug == "" {
			return fmt.Errorf("team %d has empty slug", k)
		}
		if team.Name == "" {
			return fmt.Errorf("team %d has empty name", k)
		}
		if team.ShortName == "" {
			return fmt.Errorf("team %d (%s) has empty short_name", k, team.Slug)
		}
		if team.Abbreviation == "" {
			return fmt.Errorf("team %d (%s) has empty abbreviation", k, team.Slug)
		}
	}

	return nil
}

// isSecretPattern checks for common secret patterns (case-insensitive).
func isSecretPattern(s string) bool {
	lower := strings.ToLower(s)
	commonPatterns := []string{
		"password", "secret", "token", "credential", "api_key", "apikey",
		"private_key", "auth_token", "access_token", "refresh_token",
		"aws_secret", "database_url", "db_password",
	}
	for _, pat := range commonPatterns {
		if strings.Contains(lower, pat) {
			return true
		}
	}
	return false
}

// ContentEqual returns true if content matches excluding the generatedAt timestamp.
func (s Snapshot) ContentEqual(other Snapshot) bool {
	if s.Version != other.Version {
		return false
	}
	if len(s.Stories) != len(other.Stories) {
		return false
	}
	if len(s.Latest) != len(other.Latest) {
		return false
	}
	if len(s.Teams) != len(other.Teams) {
		return false
	}

	// Compare stories (already sorted by ID)
	for i := range s.Stories {
		if !s.Stories[i].equal(other.Stories[i]) {
			return false
		}
	}

	// Compare latest articles
	for i := range s.Latest {
		if !s.Latest[i].equal(other.Latest[i]) {
			return false
		}
	}

	// Compare teams
	for i := range s.Teams {
		if !s.Teams[i].equal(other.Teams[i]) {
			return false
		}
	}

	return true
}

func (ss SnapshotStory) equal(other SnapshotStory) bool {
	return ss.ID == other.ID &&
		ss.Title == other.Title &&
		ss.Summary == other.Summary &&
		ss.FirstPublishedAt == other.FirstPublishedAt &&
		ss.LastPublishedAt == other.LastPublishedAt &&
		ss.EvidenceLevel == other.EvidenceLevel &&
		equalPublishers(ss.Publishers, other.Publishers)
}

func equalPublishers(a, b []PublisherRef) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Name != b[i].Name || a[i].URL != b[i].URL {
			return false
		}
	}
	return true
}

func (sa SnapshotArticle) equal(other SnapshotArticle) bool {
	return sa.SourceID == other.SourceID &&
		sa.SourceName == other.SourceName &&
		sa.Title == other.Title &&
		sa.URL == other.URL &&
		sa.PublishedAt == other.PublishedAt
}

func (st SnapshotTeam) equal(other SnapshotTeam) bool {
	return st.Slug == other.Slug &&
		st.Name == other.Name &&
		st.ShortName == other.ShortName &&
		st.Abbreviation == other.Abbreviation
}

// PublicationResult captures the outcome of a snapshot publication.
type PublicationResult struct {
	Committed   bool   // Snapshot was written to disk
	Changed     bool   // Content differs from previous version
	Path        string // Path to the published file
	Version     string // Snapshot version
	PublishedAt string // generatedAt timestamp (RFC3339)
	Errors      []error
}

// SnapshotPublisher is the port for writing snapshot data.
type SnapshotPublisher interface {
	Publish(ctx context.Context, snapshot Snapshot) (PublicationResult, error)
}

// DefaultSnapshotPublisher is a no-op publisher for offline/dev.
type DefaultSnapshotPublisher struct{}

func (p *DefaultSnapshotPublisher) Publish(ctx context.Context, snapshot Snapshot) (PublicationResult, error) {
	return PublicationResult{
		Committed:   true, // offline means committed
		Changed:     false,
		Path:        "",
		Version:     SnapshotVersion,
		PublishedAt: time.Now().UTC().Format(time.RFC3339),
		Errors:      nil,
	}, nil
}
