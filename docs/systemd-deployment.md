# Systemd Deployment (Raspberry Pi / VPS)

This document describes how to install the `huddlescope` binary on a Linux
host — a Raspberry Pi (arm64) or a VPS (amd64/arm64) — so that it runs an
autonomous RSS ingest **every 30 minutes** using a systemd timer, without
GitHub Actions or Cloudflare. Turso is the shared store; GitHub Actions
continues to run export and keep the committed site-data files fresh.

An optional Docker path (compose + host cron/timer) is described in
[Docker alternative](#docker-alternative-optional). Native systemd is the primary path.

Issue: #162 (HS-PORT-06).

Related docs:

- [`portable-deployment.md`](portable-deployment.md) — the config precedence
  chain and the native install steps this document extends.
- [`release-gate.md`](release-gate.md) — the release tarballs the binary is
  installed from.
- [`release-checklist.md`](release-checklist.md) — release process.

## Prerequisites

- Debian 11+ / Ubuntu 20.04+ (arm64 or amd64) with **systemd** present.
- A non-root sudo user.
- Outbound HTTPS access (RSS + Turso). No inbound ports are opened.

> **Never run the service as root.** Every step below uses a dedicated
> non-privileged `huddlescope` system user.

## 1. Create the service user and directories

```bash
# Dedicated system user: no login shell, no home directory of its own.
sudo useradd --system --no-create-home --shell /usr/sbin/nologin huddlescope

# Config + env live in /etc; runtime state in /var/lib.
sudo mkdir -p /etc/huddlescope /var/lib/huddlescope
sudo chown huddlescope:huddlescope /var/lib/huddlescope
```

## 2. Install the binary

Download the release tarball for your architecture (see
[`release-gate.md`](release-gate.md)) and install the binary to
`/usr/local/bin/huddlescope`:

```bash
# Example: install from the release artifact for your arch (amd64 or arm64).
sudo install -m 0755 huddlescope /usr/local/bin/huddlescope
```

Cross-compile reference (if you build locally):

```bash
GOARCH=arm64 GOOS=linux go build -trimpath -ldflags="-s -w" -o /tmp/hs-arm64 ./cmd/huddlescope
GOARCH=amd64 GOOS=linux go build -trimpath -ldflags="-s -w" -o /tmp/hs-amd64 ./cmd/huddlescope
```

## 3. Install the env and config files

Install the environment file (0600, owned by `huddlescope`) and the input
YAML files:

```bash
# The committed example ships placeholder-only secrets. Copy it, fill in the
# real Turso credentials, then install with safe permissions.
sudo cp deploy/examples/huddlescope.env.example /tmp/huddlescope.env
# Edit /tmp/huddlescope.env: set TURSO_DATABASE_URL and TURSO_AUTH_TOKEN.
sudo install -m 0600 /tmp/huddlescope.env /etc/huddlescope/huddlescope.env
sudo chown huddlescope:huddlescope /etc/huddlescope/huddlescope.env

# Input files (absolute paths referenced by the env file).
sudo install -m 0640 conf/sources.yaml /etc/huddlescope/sources.yaml
sudo install -m 0640 conf/teams.yaml   /etc/huddlescope/teams.yaml
sudo chown huddlescope:huddlescope /etc/huddlescope/sources.yaml /etc/huddlescope/teams.yaml
```

The env file sets `HSC_SOURCES_FILE=/etc/huddlescope/sources.yaml` and
`HSC_TEAMS_FILE=/etc/huddlescope/teams.yaml`. If you place them elsewhere,
update those values.

## 4. Install the systemd units

```bash
sudo install -m 0644 deploy/systemd/huddlescope-ingest.service /etc/systemd/system/huddlescope-ingest.service
sudo install -m 0644 deploy/systemd/huddlescope-ingest.timer   /etc/systemd/system/huddlescope-ingest.timer
sudo systemctl daemon-reload
```

The service unit:

- `Type=oneshot` — runs once per trigger, then exits; it is not a daemon.
- `User=huddlescope` / `Group=huddlescope` — non-root.
- `EnvironmentFile=/etc/huddlescope/huddlescope.env` — feeds the binary's env
  tier of the precedence chain (flags > env > `--config` file > defaults), so
  `--config` is not needed in `ExecStart`.
- `ExecStart=/usr/local/bin/huddlescope ingest run --mode primary`.
- `TimeoutStartSec=1500` (25 min) — kills a hung run before the next 30-min
  tick. The Turso lease (`LeaseTTL` = 20 min) frees the lock, so the next tick
  proceeds.
- Moderate hardening: `NoNewPrivileges`, `ProtectSystem=full`,
  `ProtectHome=read-only`, `PrivateTmp`, kernel/control-group protections,
  `LockPersonality`, `RestrictRealtime`. `MemoryDenyWriteExecute` is
  deliberately omitted (Go runtime W^X compatibility).

The timer unit:

- `OnCalendar=*-*-* *:00,30:00` — every 30 minutes.
- `Persistent=true` — at most one catch-up run after boot/downtime.
- `Unit=huddlescope-ingest.service`.

## 5. Enable and start the timer

```bash
sudo systemctl enable --now huddlescope-ingest.timer
```

## 6. First manual execution

Run the service once manually to confirm it works before relying on the timer:

```bash
sudo systemctl start huddlescope-ingest
systemctl status huddlescope-ingest
```

## 7. Verification

```bash
# Timer is active and scheduled for the next tick.
systemctl list-timers huddlescope-ingest.timer

# Full log for the unit.
journalctl -u huddlescope-ingest --since "1 hour ago"

# Config sanity check (secrets redacted). Run as the service user's env or
# with the installed env file.
/usr/local/bin/huddlescope config check --config /etc/huddlescope/huddlescope.env

# One manual ingest run to confirm the pipeline end-to-end.
sudo systemctl start huddlescope-ingest
```

A successful run logs the pipeline summary and, with `HSC_JSON_LOG=true`, one
JSON line to stderr. Check the log for `stories_created` / `stories_updated`
counts and no redacted error.

## 8. Update / revert procedure

To update the binary (and/or units):

```bash
# 1. Stop the timer so it does not trigger mid-update.
sudo systemctl stop huddlescope-ingest.timer

# 2. Replace the binary (and re-install units if changed).
sudo install -m 0755 <new-binary> /usr/local/bin/huddlescope
# (re-install the unit files + daemon-reload if they changed)
sudo systemctl daemon-reload

# 3. Restart the timer.
sudo systemctl enable --now huddlescope-ingest.timer
```

To revert, repeat the above with the **previous** release tarball's binary.
Because the units are `oneshot` and the lease is per-run, swapping the binary
between ticks is safe: no run is in flight after the timer is stopped.

## 9. Failure and edge cases

- **Reboot / repeated timer execution without duplicates.** systemd does not
  re-trigger an active `oneshot` unit, so a slow run skips the next tick
  instead of piling up. `Persistent=true` performs **at most one** catch-up
  after boot (bounded by `TimeoutStartSec`). The distributed Turso lease
  (issue #160) makes any residual double-start a no-op for the losing host —
  so even a second host or a stale duplicate start never writes two runs.

- **`kill -TERM` mid-run.** The runner catches SIGINT/SIGTERM, cancels the
  pipeline, and releases the lease (issue #160). The service unit's
  `TimeoutStopSec=30` bounds the stop; SIGTERM drives the same clean-abort +
  lease-release path.

- **Unreachable Turso.** The Turso repository constructor performs a `Ping` on
  construction, so an unreachable/bad Turso **fails fast** with a redacted
  error (the DSN is never logged). Every tick then fails loudly in the journal;
  the next tick retries. This is expected behavior — the operator action item
  is to restore Turso reachability. A single failing **source** does **not**
  fail the run: per-source errors are isolated and the other sources continue.

- **A hung run.** `TimeoutStartSec=1500` (25 min) kills it before the next
  30-min tick; the lease (20 min TTL) frees the lock, so the next tick
  proceeds cleanly.

## 10. Docker alternative (optional)

For an amd64 VPS, a one-shot Docker run is a convenience. Scheduling is
**external** (host cron or a systemd timer running `docker compose run --rm
ingest`); there is **never** an in-container loop and **no** orchestrator.

The committed `compose.yaml` at the repo root:

- `restart: "no"` — one-shot, never auto-restarted.
- `user: "65534:65534"` (nobody) — non-root.
- `env_file: ./huddlescope.env` — an **operator-provided, gitignored** file
  (the committed `deploy/examples/huddlescope.env.example` is a placeholder
  only and is never referenced by compose).
- `command: ["ingest", "run", "--mode", "primary"]`.

Build the image (no workflow publishes it — image publication is the separate,
unapproved #204 scope):

```bash
make docker-build
# Create ./huddlescope.env from deploy/examples/huddlescope.env.example,
# filling in the real Turso credentials (mode 0600).
docker compose run --rm ingest
```

Schedule it externally, e.g. a host cron entry:

```cron
*/30 * * * * cd /path/to/repo && docker compose run --rm ingest
```

> **Note on arm64:** the `Dockerfile` build stage hardcodes `GOARCH=amd64`
> (issue #161), so on an arm64 Raspberry Pi Docker requires binfmt/qemu.
> Prefer **native systemd** on the RPi; use Docker as the amd64-VPS
> convenience.

## 11. Maintenance and network posture

- **No GitHub token, no Cloudflare token.** The `turso` backend needs only the
  Turso DSN/token. The host performs egress HTTPS only.
- **No inbound ports.** Nothing documented opens a port; the host only makes
  outbound HTTPS calls to RSS feeds and Turso.
- **Export** is run from GitHub Actions, which keeps the committed
  `web/data/*` files fresh. The host never writes repo files.

## Security

> **Security callout**
>
> - **Never run the service as root.** The unit sets `User=huddlescope`
>   (non-root); the test suite asserts the user is not `root`/`0`.
> - **Config files are 0600** and owned by `huddlescope:huddlescope`, so the
>   Turso token is not readable by other users.
> - **Journal exposure is redacted:** the JSON run log uses a strict field
>   whitelist and `config check` omits the Turso auth token, so secrets never
>   appear in logs or status output.
> - **Moderate hardening** is applied in the unit (`NoNewPrivileges`,
>   `ProtectSystem=full`, `PrivateTmp`, kernel/control-group protections).
