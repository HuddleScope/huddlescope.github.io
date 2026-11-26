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

// findStepByNameIn returns the step with the given name in the given job,
// or nil if not found (non-fatal; callers decide how to report).
func findStepByNameIn(t *testing.T, wf map[any]any, jobID, name string) map[string]any {
	t.Helper()
	jobs := strMap(t, wf["jobs"])
	job, ok := jobs[jobID].(map[string]any)
	if !ok {
		return nil
	}
	steps, ok := job["steps"].([]any)
	if !ok {
		return nil
	}
	for _, s := range steps {
		step, ok := s.(map[string]any)
		if !ok {
			continue
		}
		if n, ok := step["name"].(string); ok && n == name {
			return step
		}
	}
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

	// --- Build job: export step (issue #249 / PUB-03: gated only by the manual
	// recovery path, which restores the workspace from a snapshot artifact
	// instead of the DB) ---
	exportStep := findStepByID(t, wf, "build", "export")
	exportIf, ok := exportStep["if"].(string)
	if !ok {
		t.Fatal("export step must have an if condition (issue #249: restore gate)")
	}
	if !strings.Contains(exportIf, "inputs.restore_artifact == ''") {
		t.Error("export step if must be gated off when restoring a snapshot (inputs.restore_artifact == '' )")
	}
	if hasAlwaysIf(exportStep) {
		t.Error("export step must NOT use if: always()")
	}
	if _, hasCOE := exportStep["continue-on-error"]; hasCOE {
		t.Error("export step must NOT have continue-on-error")
	}
	exportScript := stepRunScript(exportStep)
	if !strings.Contains(exportScript, "export run") {
		t.Error("export step must invoke 'export run'")
	}

	// --- Build job: snapshot package/validate/upload steps (issue #249 /
	// PUB-03 AC-1..AC-3). Gated on export success; the upload names the
	// artifact by the content-derived snapshotId with retention-days: 7 and
	// if-no-files-found: error (missing package fails the job). ---
	pkgStep := findStepByID(t, wf, "build", "package-snapshot")
	pkgIf, ok := pkgStep["if"].(string)
	if !ok || !strings.Contains(pkgIf, "steps.export.conclusion == 'success'") {
		t.Error("package-snapshot step must gate on export success")
	}
	pkgScript := stepRunScript(pkgStep)
	if !strings.Contains(pkgScript, "export package") {
		t.Error("package-snapshot step must invoke 'export package'")
	}
	if !strings.Contains(pkgScript, "snapshotId") {
		t.Error("package-snapshot step must echo snapshotId to GITHUB_OUTPUT")
	}
	validateStep := findStepByNameIn(t, wf, "build", "Validate snapshot artifact")
	if _, hasIf := validateStep["if"]; !hasIf {
		t.Error("Validate snapshot artifact step must have an if condition")
	}
	valScript := stepRunScript(validateStep)
	if !strings.Contains(valScript, "export validate") {
		t.Error("Validate snapshot artifact step must invoke 'export validate'")
	}
	uploadName := "Upload site-snapshot artifact"
	uploadStep := findStepByNameIn(t, wf, "build", uploadName)
	if uploadStep == nil {
		t.Error(uploadName + " step must exist")
	} else {
		withBlock, _ := uploadStep["with"].(map[string]any)
		if withBlock == nil {
			t.Error("snapshot upload must have a with block")
		} else {
			nameVal, _ := withBlock["name"].(string)
			if !strings.Contains(nameVal, "site-snapshot-${{ steps.package-snapshot.outputs.snapshot_id }}") {
				t.Error("snapshot artifact must be named site-snapshot-<snapshotId>")
			}
			switch rd := withBlock["retention-days"].(type) {
			case int:
				if rd != 7 {
					t.Error("snapshot artifact must retain for 7 days")
				}
			case float64:
				if rd != 7 {
					t.Error("snapshot artifact must retain for 7 days")
				}
			default:
				t.Error("snapshot upload must set retention-days: 7")
			}
			if withBlock["if-no-files-found"] != "error" {
				t.Error("snapshot upload must set if-no-files-found: error (fail closed)")
			}
		}
	}

	// --- Build job: publish-gate step gated on export success (AC-2, AC-3) ---
	// Issue #229: the gate compares against the deployed manifest (not HEAD).
	// Issue #247: the manifest is read from the durable store (Turso) via
	// `publish manifest get` BEFORE the gate, and the gate is invoked with
	// --require-manifest so a fresh checkout with no stored manifest DENIES
	// (fail closed).
	publishGateStep := findStepByID(t, wf, "build", "publish-gate")
	gateIf, ok := publishGateStep["if"].(string)
	if !ok {
		t.Fatal("publish-gate step must have an if condition")
	}
	if !strings.Contains(gateIf, "steps.export.conclusion == 'success'") {
		t.Errorf("publish-gate if = %q, must gate on export success", gateIf)
	}
	if hasAlwaysIf(publishGateStep) {
		t.Error("publish-gate step must NOT use if: always()")
	}
	gateScript := stepRunScript(publishGateStep)
	if !strings.Contains(gateScript, "publish gate") {
		t.Error("publish-gate step must invoke 'publish gate'")
	}
	if !strings.Contains(gateScript, "web/data/deployed.json") {
		t.Error("publish-gate step must compare against web/data/deployed.json (not git show HEAD:)")
	}
	// Issue #247 / AC-6: --require-manifest is mandatory.
	if !strings.Contains(gateScript, "--require-manifest") {
		t.Error("publish-gate step must invoke with --require-manifest (fail closed, issue #247)")
	}

	// Issue #247: manifest get step must run before the gate (load manifest from
	// the durable store). It is gated on export success.
	manifestGetStep := findStepByID(t, wf, "build", "manifest-get")
	mgIf, ok := manifestGetStep["if"].(string)
	if !ok || !strings.Contains(mgIf, "steps.export.conclusion == 'success'") {
		t.Errorf("manifest-get if = %q, must gate on export success", mgIf)
	}
	mgScript := stepRunScript(manifestGetStep)
	if !strings.Contains(mgScript, "publish manifest get") {
		t.Error("manifest-get step must invoke 'publish manifest get'")
	}
	if !strings.Contains(mgScript, "--output web/data/deployed.json") {
		t.Error("manifest-get step must write to web/data/deployed.json")
	}

	// --- Concurrency: shared group + cancel-in-progress: false (AC-3) ---
	// Issue #229: the shared concurrency group serializes all Pages deploys
	// across workflows; cancel-in-progress: false prevents cancellation
	// inside the publish window.
	concurrency, ok := wf["concurrency"].(map[string]any)
	if !ok {
		t.Fatal("refresh-news.yml: missing concurrency block")
	}
	if group, _ := concurrency["group"].(string); group != "huddlescope-pages-deploy" {
		t.Errorf("concurrency.group = %q, want huddlescope-pages-deploy (shared with site.yml)", group)
	}
	if cancel, _ := concurrency["cancel-in-progress"].(bool); cancel != false {
		t.Error("concurrency.cancel-in-progress must be false (AC-3: no cancel during publish)")
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

	// --- Build job: commit-and-push-retention step (AC-7, issue #247 / D5) ---
	// Issue #247: the news-data commit step is removed; the only commit+push
	// step is the retention-only commit, which stages ONLY archive/retention
	// (never web/data). It is gated on run-retention AND the retention export
	// succeeding (NOT on export success).
	commitPushStep := findStepByID(t, wf, "build", "commit-and-push-retention")
	cpsIf, ok := commitPushStep["if"].(string)
	if !ok {
		t.Fatal("commit-and-push-retention step must have an if condition")
	}
	if !strings.Contains(cpsIf, "inputs.run-retention") || !strings.Contains(cpsIf, "steps.retention-export.conclusion == 'success'") {
		t.Errorf("commit-and-push-retention if = %q, must gate on run-retention AND retention-export success", cpsIf)
	}
	if hasAlwaysIf(commitPushStep) {
		t.Error("commit-and-push-retention step must NOT use if: always()")
	}
	if _, hasCOE := commitPushStep["continue-on-error"]; hasCOE {
		t.Error("commit-and-push-retention step must NOT have continue-on-error")
	}
	// Job-level permissions on build: contents: write (GitHub Actions does not
	// support step-level permissions).
	jobPerms, ok := buildJob["permissions"].(map[string]any)
	if !ok {
		t.Fatal("build job must have job-level permissions")
	}
	if contents, ok := jobPerms["contents"].(string); !ok || contents != "write" {
		t.Errorf("build job permissions.contents = %v, want \"write\"", jobPerms["contents"])
	}
	if _, hasStepPerms := commitPushStep["permissions"]; hasStepPerms {
		t.Error("commit-and-push-retention step must NOT have step-level permissions")
	}
	// The step must commit to a short-lived branch, open a PR, wait for the
	// Build check, and squash-merge. It must stage archive/retention and
	// NEVER web/data (issue #247 / AC-3).
	cpsScript := stepRunScript(commitPushStep)
	if strings.Contains(cpsScript, "[skip ci]") {
		t.Error("commit-and-push-retention must NOT use [skip ci]: the Build check is required by the main ruleset")
	}
	if !strings.Contains(cpsScript, "git checkout -b") || !strings.Contains(cpsScript, "gh pr create") {
		t.Error("commit-and-push-retention must commit to a short-lived branch and open a PR")
	}
	if !strings.Contains(cpsScript, "gh pr merge") {
		t.Error("commit-and-push-retention must squash-merge the PR after the Build check passes")
	}
	if !strings.Contains(cpsScript, "git add archive/retention") {
		t.Error("commit-and-push-retention must stage archive/retention")
	}
	// Issue #247 / AC-3: the retention step must NOT stage web/data.
	if strings.Contains(cpsScript, "git add web/data") {
		t.Error("commit-and-push-retention must NOT stage web/data (issue #247: no news-data commits)")
	}
	if !strings.Contains(cpsScript, "pr_num=") || !strings.Contains(cpsScript, "branch=") {
		t.Error("commit-and-push-retention must write pr_num and branch to GITHUB_OUTPUT for the cleanup step")
	}
	// Conflict recovery: after the Build check, verify mergeability and
	// re-base onto origin/main when main moved (bounded re-base attempts).
	if !strings.Contains(cpsScript, "mergeable") || !strings.Contains(cpsScript, "git reset --hard origin/main") {
		t.Error("commit-and-push-retention must check mergeability and re-base onto origin/main when main moved")
	}

	// --- Build job: retention steps (issue #120 / HS-065) ---
	retExport := findStepByID(t, wf, "build", "retention-export")
	if ri, ok := retExport["if"].(string); !ok || !strings.Contains(ri, "inputs.run-retention") {
		t.Error("retention-export must be gated on inputs.run-retention")
	}
	retApply := findStepByID(t, wf, "build", "retention-apply")
	ai, ok := retApply["if"].(string)
	if !ok || !strings.Contains(ai, "inputs.run-retention") || !strings.Contains(ai, "steps.commit-and-push-retention.conclusion == 'success'") {
		t.Errorf("retention-apply if = %q, must gate on run-retention AND the retention commit step success (package persisted before apply)", ai)
	}
	// The package must ship in the retention PR: export before the commit step.
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
	if ei, ci := stepIdx("retention-export"), stepIdx("commit-and-push-retention"); ei < 0 || ci < 0 || ei >= ci {
		t.Error("retention-export must run before commit-and-push-retention so the package ships in the same PR")
	}
	// Cleanup step: closes the retention PR and deletes the branch when the run
	// did not merge (cancelled or failed).
	cleanup := findStepByID(t, wf, "build", "cleanup-retention-pr")
	ciCond, ok := cleanup["if"].(string)
	if !ok || !strings.Contains(ciCond, "always()") || !strings.Contains(ciCond, "steps.commit-and-push-retention.outcome != 'success'") || !strings.Contains(ciCond, "pr_num") {
		t.Errorf("cleanup-retention-pr if = %q, must run on always() when the retention step did not succeed and a PR was opened", ciCond)
	}
	cr := stepRunScript(cleanup)
	if !strings.Contains(cr, "gh pr close") {
		t.Error("cleanup-retention-pr must close the orphaned PR")
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

// TestWorkflowContract_Site_PublishGate validates the publish gate invariants
// in site.yml (issue #229 / AC-2, AC-3; issue #247 / PUB-01 AC-4, AC-6, AC-8).
func TestWorkflowContract_Site_PublishGate(t *testing.T) {
	t.Parallel()

	wfPath := filepath.Join("..", "..", ".github", "workflows", "site.yml")
	wf := loadWorkflow(t, wfPath)

	// --- Concurrency: shared group + cancel-in-progress: false (AC-3) ---
	concurrency, ok := wf["concurrency"].(map[string]any)
	if !ok {
		t.Fatal("site.yml: missing concurrency block")
	}
	if group, _ := concurrency["group"].(string); group != "huddlescope-pages-deploy" {
		t.Errorf("site.yml concurrency.group = %q, want huddlescope-pages-deploy (shared with refresh-news.yml)", group)
	}
	if cancel, _ := concurrency["cancel-in-progress"].(bool); cancel != false {
		t.Error("site.yml concurrency.cancel-in-progress must be false")
	}

	// --- Deploy job: pinned feature branch (AC-3) ---
	jobs := strMap(t, wf["jobs"])
	deployJob, ok := jobs["deploy"].(map[string]any)
	if !ok {
		t.Fatal("site.yml: missing deploy job")
	}
	steps := deployJob["steps"].([]any)
	// Find the checkout step.
	var checkoutStep map[string]any
	for _, s := range steps {
		step, ok := s.(map[string]any)
		if !ok {
			continue
		}
		if name, _ := step["name"].(string); strings.HasPrefix(name, "Checkout repository") {
			checkoutStep = step
			break
		}
	}
	if checkoutStep == nil {
		t.Fatal("site.yml deploy job: missing Checkout step")
	}
	withBlock, ok := checkoutStep["with"].(map[string]any)
	if !ok {
		t.Fatal("site.yml deploy job: Checkout step missing with block")
	}
	if ref, _ := withBlock["ref"].(string); !strings.Contains(ref, "source-branch") {
		t.Errorf("site.yml deploy job: Checkout ref = %q, must check out the feature branch (inputs.source-branch)", ref)
	}

	// Issue #247: the deploy job must re-project the snapshot from Turso
	// (workspace, not a committed file) and read the manifest from the store.
	// AC-4: no "git archive" restore of committed data.
	var reprojected bool
	var manifestGet bool
	var gateRequiresManifest bool
	var foundGitArchive bool
	for _, s := range steps {
		step, ok := s.(map[string]any)
		if !ok {
			continue
		}
		run := stepRunScript(step)
		if strings.Contains(run, "export run") {
			reprojected = true
		}
		if strings.Contains(run, "publish manifest get") {
			manifestGet = true
		}
		if strings.Contains(run, "publish gate") && strings.Contains(run, "--require-manifest") {
			gateRequiresManifest = true
		}
		if strings.Contains(run, "git archive") {
			foundGitArchive = true
		}
	}
	if !reprojected {
		t.Error("site.yml deploy job must re-project the snapshot from Turso (export run)")
	}
	if !manifestGet {
		t.Error("site.yml deploy job must read the manifest from the store (publish manifest get)")
	}
	if !gateRequiresManifest {
		t.Error("site.yml publish gate must use --require-manifest (fail closed, issue #247)")
	}
	if foundGitArchive {
		t.Error("site.yml deploy job must NOT use 'git archive' to restore committed data (issue #247 / AC-4)")
	}

	// --- Deploy job: publish gate step (AC-2) ---
	var gateStep map[string]any
	for _, s := range steps {
		step, ok := s.(map[string]any)
		if !ok {
			continue
		}
		if id, _ := step["id"].(string); id == "publish-gate" {
			gateStep = step
			break
		}
	}
	if gateStep == nil {
		t.Fatal("site.yml deploy job: missing publish-gate step")
	}
	gateScript := stepRunScript(gateStep)
	if !strings.Contains(gateScript, "publish gate") {
		t.Error("site.yml publish-gate step must invoke 'publish gate'")
	}
	if !strings.Contains(gateScript, "web/data/deployed.json") {
		t.Error("site.yml publish-gate step must compare against web/data/deployed.json (the stored manifest)")
	}
	if !strings.Contains(gateScript, "--require-manifest") {
		t.Error("site.yml publish-gate step must invoke with --require-manifest (issue #247)")
	}
	// Issue #247 / AC-8: the manifest put step records the deploy in the store
	// (no git).
	var foundManifestPut bool
	for _, s := range steps {
		step, ok := s.(map[string]any)
		if !ok {
			continue
		}
		if strings.Contains(stepRunScript(step), "publish manifest put") {
			foundManifestPut = true
		}
	}
	if !foundManifestPut {
		t.Error("site.yml deploy job must record the manifest in the store (publish manifest put) after deploy")
	}

	// --- Deploy job: demo marker gate kept (AC-5) ---
	var foundMarkerGate bool
	for _, s := range steps {
		step, ok := s.(map[string]any)
		if !ok {
			continue
		}
		if name, _ := step["name"].(string); name == "Verify no demo markers" {
			foundMarkerGate = true
			run := stepRunScript(step)
			if !strings.Contains(run, "MARKERS") {
				t.Error("site.yml demo marker gate must define MARKERS")
			}
		}
	}
	if !foundMarkerGate {
		t.Error("site.yml: missing 'Verify no demo markers' step (AC-5)")
	}
}

// TestWorkflowContract_RefreshNews_VerifyHTML (issue #250 / PUB-04 AC6, R7):
// refresh-news.yml must have a verify-html step that fails closed, is ordered
// after make-site and before Pages uploads, and the manifest put must pass
// new v2 flags.
func TestWorkflowContract_RefreshNews_VerifyHTML(t *testing.T) {
	t.Parallel()
	wf := loadWorkflow(t, filepath.Join("..", "..", ".github", "workflows", "refresh-news.yml"))

	// Find the verify-html step in the build job.
	verifyStep := findStepByID(t, wf, "build", "verify-html")
	if verifyStep == nil {
		t.Fatal("refresh-news.yml: build job must have a step with id 'verify-html' (issue #250)")
	}
	// verify-html must fail closed (no continue-on-error).
	if coe, ok := verifyStep["continue-on-error"].(bool); ok && coe {
		t.Error("verify-html step must NOT have continue-on-error: true (it must fail closed)")
	}

	// The manifest put must pass new v2 flags.
	putStep := findStepByID(t, wf, "deploy", "stamp-manifest")
	if putStep == nil {
		t.Fatal("refresh-news.yml: deploy job must have a step with id 'stamp-manifest'")
	}
	runBlock, _ := putStep["run"].(string)
	if !strings.Contains(runBlock, "--run-id") {
		t.Error("manifest put must pass --run-id (issue #250)")
	}
	if !strings.Contains(runBlock, "--source-commit") {
		t.Error("manifest put must pass --source-commit (issue #250)")
	}

	// A report upload step must be present in the build job.
	jobs := strMap(t, wf["jobs"])
	buildJob, ok := jobs["build"].(map[string]any)
	if !ok {
		t.Fatal("refresh-news.yml: missing build job")
	}
	steps, _ := buildJob["steps"].([]any)
	var reportUploadFound bool
	for _, s := range steps {
		step, ok := s.(map[string]any)
		if !ok {
			continue
		}
		name, _ := step["name"].(string)
		uses, _ := step["uses"].(string)
		if strings.Contains(uses, "upload-artifact") &&
			(strings.Contains(name, "publish diagnostic") || strings.Contains(name, "report")) {
			reportUploadFound = true
			break
		}
	}
	if !reportUploadFound {
		t.Error("refresh-news.yml: build job must have a publish diagnostic report upload step (issue #250)")
	}

	// Concurrency invariants retained.
	raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "refresh-news.yml"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if !strings.Contains(string(raw), "cancel-in-progress: false") {
		t.Error("refresh-news.yml: concurrency group must retain cancel-in-progress: false")
	}
}

// TestWorkflowContract_Site_VerifyHTML (issue #250 / PUB-04 AC2, AC6, R7):
// site.yml must have a verify-html step that fails closed, the gate must set
// a deploy output (skip-identical quiesces), and the Pages steps must gate on
// the deploy output.
func TestWorkflowContract_Site_VerifyHTML(t *testing.T) {
	t.Parallel()
	wf := loadWorkflow(t, filepath.Join("..", "..", ".github", "workflows", "site.yml"))

	// Find the verify-html step in the deploy job.
	verifyStep := findStepByID(t, wf, "deploy", "verify-html")
	if verifyStep == nil {
		t.Fatal("site.yml: deploy job must have a step with id 'verify-html' (issue #250)")
	}
	if coe, ok := verifyStep["continue-on-error"].(bool); ok && coe {
		t.Error("site.yml verify-html must NOT have continue-on-error: true")
	}

	// The gate must set a deploy output.
	gateStep := findStepByID(t, wf, "deploy", "publish-gate")
	if gateStep == nil {
		t.Fatal("site.yml: deploy job must have a step with id 'publish-gate'")
	}
	gateRun, _ := gateStep["run"].(string)
	if !strings.Contains(gateRun, "deploy=true") || !strings.Contains(gateRun, "deploy=false") {
		t.Error("site.yml publish-gate must set deploy=true/false output (issue #250)")
	}
	if !strings.Contains(gateRun, "skip-identical") {
		t.Error("site.yml publish-gate must handle skip-identical (quiesce deploy)")
	}

	// The Pages steps must gate on the deploy output.
	jobs := strMap(t, wf["jobs"])
	deployJob, ok := jobs["deploy"].(map[string]any)
	if !ok {
		t.Fatal("site.yml: missing deploy job")
	}
	steps, _ := deployJob["steps"].([]any)
	for _, s := range steps {
		step, ok := s.(map[string]any)
		if !ok {
			continue
		}
		name, _ := step["name"].(string)
		if strings.Contains(name, "upload artifact to github pages") || strings.Contains(name, "configure github pages") {
			ifCond, _ := step["if"].(string)
			if !strings.Contains(ifCond, "publish-gate.outputs.deploy") {
				t.Errorf("site.yml %q must gate on publish-gate.outputs.deploy (issue #250)", name)
			}
		}
	}

	// The manifest put must pass new v2 flags.
	putStep := findStepByNameIn(t, wf, "deploy", "Record deployed manifest in store")
	if putStep == nil {
		t.Fatal("site.yml: deploy job must have 'Record deployed manifest in store' step")
	}
	putRun, _ := putStep["run"].(string)
	if !strings.Contains(putRun, "--run-id") {
		t.Error("site.yml manifest put must pass --run-id (issue #250)")
	}
	if !strings.Contains(putRun, "--source-commit") {
		t.Error("site.yml manifest put must pass --source-commit (issue #250)")
	}
}
