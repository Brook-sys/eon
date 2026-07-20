package domain

import (
	"errors"
	"strings"
	"time"
)

// MemoryScope classifies the operational scope of a memory.
type MemoryScope string

const (
	MemoryScopeMission  MemoryScope = "mission"
	MemoryScopeStrategy MemoryScope = "strategy"
	MemoryScopeAgent    MemoryScope = "agent"
)

// LongTermMemory represents a semantic memory element that can be retrieved and eventually expired.
type LongTermMemory struct {
	ID         MemoryID
	Key        string
	Scope      MemoryScope
	Value      string
	StoredAt   time.Time
	Expiration time.Time
}

// Validate rejects ambiguous or unbounded semantic-memory records before they
// enter the canonical current view. The value is intentionally kept out of
// audit events, but is still bounded because it is persisted in checkpoints.
func (m LongTermMemory) Validate() error {
	if m.ID == "" || strings.TrimSpace(m.Key) == "" || strings.TrimSpace(m.Value) == "" || m.StoredAt.IsZero() {
		return errors.New("long-term memory requires id, key, value, and stored_at")
	}
	if !validMemoryScope(m.Scope) {
		return errors.New("long-term memory has invalid scope")
	}
	if len(m.Key) > 256 || len(m.Value) > 64*1024 {
		return errors.New("long-term memory exceeds size limit")
	}
	if !m.Expiration.IsZero() && !m.Expiration.After(m.StoredAt) {
		return errors.New("long-term memory expiration must be after stored_at")
	}
	return nil
}
