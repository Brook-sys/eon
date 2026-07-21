package kernel

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sync"
	"time"
)

// SessionID represents a unique identifier for a subagent session.
type SessionID string

// SubagentTransportPeerLabel is trusted routing metadata injected by runtime
// configuration. It is deliberately absent from model-controlled tool input.
const SubagentTransportPeerLabel = "transport_peer_id"

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
	Attempt   int
	State     SessionState
	Spec      SubagentSpec
	StartedAt time.Time
	Result    string
	Error     error
}

// SubagentObservation correlates an untrusted transport report with one exact
// execution generation. Delayed observations from older attempts fail closed.
type SubagentObservation struct {
	ID      SessionID
	Attempt int
	State   SessionState
	Result  string
	Failure string
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
	// RollbackSpawn removes a newly admitted session when its durable envelope
	// could not be committed. Implementations must fail closed once execution or
	// a terminal observation has begun.
	RollbackSpawn(ctx context.Context, id SessionID) error
	Restore(ctx context.Context, status SubagentStatus) error
	PublishStatus(ctx context.Context, observation SubagentObservation) error
	Retry(ctx context.Context, id SessionID) error
	Status(ctx context.Context, id SessionID) (SubagentStatus, error)
	Wait(ctx context.Context, id SessionID) (SubagentStatus, error)
}

// RollbackSpawn compensates only a process-local admission that is still
// pending. It prevents a failed durable write from leaving an untracked child
// consuming concurrency or becoming executable later.
func (m *localSessionManager) RollbackSpawn(ctx context.Context, id SessionID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	status, ok := m.sessions[id]
	if !ok {
		return ErrSessionNotFound
	}
	if status.State != SessionStatePending {
		return ErrSessionTerminal
	}
	delete(m.sessions, id)
	if taskID := status.Spec.Labels["task_id"]; taskID != "" && m.byTaskID[taskID] == id {
		delete(m.byTaskID, taskID)
	}
	return nil
}

var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionLimit    = errors.New("subagent concurrency limit reached")
	ErrSessionConflict = errors.New("subagent idempotency key conflicts with existing specification")
	ErrSessionTerminal = errors.New("terminal subagent status cannot be changed")
	ErrSessionAttempt  = errors.New("subagent observation attempt does not match active attempt")
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
			existing := m.sessions[id]
			if existing == nil || !subagentSpecsEqual(existing.Spec, spec) {
				return "", ErrSessionConflict
			}
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
	status := &SubagentStatus{ID: id, Attempt: 0, State: SessionStatePending, Spec: cloneSubagentSpec(spec), StartedAt: m.clock.Now()}
	m.sessions[id] = status
	if taskID := spec.Labels["task_id"]; taskID != "" {
		m.byTaskID[taskID] = id
	}
	return id, nil
}

// Restore rehydrates a non-terminal transport observation after process
// restart. It never invents a completed result and is idempotent only when the
// restored identity and specification are identical.
func (m *localSessionManager) Restore(ctx context.Context, status SubagentStatus) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if status.ID == "" || status.Spec.Task == "" {
		return errors.New("restored subagent identity and task are required")
	}
	if status.Spec.ContextMode != "isolated" && status.Spec.ContextMode != "fork" {
		return errors.New("restored subagent context mode must be isolated or fork")
	}
	if status.State != SessionStatePending && status.State != SessionStateRunning {
		return errors.New("only active subagent sessions can be restored")
	}
	if status.StartedAt.IsZero() {
		return errors.New("restored subagent start time is required")
	}
	if status.Attempt < 0 {
		return errors.New("restored subagent attempt cannot be negative")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.sessions[status.ID]; ok {
		if existing.State != status.State || existing.Attempt != status.Attempt || !existing.StartedAt.Equal(status.StartedAt) || !subagentSpecsEqual(existing.Spec, status.Spec) {
			return ErrSessionConflict
		}
		return nil
	}
	active := 0
	for _, existing := range m.sessions {
		if existing.State == SessionStatePending || existing.State == SessionStateRunning {
			active++
		}
	}
	if active >= m.policy.MaxConcurrent {
		return ErrSessionLimit
	}
	copy := status
	copy.Spec = cloneSubagentSpec(status.Spec)
	copy.Result = ""
	copy.Error = nil
	m.sessions[status.ID] = &copy
	if taskID := status.Spec.Labels["task_id"]; taskID != "" {
		if prior, exists := m.byTaskID[taskID]; exists && prior != status.ID {
			delete(m.sessions, status.ID)
			return ErrSessionConflict
		}
		m.byTaskID[taskID] = status.ID
	}
	return nil
}

// PublishStatus is the narrow ingress used by a real child-session transport
// to publish lifecycle observations. Terminal observations are monotonic and
// replay-safe; divergent terminal replays fail closed.
func (m *localSessionManager) PublishStatus(ctx context.Context, observation SubagentObservation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	id, state, result, failure := observation.ID, observation.State, observation.Result, observation.Failure
	if state != SessionStateRunning && state != SessionStateComplete && state != SessionStateFailed {
		return errors.New("published subagent state must be RUNNING, COMPLETE, or FAILED")
	}
	if state == SessionStateComplete && failure != "" {
		return errors.New("completed subagent cannot publish failure")
	}
	if state == SessionStateFailed && result != "" {
		return errors.New("failed subagent cannot publish result")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	status, ok := m.sessions[id]
	if !ok {
		return ErrSessionNotFound
	}
	if observation.Attempt != status.Attempt {
		return ErrSessionAttempt
	}
	if status.State == SessionStateComplete || status.State == SessionStateFailed {
		existingFailure := ""
		if status.Error != nil {
			existingFailure = status.Error.Error()
		}
		if status.State == state && status.Result == result && existingFailure == failure {
			return nil
		}
		return ErrSessionTerminal
	}
	status.State = state
	status.Result = result
	status.Error = nil
	if failure != "" {
		status.Error = errors.New(failure)
	}
	return nil
}

// Retry re-arms a failed transport session under the same durable identity.
// Replays after the session has already returned to PENDING are harmless;
// successful sessions remain terminal and cannot be re-executed.
func (m *localSessionManager) Retry(ctx context.Context, id SessionID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	status, ok := m.sessions[id]
	if !ok {
		return ErrSessionNotFound
	}
	switch status.State {
	case SessionStatePending:
		return nil
	case SessionStateFailed:
		status.State = SessionStatePending
		status.Attempt++
		status.Result = ""
		status.Error = nil
		return nil
	case SessionStateComplete:
		return ErrSessionTerminal
	default:
		return errors.New("running subagent session cannot be retried")
	}
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

func subagentSpecsEqual(a, b SubagentSpec) bool {
	return a.Task == b.Task && a.ContextMode == b.ContextMode && maps.Equal(a.Labels, b.Labels)
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
