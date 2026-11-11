# Story Detail Implementation — HS-016

## Overview

HS-016 implements a stable canonical page per story that aggregates reporting from multiple sources, using **metadata only** (no full article bodies or unlicensed images).

## Key Design Decisions

### Stable URL (AC2)

The story page URL is derived from the **representative article's headline** (normalized), not from the clustering group ID. This ensures the URL remains stable even when additional sources are added to the same story.

**Algorithm:**
1. Pick the representative article (earliest `PublishedAt`, ties on smallest canonical URL)
2. Generate slug from normalized headline using `ComputeStorySlugFromHeadline`
3. Path: `/stories/{slug}/`

### Metadata-Only Rendering (AC3)

The rendering model (`PublisherArticle`) deliberately omits article body/content fields. This ensures pages never republish full copyrighted articles or unlicensed images.

### Graceful Handling (AC4)

- Articles without valid URLs are silently omitted
- Empty groups or groups where all articles lack URLs return an error
- Build/publish failures for one story don't block others

## Files Added

| Path | Purpose |
|------|---------|
| `internal/domain/storypage.go` | Domain model for story detail page, slug generation, resolver port |
| `internal/domain/storypage_test.go` | Tests for AC1-AC4, slug stability, idempotency |
| `internal/adapters/hugo/publisher.go` | Concrete `StoryPagePublisher` writing Hugo content files |
| `internal/adapters/hugo/publisher_test.go` | Tests for Hugo rendering, idempotency, error handling |
| `internal/application/storypages.go` | `RunStoryPages` orchestration |
| `internal/application/storypages_test.go` | Tests for pipeline execution, failure isolation |
| `web/content/stories/top-free-agents-2024.md` | Sample multi-source story page |
| `web/archetypes/story.md` | Updated archetype with story detail fields |
| `web/layouts/_default/single.html` | Template with multi-source and single-source branches |
| `web/layouts/partials/head.html` | Canonical link from front-matter or permalink |
| `web/assets/css/main.css` | Styles for `.content-sources-list`, `.content-source-item`, `.content-timeline` |
| `scripts/site-test.sh` | Added HS-016 verification |

## AC Mapping

| AC | Implementation |
|----|----------------|
| 1. Lists all publisher articles, source names, original titles, publication times and outgoing URLs | `PublisherArticle` with 5 fields; template renders all |
| 2. Story URLs remain stable when additional sources arrive | Slug derived from representative headline, not group ID |
| 3. Pages never republish full articles or unlicensed photographs | `PublisherArticle` struct omits content/image fields |
| 4. Handle deleted/updated source items gracefully | Skip articles without URL; empty groups error |

## Deployment Readiness (HS-016 vs HS-017)

**HS-016 (this PR):** Page model, rendering, template, tests. Engine independent of hosting/database/rendering adapters.

**HS-017 (successor):** Export Turso data to versioned Hugo JSON + automated static-site build/deploy.

## Testing

- All domain tests pass with ≥70% coverage (71.9% application, 93.2% hugo adapter)
- Site build verification: success, failure, idempotency paths
- Hugo render produces stable output across builds