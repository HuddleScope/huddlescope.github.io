package sqlite

import (
	"testing"
)

func TestStoragePolicy(t *testing.T) {
	policy := NewStoragePolicy()
	if policy.RetentionDays != RetentionWindow {
		t.Errorf("expected retention %d, got %d", RetentionWindow, policy.RetentionDays)
	}
	if policy.BatchLimit != ArchiveBatchLimit {
		t.Errorf("expected batch limit %d, got %d", ArchiveBatchLimit, policy.BatchLimit)
	}
	if !policy.SizeBudgetOK(1_000_000_000) {
		t.Error("SizeBudgetOK should return true for exactly 1GB")
	}
	if policy.SizeBudgetOK(1_000_000_001) {
		t.Error("SizeBudgetOK should return false for >1GB")
	}
}

func TestStoragePolicyBudgetOK(t *testing.T) {
	policy := NewStoragePolicy()
	if !policy.SizeBudgetOK(policy.FreeBytes) {
		t.Error("SizeBudgetOK should return true for FreeBytes")
	}
	if policy.SizeBudgetOK(policy.FreeBytes + 1) {
		t.Error("SizeBudgetOK should return false for FreeBytes+1")
	}
}

func TestArchiveOld(t *testing.T) {
	_, err := ArchiveOld("fake.db", RetentionWindow)
	if err == nil {
		t.Error("expected error from unimplemented ArchiveOld")
	}
}

func TestStorageEstimate(t *testing.T) {
	_, err := StorageEstimate("fake.db")
	if err == nil {
		t.Error("expected error from unimplemented StorageEstimate")
	}
}

func TestApplyRetention(t *testing.T) {
	_, err := ApplyRetention("fake.db")
	if err == nil {
		t.Error("expected error from unimplemented ApplyRetention")
	}
}
