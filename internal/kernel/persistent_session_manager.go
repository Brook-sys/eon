package kernel

import (
	"context"
	"errors"
	"fmt"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// PersistentSessionPolicy controls the durable lifecycle envelope created for
// each successfully spawned process-local session.
type PersistentSessionPolicy struct {
	MissionID           domain.MissionID
	MaxAttempts         int
	Timeout             time.Duration
	DispatchMaxAttempts uint32
}

// PersistentSessionManager decorates a transport SessionManager and records
// every admitted child in canonical storage before returning it to the caller.
// Status and Wait remain transport-owned, while Supervisor reconciles their
// observations back into the durable record.
type PersistentSessionManager struct {
	manager SessionManager
	store   port.Store
	clock   interface{ Now() time.Time }
	ids     interface{ NewID(string) (string, error) }
	policy  PersistentSessionPolicy
}

func NewPersistentSessionManager(manager SessionManager, store port.Store, clock interface{ Now() time.Time }, ids interface{ NewID(string) (string, error) }, policy PersistentSessionPolicy) (*PersistentSessionManager, error) {
	if manager == nil || store == nil || clock == nil || ids == nil {
		return nil, errors.New("persistent session manager dependencies are incomplete")
	}
	if policy.MissionID == "" {
		return nil, errors.New("persistent session manager requires mission id")
	}
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = 1
	}
	if policy.Timeout <= 0 {
		policy.Timeout = 15 * time.Minute
	}
	if policy.DispatchMaxAttempts == 0 {
		policy.DispatchMaxAttempts = 3
	}
	return &PersistentSessionManager{manager: manager, store: store, clock: clock, ids: ids, policy: policy}, nil
}

func (m *PersistentSessionManager) Spawn(ctx context.Context, spec SubagentSpec) (SessionID, error) {
	id, err := m.manager.Spawn(ctx, spec)
	if err != nil {
		return "", err
	}
	now := m.clock.Now().UTC()
	taskID := spec.Labels["task_id"]
	if taskID == "" {
		taskID = string(id)
	}
	record := domain.SubagentRecord{
		SchemaVersion:   domain.SchemaVersionV1,
		ID:              string(id),
		TaskID:          taskID,
		MissionID:       string(m.policy.MissionID),
		State:           domain.SubagentStatePending,
		StartedAt:       now,
		UpdatedAt:       now,
		Task:            spec.Task,
		ContextMode:     spec.ContextMode,
		TransportPeerID: spec.Labels[SubagentTransportPeerLabel],
		MaxAttempts:     m.policy.MaxAttempts,
		Deadline:        now.Add(m.policy.Timeout),
	}
	var dispatch domain.SubagentDispatch
	if record.TransportPeerID != "" {
		_ = m.store.View(ctx, func(reader port.Reader) error {
			existing, readErr := reader.SubagentDispatchByGeneration(record.ID, record.Attempt)
			if readErr == nil {
				dispatch = existing
			}
			return nil
		})
		if dispatch.RequestID == "" {
			requestID, idErr := m.ids.NewID("subagent-dispatch")
			if idErr != nil {
				return "", fmt.Errorf("allocate subagent dispatch id: %w", idErr)
			}
			dispatch = domain.SubagentDispatch{SchemaVersion: domain.SchemaVersionV1, RequestID: domain.SubagentDispatchRequestID(requestID), SessionID: record.ID, Attempt: record.Attempt, PeerID: record.TransportPeerID, Status: domain.SubagentDispatchPending, MaxSendAttempts: m.policy.DispatchMaxAttempts, AvailableAt: now, CreatedAt: now, UpdatedAt: now}
		}
	}
	err = m.store.Update(ctx, func(tx port.Transaction) error {
		if createErr := tx.CreateSubagentRecord(record); createErr != nil {
			if !errors.Is(createErr, port.ErrConflict) {
				return createErr
			}
			existing, readErr := tx.SubagentRecord(record.ID)
			if readErr != nil {
				return readErr
			}
			if existing.TaskID != record.TaskID || existing.MissionID != record.MissionID || existing.Task != record.Task || existing.ContextMode != record.ContextMode || existing.TransportPeerID != record.TransportPeerID {
				return fmt.Errorf("%w: durable subagent record differs from idempotent spawn", ErrSessionConflict)
			}
		}
		if dispatch.RequestID != "" {
			if existing, readErr := tx.SubagentDispatchByGeneration(record.ID, record.Attempt); readErr == nil {
				if existing.RequestID != dispatch.RequestID || existing.PeerID != dispatch.PeerID {
					return fmt.Errorf("%w: durable subagent dispatch differs from idempotent spawn", ErrSessionConflict)
				}
			} else if !errors.Is(readErr, port.ErrNotFound) {
				return readErr
			} else if createErr := tx.CreateSubagentDispatch(dispatch); createErr != nil {
				if !errors.Is(createErr, port.ErrConflict) {
					return createErr
				}
				existing, readErr := tx.SubagentDispatchByGeneration(record.ID, record.Attempt)
				if readErr != nil || existing.PeerID != dispatch.PeerID {
					return fmt.Errorf("%w: durable subagent dispatch differs from idempotent spawn", ErrSessionConflict)
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("persist spawned subagent %s: %w", id, err)
	}
	return id, nil
}

func (m *PersistentSessionManager) Status(ctx context.Context, id SessionID) (SubagentStatus, error) {
	return m.manager.Status(ctx, id)
}

func (m *PersistentSessionManager) Restore(ctx context.Context, status SubagentStatus) error {
	return m.manager.Restore(ctx, status)
}

func (m *PersistentSessionManager) PublishStatus(ctx context.Context, observation SubagentObservation) error {
	return m.manager.PublishStatus(ctx, observation)
}

func (m *PersistentSessionManager) Retry(ctx context.Context, id SessionID) error {
	return m.manager.Retry(ctx, id)
}

func (m *PersistentSessionManager) Wait(ctx context.Context, id SessionID) (SubagentStatus, error) {
	return m.manager.Wait(ctx, id)
}
