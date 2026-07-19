package memory

import (
	"context"
	"sync"
	"time"
)

type mapMemoryStore struct {
	mu    sync.RWMutex
	items map[string]memoryItem
	clock func() time.Time
}

type memoryItem struct {
	value      string
	expiration time.Time
}

// NewMapMemoryStore returns an in-memory SemanticMemory implementation.
func NewMapMemoryStore(clock func() time.Time) SemanticMemory {
	return &mapMemoryStore{
		items: make(map[string]memoryItem),
		clock: clock,
	}
}

func (m *mapMemoryStore) StoreMemory(ctx context.Context, key, value string, expiration time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[key] = memoryItem{value: value, expiration: expiration}
	return nil
}

func (m *mapMemoryStore) RetrieveMemory(ctx context.Context, key string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, ok := m.items[key]
	if !ok {
		return "", ErrNotFound
	}
	if !item.expiration.IsZero() && m.clock().After(item.expiration) {
		return "", ErrNotFound // lazily expired
	}
	return item.value, nil
}

func (m *mapMemoryStore) CompactIrrelevant(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock()
	for k, item := range m.items {
		if !item.expiration.IsZero() && now.After(item.expiration) {
			delete(m.items, k)
		}
	}
	return nil
}
