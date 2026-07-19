package port

import (
	"motor-autonomo/internal/domain"
	"time"
)

// MemoryReader exposes access to stored long-term memory
type MemoryReader interface {
	LongTermMemory(key string) (domain.LongTermMemory, error)
	ListMemoriesByScope(scope domain.MemoryScope) ([]domain.LongTermMemory, error)
	ListExpiredMemories(now time.Time) ([]domain.LongTermMemory, error)
}

// MemoryWriter exposes mutations for semantic memory
type MemoryWriter interface {
	SaveMemory(domain.LongTermMemory) error
	DeleteMemory(domain.MemoryID) error
}
