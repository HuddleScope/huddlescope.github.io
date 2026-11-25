#!/usr/bin/env bash
# resolve-version.sh: Single source of truth for the release version string.
#
# Used by .github/workflows/build-and-release.yaml (issue #203) and, going
# forward, by the Docker image workflow (issue #204) so that both workflows
# resolve the exact same version for a given run. Keep the resolution logic
# here, not inline in a workflow file, so the two workflows cannot drift.
#
# Resolution order (deterministic, identical across every job in a run):
#   1. Tag push          -> strip the leading "v" from GITHUB_REF
#                           (e.g. refs/tags/v0.2.0 -> 0.2.0)
#   2. Any other context -> `git describe --tags --abbrev=0` (strip "v"),
#                           e.g. refs/heads/main -> last reachable tag
#   3. No tags at all    -> "sha-" + first 7 chars of GITHUB_SHA
#
# Output: the version string on stdout, nothing else.
# Exit codes: 0 on success, 1 on any failure.
#
# Locally (no GITHUB_* env vars) the script falls through to the git describe
# path, which makes it testable outside GitHub Actions.

set -euo pipefail

strip_v() {
  # Remove a single leading "v" if present; pass through otherwise.
  local v="$1"
  if [[ "$v" == v* ]]; then
    printf '%s' "${v#v}"
  else
    printf '%s' "$v"
  fi
}

# 1) Tag push: extract the version straight from the ref.
if [[ "${GITHUB_EVENT_NAME:-}" == "push" && "${GITHUB_REF:-}" == refs/tags/* ]]; then
  tag="${GITHUB_REF#refs/tags/}"
  stripped="$(strip_v "$tag")"
  if [[ -n "$stripped" ]]; then
    printf '%s\n' "$stripped"
    exit 0
  fi
fi

# 2) Fall back to the last reachable tag on HEAD.
if command -v git >/dev/null 2>&1 && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  if describe="$(git describe --tags --abbrev=0 2>/dev/null)"; then
    stripped="$(strip_v "$describe")"
    if [[ -n "$stripped" ]]; then
      printf '%s\n' "$stripped"
      exit 0
    fi
  fi
fi

# 3) No usable tag: use the commit SHA fallback.
if [[ -n "${GITHUB_SHA:-}" ]]; then
  sha7="$(printf '%s' "$GITHUB_SHA" | cut -c1-7)"
  printf 'sha-%s\n' "$sha7"
  exit 0
fi

# Last resort (local, no GITHUB_SHA): use the short HEAD SHA if available.
if command -v git >/dev/null 2>&1 && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  sha7="$(git rev-parse --short HEAD 2>/dev/null || true)"
  if [[ -n "$sha7" ]]; then
    printf 'sha-%s\n' "$sha7"
    exit 0
  fi
fi

echo "ERROR: could not resolve a version (no tag, no GITHUB_SHA, no git HEAD)" >&2
exit 1
