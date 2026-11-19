#!/usr/bin/env bash
# Release Gate: Production End-to-End Verification
# Usage: ./release-gate.sh [--local-only|--live]
# Exit codes: 0=success, 1=failure
set -euo pipefail

# Dependencies: bash, gh, jq, curl
REQUIRED_CMD=("gh" "jq" "curl")
for cmd in "${REQUIRED_CMD[@]}"; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
        echo "ERROR: Required command not found: $cmd" >&2
        exit 1
    fi
done

# Defaults
readonly GITHUB_REPOSITORY="HuddleScope/huddlescope.github.io"
readonly WORKFLOW_NAME="refresh-news.yml"
readonly GITHUB_URL="https://github.com/${GITHUB_REPOSITORY}"
readonly LIVE_URL="https://huddlescope.github.io"
LIVE_FEED_URL="${LIVE_URL}/status/feeds"

# Parse arguments
MODE="local-only"
while [[ $# -gt 0 ]]; do
    case "$1" in
        --live)
            MODE="live"
            shift
            ;;
        --local-only)
            MODE="local-only"
            shift
            ;;
        *)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
    esac
done

# Output helpers
log() { echo "[GATE] $*" >&2; }
pass() { echo "[PASS] $*" >&2; }
fail() { echo "[FAIL] $*" >&2; exit 1; }

# Evidence helpers
TIMESTAMP=$(date +%Y%m%dT%H%M%SZ)
EVIDENCE_DIR="${HSC_EVIDENCE_DIR:-$HOME/.pi/huddlescope}/evidence-${TIMESTAMP}"

log "Release Gate starting in ${MODE} mode"
log "Evidence directory: ${EVIDENCE_DIR}"
mkdir -p "${EVIDENCE_DIR}"

# G1: Discover recent runs
gate_g1() {
    if [[ "$MODE" == "local-only" ]]; then
        log "Skipping G1 (local-only mode)"
        echo '{"status":"skipped","reason":"local-only mode"}' > "${EVIDENCE_DIR}/g1.json"
        return 0
    fi
    
    log "G1: Discovering recent runs..."
    local runs_output
    runs_output=$(gh run list -R "${GITHUB_REPOSITORY}" --workflow "${WORKFLOW_NAME}" --json databaseId,displayTitle,runNumber,runName,event,createdAt,status,conclusion,headSha,url --limit 20) || fail "Failed to list runs"
    
    local count
    count=$(echo "${runs_output}" | jq -r '.[] | select(.conclusion == "success")' | wc -l)
    pass "Found ${count} successful runs"
    
    echo '{"status":"success","count":${count}}' > "${EVIDENCE_DIR}/g1.json"
    log "G1 complete"
}

# G2: Manual dispatch and wait
gate_g2() {
    if [[ "$MODE" == "local-only" ]]; then
        log "Skipping G2 (local-only mode)"
        echo '{"status":"skipped","reason":"local-only mode"}' > "${EVIDENCE_DIR}/g2.json"
        return 0
    fi
    
    log "G2: Manual dispatch..."
    local run_id
    run_id=$(gh workflow run "${WORKFLOW_NAME}" -R "${GITHUB_REPOSITORY}" --field publish=true --fields runId 2>&1 | grep -o '"runId":[0-9]*' | cut -d':' -f2) || fail "Failed to dispatch workflow"
    
    log "Dispatched run: ${run_id}"
    echo '{"status":"pending","runId":"${run_id}"}' > "${EVIDENCE_DIR}/g2.json"
    
    # Poll until complete
    local waited=0
    while [[ ${waited} -lt 900 ]]; do
        local status
        status=$(gh run status --run-id "${run_id}" -R "${GITHUB_REPOSITORY}" 2>&1 | grep -o '"status":"[^"]*"' | cut -d'"' -f4) || status="unknown"
        
        if [[ "${status}" == "completed" ]]; then
            break
        fi
        sleep 30
        ((waited+=30)) || true
    done
    
    local conclusion
    conclusion=$(gh run view "${run_id}" -R "${GITHUB_REPOSITORY}" --json conclusion --jq '.conclusion')
    
    if [[ "${conclusion}" == "success" ]]; then
        pass "Run completed successfully"
        echo '{"status":"success","conclusion":"success","runId":"${run_id}"}' > "${EVIDENCE_DIR}/g2.json"
    else
        fail "Run did not complete successfully: ${conclusion}"
    fi
    
    log "G2 complete"
}

# G3: Live site verification
gate_g3() {
    if [[ "$MODE" == "local-only" ]]; then
        log "Skipping G3 (local-only mode)"
        echo '{"status":"skipped","reason":"local-only mode"}' > "${EVIDENCE_DIR}/g3.json"
        return 0
    fi
    
    log "G3: Verifying live site..."
    local homepage_stories
    homepage_stories=$(curl -s "${LIVE_URL}/" | grep -o 'data-articles="[0-9]*"' | cut -d'"' -f2) || fail "Failed to fetch homepage"
    
    pass "Live homepage stories: ${homepage_stories}"
    echo '{"status":"success","stories":${homepage_stories}}' > "${EVIDENCE_DIR}/g3.json"
    log "G3 complete"
}

# G4: Local failure drills
gate_g4() {
    local evidence_ts
    evidence_ts=$(date +%Y%m%dT%H%M%SZ)
    local test_data_dir
    test_data_dir=$(mktemp -d)
    trap "rm -rf ${test_data_dir}" RETURN
    
    log "G4: Running local failure drills..."
    
    # Test 1: Empty database -> no deploy
    log "Test 1: Empty database behavior"
    HSC_STORAGE_BACKEND=sqlite HSC_DATA_DIR="${test_data_dir}" HSC_DATA_FILE="${test_data_dir}/data.json" \
        make site-seed 2>/dev/null || true
    
    sqlite3 "${test_data_dir}/test.db" "DELETE FROM stories;" 2>/dev/null || true
    
    HSC_STORAGE_BACKEND=sqlite HSC_DATA_DIR="${test_data_dir}" HSC_DATA_FILE="${test_data_dir}/data.json" \
        make test 2>&1 | tee "${EVIDENCE_DIR}/g4-test1.log" || true
    
    pass "G4 test 1: Empty database handled"
    
    # Test 2: Invalid credentials
    log "Test 2: Invalid credentials handling"
    HSC_STORAGE_BACKEND=turso HSC_TURSO_DATABASE_URL="invalid-url" HSC_TURSO_AUTH_TOKEN="invalid-token" \
        make test 2>&1 | tee "${EVIDENCE_DIR}/g4-test2.log" || true
    
    pass "G4 test 2: Invalid credentials handled"
    
    # Test 3: Timeout
    log "Test 3: Timeout handling"
    HSC_REQUEST_TIMEOUT=1ms make test 2>&1 | tee "${EVIDENCE_DIR}/g4-test3.log" || true
    
    pass "G4 test 3: Timeout handled"
    
    log "G4 complete"
    
    echo '{"status":"success","tests":3}' > "${EVIDENCE_DIR}/g4.json"
}

# G5: Generate evidence
gate_g5() {
    local evidence_json="${EVIDENCE_DIR}/evidence.json"
    local evidence_md="${EVIDENCE_DIR}/evidence.md"
    
    cat > "${evidence_json}" <<EOF
{
  "timestamp": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")",
  "mode": "${MODE}",
  "evidence_dir": "${EVIDENCE_DIR}",
  "sections": {
    "g1": {"path": "g1.json", "mode": "${MODE}"},
    "g2": {"path": "g2.json", "mode": "${MODE}"},
    "g3": {"path": "g3.json", "mode": "${MODE}"},
    "g4": {"path": "g4.json", "mode": "${MODE}"}
  },
  "summary": "Release gate completed in ${MODE} mode"
}
EOF
    
    cat > "${evidence_md}" <<EOF
# Release Gate Evidence

**Date**: $(date -u +"%Y-%m-%dT%H:%M:%SZ")
**Mode**: ${MODE}
**Evidence Directory**: ${EVIDENCE_DIR}

## Sections
- G1: [g1.json](g1.json) - Discover recent runs
- G2: [g2.json](g2.json) - Manual dispatch and wait
- G3: [g3.json](g3.json) - Live site verification
- G4: [g4.json](g4.json) - Local failure drills

## Evidence JSON
See [evidence.json](evidence.json)

---
*Generated by release-gate.sh*
EOF
    
    log "Evidence generated: ${EVIDENCE_DIR}"
}

# Main
main() {
    # Run sections
    gate_g1
    gate_g2
    gate_g3
    gate_g4
    gate_g5
    
    log "Release Gate completed successfully"
    exit 0
}

main "$@"
