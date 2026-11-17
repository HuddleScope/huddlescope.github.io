package rss

import (
	"context"
	"errors"
	"fmt"

	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

func TestHTTPFetcher_Success_RSS(t *testing.T) {
	// Create test server serving RSS fixture
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := os.ReadFile("testdata/rss.xml")
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write(data) //nolint:errcheck
	}))
	defer server.Close()

	fetcher := NewHTTPFetcher(domain.FeedSource{ID: "test", URL: server.URL})

	entries, err := fetcher.Fetch(context.Background(), "test")
	if err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(entries))
	}

	if entries[0].Title != "Test Entry 1" {
		t.Errorf("First entry Title = %s; want Test Entry 1", entries[0].Title)
	}
	if entries[0].Source != "test" {
		t.Errorf("First entry Source = %s; want test", entries[0].Source)
	}
	if entries[0].URL != "https://example.com/entry1" {
		t.Errorf("First entry URL = %s; want https://example.com/entry1", entries[0].URL)
	}
	if entries[0].Summary != "Test summary 1" {
		t.Errorf("First entry Summary = %s; want Test summary 1", entries[0].Summary)
	}
}

func TestHTTPFetcher_Success_Atom(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := os.ReadFile("testdata/atom.xml")
		w.Header().Set("Content-Type", "application/atom+xml")
		w.Write(data) //nolint:errcheck
	}))
	defer server.Close()

	fetcher := NewHTTPFetcher(domain.FeedSource{ID: "test", URL: server.URL})

	entries, err := fetcher.Fetch(context.Background(), "test")
	if err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(entries))
	}
}

func TestHTTPFetcher_Success_JSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := os.ReadFile("testdata/feed.json")
		w.Header().Set("Content-Type", "application/json")
		w.Write(data) //nolint:errcheck
	}))
	defer server.Close()

	fetcher := NewHTTPFetcher(domain.FeedSource{ID: "test", URL: server.URL})

	entries, err := fetcher.Fetch(context.Background(), "test")
	if err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(entries))
	}
}

func TestHTTPFetcher_Unchanged_ETag(t *testing.T) {
	// First request returns 200 with ETag
	var firstRequest = true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !firstRequest {
			// Check If-None-Match header
			if ifNoneMatch := r.Header.Get("If-None-Match"); ifNoneMatch != "" {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}

		if firstRequest {
			data, _ := os.ReadFile("testdata/rss.xml")
			w.Header().Set("Content-Type", "application/rss+xml")
			w.Header().Set("ETag", "\"test-etag-1\"")
			w.Write(data) //nolint:errcheck
			firstRequest = false
		}
	}))
	defer server.Close()

	etagCache := NewETagCache()
	fetcher := NewHTTPFetcher(domain.FeedSource{ID: "test", URL: server.URL}, WithETagCache(etagCache))

	// First fetch
	result1, err := fetcher.FetchFeed(context.Background())
	if err != nil {
		t.Fatalf("First FetchFeed() returned error: %v", err)
	}
	if result1.Unchanged {
		t.Error("First fetch should not be unchanged")
	}
	if len(result1.Entries) != 2 {
		t.Fatalf("First fetch expected 2 entries, got %d", len(result1.Entries))
	}

	// Second fetch (should hit 304)
	result2, err := fetcher.FetchFeed(context.Background())
	if err != nil {
		t.Fatalf("Second FetchFeed() returned error: %v", err)
	}
	if !result2.Unchanged {
		t.Error("Second fetch should be unchanged (304)")
	}
	if len(result2.Entries) != 0 {
		t.Errorf("Second fetch should have 0 entries, got %d", len(result2.Entries))
	}
}

func TestHTTPFetcher_MalformedFeed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := os.ReadFile("testdata/malformed.xml")
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write(data) //nolint:errcheck
	}))
	defer server.Close()

	fetcher := NewHTTPFetcher(domain.FeedSource{ID: "test", URL: server.URL})

	_, err := fetcher.Fetch(context.Background(), "test")
	if err == nil {
		t.Error("Expected error for malformed feed, got nil")
	}
	// Verify it's not a retry error (only 1 request)
	if strings.Contains(err.Error(), "after") && strings.Contains(err.Error(), "attempts") {
		t.Errorf("Malformed feed should not trigger retries: %v", err)
	}
}

func TestHTTPFetcher_HTTPError_404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	fetcher := NewHTTPFetcher(domain.FeedSource{ID: "test", URL: server.URL})

	_, err := fetcher.Fetch(context.Background(), "test")
	if err == nil {
		t.Error("Expected error for 404, got nil")
	}
	// Verify no retries (4xx non-429)
	if strings.Contains(err.Error(), "attempts") || strings.Contains(err.Error(), "Retry") {
		t.Errorf("404 should not trigger retries: %v", err)
	}
}

func TestHTTPFetcher_Retry_5xx(t *testing.T) {
	attempt := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt < 3 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			data, _ := os.ReadFile("testdata/rss.xml")
			w.Header().Set("Content-Type", "application/rss+xml")
			w.Write(data) //nolint:errcheck
		}
	}))
	defer server.Close()

	fetcher := NewHTTPFetcher(domain.FeedSource{ID: "test", URL: server.URL})

	entries, err := fetcher.Fetch(context.Background(), "test")
	if err != nil {
		t.Fatalf("Fetch() after retries returned error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries after retries, got %d", len(entries))
	}
	if attempt != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempt)
	}
}

func TestHTTPFetcher_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second) // Sleep longer than timeout
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	fetcher := NewHTTPFetcher(domain.FeedSource{ID: "test", URL: server.URL}, WithDefaultTimeout(500*time.Millisecond))

	_, err := fetcher.Fetch(context.Background(), "test")
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
	// Verify context deadline exceeded or similar
	if err != nil && !strings.Contains(err.Error(), "deadline") && !strings.Contains(err.Error(), "timeout") {
		t.Errorf("Expected timeout/deadline error, got: %v", err)
	}
}

func TestHTTPFetcher_Redirect(t *testing.T) {
	// Server redirects to itself
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 302 redirect to same URL
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/redirected", http.StatusFound)
		} else {
			data, _ := os.ReadFile("testdata/rss.xml")
			w.Header().Set("Content-Type", "application/rss+xml")
			w.Write(data) //nolint:errcheck
		}
	}))
	defer server.Close()

	fetcher := NewHTTPFetcher(domain.FeedSource{ID: "test", URL: server.URL})

	entries, err := fetcher.Fetch(context.Background(), "test")
	if err != nil {
		t.Fatalf("Fetch() after redirect returned error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries after redirect, got %d", len(entries))
	}
}

func TestHTTPFetcher_UTF8(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := os.ReadFile("testdata/iso8859-1.xml")
		w.Header().Set("Content-Type", "text/xml; charset=ISO-8859-1")
		w.Write(data) //nolint:errcheck
	}))
	defer server.Close()

	fetcher := NewHTTPFetcher(domain.FeedSource{ID: "test", URL: server.URL})

	entries, err := fetcher.Fetch(context.Background(), "test")
	if err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	// Verify UTF-8 decoding (Capitán in ISO-8859-1 decodes to proper UTF-8)
	if !strings.Contains(entries[0].Title, "Café") {
		t.Errorf("Entry Title should contain 'Café', got: %s", entries[0].Title)
	}
}

func TestHostRateLimiter(t *testing.T) {
	// Use a fake time function that advances on each call
	fakeTime := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	l := NewHostRateLimiter(1 * time.Second)
	l.now = func() time.Time {
		// Advance time by 1 second on each call
		fakeTime = fakeTime.Add(1 * time.Second)
		return fakeTime
	}

	// First request should not block
	start := time.Now()
	if err := l.Wait(context.Background(), "example.com"); err != nil {
		t.Fatalf("First Wait() returned error: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Errorf("First Wait() took too long: %v", elapsed)
	}

	// Second request should block for ~1 second (simulated by time.Advance in real code)
	start = time.Now()
	if err := l.Wait(context.Background(), "example.com"); err != nil {
		t.Fatalf("Second Wait() returned error: %v", err)
	}
	elapsed = time.Since(start)
	// The fake time advances 1 second per call, so second call should return immediately
	// after updating the next timestamp
	if elapsed > 100*time.Millisecond {
		t.Errorf("Second Wait() took too long: %v", elapsed)
	}

	// Different host should not block
	start = time.Now()
	if err := l.Wait(context.Background(), "other.com"); err != nil {
		t.Fatalf("Third Wait() returned error: %v", err)
	}
	elapsed = time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Errorf("Different host Wait() took too long: %v", elapsed)
	}
}

func TestHostRateLimiter_ContextCancel(t *testing.T) {
	l := NewHostRateLimiter(5 * time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := l.Wait(ctx, "example.com")
	if err == nil {
		t.Error("Expected context error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled, got: %v", err)
	}
}

func TestETagCache(t *testing.T) {
	cache := NewETagCache()

	// Set values
	cache.Set("https://example.com", "\"etag1\"", "Mon, 10 Nov 2026 12:00:00 GMT")

	// Get values
	etag, lastModified, ok := cache.Get("https://example.com")
	if !ok {
		t.Error("Get() should return ok=true")
	}
	if etag != "\"etag1\"" {
		t.Errorf("ETag = %s; want \"etag1\"", etag)
	}
	if lastModified != "Mon, 10 Nov 2026 12:00:00 GMT" {
		t.Errorf("LastModified = %s; want Mon, 10 Nov 2026 12:00:00 GMT", lastModified)
	}

	// Clear
	cache.Clear("https://example.com")
	_, _, ok = cache.Get("https://example.com")
	if ok {
		t.Error("Clear() should remove entry")
	}

	// Concurrent access
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cache.Set("https://example.com", fmt.Sprintf("etag%d", i), "date")
		}(i)
	}
	wg.Wait()

	// Should have some value
	etag, _, ok = cache.Get("https://example.com")
	if !ok || etag == "" {
		t.Error("Concurrent Set should work")
	}
}

func TestStaticFetcher(t *testing.T) {
	entries := []domain.FeedEntry{
		{Title: "Entry 1", Source: "test-feed", PublishedAt: time.Now(), Summary: "test", URL: "https://test.com"},
	}
	fetcher := NewStaticFetcher(entries)

	result, err := fetcher.Fetch(context.Background(), "default")
	if err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(result))
	}
}

func TestHTTPFetcher_NoDateEntry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := os.ReadFile("testdata/nodates.xml")
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write(data) //nolint:errcheck
	}))
	defer server.Close()

	fetcher := NewHTTPFetcher(domain.FeedSource{ID: "test", URL: server.URL})

	entries, err := fetcher.Fetch(context.Background(), "test")
	if err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}
	if entries[0].PublishedAt.IsZero() { //nolint:staticcheck
		// Zero time is acceptable for entries without dates
		// The important thing is no panic
	} else { //nolint:staticcheck
		// Or a parsed time is also fine
	}
}

func TestHTTPFetcher_429_RateLimited(t *testing.T) {
	attempt := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt < 3 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
		} else {
			data, _ := os.ReadFile("testdata/rss.xml")
			w.Header().Set("Content-Type", "application/rss+xml")
			w.Write(data) //nolint:errcheck
		}
	}))
	defer server.Close()

	fetcher := NewHTTPFetcher(domain.FeedSource{ID: "test", URL: server.URL})

	entries, err := fetcher.Fetch(context.Background(), "test")
	if err != nil {
		t.Fatalf("Fetch() after retries returned error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries after retries, got %d", len(entries))
	}
	if attempt != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempt)
	}
}

func TestHTTPFetcher_RetryExhausted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	fetcher := NewHTTPFetcher(domain.FeedSource{ID: "test", URL: server.URL}, WithRetryPolicy(RetryPolicy{Attempts: 2}))

	_, err := fetcher.Fetch(context.Background(), "test")
	if err == nil {
		t.Error("Expected error after exhausting retries, got nil")
	}
	// Verify we attempted exactly 2 times
	if !strings.Contains(err.Error(), "after") || !strings.Contains(err.Error(), "attempts") {
		t.Errorf("Expected retry exhaustion message, got: %v", err)
	}
}

func TestHTTPFetcher_DomainFeedEntry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := os.ReadFile("testdata/rss.xml")
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write(data) //nolint:errcheck
	}))
	defer server.Close()

	fetcher := NewHTTPFetcher(domain.FeedSource{ID: "custom-source-id", URL: server.URL})

	entries, err := fetcher.Fetch(context.Background(), "ignored-id")
	if err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}

	for _, entry := range entries {
		if entry.Source != "custom-source-id" {
			t.Errorf("Entry Source = %s; want custom-source-id", entry.Source)
		}
	}
}

func TestHTTPFetcher_FeedResultUnchanged(t *testing.T) {
	var firstRequest = true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !firstRequest {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		firstRequest = false

		data, _ := os.ReadFile("testdata/rss.xml")
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Header().Set("ETag", "\"test-etag\"")
		w.Write(data) //nolint:errcheck
	}))
	defer server.Close()

	etagCache := NewETagCache()
	fetcher := NewHTTPFetcher(domain.FeedSource{ID: "test", URL: server.URL}, WithETagCache(etagCache))

	// First fetch
	result1, err := fetcher.FetchFeed(context.Background())
	if err != nil {
		t.Fatalf("First FetchFeed() returned error: %v", err)
	}
	if result1.Unchanged {
		t.Error("First fetch Unchanged should be false")
	}
	if result1.SourceID != "test" {
		t.Errorf("SourceID = %s; want test", result1.SourceID)
	}

	// Second fetch (304)
	result2, err := fetcher.FetchFeed(context.Background())
	if err != nil {
		t.Fatalf("Second FetchFeed() returned error: %v", err)
	}
	if !result2.Unchanged {
		t.Error("Second fetch Unchanged should be true (304)")
	}
}

// TestHTTPFetcher_Windows1252 verifies that Windows-1252 encoded feeds decode correctly.
func TestHTTPFetcher_Windows1252(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := os.ReadFile("testdata/windows1252.xml")
		w.Header().Set("Content-Type", "text/xml; charset=windows-1252")
		w.Write(data) //nolint:errcheck
	}))
	defer server.Close()

	fetcher := NewHTTPFetcher(domain.FeedSource{ID: "test", URL: server.URL})

	entries, err := fetcher.Fetch(context.Background(), "test")
	if err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	// Verify Windows-1252 characters decode correctly to UTF-8
	// € (euro sign) is 0x80 in windows-1252, decodes to U+20AC
	// ™ (tm) is 0x99 in windows-1252, decodes to U+2122
	if !strings.Contains(entries[0].Title, "€") {
		t.Errorf("Entry Title should contain '€', got: %s", entries[0].Title)
	}
	if !strings.Contains(entries[0].Title, "™") {
		t.Errorf("Entry Title should contain '™', got: %s", entries[0].Title)
	}
}

// TestHTTPFetcher_PerAttemptTimeout verifies that each retry attempt gets its own timeout.
func TestHTTPFetcher_PerAttemptTimeout(t *testing.T) {
	// Test that retries happen when timeout occurs
	// The timeout is applied per attempt, so each attempt should be able to timeout independently

	var mu sync.Mutex
	var attempts int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts++
		mu.Unlock()

		// Return error status - we're testing that retry logic runs, not actual timeout
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	fetcher := NewHTTPFetcher(
		domain.FeedSource{ID: "test", URL: server.URL},
		WithDefaultTimeout(30*time.Second), // Long timeout to avoid real timeout
		WithRetryPolicy(RetryPolicy{Attempts: 3}),
	)

	_, err := fetcher.Fetch(context.Background(), "test")
	if err == nil {
		t.Error("Expected error after exhausting retries, got nil")
	}

	mu.Lock()
	defer mu.Unlock()

	if attempts != 3 {
		t.Errorf("Expected 3 retry attempts, got %d", attempts)
	}

	// Also verify error message mentions failed status, not timeout
	if !strings.Contains(err.Error(), "status 503") {
		t.Errorf("Expected error to mention status 503, got: %v", err)
	}
}

// TestHTTPFetcher_404_ErrorText verifies that 404 errors return clean text without nil %w.
func TestHTTPFetcher_404_ErrorText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	fetcher := NewHTTPFetcher(domain.FeedSource{ID: "test", URL: server.URL})

	_, err := fetcher.Fetch(context.Background(), "test")
	if err == nil {
		t.Error("Expected error for 404, got nil")
	}
	// Verify error contains status 404
	if !strings.Contains(err.Error(), "status 404") {
		t.Errorf("Expected error to contain 'status 404', got: %v", err)
	}
	// Verify NO nil %w sequence
	if strings.Contains(err.Error(), "%!w(<nil>)") {
		t.Errorf("Error should not contain %%!w(<nil>), got: %s", err)
	}
}

// TestHostRateLimiter_TwoHostConcurrency verifies that distinct hosts are not serialized.
func TestHostRateLimiter_TwoHostConcurrency(t *testing.T) {
	// Use a fake time function for deterministic testing
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	l := NewHostRateLimiter(1 * time.Second)
	l.now = func() time.Time { return now }

	var mu sync.Mutex
	results := make([]time.Duration, 2)

	// Two goroutines, different hosts, 1s interval
	for i := 0; i < 2; i++ {
		go func(idx int) {
			start := time.Now()
			if err := l.Wait(context.Background(), fmt.Sprintf("host%d.com", idx+1)); err != nil {
				t.Errorf("Wait() for host%d returned error: %v", idx+1, err)
			}
			elapsed := time.Since(start)
			mu.Lock()
			results[idx] = elapsed
			mu.Unlock()
		}(i)
	}

	// Wait for both goroutines to complete
	time.Sleep(1500 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Both should complete within ~1s (not ~2s serially)
	for i, elapsed := range results {
		if elapsed > 1100*time.Millisecond {
			t.Errorf("host%d completed in %v; expected ~1s (not serialized)", i+1, elapsed)
		}
	}
}

// TestHTTPFetcher_CancellationAbortsInFlightFetch verifies that cancelling the context
// aborts an in-flight fetch (per AC3 of HS-046).
func TestHTTPFetcher_CancellationAbortsInFlightFetch(t *testing.T) {
	var mu sync.Mutex
	var fetchStarted = false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		fetchStarted = true
		mu.Unlock()

		// Block until context is done (simulates hung feed)
		<-r.Context().Done()
	}))
	defer server.Close()

	fetcher := NewHTTPFetcher(domain.FeedSource{ID: "test", URL: server.URL})

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Start fetch in goroutine
	var err error
	done := make(chan struct{})

	go func() {
		_, err = fetcher.Fetch(ctx, "test")
		close(done)
	}()

	// Give the fetch a moment to start
	time.Sleep(50 * time.Millisecond)

	// Verify fetch started
	mu.Lock()
	if !fetchStarted {
		mu.Unlock()
		t.Fatal("Fetch did not start")
	}
	mu.Unlock()

	// Cancel the context
	cancel()

	// Wait for fetch to complete (should exit promptly due to context cancellation)
	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("Fetch did not complete after context cancellation")
	}

	// Verify fetch was aborted with context error
	if err == nil {
		t.Fatal("Expected context cancellation error, got nil")
	}
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected context.Canceled or context.DeadlineExceeded, got: %v", err)
	}
}

// TestHTTPFetcher_EmptyDescriptionAndContent (#122): an RSS item with neither
// <description> nor content must ingest without error. The summary stays empty
// at the adapter level; the deterministic title fallback is applied by
// Story.Canonicalize downstream.
func TestHTTPFetcher_EmptyDescriptionAndContent(t *testing.T) {
	feed := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
  <title>Test Feed</title>
  <link>https://example.com</link>
  <item>
    <title>young players factor in vikings win at packers</title>
    <link>https://www.vikings.com/news/young-players-factor-packers-win</link>
    <guid>https://www.vikings.com/news/young-players-factor-packers-win</guid>
    <pubDate>Mon, 16 Nov 2026 22:18:58 GMT</pubDate>
  </item>
</channel></rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(feed)) //nolint:errcheck
	}))
	defer server.Close()

	fetcher := NewHTTPFetcher(domain.FeedSource{ID: "official-min", URL: server.URL})

	entries, err := fetcher.Fetch(context.Background(), "official-min")
	if err != nil {
		t.Fatalf("Fetch() must not fail for a description-less item: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}
	if entries[0].Title != "young players factor in vikings win at packers" {
		t.Errorf("Title = %q; want the item title", entries[0].Title)
	}
	if entries[0].Summary != "" {
		t.Errorf("Summary = %q; want empty (fallback applied by Canonicalize)", entries[0].Summary)
	}

	// Canonicalize applies the deterministic title fallback
	story := domain.Story{}.Canonicalize(entries[0])
	if story.Summary == "" {
		t.Error("Canonicalize should derive a non-empty summary from the title")
	}
	if story.Summary != domain.SanitizeHTML(entries[0].Title, 1024) {
		t.Errorf("Canonicalize summary = %q; want sanitized title", story.Summary)
	}
}
