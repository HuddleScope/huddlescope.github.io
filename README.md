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
| `web/` | Hugo site source (see Website section below) |
| `scheduler/` | Cloudflare Worker cron dispatcher (HS-019) |
| `migrations/` | Database migrations (TBD) |
| `docs/` | Project documentation |

## Development

### Prerequisites
- Go 1.24.11+
- Node.js 20+ (for scheduler/ worker)
- Make (optional; used for convenience targets)

### Clone and setup
```sh
git clone https://github.com/HuddleScope/huddlescope.github.io.git
cd huddlescope.github.io
cp .env.example .env
# Edit .env with your local values (optional; defaults will be used if omitted)
```

### Build Go binaries
```sh
make build
# Binaries appear in dist/ingest and dist/export
```

### Build scheduler worker
```sh
cd scheduler
npm install
# Local dev: npm run dev
# Deploy: wrangler deploy
```

### Run checks (CI pipeline)
```sh
make check
# Runs fmt, lint, test, coverage, security in sequence
```

### Run scheduler tests
```sh
cd scheduler
npm test
```

### Available Make targets

**Go targets**
- `make help` — Show all available targets
- `make fmt` — Format Go files with gofmt
- `make lint` — Run golangci-lint
- `make test` — Run vet and tests with race detector
- `make coverage` — Run tests with coverage report
- `make security` — Run gosec security scanner
- `make build` — Build binaries to dist/
- `make clean` — Remove build artifacts
- `make check` — Run fmt, lint, test, coverage, security

**Site targets**
- `make site` — Build Hugo site to web/public/
- `make site-test` — Run site verification (success/failure/idempotency)
- `make site-watch` — Run Hugo server locally

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

## Website (Hugo)

The site is built with Hugo and hosted as a static site.

### Prerequisites
- Hugo 0.169.0+ (extended build recommended)

### Build
```sh
make site
# Site builds to web/public/
```

### Verify (CI tests: success/failure/idempotency)
```sh
make site-test
```

### Local development
```sh
make site-watch
# Runs Hugo server on http://localhost:1313
```

### Available site Make targets
- `make site` — Build site to web/public/
- `make site-test` — Run site verification tests
- `make site-watch` — Run Hugo server in watch mode

### Content structure
- `web/content/_index.md` — Homepage
- `web/content/stories/*.md` — Story pages
- `web/content/sources/*.md` — Source pages
- `web/content/about.md` — About page

### Customization
- Edit `web/layouts/` for template changes
- Edit `web/assets/css/main.css` for styles
- Edit `web/static/js/main.js` for JS enhancements (minimal, no tracking)

### Deployment

The built site in `web/public/` can be deployed to GitHub Pages, Netlify, Vercel, or any static host.

#### GitHub Pages (automatic via workflow)

This repository uses a GitHub Actions workflow (`.github/workflows/refresh-news.yml`) to refresh news and deploy to GitHub Pages.

- **Manual trigger**: Open the repository Actions tab and select **Run workflow** on the Refresh news workflow.
- **Inputs**:
  - `publish` (default `true`): Set to `false` to build the site without deploying.
- **Secrets required**: `TURSO_DATABASE_URL` and `TURSO_AUTH_TOKEN` must be configured in repository settings for production mode.
- **Environment**: The workflow uses the `github-pages` environment and deploys via GitHub Pages with `pages: write` and `id-token: write` permissions.
- **Safety**: If the export pipeline produces an empty snapshot (no stories), the workflow restores the last-committed homepage.json and warns instead of publishing an empty site.

See issue #17 for Turso→Hugo JSON export and issue #18 for the refresh workflow.

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

Sources define RSS feed URLs with optional fetch intervals and registry metadata.

#### YAML Configuration (Recommended)

Sources are configured in a YAML file (default: `conf/sources.yaml`).

```yaml
sources:
  - id: espn-nfl
    name: ESPN — NFL
    url: https://www.espn.com/espn/rss/nfl/news
    category: national
    enabled: true
    verification: confirmed_xml
    publication: ESPN NFL
    priority: 1
    language: en
    license: RSS syndication permitted
    acceptable_use: allowed
```

Available fields:
- `id`: Unique identifier (snake_case)
- `name`: Human-readable name
- `url`: HTTP/HTTPS feed URL
- `category`: Type of source (draft, national, aggregator, all-sports, team-official)
- `enabled`: true to poll, false to skip
- `verification`: How the feed was verified
- `publication`: Publisher name for display (optional)
- `priority`: Lower number polled first; tiebreak by ID
- `team`: NFL team abbreviation (for team feeds, optional)
- `language`: BCP-47 language tag (default: en)
- `timeout`: HTTP timeout in seconds (default: 0 = global)
- `license`: License or acceptable-use notes (optional)
- `acceptable_use`: allowed/restricted/unknown (default: unknown)

See `conf/sources.yaml` for the complete list of NFL sources.

#### Environment Variable Configuration (Legacy)

```bash
# Single source with 5-minute interval
HSC_SOURCES=nfl-news,NFL News,http://example.com,300

# Multiple sources with registry fields
HSC_SOURCES=nfl-news,NFL News,http://a.com,300,1,true,ESPN,en,60,RSS syndication permitted,allowed
```

Format: `id,name,url[,interval][,priority][,enabled][,publication][,language][,timeout][,license][,accept-use];...`

Environment variables take precedence over YAML configuration.

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

### Content
- This project does not commit full copyrighted articles.
- No unlicensed images are stored or distributed.
- All data comes from publicly available RSS feeds; users are responsible for complying with feed source terms of service.

### Privacy
- No external tracking scripts (Google Analytics, Facebook Pixel, etc.)
- No cookies or persistent storage beyond browser caching
- No user profiling or behavioral tracking

See `web/content/about.md` for more details.