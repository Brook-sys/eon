package kernel

import (
	"context"
	"errors"
	"sync"
	"time"

)

// SessionID represents a unique identifier for a subagent session.
type SessionID string

// SessionState indicates the current status of a subagent.
type SessionState string

const (
	SessionStatePending  SessionState = "PENDING"
	SessionStateRunning  SessionState = "RUNNING"
	SessionStateComplete SessionState = "COMPLETE"
	SessionStateFailed   SessionState = "FAILED"
)

// SubagentSpec defines the parameters for spawning a new subagent.
type SubagentSpec struct {
	Task        string
	ContextMode string // e.g. "isolated" or "fork"
	Labels      map[string]string
}

// SubagentStatus tracks the lifecycle of an active subagent session.
type SubagentStatus struct {
	ID        SessionID
	State     SessionState
	Spec      SubagentSpec
	StartedAt time.Time
	Result    string
	Error     error
}

// SessionManager defines the contract for coordinating child subagents.
type SessionManager interface {
	// Spawn initiates a new subagent session and returns its ID.
	Spawn(ctx context.Context, spec SubagentSpec) (SessionID, error)
	// Status retrieves the current status of a spawned session.
	Status(ctx context.Context, id SessionID) (SubagentStatus, error)
	// Wait blocks until the session completes, fails, or the context is canceled.
	Wait(ctx context.Context, id SessionID) (SubagentStatus, error)
}

// ErrSessionNotFound is returned when querying an unknown session ID.
var ErrSessionNotFound = errors.New("session not found")

// localSessionManager is a naive in-memory implementation of SessionManager
// intended primarily for testing or isolated local boundaries.
type localSessionManager struct {
	mu       sync.RWMutex
	sessions map[SessionID]*SubagentStatus
	clock    interface{ Now() time.Time }
	nextID   int
}

// NewLocalSessionManager creates an in-memory session manager.
func NewLocalSessionManager(clock interface{ Now() time.Time }) SessionManager {
	return &localSessionManager{
		sessions: make(map[SessionID]*SubagentStatus),
		clock:    clock,
		nextID:   1,
	}
}

// Spawn implements SessionManager.Spawn.
func (m *localSessionManager) Spawn(ctx context.Context, spec SubagentSpec) (SessionID, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := SessionID(string(rune(m.nextID + '0'))) // simplistic ID generation
	m.nextID++

	status := &SubagentStatus{
		ID:        id,
		State:     SessionStatePending,
		Spec:      spec,
		StartedAt: m.clock.Now(),
	}
	m.sessions[id] = status
	return id, nil
}

// Status implements SessionManager.Status.
func (m *localSessionManager) Status(ctx context.Context, id SessionID) (SubagentStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status, ok := m.sessions[id]
	if !ok {
		return SubagentStatus{}, ErrSessionNotFound
	}
	return *status, nil
}

// Wait implements SessionManager.Wait (blocking is mocked/naive here).
func (m *localSessionManager) Wait(ctx context.Context, id SessionID) (SubagentStatus, error) {
	// For the naive local manager, we just check status immediately.
	// A real implementation would block on a signal channel.
	return m.Status(ctx, id)
}


