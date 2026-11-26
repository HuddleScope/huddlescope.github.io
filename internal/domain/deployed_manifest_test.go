package domain

import "testing"

// TestDeployedManifest_V2_Validate (issue #250 / PUB-04 AC4): a v2 manifest
// with all binding fields set validates successfully.
func TestDeployedManifest_V2_Validate(t *testing.T) {
	m := DeployedManifest{
		Version:       ManifestVersion, // "2"
		DeployedAt:    "2026-01-01T00:00:00Z",
		GeneratedAt:   "2026-01-01T00:00:00Z",
		Commit:        "abc123",
		ContentSHA256: "hash123",
		Stories:       5,
		Latest:        3,
		SnapshotID:    "aabbccdd00112233445566778899aabbccddeeff00112233445566778899aabb",
		LastIngestAt:  "2026-01-01T00:00:00Z",
		SourceCommit:  "src456",
		RunID:         "12345",
	}
	if err := m.Validate(); err != nil {
		t.Errorf("v2 manifest with binding fields: Validate() error = %v", err)
	}
}

// TestDeployedManifest_V1_Validate (issue #250 / PUB-04 AC4): a legacy v1
// manifest (no binding fields) still validates (rollout compatibility).
func TestDeployedManifest_V1_Validate(t *testing.T) {
	m := DeployedManifest{
		Version:       LegacyManifestVersion, // "1"
		DeployedAt:    "2026-01-01T00:00:00Z",
		GeneratedAt:   "2026-01-01T00:00:00Z",
		Commit:        "abc123",
		ContentSHA256: "hash123",
		Stories:       5,
		Latest:        3,
	}
	if err := m.Validate(); err != nil {
		t.Errorf("v1 manifest: Validate() error = %v (rollout compatibility)", err)
	}
}

// TestDeployedManifest_UnknownVersion_Validate (issue #250 / PUB-04 AC4): an
// unknown version fails closed.
func TestDeployedManifest_UnknownVersion_Validate(t *testing.T) {
	m := DeployedManifest{
		Version:       "999",
		DeployedAt:    "2026-01-01T00:00:00Z",
		GeneratedAt:   "2026-01-01T00:00:00Z",
		Commit:        "abc123",
		ContentSHA256: "hash123",
		Stories:       5,
	}
	if err := m.Validate(); err == nil {
		t.Error("unknown version: expected Validate() error")
	}
}

// TestDeployedManifest_EmptyVersion_Validate: an empty version is malformed
// (not "absent" — absent is handled by LoadDeployedManifest returning nil).
func TestDeployedManifest_EmptyVersion_Validate(t *testing.T) {
	m := DeployedManifest{
		DeployedAt:    "2026-01-01T00:00:00Z",
		GeneratedAt:   "2026-01-01T00:00:00Z",
		Commit:        "abc123",
		ContentSHA256: "hash123",
		Stories:       5,
	}
	if err := m.Validate(); err == nil {
		t.Error("empty version: expected Validate() error (malformed)")
	}
}
