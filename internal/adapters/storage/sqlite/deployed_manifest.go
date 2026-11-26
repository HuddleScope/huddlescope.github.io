// Package sqlite: deployed-manifest store (issue #247 / PUB-01).
//
// Issue #229 tracked the deployed manifest in web/data/deployed.json (a
// committed file). Issue #247 moves that generated state out of Git into the
// shared database (deployed_manifests table, migration 011). These
// driver-agnostic functions back the turso/sqlite manifest store and the
// `publish manifest get` / `publish manifest put` CLI subcommands.
//
// The manifest is non-secret by construction: version, RFC3339 timestamps, a
// content SHA-256, a commit SHA, and integer counts. The JSON shape is the
// same as domain.DeployedManifest so the publish gate can consume it unchanged.
package sqlite

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// GetDeployedManifest reads the single deployed-manifest row. A missing row
// returns (nil, nil) (never deployed). A malformed row (bad version) returns
// an error (fail closed — the gate must not treat corrupt state as "no
// manifest").
func GetDeployedManifest(db *sql.DB) (*domain.DeployedManifest, error) {
	row := db.QueryRow(`SELECT version, deployed_at, generated_at, "commit", content_sha256, stories, latest
		FROM deployed_manifests WHERE manifest_key = '1'`)
	var m domain.DeployedManifest
	err := row.Scan(&m.Version, &m.DeployedAt, &m.GeneratedAt, &m.Commit, &m.ContentSHA256, &m.Stories, &m.Latest)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("read deployed manifest: %w", err)
	}
	if m.Version != domain.ManifestVersion {
		return nil, fmt.Errorf("deployed manifest version %q, want %q", m.Version, domain.ManifestVersion)
	}
	return &m, nil
}

// PutDeployedManifest upserts the single deployed-manifest row. The manifest
// version is normalized to domain.ManifestVersion on write so a
// caller-specified version cannot corrupt the store.
func PutDeployedManifest(db *sql.DB, m domain.DeployedManifest) error {
	if m.DeployedAt == "" || m.GeneratedAt == "" || m.ContentSHA256 == "" {
		return fmt.Errorf("deployed manifest missing required fields (deployedAt/generatedAt/contentSha256)")
	}
	if m.Version == "" {
		m.Version = domain.ManifestVersion
	}
	_, err := db.Exec(`INSERT INTO deployed_manifests
		(manifest_key, version, deployed_at, generated_at, "commit", content_sha256, stories, latest)
		VALUES ('1', ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(manifest_key) DO UPDATE SET
			version = excluded.version,
			deployed_at = excluded.deployed_at,
			generated_at = excluded.generated_at,
			"commit" = excluded."commit",
			content_sha256 = excluded.content_sha256,
			stories = excluded.stories,
			latest = excluded.latest`,
		m.Version, m.DeployedAt, m.GeneratedAt, m.Commit, m.ContentSHA256, m.Stories, m.Latest)
	if err != nil {
		return fmt.Errorf("upsert deployed manifest: %w", err)
	}
	return nil
}
