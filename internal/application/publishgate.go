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

// DeployedManifest is the tracked record of the last successfully deployed
// snapshot (web/data/deployed.json, version "1"). It is non-secret by
// construction: counts, RFC3339 timestamps, a content SHA-256, and a commit SHA.
type DeployedManifest struct {
	Version       string `json:"version"`
	DeployedAt    string `json:"deployedAt"`
	GeneratedAt   string `json:"generatedAt"`
	Commit        string `json:"commit"`
	ContentSHA256 string `json:"contentSha256"`
	Stories       int    `json:"stories"`
	Latest        int    `json:"latest"`
}

// ManifestVersion is the current version of the deployed manifest format.
const ManifestVersion = "1"

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

	// Validate manifest fields.
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
	if m.Version != ManifestVersion {
		return nil, fmt.Errorf("manifest version %q, want %q", m.Version, ManifestVersion)
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
