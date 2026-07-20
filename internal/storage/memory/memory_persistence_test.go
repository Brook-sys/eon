package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

func TestMemorySurvivesUnrelatedUpdateAndCheckpoint(t *testing.T) {
	store := New()
	memory := domain.LongTermMemory{
		ID:       "memory-1",
		Key:      "durable-key",
		Scope:    domain.MemoryScopeAgent,
		Value:    "durable-value",
		StoredAt: time.Date(2026, 7, 19, 23, 0, 0, 0, time.UTC),
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error { return tx.SaveMemory(memory) }); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got, err := store.LongTermMemory(memory.Key); err != nil || got != memory {
		t.Fatalf("memory after update = %+v, err = %v", got, err)
	}

	checkpoint, err := store.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := NewFromBinary(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := reopened.LongTermMemory(memory.Key); err != nil || got != memory {
		t.Fatalf("memory after reopen = %+v, err = %v", got, err)
	}
}

func TestMemoryIdentityValidationAndDeterministicListing(t *testing.T) {
	store := New()
	now := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	for _, mem := range []domain.LongTermMemory{
		{ID: "memory-b", Key: "b", Scope: domain.MemoryScopeAgent, Value: "two", StoredAt: now},
		{ID: "memory-a", Key: "a", Scope: domain.MemoryScopeAgent, Value: "one", StoredAt: now},
	} {
		if err := store.Update(context.Background(), func(tx port.Transaction) error { return tx.SaveMemory(mem) }); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		return tx.SaveMemory(domain.LongTermMemory{ID: "memory-c", Key: "a", Scope: domain.MemoryScopeAgent, Value: "collision", StoredAt: now})
	}); err == nil {
		t.Fatal("expected key collision rejection")
	}
	listed, err := store.ListMemoriesByScope(domain.MemoryScopeAgent)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].Key != "a" || listed[1].Key != "b" {
		t.Fatalf("memory list is not deterministic: %+v", listed)
	}
	if _, err := store.LongTermMemory("missing"); !errors.Is(err, port.ErrNotFound) {
		t.Fatalf("missing memory error = %v, want port.ErrNotFound", err)
	}
}
