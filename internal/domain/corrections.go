package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// CorrectionKind identifies the type of a correction rule.
type CorrectionKind string

const (
	// CorrectionSplit removes the listed articles from a story group and places
	// them into new singleton (or split) groups.
	CorrectionSplit CorrectionKind = "split"
	// CorrectionMerge moves all articles of StoryID into the group IntoStoryID.
	CorrectionMerge CorrectionKind = "merge"
)

// CorrectionRule is a stored, versioned, deterministic override applied after
// automatic clustering (AC-3b). Rules are idempotent: applying the same set of
// rules to the same input always yields the same groups.
type CorrectionRule struct {
	ID          string
	Kind        CorrectionKind
	StoryID     string   // the group to split (Split) or the group to move (Merge source)
	IntoStoryID string   // the group to move into (Merge target)
	ArticleIDs  []string // articles to remove from StoryID (Split)
	CreatedAt   string   // RFC3339
	CreatedBy   string
}

// Validate performs intrinsic (kind-specific) validation on the rule. It does
// NOT check references against a concrete set of groups; callers that need that
// use ValidateAgainstGroups. Rejects malformed rules so garbage cannot be
// persisted (security).
func (r CorrectionRule) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("correction rule ID must not be empty")
	}
	switch r.Kind {
	case CorrectionSplit:
		if r.StoryID == "" {
			return fmt.Errorf("split rule %s requires StoryID", r.ID)
		}
		if len(r.ArticleIDs) == 0 {
			return fmt.Errorf("split rule %s requires at least one ArticleID", r.ID)
		}
		if len(r.ArticleIDs) == 1 && !validArticleID(r.ArticleIDs[0]) {
			return fmt.Errorf("split rule %s has invalid article ID %q", r.ID, r.ArticleIDs[0])
		}
	case CorrectionMerge:
		if r.StoryID == "" || r.IntoStoryID == "" {
			return fmt.Errorf("merge rule %s requires both StoryID and IntoStoryID", r.ID)
		}
		if r.StoryID == r.IntoStoryID {
			return fmt.Errorf("merge rule %s cannot merge a group into itself", r.ID)
		}
	default:
		return fmt.Errorf("correction rule %s has unknown kind %q (want split or merge)", r.ID, r.Kind)
	}
	return nil
}

// ValidateAgainstGroups checks that every group/article reference in the rule
// resolves to something in the provided groups. Used at application time so
// stale rules referencing vanished groups surface a clear error instead of
// silently corrupting the partition.
func (r CorrectionRule) ValidateAgainstGroups(groups []StoryGroup) error {
	if err := r.Validate(); err != nil {
		return err
	}
	byID := make(map[string]StoryGroup, len(groups))
	for _, g := range groups {
		byID[g.ID] = g
	}
	switch r.Kind {
	case CorrectionSplit:
		g, ok := byID[r.StoryID]
		if !ok {
			return fmt.Errorf("split rule %s references unknown group %q", r.ID, r.StoryID)
		}
		member := make(map[string]bool, len(g.Articles))
		for _, a := range g.Articles {
			member[a.ID] = true
		}
		for _, aid := range r.ArticleIDs {
			if !member[aid] {
				return fmt.Errorf("split rule %s references article %q not in group %q", r.ID, aid, r.StoryID)
			}
		}
		// A split must not remove every article from the group.
		if len(r.ArticleIDs) >= len(g.Articles) {
			return fmt.Errorf("split rule %s would empty group %q", r.ID, r.StoryID)
		}
	case CorrectionMerge:
		if _, ok := byID[r.StoryID]; !ok {
			return fmt.Errorf("merge rule %s references unknown source group %q", r.ID, r.StoryID)
		}
		if _, ok := byID[r.IntoStoryID]; !ok {
			return fmt.Errorf("merge rule %s references unknown target group %q", r.ID, r.IntoStoryID)
		}
	}
	return nil
}

// ApplyCorrections deterministically applies a set of correction rules to a
// clustered partition (AC-3b). It is idempotent: applying the same rules to an
// already-corrected partition is a no-op. Rules are applied in a canonical
// (ID-sorted) order so the result does not depend on input ordering.
//
// Cycle detection: a set of merge rules that forms a cycle (A→B and B→A) is
// rejected with an error and the input partition is left unmodified.
func ApplyCorrections(groups []StoryGroup, rules []CorrectionRule) ([]StoryGroup, error) {
	if len(rules) == 0 {
		return groups, nil
	}

	// Intrinsic validation first (malformed rules are always rejected).
	for _, r := range rules {
		if err := r.Validate(); err != nil {
			return nil, err
		}
	}

	// Canonical order by ID for determinism.
	sortedRules := make([]CorrectionRule, len(rules))
	copy(sortedRules, rules)
	sort.Slice(sortedRules, func(i, j int) bool { return sortedRules[i].ID < sortedRules[j].ID })

	// Detect merge cycles up front (A→B and B→A, or longer chains).
	if err := detectMergeCycles(sortedRules); err != nil {
		return nil, err
	}

	// Work on copies so the input partition is never mutated on error.
	work := make([]StoryGroup, len(groups))
	for i, g := range groups {
		work[i] = g
		work[i].Articles = append([]Article(nil), g.Articles...)
	}

	for _, rule := range sortedRules {
		if err := rule.ValidateAgainstGroups(work); err != nil {
			// A rule that cannot resolve is invalid for this partition. Report it
			// without mutating: return a fresh copy of the original groups.
			return groups, err
		}
		applyRule(&work, rule)
	}

	// Recompute group IDs deterministically after any re-partitioning.
	for i := range work {
		work[i].ID = computeGroupID(work[i].Articles)
	}

	return work, nil
}

// detectMergeCycles returns an error if the merge rules form a cycle. A cycle
// such as A→B combined with B→A is rejected (AC-3b: cyclic merges rejected).
func detectMergeCycles(rules []CorrectionRule) error {
	// Build a directed graph StoryID -> IntoStoryID from merge rules.
	next := make(map[string]string)
	for _, r := range rules {
		if r.Kind != CorrectionMerge {
			continue
		}
		if prev, ok := next[r.StoryID]; ok && prev != r.IntoStoryID {
			return fmt.Errorf("merge rule %s conflicts: group %q has multiple merge targets", r.ID, r.StoryID)
		}
		next[r.StoryID] = r.IntoStoryID
	}

	// Walk each node; a node that can reach itself is a cycle.
	for start := range next {
		visited := make(map[string]bool)
		cur := start
		for !visited[cur] {
			visited[cur] = true
			nxt, ok := next[cur]
			if !ok {
				break
			}
			if nxt == start {
				return fmt.Errorf("cyclic merge detected: %q -> %q (and back)", start, nxt)
			}
			cur = nxt
		}
	}
	return nil
}

// applyRule mutates the work partition in place.
func applyRule(groups *[]StoryGroup, rule CorrectionRule) {
	var result []StoryGroup

	switch rule.Kind {
	case CorrectionSplit:
		var moved []Article
		for i := range *groups {
			g := (*groups)[i]
			if g.ID != rule.StoryID {
				result = append(result, g)
				continue
			}
			removed := make(map[string]bool, len(rule.ArticleIDs))
			for _, aid := range rule.ArticleIDs {
				removed[aid] = true
			}
			var kept []Article
			for _, a := range g.Articles {
				if removed[a.ID] {
					moved = append(moved, a)
				} else {
					kept = append(kept, a)
				}
			}
			if len(kept) > 0 {
				g.Articles = kept
				result = append(result, g)
			}
			// The moved articles become their own group(s). For a single moved
			// article that is a singleton; for multiple, one new group carrying
			// them (they were already known to be related by the operator).
			if len(moved) > 0 {
				mg := StoryGroup{Articles: moved}
				mg.Headline = moved[0].Title
				mg.CreatedAt = time.Now().UTC()
				mg.ID = computeGroupID(moved)
				result = append(result, mg)
			}
		}
		*groups = result
	case CorrectionMerge:
		// First locate the target index and collect the source articles.
		targetIdx := -1
		var sourceArticles []Article
		for i := range *groups {
			if (*groups)[i].ID == rule.IntoStoryID {
				targetIdx = i
			}
			if (*groups)[i].ID == rule.StoryID {
				sourceArticles = (*groups)[i].Articles
			}
		}
		if targetIdx == -1 {
			return // ValidateAgainstGroups would have caught this; defensive no-op
		}
		// Rebuild without the source group, appending source articles to the target.
		var result []StoryGroup
		for i := range *groups {
			if (*groups)[i].ID == rule.StoryID {
				continue // drop the source group
			}
			g := (*groups)[i]
			g.Articles = append([]Article(nil), g.Articles...)
			if i == targetIdx {
				g.Articles = append(g.Articles, sourceArticles...)
			}
			result = append(result, g)
		}
		*groups = result
	}
}

// validArticleID is a lenient shape check for article IDs (16-hex group ids or
// any non-empty token). It rejects obvious garbage (empty / whitespace only).
func validArticleID(id string) bool {
	return strings.TrimSpace(id) != ""
}
