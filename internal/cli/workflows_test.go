package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRefreshNewsWorkflow_Issue247 (issue #247 / PUB-01, AC-3, AC-8) parses
// refresh-news.yml and asserts the structural invariants that make news data
// repo-less: no news-data commit/PR step, an explicit manifest get before the
// gate, the gate invoked with --require-manifest, a retention-only commit
// step (never staging web/data), Turso env on the deploy job, and a manifest
// put (no git) after deploy. The text-parse gate stands in for an Actions
// run in CI (no Actions on check runs).
func TestRefreshNewsWorkflow_Issue247(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "refresh-news.yml"))
	require.NoError(t, err, "refresh-news.yml must exist")
	c := string(data)

	// AC-3: no news-data commit/PR step remains. The old commit step staged
	// web/data; the new retention-only step must NOT stage web/data.
	assert.NotContains(t, c, "Commit and push site-data files", "old site-data commit step must be removed (AC-3)")
	assert.NotContains(t, c, "Clean up site-data PR", "old site-data PR cleanup step must be removed (AC-3)")

	// The retention-only commit step commits ONLY archive/retention (AC-7).
	assert.Contains(t, c, "Commit and push retention package", "retention-only commit step must be present")
	assert.Contains(t, c, "git add archive/retention", "retention commit must stage only archive/retention")

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

	// The retention PR cleanup step is keyed to the retention commit (not the
	// removed site-data step).
	assert.Contains(t, c, "Clean up retention PR", "retention PR cleanup step must be present")
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
// The refresh-news retention step stages archive/retention; site.yml commits
// editorial content (stories, docs) but never web/data.
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
