// Package application: rendered-site verification (issue #250 / PUB-04).
//
// ValidateRenderedSite checks the rendered Hugo output (web/public/index.html)
// against the candidate snapshot BEFORE any Pages upload. The publish gate
// compares JSON exports; this function compares the rendered HTML, so a build
// that renders a skeleton/zero-story homepage from a valid JSON export (the
// #229 failure mode) is caught by the rendered artifact itself.
//
// Verification anchors are bound to the candidate/manifest, not to pixel-
// stable markup:
//
//   - story cards: each story renders as <article class="story-card...">
//   - freshness:   the home "Data as of" note carries
//     <time datetime="{{ .generatedAt }}">, so the rendered freshness label
//     must equal the candidate generatedAt
//   - demo leakage: snapshot.DemoMarkers must not appear in any rendered HTML
package application

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
	"github.com/HuddleScope/huddlescope.github.io/internal/snapshot"
)

// VerifyResult carries the rendered-site verification outcome for the CLI and
// the diagnostic report. Pass=true means the rendered output may be uploaded
// to Pages.
type VerifyResult struct {
	Pass       bool     `json:"pass"`
	Reason     string   `json:"reason"`
	StoryCards int      `json:"story_cards"`
	Checks     []string `json:"checks"`
}

// datetimeRe extracts RFC3339-ish datetime attribute values from HTML.
var datetimeRe = regexp.MustCompile(`datetime="([^"]+)"`)

// ValidateRenderedSite verifies the rendered Hugo output against the candidate
// snapshot and the last deployed manifest. Every check fails closed: any
// failure returns a non-passing VerifyResult (not an error), and a hard input
// error (unreadable publicDir, invalid candidate) returns an error.
//
// Exit-code contract (enforced by the CLI): 0 pass, 1 invalid input,
// 3 deny-equivalent (content regression / validation failure).
func ValidateRenderedSite(publicDir string, candidate *domain.Snapshot, m *DeployedManifest) (VerifyResult, error) {
	if candidate == nil {
		return VerifyResult{}, fmt.Errorf("candidate is nil")
	}
	if err := candidate.Validate(); err != nil {
		return VerifyResult{}, fmt.Errorf("candidate invalid: %w", err)
	}
	if publicDir == "" {
		return VerifyResult{}, fmt.Errorf("publicDir is empty")
	}

	var checks []string
	fail := func(format string, args ...any) (VerifyResult, error) {
		return VerifyResult{Pass: false, Reason: fmt.Sprintf(format, args...), Checks: checks}, nil
	}

	// 1. index.html must exist (Hugo exited 0 and produced output).
	indexPath := filepath.Join(publicDir, "index.html")
	indexData, err := os.ReadFile(indexPath) // #nosec G304: indexPath derives from publicDir (operator-provided)
	if err != nil {
		if os.IsNotExist(err) {
			return fail("rendered index.html missing (Hugo build produced no homepage)")
		}
		return VerifyResult{}, fmt.Errorf("read index.html: %w", err)
	}
	checks = append(checks, "index.html present")
	indexHTML := string(indexData)

	// 2. Story cards: each candidate story renders as <article class="story-card...">.
	// A skeleton render (0 cards) with a non-empty candidate is a regression.
	storyCards := strings.Count(indexHTML, `<article class="story-card`)
	candStories := len(candidate.Stories)
	checks = append(checks, fmt.Sprintf("story cards=%d candidate stories=%d", storyCards, candStories))

	// Minimum cards: the candidate is the source of truth for what should be
	// rendered. The manifest sets a floor for the last validated deployment —
	// a render below the manifest minimum is a content regression (AC1).
	minCards := candStories
	manifestStories := 0
	if m != nil {
		manifestStories = m.Stories
		if m.Stories > minCards {
			minCards = m.Stories
		}
	}
	if minCards > 0 && storyCards < minCards {
		return fail("rendered story cards %d below minimum %d (candidate %d, manifest %d)",
			storyCards, minCards, candStories, manifestStories)
	}
	// Candidate has no stories but the render shows some: the render is out of
	// sync with the candidate (stale build or fixture leakage). Deny (AC2).
	if candStories == 0 && storyCards > 0 {
		return fail("candidate has 0 stories but rendered index shows %d story cards (out of sync)", storyCards)
	}

	// 3. Freshness label: the home "Data as of" note carries
	// <time datetime="{{ .generatedAt }}">; the rendered freshness must equal
	// the candidate generatedAt (AC6).
	if ts := firstDatetime(indexHTML, "Data as of"); ts != "" {
		if ts != candidate.GeneratedAt {
			return fail("rendered freshness label %q does not match candidate generatedAt %q", ts, candidate.GeneratedAt)
		}
		checks = append(checks, "freshness matches candidate generatedAt")
	} else {
		// No "Data as of" note rendered: the homepage did not pick up the
		// snapshot data (skeleton render). Deny (AC6/AC7).
		return fail("rendered index has no 'Data as of' freshness label (snapshot data not rendered)")
	}

	// 4. Demo/fixture leakage: no rendered HTML may carry a demo marker.
	if hit := scanDemoMarkers(indexHTML); hit != "" {
		return fail("rendered index.html contains demo marker %q", hit)
	}
	if hit := walkDemoMarkers(publicDir, indexPath); hit != "" {
		return fail("rendered HTML contains demo marker %q", hit)
	}
	checks = append(checks, "no demo markers")

	return VerifyResult{Pass: true, Reason: "rendered site validated", StoryCards: storyCards, Checks: checks}, nil
}

// firstDatetime returns the value of the first datetime="..." attribute that
// appears after the markerText in html (the "Data as of" freshness note).
func firstDatetime(html, markerText string) string {
	idx := strings.Index(html, markerText)
	if idx < 0 {
		return ""
	}
	rest := html[idx:]
	m := datetimeRe.FindStringSubmatch(rest)
	if m == nil {
		return ""
	}
	return m[1]
}

// scanDemoMarkers returns the first demo marker present in html, or "".
func scanDemoMarkers(html string) string {
	for _, name := range snapshot.DemoMarkers {
		if strings.Contains(html, name) {
			return name
		}
	}
	return ""
}

// walkDemoMarkers scans every rendered HTML file under root (except skipPath)
// for demo markers and returns "marker (path)" for the first hit, or "".
func walkDemoMarkers(root, skipPath string) string {
	var hit string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error { //nolint:gosec // root is operator-provided
		if hit != "" {
			return filepath.SkipDir // already found a marker; stop scanning
		}
		if err != nil || d == nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".html") || path == skipPath {
			return nil
		}
		data, rerr := os.ReadFile(path) // #nosec G304 G122: path from WalkDir of a local rendered-site dir
		if rerr != nil {
			return nil
		}
		if name := scanDemoMarkers(string(data)); name != "" {
			hit = fmt.Sprintf("%q (%s)", name, path)
		}
		return nil
	})
	return hit
}
