package kernel

import (
	"context"
	"errors"
	"fmt"
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
	ContextMode string // "isolated" or "fork"
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

// SessionPolicy bounds delegation performed by a SessionManager.
type SessionPolicy struct {
	MaxConcurrent int
}

func (p SessionPolicy) validate() error {
	if p.MaxConcurrent <= 0 {
		return errors.New("max concurrent subagents must be positive")
	}
	return nil
}

// SessionManager defines the contract for coordinating child subagents.
type SessionManager interface {
	Spawn(ctx context.Context, spec SubagentSpec) (SessionID, error)
	Status(ctx context.Context, id SessionID) (SubagentStatus, error)
	Wait(ctx context.Context, id SessionID) (SubagentStatus, error)
}

var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionLimit    = errors.New("subagent concurrency limit reached")
)

// localSessionManager is a deterministic in-memory implementation intended for
// tests and isolated local boundaries. Production transports can implement the
// same interface while preserving the policy at their own authority boundary.
type localSessionManager struct {
	mu       sync.RWMutex
	sessions map[SessionID]*SubagentStatus
	byTaskID map[string]SessionID
	clock    interface{ Now() time.Time }
	policy   SessionPolicy
	nextID   uint64
}

// NewLocalSessionManager creates a manager with a conservative default limit.
func NewLocalSessionManager(clock interface{ Now() time.Time }) SessionManager {
	manager, err := NewLocalSessionManagerWithPolicy(clock, SessionPolicy{MaxConcurrent: 4})
	if err != nil {
		panic(err)
	}
	return manager
}

// NewLocalSessionManagerWithPolicy creates a bounded in-memory session manager.
func NewLocalSessionManagerWithPolicy(clock interface{ Now() time.Time }, policy SessionPolicy) (SessionManager, error) {
	if clock == nil {
		return nil, errors.New("session clock is required")
	}
	if err := policy.validate(); err != nil {
		return nil, err
	}
	return &localSessionManager{
		sessions: make(map[SessionID]*SubagentStatus),
		byTaskID: make(map[string]SessionID),
		clock:    clock,
		policy:   policy,
		nextID:   1,
	}, nil
}

func (m *localSessionManager) Spawn(ctx context.Context, spec SubagentSpec) (SessionID, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if spec.Task == "" {
		return "", errors.New("subagent task is required")
	}
	if spec.ContextMode != "isolated" && spec.ContextMode != "fork" {
		return "", errors.New("subagent context mode must be isolated or fork")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// task_id is an idempotency key supplied by the continuity frontier. A retry
	// after spawn-but-before-checkpoint returns the original session.
	if taskID := spec.Labels["task_id"]; taskID != "" {
		if id, ok := m.byTaskID[taskID]; ok {
			return id, nil
		}
	}
	active := 0
	for _, status := range m.sessions {
		if status.State == SessionStatePending || status.State == SessionStateRunning {
			active++
		}
	}
	if active >= m.policy.MaxConcurrent {
		return "", ErrSessionLimit
	}

	id := SessionID(fmt.Sprintf("subagent-%d", m.nextID))
	m.nextID++
	status := &SubagentStatus{ID: id, State: SessionStatePending, Spec: cloneSubagentSpec(spec), StartedAt: m.clock.Now()}
	m.sessions[id] = status
	if taskID := spec.Labels["task_id"]; taskID != "" {
		m.byTaskID[taskID] = id
	}
	return id, nil
}

func (m *localSessionManager) Status(ctx context.Context, id SessionID) (SubagentStatus, error) {
	if err := ctx.Err(); err != nil {
		return SubagentStatus{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	status, ok := m.sessions[id]
	if !ok {
		return SubagentStatus{}, ErrSessionNotFound
	}
	copy := *status
	copy.Spec = cloneSubagentSpec(status.Spec)
	return copy, nil
}

func (m *localSessionManager) Wait(ctx context.Context, id SessionID) (SubagentStatus, error) {
	return m.Status(ctx, id)
}

func cloneSubagentSpec(spec SubagentSpec) SubagentSpec {
	copy := spec
	if spec.Labels != nil {
		copy.Labels = make(map[string]string, len(spec.Labels))
		for key, value := range spec.Labels {
			copy.Labels[key] = value
		}
	}
	return copy
}
