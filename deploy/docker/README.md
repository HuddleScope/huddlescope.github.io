# Docker deployment (issue #204)

huddlescope publishes a minimal runtime image to GitHub Container Registry on
every merge to `main` via `.github/workflows/docker-build.yaml`. The image is
built from the repo-root `Dockerfile` (multi-stage: static `CGO_ENABLED=0`
binary on a `gcr.io/distroless/static` base) and is tagged:

| Tag | Meaning |
|-----|---------|
| `ghcr.io/huddlescope/huddlescope.github.io:latest` | Always points to newest `main` |
| `ghcr.io/huddlescope/huddlescope.github.io:v{semver}` | Pinned to a semver (matches the release binary version from #203) |
| `ghcr.io/huddlescope/huddlescope.github.io:sha-{short}` | Exact commit |

## Image properties

- **No shell, no package manager.** The runtime base is
  `gcr.io/distroless/static`; the entrypoint is the binary directly.
- **Non-root.** The container runs as `65534:65534` (nobody).
- **CA certificates.** The distroless base ships the system CA bundle, so the
  Turso HTTPS connection works out of the box.
- **No baked-in secrets.** No `.env` or credentials are copied into any image
  layer; runtime config is supplied as environment variables at `docker run`
  time (or via `--env-file`), identical to the native binary's precedence
  chain (flags > env > file > defaults).
- **amd64 only.** The build stage hardcodes `GOARCH=amd64` (issue #161);
  arm64 hosts require binfmt/qemu emulation. Prefer the native systemd
  deployment (`deploy/systemd/`) on a Raspberry Pi.

## One-shot run

The container runs a single command and exits. Configuration comes from an
operator-provided env file (mode `0600`, never committed — see
`deploy/examples/huddlescope.env.example` for the shape):

```bash
docker run --rm \
  --env-file /etc/huddlescope/huddlescope.env \
  ghcr.io/huddlescope/huddlescope.github.io:latest ingest run --mode primary
```

Verify the stamp or config without running the pipeline:

```bash
docker run --rm ghcr.io/huddlescope/huddlescope.github.io:latest version
docker run --rm ghcr.io/huddlescope/huddlescope.github.io:latest config check
```

## Scheduling

Because the container is oneshot, scheduling is **external** — the same
pattern as the systemd timer in #162. There is never an in-container loop and
no orchestrator. Pick one of the two options below; no automation script is
delivered, you set this up yourself.

### Option A: system cron

Run every 30 minutes at minute 0 and 30:

```cron
0,30 * * * * /usr/bin/docker run --rm --env-file /etc/huddlescope/huddlescope.env ghcr.io/huddlescope/huddlescope.github.io:latest ingest run --mode primary
```

### Option B: systemd timer + oneshot service

A thin systemd unit that invokes `docker run` once per timer tick:

```ini
# /etc/systemd/system/huddlescope-ingest.timer
[Unit]
Description=Run huddlescope ingest every 30 minutes

[Timer]
OnCalendar=*-*-* *:00,30:00
Persistent=true

[Install]
WantedBy=timers.target
```

```ini
# /etc/systemd/system/huddlescope-ingest.service
[Unit]
Description=huddlescope ingest oneshot

[Service]
Type=oneshot
ExecStart=/usr/bin/docker run --rm --env-file /etc/huddlescope/huddlescope.env ghcr.io/huddlescope/huddlescope.github.io:latest ingest run --mode primary
TimeoutStartSec=300
```

Enable with:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now huddlescope-ingest.timer
```

### Alternative: committed compose file

The repo ships `compose.yaml` (one-shot, non-root, `env_file:
./huddlescope.env`). For an amd64 host you can also build locally with
`make docker-build` and run `docker compose run --rm ingest`; this is
equivalent to pulling the published image above.

## Native alternative

For a fully static binary on the host (no container runtime at all), see
`deploy/systemd/` and `docs/systemd-deployment.md`.
