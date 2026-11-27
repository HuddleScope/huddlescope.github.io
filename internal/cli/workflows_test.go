package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/HuddleScope/huddlescope.github.io/internal/snapshot"
)

// TestRefreshNewsWorkflow_Issue247 (issue #247 / PUB-01, AC-3, AC-8) parses
// refresh-news.yml and asserts the structural invariants that make news data
// repo-less: no news-data commit/PR step, an explicit manifest get before the
// gate, the gate invoked with --require-manifest, Turso env on the deploy
// job, and a manifest put (no git) after deploy. The text-parse gate stands
// in for an Actions run in CI (no Actions on check runs).
//
// issue #252 / PUB-06 (AC-1) repurposes the retention assertions: the
// retention-only commit step is REMOVED. Retention/archival is isolated to
// the dedicated retention.yml workflow, so refresh-news.yml must contain no
// retention step, no archive/retention staging, no contents:write, and no
// SITE_DATA_PAT.
func TestRefreshNewsWorkflow_Issue247(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "refresh-news.yml"))
	require.NoError(t, err, "refresh-news.yml must exist")
	c := string(data)

	// AC-3: no news-data commit/PR step remains.
	assert.NotContains(t, c, "Commit and push site-data files", "old site-data commit step must be removed (AC-3)")
	assert.NotContains(t, c, "Clean up site-data PR", "old site-data PR cleanup step must be removed (AC-3)")

	// issue #252 / AC-1: retention is isolated to retention.yml. refresh-news
	// must contain NO retention commit/apply step, no archive/retention
	// staging, no contents:write, and no retention PAT.
	assert.NotContains(t, c, "Commit and push retention package", "retention-only commit step must be removed (issue #252 / AC-1)")
	assert.NotContains(t, c, "git add archive/retention", "refresh-news must not stage archive/retention (issue #252 / AC-1)")
	assert.NotContains(t, c, "Clean up retention PR", "retention PR cleanup step must be removed (issue #252 / AC-1)")
	assert.NotContains(t, c, "retention apply", "refresh-news must not run retention apply (issue #252 / AC-1)")
	assert.NotContains(t, c, "retention export", "refresh-news must not run retention export (issue #252 / AC-1)")
	assert.NotContains(t, c, "run-retention", "refresh-news must not have a run-retention input (issue #252 / AC-1)")
	assert.NotContains(t, c, "SITE_DATA_PAT", "refresh-news must not reference a retention PAT (issue #252 / AC-1)")
	assert.NotContains(t, c, "contents: write", "refresh-news must not grant contents:write (issue #252 / AC-1)")

	// AC-6: manifest get before the gate, and the gate uses --require-manifest.
	assert.Contains(t, c, "publish manifest get", "manifest get step must be present (AC-6)")
	assert.Contains(t, c, "publish gate", "publish gate step must be present")
	assert.Contains(t, c, "--require-manifest", "gate must be invoked with --require-manifest (AC-6)")

	// AC-8: manifest put records the deploy in the store (no git).
	assert.Contains(t, c, "publish manifest put", "manifest put step must be present after deploy (AC-8)")

	// Turso env must be reachable in both jobs (manifest read/write needs it).
	assert.GreaterOrEqual(t, strings.Count(c, "TURSO_DATABASE_URL:"), 2, "Turso env must be set in build and deploy jobs")

	// The deploy job must re-project the snapshot (web/data is untracked on a
	// fresh checkout) before the manifest put.
	assert.Contains(t, c, "Re-project production snapshot", "deploy job must re-project the snapshot")
}

// TestRetentionWorkflow_Issue252 (issue #252 / PUB-06, AC-1, AC-2) asserts the
// dedicated retention.yml workflow exists and is fail-closed: dry-run by
// default (apply input default false), export -> verify -> persist -> apply
// ordering, apply gated on persist success, contents:read only, and the
// scoped archive-repo PAT used only as the persist step's GH_TOKEN.
func TestRetentionWorkflow_Issue252(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "retention.yml"))
	require.NoError(t, err, "retention.yml must exist (issue #252 / AC-1)")
	c := string(data)

	// AC-2: dry-run by default — the apply dispatch input defaults to false.
	assert.Contains(t, c, "apply:", "retention.yml must have an 'apply' dispatch input (issue #252 / AC-2)")
	assert.Contains(t, c, "default: 'false'", "apply input must default to false = dry-run (issue #252 / AC-2)")
	assert.Contains(t, c, "schedule:", "retention.yml must have a schedule trigger (issue #252 / AC-5)")

	// AC-3: dedicated concurrency group, no in-progress cancel.
	assert.Contains(t, c, "group: huddlescope-retention", "dedicated concurrency group (issue #252 / AC-3)")
	assert.Contains(t, c, "cancel-in-progress: false", "never cancel an in-flight retention run (issue #252 / AC-3)")

	// AC-2: no permission escalation — contents: read only.
	assert.Contains(t, c, "contents: read", "retention.yml must use contents: read (no escalation)")
	assert.NotContains(t, c, "contents: write", "retention.yml must not grant contents: write (issue #252 / AC-2)")

	// AC-2: the export step never deletes; verify uses a scratch DB.
	assert.Contains(t, c, "retention export", "export step must be present")
	assert.Contains(t, c, "retention import --package", "verify step must import the package into a scratch DB")
	assert.Contains(t, c, "HSC_STORAGE_BACKEND=sqlite-file", "verify must run against a throwaway sqlite-file DB")

	// AC-2: the scoped archive-repo PAT is used only as the persist step's
	// GH_TOKEN; apply is gated on persist success.
	assert.Contains(t, c, "RETENTION_ARCHIVE_PAT", "persist step must use the scoped archive-repo PAT")
	assert.Contains(t, c, "inputs.apply == 'true'", "apply must be gated on the explicit apply input")
	assert.Contains(t, c, "steps.persist.conclusion == 'success'", "apply must be gated on persist success (fail closed)")
}

// TestSiteWorkflow_Issue247 (issue #247 / PUB-01, AC-4, AC-6, AC-8) parses
// site.yml and asserts the deploy job no longer depends on committed data:
// no "Restore web/data ... from main" git-archive step, no "committed homepage
// missing" failure branch, an explicit manifest get + --require-manifest gate,
// and a manifest put after deploy.
func TestSiteWorkflow_Issue247(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "site.yml"))
	require.NoError(t, err, "site.yml must exist")
	c := string(data)

	// AC-4: the deploy job no longer restores committed web/data (git archive).
	assert.NotContains(t, c, "Restore web/data and web/public from main", "old git-archive restore step must be removed (AC-4)")
	assert.NotContains(t, c, "committed homepage missing", "old 'committed homepage missing' failure branch must be removed (AC-4)")

	// The deploy job re-projects the snapshot from Turso (workspace).
	assert.Contains(t, c, "Re-project production snapshot", "deploy job must re-project the snapshot (AC-4)")

	// AC-6: manifest get + gate with --require-manifest.
	assert.Contains(t, c, "publish manifest get", "manifest get step must be present (AC-6)")
	assert.Contains(t, c, "--require-manifest", "gate must be invoked with --require-manifest (AC-6)")

	// AC-8: manifest put records the deploy in the store (no git).
	assert.Contains(t, c, "publish manifest put", "manifest put step must be present after deploy (AC-8)")

	// Turso env on the deploy job.
	assert.Contains(t, c, "TURSO_DATABASE_URL:", "deploy job must have Turso env")
}

// TestWorkflows_NoNewsDataCommit asserts neither workflow commits web/data.
// refresh-news.yml performs no Git mutation of any kind (issue #252); site.yml
// commits editorial content (stories, docs) but never web/data.
func TestWorkflows_NoNewsDataCommit(t *testing.T) {
	root := repoRoot(t)
	for _, wf := range []string{"refresh-news.yml", "site.yml"} {
		data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", wf))
		require.NoError(t, err, "%s must exist", wf)
		c := string(data)
		// Neither workflow may `git add` a web/data path.
		assert.NotContains(t, c, "git add web/data", wf+" must not commit web/data (AC-3)")
	}
}

// TestRefreshNewsWorkflow_Issue249 (issue #249 / PUB-03, AC-3, AC-4) parses
// refresh-news.yml and asserts the snapshot-artifact invariants: a
// deterministic package/validate/upload step chain with the content-derived
// artifact name, 7-day retention and if-no-files-found: error, the
// pages-binding manifest/run-identity artifact, the manual-recovery
// restore_artifact dispatch path with the DB steps gated off, and the
// demo-marker list pinned to snapshot.DemoMarkers.
func TestRefreshNewsWorkflow_Issue249(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "refresh-news.yml"))
	require.NoError(t, err, "refresh-news.yml must exist")
	c := string(data)

	// AC-1/AC-2: package + independent validate before upload.
	assert.Contains(t, c, "Package snapshot artifact", "package step must be present")
	assert.Contains(t, c, "export package", "package step must call export package")
	assert.Contains(t, c, "Validate snapshot artifact", "validate step must be present")
	assert.Contains(t, c, "export validate", "validate step must call export validate")

	// AC-3: the snapshot artifact is uploaded with the content-derived name,
	// 7-day retention, and if-no-files-found: error (a missing package fails
	// the job so the last-good Pages content is preserved).
	assert.Contains(t, c, "Upload site-snapshot artifact", "site-snapshot upload step must be present")
	assert.Contains(t, c, "name: site-snapshot-${{ steps.package-snapshot.outputs.snapshot_id }}", "artifact must be named by snapshotId")
	assert.Contains(t, c, "retention-days: 7", "snapshot artifact must retain for 7 days")
	assert.Contains(t, c, "if-no-files-found: error", "a missing snapshot package must fail the job")

	// AC-3: the Pages binding artifact links the rendered output to the
	// snapshot and the run identity.
	assert.Contains(t, c, "Write Pages binding manifest", "pages-binding step must be present")
	assert.Contains(t, c, "pages-${{ github.run_id }}", "binding artifact must be named by run id")
	assert.Contains(t, c, "githubRunId", "binding must carry the run id")
	assert.Contains(t, c, "githubSha", "binding must carry the commit sha")
	assert.Contains(t, c, "indexHtmlSha256", "binding must carry the rendered index hash")

	// AC-4: manual recovery path: dispatch input, download + restore steps,
	// DB steps gated off when restoring, and the deploy bypass gated on
	// publish=true AND restore_artifact set.
	assert.Contains(t, c, "restore_artifact:", "restore_artifact dispatch input must be present")
	assert.Contains(t, c, "Download snapshot artifact for restore", "restore download step must be present")
	assert.Contains(t, c, "Restore snapshot workspace", "restore step must be present")
	assert.Contains(t, c, "export restore", "restore step must call export restore")
	assert.Contains(t, c, "--force", "restore must require explicit operator intent (--force)")
	assert.Contains(t, c, "inputs.restore_artifact == ''", "DB steps must be gated off when restoring")
	assert.Contains(t, c, "steps.publish-gate.outputs.deploy == 'true' || inputs.restore_artifact != ''",
		"deploy must bypass the gate only for an explicit restore")

	// The demo-marker grep in the workflow must stay in sync with the Go
	// constant used by the snapshot validator (no drift between the two).
	markerList := workflowDemoMarkers(t, c)
	assert.ElementsMatch(t, snapshot.DemoMarkers, markerList, "workflow demo markers must match snapshot.DemoMarkers")
}

// workflowDemoMarkers extracts the MARKERS='a|b|c' list from the
// "Verify no demo markers" step of refresh-news.yml (unescaping the regexp
// dot escapes) so the guard test can pin it against snapshot.DemoMarkers.
func workflowDemoMarkers(t *testing.T, yml string) []string {
	t.Helper()
	re := regexp.MustCompile(`MARKERS='([^']+)'`)
	m := re.FindStringSubmatch(yml)
	require.NotNil(t, m, "MARKERS grep list must exist in refresh-news.yml")
	parts := strings.Split(m[1], "|")
	for i := range parts {
		parts[i] = strings.ReplaceAll(parts[i], `\.`, ".")
	}
	return parts
}
