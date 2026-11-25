#!/usr/bin/env bash
# scripts/e2e-matrix.sh — offline end-to-end matrix (issue #165 / HS-PORT-09).
#
# Runs the two e2e suites in internal/e2e:
#   1. binary_test.go  — builds ./cmd/huddlescope (TestMain) and execs the CLI
#      commands against a 127.0.0.1 fixture RSS server + a local sqlite-file DB.
#   2. failover_test.go (build tag `e2e`) — the 8 failover scenarios against the
#      real sqlite-file backend + shared ingest lease.
#
# The failover suite is gated behind `-tags e2e` so `make test` / `go test ./...`
# do NOT run it; this target is the only path that does.
#
# Offline invariants (AC-9):
#   - DBs live in t.TempDir(); the fixture server binds 127.0.0.1 only.
#   - No TURSO_* / GITHUB_* env is present (the binary tests run the CLI in a
#     hermetic env that excludes every HSC_* / TURSO_* / GITHUB_* variable).
#   - The published site-data directory is a temp dir (HSC_DATA_DIR), never
#     web/data. No external feeds: sources point exclusively at the fixture server.
#
# The matrix is offline by construction; this script makes that explicit and
# refuses to run with any accidental TURSO/GITHUB env present.
set -euo pipefail

cd "$(dirname "$0")/.."

# Guard: the e2e matrix must run offline. If a host TURSO_* or GITHUB_* env var
# is present, fail loudly rather than risk a test leaking a real credential or
# hitting a real service. The matrix itself isolates the binary process env, but
# this guard catches operator misconfiguration (e.g., running make e2e on a host
# with TURSO_* exported).
leaked=""
while IFS= read -r var; do
  if [ -n "${!var:-}" ]; then
    leaked="$leaked $var"
  fi
done <<'ENV'
TURSO_DATABASE_URL
TURSO_AUTH_TOKEN
GITHUB_TOKEN
GITHUB_REPOSITORY
ENV
if [ -n "$leaked" ]; then
  echo "ERROR: e2e matrix must run offline, but host env vars are set:$leaked" >&2
  echo "       Unset them (or run in a clean shell) and retry." >&2
  exit 1
fi

echo "==> e2e matrix: building ./cmd/huddlescope (offline)"
CGO_ENABLED=0 go build -trimpath -o dist/huddlescope-e2e ./cmd/huddlescope
echo "    built dist/huddlescope-e2e"

echo "==> e2e matrix: running binary + failover suites (-tags e2e, -race, -count=1)"
# -tags e2e compiles BOTH binary_test.go (no constraint) and failover_test.go
# (e2e constraint). -count=1 busts the test cache so the matrix always re-runs.
go test ./internal/e2e/ -tags e2e -race -count=1 -v

echo "==> e2e matrix: PASS (offline; no external services contacted)"
