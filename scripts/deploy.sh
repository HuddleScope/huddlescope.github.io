#!/usr/bin/env bash
#
# deploy.sh — auto-deploy the huddlescope binary and activate its service
# (issue #202 / HS-PORT-10).
#
# Phases: detect -> resolve_version -> acquire -> verify -> install (atomic
# swap) -> install_units -> config_check -> first_run -> verify_service ->
# record_checksum -> report.
#
# Defaults:
#   - Linux  -> download the matching pre-built release asset
#     (huddlescope_${VERSION}_linux_${arch}, verified against SHA256SUMS).
#   - Darwin -> local `go build` (releases are Linux-only, issue #161 AC-4);
#     override with --source release|build.
#   - --source build on Linux -> local go build fallback (also used when
#     release assets are unavailable).
#   - --binary PATH -> skip acquisition entirely (pure install path).
#
# Root model: run with sudo for system install paths; the service never runs
# as root (systemd unit User=huddlescope; launchd plist is a user agent).
#
# Output is teed to /var/log/huddlescope/deploy.log (Linux) or
# /opt/huddlescope/logs/deploy.log (Darwin) when writable. The final line is
# machine-readable: DEPLOY_STATUS=success|fail version=... sha256=... arch=...
#
# Written for bash 3.2 (macOS): no associative arrays, no ${var,,}, no mapfile.

set -euo pipefail

# ---------------------------------------------------------------------------
# Defaults (env-overridable; CLI flags win over env)
# ---------------------------------------------------------------------------
DEPLOY_OS="${HSC_DEPLOY_OS:-}"
DEPLOY_ARCH="${HSC_DEPLOY_ARCH:-}"
DEPLOY_VERSION="${HSC_DEPLOY_VERSION:-latest}"
INSTALL_DIR="${HSC_DEPLOY_INSTALL_DIR:-}"
CONFIG_FILE="${HSC_DEPLOY_CONFIG:-}"
SOURCE="${HSC_DEPLOY_SOURCE:-auto}"
BINARY=""
NO_SERVICE=0
DRY_RUN=0
DEPLOY_REPO="${HSC_DEPLOY_REPO:-HuddleScope/huddlescope.github.io}"
SERVICE_USER="huddlescope"

DEPLOY_STATUS=""
DEPLOY_SHA256=""
TAG=""
BIN=""
REPO_ROOT=""
STAGE_DIR=""
INSTALL_SKIPPED=0
PLIST=""
LOG_DIR=""
STATE_DIR=""
CONF_DIR=""

usage() {
  cat <<'EOF'
Usage: deploy.sh [options]

Options:
  --version V         Release version to install (X.Y.Z or with leading v).
                      Default: latest (highest semver tag).
  --os OS             Target OS: linux|darwin. Default: uname -s.
                      Env: HSC_DEPLOY_OS
  --arch ARCH         Target architecture: amd64|arm64|armv7l.
                      Default: uname -m, mapped. Env: HSC_DEPLOY_ARCH
  --install-dir DIR   Binary install directory. Default: /usr/local/bin (linux)
                      or /opt/huddlescope (darwin). Env: HSC_DEPLOY_INSTALL_DIR
  --config-file PATH  Env file for `config check`. Default: /etc/huddlescope/
                      huddlescope.env (linux) or /opt/huddlescope/huddlescope.env
                      (darwin). Env: HSC_DEPLOY_CONFIG
  --source SRC        Binary source: release|build|auto. Default: auto
                      (release on linux, build on darwin). Env: HSC_DEPLOY_SOURCE
  --binary PATH       Pre-built binary to install; skips download/build.
  --no-service        Install binary only; skip unit installation/activation.
  --dry-run           Print detection, resolved version, source and target
                      URL/command; make zero mutations.
  -h, --help          Show this help.
EOF
  exit "${1:-0}"
}

die() {
  echo "Error: $*" >&2
  exit 1
}

log() {
  echo "[deploy] $*"
}

on_exit() {
  status="$?"
  # Clean up the temp staging dir once install_binary has moved the binary out
  # (it is empty at that point). No-op for dry-run / build-before-install.
  if [ -n "${STAGE_DIR:-}" ] && [ -d "${STAGE_DIR:-}" ]; then
    rm -rf "$STAGE_DIR"
  fi
  if [ "$status" -eq 0 ] && [ "$DEPLOY_STATUS" = "success" ]; then
    echo "DEPLOY_STATUS=success version=$DEPLOY_VERSION sha256=$DEPLOY_SHA256 arch=$DEPLOY_ARCH os=$DEPLOY_OS"
  elif [ "$status" -ne 0 ]; then
    echo "DEPLOY_STATUS=fail version=${DEPLOY_VERSION:-unknown} sha256=${DEPLOY_SHA256:-none} arch=${DEPLOY_ARCH:-unknown} os=${DEPLOY_OS:-unknown}" >&2
  fi
}
trap on_exit EXIT

# ---------------------------------------------------------------------------
# Parse args
# ---------------------------------------------------------------------------
while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)     DEPLOY_VERSION="${2:?--version needs a value}"; shift 2 ;;
    --os)          DEPLOY_OS="${2:?--os needs a value}"; shift 2 ;;
    --arch)        DEPLOY_ARCH="${2:?--arch needs a value}"; shift 2 ;;
    --install-dir) INSTALL_DIR="${2:?--install-dir needs a value}"; shift 2 ;;
    --config-file) CONFIG_FILE="${2:?--config-file needs a value}"; shift 2 ;;
    --source)      SOURCE="${2:?--source needs a value}"; shift 2 ;;
    --binary)      BINARY="${2:?--binary needs a value}"; shift 2 ;;
    --no-service)  NO_SERVICE=1; shift ;;
    --dry-run)     DRY_RUN=1; shift ;;
    -h|--help)     usage 0 ;;
    *)             usage 1 ;;
  esac
done

# ---------------------------------------------------------------------------
# Validate external inputs before they are used in string-built commands.
# ---------------------------------------------------------------------------
case "$DEPLOY_REPO" in
  *..*|*/|/*|*" "*) die "invalid HSC_DEPLOY_REPO '$DEPLOY_REPO' (expected owner/name)" ;;
  */*) : ;;
  *) die "invalid HSC_DEPLOY_REPO '$DEPLOY_REPO' (expected owner/name)" ;;
esac

# ---------------------------------------------------------------------------
# AC-1: detect OS / architecture
# ---------------------------------------------------------------------------
detect() {
  if [ -z "$DEPLOY_OS" ]; then
    case "$(uname -s)" in
      Linux)  DEPLOY_OS="linux" ;;
      Darwin) DEPLOY_OS="darwin" ;;
      *)      die "unsupported OS $(uname -s) (expected Linux or Darwin)" ;;
    esac
  fi
  case "$DEPLOY_OS" in
    linux|darwin) : ;;
    *) die "invalid --os '$DEPLOY_OS' (expected linux|darwin)" ;;
  esac

  if [ -z "$DEPLOY_ARCH" ]; then
    case "$(uname -m)" in
      x86_64|amd64)    DEPLOY_ARCH="amd64" ;;
      arm64|aarch64)   DEPLOY_ARCH="arm64" ;;
      armv7l|armv7h)   DEPLOY_ARCH="armv7l" ;;
      i386|i686)       die "32-bit hosts unsupported; set --arch explicitly (amd64|arm64|armv7l)" ;;
      *) die "unsupported machine $(uname -m); set --arch explicitly (amd64|arm64|armv7l)" ;;
    esac
  fi
  case "$DEPLOY_ARCH" in
    amd64|arm64|armv7l) : ;;
    *) die "invalid --arch '$DEPLOY_ARCH' (expected amd64|arm64|armv7l)" ;;
  esac

  log "detected os=$DEPLOY_OS arch=$DEPLOY_ARCH"
}

# goos:goarch pair -> "GOOS GOARCH GOARM"
path_for() {
  case "$1" in
    linux/amd64)  echo "linux amd64" ;;
    linux/arm64)  echo "linux arm64" ;;
    linux/armv7l) echo "linux arm 7" ;;
    darwin/arm64) echo "darwin arm64" ;;
    *) die "unsupported target $1 (releases only cover linux amd64/arm64/armv7l)" ;;
  esac
}

go_mod_version() {
  sed -n 's/^go //p' "$REPO_ROOT/go.mod" | head -1
}

# ---------------------------------------------------------------------------
# Paths (AC-6/AC-7)
# ---------------------------------------------------------------------------
init_paths() {
  case "$DEPLOY_OS" in
    linux)
      [ -n "$INSTALL_DIR" ] || INSTALL_DIR="/usr/local/bin"
      [ -n "$CONFIG_FILE" ] || CONFIG_FILE="/etc/huddlescope/huddlescope.env"
      LOG_DIR="/var/log/huddlescope"
      STATE_DIR="/var/lib/huddlescope"
      CONF_DIR="/etc/huddlescope"
      PLIST=""
      ;;
    darwin)
      [ -n "$INSTALL_DIR" ] || INSTALL_DIR="/opt/huddlescope"
      [ -n "$CONFIG_FILE" ] || CONFIG_FILE="$INSTALL_DIR/huddlescope.env"
      # Logs live alongside the binary so a --install-dir override (e.g. a
      # dev machine / temp dir) never forces a system path or root.
      LOG_DIR="$INSTALL_DIR/logs"
      STATE_DIR=""
      CONF_DIR=""
      PLIST="$REPO_ROOT/deploy/launchd/com.huddlescope.ingest.plist"
      ;;
  esac
  BIN="$INSTALL_DIR/huddlescope"

  if [ "$DRY_RUN" -eq 0 ]; then
    # Tee stdout+stderr to the deploy log once the log dir is writable.
    # (0640 log; the script never prints env-file contents.)
    if mkdir -p "$LOG_DIR" 2>/dev/null && [ -w "$LOG_DIR" ]; then
      # Create/touch the log at 0640 before the tee redirect opens it, so the
      # permissions never sit at the umask default even momentarily.
      touch "$LOG_DIR/deploy.log" 2>/dev/null && chmod 0640 "$LOG_DIR/deploy.log" 2>/dev/null || true
      exec > >(tee -a "$LOG_DIR/deploy.log") 2>&1
    fi
  fi
}

# ---------------------------------------------------------------------------
# AC-2: resolve version (explicit X.Y.Z / vX.Y.Z, or highest semver tag).
# resolve-version.sh is deliberately NOT called: it resolves *build* versions
# from a tag context; the deployer takes `latest` or an explicit tag.
# ---------------------------------------------------------------------------
resolve_version() {
  local v="$DEPLOY_VERSION"
  if [ "$v" != "latest" ]; then
    v="${v#v}"
  fi

  if [ "$v" = "latest" ]; then
    v=""
    # For local-build / pre-built-binary sources, "latest" means the local
    # HEAD (a build has no release to download), so resolve from `git
    # describe` like scripts/resolve-version.sh — no network needed. For the
    # release source, the highest semver tag comes from the remote.
    if [ "$SOURCE" = "build" ] || [ "$SOURCE" = "binary" ]; then
      if command -v git >/dev/null 2>&1 && git -C "$REPO_ROOT" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
        v="$(git -C "$REPO_ROOT" describe --tags --abbrev=0 2>/dev/null | sed 's/^v//')" || v=""
        if [ -z "$v" ]; then
          v="$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null | sed 's/^/sha-/')" || v=""
        fi
      fi
    else
      if command -v git >/dev/null 2>&1; then
        v="$(git ls-remote --tags "https://github.com/$DEPLOY_REPO.git" 2>/dev/null \
              | awk '{print $2}' \
              | sed 's|refs/tags/||' \
              | grep -E '^v[0-9]+(\.[0-9]+){2}$' \
              | sort -t. -k1,1n -k2,2n -k3,3n \
              | tail -1)" || v=""
        v="${v#v}"
      fi
    fi
    if [ -z "$v" ]; then
      if [ "$SOURCE" = "release" ]; then
        die "could not resolve latest release version for $DEPLOY_REPO (offline?); pass --version X.Y.Z"
      fi
      die "could not resolve a local version (no git tags / no HEAD); pass --version X.Y.Z"
    fi
  fi

  # Release source requires a strict semver (matches a real release tag).
  # Build/binary sources accept semver or a sha- fallback.
  if [ "$SOURCE" = "release" ]; then
    if ! printf '%s' "$v" | grep -Eq '^[0-9]+(\.[0-9]+){2}$'; then
      die "invalid version '$DEPLOY_VERSION' (expected X.Y.Z, e.g. 1.2.3)"
    fi
  fi

  DEPLOY_VERSION="$v"
  TAG="v$DEPLOY_VERSION"
  log "resolved version $TAG (source: $SOURCE)"
}

# ---------------------------------------------------------------------------
# AC-3: decide acquisition source
# ---------------------------------------------------------------------------
decide_source() {
  if [ -n "$BINARY" ]; then
    SOURCE="binary"
    return
  fi
  if [ "$SOURCE" = "auto" ]; then
    if [ "$DEPLOY_OS" = "linux" ]; then
      SOURCE="release"
    else
      SOURCE="build"
    fi
  fi
  case "$SOURCE" in
    release|build) : ;;
    *) die "invalid --source '$SOURCE' (expected release|build|auto)" ;;
  esac
  log "binary source: $SOURCE"
}

# ---------------------------------------------------------------------------
# AC-2/AC-3: acquire the binary into $STAGE_DIR/huddlescope
# ---------------------------------------------------------------------------
acquire() {
  if [ "$DRY_RUN" -eq 0 ]; then
    STAGE_DIR="$(mktemp -d)"
  fi

  if [ "$SOURCE" = "binary" ]; then
    [ -f "$BINARY" ] || die "--binary file not found: $BINARY"
    if [ "$DRY_RUN" -eq 0 ]; then
      cp "$BINARY" "$STAGE_DIR/huddlescope"
    fi
    log "staged pre-built binary from $BINARY"
    return
  fi

  if [ "$SOURCE" = "build" ]; then
    build_local
    return
  fi

  # SOURCE=release (linux only — darwin has no release assets)
  if [ "$DEPLOY_OS" != "linux" ]; then
    die "no release assets exist for $DEPLOY_OS (releases are Linux-only, issue #161 AC-4); use --source build"
  fi
  download_release
}

build_local() {
  command -v go >/dev/null 2>&1 || die "go toolchain not found; install Go >= $(go_mod_version) or use --source release (linux) / --binary"
  [ -f "$REPO_ROOT/go.mod" ] || die "go.mod not found under $REPO_ROOT; local build needs the repository checkout"

  local goos goarch goarm
  set -- $(path_for "$DEPLOY_OS/$DEPLOY_ARCH")
  goos="$1"; goarch="$2"; goarm="${3:-}"

  local commit
  commit="$(cd "$REPO_ROOT" && git rev-parse --short HEAD 2>/dev/null || echo unknown)"
  local built_at
  built_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  local ldflags
  ldflags="-s -w -X github.com/HuddleScope/huddlescope.github.io/internal/cli.cliVersion=$DEPLOY_VERSION -X github.com/HuddleScope/huddlescope.github.io/internal/cli.cliCommit=$commit -X github.com/HuddleScope/huddlescope.github.io/internal/cli.cliBuiltAt=$built_at"

  if [ "$DRY_RUN" -eq 1 ]; then
    log "would build: CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch${goarm:+ GOARM=$goarm} go build -trimpath -ldflags=\"$ldflags\" -o <staging>/huddlescope ./cmd/huddlescope"
    DEPLOY_SHA256="none (dry-run)"
    return
  fi

  log "building huddlescope $DEPLOY_VERSION ($goos/$goarch) from $REPO_ROOT"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" GOARM="${goarm:-}" \
    go build -trimpath -ldflags="$ldflags" -o "$STAGE_DIR/huddlescope" ./cmd/huddlescope
  log "built local binary ($goos/$goarch)"
}

download_release() {
  local asset="huddlescope_${DEPLOY_VERSION}_linux_${DEPLOY_ARCH}"
  local asset_url="https://github.com/$DEPLOY_REPO/releases/download/$TAG/$asset"
  local sums_url="https://github.com/$DEPLOY_REPO/releases/download/$TAG/SHA256SUMS"

  if [ "$DRY_RUN" -eq 1 ]; then
    log "would download $asset_url (verify via $sums_url)"
    DEPLOY_SHA256="none (dry-run)"
    return
  fi

  log "downloading $asset from $TAG"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL --proto =https --retry 3 -o "$STAGE_DIR/$asset" "$asset_url" \
      || die "download failed for $asset_url"
    curl -fsSL --proto =https --retry 3 -o "$STAGE_DIR/SHA256SUMS" "$sums_url" \
      || die "download failed for $sums_url"
  elif command -v gh >/dev/null 2>&1; then
    gh release download "$TAG" -p "$asset" -p "SHA256SUMS" --dir "$STAGE_DIR" \
      || die "gh release download failed for $TAG"
  else
    die "neither curl nor gh available to download release assets"
  fi

  [ -f "$STAGE_DIR/$asset" ] || die "asset $asset was not downloaded"
  mv "$STAGE_DIR/$asset" "$STAGE_DIR/huddlescope"
  chmod 0755 "$STAGE_DIR/huddlescope"
}

# ---------------------------------------------------------------------------
# AC-4: verify SHA256 (fail-closed before any swap)
# ---------------------------------------------------------------------------
sha_of() {
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  elif command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    die "neither shasum nor sha256sum available"
  fi
}

verify() {
  if [ "$DRY_RUN" -eq 1 ]; then
    log "would verify sha256 of staged binary"
    return
  fi
  [ -f "$STAGE_DIR/huddlescope" ] || die "staged binary missing"

  if [ "$SOURCE" = "build" ] || [ "$SOURCE" = "binary" ]; then
    DEPLOY_SHA256="$(sha_of "$STAGE_DIR/huddlescope")"
    log "staged binary sha256=$DEPLOY_SHA256 (self-computed; no manifest for build/binary source)"
    return
  fi

  [ -f "$STAGE_DIR/SHA256SUMS" ] || die "SHA256SUMS manifest missing; refusing to install unverified binary"
  local expected
  expected="$(grep -E '^[0-9a-f]{64}[[:space:]]+.*huddlescope' "$STAGE_DIR/SHA256SUMS" | awk '{print $1}' | head -1)"
  [ -n "$expected" ] || die "no huddlescope entry in SHA256SUMS"
  DEPLOY_SHA256="$(sha_of "$STAGE_DIR/huddlescope")"
  if [ "$DEPLOY_SHA256" != "$expected" ]; then
    die "checksum mismatch: got $DEPLOY_SHA256 expected $expected"
  fi
  log "checksum verified: $DEPLOY_SHA256"
}

# ---------------------------------------------------------------------------
# AC-6: service user + directories (linux)
# ---------------------------------------------------------------------------
install_user_and_dirs() {
  if [ "$DRY_RUN" -eq 1 ]; then
    log "would create service user $SERVICE_USER and dirs ${CONF_DIR:-.} ${STATE_DIR:-.} $LOG_DIR"
    return
  fi
  if [ "$DEPLOY_OS" = "linux" ]; then
    if ! id "$SERVICE_USER" >/dev/null 2>&1; then
      useradd --system --no-create-home --shell /usr/sbin/nologin "$SERVICE_USER"
      log "created system user $SERVICE_USER"
    fi
    mkdir -p "$CONF_DIR" "$STATE_DIR" "$LOG_DIR"
    chown "$SERVICE_USER:$SERVICE_USER" "$STATE_DIR" "$LOG_DIR"
    chmod 0755 "$CONF_DIR" "$STATE_DIR"
  else
    mkdir -p "$LOG_DIR"
  fi
}

# ---------------------------------------------------------------------------
# AC-5/AC-7/AC-8: atomic binary install (idempotent skip on checksum match)
# ---------------------------------------------------------------------------
install_binary() {
  if [ "$DRY_RUN" -eq 1 ]; then
    log "would install $BIN (atomic swap via ${BIN}.new)"
    return
  fi

  # Idempotent skip (AC-5): existing binary's checksum equals the target's.
  if [ -f "$BIN" ] && [ -n "$DEPLOY_SHA256" ]; then
    local existing
    existing="$(sha_of "$BIN" 2>/dev/null || echo none)"
    if [ "$existing" = "$DEPLOY_SHA256" ]; then
      log "checksum matches existing binary at $BIN ($DEPLOY_SHA256); skipping install"
      INSTALL_SKIPPED=1
      return
    fi
  fi
  INSTALL_SKIPPED=0

  mkdir -p "$INSTALL_DIR"
  # AC-13 rollback point: keep the previous binary for restore.
  if [ -f "$BIN" ]; then
    cp -p "$BIN" "${BIN}.prev"
  fi

  mv "$STAGE_DIR/huddlescope" "${BIN}.new"
  chmod 0755 "${BIN}.new"
  mv "${BIN}.new" "$BIN"
  log "installed binary to $BIN (0755)"
}

# ---------------------------------------------------------------------------
# AC-9: install init units
# ---------------------------------------------------------------------------
install_units() {
  if [ "$NO_SERVICE" -eq 1 ]; then
    log "skipping service install (--no-service)"
    return
  fi
  if [ "$DRY_RUN" -eq 1 ]; then
    if [ "$DEPLOY_OS" = "linux" ]; then
      log "would install /etc/systemd/system/huddlescope-ingest.{service,timer} + daemon-reload + enable --now timer"
    else
      log "would install launchd agent com.huddlescope.ingest (user LaunchAgent)"
    fi
    return
  fi

  if [ "$DEPLOY_OS" = "linux" ]; then
    # R6: stop a possibly in-flight run before (re)installing units.
    systemctl stop huddlescope-ingest.service 2>/dev/null || true
    install -m 0644 "$REPO_ROOT/deploy/systemd/huddlescope-ingest.service" /etc/systemd/system/huddlescope-ingest.service
    install -m 0644 "$REPO_ROOT/deploy/systemd/huddlescope-ingest.timer" /etc/systemd/system/huddlescope-ingest.timer
    systemctl daemon-reload
    systemctl enable huddlescope-ingest.timer
    log "systemd units installed"
  else
    [ -f "$PLIST" ] || die "plist source not found: $PLIST"
    local agents_dir
    agents_dir="$HOME/Library/LaunchAgents"
    mkdir -p "$agents_dir"
    # The committed plist ships the canonical path; rewrite only the binary
    # path to the actual Darwin install location before loading.
    local tmp_plist
    tmp_plist="$(mktemp)"
    sed "s|/opt/huddlescope/huddlescope|$BIN|g" "$PLIST" > "$tmp_plist"
    mv "$tmp_plist" "$agents_dir/com.huddlescope.ingest.plist"
    launchctl unload "$agents_dir/com.huddlescope.ingest.plist" 2>/dev/null || true
    launchctl load "$agents_dir/com.huddlescope.ingest.plist"
    log "launchd agent installed and loaded"
  fi
}

# ---------------------------------------------------------------------------
# AC-10: config check before activation (fail -> abort before enable/restart)
# ---------------------------------------------------------------------------
config_check() {
  if [ "$DRY_RUN" -eq 1 ]; then
    log "would run $BIN config check --config $CONFIG_FILE"
    return
  fi
  [ -f "$CONFIG_FILE" ] || die "config file not found: $CONFIG_FILE (install it first, see docs/systemd-deployment.md §3)"
  if [ "$DEPLOY_OS" = "linux" ]; then
    sudo -u "$SERVICE_USER" "$BIN" config check --config "$CONFIG_FILE" \
      || die "config check failed for $CONFIG_FILE (aborting before service activation)"
  else
    "$BIN" config check --config "$CONFIG_FILE" \
      || die "config check failed for $CONFIG_FILE (aborting before service activation)"
  fi
  log "config check passed"
}

# ---------------------------------------------------------------------------
# AC-11: first ingest run (via service on linux, direct exec on darwin)
# ---------------------------------------------------------------------------
first_run() {
  if [ "$DRY_RUN" -eq 1 ]; then
    log "would run initial ingest: ingest run --mode primary --stale-after 75m"
    return
  fi
  if [ "$DEPLOY_OS" = "linux" ]; then
    if [ "$NO_SERVICE" -eq 1 ]; then
      sudo -u "$SERVICE_USER" "$BIN" ingest run --mode primary --stale-after 75m \
        || die "initial ingest run failed"
      log "initial ingest run (direct, service user) succeeded"
    else
      # Trigger the oneshot via systemd: runs as the service user, not root.
      systemctl start huddlescope-ingest.service \
        || die "initial ingest service start failed"
      log "initial ingest service start succeeded"
    fi
  else
    # launchd: user agent — run directly as the current (non-root) user.
    "$BIN" ingest run --mode primary --stale-after 75m \
      || die "initial ingest run failed"
    log "initial ingest run (darwin direct) succeeded"
  fi
}

# ---------------------------------------------------------------------------
# AC-12: verify service active after deploy
# ---------------------------------------------------------------------------
verify_service() {
  if [ "$DRY_RUN" -eq 1 ]; then
    log "would verify service state"
    return
  fi
  if [ "$NO_SERVICE" -eq 1 ]; then
    log "skipping service verification (--no-service)"
    return
  fi
  if [ "$DEPLOY_OS" = "linux" ]; then
    # The timer is the durable unit; the oneshot service is transient by
    # design (Type=oneshot), so is-active is checked on the timer.
    local timer_state
    timer_state="$(systemctl is-active huddlescope-ingest.timer 2>/dev/null || true)"
    [ "$timer_state" = "active" ] || die "timer not active (state: ${timer_state:-unknown})"
    log "systemd timer active"
  else
    local label
    label="com.huddlescope.ingest"
    launchctl list "$label" >/dev/null 2>&1 || die "launchd agent $label not loaded"
    log "launchd agent loaded"
  fi
}

# ---------------------------------------------------------------------------
# AC-13: rollback — restore the previous binary after a failed post-swap phase
# (called explicitly from each post-swap phase below via `|| { rollback; die }`).
# ---------------------------------------------------------------------------
rollback() {
  if [ -n "${BIN:-}" ] && [ -f "${BIN}.prev" ]; then
    mv "${BIN}.prev" "$BIN"
    echo "rolled back to previous binary" >&2
  fi
}

# ---------------------------------------------------------------------------
# AC-16: record installed checksum (written last, after the swap succeeds)
# ---------------------------------------------------------------------------
record_checksum() {
  if [ "$DRY_RUN" -eq 1 ]; then
    log "would record checksum to $CONFIG_FILE (HSC_INSTALLED_SHA256) and installed.sha256"
    return
  fi
  # Record the checksum of what is actually on disk (AC-16).
  DEPLOY_SHA256="$(sha_of "$BIN")"
  [ -n "$DEPLOY_SHA256" ] || die "could not compute checksum of installed binary"

  if [ -f "$CONFIG_FILE" ]; then
    if grep -q '^HSC_INSTALLED_SHA256=' "$CONFIG_FILE"; then
      local tmp
      tmp="$(mktemp)"
      sed "s|^HSC_INSTALLED_SHA256=.*|HSC_INSTALLED_SHA256=$DEPLOY_SHA256|" "$CONFIG_FILE" > "$tmp"
      cat "$tmp" > "$CONFIG_FILE"
      rm -f "$tmp"
    else
      printf '\n# Set by scripts/deploy.sh; ignored by the huddlescope config loader (permissive env tier).\nHSC_INSTALLED_SHA256=%s\n' "$DEPLOY_SHA256" >> "$CONFIG_FILE"
    fi
  fi

  if [ "$DEPLOY_OS" = "linux" ]; then
    printf '%s\n' "$DEPLOY_SHA256" > "$STATE_DIR/installed.sha256"
    chown "$SERVICE_USER:$SERVICE_USER" "$STATE_DIR/installed.sha256" 2>/dev/null || true
    chmod 0640 "$STATE_DIR/installed.sha256"
    log "recorded checksum $DEPLOY_SHA256 to $STATE_DIR/installed.sha256"
  else
    printf '%s\n' "$DEPLOY_SHA256" > "$INSTALL_DIR/installed.sha256"
    chmod 0640 "$INSTALL_DIR/installed.sha256"
    log "recorded checksum $DEPLOY_SHA256 to $INSTALL_DIR/installed.sha256"
  fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
detect
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
init_paths
decide_source
resolve_version
acquire
verify
install_user_and_dirs
install_binary
# Post-swap phases: any failure restores the previous binary (AC-13).
install_units   || { rollback; die "unit installation failed"; }
config_check    || { rollback; die "config check phase failed"; }
first_run       || { rollback; die "first run phase failed"; }
verify_service  || { rollback; die "service verification failed"; }
record_checksum || { rollback; die "checksum recording failed"; }

DEPLOY_STATUS="success"
log "deploy complete: $BIN ($DEPLOY_OS/$DEPLOY_ARCH, $TAG)"
exit 0
