# Feed Registry

This package provides a curated list of NFL RSS/Atom feed sources for HuddleScope.

## Adding or Removing Feeds

No code changes are needed beyond editing `feeds.go`. The `SeededSources` slice defines all sources.

### To Add a Feed

1. Find a public RSS or Atom feed URL for the NFL content (news, analysis, scores, etc.).
2. Check if the publisher allows syndication or RSS scraping (check their terms of service).
3. Add a new entry to `SeededSources` with appropriate values:
   - `ID`: Unique stable identifier (e.g., `espn-nfl`, `cbs-nfl`)
   - `URL`: Full RSS/Atom URL (must start with `http://` or `https://`)
   - `Publication`: Publisher name for display
   - `Priority`: Lower number polled first; positive integer
   - `Enabled`: `true` to poll, `false` to skip (default for restricted feeds)
   - `Language`: BCP-47 tag (e.g., `en`, `es`)
   - `Timeout`: HTTP timeout in seconds (0 uses global default)
   - `License`: License or acceptable-use notes (free text)
   - `AcceptableUse`: `allowed` (syndication OK) or `restricted` (no paywall bypass)

4. Ensure the ID is unique among all sources.

### To Remove a Feed

1. Delete the line corresponding to the feed from `SeededSources`.

2. Run tests to verify no code depends on the removed source.

### Best Practices

- Prioritize public, well-known NFL feeds.
- Prefer feeds that explicitly permit RSS syndication.
- Mark paywalled or restricted feeds as `Enabled: false`.
- Do not hardcode credentials.
- Use descriptive ID names (snake_case, include domain if needed).
- Update this README if you add a pattern or convention.

## Seeded Sources

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
- `the-athletic-nfl`: The Athletic NFL (priority 5, **disabled** due to paywall)
- `reddit-r-nfl`: Reddit r/nfl (priority 6, enabled)

## Acceptable-Use Policy

- **Allowed**: Syndication via RSS/Atom is permitted. Publicly available content may be cached and displayed.
- **Restricted**: Paywalled or subscription-required content. Setting `Enabled: false` ensures the feed is never fetched (preventing access attempts).

We honor publisher terms of service and never attempt to bypass access controls.

## Implementation Details

- `Load()` returns a pre-validated `FeedRegistry`.
- `All()` returns all sources sorted by priority (ascending), then ID.
- `Enabled()` returns only enabled sources, sorted by priority then ID.
- `RecordStatus(id, ok, err)` records polling outcomes per-source.
- `Status(id)` returns runtime polling status (last attempt time, error, success flag).

## See Also

- `internal/domain/source.go`: `FeedSource`, `FeedRegistry`, `PollStatus` types
- `internal/application/app.go`: Registry-driven ingestion with per-source failure isolation