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
	// CorrectionArchive marks the story referenced by StoryID (as a slug) as
	// archived (issue #26).
	CorrectionArchive CorrectionKind = "archive"
	// CorrectionClose marks the story referenced by StoryID (as a slug) as closed
	// (issue #26).
	CorrectionClose CorrectionKind = "close"
	// CorrectionReactivate restores the story referenced by StoryID (as a slug)
	// to active (issue #26).
	CorrectionReactivate CorrectionKind = "reactivate"
	// CorrectionAlias records an explicit alias (IntoStoryID, as a slug) on the
	// story referenced by StoryID (as a slug) (issue #26). Escape hatch for
	// representative swaps.
	CorrectionAlias CorrectionKind = "alias"
)

// CorrectionRule is a stored, versioned, deterministic override applied after
// automatic clustering (AC-3b). Rules are idempotent: applying the same set of
// rules to the same input always yields the same groups.
//
// Reference semantics differ by kind (issue #26) and map onto the existing
// correction_rules schema (story_id / into_story_id TEXT columns) so no schema
// migration is required:
//   - split: StoryID = the group to split (group ID); ArticleIDs = articles to
//     remove.
//   - merge: StoryID = the group to move (group ID); IntoStoryID = the target
//     group (group ID).
//   - archive/close/reactivate: StoryID = the story's stable URL slug.
//   - alias: StoryID = the target story's stable URL slug; IntoStoryID = the
//     alias slug to record.
//
// Lifecycle kinds reference stories by slug (stable across days, unlike group
// IDs). A slug that also matches a group ID keeps slug semantics for lifecycle
// kinds (plan risk #2); ambiguity (a slug matching multiple groups) fails
// loudly in ApplyLifecycle.
type CorrectionRule struct {
	ID          string
	Kind        CorrectionKind
	StoryID     string   // split: group to split; merge: source group; lifecycle/alias: target story slug
	IntoStoryID string   // merge: target group; alias: the alias slug to record
	ArticleIDs  []string // split: articles to remove from StoryID
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
	case CorrectionArchive, CorrectionClose, CorrectionReactivate:
		// Lifecycle kinds reference the story by its stable slug, carried in the
		// existing story_id column (no schema migration).
		if !validSlug(r.StoryID) {
			return fmt.Errorf("%s rule %s requires a valid story slug in StoryID (got %q)", r.Kind, r.ID, r.StoryID)
		}
	case CorrectionAlias:
		// alias: StoryID = target story slug; IntoStoryID = alias slug.
		if !validSlug(r.StoryID) {
			return fmt.Errorf("alias rule %s requires a valid target story slug in StoryID (got %q)", r.ID, r.StoryID)
		}
		if !validSlug(r.IntoStoryID) {
			return fmt.Errorf("alias rule %s requires a valid alias slug in IntoStoryID (got %q)", r.ID, r.IntoStoryID)
		}
		if r.StoryID == r.IntoStoryID {
			return fmt.Errorf("alias rule %s cannot alias a story onto itself", r.ID)
		}
	default:
		return fmt.Errorf("correction rule %s has unknown kind %q (want split, merge, archive, close, reactivate or alias)", r.ID, r.Kind)
	}
	return nil
}

// validSlug reports whether s is a non-empty URL-safe slug restricted to the
// [a-z0-9-] charset (the same charset ComputeStorySlugFromHeadline emits).
func validSlug(s string) bool {
	if s == "" || strings.TrimSpace(s) != s {
		return false
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return false
	}
	return true
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
	case CorrectionArchive, CorrectionClose, CorrectionReactivate:
		// StoryID carries the slug for lifecycle kinds.
		if !groupHasSlug(groups, r.StoryID) {
			return fmt.Errorf("%s rule %s references unknown story slug %q", r.Kind, r.ID, r.StoryID)
		}
	case CorrectionAlias:
		// StoryID carries the target story slug; IntoStoryID (the alias slug) is
		// not required to reference an existing group (it may be a historical
		// slug).
		if !groupHasSlug(groups, r.StoryID) {
			return fmt.Errorf("alias rule %s references unknown story slug %q", r.ID, r.StoryID)
		}
	}
	return nil
}

// groupHasSlug reports whether any group's representative slug matches s.
// The representative slug is the stable public identity of the group.
func groupHasSlug(groups []StoryGroup, s string) bool {
	for _, g := range groups {
		if groupRepSlug(g) == StorySlug(s) {
			return true
		}
	}
	return false
}

// groupRepSlug returns the representative slug of a group (empty for an empty
// group). It mirrors the slug computation in BuildStoryDetailPage so that
// rule resolution and page identity stay in lockstep.
func groupRepSlug(g StoryGroup) StorySlug {
	if len(g.Articles) == 0 {
		return ""
	}
	return StorySlug(ComputeStorySlugFromHeadline(pickRepresentative(g.Articles).Title))
}

// ApplyCorrections deterministically applies a set of correction rules to a
// clustered partition (AC-3b). It is idempotent: applying the same rules to an
// already-corrected partition is a no-op. Rules are applied in a canonical
// (ID-sorted) order so the result does not depend on input ordering.
//
// Only split/merge rules re-partition here; lifecycle kinds (archive/close/
// reactivate/alias, issue #26) are accepted by Validate and skipped, then
// applied by ApplyLifecycle on the corrected partition. Rules of every kind
// may be mixed in one set.
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

	// Partition rules by concern: split/merge re-partition, lifecycle kinds do
	// not (issue #26). Cycle detection and application only see split/merge.
	partitionRules := make([]CorrectionRule, 0, len(rules))
	for _, r := range rules {
		if r.Kind == CorrectionSplit || r.Kind == CorrectionMerge {
			partitionRules = append(partitionRules, r)
		}
	}

	// Canonical order by ID for determinism.
	sortedRules := make([]CorrectionRule, len(partitionRules))
	copy(sortedRules, partitionRules)
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
		work[i].Aliases = append([]StoryAlias(nil), g.Aliases...)
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
	// Status and Aliases are carried by applyRule and intentionally NOT
	// recomputed: they are stable lifecycle state, not membership hashes.
	for i := range work {
		work[i].ID = computeGroupID(work[i].Articles)
	}

	return work, nil
}

// ApplyLifecycle applies the lifecycle correction rules (archive, close,
// reactivate, alias; issue #26, AC-2) to a partition. It is intended to run
// after ApplyCorrections so that merge-induced representative changes (and
// therefore slug changes) are already settled when slug references resolve.
//
// Semantics:
//   - archive/close set the group's Status; reactivate sets Status to active.
//   - For a given slug, conflicting transition rules are rejected with an
//     error and the partition is left unmodified. A conflict is any two rules
//     that resolve to different target states for the same slug (e.g. archive
//   - reactivate, archive + close). archive + archive (same state) is not a
//     conflict (idempotent).
//   - alias rules append the referenced slug to the target group's Aliases;
//     an alias equal to the target's own representative slug is dropped
//     (representative-earlier-than-target case), and re-applying the same
//     alias rule is a deterministic no-op (idempotent).
//   - Applying the same rule set twice yields the same partition (idempotent).
func ApplyLifecycle(groups []StoryGroup, rules []CorrectionRule) ([]StoryGroup, error) {
	if len(rules) == 0 {
		return groups, nil
	}

	// Intrinsic validation first (malformed rules are always rejected).
	for _, r := range rules {
		if err := r.Validate(); err != nil {
			return nil, err
		}
		if r.Kind == CorrectionSplit || r.Kind == CorrectionMerge {
			continue // not a lifecycle rule; nothing to check here
		}
		if err := r.ValidateAgainstGroups(groups); err != nil {
			return nil, err
		}
	}

	// Collect lifecycle rules in canonical (ID-sorted) order for determinism.
	var lifecycleRules []CorrectionRule
	for _, r := range rules {
		if r.Kind == CorrectionSplit || r.Kind == CorrectionMerge {
			continue
		}
		lifecycleRules = append(lifecycleRules, r)
	}
	if len(lifecycleRules) == 0 {
		// No lifecycle rules in the set: nothing to apply. Return a copy of the
		// input (split/merge kinds were handled by ApplyCorrections). Short-circuit
		// BEFORE building the slug map so that partitions where a split produced
		// two groups sharing a representative slug do not spuriously error here.
		return groups, nil
	}
	sort.Slice(lifecycleRules, func(i, j int) bool { return lifecycleRules[i].ID < lifecycleRules[j].ID })

	// Resolve each group's representative slug. A slug that maps to multiple
	// groups is ambiguous: fail loudly only when a lifecycle rule actually
	// references that slug (plan risk #2). Slugs no rule references may be
	// duplicated (e.g. a split of a same-title group) without error.
	slugToIdxs := make(map[StorySlug][]int)
	for i, g := range groups {
		slug := groupRepSlug(g)
		if slug == "" {
			continue
		}
		slugToIdxs[slug] = append(slugToIdxs[slug], i)
	}
	// For slugs with exactly one group, a single-index lookup map.
	slugToIdx := make(map[StorySlug]int)
	for slug, idxs := range slugToIdxs {
		if len(idxs) == 1 {
			slugToIdx[slug] = idxs[0]
		}
	}
	// The set of slugs referenced by any lifecycle rule (for ambiguity check).
	// StoryID carries the slug for lifecycle kinds; IntoStoryID carries the alias
	// slug for alias rules (not a group reference).
	referencedSlugs := make(map[StorySlug]bool)
	for _, rule := range lifecycleRules {
		if rule.Kind == CorrectionArchive || rule.Kind == CorrectionClose || rule.Kind == CorrectionReactivate {
			referencedSlugs[StorySlug(rule.StoryID)] = true
		}
		if rule.Kind == CorrectionAlias {
			referencedSlugs[StorySlug(rule.StoryID)] = true
		}
	}
	for slug, idxs := range slugToIdxs {
		if len(idxs) > 1 && referencedSlugs[slug] {
			return nil, fmt.Errorf("ambiguous story slug %q resolves to more than one group", slug)
		}
	}

	// Work on copies so the input partition is never mutated on error.
	work := make([]StoryGroup, len(groups))
	for i, g := range groups {
		work[i] = g
		work[i].Articles = append([]Article(nil), g.Articles...)
		work[i].Aliases = append([]StoryAlias(nil), g.Aliases...)
	}

	// Resolve transition intents per slug before mutating anything, so that
	// conflicting rules (e.g. archive + reactivate on the same slug) are
	// rejected atomically with the partition left unmodified.
	type intent struct {
		target StoryStatus
		ruleID string
	}
	intents := make(map[StorySlug]*intent)
	for _, rule := range lifecycleRules {
		if rule.Kind != CorrectionArchive && rule.Kind != CorrectionClose && rule.Kind != CorrectionReactivate {
			continue
		}
		slug := StorySlug(rule.StoryID)
		var target StoryStatus
		switch rule.Kind {
		case CorrectionArchive:
			target = StatusArchived
		case CorrectionClose:
			target = StatusClosed
		case CorrectionReactivate:
			target = StatusActive
		}
		if prev, ok := intents[slug]; ok && prev.target != target {
			return nil, fmt.Errorf("conflicting lifecycle rules for slug %q: %s (rule %s) vs %s (rule %s)",
				slug, prev.target, prev.ruleID, target, rule.ID)
		}
		intents[slug] = &intent{target: target, ruleID: rule.ID}
	}

	now := time.Now().UTC().Format(time.RFC3339)

	for _, rule := range lifecycleRules {
		switch rule.Kind {
		case CorrectionArchive, CorrectionClose, CorrectionReactivate:
			idx, ok := slugToIdx[StorySlug(rule.StoryID)]
			if !ok {
				// ValidateAgainstGroups already checked; defensive no-op.
				continue
			}
			work[idx].Status = intents[StorySlug(rule.StoryID)].target
		case CorrectionAlias:
			idx, ok := slugToIdx[StorySlug(rule.StoryID)]
			if !ok {
				continue
			}
			aliasSlug := StorySlug(rule.IntoStoryID)
			// Dedupe: an alias equal to the target's own representative slug is
			// dropped (representative-earlier-than-target case), and an alias
			// already present (e.g. merge-recorded) is a deterministic no-op.
			if aliasSlug == groupRepSlug(work[idx]) {
				continue
			}
			found := false
			for _, a := range work[idx].Aliases {
				if a.Slug == aliasSlug {
					found = true
					break
				}
			}
			if found {
				continue // idempotent re-apply
			}
			work[idx].Aliases = append(work[idx].Aliases, StoryAlias{Slug: aliasSlug, Headline: "", MergedAt: now})
		}
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
		// First locate the target index and collect the source group (articles,
		// existing aliases, and representative slug for aliasing).
		targetIdx := -1
		sourceIdx := -1
		var sourceArticles []Article
		for i := range *groups {
			if (*groups)[i].ID == rule.IntoStoryID {
				targetIdx = i
			}
			if (*groups)[i].ID == rule.StoryID {
				sourceIdx = i
				sourceArticles = (*groups)[i].Articles
			}
		}
		if targetIdx == -1 || sourceIdx == -1 {
			return // ValidateAgainstGroups would have caught this; defensive no-op
		}
		target := (*groups)[targetIdx]
		source := (*groups)[sourceIdx]
		// Representative slug of the merged-away source becomes an alias on the
		// target (issue #26, AC-2). Dedupe: an alias equal to the target's own
		// representative slug is dropped (representative-earlier-than-target
		// case, where the merged page takes over the source's URL).
		mergedNow := time.Now().UTC().Format(time.RFC3339)
		targetSlug := groupRepSlug(target)
		sourceSlug := groupRepSlug(source)
		newAliases := make([]StoryAlias, 0, len(target.Aliases)+len(source.Aliases)+1)
		aliasSeen := make(map[StorySlug]bool)
		if sourceSlug != "" && sourceSlug != targetSlug {
			newAliases = append(newAliases, StoryAlias{Slug: sourceSlug, Headline: source.Headline, MergedAt: mergedNow})
			aliasSeen[sourceSlug] = true
		}
		// Carry the target's existing aliases first (stable order), then the
		// source's (in case the source was itself a merge target), deduping.
		for _, a := range target.Aliases {
			if aliasSeen[a.Slug] {
				continue
			}
			aliasSeen[a.Slug] = true
			newAliases = append(newAliases, a)
		}
		for _, a := range source.Aliases {
			if aliasSeen[a.Slug] {
				continue
			}
			if a.Slug == targetSlug {
				continue
			}
			aliasSeen[a.Slug] = true
			newAliases = append(newAliases, a)
		}
		// Rebuild without the source group, appending source articles to the target.
		// The merged group keeps the target's Status (the surviving identity)
		// and the combined alias list.
		var result []StoryGroup
		for i := range *groups {
			if i == sourceIdx {
				continue // drop the source group
			}
			g := (*groups)[i]
			g.Articles = append([]Article(nil), g.Articles...)
			if i == targetIdx {
				g.Articles = append(g.Articles, sourceArticles...)
				g.Aliases = newAliases
				// Recompute First/LastPublishedAt to cover the merged articles
				// (the target's existing window may not include the source).
				for _, a := range sourceArticles {
					pt := a.ParsePublishedAtOrZero()
					if pt.IsZero() {
						continue
					}
					if g.FirstPublishedAt.IsZero() || pt.Before(g.FirstPublishedAt) {
						g.FirstPublishedAt = pt
					}
					if pt.After(g.LastPublishedAt) {
						g.LastPublishedAt = pt
					}
				}
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
