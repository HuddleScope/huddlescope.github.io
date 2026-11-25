package domain

import (
	"crypto/sha256"
	"fmt"
	"math"
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
	ID           string
	SourceID     string
	Title        string
	DisplayTitle string // original feed casing for display; empty falls back to Title (#201)
	Summary      string
	URL          string
	GUID         string // optional feed GUID
	PublishedAt  string
	FirstSeenAt  string
	Author       string // optional author name
	Entities     []string
}

// Display returns the display title for the article: the original feed
// casing (DisplayTitle) when set, otherwise the normalized Title (#201).
func (a Article) Display() string {
	if d := strings.TrimSpace(a.DisplayTitle); d != "" {
		return a.DisplayTitle
	}
	return a.Title
}

// NewArticle creates an Article from a FeedEntry.
func NewArticle(feedEntry FeedEntry, id string, firstSeenAt string) Article {
	return Article{
		ID:           id,
		SourceID:     NormalizeSourceID(feedEntry.Source),
		Title:        NormalizeTitle(feedEntry.Title),
		DisplayTitle: DisplayNormalizeTitle(feedEntry.Title),
		Summary:      SanitizeHTML(feedEntry.Summary, 1024),
		URL:          CanonicalURLEntry(feedEntry.URL),
		GUID:         feedEntry.GUID,
		PublishedAt:  feedEntry.PublishedAt.UTC().Format(time.RFC3339),
		FirstSeenAt:  firstSeenAt,
		Author:       "",
		Entities:     feedEntry.Entities,
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

// NormalizedEntitySet returns the set of NFC-normalized, non-empty entity
// slugs for an article. Whitespace-only entities are ignored.
func (a Article) NormalizedEntitySet() map[string]bool {
	set := make(map[string]bool, len(a.Entities))
	for _, e := range a.Entities {
		key := NormalizeSourceID(e)
		if key == "" {
			continue
		}
		set[key] = true
	}
	return set
}

// EntityAffinity computes the Jaccard affinity between two articles' entity
// sets. The semantics encode the issue #25 constraint that entity overlap is a
// *constraint*, never a merge signal on its own:
//   - both entity-less            -> 1.0 (neutral; feeds without entities stay lenient)
//   - one side entity-less        -> 1.0 (neutral; no evidence against merging)
//   - both have entities          -> Jaccard over normalized sets (0.0 disjoint .. 1.0 identical)
func EntityAffinity(a, b Article) float64 {
	aHas, bHas := a.HasEntities(), b.HasEntities()
	if !aHas || !bHas {
		return 1.0
	}
	aSet := a.NormalizedEntitySet()
	bSet := b.NormalizedEntitySet()
	if len(aSet) == 0 || len(bSet) == 0 {
		return 1.0
	}
	intersection := 0
	for k := range aSet {
		if bSet[k] {
			intersection++
		}
	}
	union := len(aSet) + len(bSet) - intersection
	if union == 0 {
		return 0.0
	}
	return float64(intersection) / float64(union)
}

// JoinDecision records why an article joined a story group (AC-3a).
// Reason is one of: "url" (canonical-URL fast path), "title" (title Jaccard alone
// met the threshold), "summary" (summary fast path with identical/compatible entities),
// or "title+summary" (weighted combination met the threshold).
type JoinDecision struct {
	GroupID        string  `json:"groupId"`
	ArticleID      string  `json:"articleId"`
	SourceID       string  `json:"sourceId"`
	Reason         string  `json:"reason"`
	TitleScore     float64 `json:"titleScore"`
	SummaryScore   float64 `json:"summaryScore"`
	EntityAffinity float64 `json:"entityAffinity"`
	Threshold      float64 `json:"threshold"`
	At             string  `json:"at"`
}

// StoryStatus is the lifecycle state of a story group (issue #26, AC-2).
// The zero value ("") means active, so groups produced by clustering are
// active by default.
type StoryStatus string

const (
	// StatusActive is the default lifecycle state.
	StatusActive StoryStatus = "active"
	// StatusArchived marks a story as archived (no longer tracked, page
	// renders an archive banner).
	StatusArchived StoryStatus = "archived"
	// StatusClosed marks a story as closed (same rendering as archived;
	// "closed" is the editorial term for a story that has ended).
	StatusClosed StoryStatus = "closed"
)

// StoryAlias records the identity of a story that was merged into this one
// (issue #26, AC-2). Slug is the stable URL slug of the merged-away story;
// Headline is its representative headline (display text only); MergedAt is
// the RFC3339 time the merge rule was applied in this run.
type StoryAlias struct {
	Slug     StorySlug
	Headline string
	MergedAt string
}

// StoryGroup represents a group of articles that belong to the same story.
type StoryGroup struct {
	ID               string
	Headline         string // normalized headline (feeds slugs, group ID, tokenization)
	DisplayHeadline  string // original-casing headline for display; empty falls back to Headline (#201)
	Articles         []Article
	CreatedAt        time.Time
	FirstPublishedAt time.Time
	LastPublishedAt  time.Time
	JoinReasons      []JoinDecision
	// Status is the story lifecycle state set by ApplyLifecycle (issue #26).
	// Zero value = active.
	Status StoryStatus
	// Aliases are identities of stories merged into this group (issue #26).
	// Deterministic: recomputed from the partition + rules on every run.
	Aliases []StoryAlias
}

// NewStoryGroup creates a new story group from a single article.
func NewStoryGroup(article Article) StoryGroup {
	return StoryGroup{
		ID:               computeGroupID([]Article{article}),
		Headline:         article.Title,
		DisplayHeadline:  article.Display(),
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
		g.DisplayHeadline = article.Display()
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
	// EntityAffinityMin is the minimum entity-set Jaccard affinity required
	// before a scored merge is allowed when BOTH articles carry entities
	// (AC-1). Defaults to 0.5. Setting it to 1.0 restores the pre-#25 lenient
	// behavior (any shared entity suffices).
	EntityAffinityMin float64
}

// TitleWeight and SummaryWeight are the weights for combining title and summary
// similarity scores in the clustering algorithm. Title weight should be higher
// as titles are typically more discriminative than summaries.
const (
	TitleWeight   = 0.70
	SummaryWeight = 0.30
)

// DefaultEntityAffinityMin is the default minimum entity-set affinity (AC-1).
const DefaultEntityAffinityMin = 0.5

// NewClusterer creates a new Clusterer with default settings.
func NewClusterer() *Clusterer {
	return &Clusterer{
		SimilarityThreshold: 0.50,
		TemporalWindow:      48 * time.Hour,
		EntityAffinityMin:   DefaultEntityAffinityMin,
	}
}

// FingerprintSummary tokenizes a summary/lead text for matching.
// Uses the same normalization as Title (stopwords removal, lowercase, etc.).
func FingerprintSummary(text string) []string {
	return TokenizeTitle(text)
}

// Cluster groups the input articles into story groups. It is a thin wrapper
// over ClusterWithDecisions that discards the per-article join decisions, so
// existing call sites keep working unchanged.
func (c *Clusterer) Cluster(articles []Article) []StoryGroup {
	groups, _ := c.ClusterWithDecisions(articles)
	return groups
}

// ClusterWithDecisions groups the input articles into story groups and returns
// the per-article join decisions (AC-3a). It is deterministic: the same input
// always yields the same group IDs and the same decisions (except the decision
// timestamps, which reflect the run).
func (c *Clusterer) ClusterWithDecisions(articles []Article) ([]StoryGroup, []JoinDecision) {
	if len(articles) == 0 {
		return nil, nil
	}

	// If a caller constructed the Clusterer directly without a default affinity
	// floor (e.g. a zero struct literal), fall back to the documented default so
	// the AC-1 constraint is always in effect.
	affinityMin := c.EntityAffinityMin
	if affinityMin <= 0 {
		affinityMin = DefaultEntityAffinityMin
	}

	sorted := make([]Article, len(articles))
	copy(sorted, articles)
	sort.Slice(sorted, func(i, j int) bool {
		ti := sorted[i].ParsePublishedAtOrZero()
		tj := sorted[j].ParsePublishedAtOrZero()
		return ti.Before(tj)
	})

	var groups []StoryGroup
	var decisions []JoinDecision
	now := time.Now().UTC().Format(time.RFC3339)

	for _, article := range sorted {
		bestGroup := -1
		bestScore := -1.0
		var bestDecision JoinDecision

		for i, group := range groups {
			groupFirstTime := group.FirstPublishedAt
			articleTime := article.ParsePublishedAtOrZero()

			if !TimeWithinWindow(articleTime, groupFirstTime, c.TemporalWindow) {
				continue
			}

			if len(group.Articles) == 0 {
				continue
			}
			head := group.Articles[0]

			// Entity gate (unchanged): an article with entities never joins a group
			// whose head has entities and shares none of them. This is a hard
			// disjointness constraint.
			if article.HasEntities() && head.HasEntities() && !article.HasOverlappingEntity(&head) {
				continue
			}

			affinity := EntityAffinity(article, head)
			bothHaveEntities := article.HasEntities() && head.HasEntities()

			// Check for exact URL match (fast path for canonicalized URLs).
			// AC-1: a cross-source URL match only merges when the entity sets are
			// compatible (identical) or neutral (one/both sides entity-less), or when
			// the sources match. Partial-overlap-only URL matches fall through to the
			// scored path, where the affinity floor applies.
			urlMatch := false
			urlSameSource := false
			if article.URL != "" {
				urlSameSource = article.SourceID == head.SourceID
				for _, ga := range group.Articles {
					if CanonicalURL(article.URL) == CanonicalURL(ga.URL) {
						if urlSameSource || affinity >= 1.0 || (!article.HasEntities() && !ga.HasEntities()) {
							urlMatch = true
						}
						break
					}
				}
			}

			if urlMatch {
				if i > bestGroup {
					bestGroup = i
					bestScore = 1.0
					bestDecision = JoinDecision{
						Reason:         "url",
						EntityAffinity: affinity,
						Threshold:      1.0,
						At:             now,
					}
				}
				continue
			}

			// Scored path.
			articleTokens := TokenizeTitle(article.Title)
			groupTokens := TokenizeTitle(group.Headline)
			titleScore := JaccardSimilarity(articleTokens, groupTokens)

			summaryScore := 0.0
			if article.Summary != "" && head.Summary != "" {
				articleSummaryTokens := FingerprintSummary(article.Summary)
				groupSummaryTokens := FingerprintSummary(head.Summary)
				summaryScore = JaccardSimilarity(articleSummaryTokens, groupSummaryTokens)
			}

			combinedScore := TitleWeight*titleScore + SummaryWeight*summaryScore

			// AC-1 constraint: when both articles carry entities, the entity sets
			// must be at least minimally compatible. Disjoint / low-affinity entity
			// sets (e.g. the same team name on two unrelated events) block merging.
			if bothHaveEntities && affinity < affinityMin {
				continue
			}

			// Determine merge score + reason. The raw summary fast path
			// (math.Max(combined, summary)) is only allowed when entity sets are
			// identical (affinity == 1.0) or neutral; otherwise the boilerplate-lead
			// bypass is closed and we require titleScore alone to clear the
			// threshold.
			var score float64
			var reason string
			switch {
			case affinity >= 1.0:
				// Identical/compatible entity sets: keep the historical lenient
				// behavior (summary may rescue a changed title).
				score = math.Max(combinedScore, summaryScore)
				reason = reasonForScore(combinedScore, titleScore, summaryScore)
			case !bothHaveEntities:
				// One side entity-less: neutral; keep historical behavior.
				score = math.Max(combinedScore, summaryScore)
				reason = reasonForScore(combinedScore, titleScore, summaryScore)
			default:
				// Shared-but-not-identical entity sets: require the title alone to
				// clear the threshold. Kills the boilerplate-summary bypass.
				if titleScore < c.SimilarityThreshold {
					continue
				}
				score = math.Max(titleScore, combinedScore)
				reason = "title"
			}

			if score > bestScore && score >= c.SimilarityThreshold {
				bestScore = score
				bestGroup = i
				bestDecision = JoinDecision{
					Reason:         reason,
					TitleScore:     titleScore,
					SummaryScore:   summaryScore,
					EntityAffinity: affinity,
					Threshold:      c.SimilarityThreshold,
					At:             now,
				}
			}
		}

		if bestGroup >= 0 {
			groups[bestGroup].AddArticle(article)
			bestDecision.GroupID = groups[bestGroup].ID
			bestDecision.ArticleID = article.ID
			bestDecision.SourceID = article.SourceID
			// Persist the join decision on the group so it survives into the
			// Story (AC-3a) and into ApplyCorrections re-partitioning.
			groups[bestGroup].JoinReasons = append(groups[bestGroup].JoinReasons, bestDecision)
			decisions = append(decisions, bestDecision)
		} else {
			groups = append(groups, NewStoryGroup(article))
		}
	}

	return groups, decisions
}

// reasonForScore picks a reason code from the score components. "summary" is
// returned when the summary fast path alone clears the threshold while the title
// does not; "title" when the title alone does; otherwise "title+summary".
func reasonForScore(combined, titleScore, summaryScore float64) string {
	switch {
	case titleScore >= 0.50 && summaryScore < 0.50:
		return "title"
	case summaryScore >= 0.50 && titleScore < 0.50:
		return "summary"
	default:
		return "title+summary"
	}
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
	// Track earliest and latest for metadata
	var earliest, latest Article
	var earliestIsSet, latestIsSet bool

	for _, a := range group.Articles {
		publishedAt := a.ParsePublishedAtOrZero()
		if !earliestIsSet || publishedAt.Before(earliest.ParsePublishedAtOrZero()) {
			earliest = a
			earliestIsSet = true
		}
		if !latestIsSet || publishedAt.After(latest.ParsePublishedAtOrZero()) {
			latest = a
			latestIsSet = true
		}
	}

	articleData := make([]Article, len(group.Articles))
	articleIDs := make([]string, len(group.Articles))
	for i, a := range group.Articles {
		articleData[i] = a
		articleIDs[i] = a.ID
	}
	sort.Strings(articleIDs)

	return Story{
		ID:           group.ID,
		Title:        group.Headline,
		DisplayTitle: group.DisplayHeadline,
		Source:       earliest.SourceID,
		PublishedAt:  earliest.ParsePublishedAtOrZero(),
		Summary:      earliest.Summary,
		URL:          earliest.URL,
		ArticleData:  articleData,
		ArticleIDs:   articleIDs,
		Headline:     group.Headline,
		GUID:         earliest.URL,
		CanonicalURL: earliest.URL,
		DedupeKey:    "",
		Entities:     earliest.Entities,
		FirstSeenAt:  time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
		JoinReasons:  group.JoinReasons,
	}
}
