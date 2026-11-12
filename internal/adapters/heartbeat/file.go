// Package heartbeat provides file-based persistence for heartbeat records.
package heartbeat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// ErrNotFound is returned when the heartbeat file does not exist.
var ErrNotFound = errors.New("heartbeat file not found")

// ErrCorrupt is returned when the heartbeat file is invalid JSON.
var ErrCorrupt = errors.New("heartbeat file is corrupt")

// FilePublisher implements domain.HeartbeatPublisher by writing to a file.
type FilePublisher struct {
	Path string
}

// Publish writes the heartbeat to the configured file atomically.
// If the directory does not exist, it is created with mode 0750.
func (p *FilePublisher) Publish(ctx context.Context, hb *domain.Heartbeat) error {
	if hb == nil {
		return errors.New("heartbeat cannot be nil")
	}
	if err := hb.Validate(); err != nil {
		return fmt.Errorf("heartbeat validation failed: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(p.Path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("mkdir failed: %w", err)
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(hb, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal failed: %w", err)
	}

	// Atomic write: write to temp file then rename
	tmpPath := p.Path + ".tmp"
	// #nosec G306: heartbeat is public site data and must stay world-readable
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("temp write failed: %w", err)
	}

	if err := os.Rename(tmpPath, p.Path); err != nil {
		// Clean up temp on failure
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename failed: %w", err)
	}

	return nil
}

// Load reads the heartbeat from the configured file.
// Returns (heartbeat, found, error).
func Load(path string) (*domain.Heartbeat, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read failed: %w", err)
	}

	var hb domain.Heartbeat
	if err := json.Unmarshal(data, &hb); err != nil {
		return nil, false, fmt.Errorf("unmarshal failed: %w", err)
	}

	if err := hb.Validate(); err != nil {
		return nil, false, fmt.Errorf("validation failed: %w", err)
	}

	return &hb, true, nil
}

// NewFilePublisher creates a new FilePublisher with the given path.
func NewFilePublisher(path string) *FilePublisher {
	return &FilePublisher{Path: path}
}