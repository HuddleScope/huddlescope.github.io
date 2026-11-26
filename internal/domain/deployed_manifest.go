// Package domain: deployed-manifest value type (issue #247 / PUB-01, v2 for
// issue #250 / PUB-04).
//
// The deployed manifest is the durably-stored record of the last successful
// deployment (issue #229 introduced it as a tracked web/data/deployed.json
// file; issue #247 moves it into the shared database so generated state never
// requires a Git commit). It is non-secret by construction: version, RFC3339
// timestamps, a content SHA-256, a commit SHA, and integer counts.
//
// Version "2" (issue #250 / PUB-04) binds the manifest to the exact validated
// artifact and run: snapshotId (the content-derived site-snapshot ID),
// lastIngestAt, sourceCommit, runID, and an audited overrideReason for
// bootstrap override. Version "1" remains readable — missing v2 fields are
// zero values — so the gate accepts v1∪v2 during rollout (AC4, risk 3).
//
// The type lives in domain (not application) so both the publish gate
// (application) and the storage adapters (sqlite/turso) can reference it
// without an import cycle (application -> storage -> sqlite -> application).
package domain

import (
	"fmt"
	"regexp"
)

// ManifestVersion is the current version of the deployed manifest format.
const ManifestVersion = "2"

// LegacyManifestVersion is the pre-#250 manifest version. Rows written as v1
// remain valid: the gate and the stores accept v1 on read (missing v2 fields
// are zero values) and always write v2.
const LegacyManifestVersion = "1"

// DeployedManifest is the record of the last successfully deployed snapshot.
// JSON field names match the historical tracked file for the v1 fields so the
// manifest JSON the gate consumes is unchanged; v2 fields are additive.
type DeployedManifest struct {
	Version       string `json:"version"`
	DeployedAt    string `json:"deployedAt"`
	GeneratedAt   string `json:"generatedAt"`
	Commit        string `json:"commit"`
	ContentSHA256 string `json:"contentSha256"`
	Stories       int    `json:"stories"`
	Latest        int    `json:"latest"`
	// v2 (issue #250 / PUB-04): binding to the exact validated artifact/run.
	// Empty on legacy v1 rows (readable, valid).
	SnapshotID     string `json:"snapshotId,omitempty"`
	LastIngestAt   string `json:"lastIngestAt,omitempty"`   // RFC3339 UTC or "unknown"
	SourceCommit   string `json:"sourceCommit,omitempty"`   // git SHA or "unknown"
	RunID          string `json:"runId,omitempty"`          // CI run identity
	OverrideReason string `json:"overrideReason,omitempty"` // audited bootstrap override
}

// Validate checks the deployed manifest for the fail-closed invariants that
// apply to every version: known version, non-empty required timestamps and
// content hash, non-negative counts. v1 rows pass (v2 fields empty); v2 rows
// additionally require snapshotId to be a 64-char lowercase sha256 hex when
// present.
func (m DeployedManifest) Validate() error {
	switch m.Version {
	case ManifestVersion, LegacyManifestVersion:
	default:
		return fmt.Errorf("deployed manifest version %q, want %q or %q", m.Version, ManifestVersion, LegacyManifestVersion)
	}
	if m.DeployedAt == "" {
		return fmt.Errorf("deployed manifest missing deployedAt")
	}
	if m.GeneratedAt == "" {
		return fmt.Errorf("deployed manifest missing generatedAt")
	}
	if m.ContentSHA256 == "" {
		return fmt.Errorf("deployed manifest missing contentSha256")
	}
	if m.Stories < 0 || m.Latest < 0 {
		return fmt.Errorf("deployed manifest counts must be non-negative (stories=%d latest=%d)", m.Stories, m.Latest)
	}
	if m.SnapshotID != "" && !manifestSHA256Re.MatchString(m.SnapshotID) {
		return fmt.Errorf("deployed manifest snapshotId %q is not 64-char lowercase sha256 hex", m.SnapshotID)
	}
	return nil
}

// manifestSHA256Re matches a 64-char lowercase sha256 hex digest (shared
// shape with the site-snapshot manifest ID, issue #249 / PUB-03).
var manifestSHA256Re = regexp.MustCompile(`^[0-9a-f]{64}$`)
