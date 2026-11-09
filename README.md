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

## Legal
- This project does not commit full copyrighted articles.
- No unlicensed images are stored or distributed.
- All data comes from publicly available RSS feeds; users are responsible for complying with feed source terms of service.