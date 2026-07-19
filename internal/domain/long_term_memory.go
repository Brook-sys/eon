package domain

import (
	"time"
)

// MemoryScope classifies the operational scope of a memory.
type MemoryScope string

const (
	MemoryScopeMission   MemoryScope = "mission"
	MemoryScopeStrategy  MemoryScope = "strategy"
	MemoryScopeAgent     MemoryScope = "agent"
)

// LongTermMemory represents a semantic memory element that can be retrieved and eventually expired.
type LongTermMemory struct {
	ID         ID
	Key        string
	Scope      MemoryScope
	Value      string
	StoredAt   time.Time
	Expiration time.Time
}
