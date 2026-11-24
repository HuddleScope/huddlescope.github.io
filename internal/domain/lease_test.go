package domain

import (
	"testing"
	"time"
)

// TestLeaseStructBasics documents the Lease persistence-contract fields and the
// time semantics (all UTC; LeaseUntil computed server-side in every adapter).
func TestLeaseStructBasics(t *testing.T) {
	updated := time.Date(2026, 11, 24, 19, 0, 0, 0, time.UTC)
	until := updated.Add(20 * time.Minute)

	l := Lease{
		Key:          "rss-ingest",
		HolderID:     "github-backup:host-a:ab12cd34",
		FencingToken: 7,
		LeaseUntil:   until,
		UpdatedAt:    updated,
	}
	if l.Key != "rss-ingest" || l.HolderID == "" || l.FencingToken != 7 {
		t.Errorf("lease fields not retained: %+v", l)
	}
	if !l.LeaseUntil.After(l.UpdatedAt) {
		t.Error("LeaseUntil should be after UpdatedAt for a fresh grant")
	}
}
