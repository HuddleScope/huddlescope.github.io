# Portable Deployment

This document describes how to run the `huddlescope` binary on hosts that do
not sit inside the huddlescope GitHub repository — specifically a VPS and a
Raspberry Pi — using Turso as the shared store. GitHub Actions continues to
run export and keep the committed site-data files fresh.

Issue: #158 (HS-PORT-02).

## Goals

- **VPS / Raspberry Pi**: ingest to Turso without writing to `web/data` or any
  repo path. The binary must work from any working directory with absolute
  paths.
- **GitHub Actions**: run export and publish the site-data files
  (`web/data/heartbeat.json`, `web/data/feeds.json`, `web/data/homepage.json`)
  so the committed versions stay fresh.
- **Config precedence**: CLI flags > environment variables > `--config` file >
  defaults.
- **Fail-fast**: missing mandatory production config (Turso URL/token,
  non-memory backend) fails clearly, without partial writes.

## Configuration precedence

The documented precedence chain (highest wins):

1. **CLI flags**: `--config`, `--sources-file`, `--teams-file`, `--log-level`,
   `--timeout` (persistent flags on the root command).
2. **Environment variables**: `HSC_*`, `TURSO_*`.
3. **`--config` file**: `KEY=VALUE` lines (e.g., `/etc/huddlescope/deploy.env`).
4. **Built-in defaults**.

Only flags the operator explicitly sets (not left at their default) are applied
as overrides, so a flag left at its default never clobbers an environment
value.

## Key environment variables

| Variable | Purpose | Default |
|---|---|---|
| `HSC_ENV` | `dev` or `production` | `dev` |
| `HSC_STORAGE_BACKEND` | `memory`, `sqlite-file`, or `turso` | `memory` |
| `TURSO_DATABASE_URL` | Turso database URL (required in production) | — |
| `TURSO_AUTH_TOKEN` | Turso auth token (required in production) | — |
| `HSC_SOURCES_FILE` | Absolute path to a YAML sources file | `conf/sources.yaml` |
| `HSC_TEAMS_FILE` | Absolute path to a YAML teams file | `conf/teams.yaml` |
| `HSC_PUBLISH_SITE_DATA` | `auto`, `true`, or `false` (site-data writes) | `auto` |
| `HSC_JSON_LOG` | `true`/`false` (opt-in single-line JSON run log) | `false` |
| `HSC_RUNNER_ORIGIN` | Execution origin label (issue #159) | `local` |
| `HSC_HEARTBEAT_FILE` | Path to `heartbeat.json` | `web/data/heartbeat.json` |
| `HSC_FEEDS_FILE` | Path to `feeds.json` | `web/data/feeds.json` |
| `HSC_DATA_DIR` | Hugo data directory | `web/data` |
| `HSC_HUGO_CONTENT_DIR` | Hugo content directory | `web/content` |
| `HSC_STORAGE_PATH` | SQLite file path (only for `sqlite-file` backend) | `data/story.db` |

### `HSC_PUBLISH_SITE_DATA`

Controls whether ingest writes the site-data files
(`web/data/heartbeat.json` + `web/data/feeds.json`):

- **`auto`** (default): dev/test writes the files; **production skips them**.
  This is what a repo-less VPS/Raspberry host wants — ingest never creates
  `web/data` on a production host.
- **`true`**: always write the files, even in production. GitHub Actions sets
  this so the committed files stay fresh.
- **`false`**: never write the files.

The run indicators (ingest run status, feed-health counters) are always
persisted to the shared storage (Turso) regardless of this setting, so the
dashboard can read them from the database later (HS-PORT-07).

### `HSC_JSON_LOG`

Opt-in single-line JSON run log. When `true`, ingest emits one JSON line to
stderr per run with a strict field whitelist: `time`, `run_id`, `origin`,
`command`, `status`, `exit_code`, `committed`, and integer counters
(`entries_fetched`, `stories_created`, `stories_updated`, `stories_skipped`,
`errors`). The whitelist guarantees no secret (Turso token, DSN, etc.) can
appear in the output.

## Path independence

When an operator **explicitly sets** a path (via flag, env, or config file),
it must be an **absolute path**:

- **Input files** (`HSC_SOURCES_FILE`, `HSC_TEAMS_FILE`): absolute paths are
  required in **all** environments. Relative paths are rejected.
- **Output paths** (`HSC_HEARTBEAT_FILE`, `HSC_FEEDS_FILE`, `HSC_DATA_DIR`,
  `HSC_HUGO_CONTENT_DIR`, `HSC_STORAGE_PATH`): absolute paths are required in
  **production**. Relative paths are rejected.

The built-in repo-relative defaults (`conf/sources.yaml`,
`conf/teams.yaml`, `web/data/*`) remain valid so documented dev usage keeps
working. Only an operator-set relative path is rejected.

## VPS deployment

1. Build the binary: `make build-huddlescope` (or cross-compile for the VPS
   architecture).
2. Install the binary: `sudo install -m 0755 dist/huddlescope /usr/local/bin/huddlescope`
3. Install the config:
   ```bash
   cp conf/deploy.production.example.env /tmp/deploy.env
   # edit /tmp/deploy.env: set TURSO_DATABASE_URL and TURSO_AUTH_TOKEN
   sudo scripts/install-config.sh deploy /tmp/deploy.env
   ```
   This writes `/etc/huddlescope/deploy.env` with mode `0600`.
4. Copy `conf/sources.yaml` and `conf/teams.yaml` to absolute locations
   (e.g., `/etc/huddlescope/`) and set `HSC_SOURCES_FILE` / `HSC_TEAMS_FILE`
   to absolute paths in the deploy env.
5. Verify: `huddlescope config --check --config /etc/huddlescope/deploy.env`
6. Run ingest: `huddlescope ingest run --config /etc/huddlescope/deploy.env`

With `HSC_PUBLISH_SITE_DATA=auto` (the default) and `HSC_ENV=production`, the
VPS ingest **skips** writing `web/data` files. The run indicators go to Turso.

## Raspberry Pi deployment

The RPi is repo-less and runs ingest to Turso. Export is run from GitHub
Actions.

1. Build the binary (cross-compile for arm64): `make build-huddlescope-rpi`
   (or `GOARCH=arm64 GOOS=linux go build ...`).
2. Install under `/opt/huddlescope`:
   ```bash
   sudo mkdir -p /opt/huddlescope
   sudo install -m 0755 dist/huddlescope /opt/huddlescope/huddlescope
   cp conf/sources.yaml conf/teams.yaml /opt/huddlescope/
   ```
3. Install the config:
   ```bash
   cp conf/rpi.production.example.env /tmp/rpi.env
   # edit /tmp/rpi.env: set TURSO_DATABASE_URL and TURSO_AUTH_TOKEN
   sudo scripts/install-config.sh rpi /tmp/rpi.env
   ```
   This writes `/opt/huddlescope/huddlescope.env` with mode `0600`.
4. Verify: `/opt/huddlescope/huddlescope config --check --config /opt/huddlescope/huddlescope.env`
5. Run ingest: `/opt/huddlescope/huddlescope ingest run --config /opt/huddlescope/huddlescope.env`

The RPi config uses absolute paths under `/opt/huddlescope/` and
`HSC_RUNNER_ORIGIN=raspberry`. With `HSC_PUBLISH_SITE_DATA=auto` and
`HSC_ENV=production`, the RPi ingest skips writing `web/data`.

## GitHub Actions

GitHub Actions (`refresh-news.yml`) runs ingest (as backup, per issue #160)
and export. It sets:

- `HSC_ENV=production`
- `HSC_STORAGE_BACKEND=turso`
- `HSC_RUNNER_ORIGIN=github-backup`
- **`HSC_PUBLISH_SITE_DATA=true`** (issue #158 / AC-5)

The `HSC_PUBLISH_SITE_DATA=true` ensures the committed
`web/data/heartbeat.json` and `web/data/feeds.json` stay fresh. The workflow
commits these files after each run (with `continue-on-error: true` so a
transient commit failure does not break the deploy).

## Fail-fast diagnostics

In production, the following are required and produce clear errors if missing:

- `HSC_STORAGE_BACKEND` must be non-memory (`turso` or `sqlite-file`).
- `TURSO_DATABASE_URL` must be set.
- `TURSO_AUTH_TOKEN` must be set.
- Explicitly-set relative paths are rejected (see Path independence above).

Turso is fail-fast: the repository constructor performs a `Ping` on
construction, so a bad DSN or token fails immediately with a redacted error
message (the DSN is never logged).

One-line warnings (non-fatal) are emitted for:

- **memory / sqlite-file backend**: `Warning: storage backend "memory" is not
  shared/persistent; data is per-host only` — reminds the operator that the
  data is not shared across hosts.

## Security

- Config files installed by `scripts/install-config.sh` have mode `0600`
  (owner read/write only), protecting Turso tokens from other users.
- The JSON run log uses a strict field whitelist; no secret can appear.
- Error messages redact the Turso DSN (existing behavior).
- `config check` output uses `MarshalJSON` which omits `TursoAuthToken` and
  `HugoDraftsToken` (redacted to `***`).

## Cross-compile reference

```bash
# Raspberry Pi (arm64 Linux)
GOARCH=arm64 GOOS=linux go build -o dist/huddlescope-rpi ./cmd/huddlescope

# VPS (amd64 Linux)
GOARCH=amd64 GOOS=linux go build -o dist/huddlescope-vps ./cmd/huddlescope
```

The binary is fully static and portable; no repo or web-data is required at
runtime when using the turso backend with absolute paths.

## Container deployment

The same binary runs inside a container with **identical configuration** to the
native (VPS/RPi) deployment — the container and native binary share the same
precedence chain (flags > env > `--config` file > defaults) and execution model.
No repo, web-data, or `.env` is baked into the image; config is supplied as
environment variables at `docker run` time.

Build the image locally, or pull the published image (issue #204 publishes
`ghcr.io/huddlescope/huddlescope.github.io` on every merge to `main`):

```bash
# Local build:
make docker-build
# tags huddlescope:<version>

# Or pull the published image:
docker pull ghcr.io/huddlescope/huddlescope.github.io:latest
```

Run it, mirroring the native production config (`HSC_ENV=production`,
`HSC_STORAGE_BACKEND=turso`): pass the Turso DSN and token, and the absolute
sources/teams paths, as env:

```bash
docker run --rm \
  -e HSC_ENV=production \
  -e HSC_STORAGE_BACKEND=turso \
  -e TURSO_DATABASE_URL=... \
  -e TURSO_AUTH_TOKEN=... \
  -e HSC_SOURCES_FILE=/abs/path/sources.yaml \
  -e HSC_TEAMS_FILE=/abs/path/teams.yaml \
  -e HSC_RUNNER_ORIGIN=docker \
  huddlescope:$(git describe --tags --abbrev=0 | sed 's/^v//') ingest run
```

Image invariants (enforced by `TestDockerfileSafeguards` / `TestNoImagePushWorkflow`):

- multi-stage build; build stage sets `CGO_ENABLED=0` and uses `-trimpath`;
- runtime stage is `gcr.io/distroless/static` (ships CA certs, no shell) and
  runs as a non-root `USER`;
- the binary is installed at `/usr/local/bin/huddlescope` and set as the
  `ENTRYPOINT`;
- no `COPY` of `.env` or credentials into any layer;
- image publication is restricted to the approved Docker workflow
  (`.github/workflows/docker-build.yaml`, issue #204); no other workflow may
  log in to or push to a container registry.

Because the container uses env for config exactly like the native binary, the
fail-fast production checks (Turso URL/token required, non-memory backend,
absolute paths) apply unchanged inside the container, and `config check`
redacts secrets the same way.
