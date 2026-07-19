package memory

import (
	"context"
	"errors"
	"time"
)

// SemanticMemory defines the abstraction for long-term memory retrieval and compaction.
type SemanticMemory interface {
	StoreMemory(ctx context.Context, key, value string, expiration time.Time) error
	RetrieveMemory(ctx context.Context, key string) (string, error)
	CompactIrrelevant(ctx context.Context) error
}

var ErrNotFound = errors.New("memory: item not found")

