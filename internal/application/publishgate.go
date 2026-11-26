// Package application: publish gate (issue #229).
//
// The publish gate is the single authoritative decision that prevents a stale
// or empty snapshot from overwriting a newer, populated deployment. It compares
// a candidate snapshot (the freshly exported homepage.json) against the last
// deployed manifest (web/data/deployed.json) and returns a verdict:
//
//   - "allow": the candidate is newer or equal and has stories; deploy it.
//   - "skip-identical": the candidate is content-identical to the deployed
//     manifest; the deploy is a no-op (quiesces the pipeline).
//   - "deny": the candidate is older, or zero-over-N; retain the last-good
//     deployment.
//
// A missing or invalid candidate/manifest fails closed (error), never treated
// as a "deliberate empty" (AC-4).
package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// DeployedManifest is the record of the last successfully deployed snapshot.
// Issue #247 / PUB-01: the canonical type lives in domain so both the gate and
// the storage adapters can reference it without an import cycle; this alias
// keeps the existing application.DeployedManifest name stable.
type DeployedManifest = domain.DeployedManifest

// ManifestVersion is the current version of the deployed manifest format.
//
// issue #250 / PUB-04: the gate accepts both the current v2 and the legacy v1
// manifest versions on read (v1 rows are readable with zero-value v2 fields);
// writes always produce v2. See domain.LegacyManifestVersion.
const ManifestVersion = domain.ManifestVersion

// LegacyManifestVersion is the pre-#250 manifest version (still readable).
const LegacyManifestVersion = domain.LegacyManifestVersion

// GateVerdict is the outcome of a publish gate evaluation.
type GateVerdict string

const (
	GateVerdictAllow         GateVerdict = "allow"
	GateVerdictSkipIdentical GateVerdict = "skip-identical"
	GateVerdictDeny          GateVerdict = "deny"
)

// GateResult carries the verdict and a human-readable reason, plus the
// diagnostic fields for AC-7 (story counts, timestamps, content hash).
type GateResult struct {
	Verdict          GateVerdict `json:"verdict"`
	Reason           string      `json:"reason"`
	CandidateStories int         `json:"candidate_stories"`
	DeployedStories  int         `json:"deployed_stories"`
	CandidateGenAt   string      `json:"candidate_generated_at"`
	DeployedGenAt    string      `json:"deployed_generated_at"`
	ContentSHA256    string      `json:"content_sha256"`
	Warning          string      `json:"warning,omitempty"`
}

// EvaluatePublishGate compares a candidate snapshot against the last deployed
// manifest and returns a verdict. The rules are:
//
//  1. Candidate is nil/invalid → error (fail closed, AC-4).
//  2. No manifest (never deployed) → allow (the only legal first empty deploy).
//  3. Candidate has 0 stories and manifest has > 0 → deny (zero-over-N).
//  4. Candidate.generatedAt < manifest.generatedAt → deny (older cannot replace newer).
//  5. ContentSHA256(candidate) == manifest.ContentSHA256 → skip-identical.
//  6. Otherwise → allow; if candidate.stories < manifest.stories/2 → allow + warning.
func EvaluatePublishGate(candidate *domain.Snapshot, m *DeployedManifest) (GateResult, error) {
	if candidate == nil {
		return GateResult{}, fmt.Errorf("candidate snapshot is nil")
	}
	if err := candidate.Validate(); err != nil {
		return GateResult{}, fmt.Errorf("candidate snapshot invalid: %w", err)
	}

	candStories := len(candidate.Stories)
	candGenAt := candidate.GeneratedAt
	candSHA := ContentSHA256(candidate)

	// No manifest: never deployed, allow (first deploy).
	if m == nil || m.Version == "" {
		return GateResult{
			Verdict:          GateVerdictAllow,
			Reason:           "no deployed manifest; allowing first deploy",
			CandidateStories: candStories,
			DeployedStories:  0,
			CandidateGenAt:   candGenAt,
			DeployedGenAt:    "",
			ContentSHA256:    candSHA,
		}, nil
	}

	// Validate manifest fields (fail closed on unknown version or missing
	// required fields). v1 and v2 are both accepted (issue #250 / PUB-04).
	if m.Version != ManifestVersion && m.Version != LegacyManifestVersion {
		return GateResult{}, fmt.Errorf("manifest version %q, want %q or %q", m.Version, ManifestVersion, LegacyManifestVersion)
	}
	if m.GeneratedAt == "" {
		return GateResult{}, fmt.Errorf("manifest missing generatedAt")
	}
	if m.ContentSHA256 == "" {
		return GateResult{}, fmt.Errorf("manifest missing contentSha256")
	}

	depStories := m.Stories
	depGenAt := m.GeneratedAt

	// Zero-over-N: candidate has 0 stories, manifest has > 0.
	if candStories == 0 && depStories > 0 {
		return GateResult{
			Verdict:          GateVerdictDeny,
			Reason:           fmt.Sprintf("zero-over-N: candidate has 0 stories, deployed has %d", depStories),
			CandidateStories: 0,
			DeployedStories:  depStories,
			CandidateGenAt:   candGenAt,
			DeployedGenAt:    depGenAt,
			ContentSHA256:    candSHA,
		}, nil
	}

	// Staleness: candidate.generatedAt < manifest.generatedAt.
	candT, cerr := time.Parse(time.RFC3339, candGenAt)
	depT, derr := time.Parse(time.RFC3339, depGenAt)
	if cerr != nil {
		return GateResult{}, fmt.Errorf("candidate generatedAt parse error: %w", cerr)
	}
	if derr != nil {
		return GateResult{}, fmt.Errorf("manifest generatedAt parse error: %w", derr)
	}
	if candT.Before(depT) {
		return GateResult{
			Verdict:          GateVerdictDeny,
			Reason:           fmt.Sprintf("stale: candidate generatedAt %s is older than deployed %s", candGenAt, depGenAt),
			CandidateStories: candStories,
			DeployedStories:  depStories,
			CandidateGenAt:   candGenAt,
			DeployedGenAt:    depGenAt,
			ContentSHA256:    candSHA,
		}, nil
	}

	// Identical content: skip (quiesce).
	if candSHA == m.ContentSHA256 {
		return GateResult{
			Verdict:          GateVerdictSkipIdentical,
			Reason:           "content identical to deployed manifest",
			CandidateStories: candStories,
			DeployedStories:  depStories,
			CandidateGenAt:   candGenAt,
			DeployedGenAt:    depGenAt,
			ContentSHA256:    candSHA,
		}, nil
	}

	// Allow (newer or equal content).
	result := GateResult{
		Verdict:          GateVerdictAllow,
		Reason:           "candidate is newer or different from deployed",
		CandidateStories: candStories,
		DeployedStories:  depStories,
		CandidateGenAt:   candGenAt,
		DeployedGenAt:    depGenAt,
		ContentSHA256:    candSHA,
	}

	// Suspicious drop: allow but warn (AC-7 alerting).
	if depStories > 0 && candStories < depStories/2 {
		result.Warning = fmt.Sprintf("suspicious drop: candidate has %d stories, deployed has %d", candStories, depStories)
	}

	return result, nil
}

// EvaluatePublishGateRequired is the fail-closed variant of EvaluatePublishGate
// (issue #247 / PUB-01, AC-6). With the deployed manifest moved out of Git
// (untracked), a fresh checkout has no manifest file; the plain gate treats a
// missing manifest as "never deployed" and would ALLOW a zero-story export,
// deploying an empty site. When the caller knows a real deployment previously
// existed (a production deploy path), a missing manifest must DENY instead.
//
// Behavior: identical to EvaluatePublishGate except that a nil/empty manifest
// yields a deny verdict (fail closed) rather than the "first deploy" allow.
// The candidate validation rules are unchanged.
func EvaluatePublishGateRequired(candidate *domain.Snapshot) (GateResult, error) {
	if candidate == nil {
		return GateResult{}, fmt.Errorf("candidate snapshot is nil")
	}
	if err := candidate.Validate(); err != nil {
		return GateResult{}, fmt.Errorf("candidate snapshot invalid: %w", err)
	}
	return GateResult{
		Verdict:          GateVerdictDeny,
		Reason:           "no deployed manifest and manifest required (fail closed; seed the manifest before deploying)",
		CandidateStories: len(candidate.Stories),
		DeployedStories:  0,
		CandidateGenAt:   candidate.GeneratedAt,
		DeployedGenAt:    "",
		ContentSHA256:    ContentSHA256(candidate),
	}, nil
}

// ContentSHA256 computes a SHA-256 hash of the snapshot's content (excluding
// generatedAt) for identity comparison. The hash is stable across re-exports
// of the same data (ContentEqual semantics).
func ContentSHA256(s *domain.Snapshot) string {
	// Marshal a content-only view (exclude generatedAt).
	content := struct {
		Version string                   `json:"version"`
		Stories []domain.SnapshotStory   `json:"stories"`
		Latest  []domain.SnapshotArticle `json:"latest"`
		Teams   []domain.SnapshotTeam    `json:"teams"`
	}{
		Version: s.Version,
		Stories: s.Stories,
		Latest:  s.Latest,
		Teams:   s.Teams,
	}
	b, _ := json.Marshal(content)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// LoadDeployedManifest reads the deployed manifest from disk. A missing file
// returns (nil, nil) (never deployed). An invalid file returns an error.
func LoadDeployedManifest(path string) (*DeployedManifest, error) {
	data, err := os.ReadFile(path) // #nosec G304: path is operator-controlled (web/data/deployed.json)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // never deployed
		}
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m DeployedManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if m.Version != ManifestVersion && m.Version != LegacyManifestVersion {
		return nil, fmt.Errorf("manifest version %q, want %q or %q", m.Version, ManifestVersion, LegacyManifestVersion)
	}
	return &m, nil
}

// LoadCandidateSnapshot reads the candidate snapshot (homepage.json) from disk.
// A missing or invalid file returns an error (fail closed, AC-4).
func LoadCandidateSnapshot(path string) (*domain.Snapshot, error) {
	data, err := os.ReadFile(path) // #nosec G304: path is operator-controlled (web/data/homepage.json)
	if err != nil {
		return nil, fmt.Errorf("read candidate: %w", err)
	}
	var s domain.Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse candidate: %w", err)
	}
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("candidate invalid: %w", err)
	}
	return &s, nil
}

// StampManifest creates a DeployedManifest from a candidate snapshot and commit SHA.
//
// issue #250 / PUB-04: the manifest is v2 and binds the deployment to the exact
// validated artifact and run (snapshotId, lastIngestAt, sourceCommit, runID).
// Empty snapshotId/lastIngestAt/sourceCommit/runID are written as-is (empty) —
// the CLI supplies them from the site-snapshot package + CI context when
// available.
func StampManifest(candidate *domain.Snapshot, commit string) DeployedManifest {
	now := time.Now().UTC().Format(time.RFC3339)
	return DeployedManifest{
		Version:       ManifestVersion,
		DeployedAt:    now,
		GeneratedAt:   candidate.GeneratedAt,
		Commit:        commit,
		ContentSHA256: ContentSHA256(candidate),
		Stories:       len(candidate.Stories),
		Latest:        len(candidate.Latest),
	}
}

// StampManifestV2 creates a v2 DeployedManifest from a candidate snapshot,
// commit SHA, and the validated-artifact/run binding fields (issue #250 /
// PUB-04 AC4). The binding fields are written as-is; empty values stay empty
// (a caller without site-snapshot/CI context produces a v2 manifest with empty
// binding fields, which is still valid).
func StampManifestV2(candidate *domain.Snapshot, commit, snapshotID, lastIngestAt, sourceCommit, runID string) DeployedManifest {
	m := StampManifest(candidate, commit)
	m.SnapshotID = snapshotID
	m.LastIngestAt = lastIngestAt
	m.SourceCommit = sourceCommit
	m.RunID = runID
	return m
}

// MonotonicStampResult reports the outcome of a monotonic manifest put (issue
// #250 / PUB-04 AC3): whether the stored manifest was advanced, left
// unchanged, or the put was a no-op warning (the candidate is older than the
// stored manifest).
type MonotonicStampResult struct {
	Advanced bool             `json:"advanced"`
	Skipped  bool             `json:"skipped"`
	Warning  string           `json:"warning,omitempty"`
	Manifest DeployedManifest `json:"manifest,omitempty"`
}

// MonotonicStamp compares a candidate manifest against the stored manifest and
// returns whether the put should advance the store (AC3: never lower the
// stored generatedAt). Rules:
//
//   - stored is nil/empty: advance (first put).
//   - candidate.generatedAt == stored.generatedAt: skip if content is identical
//     (no-op, no warning); advance if content differs (same instant, newer
//     content is a legitimate re-stamp).
//   - candidate.generatedAt < stored.generatedAt: skip with a warning (a
//     late/delayed run must not regress the stored manifest).
//   - candidate.generatedAt > stored.generatedAt: advance.
//
// The returned Manifest is the manifest to store (the candidate) when
// Advanced=true, or zero when Skipped.
func MonotonicStamp(stored *DeployedManifest, candidate DeployedManifest) MonotonicStampResult {
	if stored == nil || stored.Version == "" {
		return MonotonicStampResult{Advanced: true, Manifest: candidate}
	}
	candT, cerr := time.Parse(time.RFC3339, candidate.GeneratedAt)
	depT, derr := time.Parse(time.RFC3339, stored.GeneratedAt)
	if cerr != nil || derr != nil {
		// Unparseable timestamps: fail closed by treating as a regression-skip
		// with a warning (never lower a manifest we cannot compare).
		return MonotonicStampResult{Skipped: true, Warning: "cannot compare generatedAt; skipping put (fail closed)"}
	}
	if candT.Before(depT) {
		return MonotonicStampResult{
			Skipped: true,
			Warning: fmt.Sprintf("candidate generatedAt %s is older than stored %s; skipping put (no regression)", candidate.GeneratedAt, stored.GeneratedAt),
		}
	}
	if candT.Equal(depT) && candidate.ContentSHA256 == stored.ContentSHA256 {
		// Same instant, identical content: no-op, no warning.
		return MonotonicStampResult{Skipped: true}
	}
	return MonotonicStampResult{Advanced: true, Manifest: candidate}
}
