# Implementation: Issue #201 — [UI-04] Preserve original headline casing in display titles

Implemented `autopilot/issue-201` per the saved plan (plan §1–§7). Display
titles (original feed casing) are now carried separately from identity titles
(normalized/lowercased) across the domain, storage, snapshot, and Hugo layers.
The identity primitives (`ComputeStoryID`, `ComputeStorySlugFromHeadline`,
`ComputeStoryPageKey`, `DedupeKey`, `computeGroupID`, `TokenizeTitle`-based
clustering) are unchanged and still read the normalized `Title`/`Headline`.

A new `DisplayTitle` (articles/stories) and `DisplayHeadline` (story groups)
is threaded from feed ingestion → domain → persistence → snapshot JSON → Hugo
story pages. Every display read falls back to the normalized `Title` when
`DisplayTitle` is empty, so pre-change rows render exactly as today. Display
normalization is `trim + collapse whitespace + NFC` — case and punctuation are
preserved (AC-1). The `NormalizeTitle` function is byte-for-byte unchanged (its
lowercase step is identity-critical).

## What was built

### Domain (`internal/domain/`)

- **`normalize.go`** — new `DisplayNormalizeTitle(title string) string`:
  `""` → `""`; `TrimSpace` → collapse whitespace → `NormalizeUnicode` (NFC).
  **No lowercasing.** `NormalizeTitle` is untouched.
- **`story.go`** — `Story` gains `DisplayTitle string` (JSON tag
  `displayTitle` in `MarshalJSON`/`UnmarshalJSON`; old payloads unmarshal to
  `""`). New helper `func (s Story) Display() string` returns `DisplayTitle`
  when non-empty after trim, else `Title` — the single fallback point every
  display surface uses. `MeaningfulFieldsChanged` now compares the
  fallback-aware `Display()` values, so a display-only re-ingestion (restoring
  original casing) counts as an update (AC-3) while `""` vs same-as-`Title`
  stays unchanged (storage-column/JSON hydration stability).
- **`source.go`** — `Story.Canonicalize` sets `DisplayTitle:
  DisplayNormalizeTitle(entry.Title)` alongside the normalized `Title`.
  Identity fields (`ID`, `DedupeKey`, slug inputs) untouched.
- **`clustering.go`** — `Article` gains `DisplayTitle string`. `NewArticle`
  sets it from `DisplayNormalizeTitle(feedEntry.Title)`. `StoryGroup` gains
  `DisplayHeadline string`; `NewStoryGroup`/`AddArticle` set it from the
  earliest article's `DisplayTitle` (fallback `Title`). `Headline` semantics
  unchanged (normalized — still feeds slugs, group ID, scoring, summary
  fallback). `NewStoryFromGroup` sets `Story.DisplayTitle =
  group.DisplayHeadline`.
- **`storypage.go`** — `BuildStoryDetailPage`: `Headline` = group display
  headline (fallback `group.Headline`); each `PublisherArticle.Title` =
  `art.Display()`. **`Slug` and `PageKey` remain computed from the normalized
  `rep.Title`** (AC-2).
- **`snapshot.go`** — `buildStories`: `SnapshotStory.Title` uses the display
  headline (group `DisplayHeadline`-or-`Headline` fallback);
  `snapshotStorySummary` fallback uses the same display headline.
  `buildLatestArticles`: `SnapshotArticle.Title` = `s.Display()`. `Validate`,
  `Render`, `ContentEqual`, sort orders unchanged.

### Storage (`internal/adapters/storage/sqlite/`)

- **`migrations/010_display_title.sql`** (new) — forward-only, additive:
  `ALTER TABLE articles ADD COLUMN display_title TEXT NOT NULL DEFAULT '';`
  `UPDATE articles SET display_title = COALESCE(title, '');`
  `ALTER TABLE stories ADD COLUMN display_title TEXT NOT NULL DEFAULT '';`
  `UPDATE stories SET display_title = COALESCE(headline, COALESCE(title, ''));`
  Header documents the normalized-fallback backfill (AC-3), Turso/libSQL
  compatibility, and the schema_migrations guard (v10) preventing re-run.
- **`repository.go`** —
  - `Upsert`: the story INSERT, the found-by-story_key UPDATE, and the
    dedupe-hit UPDATE all persist `display_title` (bind `storyDisplayTitle`,
    which coalesces `story.Title` when `DisplayTitle` is empty). The
    found-by-story_key branch now also reads the existing `display_title`
    column + `story_data` `displayTitle` key into `otherStory.DisplayTitle`
    (via new helper `storyDataDisplayTitle`) so unchanged-detection is stable
    across column/JSON hydration.
  - `upsertArticles` / `UpsertArticle`: UPDATE/INSERT include
    `display_title` (coalesce `article.Title` when empty).
  - `List` / `ListWithCtx`: article hydration SELECT adds `a.display_title`;
    when empty, `DisplayTitle = article.Title` (legacy fallback). Story rows
    hydrate `DisplayTitle` from the `display_title` column when the
    `story_data` JSON key is absent (legacy rows render as today, AC-3).
- **`turso/repository.go`** — no change: it delegates to the `sqlite`
  package, so the shared SQL covers both drivers.

### Application layer (`internal/application/`)

- `app.go` (`RunGrouping`, `runGroupAndScorePipeline`, `RebuildRanking`, and
  the `RunStoryPages` story→article loop), `snapshot.go`
  (`storiesToArticles`), and `storypages.go` (`RunStoryPages`) — all
  `Story`→`Article` conversion sites add `DisplayTitle: story.DisplayTitle`.

## Tests added (targeted regression coverage)

- `internal/domain/normalize_test.go` — `DisplayNormalizeTitle` table tests:
  mixed case kept, leading/trailing spaces trimmed, runs collapsed, tabs/
  newlines collapsed, NFC composition (pre-composed and decomposed inputs),
  empty→empty, punctuation preserved; `DisplayNormalizeTitleKeepsCase`
  (display differs from normalized); `DisplayNormalizeTitleIdempotent`.
- `internal/domain/source_test.go` — `TestCanonicalize_DisplayTitle`
  (Title lowercased, DisplayTitle original casing, `Display()` prefers it,
  `ComputeStoryID`/`DedupeKey` unchanged, lowercased entry → same ID);
  `TestStory_Display_Fallback`; `MeaningfulFieldsChanged` display-title
  cases (changed → true; empty-vs-empty → false).
- `internal/domain/clustering_test.go` — `NewArticle` sets DisplayTitle;
  `Article.Display()` fallback; `NewStoryGroup`/`NewStoryFromGroup` propagate
  display headline; `computeGroupID` invariant under display casing;
  `Cluster` group count/ID/Headline identical for mixed-case vs all-lowercase
  inputs (display headline differs); `TokenizeTitle` lowercases internally.
- `internal/domain/story_test.go` — JSON round-trip with a set DisplayTitle
  (key present, value preserved); legacy payload without `displayTitle` key
  (required `publishedAt`/`firstSeenAt`/`updatedAt` included) decodes to
  empty DisplayTitle → `Display()` falls back to Title; empty DisplayTitle
  round-trips to empty.
- `internal/domain/snapshot_test.go` — `TestBuildSnapshot_DisplayTitle`
  (story + latest article titles carry display casing, Render contains the
  display headline); `TestBuildSnapshot_DisplayTitleFallback` (empty
  DisplayTitle → normalized fallback, `Validate()` passes).
- `internal/domain/storypage_test.go` — `BuildStoryDetailPage` Headline
  carries display casing while Slug is normalized; display-heading fallback
  when no DisplayHeadline; slug/page-key invariant under display casing.
- `internal/adapters/storage/sqlite/repository_test.go` — article
  `display_title` round-trip (column + normalized title); story
  `display_title` round-trip (List hydration + unchanged re-upsert); legacy
  story fallback (cleared `display_title` → `Display()` falls back to Title;
  re-ingest with casing restored → `UpsertUpdated`); migration 010 backfill
  on a v9 DB (story at schema 9 → 010 backfills `display_title` from
  headline). Existing migration-count and schema-version assertions updated
  from 9 → 10 across `migrations_test.go`, `repository_test.go`,
  `ingest_lease_migration_test.go`, `ingest_runs_migration_test.go`,
  `retentiondb_test.go`, `retentionpkg_test.go`, `turso_test.go`, and the
  storage/turso retention tests.
- `internal/adapters/hugo/publisher_test.go` — `RenderMarkdown` preserves
  display casing + punctuation in front-matter `title:`/`description:`, body
  `# Headline`, and article `title:`; slug stays normalized.
- `internal/application/snapshot_test.go` — end-to-end: story upserted with a
  display title flows through `RunSnapshot` into the snapshot story title and
  latest article title; `Validate()` passes.
- `internal/application/storypages_test.go` — end-to-end: story upserted with
  a display title flows through `RunStoryPages` into the rendered page's
  front-matter title (display casing) while the slug file name stays the
  normalized form.

## Verification

- `make test` — **passed** (`go vet ./...` + `go test ./... -race`; all
  packages green).
- `gofmt -l` (repo-wide gate) — clean (0 files unformatted).
- `make lint` — **0 issues** (golangci-lint).
- `make security` — **passed** (gosec 0 issues; govulncheck no
  vulnerabilities).
- `make check-coverage` — **passed** (core `internal/domain` 85.4%,
  `internal/application` 85.6%; all other internal packages at/above the
  ≥70% threshold).
- `make build` — **passed** (`dist/huddlescope` built).
- `scripts/site-test.sh` — **all site tests passed** (fixture overlay,
  success/failure/idempotency paths, HS-015/HS-023 assertions).
- `scheduler` npm tests — **23/23 passed** (independent CI job).

## Key files

- `internal/domain/normalize.go` — `DisplayNormalizeTitle` (no lowercasing).
- `internal/domain/story.go` — `Story.DisplayTitle` (+JSON), `Display()`
  helper, `MeaningfulFieldsChanged` display-aware comparison.
- `internal/domain/source.go` — `Canonicalize` sets DisplayTitle.
- `internal/domain/clustering.go` — `Article.DisplayTitle`,
  `StoryGroup.DisplayHeadline`, `NewArticle`/`NewStoryGroup`/
  `NewStoryFromGroup`.
- `internal/domain/storypage.go` — display Headline, normalized Slug/PageKey.
- `internal/domain/snapshot.go` — display story/article titles, summary
  fallback.
- `internal/adapters/storage/sqlite/migrations/010_display_title.sql` (new)
  — additive `display_title` columns + normalized-fallback backfill.
- `internal/adapters/storage/sqlite/repository.go` — upsert/list display_title
  persistence + hydration + unchanged-detection stability.
- `internal/application/{app,snapshot,storypages}.go` — Story→Article
  `DisplayTitle` mapping.
- Test files listed above (targeted regression coverage).

## Deviations from the plan

- **`MeaningfulFieldsChanged` compares `Display()` (fallback-aware) rather
  than raw `DisplayTitle`.** The plan (§3 source.go) specified a raw
  `DisplayTitle` comparison. In practice the storage layer's
  found-by-story_key branch hydrates `otherStory.DisplayTitle` from the
  `display_title` column (which `Upsert` coalesces to `Title` when
  `DisplayTitle` is empty) while the incoming story keeps `DisplayTitle`
  `""` — a raw comparison would falsely flag every re-upsert as `Updated`.
  Comparing the fallback-aware `Display()` values makes unchanged-detection
  stable (empty DisplayTitle ≡ DisplayTitle-set-to-Title) while still
  detecting genuine display-casing restorations (AC-3). This is a
  behavior-preserving refinement of the plan's intent, not a scope change.
- **`Story.MarshalJSON` includes `displayTitle` unconditionally** (no
  `omitempty`), consistent with the other always-present fields (`title`,
  `summary`, `url`, …). Legacy hydration (column fallback) still covers rows
  whose `story_data` JSON predates the key.
