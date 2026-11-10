# huddlescope.github.io
HuddleScope — independent NFL news discovery and story aggregation (Go + Turso + Hugo).

## Purpose
This is a non-commercial, independent news aggregator for NFL-related content. The project's goal is to provide a clean, modular codebase for ingesting news feeds, storing stories, and exporting to Hugo-based static sites.

## Architecture
The application follows a layered architecture with clear separation of concerns:

- **cmd/** — CLI entry points (ingest, export)
- **internal/application/** — Business logic and orchestration
- **internal/domain/** — Core domain models and interfaces (Story, Source, FeedEntry)
- **internal/adapters/** — Pluggable implementations (RSS fetcher, storage backends)

The Go engine is independent of hosting, database, and rendering adapters. Adapters implement domain interfaces (Repository, FeedFetcher) to enable swapping storage or feed sources without affecting core logic.

## Repository Layout

| Path | Description |
|------|-------------|
| `cmd/ingest` | CLI to fetch RSS feeds and store stories |
| `cmd/export` | CLI to export stories to the configured backend |
| `internal/domain` | Core domain models and interfaces |
| `internal/application` | Application orchestration and business logic |
| `internal/adapters/rss` | RSS feed fetching adapter |
| `internal/adapters/storage` | Storage adapters (in-memory, Turso, file) |
| `internal/config` | Configuration loading and validation |
| `web/` | Hugo site source (generated) |
| `scheduler/` | Scheduled tasks (TBD) |
| `migrations/` | Database migrations (TBD) |
| `docs/` | Project documentation |

## Development

### Prerequisites
- Go 1.24.11+
- Make (optional; used for convenience targets)

### Clone and setup
```sh
git clone https://github.com/HuddleScope/huddlescope.github.io.git
cd huddlescope.github.io
cp .env.example .env
# Edit .env with your local values (optional; defaults will be used if omitted)
```

### Build
```sh
make build
# Binaries appear in dist/ingest and dist/export
```

### Run checks (CI pipeline)
```sh
make check
# Runs fmt, lint, test, coverage, security in sequence
```

### Available Make targets
- `make help` — Show all available targets
- `make fmt` — Format Go files with gofmt
- `make lint` — Run golangci-lint
- `make test` — Run vet and tests with race detector
- `make coverage` — Run tests with coverage report
- `make security` — Run gosec security scanner
- `make build` — Build binaries to dist/
- `make clean` — Remove build artifacts

### Run ingest (fetch feeds)
```sh
./dist/ingest
# or with config:
./dist/ingest -config .env
```

### Run export (process stories)
```sh
./dist/export
# or with config:
./dist/export -config .env
```

## Configuration

### Environment Variables

| Variable | Default | Production | Description |
|----------|---------|------------|-------------|
| `HSC_LOG_LEVEL` | `info` | No | Log level: debug, info, warn, error |
| `HSC_USER_AGENT` | `huddlescope/0.1` | No | User agent string for HTTP requests |
| `HSC_REQUEST_TIMEOUT` | `30s` | No | Request timeout (e.g., `60s`) |
| `HSC_HUGO_BASE_URL` | `https://huddlescope.github.io/` | No | Base URL for Hugo exports |
| `HSC_STORAGE_BACKEND` | `memory` | Yes | Storage backend: memory, turso, sqlite-file |
| `HSC_SOURCES` | (none) | No | Source list: `id,name,url[,interval];...` |
| `HSC_ENV` | `dev` | Yes | Environment: dev, production |
| `TURSO_DATABASE_URL` | (none) | Yes | Turso database URL |
| `TURSO_AUTH_TOKEN` | (none) | Yes | Turso authentication token |

### Source Configuration

Sources define RSS feed URLs with optional fetch intervals and registry metadata:

```bash
# Single source with 5-minute interval
HSC_SOURCES=nfl-news,NFL News,http://example.com,300

# Multiple sources with registry fields
HSC_SOURCES=nfl-news,NFL News,http://a.com,300,1,true,ESPN,en,60,RSS syndication permitted,allowed
```

Format: `id,name,url[,interval][,priority][,enabled][,publication][,language][,timeout][,license][,accept-use];...`

- `id`: Unique identifier (used for deduplication and per-source overrides)
- `name`: Human-readable name
- `url`: HTTP/HTTPS feed URL
- `interval`: Optional fetch interval in seconds (default: 900 = 15 minutes)
- `priority`: Lower number polled first; tiebreak by ID (default: 0)
- `enabled`: true to poll, false to skip (default: true)
- `publication`: Publisher name for display (optional)
- `language`: BCP-47 language tag (e.g., en, es) (default: en)
- `timeout`: Per-source HTTP timeout in seconds (default: 0 = global)
- `license`: License or acceptable-use notes (optional)
- `accept-use`: allowed/restricted/unknown (default: unknown)

See `internal/feeds/README.md` for the curated list of NFL feeds and instructions on how to add/remove feeds without code changes.

### Runtime Status

The application tracks runtime polling status for each source:
- Last attempt timestamp
- Success/failure flag
- Error details (if failed)

Status is recorded in memory and logged. For persistent tracking, check application logs or integrate with monitoring.

### Feed Registry

HuddleScope includes a curated list of NFL RSS/Atom feeds in `internal/feeds/`. The registry supports:

- **Per-source failure isolation**: One failing source never blocks others
- **Priority-based ordering**: Sources polled in priority order
- **Selective disabling**: Set `enabled=false` to skip a source without deleting it
- **Acceptable-use policy**: Mark restricted/paywalled feeds appropriately

See `internal/feeds/README.md` for how to add or remove feeds without code changes.

### Production Mode

Set `HSC_ENV=production` to enable production mode. Production requires:
- `HSC_STORAGE_BACKEND` must be `turso` or `sqlite-file` (not `memory`)
- `TURSO_DATABASE_URL` must be set
- `TURSO_AUTH_TOKEN` must be set

Invalid production configuration causes the application to fail with a clear error message listing all missing/invalid fields.

### Secrets Handling

- Tokens and sensitive data are never logged
- The `-check` mode prints a redacted configuration (secrets replaced with `***`)
- JSON serialization of the config never includes secrets
- The `.env` file is git-ignored (see `.gitignore`)

## Legal
- This project does not commit full copyrighted articles.
- No unlicensed images are stored or distributed.
- All data comes from publicly available RSS feeds; users are responsible for complying with feed source terms of service.