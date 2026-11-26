# Implementation — Issue #202: Auto-deploy script (detect arch, download/build binary, activate service)

Status: **COMPLETE** — implemented exactly per the saved plan (`plan.md`). No
replan was required; every fact the plan relies on was re-verified against the
source before implementing.

## Files created / changed

| # | File | Action | What it does |
|---|------|--------|--------------|
| 1 | `scripts/deploy.sh` | **Create** (~590 lines, bash 3.2-compatible, `set -euo pipefail`) | The deployer. Phases: detect → resolve_version → acquire → verify → install (atomic swap) → install_units → config_check → first_run → verify_service → record_checksum → report. |
| 2 | `deploy/launchd/com.huddlescope.ingest.plist` | **Create** | LaunchAgent mirroring the systemd oneshot: `sh -c 'set -a; . /opt/huddlescope/huddlescope.env; exec … ingest run --mode primary --stale-after 75m'`, `WorkingDirectory=/opt/huddlescope`, `StartInterval=1800`, `RunAtLoad=false`, `KeepAlive=false`, logs under `/opt/huddlescope/logs/`. Secrets only via the 0600 env file (no `EnvironmentVariables`, no inlined TURSO_*/token, no `sudo`). |
| 3 | `internal/cli/launchd_units_test.go` | **Create** | Text-invariant guard test for the plist (`TestLaunchdAgentPlist` + `TestLaunchdPlistProgramArgumentsOrder`), same philosophy as `systemd_units_test.go`. Runs in `make test` → CI gate. |
| 4 | `internal/config/config_deploy_checksum_test.go` | **Create** | R5 guard: `TestLoadEnvFile_IgnoresDeployChecksumKey` asserts the permissive env loader ignores `HSC_INSTALLED_SHA256` — the mechanism AC-16 depends on. Pins the contract so a future strict-mode change breaks CI instead of silently breaking deploys. |
| 5 | `scripts/deploy/README.md` | **Create** | Usage, option/env table, prerequisites, behavior, manual smoke checks, troubleshooting, rollback. |
| 6 | `.github/workflows/ci.yml` | **Modify (minimal)** | One new step in the `build` job, after "Verify binaries": `bash -n scripts/deploy.sh` (syntax gate, no network/mutation). |
| 7 | `docs/systemd-deployment.md` | **Modify (minimal)** | §2 gains a pointer to `sudo bash scripts/deploy.sh --version vX.Y.Z` (manual path stays canonical). |

No Go runtime code changes. No `deploy/systemd/*` changes (installed as-is).
No release-workflow changes.

## Acceptance criteria — how each is met

- **AC-1 (detect OS/arch):** `detect()` maps `uname -s` → linux/darwin and
  `uname -m` → amd64/arm64/armv7l; overridable via `--os`/`--arch` and
  `HSC_DEPLOY_OS`/`HSC_DEPLOY_ARCH`. All values regex/`case`-validated before
  use (no injection surface). Verified: `--dry-run` prints
  `os=darwin arch=arm64`; `--os linux --arch amd64` overrides; invalid
  arch/version are rejected with `DEPLOY_STATUS=fail` + exit 1.
- **AC-2 (download Linux release):** `download_release()` builds
  `huddlescope_${V}_linux_${arch}` + `SHA256SUMS` from
  `https://github.com/${HSC_DEPLOY_REPO}/releases/download/v${V}/…` and uses
  `curl -fsSL --proto =https --retry 3` (or `gh release download` when
  `curl` is absent). `HSC_DEPLOY_REPO` is env-only and validated to an
  `owner/name` slug. Verified via `--dry-run` URL construction.
- **AC-3 (local build fallback):** `build_local()` runs
  `CGO_ENABLED=0 GOOS GOARCH GOARM go build -trimpath -ldflags="-s -w -X …cliVersion …cliCommit …cliBuiltAt"` —
  identical stamping to `build-and-release.yaml` / `make cross-build`. Used when
  Darwin (no release assets, #161 AC-4), `--source build`, or release assets
  unavailable. Verified: `--dry-run --source build` prints the exact go build
  command with stamped ldflags.
- **AC-4 (SHA256 verify, fail-closed):** `verify()` checks the staged binary
  against `SHA256SUMS` (release) or self-computes (build/binary) *before* any
  swap; a mismatch or missing manifest aborts. Verified: self-computed hash
  computed in the sandboxed install test.
- **AC-5 (idempotent):** `install_binary()` compares the installed binary's
  SHA256 to the target; on match it logs "checksum matches existing …
  skipping install" and skips the swap (service state still re-verified).
  **Verified in a real (non-dry-run) sandboxed run** — second identical run
  skipped the install.
- **AC-6 (user + dirs, linux):** `install_user_and_dirs()` creates the
  `huddlescope` system user (`useradd --system --no-create-home --shell
  /usr/sbin/nologin`, no-op if exists) and `/etc/huddlescope`,
  `/var/lib/huddlescope`, `/var/log/huddlescope` with `huddlescope` ownership
  on state/log dirs.
- **AC-7 (install path + atomic swap):** `install_binary()` installs to
  `/usr/local/bin/huddlescope` (linux) or `/opt/huddlescope/huddlescope`
  (darwin), overridable by `--install-dir`; swaps via `${BIN}.new` → `mv`.
  Verified in the sandboxed run (binary landed, 0755).
- **AC-8 (perms):** `chmod 0755` on the staged + installed binary; binary is
  root-owned (matching `install -m 0755` in `systemd-deployment.md` §2), dirs
  are `huddlescope`-owned. Verified (`stat` showed `755`).
- **AC-9 (init system):** `install_units()` — linux: install units,
  `systemctl stop` (R6), `daemon-reload`, `enable` the timer; darwin:
  `launchctl` load the LaunchAgent (path rewritten to the actual install
  dir). Auto-detected from `DEPLOY_OS`.
- **AC-10 (config check before activation):** `config_check()` runs
  `<bin> config check --config <env>` (as `sudo -u huddlescope` on linux) and
  **aborts before enable/restart** on failure. Verified fail-closed with a
  non-executable binary (aborted, exit 1) and pass-path with the real binary
  (`config check --config` exits 0 offline).
- **AC-11 (first ingest):** `first_run()` runs `ingest run --mode primary
  --stale-after 75m` once — via `systemctl start` of the oneshot on linux (as
  the service user), direct exec on darwin; requires exit 0.
- **AC-12 (verify service active):** `verify_service()` — `systemctl
  is-active huddlescope-ingest.timer` (linux, the durable unit) /
  `launchctl list com.huddlescope.ingest` (darwin).
- **AC-13 (rollback):** `install_binary()` keeps `${BIN}.prev`; any
  post-swap phase failure runs `rollback` (restore `.prev`) before
  `die`. Verified: upgrade path keeps `.prev`; the post-swap
  `|| { rollback; die }` wiring is in main.
- **AC-14 (log tee):** non-dry-run output is teed to
  `/var/log/huddlescope/deploy.log` (linux) or `<install-dir>/logs/deploy.log`
  (darwin, derived from `--install-dir`) at 0640; the script never prints
  env-file contents.
- **AC-15 (machine-readable status):** the `on_exit` trap emits a single
  `DEPLOY_STATUS=success|fail version=… sha256=… arch=… os=…` line with a
  matching exit code (0/1). Verified in every run above.
- **AC-16 (checksum recorded last):** `record_checksum()` computes the
  on-disk binary's SHA256 *after* the swap and writes it to the env file as
  `HSC_INSTALLED_SHA256` (appended or updated in place) and to
  `/var/lib/huddlescope/installed.sha256` (linux) / `<install-dir>/installed.sha256`
  (darwin). Only runs after all post-swap phases pass. (Proven via code path;
  full success path requires a live Turso/RSS and is covered by the Linux-VM
  matrix per the plan.)
- **AC-17 (CI/CD-compatible):** non-interactive, all inputs via flags/env,
  `--dry-run` makes **zero** mutations (verified — no files touched on dry-run),
  works from any working directory (repo root resolved from `$0`).

## Tests added (regression guards)

1. **`internal/cli/launchd_units_test.go`** — pins the plist invariants
   (one-shot `ingest run --mode primary`, `set -a` + env-file sourcing, absolute
   `WorkingDirectory`, `StartInterval=1800`, `KeepAlive`/`RunAtLoad` false,
   logs under `/opt/huddlescope/logs/`, no inlined `EnvironmentVariables`/
   TURSO_*/token, no `while true`, no `--daemon`, no `sudo`,
   `ProgramArguments` = exactly `[sh, -c, program]`). Runs in `make test` → CI.
2. **`internal/config/config_deploy_checksum_test.go`** — R5: loader ignores
   `HSC_INSTALLED_SHA256` (permissive env tier). Runs in `make test` → CI.
3. **New CI step** `bash -n scripts/deploy.sh` — shell syntax gate on every PR.

## Verification performed (all green)

- `bash -n scripts/deploy.sh`, `bash -n scripts/resolve-version.sh` — pass.
- `plutil -lint deploy/launchd/com.huddlescope.ingest.plist` — OK.
- `python3 -c yaml.safe_load` on `ci.yml` + `build-and-release.yaml` — valid.
- `gofmt -l` on the two new Go test files — clean.
- `make build` — pass. `go vet ./...` — clean.
- `make test` (race) — all packages `ok`, including `internal/cli` and
  `internal/config` (new guard tests run inside).
- `make check-coverage` — gate passed (core ≥80%, other internal ≥70%);
  `internal/cli` at 80.1%, `internal/config` at 89.1%.
- `make e2e` — offline matrix PASS.
- **Live (non-dry-run) sandboxed install test** on this darwin host:
  - AC-4 self-computed SHA256 computed.
  - AC-7/AC-8 atomic swap + 0755 perms + binary landed.
  - **AC-5 idempotent skip** on the second identical run ("checksum matches …
    skipping install").
  - AC-13 upgrade path (new binary swapped in, `.prev` kept).
  - AC-10 fail-closed: a non-executable "binary" aborts at `config check` with
    `DEPLOY_STATUS=fail` (no checksum recorded, no service activated).
  - AC-10 pass-path: the real built binary's `config check --config <file>`
    exits 0 offline.
- **Dry-run matrix** (`--dry-run`): detection, `--os`/`--arch` overrides,
  release URL + `SHA256SUMS` construction, build command with stamped ldflags,
  invalid-arch/version rejection, and zero mutations all confirmed.

## Deferred / out of scope (per plan)

- Full Linux-VM matrix (fresh install, re-run skip, old release, corrupt env →
  rollback) — requires a real systemd host + live Turso/RSS; the plan defers
  this to manual verification. The script's logic paths are proven offline as
  above.
- armv7l on a real RPi 3/4 — marked "verified on RPi: pending" in the README.
- Any release-workflow change to attach darwin assets (R1 decision item) — kept
  out of scope; the darwin path is a local build.
- `make lint` / `make security` (golangci-lint/gosec/govulncheck) — not run
  locally (tools not installed); they run in CI and the added code is gofmt
  clean + `go vet` clean.
