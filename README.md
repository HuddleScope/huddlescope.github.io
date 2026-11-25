# huddlescope.github.io
HuddleScope — independent NFL news discovery and story aggregation (Go + Turso + Hugo).

## Purpose
This is a non-commercial, independent news aggregator for NFL-related content. The project's goal is to provide a clean, modular codebase for ingesting news feeds, storing stories, and exporting to Hugo-based static sites.

## Architecture
The application follows a layered architecture with clear separation of concerns:

- **cmd/huddlescope/** — Unified CLI entry point (subcommands: `ingest run`, `export`, `heartbeat check`, `config`)
- **internal/application/** — Business logic and orchestration
- **internal/domain/** — Core domain models and interfaces (Story, Source, FeedEntry)
- **internal/adapters/** — Pluggable implementations (RSS fetcher, storage backends, Hugo publisher)
- **internal/config/** — Configuration loading and validation (sources, teams)

The Go engine is independent of hosting, database, and rendering adapters. Adapters implement domain interfaces (Repository, FeedFetcher) to enable swapping storage or feed sources without affecting core logic.

## Repository Layout

| Path | Description |
|------|-------------|
| `cmd/huddlescope` | Unified CLI: `ingest run` (fetch RSS feeds and store stories), `export` (export stories to Hugo data), `heartbeat check` (heartbeat health/staleness), `config` (validate config) |
| `internal/domain` | Core domain models and interfaces |
| `internal/application` | Application orchestration and business logic |
| `internal/adapters/rss` | RSS feed fetching adapter |
| `internal/adapters/storage` | Storage adapters (in-memory, SQLite file, Turso) |
| `internal/adapters/hugo` | Hugo data/snapshot publisher adapter |
| `internal/adapters/publish` | Story publishing adapter |
| `internal/config` | Configuration loading and validation |
| `web/` | Hugo site source (see Website section below) |
| `scheduler/` | Cloudflare Worker cron dispatcher (HS-019) |
| `internal/adapters/storage/sqlite/migrations` | SQLite schema migrations |
| `docs/` | Project documentation |

## Development

### Prerequisites
- Go 1.27.4+ (go.mod declares `go 1.27.4`)
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
# Unified binary appears in dist/huddlescope
```

The binary is built static (`CGO_ENABLED=0`) with `-trimpath` and stamped at
build time with the version, commit, and a deterministic build-time stamp, so
`huddlescope version` reports exactly where it came from:

```sh
$ ./dist/huddlescope version
huddlescope version 0.3.0
revision 8ccf026
built 2026-11-25T10:53:29Z
linux/amd64
```

The stamp is overridable (useful for reproducible builds and cross-compiles):

```sh
# Override any of these; defaults resolve from the repo (tag -> git describe
# -> sha-<short>) and the HEAD commit timestamp (honoring SOURCE_DATE_EPOCH).
make build VERSION=0.3.0 COMMIT=8ccf026 BUILT_AT=2026-11-25T00:00:00Z GOOS=linux GOARCH=arm64

# Build the full release matrix (linux amd64/arm64/armv7l + darwin/arm64) into dist/
make cross-build
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
- `make build` — Build binaries to dist/ (stamped, static, -trimpath)
- `make cross-build` — Build the release matrix (linux amd64/arm64/armv7l + darwin/arm64)
- `make docker-build` — Build the huddlescope runtime image locally
- `make clean` — Remove build artifacts
- `make check` — Run fmt, lint, test, coverage, security

**Site targets**
- `make site` — Build Hugo site to web/public/
- `make site-test` — Run site verification (success/failure/idempotency)
- `make site-watch` — Run Hugo server locally

### Run ingest (fetch feeds)
```sh
./dist/huddlescope ingest run
# or with config:
./dist/huddlescope ingest run -config .env
```

### Run export (export stories to Hugo data)
```sh
./dist/huddlescope export
# or with config:
./dist/huddlescope export -config .env
```

### Check heartbeat staleness
```sh
./dist/huddlescope heartbeat check --file web/data/heartbeat.json --stale-after 90m
```

## Releases

Release binaries are built and uploaded as assets of a GitHub Release by the
`build-and-release.yaml` workflow (on `v*` tag push or manual dispatch).
Releases are **Linux-only** (amd64, arm64, armv7l); a `darwin/arm64` dev
artifact is also built but kept as a GitHub artifact, never attached to the
release.

Each release includes a `SHA256SUMS` manifest. Verify an artifact before use:

```sh
# Download huddlescope_<version>_linux_<arch> and SHA256SUMS from the release,
# then:
sha256sum -c SHA256SUMS
```

Artifacts are built with `CGO_ENABLED=0 -trimpath` and a deterministic build
stamp, are statically linked (verified with `readelf` in CI), and embed only
non-secret build metadata (version, commit, time). No secrets are ever
embedded in the binaries.

## Docker

A multi-stage `Dockerfile` at the repo root yields a minimal runtime image:
the static binary at `/usr/local/bin/huddlescope`, CA certs present, and a
non-root user. No `.env` or credentials are baked into the image — runtime
config is supplied as environment variables at `docker run` time, identical to
the native binary's precedence chain (flags > env > file > defaults), so the
container and native binary share the same execution model.

```sh
# Build the image locally (not published automatically — image publishing is
# the separate, unapproved #204 scope):
make docker-build

# Run it, supplying config as env (matches native):
docker run --rm \
  -e HSC_ENV=production \
  -e HSC_STORAGE_BACKEND=turso \
  -e TURSO_DATABASE_URL=... \
  -e TURSO_AUTH_TOKEN=... \
  huddlescope:$(git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//') version
```

No images or tags are published automatically by any workflow; building and
publishing images remains a manual, reviewed operation.

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
- **Publish decision (HS-056)**: After export, the workflow makes an explicit publish decision (the old "data safety guard" that restored a committed demo file was removed):
  - exported snapshot has **≥ 1 story** → commit and deploy the snapshot;
  - **zero stories** but the previously committed snapshot had ≥ 1 story → skip the homepage commit and the Pages deploy: the last successfully deployed (real) content is retained;
  - **zero stories** and never a real snapshot → deploy the clearly-labelled "No stories published yet" empty state.
  A failed ingest or export fails the build job, so the deploy job is skipped and the last-good Pages deployment is preserved. A hard "Verify no demo markers" step greps the built `web/public/` for sample-data signatures and blocks any Pages upload that would present the sample as real news. See `web/README-data.md` for the full contract.
- **Production backend**: The refresh workflow sets `HSC_STORAGE_BACKEND=turso`. The Turso backend is implemented via `libsql-client-go` (`internal/adapters/storage/turso`); production mode requires `TURSO_DATABASE_URL` and `TURSO_AUTH_TOKEN`, which are validated at startup (a missing value fails the ingest step with a clear, redacted error). Use `sqlite-file` for local runs.

See issue #17 for Turso→Hugo JSON export and issue #18 for the refresh workflow.

Release and operations documentation: [docs/release-checklist.md](docs/release-checklist.md) (operator release checklist and evidence rule), [docs/ops-runbook.md](docs/ops-runbook.md) (provisioning, manual refresh, failure diagnosis, cost review), [docs/release-gate.md](docs/release-gate.md) (HS-058 release gate).

## Configuration

### Portable Deployment

The `huddlescope` binary is fully portable. It can run on a VPS or Raspberry Pi
that does **not** sit inside the huddlescope repo, using Turso as the shared
store. See [docs/portable-deployment.md](docs/portable-deployment.md) for the
detailed guide.

- **Config precedence**: CLI flags (`--config`, `--sources-file`, `--teams-file`,
  `--log-level`, `--timeout`) > environment variables > `--config` file > defaults.
- **`HSC_PUBLISH_SITE_DATA`**: `auto` (default) skips `web/data` writes in
  production (repo-less hosts); `true` always writes (GitHub Actions); `false`
  never writes.
- **Absolute paths**: explicitly-set input paths (`HSC_SOURCES_FILE`,
  `HSC_TEAMS_FILE`) must be absolute in all envs; output paths must be absolute
  in production.
- **`scripts/install-config.sh`**: installs a deployment config with `0600`
  permissions (`deploy` → `/etc/huddlescope/deploy.env`, `rpi` →
  `/opt/huddlescope/huddlescope.env`).
- **Fail-fast**: production requires `HSC_STORAGE_BACKEND=turso`,
  `TURSO_DATABASE_URL`, and `TURSO_AUTH_TOKEN`; missing values fail clearly
  without partial writes.

### Environment Variables

| Variable | Default | Production | Description |
|----------|---------|------------|-------------|
| `HSC_LOG_LEVEL` | `info` | No | Log level: debug, info, warn, error |
| `HSC_USER_AGENT` | `huddlescope/0.1` | No | User agent string for HTTP requests |
| `HSC_REQUEST_TIMEOUT` | `30s` | No | Request timeout (e.g., `60s`) |
| `HSC_HUGO_BASE_URL` | `https://huddlescope.github.io/` | No | Base URL for Hugo exports |
| `HSC_STORAGE_BACKEND` | `memory` | Yes | Storage backend: memory, turso, sqlite-file; turso is implemented via libsql-client-go and requires `TURSO_DATABASE_URL` + `TURSO_AUTH_TOKEN` in production |
| `HSC_SOURCES` | (none) | No | Source list: `id,name,url[,interval];...` (legacy; prefer `HSC_SOURCES_FILE`) |
| `HSC_SOURCES_FILE` | `conf/sources.yaml` | No | Path to the YAML sources file |
| `HSC_TEAMS_FILE` | `conf/teams.yaml` | No | Path to the YAML teams file |
| `HSC_LOOKBACK` | `24h` | No | Lookback window for incremental ingestion (e.g., `6h`) |
| `HSC_ENV` | `dev` | Yes | Environment: dev, production |
| `TURSO_DATABASE_URL` | (none) | Yes | Turso database URL |
| `TURSO_AUTH_TOKEN` | (none) | Yes | Turso authentication token |
| `HSC_RUNNER_ORIGIN` | `local` | No | Execution origin recorded on ingest run rows (e.g., `vps`, `raspberry`, `github-backup`) |
| `HSC_PUBLISH_SITE_DATA` | `auto` | No | Site-data writes: `auto` (dev writes, production skips), `true` (always), `false` (never) |
| `HSC_JSON_LOG` | `false` | No | Opt-in single-line JSON run log (no secrets) |

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

Feed sources are configured in `conf/sources.yaml` and loaded by `internal/config`, which builds the domain `FeedRegistry` (`Config.SourceRegistry()`). The registry supports:

- **Per-source failure isolation**: One failing source never blocks others
- **Priority-based ordering**: Sources polled in priority order
- **Selective disabling**: Set `enabled=false` to skip a source without deleting it
- **Acceptable-use policy**: Mark restricted/paywalled feeds appropriately

To add a feed, append an entry under `sources:` in `conf/sources.yaml` (the field reference is under "Source Configuration" above) using a unique `id`; to remove a feed, delete its entry. No code changes are required. If no sources are configured, `ingest` exits with an error.

#### Acceptable-Use Policy

- **Allowed**: RSS/Atom syndication is permitted; publicly available content may be cached and displayed.
- **Restricted**: Paywalled or subscription-required content. Set `enabled: false` so the feed is never fetched, preventing access attempts.

HuddleScope honors publisher terms of service and never attempts to bypass access controls.

#### Best Practices

- Prefer public, well-known NFL feeds that explicitly permit RSS syndication.
- Mark paywalled or restricted feeds as `enabled: false`.
- Never hardcode credentials.
- Use descriptive, stable IDs (`snake_case`; include the domain if it helps).

### Issue tracking

See [docs/ops-runbook.md](docs/ops-runbook.md) for issue tracking and CI status.

See [docs/worker-deployment.md](docs/worker-deployment.md) for step-by-step Cloudflare Worker first-time deployment.

### Production Mode

Set `HSC_ENV=production` to enable production mode. Production requires:
- `HSC_STORAGE_BACKEND` must be `turso` or `sqlite-file` (not `memory`)
- `TURSO_DATABASE_URL` must be set
- `TURSO_AUTH_TOKEN` must be set

The Turso backend is implemented via `libsql-client-go`; it requires `TURSO_DATABASE_URL` and `TURSO_AUTH_TOKEN` (validated at startup). `sqlite-file` is available for local runs and does not require credentials.

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