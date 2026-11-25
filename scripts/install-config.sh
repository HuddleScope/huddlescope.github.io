#!/usr/bin/env bash
#
# install-config.sh — install a HuddleScope deployment config with safe
# permissions (issue #158 / HS-PORT-02).
#
# Usage:
#   scripts/install-config.sh deploy <path/to/deploy.env>
#   scripts/install-config.sh rpi <path/to/rpi.env>
#
# deploy -> /etc/huddlescope/deploy.env
# rpi    -> /opt/huddlescope/huddlescope.env
#
# The target file is written with 0600 permissions (owner read/write only) so
# Turso auth tokens and other secrets are not world-readable.
#
# This script is idempotent: re-running it overwrites the target with the
# provided file's contents.

set -euo pipefail

die() {
  echo "Error: $*" >&2
  exit 1
}

usage() {
  echo "Usage: $0 <deploy|rpi> <path/to/config.env>" >&2
  exit 1
}

[ "$#" -eq 2 ] || usage

kind="$1"
src="$2"

[ -f "$src" ] || die "source config file not found: $src"

case "$kind" in
  deploy)
    dest_dir="/etc/huddlescope"
    dest="$dest_dir/deploy.env"
    ;;
  rpi)
    dest_dir="/opt/huddlescope"
    dest="$dest_dir/huddlescope.env"
    ;;
  *)
    usage
    ;;
esac

# Ensure the destination directory exists (requires write permission; on most
# systems this means running as root or with sudo).
if [ ! -d "$dest_dir" ]; then
  if [ -w "$(dirname "$dest_dir")" ]; then
    mkdir -p "$dest_dir"
  else
    die "cannot create $dest_dir (no write permission on $(dirname "$dest_dir")); run with sudo"
  fi
fi

# Install the file with 0600 permissions.
install -m 0600 "$src" "$dest"

echo "Installed $src -> $dest (mode 0600)"

# Remind the operator about the binary and absolute paths.
case "$kind" in
  deploy)
    echo ""
    echo "Next steps:"
    echo "  1. Install the huddlescope binary (e.g., /usr/local/bin/huddlescope)"
    echo "  2. Verify the config: huddlescope config --check --config $dest"
    echo "  3. Run ingest:        huddlescope ingest run --config $dest"
    ;;
  rpi)
    echo ""
    echo "Next steps:"
    echo "  1. Install the huddlescope binary (e.g., /opt/huddlescope/huddlescope)"
    echo "  2. Copy conf/sources.yaml and conf/teams.yaml to /opt/huddlescope/"
    echo "  3. Verify the config: /opt/huddlescope/huddlescope config --check --config $dest"
    echo "  4. Run ingest:        /opt/huddlescope/huddlescope ingest run --config $dest"
    ;;
esac
