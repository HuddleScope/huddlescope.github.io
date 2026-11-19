package hugo

// HS-056 (AC-4/AC-6): fixture-vs-production validation gate at repository
// level.
//   - The production data path (web/data) must never contain the demo sample
//     markers. The homepage/feeds files are untracked until a real refresh
//     export commits them, so their absence is the honest state and passes.
//   - The committed test fixtures (web/fixtures) must keep the deterministic
//     demo sample so CI site builds stay reproducible.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Repo root relative to this package (internal/adapters/hugo -> three up).
const repoRoot = "../../.."

// demoMarkers identify the committed 2024 sample data. A production file
// containing any of them would present demo content as real NFL news.
var demoMarkers = []string{
	"example.com",
	"seed-run-",
	"top-free-agents-2024",
	"pro-bowl-2025-format",
	"preseason-week-1-overview",
}

func TestProductionDataFilesContainNoDemoMarkers(t *testing.T) {
	for _, name := range []string{"homepage.json", "feeds.json"} {
		path := filepath.Join(repoRoot, "web", "data", name)
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			// Absent file: the honest empty state before the first real export.
			continue
		}
		if err != nil {
			t.Fatalf("web/data/%s: %v", name, err)
		}
		for _, marker := range demoMarkers {
			if bytes.Contains(data, []byte(marker)) {
				t.Errorf("web/data/%s contains demo marker %q; production data must come exclusively from real refresh runs", name, marker)
			}
		}
	}
}

func TestCommittedFixturesPreserveDemoSample(t *testing.T) {
	homePage := filepath.Join(repoRoot, "web", "fixtures", "homepage.json")
	data, err := os.ReadFile(homePage)
	if err != nil {
		t.Fatalf("committed fixture %s must exist: %v", homePage, err)
	}
	if err := json.Unmarshal(data, &struct{}{}); err != nil {
		t.Errorf("fixture homepage.json is not valid JSON: %v", err)
	}
	if !bytes.Contains(data, []byte("top-free-agents-2024")) {
		t.Error("fixture homepage.json lost the deterministic demo sample (top-free-agents-2024)")
	}

	feeds := filepath.Join(repoRoot, "web", "fixtures", "feeds.json")
	data, err = os.ReadFile(feeds)
	if err != nil {
		t.Fatalf("committed fixture %s must exist: %v", feeds, err)
	}
	if err := json.Unmarshal(data, &struct{}{}); err != nil {
		t.Errorf("fixture feeds.json is not valid JSON: %v", err)
	}
	if !bytes.Contains(data, []byte("seed-run-001")) {
		t.Error("fixture feeds.json lost the deterministic demo sample (seed-run-001)")
	}
}
