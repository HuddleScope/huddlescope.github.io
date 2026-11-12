// Package hugo provides a concrete SnapshotPublisher that writes to the Hugo data directory.
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

// DataPublisher implements domain.SnapshotPublisher by writing to the Hugo data directory.
type DataPublisher struct {
	OutDir   string
	DataFile string
	BaseURL  string
}

// Publish writes the snapshot to the configured data file atomically.
// If the content hasn't changed (excluding generatedAt), the file is not rewritten.
func (p *DataPublisher) Publish(ctx context.Context, snapshot domain.Snapshot) (domain.PublicationResult, error) {
	result := domain.PublicationResult{
		Committed:   false,
		Changed:     false,
		Path:        "",
		Version:     domain.SnapshotVersion,
		PublishedAt: time.Now().UTC().Format(time.RFC3339),
		Errors:      nil,
	}

	// Validate
	if err := snapshot.Validate(); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("snapshot validation failed: %w", err))
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

	// Render the snapshot
	rendered, err := snapshot.Render()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("render snapshot failed: %w", err))
		return result, nil
	}

	// Compare content (excluding generatedAt) to determine if write is needed
	if existingContent != nil {
		// Parse both to compare content
		var existingSnap domain.Snapshot
		if err := json.Unmarshal(existingContent, &existingSnap); err != nil {
			// Existing file is corrupted; proceed with overwrite
			// Note: in production, you might want to log this
		} else {
			// Compare content only (excluding generatedAt)
			if existingSnap.ContentEqual(snapshot) {
				// Content unchanged, skip write
				result.Committed = true
				result.Changed = false
				result.Path = dataPath
				result.Version = domain.SnapshotVersion
				result.PublishedAt = existingSnap.GeneratedAt
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
	// #nosec G306: the rendered snapshot is public static site data and must stay world-readable
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
	result.Version = domain.SnapshotVersion
	result.PublishedAt = snapshot.GeneratedAt

	return result, nil
}

// NewDataPublisher creates a new DataPublisher with the given directory and filename.
func NewDataPublisher(outDir, dataFile string) *DataPublisher {
	return &DataPublisher{
		OutDir:   outDir,
		DataFile: dataFile,
	}
}

// NewDataPublisherWithBaseURL creates a DataPublisher with optional base URL.
func NewDataPublisherWithBaseURL(outDir, dataFile, baseURL string) *DataPublisher {
	return &DataPublisher{
		OutDir:   outDir,
		DataFile: dataFile,
		BaseURL:  baseURL,
	}
}

// NoopDataPublisher is a no-op publisher for testing or offline scenarios.
type NoopDataPublisher struct{}

func (p *NoopDataPublisher) Publish(ctx context.Context, snapshot domain.Snapshot) (domain.PublicationResult, error) {
	return domain.PublicationResult{
		Committed:   true,
		Changed:     false,
		Path:        "",
		Version:     domain.SnapshotVersion,
		PublishedAt: time.Now().UTC().Format(time.RFC3339),
		Errors:      nil,
	}, nil
}

// Test helpers

// GetHash computes the SHA256 hash of rendered snapshot content.
func (p *DataPublisher) GetHash(snapshot domain.Snapshot) ([32]byte, error) {
	rendered, err := snapshot.Render()
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(rendered), nil
}
