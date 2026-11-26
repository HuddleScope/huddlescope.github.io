// Package domain: deployed-manifest store interface (issue #247 / PUB-01).
//
// A DeployedManifestStore is the durable home for the deployed manifest that
// the publish gate compares candidates against. Issue #247 moves this state
// out of Git (previously the tracked web/data/deployed.json) into the shared
// database so a fresh checkout can evaluate the gate without any committed
// file.
//
// The store is decoupled from the story Repository (engine independence) the
// same way the retention service and ingest-run repository are. Get returns
// (nil, nil) when no manifest is stored (never deployed); a non-nil error
// means a read failure and callers must fail closed. Put validates and
// upserts the single manifest row.
package domain

// DeployedManifestStore persists the single deployed-manifest row.
type DeployedManifestStore interface {
	// Get returns the stored manifest, or (nil, nil) when none is stored.
	// A non-nil error is a read failure (callers fail closed).
	Get() (*DeployedManifest, error)
	// Put validates and upserts the manifest. A zero/required-field-missing
	// manifest is an error.
	Put(m DeployedManifest) error
	// Close releases any underlying handle. It is safe to call multiple
	// times and on a nil receiver is not required (callers guard).
	Close() error
}
