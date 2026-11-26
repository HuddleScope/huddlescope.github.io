// Package domain: deployed-manifest value type (issue #247 / PUB-01).
//
// The deployed manifest is the durably-stored record of the last successful
// deployment (issue #229 introduced it as a tracked web/data/deployed.json
// file; issue #247 moves it into the shared database so generated state never
// requires a Git commit). It is non-secret by construction: version, RFC3339
// timestamps, a content SHA-256, a commit SHA, and integer counts.
//
// The type lives in domain (not application) so both the publish gate
// (application) and the storage adapters (sqlite/turso) can reference it
// without an import cycle (application -> storage -> sqlite -> application).
package domain

// ManifestVersion is the current version of the deployed manifest format.
const ManifestVersion = "1"

// DeployedManifest is the record of the last successfully deployed snapshot
// (version "1"). JSON field names match the historical tracked file so the
// manifest JSON the gate consumes is unchanged.
type DeployedManifest struct {
	Version       string `json:"version"`
	DeployedAt    string `json:"deployedAt"`
	GeneratedAt   string `json:"generatedAt"`
	Commit        string `json:"commit"`
	ContentSHA256 string `json:"contentSha256"`
	Stories       int    `json:"stories"`
	Latest        int    `json:"latest"`
}
