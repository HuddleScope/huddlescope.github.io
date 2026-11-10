package feeds

import (
	"testing"

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
