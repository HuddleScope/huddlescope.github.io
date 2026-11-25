// Package e2e: workflow contract tests (issue #163 / HS-PORT-07).
//
// These tests parse the GitHub Actions workflow YAML files and assert
// structural invariants (syntactic validity + branch-selection contract).
// They guard against accidental workflow regressions: missing decision
// steps, if: always() on commit/deploy steps, missing force_ingest input,
// missing push step, wrong permission scoping.
package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// loadWorkflow reads and parses a workflow YAML file. The top-level map uses
// map[any]any because YAML 1.1 parses the bare "on" key as boolean true.
// Nested maps are map[string]any (yaml.v3 uses map[string]interface{} for
// non-bool keys).
func loadWorkflow(t *testing.T, path string) map[any]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read workflow %s: %v", path, err)
	}
	var wf map[any]any
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatalf("failed to parse workflow YAML %s: %v", path, err)
	}
	return wf
}

// strMap converts a map[any]any to a map[string]any for nested lookups.
// yaml.v3 parses nested maps as map[string]interface{}, so this is a safe
// type assertion at the nesting level.
func strMap(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", v)
	}
	return m
}

// findStepByID searches a job's steps for a step with the given id.
func findStepByID(t *testing.T, wf map[any]any, jobID, stepID string) map[string]any {
	t.Helper()
	jobs := strMap(t, wf["jobs"])
	job, ok := jobs[jobID].(map[string]any)
	if !ok {
		t.Fatalf("workflow: missing job %q", jobID)
	}
	steps, ok := job["steps"].([]any)
	if !ok {
		t.Fatalf("workflow: job %q missing steps", jobID)
	}
	for _, s := range steps {
		step, ok := s.(map[string]any)
		if !ok {
			continue
		}
		if id, ok := step["id"].(string); ok && id == stepID {
			return step
		}
	}
	t.Fatalf("workflow: job %q missing step with id %q", jobID, stepID)
	return nil
}

// hasAlwaysIf reports whether a step's if expression contains "always()".
func hasAlwaysIf(step map[string]any) bool {
	ifCond, ok := step["if"]
	if !ok {
		return false
	}
	s, ok := ifCond.(string)
	if !ok {
		return false
	}
	return strings.Contains(s, "always()")
}

// stepRunScript returns the run script of a step (empty if not a run step).
func stepRunScript(step map[string]any) string {
	run, ok := step["run"]
	if !ok {
		return ""
	}
	s, ok := run.(string)
	if !ok {
		return ""
	}
	return s
}

// getOnSection extracts the "on" section, handling both the string key "on"
// and the boolean key true (YAML 1.1 parses bare "on" as bool).
func getOnSection(t *testing.T, wf map[any]any) map[string]any {
	t.Helper()
	if v, ok := wf["on"]; ok {
		return strMap(t, v)
	}
	if v, ok := wf[true]; ok {
		return strMap(t, v)
	}
	t.Fatalf("workflow: missing or invalid 'on' section")
	return nil
}

// TestWorkflowContract_RefreshNews_Structure validates the structural
// invariants of refresh-news.yml (issue #163 / AC-3, AC-5, AC-6, AC-7, AC-9).
func TestWorkflowContract_RefreshNews_Structure(t *testing.T) {
	t.Parallel()

	wfPath := filepath.Join("..", "..", ".github", "workflows", "refresh-news.yml")
	wf := loadWorkflow(t, wfPath)

	// --- Triggers: workflow_dispatch with force_ingest input (AC-6) ---
	onSection := getOnSection(t, wf)
	dispatch, ok := onSection["workflow_dispatch"].(map[string]any)
	if !ok {
		t.Fatal("refresh-news.yml: missing workflow_dispatch trigger")
	}
	inputs, ok := dispatch["inputs"].(map[string]any)
	if !ok {
		t.Fatal("refresh-news.yml: workflow_dispatch missing inputs")
	}
	forceIngest, ok := inputs["force_ingest"].(map[string]any)
	if !ok {
		t.Fatal("refresh-news.yml: missing force_ingest input")
	}
	if ft, ok := forceIngest["type"].(string); !ok || ft != "boolean" {
		t.Errorf("force_ingest input type = %v, want \"boolean\"", forceIngest["type"])
	}
	if fd, ok := forceIngest["default"].(string); !ok || fd != "false" {
		t.Errorf("force_ingest input default = %v, want \"false\"", forceIngest["default"])
	}

	// --- Build job: decision step exists (AC-1, AC-2, AC-5) ---
	decisionStep := findStepByID(t, wf, "build", "decision")
	decisionScript := stepRunScript(decisionStep)
	if !strings.Contains(decisionScript, "ingest should-run") {
		t.Error("decision step must invoke 'ingest should-run'")
	}
	if !strings.Contains(decisionScript, "--stale-after 75m") {
		t.Error("decision step must use --stale-after 75m")
	}
	if !strings.Contains(decisionScript, "--output json") {
		t.Error("decision step must use --output json")
	}
	if !strings.Contains(decisionScript, "should_run") {
		t.Error("decision step must echo should_run to GITHUB_OUTPUT")
	}
	if !strings.Contains(decisionScript, "reason") {
		t.Error("decision step must echo reason to GITHUB_OUTPUT")
	}
	if hasAlwaysIf(decisionStep) {
		t.Error("decision step must NOT use if: always()")
	}
	if _, hasCOE := decisionStep["continue-on-error"]; hasCOE {
		t.Error("decision step must NOT have continue-on-error (fail closed on DB error)")
	}

	// --- Build job: ingest step gated on decision or force_ingest (AC-1, AC-6) ---
	ingestStep := findStepByID(t, wf, "build", "ingest")
	ingestIf, ok := ingestStep["if"].(string)
	if !ok {
		t.Fatal("ingest step must have an if condition")
	}
	if !strings.Contains(ingestIf, "steps.decision.outputs.should_run") {
		t.Error("ingest step if must reference steps.decision.outputs.should_run")
	}
	if !strings.Contains(ingestIf, "inputs.force_ingest") {
		t.Error("ingest step if must reference inputs.force_ingest")
	}
	ingestScript := stepRunScript(ingestStep)
	if !strings.Contains(ingestScript, "ingest run --mode backup") {
		t.Error("ingest step must invoke 'ingest run --mode backup'")
	}
	if !strings.Contains(ingestScript, "--stale-after 75m") {
		t.Error("ingest step must use --stale-after 75m")
	}

	// --- Build job: export step (no if, always runs) (AC-3) ---
	exportStep := findStepByID(t, wf, "build", "export")
	if _, hasIf := exportStep["if"]; hasIf {
		t.Error("export step must NOT have an if condition (always runs)")
	}
	exportScript := stepRunScript(exportStep)
	if !strings.Contains(exportScript, "export run") {
		t.Error("export step must invoke 'export run'")
	}

	// --- Build job: publish-decision step gated on export success (AC-4) ---
	publishDecisionStep := findStepByID(t, wf, "build", "publish-decision")
	pdIf, ok := publishDecisionStep["if"].(string)
	if !ok {
		t.Fatal("publish-decision step must have an if condition")
	}
	if !strings.Contains(pdIf, "steps.export.conclusion == 'success'") {
		t.Errorf("publish-decision if = %q, must gate on export success", pdIf)
	}
	if hasAlwaysIf(publishDecisionStep) {
		t.Error("publish-decision step must NOT use if: always()")
	}

	// --- Build job: exactly one commit+push step (AC-7) ---
	jobs := strMap(t, wf["jobs"])
	buildJob := jobs["build"].(map[string]any)
	steps := buildJob["steps"].([]any)
	var pushSteps []string
	for _, s := range steps {
		step, ok := s.(map[string]any)
		if !ok {
			continue
		}
		run := stepRunScript(step)
		if strings.Contains(run, "git push") {
			id, _ := step["id"].(string)
			name, _ := step["name"].(string)
			pushSteps = append(pushSteps, id+":"+name)
		}
	}
	if len(pushSteps) != 1 {
		t.Errorf("expected exactly one step with 'git push', got %d: %v", len(pushSteps), pushSteps)
	}

	// --- Build job: commit-and-push-site-data step (AC-7) ---
	commitPushStep := findStepByID(t, wf, "build", "commit-and-push-site-data")
	cpsIf, ok := commitPushStep["if"].(string)
	if !ok {
		t.Fatal("commit-and-push-site-data step must have an if condition")
	}
	if !strings.Contains(cpsIf, "steps.export.conclusion == 'success'") {
		t.Errorf("commit-and-push-site-data if = %q, must gate on export success", cpsIf)
	}
	if hasAlwaysIf(commitPushStep) {
		t.Error("commit-and-push-site-data step must NOT use if: always()")
	}
	if _, hasCOE := commitPushStep["continue-on-error"]; hasCOE {
		t.Error("commit-and-push-site-data step must NOT have continue-on-error")
	}
	// Job-level permissions on build: contents: write. GitHub Actions does
	// not support step-level permissions (a workflow with them is rejected as
	// "Invalid workflow file"), so the AC-7 write grant lives on the build job.
	jobPerms, ok := buildJob["permissions"].(map[string]any)
	if !ok {
		t.Fatal("build job must have job-level permissions")
	}
	if contents, ok := jobPerms["contents"].(string); !ok || contents != "write" {
		t.Errorf("build job permissions.contents = %v, want \"write\"", jobPerms["contents"])
	}
	if _, hasStepPerms := commitPushStep["permissions"]; hasStepPerms {
		t.Error("commit-and-push-site-data step must NOT have step-level permissions (unsupported by GitHub Actions)")
	}
	// The main branch requires the `Build` status check (ruleset "Require CI
	// before merging to main"), so the step must not push directly to main
	// with [skip ci]: it commits to a short-lived branch, opens a PR, waits
	// for the Build check, and squash-merges.
	cpsScript := stepRunScript(commitPushStep)
	if strings.Contains(cpsScript, "[skip ci]") {
		t.Error("commit-and-push-site-data must NOT use [skip ci]: the Build check is required by the main ruleset")
	}
	if !strings.Contains(cpsScript, "git checkout -b") || !strings.Contains(cpsScript, "gh pr create") {
		t.Error("commit-and-push-site-data must commit to a short-lived branch and open a PR")
	}
	if !strings.Contains(cpsScript, "gh pr merge") {
		t.Error("commit-and-push-site-data must squash-merge the PR after the Build check passes")
	}
	// The retention package (issue #120 / HS-065) ships in the same PR as the
	// site-data files, and the PR/branch are recorded for the cleanup step.
	if !strings.Contains(cpsScript, "archive/retention") {
		t.Error("commit-and-push-site-data must stage archive/retention in the same PR as the site-data files")
	}
	if !strings.Contains(cpsScript, "pr_num=") || !strings.Contains(cpsScript, "branch=") {
		t.Error("commit-and-push-site-data must write pr_num and branch to GITHUB_OUTPUT for the cleanup step")
	}
	// Conflict recovery: after the Build check, verify mergeability and
	// re-base onto origin/main when main moved (bounded re-base attempts).
	if !strings.Contains(cpsScript, "mergeable") || !strings.Contains(cpsScript, "git reset --hard origin/main") {
		t.Error("commit-and-push-site-data must check mergeability and re-base onto origin/main when main moved")
	}

	// --- Build job: retention steps (issue #120 / HS-065) ---
	retExport := findStepByID(t, wf, "build", "retention-export")
	if ri, ok := retExport["if"].(string); !ok || !strings.Contains(ri, "inputs.run-retention") {
		t.Error("retention-export must be gated on inputs.run-retention")
	}
	retApply := findStepByID(t, wf, "build", "retention-apply")
	ai, ok := retApply["if"].(string)
	if !ok || !strings.Contains(ai, "inputs.run-retention") || !strings.Contains(ai, "steps.commit-and-push-site-data.conclusion == 'success'") {
		t.Errorf("retention-apply if = %q, must gate on run-retention AND the site-data step success (package persisted before apply)", ai)
	}
	// The package must ship in the site-data PR: export before the commit step.
	stepIdx := func(stepID string) int {
		for i, s := range steps {
			st, ok := s.(map[string]any)
			if ok {
				if id, ok := st["id"].(string); ok && id == stepID {
					return i
				}
			}
		}
		return -1
	}
	if ei, ci := stepIdx("retention-export"), stepIdx("commit-and-push-site-data"); ei < 0 || ci < 0 || ei >= ci {
		t.Error("retention-export must run before commit-and-push-site-data so the package ships in the same PR")
	}
	if stepIdx("retention-commit") >= 0 {
		t.Error("retention-commit must not exist: the retention package ships in the site-data PR (exactly one commit+push step)")
	}
	// Cleanup step: closes the PR and deletes the branch when the run did not
	// merge (cancelled or failed), so no orphaned PR conflicts with later runs.
	cleanup := findStepByID(t, wf, "build", "cleanup-site-data-pr")
	ciCond, ok := cleanup["if"].(string)
	if !ok || !strings.Contains(ciCond, "always()") || !strings.Contains(ciCond, "steps.commit-and-push-site-data.outcome != 'success'") || !strings.Contains(ciCond, "pr_num") {
		t.Errorf("cleanup-site-data-pr if = %q, must run on always() when the site-data step did not succeed and a PR was opened", ciCond)
	}
	cr := stepRunScript(cleanup)
	if !strings.Contains(cr, "gh pr close") {
		t.Error("cleanup-site-data-pr must close the orphaned PR")
	}

	// --- Build job: no if: always() on any commit/deploy step (AC-7) ---
	for _, s := range steps {
		step, ok := s.(map[string]any)
		if !ok {
			continue
		}
		run := stepRunScript(step)
		if (strings.Contains(run, "git commit") || strings.Contains(run, "git push") ||
			strings.Contains(run, "upload-pages-artifact") || strings.Contains(run, "deploy-pages")) &&
			hasAlwaysIf(step) {
			id, _ := step["id"].(string)
			name, _ := step["name"].(string)
			t.Errorf("step %q (%q) commits/deploys but uses if: always()", id, name)
		}
	}

	// --- Build job: artifact upload gated on success (AC-7) ---
	foundUpload := false
	for _, s := range steps {
		step, ok := s.(map[string]any)
		if !ok {
			continue
		}
		uses, _ := step["uses"].(string)
		if strings.Contains(uses, "upload-artifact") {
			foundUpload = true
			if hasAlwaysIf(step) {
				t.Error("upload-artifact step must NOT use if: always()")
			}
			uploadIf, _ := step["if"].(string)
			if !strings.Contains(uploadIf, "success") {
				t.Errorf("upload-artifact if = %q, must gate on success", uploadIf)
			}
		}
	}
	if !foundUpload {
		t.Error("refresh-news.yml: missing upload-artifact step")
	}

	// --- Build job: Pages steps gated on publish + deploy (AC-4) ---
	for _, s := range steps {
		step, ok := s.(map[string]any)
		if !ok {
			continue
		}
		uses, _ := step["uses"].(string)
		if strings.Contains(uses, "configure-pages") || strings.Contains(uses, "upload-pages-artifact") {
			ifCond, ok := step["if"].(string)
			if !ok {
				t.Errorf("Pages step %q must have an if condition", uses)
			}
			if !strings.Contains(ifCond, "inputs.publish") {
				t.Errorf("Pages step %q if must reference inputs.publish", uses)
			}
			if !strings.Contains(ifCond, "deploy") {
				t.Errorf("Pages step %q if must reference the deploy decision", uses)
			}
		}
	}

	// --- Deploy job: gated on publish + deploy (AC-4) ---
	deployJob, ok := jobs["deploy"].(map[string]any)
	if !ok {
		t.Fatal("refresh-news.yml: missing deploy job")
	}
	deployIf, ok := deployJob["if"].(string)
	if !ok {
		t.Fatal("deploy job must have an if condition")
	}
	if !strings.Contains(deployIf, "inputs.publish") {
		t.Error("deploy job if must reference inputs.publish")
	}
	if !strings.Contains(deployIf, "deploy") {
		t.Error("deploy job if must reference the build's deploy output")
	}
}

// TestWorkflowContract_HeartbeatCheck_Structure validates the structural
// invariants of heartbeat-check.yml (issue #163 / AC-9).
func TestWorkflowContract_HeartbeatCheck_Structure(t *testing.T) {
	t.Parallel()

	wfPath := filepath.Join("..", "..", ".github", "workflows", "heartbeat-check.yml")
	wf := loadWorkflow(t, wfPath)

	jobs := strMap(t, wf["jobs"])
	checkJob, ok := jobs["check"].(map[string]any)
	if !ok {
		t.Fatal("heartbeat-check.yml: missing check job")
	}
	steps := checkJob["steps"].([]any)

	var foundTursoCheck bool
	var foundHTTPCheck bool
	var foundCheckoutFileCheck bool

	for _, s := range steps {
		step, ok := s.(map[string]any)
		if !ok {
			continue
		}
		run := stepRunScript(step)
		name, _ := step["name"].(string)

		// AC-9: Turso-backed should-run check
		if strings.Contains(run, "ingest should-run") && strings.Contains(run, "--stale-after 90m") {
			foundTursoCheck = true
			if !strings.Contains(run, "--output json") {
				t.Error("Turso check must use --output json")
			}
		}

		// AC-9: HTTP fetch of deployed site's data/heartbeat.json
		if strings.Contains(run, "curl") && strings.Contains(run, "huddlescope.github.io") {
			foundHTTPCheck = true
			if strings.Contains(run, "web/data/heartbeat.json") {
				foundCheckoutFileCheck = true
			}
		}

		// AC-9: must NOT read checkout's web/data/heartbeat.json as a remote signal
		if strings.Contains(run, "heartbeat check --file web/data/heartbeat.json") {
			foundCheckoutFileCheck = true
			t.Errorf("step %q reads checkout web/data/heartbeat.json as a remote signal (AC-9 violation)", name)
		}
	}

	if !foundTursoCheck {
		t.Error("heartbeat-check.yml: missing Turso-backed ingest freshness check (should-run --stale-after 90m)")
	}
	if !foundHTTPCheck {
		t.Error("heartbeat-check.yml: missing HTTP publication freshness check (curl to deployed site)")
	}
	if foundCheckoutFileCheck {
		t.Error("heartbeat-check.yml: must NOT read checkout files as a remote signal (AC-9)")
	}

	// AC-9: workflow must have TURSO_* env vars for the Turso check
	env, ok := checkJob["env"].(map[string]any)
	if !ok {
		t.Fatal("heartbeat-check.yml: check job must have env block")
	}
	if _, ok := env["TURSO_DATABASE_URL"]; !ok {
		t.Error("heartbeat-check.yml: check job env must include TURSO_DATABASE_URL")
	}
	if _, ok := env["TURSO_AUTH_TOKEN"]; !ok {
		t.Error("heartbeat-check.yml: check job env must include TURSO_AUTH_TOKEN")
	}
}
