#!/bin/bash
# coverage-check.sh: Enforce >=70% coverage on internal packages
# Usage: ./scripts/coverage-check.sh
# Exit codes: 0=pass, 1=fail

set -e

echo "Checking coverage of internal packages..."
for pkg in $(go list ./internal/...); do
  go test -coverprofile=/tmp/cover.out "$pkg" >/dev/null 2>&1 || { echo "FAIL: $pkg - test failed"; exit 1; }
  pct=$(go tool cover -func=/tmp/cover.out 2>/dev/null | grep "total:" | awk '{print $3}' | tr -d '%')
  if awk "BEGIN {exit !($pct < 70)}"; then
    echo "FAIL: $pkg has $pct% coverage (< 70%)"
    exit 1
  fi
done
echo "Coverage gate passed (all internal packages >= 70%)"
