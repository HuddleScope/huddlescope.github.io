// Package hugo provides a concrete FeedHealthPublisher that writes to the Hugo data directory.
package hugo

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// FeedsDataPublisher implements domain.FeedHealthPublisher by writing to the Hugo data directory.
type FeedsDataPublisher struct {
	OutDir   string
	DataFile string
	BaseURL  string
}

// Publish writes the feed health snapshot to the configured data file atomically.
// If the content hasn't changed (excluding generatedAt), the file is not rewritten.
func (p *FeedsDataPublisher) Publish(ctx context.Context, feedHealth domain.FeedHealth) (domain.PublicationResult, error) {
	result := domain.PublicationResult{
		Committed:   false,
		Changed:     false,
		Path:        "",
		Version:     domain.FeedHealthVersion,
		PublishedAt: time.Now().UTC().Format(time.RFC3339),
		Errors:      nil,
	}

	// Validate
	if err := feedHealth.Validate(); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("feed health validation failed: %w", err))
		return result, nil
	}

	// Compute the data file path
	dataPath := filepath.Join(p.OutDir, p.DataFile)

	// Read existing file if present
	var existingContent []byte
	// #nosec G304: dataPath is OutDir + configured DataFile, both operator-controlled
	if existing, err := os.ReadFile(dataPath); err == nil {
		existingContent = existing
	}

	// Render the feed health
	rendered, err := feedHealth.Render()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("render feed health failed: %w", err))
		return result, nil
	}

	// Compare content (excluding generatedAt) to determine if write is needed
	if existingContent != nil {
		// Parse both to compare content
		var existingFH domain.FeedHealth
		if err := json.Unmarshal(existingContent, &existingFH); err != nil {
			// Existing file is corrupted; proceed with overwrite
			// Note: in production, you might want to log this
		} else {
			// Compare content only (excluding generatedAt)
			if existingFH.ContentEqual(feedHealth) {
				// Content unchanged, skip write
				result.Committed = true
				result.Changed = false
				result.Path = dataPath
				result.Version = domain.FeedHealthVersion
				result.PublishedAt = existingFH.GeneratedAt
				return result, nil
			}
		}
	}

	// Ensure output directory exists
	if err := os.MkdirAll(p.OutDir, 0o750); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("mkdir failed: %w", err))
		return result, nil
	}

	// Atomic write: temp file then rename
	tmpPath := dataPath + ".tmp"
	// #nosec G306: the rendered feed health is public static site data and must stay world-readable
	if err := os.WriteFile(tmpPath, rendered, 0o644); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("temp write failed: %w", err))
		return result, nil
	}

	// Compute hash for debugging (optional)
	_ = sha256.Sum256(rendered)

	if err := os.Rename(tmpPath, dataPath); err != nil {
		// Clean up temp on failure
		_ = os.Remove(tmpPath)
		result.Errors = append(result.Errors, fmt.Errorf("rename failed: %w", err))
		return result, nil
	}

	// Publication successful
	result.Committed = true
	result.Changed = true
	result.Path = dataPath
	result.Version = domain.FeedHealthVersion
	result.PublishedAt = feedHealth.GeneratedAt

	return result, nil
}

// NewFeedsDataPublisher creates a new FeedsDataPublisher with the given directory and filename.
func NewFeedsDataPublisher(outDir, dataFile string) *FeedsDataPublisher {
	return &FeedsDataPublisher{
		OutDir:   outDir,
		DataFile: dataFile,
	}
}

// NewFeedsDataPublisherWithBaseURL creates a FeedsDataPublisher with optional base URL.
func NewFeedsDataPublisherWithBaseURL(outDir, dataFile, baseURL string) *FeedsDataPublisher {
	return &FeedsDataPublisher{
		OutDir:   outDir,
		DataFile: dataFile,
		BaseURL:  baseURL,
	}
}

// NoopFeedsDataPublisher is a no-op publisher for testing or offline scenarios.
type NoopFeedsDataPublisher struct{}

func (p *NoopFeedsDataPublisher) Publish(ctx context.Context, feedHealth domain.FeedHealth) (domain.PublicationResult, error) {
	return domain.PublicationResult{
		Committed:   true,
		Changed:     false,
		Path:        "",
		Version:     domain.FeedHealthVersion,
		PublishedAt: time.Now().UTC().Format(time.RFC3339),
		Errors:      nil,
	}, nil
}

// Test helpers

// GetHash computes the SHA256 hash of rendered feed health content.
func (p *FeedsDataPublisher) GetHash(feedHealth domain.FeedHealth) ([32]byte, error) {
	rendered, err := feedHealth.Render()
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(rendered), nil
}
