package heartbeat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

func TestFilePublisher_Publish(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "heartbeat.json")
	p := NewFilePublisher(filePath)

	hb := &domain.Heartbeat{
		Version:        domain.HeartbeatVersion,
		Status:         domain.HeartbeatStatusOK,
		RunID:          "test-run-123",
		AttemptedAt:    time.Now().UTC().Format(time.RFC3339),
		SourcesChecked: 1,
		SourcesFailed:  0,
	}

	if err := p.Publish(nil, hb); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	// Verify file exists and content matches
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var parsed domain.Heartbeat
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if parsed.RunID != hb.RunID {
		t.Errorf("RunID = %v, want %v", parsed.RunID, hb.RunID)
	}
	if parsed.Status != hb.Status {
		t.Errorf("Status = %v, want %v", parsed.Status, hb.Status)
	}
}

func TestFilePublisher_Publish_Validation(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "heartbeat.json")
	p := NewFilePublisher(filePath)

	tests := []struct {
		name    string
		hb      *domain.Heartbeat
		wantErr bool
	}{
		{
			name: "valid heartbeat",
			hb: &domain.Heartbeat{
				Version:        domain.HeartbeatVersion,
				Status:         domain.HeartbeatStatusOK,
				RunID:          "test",
				AttemptedAt:    time.Now().UTC().Format(time.RFC3339),
				SourcesChecked: 0,
				SourcesFailed:  0,
			},
			wantErr: false,
		},
		{
			name:    "nil heartbeat",
			hb:      nil,
			wantErr: true,
		},
		{
			name: "invalid heartbeat (missing version)",
			hb: &domain.Heartbeat{
				Status:      domain.HeartbeatStatusOK,
				RunID:       "test",
				AttemptedAt: time.Now().UTC().Format(time.RFC3339),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.Publish(nil, tt.hb)
			if (err != nil) != tt.wantErr {
				t.Errorf("Publish() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "heartbeat.json")

	// Test: file not found
	hb, found, err := Load(filePath)
	if err != nil {
		t.Fatalf("Load() on missing file error = %v", err)
	}
	if found {
		t.Error("Load() found = true, want false on missing file")
	}
	if hb != nil {
		t.Error("Load() hb != nil on missing file")
	}

	// Test: valid file
	hb = &domain.Heartbeat{
		Version:        domain.HeartbeatVersion,
		Status:         domain.HeartbeatStatusOK,
		RunID:          "test-run",
		AttemptedAt:    time.Now().UTC().Format(time.RFC3339),
		SourcesChecked: 1,
		SourcesFailed:  0,
	}

	data, _ := json.Marshal(hb)
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	hb2, found, err := Load(filePath)
	if err != nil {
		t.Fatalf("Load() on existing file error = %v", err)
	}
	if !found {
		t.Error("Load() found = false, want true")
	}
	if hb2.RunID != hb.RunID {
		t.Errorf("Loaded RunID = %v, want %v", hb2.RunID, hb.RunID)
	}
}

func TestLoad_Errors(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "heartbeat.json")

	// Test: corrupt JSON
	if err := os.WriteFile(filePath, []byte("not json"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, _, err := Load(filePath)
	if err == nil {
		t.Error("Load() err = nil, want error on corrupt JSON")
	}

	// Test: validation failure (invalid version)
	hb := &domain.Heartbeat{
		Version:     "999",
		Status:      domain.HeartbeatStatusOK,
		RunID:       "test",
		AttemptedAt: time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(hb)
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, _, err = Load(filePath)
	if err == nil {
		t.Error("Load() err = nil, want validation error")
	}
}

func TestNewFilePublisher(t *testing.T) {
	path := "/tmp/test-heartbeat.json"
	p := NewFilePublisher(path)
	if p.Path != path {
		t.Errorf("Path = %v, want %v", p.Path, path)
	}
}
