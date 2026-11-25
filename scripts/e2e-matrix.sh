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
#   - No TURSO_* / GITHUB_* env reaches the tested binary (the binary tests run
#     the CLI in a hermetic env that excludes every HSC_* / TURSO_* / GITHUB_*
#     variable).
#   - The published site-data directory is a temp dir (HSC_DATA_DIR), never
#     web/data. No external feeds: sources point exclusively at the fixture server.
#
# The matrix is offline by construction; this script makes that explicit by
# scrubbing any accidental TURSO/GITHUB env from the matrix environment.
set -euo pipefail

cd "$(dirname "$0")/.."

# Offline invariant: the matrix environment must be free of credentials and of
# GitHub context. GitHub Actions sets GITHUB_REPOSITORY (and friends) on every
# job as non-secret repo context, so the guard SCRUBS these vars instead of
# failing — failing on GITHUB_REPOSITORY broke the required "Build" check for
# any run hosted on GitHub Actions. Safety is preserved two ways: this scrub
# removes the vars from the matrix process env, and the suites themselves exec
# the CLI in a fully hermetic env (no HSC_*/TURSO_*/GITHUB_* variables) against
# t.TempDir()-scoped sqlite DBs and a 127.0.0.1-only fixture server, so no
# external service or real credential is reachable regardless of the host env.
scrubbed=""
for var in TURSO_DATABASE_URL TURSO_AUTH_TOKEN GITHUB_TOKEN GITHUB_REPOSITORY; do
  if [ -n "${!var:-}" ]; then
    scrubbed="$scrubbed $var"
    unset "$var"
  fi
done
if [ -n "$scrubbed" ]; then
  echo "    scrubbed from matrix env:$scrubbed"
fi

# Self-test mode: verify the offline guard (env scrubbing) without running the
# suites. Exercised by internal/e2e/matrix_script_test.go.
if [ "${E2E_MATRIX_SELFTEST:-}" = "1" ]; then
  for var in TURSO_DATABASE_URL TURSO_AUTH_TOKEN GITHUB_TOKEN GITHUB_REPOSITORY; do
    if [ -n "${!var:-}" ]; then
      echo "ERROR: self-test failed, $var still set after scrub" >&2
      exit 1
    fi
  done
  echo "e2e-matrix self-test: PASS (env scrubbed)"
  exit 0
fi

echo "==> e2e matrix: building ./cmd/huddlescope (offline)"
CGO_ENABLED=0 go build -trimpath -o dist/huddlescope-e2e ./cmd/huddlescope
echo "    built dist/huddlescope-e2e"

echo "==> e2e matrix: running binary + failover suites (-tags e2e, -race, -count=1)"
# -tags e2e compiles BOTH binary_test.go (no constraint) and failover_test.go
# (e2e constraint). -count=1 busts the test cache so the matrix always re-runs.
go test ./internal/e2e/ -tags e2e -race -count=1 -v

echo "==> e2e matrix: PASS (offline; no external services contacted)"
