#!/bin/bash
# coverage-check.sh: Enforce >=80% coverage on core internal packages, >=70% on others
# Usage: ./scripts/coverage-check.sh
# Exit codes: 0=pass, 1=fail

set -e

# Core packages that require >=80% coverage (per HS-020 AC5)
CORE_PKGS=(
	"github.com/HuddleScope/huddlescope.github.io/internal/application"
	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

# All internal packages
ALL_PKGS=$(go list ./internal/...)

echo "Checking coverage of internal packages..."

# First check core packages with 80% threshold
for pkg in "${CORE_PKGS[@]}"; do
  # Force clean test cache for accurate coverage
  go clean -testcache >/dev/null 2>&1
  go test -coverprofile=/tmp/cover.out "$pkg" >/dev/null 2>&1 || { echo "FAIL: $pkg - test failed"; exit 1; }
  # Extract percentage: "ok pkg ... coverage: XX.X%" -> XX.X
  pct=$(go tool cover -func=/tmp/cover.out 2>/dev/null | grep "total:" | awk '{print $3}' | tr -d '%')
  if [ -z "$pct" ]; then
    echo "FAIL: $pkg - no coverage data"
    exit 1
  fi
  if awk "BEGIN {exit !($pct < 80)}"; then
    echo "FAIL: $pkg has $pct% coverage (< 80%)"
    exit 1
  fi
  echo "OK: $pkg has $pct% coverage (>= 80%)"
done

# Then check remaining packages with 70% threshold
for pkg in $ALL_PKGS; do
  # Skip core packages (already checked above)
  for core in "${CORE_PKGS[@]}"; do
    if [ "$pkg" = "$core" ]; then
      continue 2
    fi
  done

  # Test-only packages (no non-test .go files) have no production
  # statements to measure, so no percentage gate applies to them.
  if [ -z "$(go list -f '{{if .GoFiles}}has-production-go{{end}}' "$pkg")" ]; then
    echo "SKIP: $pkg (test-only package, no production statements)"
    continue
  fi

  go clean -testcache >/dev/null 2>&1
  go test -coverprofile=/tmp/cover.out "$pkg" >/dev/null 2>&1 || { echo "FAIL: $pkg - test failed"; exit 1; }
  pct=$(go tool cover -func=/tmp/cover.out 2>/dev/null | grep "total:" | awk '{print $3}' | tr -d '%')
  if awk "BEGIN {exit !($pct < 70)}"; then
    echo "FAIL: $pkg has $pct% coverage (< 70%)"
    exit 1
  fi
  echo "OK: $pkg has $pct% coverage (>= 70%)"
done

echo "Coverage gate passed (core >= 80%, other internal >= 70%)"