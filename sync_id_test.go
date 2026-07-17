package main

import (
	"regexp"
	"testing"
)

func TestNewSyncIDReturnsUniqueVersion4UUIDs(t *testing.T) {
	const samples = 100
	uuidV4 := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	seen := make(map[string]struct{}, samples)

	for range samples {
		syncID, err := newSyncID()
		if err != nil {
			t.Fatalf("newSyncID() error = %v", err)
		}
		if !uuidV4.MatchString(syncID.String()) {
			t.Fatalf("newSyncID() = %q, want an RFC 4122 version 4 UUID", syncID)
		}
		if _, exists := seen[syncID.String()]; exists {
			t.Fatalf("newSyncID() generated duplicate %q", syncID)
		}
		seen[syncID.String()] = struct{}{}
	}
}
