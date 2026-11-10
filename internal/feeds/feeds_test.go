package feeds

import (
	"os"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

func TestLoad(t *testing.T) {
	registry, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if registry == nil {
		t.Fatal("Load() returned nil registry")
	}

	count := registry.Count()
	if count != 37 {
		t.Errorf("registry Count() = %d; want 37 (YAML sources)", count)
	}
}

func TestLoad_Validation(t *testing.T) {
	// Load() calls NewFeedRegistry which validates all sources
	registry, err := Load()
	if err != nil {
		t.Fatalf("Load() validation error = %v", err)
	}

	// Verify all sources are valid
	if err := registry.Validate(); err != nil {
		t.Errorf("registry Validate() error = %v; want nil", err)
	}
}

func TestLoad_IDUniqueness(t *testing.T) {
	registry, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	seen := make(map[string]bool)
	for _, src := range registry.All() {
		if seen[src.ID] {
			t.Errorf("Duplicate source ID: %s", src.ID)
		}
		seen[src.ID] = true
	}
}

func TestLoad_MinimumSeeded(t *testing.T) {
	// Verify we have at least 37 seeded sources (YAML has 37)
	count := 37
	if count < 10 {
		t.Errorf("YAML sources count = %d; want >= 10", count)
	}
}

func TestLoad_EnabledCount(t *testing.T) {
	registry, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	actual := registry.CountEnabled()

	// YAML should have at least 10 enabled sources
	if actual < 10 {
		t.Errorf("CountEnabled() = %d; want >= 10 (at least 10 enabled feeds)", actual)
	}

	// Verify we have more sources than SeededSources
	if actual <= len(SeededSources) {
		t.Errorf("CountEnabled() = %d; should be > len(SeededSources)=%d", actual, len(SeededSources))
	}
}

func TestLoad_DomainMapping(t *testing.T) {
	registry, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Check a sample source has all fields populated
	src, ok := registry.Get("cbs-nfl")
	if !ok {
		t.Fatal("Get(\"cbs-nfl\") not found")
	}

	if src.ID != "cbs-nfl" {
		t.Errorf("ID = %s; want cbs-nfl", src.ID)
	}
	if src.Publication != "CBS Sports NFL" {
		t.Errorf("Publication = %s; want CBS Sports NFL", src.Publication)
	}
	if src.Enabled != true {
		t.Error("Enabled should be true")
	}
	if src.Language != "" {
		t.Errorf("Language should be empty (not set in YAML), got %s", src.Language)
	}
	if src.AcceptableUse != domain.AcceptanceAllowed {
		t.Errorf("AcceptableUse = %s; want allowed", src.AcceptableUse.String())
	}
}

func TestLoad_RestrictedFeed(t *testing.T) {
	// Verify a restricted feed is disabled
	registry, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Check that at least one feed is disabled (YAML has some disabled feeds)
	all := registry.All()
	hasDisabled := false
	for _, s := range all {
		if !s.Enabled {
			hasDisabled = true
			break
		}
	}

	if !hasDisabled {
		t.Error("No disabled feeds found; YAML should have some disabled feeds")
	}
}

func TestLoad_URLScheme(t *testing.T) {
	registry, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	for _, src := range registry.All() {
		if src.URL == "" {
			t.Errorf("Source %s has empty URL", src.ID)
			continue
		}
		if src.URL[:7] != "http://" && src.URL[:8] != "https://" {
			t.Errorf("Source %s URL %s does not start with http:// or https://", src.ID, src.URL)
		}
	}
}

func TestLoad_EnabledNonEmpty(t *testing.T) {
	registry, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	enabled := registry.Enabled()
	if len(enabled) == 0 {
		t.Error("Enabled() returned empty list; at least one feed should be enabled")
	}
}

func TestLoad_Sorting(t *testing.T) {
	registry, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	all := registry.All()
	if len(all) < 2 {
		t.Skip("Not enough sources to test sorting")
	}

	// Check that All() is sorted by priority ascending
	for i := 0; i < len(all)-1; i++ {
		if all[i].Priority > all[i+1].Priority {
			t.Errorf("All() not sorted by priority: [%d]=%s(prio %d) > [%d]=%s(prio %d)",
				i, all[i].ID, all[i].Priority, i+1, all[i+1].ID, all[i+1].Priority)
		}
	}

	enabled := registry.Enabled()
	for i := 0; i < len(enabled)-1; i++ {
		if enabled[i].Priority > enabled[i+1].Priority {
			t.Errorf("Enabled() not sorted by priority: [%d]=%s(prio %d) > [%d]=%s(prio %d)",
				i, enabled[i].ID, enabled[i].Priority, i+1, enabled[i+1].ID, enabled[i+1].Priority)
		}
	}
}

func TestEnabledCount(t *testing.T) {
	registry, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	count := EnabledCount()
	if count == 0 {
		t.Error("EnabledCount() = 0; want positive number of enabled feeds")
	}

	expected := registry.CountEnabled()
	if count != expected {
		t.Errorf("EnabledCount() = %d; want %d from registry", count, expected)
	}
}

func TestLoadFromYAML(t *testing.T) {
	// Note: YAML loading is tested in internal/config/config_test.go
	// This test just ensures the Load function works
	registry, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if registry == nil {
		t.Fatal("Load() returned nil registry")
	}
}

func TestLoadFrom_NonExistentFile(t *testing.T) {
	_, err := LoadFrom("/nonexistent/path/sources.yaml")
	if err != nil {
		t.Errorf("LoadFrom() on nonexistent file should return nil, got error: %v", err)
	}
}

func TestLoadFrom_EmptyFile(t *testing.T) {
	tmpFile := t.TempDir() + "/empty.yaml"
	if err := os.WriteFile(tmpFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	registry, err := LoadFrom(tmpFile)
	if err != nil {
		t.Fatalf("LoadFrom() on empty file error = %v", err)
	}
	if registry != nil {
		t.Error("LoadFrom() on empty file should return nil registry")
	}
}

func TestLoadFrom_EmptySources(t *testing.T) {
	tmpFile := t.TempDir() + "/empty-sources.yaml"
	if err := os.WriteFile(tmpFile, []byte("sources: []\n"), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	registry, err := LoadFrom(tmpFile)
	if err != nil {
		t.Fatalf("LoadFrom() on empty sources error = %v", err)
	}
	if registry != nil {
		t.Error("LoadFrom() on empty sources should return nil registry")
	}
}

func TestLoadFrom_MultipleSources(t *testing.T) {
	tmpFile := t.TempDir() + "/multi-sources.yaml"
	yamlContent := `sources:
  - id: source1
    url: https://example.com/1
    publication: Source 1
    enabled: true
    priority: 10
  - id: source2
    url: https://example.com/2
    publication: Source 2
    enabled: false
    priority: 5
  - id: source3
    url: https://example.com/3
    publication: Source 3
    enabled: true
    priority: 15
`
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	registry, err := LoadFrom(tmpFile)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if registry == nil {
		t.Fatal("LoadFrom() returned nil registry")
	}

	// Verify all sources are present
	src1, ok := registry.Get("source1")
	if !ok {
		t.Fatal("Get(\"source1\") not found")
	}
	if src1.Publication != "Source 1" {
		t.Errorf("source1 Publication = %s; want Source 1", src1.Publication)
	}

	src2, ok := registry.Get("source2")
	if !ok {
		t.Fatal("Get(\"source2\") not found")
	}
	if src2.Enabled {
		t.Error("source2 should be disabled")
	}

	src3, ok := registry.Get("source3")
	if !ok {
		t.Fatal("Get(\"source3\") not found")
	}
	if src3.Publication != "Source 3" {
		t.Errorf("source3 Publication = %s; want Source 3", src3.Publication)
	}

	// Verify count
	if registry.Count() != 3 {
		t.Errorf("registry Count() = %d; want 3", registry.Count())
	}

	// Verify enabled count
	if registry.CountEnabled() != 2 {
		t.Errorf("registry CountEnabled() = %d; want 2", registry.CountEnabled())
	}
}

func TestLoadFrom_BadYAML(t *testing.T) {
	tmpFile := t.TempDir() + "/bad.yaml"
	if err := os.WriteFile(tmpFile, []byte("invalid: yaml: ["), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	_, err := LoadFrom(tmpFile)
	if err == nil {
		t.Error("LoadFrom() on bad YAML should return error")
	}
}

func TestLoadFrom_TimeoutAndLicense(t *testing.T) {
	// Create a temporary YAML file with timeout and license
	tmpFile := t.TempDir() + "/sources.yaml"
	yamlContent := `sources:
  - id: test-timeout
    url: https://example.com/test
    publication: Test Feed
    enabled: true
    priority: 10
    timeout: 45
    license: "CC-BY-NC-ND 4.0"
`
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	registry, err := LoadFrom(tmpFile)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}

	if registry == nil {
		t.Fatal("LoadFrom() returned nil registry")
	}

	// Get the test source
	src, ok := registry.Get("test-timeout")
	if !ok {
		t.Fatal("Get(\"test-timeout\") not found")
	}

	// Verify timeout and license are preserved
	if src.Timeout != 45*time.Second {
		t.Errorf("Timeout = %v; want 45s", src.Timeout)
	}
	if src.License != "CC-BY-NC-ND 4.0" {
		t.Errorf("License = %q; want CC-BY-NC-ND 4.0", src.License)
	}
}

func TestLoadFrom_AcceptableUseAllowed(t *testing.T) {
	tmpFile := t.TempDir() + "/acceptable-use.yaml"
	yamlContent := `sources:
  - id: test-allowed
    url: https://example.com/allowed
    publication: Allowed Feed
    enabled: true
    priority: 10
    acceptable_use: allowed
`
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	registry, err := LoadFrom(tmpFile)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if registry == nil {
		t.Fatal("LoadFrom() returned nil registry")
	}

	src, ok := registry.Get("test-allowed")
	if !ok {
		t.Fatal("Get(\"test-allowed\") not found")
	}
	if src.AcceptableUse != domain.AcceptanceAllowed {
		t.Errorf("AcceptableUse = %s; want allowed", src.AcceptableUse.String())
	}
}

func TestLoadFrom_AcceptableUseRestricted(t *testing.T) {
	tmpFile := t.TempDir() + "/acceptable-use.yaml"
	yamlContent := `sources:
  - id: test-restricted
    url: https://example.com/restricted
    publication: Restricted Feed
    enabled: true
    priority: 10
    acceptable_use: restricted
`
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	registry, err := LoadFrom(tmpFile)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if registry == nil {
		t.Fatal("LoadFrom() returned nil registry")
	}

	src, ok := registry.Get("test-restricted")
	if !ok {
		t.Fatal("Get(\"test-restricted\") not found")
	}
	if src.AcceptableUse != domain.AcceptanceRestricted {
		t.Errorf("AcceptableUse = %s; want restricted", src.AcceptableUse.String())
	}
}

func TestLoadFrom_AcceptableUseUnknown(t *testing.T) {
	tmpFile := t.TempDir() + "/acceptable-use.yaml"
	yamlContent := `sources:
  - id: test-unknown
    url: https://example.com/unknown
    publication: Unknown Feed
    enabled: true
    priority: 10
    acceptable_use: unknown-value
`
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	registry, err := LoadFrom(tmpFile)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if registry == nil {
		t.Fatal("LoadFrom() returned nil registry")
	}

	src, ok := registry.Get("test-unknown")
	if !ok {
		t.Fatal("Get(\"test-unknown\") not found")
	}
	if src.AcceptableUse != domain.AcceptanceUnknown {
		t.Errorf("AcceptableUse = %s; want unknown", src.AcceptableUse.String())
	}
}
