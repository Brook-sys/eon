package memory

import (
	"context"
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
	if err := store.SaveMemory(memory); err != nil {
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
