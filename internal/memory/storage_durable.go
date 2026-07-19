package memory

import (
	"context"
	"motor-autonomo/internal/port"
	"time"
)

// DurableMemoryStore wraps port.Store to provide SemanticMemory.
type DurableMemoryStore struct {
	store port.Store
	clock func() time.Time
}

func NewDurableMemoryStore(store port.Store, clock func() time.Time) SemanticMemory {
	return &DurableMemoryStore{
		store: store,
		clock: clock,
	}
}

func (d *DurableMemoryStore) StoreMemory(ctx context.Context, key, value string, expiration time.Time) error {
	// For now, this is a placeholder.
	// To actually store this we'd need domain objects for LongTermMemory
	// and to append them to the event log.
	return nil
}

func (d *DurableMemoryStore) RetrieveMemory(ctx context.Context, key string) (string, error) {
	return "", ErrNotFound
}

func (d *DurableMemoryStore) CompactIrrelevant(ctx context.Context) error {
	return nil
}
