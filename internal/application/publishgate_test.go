package application

import (
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// makeSnapshot builds a minimal valid snapshot for gate tests.
func makeSnapshot(t *testing.T, stories int, generatedAt string, teamSlug string) *domain.Snapshot {
	t.Helper()
	s := &domain.Snapshot{
		Version:     domain.SnapshotVersion,
		GeneratedAt: generatedAt,
		Stories:     make([]domain.SnapshotStory, 0, stories),
		Latest:      []domain.SnapshotArticle{},
		Teams:       []domain.SnapshotTeam{},
	}
	if teamSlug != "" {
		s.Teams = append(s.Teams, domain.SnapshotTeam{
			Slug:         teamSlug,
			Name:         "Test Team",
			ShortName:    "TT",
			Abbreviation: "TT",
		})
	}
	for i := 0; i < stories; i++ {
		s.Stories = append(s.Stories, domain.SnapshotStory{
			ID:               string(rune('a'+i)) + string(rune('0'+i%10)),
			Title:            "Story " + string(rune('a'+i)),
			Summary:          "Summary for story " + string(rune('a'+i)),
			FirstPublishedAt: generatedAt,
			LastPublishedAt:  generatedAt,
			Score:            5.0,
			EvidenceLevel:    domain.EvidenceUnknown,
			EvidenceLevelCSS: "unknown",
			Publishers:       []domain.PublisherRef{{ID: "src", Name: "Source", URL: "https://example.com"}},
		})
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("test snapshot invalid: %v", err)
	}
	return s
}

func TestEvaluatePublishGate_NilCandidate(t *testing.T) {
	_, err := EvaluatePublishGate(nil, nil)
	if err == nil {
		t.Error("expected error for nil candidate")
	}
}

func TestEvaluatePublishGate_InvalidCandidate(t *testing.T) {
	bad := &domain.Snapshot{Version: "999", Stories: []domain.SnapshotStory{}}
	_, err := EvaluatePublishGate(bad, nil)
	if err == nil {
		t.Error("expected error for invalid candidate (bad version)")
	}
}

func TestEvaluatePublishGate_NoManifest_Allow(t *testing.T) {
	cand := makeSnapshot(t, 5, "2026-01-01T00:00:00Z", "dal")
	res, err := EvaluatePublishGate(cand, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Verdict != GateVerdictAllow {
		t.Errorf("verdict = %q, want allow", res.Verdict)
	}
	if res.CandidateStories != 5 {
		t.Errorf("candidate_stories = %d, want 5", res.CandidateStories)
	}
}

func TestEvaluatePublishGate_ZeroOverN_Deny(t *testing.T) {
	cand := makeSnapshot(t, 0, "2026-01-02T00:00:00Z", "dal")
	m := &DeployedManifest{
		Version:       ManifestVersion,
		DeployedAt:    "2026-01-01T00:00:00Z",
		GeneratedAt:   "2026-01-01T00:00:00Z",
		Commit:        "abc123",
		ContentSHA256: "hash123",
		Stories:       10,
		Latest:        5,
	}
	res, err := EvaluatePublishGate(cand, m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Verdict != GateVerdictDeny {
		t.Errorf("verdict = %q, want deny", res.Verdict)
	}
	if res.CandidateStories != 0 || res.DeployedStories != 10 {
		t.Errorf("stories = %d/%d, want 0/10", res.CandidateStories, res.DeployedStories)
	}
}

func TestEvaluatePublishGate_Stale_Deny(t *testing.T) {
	// Candidate is older than deployed.
	cand := makeSnapshot(t, 5, "2026-01-01T00:00:00Z", "dal")
	m := &DeployedManifest{
		Version:       ManifestVersion,
		DeployedAt:    "2026-01-02T00:00:00Z",
		GeneratedAt:   "2026-01-02T00:00:00Z",
		Commit:        "abc123",
		ContentSHA256: "hash123",
		Stories:       5,
		Latest:        3,
	}
	res, err := EvaluatePublishGate(cand, m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Verdict != GateVerdictDeny {
		t.Errorf("verdict = %q, want deny (stale)", res.Verdict)
	}
}

func TestEvaluatePublishGate_Newer_Allow(t *testing.T) {
	cand := makeSnapshot(t, 5, "2026-01-03T00:00:00Z", "dal")
	m := &DeployedManifest{
		Version:       ManifestVersion,
		DeployedAt:    "2026-01-02T00:00:00Z",
		GeneratedAt:   "2026-01-02T00:00:00Z",
		Commit:        "abc123",
		ContentSHA256: "different-hash",
		Stories:       5,
		Latest:        3,
	}
	res, err := EvaluatePublishGate(cand, m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Verdict != GateVerdictAllow {
		t.Errorf("verdict = %q, want allow (newer)", res.Verdict)
	}
}

func TestEvaluatePublishGate_Identical_Skip(t *testing.T) {
	cand := makeSnapshot(t, 5, "2026-01-01T00:00:00Z", "dal")
	// Build manifest from the same candidate so ContentSHA256 matches.
	m := &DeployedManifest{
		Version:       ManifestVersion,
		DeployedAt:    "2026-01-01T00:00:00Z",
		GeneratedAt:   cand.GeneratedAt,
		Commit:        "abc123",
		ContentSHA256: ContentSHA256(cand),
		Stories:       len(cand.Stories),
		Latest:        len(cand.Latest),
	}
	res, err := EvaluatePublishGate(cand, m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Verdict != GateVerdictSkipIdentical {
		t.Errorf("verdict = %q, want skip-identical", res.Verdict)
	}
}

func TestEvaluatePublishGate_SuspiciousDrop_AllowWithWarning(t *testing.T) {
	// Candidate has fewer than half the stories of deployed → allow + warning.
	cand := makeSnapshot(t, 2, "2026-01-03T00:00:00Z", "dal")
	m := &DeployedManifest{
		Version:       ManifestVersion,
		DeployedAt:    "2026-01-02T00:00:00Z",
		GeneratedAt:   "2026-01-02T00:00:00Z",
		Commit:        "abc123",
		ContentSHA256: "different-hash",
		Stories:       10,
		Latest:        5,
	}
	res, err := EvaluatePublishGate(cand, m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Verdict != GateVerdictAllow {
		t.Errorf("verdict = %q, want allow", res.Verdict)
	}
	if res.Warning == "" {
		t.Error("expected warning for suspicious drop")
	}
}

func TestEvaluatePublishGate_ValidDrop_NoWarning(t *testing.T) {
	// Candidate has >= half the stories → allow, no warning.
	cand := makeSnapshot(t, 6, "2026-01-03T00:00:00Z", "dal")
	m := &DeployedManifest{
		Version:       ManifestVersion,
		DeployedAt:    "2026-01-02T00:00:00Z",
		GeneratedAt:   "2026-01-02T00:00:00Z",
		Commit:        "abc123",
		ContentSHA256: "different-hash",
		Stories:       10,
		Latest:        5,
	}
	res, err := EvaluatePublishGate(cand, m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Verdict != GateVerdictAllow {
		t.Errorf("verdict = %q, want allow", res.Verdict)
	}
	if res.Warning != "" {
		t.Errorf("unexpected warning: %q", res.Warning)
	}
}

func TestEvaluatePublishGate_EqualGeneratedAt_DifferentContent_Allow(t *testing.T) {
	// Same generatedAt but different content → allow (not stale, not identical).
	cand := makeSnapshot(t, 5, "2026-01-01T00:00:00Z", "dal")
	cand.Stories[0].Title = "Changed Title"
	m := &DeployedManifest{
		Version:       ManifestVersion,
		DeployedAt:    "2026-01-01T00:00:00Z",
		GeneratedAt:   "2026-01-01T00:00:00Z",
		Commit:        "abc123",
		ContentSHA256: "different-hash",
		Stories:       5,
		Latest:        3,
	}
	res, err := EvaluatePublishGate(cand, m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Verdict != GateVerdictAllow {
		t.Errorf("verdict = %q, want allow (equal genAt, different content)", res.Verdict)
	}
}

func TestEvaluatePublishGate_ZeroCandidate_ZeroDeployed_Allow(t *testing.T) {
	// Both zero: first empty deploy (legal).
	cand := makeSnapshot(t, 0, "2026-01-01T00:00:00Z", "dal")
	m := &DeployedManifest{
		Version:       ManifestVersion,
		DeployedAt:    "2026-01-01T00:00:00Z",
		GeneratedAt:   "2026-01-01T00:00:00Z",
		Commit:        "abc123",
		ContentSHA256: "hash123",
		Stories:       0,
		Latest:        0,
	}
	res, err := EvaluatePublishGate(cand, m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Verdict != GateVerdictAllow {
		t.Errorf("verdict = %q, want allow (both zero)", res.Verdict)
	}
}

func TestContentSHA256_Stable(t *testing.T) {
	s1 := makeSnapshot(t, 3, "2026-01-01T00:00:00Z", "dal")
	s2 := makeSnapshot(t, 3, "2026-01-01T00:00:00Z", "dal")
	// Different generatedAt but same content → same hash.
	s2.GeneratedAt = "2026-02-01T00:00:00Z"
	h1 := ContentSHA256(s1)
	h2 := ContentSHA256(s2)
	if h1 != h2 {
		t.Error("ContentSHA256 should be stable across generatedAt changes")
	}
}

func TestContentSHA256_DifferentContent(t *testing.T) {
	s1 := makeSnapshot(t, 3, "2026-01-01T00:00:00Z", "dal")
	s2 := makeSnapshot(t, 4, "2026-01-01T00:00:00Z", "dal")
	h1 := ContentSHA256(s1)
	h2 := ContentSHA256(s2)
	if h1 == h2 {
		t.Error("ContentSHA256 should differ for different story counts")
	}
}

// TestEvaluatePublishGateRequired_NoManifest_Deny (issue #247 / AC-6): with the
// manifest moved out of Git, a fresh checkout has no manifest; the fail-closed
// gate must DENY (never allow a zero-story / empty deploy) rather than the
// plain gate's "never deployed" allow.
func TestEvaluatePublishGateRequired_NoManifest_Deny(t *testing.T) {
	cand := makeSnapshot(t, 0, "2026-01-01T00:00:00Z", "dal")
	res, err := EvaluatePublishGateRequired(cand)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Verdict != GateVerdictDeny {
		t.Errorf("verdict = %q, want deny", res.Verdict)
	}
	if res.Reason == "" {
		t.Error("expected a deny reason")
	}
	if res.DeployedStories != 0 {
		t.Errorf("deployed_stories = %d, want 0", res.DeployedStories)
	}
}

// TestEvaluatePublishGateRequired_NilCandidate errors (fail closed).
func TestEvaluatePublishGateRequired_NilCandidate(t *testing.T) {
	if _, err := EvaluatePublishGateRequired(nil); err == nil {
		t.Error("expected error for nil candidate")
	}
}

func TestStampManifest(t *testing.T) {
	cand := makeSnapshot(t, 5, "2026-01-01T00:00:00Z", "dal")
	m := StampManifest(cand, "abc123")
	if m.Version != ManifestVersion {
		t.Errorf("version = %q, want %q", m.Version, ManifestVersion)
	}
	if m.GeneratedAt != cand.GeneratedAt {
		t.Errorf("generatedAt = %q, want %q", m.GeneratedAt, cand.GeneratedAt)
	}
	if m.Commit != "abc123" {
		t.Errorf("commit = %q, want abc123", m.Commit)
	}
	if m.Stories != 5 {
		t.Errorf("stories = %d, want 5", m.Stories)
	}
	if m.ContentSHA256 != ContentSHA256(cand) {
		t.Error("contentSha256 mismatch")
	}
	if m.DeployedAt == "" {
		t.Error("deployedAt should be set")
	}
	// DeployedAt should be a valid RFC3339 timestamp.
	if _, err := time.Parse(time.RFC3339, m.DeployedAt); err != nil {
		t.Errorf("deployedAt not RFC3339: %v", err)
	}
}
