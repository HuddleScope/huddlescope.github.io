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

# HS-016: Verify multi-source story structure exists
if ! find "$WEB_DIR/public" -name "*multi-source*" -o -name "*top-free-agents*" | grep -q .; then
  log "FAIL: multi-source story page not found"
  exit 1
fi

log "PASS: HS-016 multi-source story structure (story page exists)"

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

# =============================================================================
# HS-015: Homepage with grouped stories and source attribution
# =============================================================================

log "TEST: HS-015 homepage assertions"

# HS-015: Verify homepage contains featured section
if ! grep -q 'Featured story' "$WEB_DIR/public/index.html"; then
  log "FAIL: featured story section not found in homepage"
  exit 1
fi

# HS-015: Verify homepage contains ranked/latest sections
if ! grep -q 'Top ranked stories' "$WEB_DIR/public/index.html"; then
  log "FAIL: top ranked stories section not found in homepage"
  exit 1
fi

if ! grep -q 'Latest updates' "$WEB_DIR/public/index.html"; then
  log "FAIL: latest updates section not found in homepage"
  exit 1
fi

# HS-015: Verify timestamp and disclaimer
if ! grep -q 'Data as of' "$WEB_DIR/public/index.html"; then
  log "FAIL: timestamp metadata not found in homepage"
  exit 1
fi

if ! grep -q 'not a live or real-time' "$WEB_DIR/public/index.html"; then
  log "FAIL: real-time disclaimer not found in homepage"
  exit 1
fi

# HS-015: Verify publisher count and coverage list
if ! grep -q 'Covered by' "$WEB_DIR/public/index.html"; then
  log "FAIL: publisher count text not found in homepage"
  exit 1
fi

if ! grep -q 'coverage' "$WEB_DIR/public/index.html"; then
  log "FAIL: coverage list not found in homepage"
  exit 1
fi

# HS-015: Verify evidence badge (at least one confirmed)
if ! grep -q 'story-evidence-confirmed' "$WEB_DIR/public/index.html"; then
  log "FAIL: evidence badge (confirmed) not found in homepage"
  exit 1
fi

# HS-015: Verify disclaimer for rumour/speculation is absent (all confirmed)
if grep -q 'Rumor/speculation' "$WEB_DIR/public/index.html"; then
  log "FAIL: Rumor/speculation disclaimer present but all stories are confirmed"
  exit 1
fi

# HS-015: Verify basic a11y: lang on html, skip link present
if ! grep -q 'lang="' "$WEB_DIR/public/index.html"; then
  log "FAIL: lang attribute not found in <html>"
  exit 1
fi

if ! grep -q 'Skip to main content' "$WEB_DIR/public/index.html"; then
  log "FAIL: skip link not found in homepage"
  exit 1
fi

# HS-015: Verify aria attributes on sections
if ! grep -q 'aria-labelledby' "$WEB_DIR/public/index.html"; then
  log "FAIL: aria-labelledby attributes not found in homepage"
  exit 1
fi

# HS-015: Verify featured story is top-scored (score sort F1 guard)
FEATURED_SCORE=$(grep -o 'class="story-score"[^>]*>[0-9.]*' "$WEB_DIR/public/index.html" | head -1 | sed 's/.*>//')
MAX_SCORE=$(grep -o 'class="story-score"[^>]*>[0-9.]*' "$WEB_DIR/public/index.html" | sed 's/.*>//g' | sort -n | tail -1)
if [ "$FEATURED_SCORE" != "$MAX_SCORE" ]; then
  log "FAIL: featured story score ($FEATURED_SCORE) != max score ($MAX_SCORE)"
  exit 1
fi

log "PASS: HS-015 homepage assertions (featured is top-scored)"

# =============================================================================
# HS-023: Launch verification — accessibility, SEO, sitemap, cache, data
# =============================================================================

log "TEST: HS-023 launch verification"

# HS-023a: sitemap.xml emitted and contains home URL
if [[ ! -f "$WEB_DIR/public/sitemap.xml" ]]; then
  log "FAIL: sitemap.xml not found"
  exit 1
fi
if ! grep -q 'https://huddlescope.github.io/' "$WEB_DIR/public/sitemap.xml"; then
  log "FAIL: sitemap.xml does not contain home URL"
  exit 1
fi
log "PASS: sitemap.xml present and references home"

# HS-023b: robots.txt emitted and references sitemap
if [[ ! -f "$WEB_DIR/public/robots.txt" ]]; then
  log "FAIL: robots.txt not found"
  exit 1
fi
if ! grep -q 'Sitemap:' "$WEB_DIR/public/robots.txt"; then
  log "FAIL: robots.txt does not contain Sitemap directive"
  exit 1
fi
if ! grep -q 'https://huddlescope.github.io/sitemap.xml' "$WEB_DIR/public/robots.txt"; then
  log "FAIL: robots.txt does not reference sitemap.xml with full URL"
  exit 1
fi
log "PASS: robots.txt present and references sitemap"

# HS-023c: 404.html emitted
if [[ ! -f "$WEB_DIR/public/404.html" ]]; then
  log "FAIL: 404.html not found"
  exit 1
fi
log "PASS: 404.html emitted"

# HS-023d: Every HTML page has canonical, description, og:title
for htmlfile in "$WEB_DIR/public"/*.html "$WEB_DIR/public"/*/*.html; do
  [[ -f "$htmlfile" ]] || continue
  if ! grep -q 'rel="canonical"' "$htmlfile"; then
    log "FAIL: $htmlfile missing canonical link"
    exit 1
  fi
  if ! grep -q '<meta name="description"' "$htmlfile"; then
    log "FAIL: $htmlfile missing description meta"
    exit 1
  fi
  if ! grep -q '<meta property="og:title"' "$htmlfile"; then
    log "FAIL: $htmlfile missing og:title meta"
    exit 1
  fi
done
log "PASS: All HTML pages have canonical, description, og:title"

# HS-023e: Internal link scan — every internal href resolves to a file
# Skip script/style tags; resolve directory links to index.html
while IFS= read -r href; do
  [[ -z "$href" ]] && continue
  # Skip external links
  case "$href" in
    http://*|https://*|mailto:*|tel:*|//*) continue ;;
    \#*) continue ;;  # fragment only
    /*) href="${href#/}" ;;  # absolute path within site
  esac
  # Normalize directory → index.html
  target="$WEB_DIR/public/$href"
  if [[ -d "$target" ]]; then
    target="$target/index.html"
  else
    target="$target"
  fi
  # Allow hash-suffixed links to remain in site (anchor links on same page)
  if [[ "$href" == *"#"* ]]; then
    continue
  fi
  if [[ ! -f "$target" ]]; then
    log "FAIL: broken internal link $href (target $target missing)"
    exit 1
  fi
done < <(grep -oE 'href="[^"]*"' "$WEB_DIR/public/index.html" | sed 's/href="\(.*\)"/\1/' | grep -v '^https\?://' | grep -v '^//' | grep -v '^mailto:' | grep -v '^tel:')
log "PASS: No broken internal links on homepage"

# HS-023f: Homepage JSON validation
if ! python3 -c "import json; json.load(open('$WEB_DIR/public/../data/homepage.json'))" 2>/dev/null; then
  log "FAIL: homepage.json is not valid JSON"
  exit 1
fi
if ! jq -e '.version and .generatedAt and (.stories | length > 0)' "$WEB_DIR/public/../data/homepage.json" > /dev/null 2>&1; then
  log "FAIL: homepage.json missing required fields (version, generatedAt, stories)"
  exit 1
fi
# Verify story IDs unique
story_ids=$(jq -r '.stories[].id' "$WEB_DIR/public/../data/homepage.json" | sort)
unique_ids=$(echo "$story_ids" | uniq)
if [[ "$story_ids" != "$unique_ids" ]]; then
  log "FAIL: homepage.json has duplicate story IDs"
  exit 1
fi
log "PASS: homepage.json is valid with unique story IDs"

# HS-023g: Homepage story IDs match content pages (WARN if missing, don't fail)
content_pages=$(find "$WEB_DIR/content/stories" -type f -name '*.md' -exec basename {} .md \;)
json_ids=$(jq -r '.stories[].id' "$WEB_DIR/public/../data/homepage.json")
missing=0
for id in $json_ids; do
  if ! echo "$content_pages" | grep -q "^${id}$"; then
    log "WARN: homepage story ID '$id' has no matching content/stories/$id.md (production may emit fresh IDs)"
    missing=$((missing+1))
  fi
done
if [[ $missing -gt 0 ]]; then
  log "WARN: $missing homepage story IDs without content pages"
fi
log "PASS: story ID to content page check completed (warnings logged above if any)"

# HS-023h: Secret/private-data scan
# Exclude integrity hashes (sha256-...) and hugo cache identifiers
SECRETS_FOUND=$(grep -rnE '(ghp_[a-zA-Z0-9]{36}|gho_[a-zA-Z0-9]{36}|xoxb-[0-9]{12}-[0-9]{12}-[a-zA-Z0-9]{24}|TURSO_AUTH_TOKEN=.*[a-zA-Z0-9]{20,})' "$WEB_DIR/public" "$WEB_DIR/public/../data" 2>/dev/null || true)
if [[ -n "$SECRETS_FOUND" ]]; then
  log "FAIL: possible secret/private data found in web/public or web/data"
  echo "$SECRETS_FOUND"
  exit 1
fi
log "PASS: No obvious secrets/private data in published artifacts"

# HS-023i: Evidence badge assertions (data-driven, replacing hardcoded HS-015 greps)
evidence_levels=$(jq -r '.stories[].evidenceLevel' "$WEB_DIR/public/../data/homepage.json" | sort -u)
for level in $evidence_levels; do
  if [[ "$level" == "unverified" ]]; then
    if ! grep -q 'story-evidence-unverified' "$WEB_DIR/public/index.html"; then
      log "FAIL: unverified evidence badge not rendered"
      exit 1
    fi
    if ! grep -q 'Rumor/speculation' "$WEB_DIR/public/index.html"; then
      log "FAIL: Rumor/speculation disclaimer missing for unverified stories"
      exit 1
    fi
    log "PASS: unverified evidence badge and disclaimer present"
  elif [[ "$level" == "confirmed" ]]; then
    if ! grep -q 'story-evidence-confirmed' "$WEB_DIR/public/index.html"; then
      log "FAIL: confirmed evidence badge not rendered"
      exit 1
    fi
    log "PASS: confirmed evidence badge present"
  fi
done

# SUMMARY
# =============================================================================
log "PASS: HS-023 launch verification (sitemap, robots, 404, metadata, internal links, data, secrets, evidence badges)"

# =============================================================================
# SUMMARY
# =============================================================================
log "=== All site tests passed ==="
exit 0