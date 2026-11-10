# Feed Registry

This package provides a curated list of NFL RSS/Atom feed sources for HuddleScope.

## Adding or Removing Feeds

No code changes are needed beyond editing `conf/sources.yaml`. The `sources:` list defines all sources.

### To Add a Feed

1. Find a public RSS or Atom feed URL for the NFL content (news, analysis, scores, etc.).
2. Check if the publisher allows syndication or RSS scraping (check their terms of service).
3. Add a new entry to `sources:` in `conf/sources.yaml` with appropriate values:
   - `id`: Unique stable identifier (e.g., `espn-nfl`, `cbs-nfl`)
   - `name`: Display name for the source
   - `url`: Full RSS/Atom URL (must start with `http://` or `https://`)
   - `category`: Type of source (draft, national, aggregator, all-sports, team-official)
   - `enabled`: `true` to poll, `false` to skip
   - `verification`: How the feed was verified
   - `publication`: Publisher name for display (optional)
   - `priority`: Lower number polled first; tiebreak by ID
   - `team`: NFL team abbreviation for team-specific feeds (optional)
   - `language`: BCP-47 tag (e.g., `en`, `es`)
   - `timeout`: HTTP timeout in seconds (0 uses global default)
   - `license`: License or acceptable-use notes (free text)
   - `acceptable_use`: `allowed` (syndication OK) or `restricted` (no paywall bypass)

4. Ensure the ID is unique among all sources.

### To Remove a Feed

1. Delete the entry corresponding to the feed from `sources:` in `conf/sources.yaml`.

2. Run tests to verify no code depends on the removed source.

### Best Practices

- Prioritize public, well-known NFL feeds.
- Prefer feeds that explicitly permit RSS syndication.
- Mark paywalled or restricted feeds as `enabled: false`.
- Do not hardcode credentials.
- Use descriptive ID names (snake_case, include domain if needed).
- Update this README if you add a pattern or convention.

## Seeded Sources

See `conf/sources.yaml` for the complete list of NFL sources. Key sources include:

- `espn-nfl`: ESPN NFL News (priority 1, enabled)
- `espn-fantasy`: ESPN Fantasy Football (priority 2, enabled)
- `cbs-nfl`: CBS Sports NFL (priority 1, enabled)
- `fox-nfl`: FOX Sports NFL (priority 2, enabled)
- `nfl-com`: NFL.com News (priority 1, enabled)
- `nfl-com-boxscore`: NFL.com Boxscore (priority 3, enabled)
- `bleacher-report-nfl`: Bleacher Report NFL (priority 2, enabled)
- `sports-illustrated`: Sports Illustrated NFL (priority 3, enabled)
- `usa-today-nfl`: USA Today Sports NFL (priority 3, enabled)
- `the-score`: The Score NFL (priority 4, enabled)
- `the-athletic-nfl`: The Athletic NFL (priority 5, disabled due to paywall)
- `reddit-r-nfl`: Reddit r/nfl (priority 6, enabled)

## Acceptable-Use Policy

- **Allowed**: Syndication via RSS/Atom is permitted. Publicly available content may be cached and displayed.
- **Restricted**: Paywalled or subscription-required content. Setting `enabled: false` ensures the feed is never fetched (preventing access attempts).

We honor publisher terms of service and never attempt to bypass access controls.

## Implementation Details

- `Load()` returns a pre-validated `FeedRegistry` populated from `conf/sources.yaml`.
- Falls back to built-in seeds if YAML file is not found.
- `All()` returns all sources sorted by priority (ascending), then ID.
- `Enabled()` returns only enabled sources, sorted by priority then ID.
- `RecordStatus(id, ok, err)` records polling outcomes per-source.
- `Status(id)` returns runtime polling status (last attempt time, error, success flag).

## See Also

- `internal/domain/source.go`: `FeedSource`, `FeedRegistry`, `PollStatus` types
- `internal/application/app.go`: Registry-driven ingestion with per-source failure isolation