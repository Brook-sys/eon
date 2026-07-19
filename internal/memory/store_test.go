package memory

import (
	"context"
	"testing"
	"time"
)

func TestMapMemoryStore(t *testing.T) {
	t.Parallel()
	now := time.Now()
	clock := func() time.Time { return now }
	store := NewMapMemoryStore(clock)

	ctx := context.Background()

	// Retrieve non-existent
	if _, err := store.RetrieveMemory(ctx, "k1"); err != ErrNotFound {
		t.Errorf("want ErrNotFound, got %v", err)
	}

	// Store and retrieve valid
	err := store.StoreMemory(ctx, "k1", "v1", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	val, err := store.RetrieveMemory(ctx, "k1")
	if err != nil || val != "v1" {
		t.Errorf("want v1, got %q (err: %v)", val, err)
	}

	// Store with expiration
	err = store.StoreMemory(ctx, "k2", "v2", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	val, err = store.RetrieveMemory(ctx, "k2")
	if err != nil || val != "v2" {
		t.Errorf("want v2, got %q (err: %v)", val, err)
	}

	// Advance time to expire k2
	now = now.Add(2 * time.Hour)
	if _, err := store.RetrieveMemory(ctx, "k2"); err != ErrNotFound {
		t.Errorf("want ErrNotFound for expired item, got %v", err)
	}

	// Compact
	err = store.CompactIrrelevant(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Verify it was actually deleted from underlying map by exploiting behavior
	// (Though lazy retrieval covers it, this asserts the map size is managed)
	ms := store.(*mapMemoryStore)
	ms.mu.RLock()
	_, k2Exists := ms.items["k2"]
	_, k1Exists := ms.items["k1"]
	ms.mu.RUnlock()
	
	if k2Exists {
		t.Errorf("k2 should be compacted")
	}
	if !k1Exists {
		t.Errorf("k1 should survive compaction")
	}
}
