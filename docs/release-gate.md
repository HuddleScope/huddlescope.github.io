# Release Gate Runbook: Production End-to-End Verification

This document describes how to execute the production release gate for verifying Cloudflare → GitHub Pages deployments (HS-058).

## Overview

The release gate verifies:
- **AC-1**: Cloudflare cron is observed firing and each dispatched run is discoverable in Actions
- **AC-2**: Two scheduled consecutive runs complete without manual intervention and publish a fresh nonempty snapshot
- **AC-3**: The live homepage and `/status/feeds/` are consistent with Turso and the workflow log
- **AC-4**: Failure scenarios keep the last-good site and produce a visible failure signal
- **AC-5**: Verification evidence (timestamps, counts, run links, deployed commit) is attached to the issue

## Prerequisites

Before running the release gate, ensure:

1. **GitHub CLI is authenticated** with the `HuddleScope/huddlescope.github.io` repository:
   ```bash
   gh auth login
   gh repo view HuddleScope/huddlescope.github.io
   ```

2. **Required tools installed**: `bash` (>=5.0), `jq`, `curl`, `gh`

3. **Repository access**: Read access to GitHub Actions runs and Pages, write access to trigger workflows

## Execute the Release Gate

### Section G1: Discover Recent Runs

```bash
gh run list -R HuddleScope/huddlescope.github.io --workflow refresh-news.yml \
  --json databaseId,displayTitle,runNumber,runName,event,createdAt,status,conclusion,headSha,url \
  --limit 20
```

Look for at least two completed runs with `workflow_dispatch` events or cron-triggered runs (7,37 UTC pattern) spaced at least 30 minutes apart.

Record the URLs and artifact IDs from the run list output.

### Section G2: Manual Dispatch and Wait

```bash
# Dispatch a manual run
gh workflow run refresh-news.yml -R HuddleScope/huddlescope.github.io --field publish=true

# Poll until complete (bounded timeout)
gh run wait --run-id <RUN_ID> -R HuddleScope/huddlescope.github.io --timeout 15m
```

Record the conclusion (`success`, `failure`, `cancelled`) and the deployed commit SHA from the run log.

### Section G3: Live Site Verification

```bash
# Fetch live homepage
curl -s https://huddlescope.github.io/ | grep -o 'data-articles="[0-9]*"' | cut -d'"' -f2

# Fetch live feeds status page
curl -s https://huddlescope.github.io/status/feeds/ | grep -o 'data-articles="[0-9]*"' | cut -d'"' -f2

# Compare against committed snapshot
git show origin/main:web/data/homepage.json | jq -r '.generatedAt'
```

Verify the story counts and `generatedAt` timestamp match between live pages and the committed `web/data/homepage.json`.

### Section G4: Local Failure Drills (Offline)

```bash
# Test empty database behavior
HSC_STORAGE_BACKEND=sqlite HSC_DATA_DIR=/tmp/test-data HSC_DATA_FILE=/tmp/test-data/data.json \
  bash -c 'sqlite3 /tmp/test-data/test.db "DELETE FROM stories;" && rm -f /tmp/test-data/data.json'

# Test invalid Turso credentials
HSC_STORAGE_BACKEND=turso HSC_TURSO_DATABASE_URL=invalid-url HSC_TURSO_AUTH_TOKEN=invalid-token \
  make test

# Test partial failure with timeout
HSC_REQUEST_TIMEOUT=1ms \
  make test
```

### Section G5: Generate Evidence

```bash
HSC_EVIDENCE_DIR=$HOME/.pi/huddlescope/evidence-$(date +%Y%m%dT%H%M%SZ) \
  bash scripts/release-gate.sh --local-only
```

The evidence directory contains:
- `evidence.json`: Machine-readable evidence with timestamps, counts, run links, artifact IDs, deployed commit, and per-AC PASS/FAIL status
- `evidence.md`: Human-readable summary ready to paste into GitHub issue comments

## Evidence Template for Issue #110

When posting evidence to GitHub issue #110, include:

```markdown
### Release Gate Evidence
**Date**: 2026-11-19
**Evidence path**: $HOME/.pi/huddlescope/evidence-20261119T120000Z/

**Summary**:
- **AC-1**: PASS - Two cron-triggered runs observed
- **AC-2**: PASS - Fresh snapshot with generatedAt: 2026-11-19T12:00:00Z
- **AC-3**: PASS - Live site consistent with committed snapshot
- **AC-4**: PASS - All failure scenarios preserve last-good state
- **AC-5**: PASS - Evidence generated

**Run Links**:
1. https://github.com/HuddleScope/huddlescope.github.io/actions/runs/12345
2. https://github.com/HuddleScope/huddlescope.github.io/actions/runs/12346

**Artifacts**: artifact-1, artifact-2

**Deployed Commit**: de9265f23a9e273c89bb45302dc6214beddc36a9
```

## Failure Scenario Matrix

| Scenario | Covered By | External Observation |
|----------|------------|---------------------|
| Expired Cloudflare Worker token | G4 (local drills), docs | Cloudflare `wrangler logs` 401 error, no new Actions run |
| Turso outage | Go tests (TestReleaseGateStorageOutageKeepsLastGood) | Workflow skips deploy, last-good site preserved |
| RSS feed total failure | Go tests (TestReleaseGateFeedTotalFailureKeepsLastGood) | Run fails, heartbeat status `failed`/`partial` |
| Pages upload failure | Go tests (publish-decision table) | Actions log shows `upload-pages-artifact` failure, deploy skipped |
| Empty export | Go tests (publish-decision table), G4 | Homepage shows labeled empty state, `generatedAt` truthful |

## refresh-news.yml step graph (current)

```
build job:
  checkout → setup-go → build CLI
  → **Decide whether backup ingest should run** (huddlescope ingest should-run --stale-after 75m --output json; echo should_run/reason to GITHUB_OUTPUT; fail closed on DB error)
  → **Run backup ingest** (huddlescope ingest run --mode backup --stale-after 75m [--force]; if: force_ingest || should_run == true; skipped step is non-failing)
  → **Export snapshot** (huddlescope export run; always runs; re-publishes site-data from DB)
  → **Publish decision** (jq count + git show HEAD:; if: steps.export.conclusion == 'success')
  → **Commit and push site-data files** (git add web/data/{heartbeat,feeds,homepage}.json; git commit [skip ci]; git push; if: steps.export.conclusion == 'success'; permissions: contents: write at step level)
  → install Hugo → **Run site verification** (continue-on-error)
  → **Build Hugo site** (make site)
  → **Verify no demo markers**
  → **Verify Hugo build artifacts**
  → **Upload site artifacts** (diagnostic only; if: export or make-site success)
  → configure-pages → upload-pages-artifact (if: inputs.publish && deploy == 'true')
deploy job: needs build, if: inputs.publish && deploy == 'true'
```

### force_ingest input (issue #163 / AC-6)

`force_ingest` is a **maintenance-only** workflow_dispatch input (boolean, default
`false`). When true, the ingest step runs even if Turso is fresh — but it
**skips only the staleness pre-check, never the distributed lease**. A held
lease still produces `skipped_locked`. The Cloudflare Worker dispatch body
must **NOT** include `force_ingest` (enforced by `scheduler/src/index.test.js`).

### heartbeat-check.yml (issue #163 / AC-9)

The heartbeat check is split into two independent checks:

1. **Ingest freshness (Turso)** — `huddlescope ingest should-run --stale-after 90m --output json`.
   Fail-closed on DB error. Answers "did any host successfully ingest recently?"
2. **Publication freshness (deployed site)** — HTTP `curl` of
   `https://huddlescope.github.io/data/heartbeat.json`, parse `published_at`,
   compare to 90-minute threshold. Answers "is the live site fresh?"

Neither check reads the checkout's `web/data/heartbeat.json` (a historical
repo file) as a remote signal.

## Quarterly Re-run Instructions

To re-run the release gate quarterly:

```bash
# 1. Verify gh is authenticated
gh auth status

# 2. Run the gate
bash scripts/release-gate.sh --local-only  # For offline drills

# 3. For production evidence
bash scripts/release-gate.sh --live

# 4. Post evidence to issue #110 as a comment
```

## Exit Codes

- **0**: All AC pass
- **1**: One or more AC fail

---

**Last Updated**: 2026-11-19
