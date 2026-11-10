package domain

import (
	"fmt"
	"testing"
)

func TestFeedRegistry_Sorting(t *testing.T) {
	// Test that All and Enabled both return sorted results
	sources := []FeedSource{
		{ID: "z", URL: "https://z.com", Priority: 10, Enabled: true},
		{ID: "a", URL: "https://a.com", Priority: 1, Enabled: false}, // disabled, should not appear in Enabled()
		{ID: "b", URL: "https://b.com", Priority: 1, Enabled: true},
		{ID: "c", URL: "https://c.com", Priority: 5, Enabled: true},
		{ID: "y", URL: "https://y.com", Priority: 1, Enabled: true}, // same priority as a,b, but later ID
	}

	registry, err := NewFeedRegistry(sources...)
	if err != nil {
		t.Fatalf("NewFeedRegistry() error = %v", err)
	}

	// All should include all sources, sorted
	all := registry.All()
	if len(all) != 5 {
		t.Fatalf("All() has %d items, expected 5", len(all))
	}
	// Check order: priority ascending, then ID
	expectedAll := []string{"a", "b", "y", "c", "z"} // a,b,y all priority 1 (sorted by ID), then c (5), then z (10)
	for i, src := range all {
		if src.ID != expectedAll[i] {
			t.Errorf("All()[%d] = %s (priority %d); want %s", i, src.ID, src.Priority, expectedAll[i])
		}
	}

	// Enabled should only include enabled sources, sorted
	enabled := registry.Enabled()
	if len(enabled) != 4 {
		t.Fatalf("Enabled() has %d items, expected 4", len(enabled))
	}
	// a is disabled, so order is: b, y, c, z
	expectedEnabled := []string{"b", "y", "c", "z"}
	for i, src := range enabled {
		if src.ID != expectedEnabled[i] {
			t.Errorf("Enabled()[%d] = %s (priority %d); want %s", i, src.ID, src.Priority, expectedEnabled[i])
		}
		if src.Enabled != true {
			t.Errorf("Enabled()[%d] has Enabled=false", i)
		}
	}
}

func TestFeedRegistry_GetThreadSafety(t *testing.T) {
	sources := []FeedSource{
		{ID: "x", URL: "https://x.com", Priority: 0, Enabled: true},
	}

	registry, err := NewFeedRegistry(sources...)
	if err != nil {
		t.Fatalf("NewFeedRegistry() error = %v", err)
	}

	done := make(chan bool, 2)
	var result string

	// Concurrent reads
	go func() {
		src, ok := registry.Get("x")
		if ok && src != nil {
			result = src.ID
		}
		done <- true
	}()

	go func() {
		_, ok := registry.Get("x")
		if ok {
			done <- true
		}
	}()

	<-done
	<-done

	if result != "x" {
		t.Errorf("Concurrent Get() result = %s; want x", result)
	}
}

func TestFeedRegistry_StatusIsolation(t *testing.T) {
	// Test that RecordStatus for one ID doesn't affect another
	sources := []FeedSource{
		{ID: "a", URL: "https://a.com", Priority: 0, Enabled: true},
		{ID: "b", URL: "https://b.com", Priority: 1, Enabled: true},
	}

	registry, err := NewFeedRegistry(sources...)
	if err != nil {
		t.Fatalf("NewFeedRegistry() error = %v", err)
	}

	// Record status for 'a' only
	errA := fmt.Errorf("failed for a")
	registry.RecordStatus("a", false, errA)

	// Check 'a' has error, 'b' is clean
	statusA := registry.Status("a")
	statusB := registry.Status("b")

	if statusA.LastError == nil || statusA.LastError.Error() != errA.Error() {
		t.Errorf("Status(\"a\").LastError = %v; want %v", statusA.LastError, errA)
	}
	if statusA.LastOK != false {
		t.Errorf("Status(\"a\").LastOK = %v; want false", statusA.LastOK)
	}

	if statusB.LastError != nil {
		t.Errorf("Status(\"b\").LastError = %v; want nil", statusB.LastError)
	}
	if statusB.LastOK != false {
		t.Errorf("Status(\"b\").LastOK = %v; want false (never recorded)", statusB.LastOK)
	}
}

func TestFeedRegistry_IDempotency(t *testing.T) {
	sources := []FeedSource{
		{ID: "a", URL: "https://a.com", Priority: 1, Enabled: true},
		{ID: "b", URL: "https://b.com", Priority: 2, Enabled: true},
	}

	registry1, err := NewFeedRegistry(sources...)
	if err != nil {
		t.Fatalf("First NewFeedRegistry() error = %v", err)
	}

	// Call All multiple times, should return same content
	all1 := registry1.All()
	all2 := registry1.All()
	all3 := registry1.All()

	if len(all1) != len(all2) || len(all2) != len(all3) {
		t.Errorf("All() calls returned different lengths: %d, %d, %d", len(all1), len(all2), len(all3))
	}

	// Check order stability
	for i := range all1 {
		if all1[i].ID != all2[i].ID || all2[i].ID != all3[i].ID {
			t.Errorf("All() order unstable at index %d", i)
		}
	}

	// Status should also be idempotent
	registry1.RecordStatus("a", true, nil)
	status1 := registry1.Status("a")
	status2 := registry1.Status("a")

	if status1.LastOK != status2.LastOK {
		t.Errorf("Status() not idempotent after RecordStatus")
	}
}

func TestNewFeedRegistry_Empty(t *testing.T) {
	registry, err := NewFeedRegistry()
	if err != nil {
		t.Fatalf("NewFeedRegistry() with no args error = %v", err)
	}
	if registry.Count() != 0 {
		t.Errorf("Empty registry Count() = %d; want 0", registry.Count())
	}
	if len(registry.All()) != 0 {
		t.Errorf("Empty registry All() length = %d; want 0", len(registry.All()))
	}
}

func TestNewFeedRegistry_SingleSource(t *testing.T) {
	sources := []FeedSource{
		{ID: "single", URL: "https://single.com", Priority: 0, Enabled: true, Language: "en"},
	}

	registry, err := NewFeedRegistry(sources...)
	if err != nil {
		t.Fatalf("NewFeedRegistry() error = %v", err)
	}

	if registry.Count() != 1 {
		t.Errorf("Count() = %d; want 1", registry.Count())
	}

	src, ok := registry.Get("single")
	if !ok {
		t.Fatal("Get(\"single\") not found")
	}
	if src.Language != "en" {
		t.Errorf("Get(\"single\").Language = %s; want en", src.Language)
	}
}

func TestFeedSource_ValidateURLScheme(t *testing.T) {
	// Test case-insensitivity
	invalidCases := []string{
		"ftp://example.com",
		"file:///etc/passwd",
		"mailto:test@example.com",
	}

	for _, url := range invalidCases {
		source := FeedSource{ID: "test", URL: url, Priority: 0, Enabled: true}
		err := source.Validate()
		if err == nil {
			t.Errorf("Validate() with URL %s: expected error, got nil", url)
		}
	}

	// Test valid schemes
	validCases := []string{
		"http://example.com",
		"https://example.com",
		"http://example.com/path",
		"https://example.com?query=1",
	}

	for _, url := range validCases {
		source := FeedSource{ID: "test", URL: url, Priority: 0, Enabled: true}
		err := source.Validate()
		if err != nil {
			t.Errorf("Validate() with valid URL %s: unexpected error %v", url, err)
		}
	}
}

func TestFeedRegistry_MultipleSuccessiveModifications(t *testing.T) {
	sources := []FeedSource{
		{ID: "test", URL: "https://test.com", Priority: 0, Enabled: true},
	}

	registry, err := NewFeedRegistry(sources...)
	if err != nil {
		t.Fatalf("NewFeedRegistry() error = %v", err)
	}

	// Multiple success status updates
	for i := 0; i < 10; i++ {
		registry.RecordStatus("test", true, nil)
		status := registry.Status("test")
		if !status.LastOK {
			t.Errorf("After %d RecordStatus calls, LastOK = false", i)
		}
	}

	// Multiple error status updates
	errExpected := fmt.Errorf("test error")
	for i := 0; i < 5; i++ {
		registry.RecordStatus("test", false, errExpected)
		status := registry.Status("test")
		if status.LastOK {
			t.Errorf("After error RecordStatus call, LastOK = true")
		}
	}
}

func TestFeedRegistry_MixedValidation(t *testing.T) {
	sources := []FeedSource{
		{ID: "valid", URL: "https://valid.com", Priority: 1, Enabled: true},
		{ID: "no-url", Priority: 2, Enabled: true}, // invalid
		{ID: "valid2", URL: "https://valid2.com", Priority: 3, Enabled: true},
	}

	_, err := NewFeedRegistry(sources...)
	if err == nil {
		t.Fatalf("NewFeedRegistry with invalid sources: expected error, got nil")
	}

	// Should have validation error
	errStr := err.Error()
	if !contains(errStr, "no-url") && !contains(errStr, "empty URL") {
		t.Errorf("Error message %q does not contain validation issue", errStr)
	}
}

func TestFeedRegistry_StatusTimeoutReset(t *testing.T) {
	sources := []FeedSource{
		{ID: "test", URL: "https://test.com", Priority: 0, Enabled: true},
	}

	registry, err := NewFeedRegistry(sources...)
	if err != nil {
		t.Fatalf("NewFeedRegistry() error = %v", err)
	}

	// Record failure
	registry.RecordStatus("test", false, fmt.Errorf("first error"))
	status1 := registry.Status("test")
	if status1.LastError == nil {
		t.Error("Status().LastError is nil; want error")
	}

	// Record success later
	registry.RecordStatus("test", true, nil)
	status2 := registry.Status("test")
	if status2.LastError != nil {
		t.Errorf("After success, LastError = %v; want nil", status2.LastError)
	}
	if !status2.LastOK {
		t.Error("After success, LastOK should be true")
	}
}
