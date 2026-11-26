# scripts/deploy.sh — auto-deploy (issue #202 / HS-PORT-10)

One-shot deploy of the `huddlescope` binary + service. Detects OS/arch,
acquires the matching binary (release download on Linux, local `go build` on
macOS), verifies its SHA256, installs it atomically, installs and activates
the right init system (systemd timer on Linux, launchd LaunchAgent on macOS),
runs `config check` and a first ingest, and reports a machine-readable status.

## Quick start

```bash
# Linux (fresh or upgrade), latest release:
sudo bash scripts/deploy.sh

# Pin a version:
sudo bash scripts/deploy.sh --version 1.2.3

# Explicit source + arch:
sudo bash scripts/deploy.sh --source release --os linux --arch arm64

# macOS dev machine (builds locally; no darwin release assets exist):
bash scripts/deploy.sh            # -> local go build, installs to /opt/huddlescope

# Preview with zero mutations (safe in a PR check / on a clean host):
bash scripts/deploy.sh --dry-run
```

## Options

| Flag | Env | Default | Meaning |
|---|---|---|---|
| `--version V` | `HSC_DEPLOY_VERSION` | `latest` | Release version (`X.Y.Z` or `vX.Y.Z`). `latest` = highest semver tag. |
| `--os OS` | `HSC_DEPLOY_OS` | `uname -s` | Target OS: `linux` \| `darwin`. |
| `--arch ARCH` | `HSC_DEPLOY_ARCH` | `uname -m` | Target arch: `amd64` \| `arm64` \| `armv7l`. |
| `--install-dir DIR` | `HSC_DEPLOY_INSTALL_DIR` | `/usr/local/bin` (linux) / `/opt/huddlescope` (darwin) | Binary install directory. |
| `--config-file PATH` | `HSC_DEPLOY_CONFIG` | `/etc/huddlescope/huddlescope.env` (linux) / `/opt/huddlescope/huddlescope.env` (darwin) | Env file passed to `config check` and used to record the checksum. |
| `--source SRC` | `HSC_DEPLOY_SOURCE` | `auto` | `release` \| `build` \| `auto`. `auto` = release on linux, build on darwin. |
| `--binary PATH` | — | — | Pre-built binary to install; skips download/build entirely. |
| `--no-service` | — | off | Install the binary only; skip unit install/activation. |
| `--dry-run` | — | off | Print detection, resolved version, source and target URL/command; make **zero** mutations. |

`HSC_DEPLOY_REPO` (env only, default `HuddleScope/huddlescope.github.io`) pins
the download source; it is never settable via a CLI flag and is validated to a
`owner/name` slug.

## Prerequisites

- **Linux (systemd path):** Debian 11+/Ubuntu 20.04+ with systemd; run with
  `sudo`. The script creates the `huddlescope` system user and the
  `/etc/huddlescope`, `/var/lib/huddlescope`, `/var/log/huddlescope` dirs.
- **macOS (launchd path):** a Go toolchain `>=` the `go.mod` version (the
  darwin path is always a local build — releases are Linux-only per
  issue #161 AC-4); run **without** sudo (the LaunchAgent is a user agent and
  the install dir is `/opt/huddlescope`, created if absent).
- Outbound HTTPS to `github.com` for release downloads / `latest` resolution
  (not needed for `--binary`, `--source build`, or an explicit `--version`).

The env/config files (`huddlescope.env`, `sources.yaml`, `teams.yaml`) must
already be installed before the first run — see
[`docs/systemd-deployment.md` §3](../../docs/systemd-deployment.md). The
script refuses to activate the service if `config check` fails.

## Behavior

- **Idempotent (AC-5):** if the installed binary's SHA256 equals the target's,
  the swap is skipped; service state is still re-verified.
- **Fail-closed checksum (AC-4):** the downloaded binary is verified against
  the release `SHA256SUMS` before any swap; a mismatch aborts.
- **Atomic swap + rollback (AC-13):** the new binary lands at
  `${BIN}.new` and is `mv`ed into place; the previous binary is kept at
  `${BIN}.prev` and restored automatically if any post-swap phase fails.
- **Service activation (AC-9/AC-12):** Linux → install units,
  `daemon-reload`, `enable --now` the timer, start the oneshot once.
  macOS → install + `launchctl load` the LaunchAgent, run once directly.
- **Checksum recorded (AC-16):** the on-disk binary's SHA256 is written last,
  to the env file as `HSC_INSTALLED_SHA256` (ignored by the permissive config
  loader) and to `/var/lib/huddlescope/installed.sha256` (linux) or
  `/opt/huddlescope/installed.sha256` (darwin).
- **Status (AC-15):** the final line is
  `DEPLOY_STATUS=success|fail version=... sha256=... arch=... os=...` with
  exit code 0/1 — usable by humans, cron, and CI/CD.
- **Logging (AC-14):** output is teed to `/var/log/huddlescope/deploy.log`
  (linux) or `/opt/huddlescope/logs/deploy.log` (darwin) when writable (0640).
  The script never prints env-file contents.

## Manual smoke checks (README smoke section)

```bash
# AC-1: detection + override (zero mutations).
bash scripts/deploy.sh --dry-run                       # -> detected os=... arch=...
bash scripts/deploy.sh --dry-run --os linux --arch amd64
bash scripts/deploy.sh --dry-run --os darwin --arch arm64

# AC-3: local build path prints the exact go build command with stamped ldflags.
bash scripts/deploy.sh --dry-run --source build

# Syntax gate (mirrors the new CI step).
bash -n scripts/deploy.sh
bash -n scripts/resolve-version.sh

# Plist validity (macOS host; CI has no plist linter).
plutil -lint deploy/launchd/com.huddlescope.ingest.plist
```

## Troubleshooting

- **`could not resolve latest release version`** — you're offline or the repo
  has no tags. Pass `--version X.Y.Z` explicitly, or `--binary PATH` /
  `--source build` to avoid the network entirely.
- **`no release assets exist for darwin`** — expected; darwin is always a
  local build. Use `--source build` (or let `auto` pick it).
- **`checksum mismatch`** — the downloaded asset does not match `SHA256SUMS`;
  aborts by design. Re-run against a known-good tag or rebuild locally.
- **`config check failed`** — fix the env/config files first; the service is
  not activated until `config check` passes.
- **`timer not active`** (linux) — check `systemctl status
  huddlescope-ingest.timer` and the journal (`journalctl -u
  huddlescope-ingest`). The oneshot service is transient by design; the timer
  is the durable unit.
- **armv7l on a real RPi 3/4** — verification is URL/ELF-level via `--dry-run`
  + cross-build on this host; mark "verified on RPi: pending" until a real
  RPi 3/4 accepts a deploy.
- **GitHub API rate limit** (60/hr anonymous) on many hosts deploying at once —
  low risk (deploys are rare); pin an explicit `--version` to avoid the
  `latest` `git ls-remote` call.

## Rollback

In-script rollback is automatic for post-swap failures. To roll back to a
previous release manually:

```bash
sudo bash scripts/deploy.sh --version <previous-tag>
```

This re-downloads, checksums, and swaps; if the target equals the installed
checksum it is a safe no-op for state files.
