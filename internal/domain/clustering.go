package domain

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

// TokenizeTitle splits the normalized title into word tokens.
// Removes punctuation, converts to lowercase, and filters out common stopwords.
func TokenizeTitle(title string) []string {
	if title == "" {
		return []string{}
	}

	stopwords := map[string]bool{
		"a": true, "an": true, "the": true, "and": true, "or": true, "but": true,
		"in": true, "on": true, "at": true, "to": true, "for": true,
		"of": true, "with": true, "by": true, "from": true,
		"is": true, "are": true, "was": true, "were": true, "be": true, "been": true,
		"have": true, "has": true, "had": true, "do": true, "does": true, "did": true,
		"will": true, "would": true, "could": true, "should": true, "may": true, "might": true,
		"must": true, "shall": true, "can": true, "need": true, "dare": true, "ought": true,
		"used": true,
	}

	var tokens []string
	var currentToken []rune

	for _, r := range strings.ToLower(title) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			currentToken = append(currentToken, r)
		} else {
			if len(currentToken) > 0 {
				token := string(currentToken)
				if !stopwords[token] {
					tokens = append(tokens, token)
				}
				currentToken = nil
			}
		}
	}

	if len(currentToken) > 0 {
		token := string(currentToken)
		if !stopwords[token] {
			tokens = append(tokens, token)
		}
	}

	return tokens
}

// JaccardSimilarity computes the Jaccard similarity between two token sets.
func JaccardSimilarity(tokens1, tokens2 []string) float64 {
	if len(tokens1) == 0 && len(tokens2) == 0 {
		return 1.0
	}
	if len(tokens1) == 0 || len(tokens2) == 0 {
		return 0.0
	}

	set1 := make(map[string]bool)
	for _, t := range tokens1 {
		set1[t] = true
	}

	set2 := make(map[string]bool)
	for _, t := range tokens2 {
		set2[t] = true
	}

	intersection := 0
	for t := range set1 {
		if set2[t] {
			intersection++
		}
	}

	union := len(set1) + len(set2) - intersection

	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// TimeDiffHours returns the absolute difference in hours between two times.
func TimeDiffHours(t1, t2 time.Time) float64 {
	diff := t1.Sub(t2)
	if diff < 0 {
		diff = -diff
	}
	return diff.Hours()
}

// TimeWithinWindow checks if two times are within the given duration window.
func TimeWithinWindow(t1, t2 time.Time, window time.Duration) bool {
	return TimeDiffHours(t1, t2) <= window.Hours()
}

// Article represents a single article that can be grouped into stories.
type Article struct {
	ID          string
	SourceID    string
	Title       string
	Summary     string
	URL         string
	PublishedAt string
	FirstSeenAt string
	Entities    []string
}

// NewArticle creates an Article from a FeedEntry.
func NewArticle(feedEntry FeedEntry, id string, firstSeenAt string) Article {
	return Article{
		ID:          id,
		SourceID:    NormalizeSourceID(feedEntry.Source),
		Title:       NormalizeTitle(feedEntry.Title),
		Summary:     SanitizeHTML(feedEntry.Summary, 1024),
		URL:         CanonicalURLEntry(feedEntry.URL),
		PublishedAt: feedEntry.PublishedAt.UTC().Format(time.RFC3339),
		FirstSeenAt: firstSeenAt,
		Entities:    feedEntry.Entities,
	}
}

// ParsePublishedAt parses the PublishedAt string back to time.Time.
func (a Article) ParsePublishedAt() (time.Time, error) {
	return time.Parse(time.RFC3339, a.PublishedAt)
}

// ParsePublishedAtOrZero parses the PublishedAt string or returns zero time.
func (a Article) ParsePublishedAtOrZero() time.Time {
	t, err := time.Parse(time.RFC3339, a.PublishedAt)
	if err != nil {
		return time.Time{}
	}
	return t
}

// HasEntities returns true if the article has at least one entity.
func (a Article) HasEntities() bool {
	return len(a.Entities) > 0
}

// HasOverlappingEntity checks if the article shares any entity with another.
func (a Article) HasOverlappingEntity(other *Article) bool {
	if !a.HasEntities() || !other.HasEntities() {
		return false
	}
	for _, e1 := range a.Entities {
		for _, e2 := range other.Entities {
			if NormalizeSourceID(e1) == NormalizeSourceID(e2) {
				return true
			}
		}
	}
	return false
}

// StoryGroup represents a group of articles that belong to the same story.
type StoryGroup struct {
	ID               string
	Headline         string
	Articles         []Article
	CreatedAt        time.Time
	FirstPublishedAt time.Time
	LastPublishedAt  time.Time
}

// NewStoryGroup creates a new story group from a single article.
func NewStoryGroup(article Article) StoryGroup {
	return StoryGroup{
		ID:               computeGroupID([]Article{article}),
		Headline:         article.Title,
		Articles:         []Article{article},
		CreatedAt:        time.Now().UTC(),
		FirstPublishedAt: article.ParsePublishedAtOrZero(),
		LastPublishedAt:  article.ParsePublishedAtOrZero(),
	}
}

// AddArticle adds an article to the group, updating metadata if needed.
func (g *StoryGroup) AddArticle(article Article) {
	g.Articles = append(g.Articles, article)

	publishedAt := article.ParsePublishedAtOrZero()
	if publishedAt.Before(g.FirstPublishedAt) {
		g.FirstPublishedAt = publishedAt
		g.Headline = article.Title
	}

	if publishedAt.After(g.LastPublishedAt) {
		g.LastPublishedAt = publishedAt
	}

	g.ID = computeGroupID(g.Articles)
}

// Size returns the number of articles in the group.
func (g StoryGroup) Size() int {
	return len(g.Articles)
}

// computeGroupID computes a deterministic ID for a group.
func computeGroupID(articles []Article) string {
	sorted := make([]Article, len(articles))
	copy(sorted, articles)
	sort.Slice(sorted, func(i, j int) bool {
		ti := sorted[i].ParsePublishedAtOrZero()
		tj := sorted[j].ParsePublishedAtOrZero()
		return ti.Before(tj)
	})

	var parts []string
	for _, a := range sorted {
		parts = append(parts, a.Title)
	}
	parts = append(parts, sorted[0].PublishedAt)

	input := strings.Join(parts, "||")
	return fmt.Sprintf("%x", sha256.Sum256([]byte(input)))[:16]
}

// Clusterer groups articles into story groups.
type Clusterer struct {
	SimilarityThreshold float64
	TemporalWindow      time.Duration
}

// NewClusterer creates a new Clusterer with default settings.
func NewClusterer() *Clusterer {
	return &Clusterer{
		SimilarityThreshold: 0.50,
		TemporalWindow:      48 * time.Hour,
	}
}

// Cluster groups the input articles into story groups.
func (c *Clusterer) Cluster(articles []Article) []StoryGroup {
	if len(articles) == 0 {
		return nil
	}

	sorted := make([]Article, len(articles))
	copy(sorted, articles)
	sort.Slice(sorted, func(i, j int) bool {
		ti := sorted[i].ParsePublishedAtOrZero()
		tj := sorted[j].ParsePublishedAtOrZero()
		return ti.Before(tj)
	})

	var groups []StoryGroup

	for _, article := range sorted {
		bestGroup := -1
		bestScore := -1.0

		for i, group := range groups {
			groupFirstTime := group.FirstPublishedAt
			articleTime := article.ParsePublishedAtOrZero()

			if !TimeWithinWindow(articleTime, groupFirstTime, c.TemporalWindow) {
				continue
			}

			if article.HasEntities() {
				if len(group.Articles) > 0 {
					groupFirstArticle := group.Articles[0]
					if groupFirstArticle.HasEntities() && !article.HasOverlappingEntity(&groupFirstArticle) {
						continue
					}
				}
			}

			score := JaccardSimilarity(
				TokenizeTitle(article.Title),
				TokenizeTitle(group.Headline),
			)

			if score > bestScore && score >= c.SimilarityThreshold {
				bestScore = score
				bestGroup = i
			}
		}

		if bestGroup >= 0 {
			groups[bestGroup].AddArticle(article)
		} else {
			groups = append(groups, NewStoryGroup(article))
		}
	}

	return groups
}

// GroupingResult represents the result of a grouping operation.
type GroupingResult struct {
	GroupsCreated   int
	GroupsMerged    int
	ArticlesGrouped int
	ArticlesSolo    int
	Errors          []error
}

// NewGroupingResult creates a new GroupingResult.
func NewGroupingResult() GroupingResult {
	return GroupingResult{
		GroupsCreated:   0,
		GroupsMerged:    0,
		ArticlesGrouped: 0,
		ArticlesSolo:    0,
		Errors:          nil,
	}
}

// NewStoryFromGroup creates a Story from a StoryGroup.
func NewStoryFromGroup(group StoryGroup) Story {
	var earliest Article
	for i, a := range group.Articles {
		if i == 0 {
			earliest = a
		} else if a.ParsePublishedAtOrZero().Before(earliest.ParsePublishedAtOrZero()) {
			earliest = a
		}
	}

	articleIDs := make([]string, len(group.Articles))
	for i, a := range group.Articles {
		articleIDs[i] = a.ID
	}
	sort.Strings(articleIDs)

	return Story{
		ID:           group.ID,
		Title:        group.Headline,
		Source:       earliest.SourceID,
		PublishedAt:  earliest.ParsePublishedAtOrZero(),
		Summary:      earliest.Summary,
		URL:          earliest.URL,
		ArticleIDs:   articleIDs,
		Headline:     group.Headline,
		GUID:         earliest.URL,
		CanonicalURL: earliest.URL,
		DedupeKey:    "",
		Entities:     earliest.Entities,
		FirstSeenAt:  time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
}
