// Package application: rendered-site verification unit tests (issue #250 /
// PUB-04 AC6, R3).
package application

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// writeRenderedIndex writes a minimal rendered index.html that mimics the
// Hugo home layout: an optional "Data as of" freshness note with a datetime
// attribute, and N story-card <article> elements.
func writeRenderedIndex(t *testing.T, publicDir string, generatedAt string, storyCards int) {
	t.Helper()
	if err := os.MkdirAll(publicDir, 0o755); err != nil {
		t.Fatalf("mkdir publicDir: %v", err)
	}
	var html string
	if generatedAt != "" {
		html += `<p class="update-note">Data as of <time datetime="` + generatedAt + `">Jan 1, 2026 · 00:00 UTC</time></p>`
	}
	for i := 0; i < storyCards; i++ {
		html += `<article class="story-card"><h3>Story ` + string(rune('a'+i)) + `</h3></article>`
	}
	indexPath := filepath.Join(publicDir, "index.html")
	if err := os.WriteFile(indexPath, []byte(html), 0o644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}
}

// TestValidateRenderedSite_Pass: rendered cards match candidate stories,
// freshness label matches generatedAt, no demo markers.
func TestValidateRenderedSite_Pass(t *testing.T) {
	cand := makeSnapshot(t, 3, "2026-01-01T00:00:00Z", "dal")
	publicDir := t.TempDir()
	writeRenderedIndex(t, publicDir, "2026-01-01T00:00:00Z", 3)
	res, err := ValidateRenderedSite(publicDir, cand, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Pass {
		t.Errorf("expected pass, got reason=%q", res.Reason)
	}
	if res.StoryCards != 3 {
		t.Errorf("story_cards = %d, want 3", res.StoryCards)
	}
}

// TestValidateRenderedSite_SkeletonWithNonEmptyCandidate_Deny: the #229
// failure mode — a valid JSON export (3 stories) but a rendered skeleton
// (0 story cards). Must deny.
func TestValidateRenderedSite_SkeletonWithNonEmptyCandidate_Deny(t *testing.T) {
	cand := makeSnapshot(t, 3, "2026-01-01T00:00:00Z", "dal")
	publicDir := t.TempDir()
	// Rendered index has a freshness note but 0 story cards.
	writeRenderedIndex(t, publicDir, "2026-01-01T00:00:00Z", 0)
	res, err := ValidateRenderedSite(publicDir, cand, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Pass {
		t.Error("expected deny for skeleton render with non-empty candidate")
	}
}

// TestValidateRenderedSite_CardsBelowManifestMinimum_Deny: AC1 — rendered
// cards below the last validated manifest minimum is a content regression.
func TestValidateRenderedSite_CardsBelowManifestMinimum_Deny(t *testing.T) {
	cand := makeSnapshot(t, 2, "2026-01-01T00:00:00Z", "dal")
	publicDir := t.TempDir()
	// 2 rendered cards (matches candidate) but the manifest validated 5.
	writeRenderedIndex(t, publicDir, "2026-01-01T00:00:00Z", 2)
	m := &DeployedManifest{
		Version:       ManifestVersion,
		DeployedAt:    "2026-01-01T00:00:00Z",
		GeneratedAt:   "2026-01-01T00:00:00Z",
		Commit:        "abc123",
		ContentSHA256: "hash123",
		Stories:       5,
	}
	res, err := ValidateRenderedSite(publicDir, cand, m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Pass {
		t.Error("expected deny: rendered cards below manifest minimum")
	}
}

// TestValidateRenderedSite_StaleFreshnessLabel_Deny: the rendered "Data as of"
// label does not match the candidate generatedAt.
func TestValidateRenderedSite_StaleFreshnessLabel_Deny(t *testing.T) {
	cand := makeSnapshot(t, 3, "2026-01-01T00:00:00Z", "dal")
	publicDir := t.TempDir()
	// Rendered freshness is older than the candidate.
	writeRenderedIndex(t, publicDir, "2025-12-01T00:00:00Z", 3)
	res, err := ValidateRenderedSite(publicDir, cand, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Pass {
		t.Error("expected deny: freshness label does not match candidate generatedAt")
	}
}

// TestValidateRenderedSite_DemoMarker_Deny: a demo marker in the rendered
// index is fixture leakage.
func TestValidateRenderedSite_DemoMarker_Deny(t *testing.T) {
	cand := makeSnapshot(t, 3, "2026-01-01T00:00:00Z", "dal")
	publicDir := t.TempDir()
	writeRenderedIndex(t, publicDir, "2026-01-01T00:00:00Z", 3)
	// Overwrite index.html to inject a demo marker.
	indexPath := filepath.Join(publicDir, "index.html")
	if err := os.WriteFile(indexPath, []byte(`<p>Data as of <time datetime="2026-01-01T00:00:00Z"></time></p><article class="story-card">example.com top-free-agents-2024</article>`), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	res, err := ValidateRenderedSite(publicDir, cand, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Pass {
		t.Error("expected deny: demo marker in rendered index")
	}
}

// TestValidateRenderedSite_ZeroCandidateWithCards_Deny: candidate has 0
// stories but the render shows cards (out of sync / fixture leakage).
func TestValidateRenderedSite_ZeroCandidateWithCards_Deny(t *testing.T) {
	cand := makeSnapshot(t, 0, "2026-01-01T00:00:00Z", "dal")
	publicDir := t.TempDir()
	writeRenderedIndex(t, publicDir, "2026-01-01T00:00:00Z", 3)
	res, err := ValidateRenderedSite(publicDir, cand, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Pass {
		t.Error("expected deny: candidate has 0 stories but render shows cards")
	}
}

// TestValidateRenderedSite_MissingIndex_Error: a missing index.html (Hugo
// produced no homepage) is a hard input error (exit 1 path).
func TestValidateRenderedSite_MissingIndex_Error(t *testing.T) {
	cand := makeSnapshot(t, 3, "2026-01-01T00:00:00Z", "dal")
	publicDir := t.TempDir() // no index.html
	if _, err := ValidateRenderedSite(publicDir, cand, nil); err != nil {
		t.Fatalf("a missing index.html is a deny, not an error: %v", err)
	}
}

// TestValidateRenderedSite_MissingFreshnessLabel_Deny: no "Data as of" note
// rendered (snapshot data not picked up) is a deny.
func TestValidateRenderedSite_MissingFreshnessLabel_Deny(t *testing.T) {
	cand := makeSnapshot(t, 3, "2026-01-01T00:00:00Z", "dal")
	publicDir := t.TempDir()
	// index.html with story cards but no "Data as of" note.
	indexPath := filepath.Join(publicDir, "index.html")
	if err := os.WriteFile(indexPath, []byte(`<article class="story-card">a</article>`), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	res, err := ValidateRenderedSite(publicDir, cand, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Pass {
		t.Error("expected deny: no 'Data as of' freshness label")
	}
}

// TestValidateRenderedSite_NilCandidate_Error.
func TestValidateRenderedSite_NilCandidate_Error(t *testing.T) {
	if _, err := ValidateRenderedSite(t.TempDir(), nil, nil); err == nil {
		t.Error("expected error for nil candidate")
	}
}

// TestValidateRenderedSite_InvalidCandidate_Error.
func TestValidateRenderedSite_InvalidCandidate_Error(t *testing.T) {
	bad := &domain.Snapshot{Version: "999", Stories: []domain.SnapshotStory{}}
	if _, err := ValidateRenderedSite(t.TempDir(), bad, nil); err == nil {
		t.Error("expected error for invalid candidate")
	}
}

// TestValidateRenderedSite_EmptyPublicDir_Error.
func TestValidateRenderedSite_EmptyPublicDir_Error(t *testing.T) {
	cand := makeSnapshot(t, 3, "2026-01-01T00:00:00Z", "dal")
	if _, err := ValidateRenderedSite("", cand, nil); err == nil {
		t.Error("expected error for empty publicDir")
	}
}
