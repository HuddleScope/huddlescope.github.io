// Package rss provides adapters for RSS/Atom feed operations.
package rss

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
	"github.com/mmcdole/gofeed"
)

// Verdict represents the health status of a source
type Verdict string

const (
	VerdictHealthy    Verdict = "healthy"
	VerdictNotChecked Verdict = "not_checked"
	VerdictFailed     Verdict = "failed"
	VerdictDisabled   Verdict = "disabled"
)

// SourceCheck represents the result of validating a single RSS source
type SourceCheck struct {
	ID             string
	HTTPStatus     int
	ContentType    string
	Parseable      bool
	ItemCount      int
	NewestEntryAge time.Duration
	BodySize       int64
	Error          string
	Verdict        Verdict
}

// SourceCheckOptions contains optional configuration for validation
type SourceCheckOptions struct {
	BodySizeLimit int64
}

// Validator validates RSS sources
type Validator struct {
	client      *http.Client
	opts        SourceCheckOptions
	maxBodySize int64
}

// NewValidator creates a new RSS validator with the given HTTP client
func NewValidator(client *http.Client, opts SourceCheckOptions) *Validator {
	if opts.BodySizeLimit == 0 {
		opts.BodySizeLimit = 10 * 1024 * 1024 // 10MB default
	}
	return &Validator{
		client:      client,
		opts:        opts,
		maxBodySize: opts.BodySizeLimit,
	}
}

// Check validates a single RSS source and returns the result
func (v *Validator) Check(ctx context.Context, src domain.FeedSource) (SourceCheck, error) {
	var result SourceCheck

	// Build request
	url := src.URL
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		result.Error = err.Error()
		result.Verdict = VerdictFailed
		return result, nil
	}

	// Accept headers for XML/Atom
	req.Header.Set("Accept", "application/rss+xml,application/atom+xml,*/*")
	req.Header.Set("User-Agent", "HuddleScope/RSS-Validator/0.1")
	result.HTTPStatus = 0

	// Fetch the feed
	resp, err := v.client.Do(req)
	if err != nil {
		result.Error = err.Error()
		result.Verdict = VerdictFailed
		return result, nil
	}
	_ = resp.Body.Close()

	result.HTTPStatus = resp.StatusCode

	// Check for bot protection page (common 402/429)
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		result.Error = fmt.Sprintf("%d: %s", resp.StatusCode, string(body[:min(100, len(body))]))
		result.Verdict = VerdictFailed
		return result, nil
	}

	// Read body
	var body []byte
	bodySize := int64(0)
	if resp.ContentLength > 0 && resp.ContentLength <= v.maxBodySize {
		body, err = io.ReadAll(resp.Body)
		bodySize = resp.ContentLength
	} else {
		if resp.ContentLength > v.maxBodySize {
			body, err = io.ReadAll(io.LimitReader(resp.Body, v.maxBodySize+1))
			bodySize = v.maxBodySize + 1
		} else {
			body, err = io.ReadAll(resp.Body)
			bodySize = resp.ContentLength
		}
	}

	result.BodySize = bodySize
	if err != nil {
		result.Error = err.Error()
		result.Verdict = VerdictFailed
		return result, nil
	}

	// Check HTTP status
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		result.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		result.Verdict = VerdictFailed
		return result, nil
	}

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "text/plain; charset=utf-8"
	}
	result.ContentType = contentType

	// Parse the feed
	reader := gofeed.NewParser()
	reader.UserAgent = "HuddleScope/RSS-Validator/0.1"
	feed, parseErr := reader.ParseString(string(body))
	result.Parseable = parseErr == nil

	if parseErr != nil {
		result.Error = fmt.Sprintf("parse error: %v", parseErr)
		result.Verdict = VerdictFailed
		return result, nil
	}

	// Check items
	result.ItemCount = len(feed.Items)
	if result.ItemCount == 0 {
		result.Error = "empty feed"
		result.Verdict = VerdictFailed
		return result, nil
	}

	// Check freshness (newest item age)
	oldestItem := time.Now().Add(24 * time.Hour)
	for _, item := range feed.Items {
		published, parseErr := time.Parse(time.RFC3339, item.Published)
		if parseErr == nil && !published.IsZero() {
			if published.Before(oldestItem) {
				oldestItem = published
			}
		}
	}
	result.NewestEntryAge = time.Since(oldestItem)

	// Determine verdict
	if result.ItemCount > 0 && result.NewestEntryAge < 24*time.Hour {
		result.Verdict = VerdictHealthy
	} else {
		result.Verdict = VerdictNotChecked
		if result.NewestEntryAge > 7*24*time.Hour {
			result.Error = fmt.Sprintf("stale feed: newest item %s ago", result.NewestEntryAge.Round(time.Hour))
		}
	}

	return result, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
