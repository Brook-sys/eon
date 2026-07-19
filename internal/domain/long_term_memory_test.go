package domain

import (
	"testing"
	"time"
)

func TestLongTermMemory_Basic(t *testing.T) {
	now := time.Unix(1710000000, 0)
	mem := LongTermMemory{
		ID:         "mem-123",
		Key:        "test_key",
		Scope:      MemoryScopeMission,
		Value:      "Important context",
		StoredAt:   now,
		Expiration: now.Add(24 * time.Hour),
	}

	if mem.ID != "mem-123" {
		t.Errorf("Expected ID mem-123, got %s", mem.ID)
	}
	if mem.Scope != MemoryScopeMission {
		t.Errorf("Expected Scope %s, got %s", MemoryScopeMission, mem.Scope)
	}
}
