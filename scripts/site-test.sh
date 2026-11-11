#!/bin/bash
# scripts/site-test.sh — HS-014: Bootstrap Hugo and responsive site design
# Verify: success, failure, idempotency of site build

set -euo pipefail

cd "$(dirname "$0")/.."

WEB_DIR="web"
BUILD_DIR="web/public"
TEST_FIXTURES_DIR="web/fixtures"
LOG_FILE="web/site-test.log"

cleanup() {
  rm -rf "$BUILD_DIR" "$TEST_FIXTURES_DIR" 2>/dev/null || true
  rm -f "$LOG_FILE" 2>/dev/null || true
}
trap cleanup EXIT

log() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG_FILE"
}

# Fail if hugo not available
if ! command -v hugo &> /dev/null; then
  echo "ERROR: hugo not found in PATH"
  exit 1
fi

log "=== Site build verification started ==="
log "Hugo version: $(hugo version)"

# =============================================================================
# 1. SUCCESS: Build should succeed with fixtures
# =============================================================================
log "TEST: success path"

# Create fixtures dir for testing
mkdir -p "$TEST_FIXTURES_DIR"

# Simple fixture: minimal valid content
echo "# Test story" > "$TEST_FIXTURES_DIR/test-story.md"

# Build normally
# Hugo outputs to 'public/' relative to -s; web/public/ in our case
if ! hugo -s "$WEB_DIR" > "$LOG_FILE" 2>&1; then
  log "FAIL: hugo build failed"
  cat "$LOG_FILE"
  exit 1
fi

# Verify output structure
if [[ ! -f "$WEB_DIR/public/index.html" ]]; then
  log "FAIL: index.html not found"
  exit 1
fi

if [[ ! -d "$WEB_DIR/public/stories" ]]; then
  log "FAIL: stories/ directory not created"
  exit 1
fi

if [[ ! -d "$WEB_DIR/public/sources" ]]; then
  log "FAIL: sources/ directory not created"
  exit 1
fi

# Verify no external tracking scripts in built HTML
if grep -qE '(https?://.*\.js|analytics|gtag|google-analytics|facebook\.com/tr|pixel\.facebook)' "$WEB_DIR/public/index.html" 2>/dev/null; then
  log "FAIL: external tracking script found in index.html"
  grep -E '(https?://.*\.js|analytics|gtag|google-analytics|facebook\.com/tr|pixel\.facebook)' "$WEB_DIR/public/index.html" || true
  exit 1
fi

# Verify at least one story page exists
if [[ ! -f "$WEB_DIR/public/stories/index.html" ]]; then
  log "FAIL: stories/index.html not found"
  exit 1
fi

# Verify at least one source page exists
if [[ ! -f "$WEB_DIR/public/sources/index.html" ]]; then
  log "FAIL: sources/index.html not found"
  exit 1
fi

log "PASS: success path"
log "  - index.html built"
log "  - stories/ directory present"
log "  - sources/ directory present"
log "  - no external tracking scripts"
log "  - story/source pages generated"

# =============================================================================
# 2. FAILURE: Missing directory should produce non-zero exit
# =============================================================================
log "TEST: failure path"

# Use a non-existent source directory
if hugo -s "$WEB_DIR/nonexistent" > /dev/null 2>&1; then
  log "FAIL: hugo should have failed with missing source"
  exit 1
fi

log "PASS: failure path (missing source directory rejected)"

# =============================================================================
# 3. IDEMPOTENCY: Build twice, outputs should be identical
# =============================================================================
log "TEST: idempotency"

# Clean build
rm -rf "$WEB_DIR/public"

# First build
hugo -s "$WEB_DIR" > /dev/null 2>&1
# Capture a checksum of a known file
FIRST_SUM=$(shasum "$WEB_DIR/public/index.html" | cut -d' ' -f1)

# Second build
rm -rf "$WEB_DIR/public"
hugo -s "$WEB_DIR" > /dev/null 2>&1
SECOND_SUM=$(shasum "$WEB_DIR/public/index.html" | cut -d' ' -f1)

if [[ "$FIRST_SUM" != "$SECOND_SUM" ]]; then
  log "FAIL: outputs differ between builds"
  log "  First:  $FIRST_SUM"
  log "  Second: $SECOND_SUM"
  exit 1
fi

log "PASS: idempotency path (outputs identical across builds)"

# =============================================================================
# SUMMARY
# =============================================================================
log "=== All site tests passed ==="
exit 0