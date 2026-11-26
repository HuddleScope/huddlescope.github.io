// Package sqlite: deployed-manifest store (issue #247 / PUB-01, v2 columns
// for issue #250 / PUB-04).
//
// Issue #229 tracked the deployed manifest in web/data/deployed.json (a
// committed file). Issue #247 moves that generated state out of Git into the
// shared database (deployed_manifests table, migration 011). Issue #250 adds
// the v2 binding columns (migration 012): snapshotId, lastIngestAt,
// sourceCommit, runID, overrideReason. These driver-agnostic functions back
// the turso/sqlite manifest store and the `publish manifest get` / `put` CLI
// subcommands.
//
// The manifest is non-secret by construction: version, RFC3339 timestamps, a
// content SHA-256, a commit SHA, integer counts, and v2 binding fields. The
// JSON shape is the same as domain.DeployedManifest so the publish gate can
// consume it unchanged.
//
// Read compatibility: a legacy v1 row (version "1", empty v2 columns) is
// returned as-is — the gate accepts v1∪v2. Writes always normalize to the
// current version (domain.ManifestVersion = "2").
package sqlite

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// GetDeployedManifest reads the single deployed-manifest row. A missing row
// returns (nil, nil) (never deployed). A malformed row (unknown version)
// returns an error (fail closed — the gate must not treat corrupt state as
// "no manifest"). Legacy v1 rows are returned unchanged (v2 fields empty).
func GetDeployedManifest(db *sql.DB) (*domain.DeployedManifest, error) {
	row := db.QueryRow(`SELECT version, deployed_at, generated_at, "commit", content_sha256, stories, latest,
		COALESCE(snapshot_id, ''), COALESCE(last_ingest_at, ''), COALESCE(source_commit, ''),
		COALESCE(run_id, ''), COALESCE(override_reason, '')
		FROM deployed_manifests WHERE manifest_key = '1'`)
	var m domain.DeployedManifest
	err := row.Scan(&m.Version, &m.DeployedAt, &m.GeneratedAt, &m.Commit, &m.ContentSHA256,
		&m.Stories, &m.Latest, &m.SnapshotID, &m.LastIngestAt, &m.SourceCommit, &m.RunID, &m.OverrideReason)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("read deployed manifest: %w", err)
	}
	// Fail closed only on an unknown version. v1 and v2 are both valid; the
	// gate accepts v1∪v2 (issue #250 / PUB-04 AC4, risk 3).
	if m.Version != domain.ManifestVersion && m.Version != domain.LegacyManifestVersion {
		return nil, fmt.Errorf("deployed manifest version %q, want %q or %q", m.Version, domain.ManifestVersion, domain.LegacyManifestVersion)
	}
	return &m, nil
}

// PutDeployedManifest upserts the single deployed-manifest row. The manifest
// version is normalized to domain.ManifestVersion ("2") on write so a
// caller-specified version cannot corrupt the store; v2 binding fields are
// written as-is (empty stays empty).
func PutDeployedManifest(db *sql.DB, m domain.DeployedManifest) error {
	if m.DeployedAt == "" || m.GeneratedAt == "" || m.ContentSHA256 == "" {
		return fmt.Errorf("deployed manifest missing required fields (deployedAt/generatedAt/contentSha256)")
	}
	if m.Version == "" {
		m.Version = domain.ManifestVersion
	}
	_, err := db.Exec(`INSERT INTO deployed_manifests
		(manifest_key, version, deployed_at, generated_at, "commit", content_sha256, stories, latest,
		 snapshot_id, last_ingest_at, source_commit, run_id, override_reason)
		VALUES ('1', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(manifest_key) DO UPDATE SET
			version = excluded.version,
			deployed_at = excluded.deployed_at,
			generated_at = excluded.generated_at,
			"commit" = excluded."commit",
			content_sha256 = excluded.content_sha256,
			stories = excluded.stories,
			latest = excluded.latest,
			snapshot_id = excluded.snapshot_id,
			last_ingest_at = excluded.last_ingest_at,
			source_commit = excluded.source_commit,
			run_id = excluded.run_id,
			override_reason = excluded.override_reason`,
		m.Version, m.DeployedAt, m.GeneratedAt, m.Commit, m.ContentSHA256, m.Stories, m.Latest,
		m.SnapshotID, m.LastIngestAt, m.SourceCommit, m.RunID, m.OverrideReason)
	if err != nil {
		return fmt.Errorf("upsert deployed manifest: %w", err)
	}
	return nil
}
