package kernel

import (
	"context"
	"crypto/sha256"
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
	return m.spawnAndPersist(ctx, spec, "", domain.SubagentSpawnRequest{})
}

// AcceptRemoteSpawn atomically persists the receiver lifecycle record and its
// authenticated replay receipt. A committed request can therefore return the
// same acknowledgement after restart even when the original response was lost.
func (m *PersistentSessionManager) AcceptRemoteSpawn(ctx context.Context, callerPeerID string, request domain.SubagentSpawnRequest) (domain.SubagentSpawnAcknowledgement, error) {
	if !domain.ValidSubagentRPCField(callerPeerID) || domain.ValidateSubagentSpawnRequest(request) != nil {
		return domain.SubagentSpawnAcknowledgement{}, domain.ErrInvalidSubagentSpawnRPC
	}
	var existing domain.SubagentSpawnReceipt
	err := m.store.View(ctx, func(reader port.Reader) error {
		var readErr error
		existing, readErr = reader.SubagentSpawnReceipt(callerPeerID, request.RequestID)
		return readErr
	})
	if err == nil {
		if !existing.Matches(callerPeerID, request) {
			return domain.SubagentSpawnAcknowledgement{}, ErrSessionConflict
		}
		return existing.Acknowledgement(), nil
	}
	if !errors.Is(err, port.ErrNotFound) {
		return domain.SubagentSpawnAcknowledgement{}, err
	}
	spec := SubagentSpec{Task: request.Task, ContextMode: request.ContextMode, Labels: map[string]string{"task_id": remoteSpawnTaskID(callerPeerID, request.RequestID), "source_peer_id": callerPeerID, "source_session_id": request.SessionID}}
	id, err := m.spawnAndPersist(ctx, spec, callerPeerID, request)
	if err != nil {
		return domain.SubagentSpawnAcknowledgement{}, err
	}
	return domain.SubagentSpawnAcknowledgement{RequestID: request.RequestID, SessionID: request.SessionID, Attempt: request.Attempt, ReceiverSessionID: string(id), Accepted: true}, nil
}

func remoteSpawnTaskID(callerPeerID, requestID string) string {
	return fmt.Sprintf("subagent-spawn:%x", sha256.Sum256([]byte(callerPeerID+"\x00"+requestID)))
}

func (m *PersistentSessionManager) spawnAndPersist(ctx context.Context, spec SubagentSpec, callerPeerID string, request domain.SubagentSpawnRequest) (SessionID, error) {
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
				return "", m.rollbackSpawn(ctx, id, fmt.Errorf("allocate subagent dispatch id: %w", idErr))
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
		if callerPeerID != "" {
			receipt := domain.SubagentSpawnReceipt{SchemaVersion: domain.SchemaVersionV1, CallerPeerID: callerPeerID, RequestID: request.RequestID, SourceSessionID: request.SessionID, Attempt: request.Attempt, Task: request.Task, ContextMode: request.ContextMode, ReceiverSessionID: string(id), RecordedAt: now}
			if existing, readErr := tx.SubagentSpawnReceipt(callerPeerID, request.RequestID); readErr == nil {
				if !existing.Matches(callerPeerID, request) || existing.ReceiverSessionID != string(id) {
					return fmt.Errorf("%w: durable subagent spawn receipt differs", ErrSessionConflict)
				}
			} else if !errors.Is(readErr, port.ErrNotFound) {
				return readErr
			} else if createErr := tx.CreateSubagentSpawnReceipt(receipt); createErr != nil {
				return createErr
			}
		}
		return nil
	})
	if err != nil {
		return "", m.rollbackSpawn(ctx, id, fmt.Errorf("persist spawned subagent %s: %w", id, err))
	}
	return id, nil
}

func (m *PersistentSessionManager) rollbackSpawn(ctx context.Context, id SessionID, cause error) error {
	// Only compensate process-local admissions that never became durable. If a
	// record already exists under the same identity, rolling it back would
	// destroy a live session that merely failed an idempotent write/check.
	var durableExists bool
	_ = m.store.View(context.WithoutCancel(ctx), func(reader port.Reader) error {
		_, err := reader.SubagentRecord(string(id))
		durableExists = err == nil
		return nil
	})
	if durableExists {
		return cause
	}
	if rollbackErr := m.manager.RollbackSpawn(context.WithoutCancel(ctx), id); rollbackErr != nil {
		return errors.Join(cause, fmt.Errorf("rollback process-local subagent %s: %w", id, rollbackErr))
	}
	return cause
}

func (m *PersistentSessionManager) RollbackSpawn(ctx context.Context, id SessionID) error {
	return m.manager.RollbackSpawn(ctx, id)
}

func (m *PersistentSessionManager) Status(ctx context.Context, id SessionID) (SubagentStatus, error) {
	return m.manager.Status(ctx, id)
}

func (m *PersistentSessionManager) Restore(ctx context.Context, status SubagentStatus) error {
	return m.manager.Restore(ctx, status)
}

// AdmitRemoteStatus verifies the authenticated reporter against the durable
// transport binding and commits an immutable ingress receipt before ACK.
func (m *PersistentSessionManager) AdmitRemoteStatus(ctx context.Context, callerPeerID, deliveryID string, observation SubagentObservation) error {
	candidate := domain.SubagentStatusIngressReceipt{SchemaVersion: domain.SchemaVersionV1, CallerPeerID: callerPeerID, DeliveryID: deliveryID, SessionID: string(observation.ID), Attempt: observation.Attempt, State: string(observation.State), Result: observation.Result, Failure: observation.Failure, Status: domain.SubagentStatusIngressPending, RecordedAt: m.clock.Now().UTC()}
	if err := candidate.Validate(); err != nil {
		return err
	}
	return m.store.Update(ctx, func(tx port.Transaction) error {
		record, err := tx.SubagentRecord(string(observation.ID))
		if err != nil {
			return err
		}
		if record.TransportPeerID == "" || record.TransportPeerID != callerPeerID {
			return domain.ErrInvalidSubagentStatusIngress
		}
		existing, err := tx.SubagentStatusIngressReceipt(callerPeerID, deliveryID)
		if err == nil {
			if !existing.Matches(candidate) {
				return fmt.Errorf("%w: durable subagent status ingress differs", ErrSessionConflict)
			}
			return nil
		}
		if !errors.Is(err, port.ErrNotFound) {
			return err
		}
		return tx.CreateSubagentStatusIngressReceipt(candidate)
	})
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
