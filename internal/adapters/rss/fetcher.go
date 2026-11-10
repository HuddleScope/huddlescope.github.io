// Package rss provides adapters for fetching RSS feeds.
package rss

import (
	"bytes"
	"context"
	cryptoRand "crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// FeedResult represents the result of a feed fetch operation.
type FeedResult struct {
	Entries      []domain.FeedEntry
	Unchanged    bool // true on 304 Not Modified (no new entries)
	ETag         string
	LastModified string
	SourceID     string
	ItemCount    int
}

// RetryPolicy defines the retry behavior for failed requests.
type RetryPolicy struct {
	Attempts  int
	BaseDelay time.Duration
	MaxDelay  time.Duration
	Jitter    func(base time.Duration) time.Duration
}

// DefaultRetryPolicy is the default retry configuration.
var DefaultRetryPolicy = RetryPolicy{
	Attempts:  3,
	BaseDelay: 500 * time.Millisecond,
	MaxDelay:  30 * time.Second,
	Jitter:    func(base time.Duration) time.Duration { return time.Duration(int64(float64(base) * randomFloat64())) },
}

// randomFloat64 returns a random float64 in [0, 1) using crypto/rand.
func randomFloat64() float64 {
	var b [8]byte
	_, err := cryptoRand.Read(b[:])
	if err != nil {
		// Fallback to math/rand if crypto/rand fails
		return rand.Float64() //nolint:G404
	}
	// Convert 8 bytes to float64 in [0, 1)
	return float64(binary.BigEndian.Uint64(b[:])) / (1 << 64)
}

// HostRateLimiter enforces a minimum interval between requests to the same host.
type HostRateLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	next     map[string]time.Time
	now      func() time.Time
}

// NewHostRateLimiter creates a new rate limiter with the given minimum interval between requests.
func NewHostRateLimiter(interval time.Duration) *HostRateLimiter {
	return &HostRateLimiter{
		interval: interval,
		next:     make(map[string]time.Time),
		now:      time.Now,
	}
}

// Wait blocks until the minimum interval has elapsed since the last request to this host.
func (l *HostRateLimiter) Wait(ctx context.Context, host string) error {
	// Check if context is already cancelled
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	if next, ok := l.next[host]; ok && now.Before(next) {
		waitFor := next.Sub(now)
		if waitFor > l.interval {
			waitFor = l.interval
		}

		// Check if context is cancelled before waiting
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitFor):
			// continue
		}

		now = l.now()
	}

	l.next[host] = now.Add(l.interval)
	return nil
}

// ETagCache stores ETag and Last-Modified headers per URL.
type ETagCache struct {
	mu      sync.RWMutex
	entries map[string]*etagEntry
}

type etagEntry struct {
	ETag         string
	LastModified string
}

// NewETagCache creates a new ETag cache.
func NewETagCache() *ETagCache {
	return &ETagCache{
		entries: make(map[string]*etagEntry),
	}
}

// Get returns the cached ETag and Last-Modified for a URL, if any.
func (c *ETagCache) Get(url string) (etag, lastModified string, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[url]
	if !ok {
		return "", "", false
	}
	return e.ETag, e.LastModified, ok
}

// Set stores the ETag and Last-Modified headers from an HTTP response.
func (c *ETagCache) Set(url string, etag, lastModified string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[url] = &etagEntry{ETag: etag, LastModified: lastModified}
}

// Clear removes cached validators for a URL.
func (c *ETagCache) Clear(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, url)
}

// HTTPFetcher implements domain.FeedFetcher with robust fetching.
type HTTPFetcher struct {
	source         domain.FeedSource
	client         *http.Client
	parser         *gofeed.Parser
	userAgent      string
	defaultTimeout time.Duration
	retryPolicy    RetryPolicy
	limiter        *HostRateLimiter
	etagCache      *ETagCache
	maxBytes       int64
}

// Option configures an HTTPFetcher.
type Option func(*HTTPFetcher)

// WithClient sets the HTTP client.
func WithClient(c *http.Client) Option {
	return func(f *HTTPFetcher) {
		f.client = c
	}
}

// WithUserAgent sets the User-Agent header.
func WithUserAgent(ua string) Option {
	return func(f *HTTPFetcher) {
		f.userAgent = ua
	}
}

// WithDefaultTimeout sets the default timeout (used when source.Timeout is 0).
func WithDefaultTimeout(d time.Duration) Option {
	return func(f *HTTPFetcher) {
		f.defaultTimeout = d
	}
}

// WithRetryPolicy sets the retry behavior.
func WithRetryPolicy(p RetryPolicy) Option {
	return func(f *HTTPFetcher) {
		f.retryPolicy = p
	}
}

// WithRateLimiter sets the per-host rate limiter.
func WithRateLimiter(l *HostRateLimiter) Option {
	return func(f *HTTPFetcher) {
		f.limiter = l
	}
}

// WithETagCache sets the ETag cache for conditional GET.
func WithETagCache(c *ETagCache) Option {
	return func(f *HTTPFetcher) {
		f.etagCache = c
	}
}

// WithMaxBodyBytes sets the maximum response body size.
func WithMaxBodyBytes(n int64) Option {
	return func(f *HTTPFetcher) {
		f.maxBytes = n
	}
}

// NewHTTPFetcher creates a new HTTP fetcher for a feed source.
func NewHTTPFetcher(src domain.FeedSource, opts ...Option) *HTTPFetcher {
	f := &HTTPFetcher{
		source:         src,
		client:         &http.Client{},
		parser:         gofeed.NewParser(),
		userAgent:      "huddlescope/0.1",
		defaultTimeout: 30 * time.Second,
		retryPolicy:    DefaultRetryPolicy,
		maxBytes:       10 * 1024 * 1024, // 10 MiB
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// Fetch implements domain.FeedFetcher.
func (f *HTTPFetcher) Fetch(sourceID string) ([]domain.FeedEntry, error) {
	ctx := context.Background()
	result, err := f.FetchFeed(ctx)
	if err != nil {
		return nil, err
	}
	return result.Entries, nil
}

// FetchFeed fetches a feed and returns the full result.
func (f *HTTPFetcher) FetchFeed(ctx context.Context) (*FeedResult, error) {
	// Validate URL scheme
	u, err := url.Parse(f.source.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %q: %w", f.source.URL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("URL must use http or https, got %s", u.Scheme)
	}

	// Resolve timeout
	timeout := f.source.Timeout
	if timeout <= 0 {
		timeout = f.defaultTimeout
	}

	// Create request context with timeout
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Wait for rate limiter
	if f.limiter != nil {
		if err := f.limiter.Wait(reqCtx, u.Host); err != nil {
			return nil, fmt.Errorf("rate limit wait failed: %w", err)
		}
	}

	// Prepare request
	req, err := http.NewRequestWithContext(reqCtx, "GET", f.source.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", f.userAgent)

	// Add conditional GET headers if we have cached validators
	if f.etagCache != nil {
		if etag, lastModified, ok := f.etagCache.Get(f.source.URL); ok {
			if etag != "" {
				req.Header.Set("If-None-Match", etag)
			}
			if lastModified != "" {
				req.Header.Set("If-Modified-Since", lastModified)
			}
		}
	}

	// Perform request
	var resp *http.Response
	var attempt int
	var lastErr error

	for attempt = 1; attempt <= f.retryPolicy.Attempts; attempt++ {
		resp, lastErr = f.client.Do(req)
		if lastErr == nil && resp != nil {
			// Check response status
			if resp.StatusCode == http.StatusNotModified {
				// 304 Not Modified is a success case - content hasn't changed
				break
			}
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				// For 4xx errors, only retry on 429 (rate limited)
				if resp.StatusCode == http.StatusTooManyRequests {
					// Continue to retry for 429
				} else if resp.StatusCode >= 400 && resp.StatusCode < 500 {
					// 4xx errors are not retryable (except 429)
					resp.Body.Close() //nolint:errcheck
					return nil, fmt.Errorf("request failed with status %d: %w", resp.StatusCode, lastErr)
				}
				// For 5xx errors, continue to retry
			} else {
				// Success (2xx)
				break
			}
		}

		if lastErr != nil {
			// Network error, check if it's a context error
			if errors.Is(lastErr, context.Canceled) || errors.Is(lastErr, context.DeadlineExceeded) {
				return nil, fmt.Errorf("request cancelled or timed out after %d attempts: %w", attempt, lastErr)
			}
		}

		// If we have a response, close the body before retry
		if resp != nil {
			resp.Body.Close() //nolint:errcheck
		}

		if attempt < f.retryPolicy.Attempts {
			delay := f.calculateBackoff(attempt)
			select {
			case <-reqCtx.Done():
				return nil, fmt.Errorf("request cancelled during backoff: %w", reqCtx.Err())
			case <-time.After(delay):
				// Continue to next attempt
			}
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("request failed after %d attempts: %w", attempt-1, lastErr)
	}

	// Process response
	result := &FeedResult{SourceID: f.source.ID}

	// Handle 304 Not Modified (unchanged) - this is a success case
	if resp != nil && resp.StatusCode == http.StatusNotModified {
		resp.Body.Close() //nolint:errcheck
		result.Unchanged = true
		return result, nil
	}

	// Check for error status codes
	if resp != nil && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		resp.Body.Close() //nolint:errcheck
		return nil, fmt.Errorf("request failed after %d attempts with status %d: %w", attempt-1, resp.StatusCode, fmt.Errorf("unexpected status code %d", resp.StatusCode))
	}

	// Read body with limit
	var bodyBuf bytes.Buffer
	lr := io.LimitReader(resp.Body, f.maxBytes+1)
	n, err := io.Copy(&bodyBuf, lr)
	if err != nil {
		resp.Body.Close() //nolint:errcheck
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	resp.Body.Close() //nolint:errcheck

	// Check for body size limit
	if n > f.maxBytes {
		return nil, fmt.Errorf("response body exceeds maximum size of %d bytes", f.maxBytes)
	}

	// Decode body based on charset if needed
	bodyBytes := bodyBuf.Bytes()
	if charset := getCharsetFromHeader(resp.Header.Get("Content-Type")); charset != "" {
		var err error
		bodyBytes, err = decodeWithCharset(bodyBytes, charset) //nolint:ineffassign,staticcheck
		if err != nil {
			return nil, fmt.Errorf("failed to decode response body with charset %s: %w", charset, err)
		}
	}

	// Update cache with validators if present
	if f.etagCache != nil {
		etag := resp.Header.Get("ETag")
		lastModified := resp.Header.Get("Last-Modified")
		if etag != "" || lastModified != "" {
			f.etagCache.Set(f.source.URL, etag, lastModified)
		} else if etag == "" && lastModified == "" && len(bodyBuf.Bytes()) > 0 {
			// 200 without validators → clear cached validators
			f.etagCache.Clear(f.source.URL)
		}
	}

	// Parse feed
	feed, err := f.parser.Parse(bytes.NewReader(bodyBuf.Bytes()))
	if err != nil {
		return nil, fmt.Errorf("failed to parse feed: %w", err)
	}

	// Map items to domain.FeedEntry
	for _, item := range feed.Items {
		entry := domain.FeedEntry{
			Title:       strings.TrimSpace(item.Title),
			Source:      f.source.ID,
			PublishedAt: parseTime(item.PublishedParsed, item.UpdatedParsed),
			Summary:     strings.TrimSpace(item.Description),
			URL:         item.Link,
		}
		if entry.URL == "" && item.GUID != "" {
			entry.URL = item.GUID
		}
		if entry.Summary == "" && item.Content != "" {
			entry.Summary = strings.TrimSpace(item.Content)
		}
		result.Entries = append(result.Entries, entry)
	}

	result.ItemCount = len(result.Entries)

	return result, nil
}

// parseTime returns a time.Time from parsed time values, with fallback to zero.
func parseTime(published, updated *time.Time) time.Time {
	if published != nil {
		return *published
	}
	if updated != nil {
		return *updated
	}
	return time.Time{}
}

// calculateBackoff computes the delay for the given attempt number.
func (f *HTTPFetcher) calculateBackoff(attempt int) time.Duration {
	if f.retryPolicy.Jitter == nil {
		// Use no jitter if not configured
		return f.calculateSimpleBackoff(attempt)
	}
	delay := f.retryPolicy.BaseDelay * time.Duration(1<<(attempt-1)) // 2^(attempt-1)
	if delay > f.retryPolicy.MaxDelay {
		delay = f.retryPolicy.MaxDelay
	}
	return f.retryPolicy.Jitter(delay)
}

// calculateSimpleBackoff computes delay without jitter.
func (f *HTTPFetcher) calculateSimpleBackoff(attempt int) time.Duration {
	delay := f.retryPolicy.BaseDelay * time.Duration(1<<(attempt-1)) // 2^(attempt-1)
	if delay > f.retryPolicy.MaxDelay {
		delay = f.retryPolicy.MaxDelay
	}
	return delay
}

// getCharsetFromHeader extracts charset from Content-Type header.
func getCharsetFromHeader(contentType string) string {
	parts := strings.Split(contentType, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(part), "charset=") {
			charset := strings.TrimPrefix(part, "charset=")
			return strings.Trim(strings.ToLower(charset), "\"")
		}
	}
	return ""
}

// decodeWithCharset decodes bytes from the given charset to UTF-8.
func decodeWithCharset(data []byte, charset string) ([]byte, error) {
	// Handle common charsets
	charset = strings.ToLower(charset)
	switch charset {
	case "iso-8859-1", "iso8859-1", "latin1", "latin-1":
		// ISO-8859-1 maps directly to first 256 Unicode codepoints
		// We need to properly decode it
		decoded := make([]rune, len(data))
		for i, b := range data {
			decoded[i] = rune(b)
		}
		return []byte(string(decoded)), nil
	case "utf-8", "utf8":
		return data, nil
	default:
		// For other charsets, try to use golang.org/x/text
		return nil, fmt.Errorf("unsupported charset: %s", charset)
	}
}
