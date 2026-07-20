package kernel

import (
	"context"
	"errors"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// Supervisor manages persistent subagent lifecycles across crashes, matching domain states
// to SessionManager instances.
type Supervisor struct {
	Store   port.Store
	Manager SessionManager
	Clock   interface{ Now() time.Time }
	IDs     interface{ NewID(string) (string, error) }
}

// Reconcile scans pending and running subagent records in storage,
// checks their actual status in the SessionManager, and updates
// their persisted state if they have completed or failed.
func (s *Supervisor) Reconcile(ctx context.Context) (int, error) {
	var reconciled int

	err := s.Store.Update(ctx, func(tx port.Transaction) error {
		pending, err := tx.SubagentRecordsByState(domain.SubagentStatePending, 0)
		if err != nil {
			return err
		}

		running, err := tx.SubagentRecordsByState(domain.SubagentStateRunning, 0)
		if err != nil {
			return err
		}

		var active []domain.SubagentRecord
		active = append(active, pending...)
		active = append(active, running...)

		for _, rec := range active {
			now := s.Clock.Now()
			if !rec.Deadline.IsZero() && !now.Before(rec.Deadline) {
				rec.State = domain.SubagentStateError
				rec.ErrorCode = "deadline_exceeded"
				rec.UpdatedAt = now
				if err := tx.SaveSubagentRecord(rec); err != nil {
					return err
				}
				if err := s.appendTerminalEvent(tx, rec, now); err != nil {
					return err
				}
				reconciled++
				continue
			}

			status, err := s.Manager.Status(ctx, SessionID(rec.ID))
			if err != nil {
				continue
			}
			if status.Attempt < rec.Attempt || status.Attempt > rec.Attempt+1 {
				continue
			}

			changed := false
			switch status.State {
			case SessionStatePending:
				// A previous retry may have re-armed the transport immediately before
				// the durable transaction rolled back. Complete that split observation
				// without issuing another transport retry.
				if rec.State == domain.SubagentStateRunning && status.Attempt == rec.Attempt+1 && rec.Attempt < rec.MaxAttempts-1 {
					rec.State = domain.SubagentStatePending
					rec.Attempt++
					rec.Result = ""
					rec.ErrorCode = ""
					changed = true
				}
			case SessionStateComplete:
				if status.Attempt != rec.Attempt {
					continue
				}
				rec.State = domain.SubagentStateComplete
				rec.Result = status.Result
				changed = true
			case SessionStateFailed:
				if status.Attempt != rec.Attempt {
					continue
				}
				if rec.Attempt < rec.MaxAttempts-1 {
					// Re-arm the transport before publishing PENDING durably. Retry is
					// idempotent so a store rollback can be reconciled safely next cycle.
					if err := s.Manager.Retry(ctx, SessionID(rec.ID)); err != nil {
						return err
					}
					rec.State = domain.SubagentStatePending
					rec.Attempt++
					// clear previous result/error
					rec.Result = ""
					rec.ErrorCode = ""
				} else {
					rec.State = domain.SubagentStateError
					if status.Error != nil {
						rec.ErrorCode = status.Error.Error()
					} else {
						rec.ErrorCode = "subagent_failed"
					}
				}
				changed = true
			case SessionStateRunning:
				if status.Attempt != rec.Attempt {
					continue
				}
				if rec.State != domain.SubagentStateRunning {
					rec.State = domain.SubagentStateRunning
					changed = true
				}
			}

			if changed {
				rec.UpdatedAt = s.Clock.Now()
				if err := tx.SaveSubagentRecord(rec); err != nil {
					return err
				}
				if rec.State == domain.SubagentStatePending {
					if err := s.createDispatchForGeneration(tx, rec, rec.UpdatedAt); err != nil {
						return err
					}
				}
				if (rec.State == domain.SubagentStateComplete || rec.State == domain.SubagentStateError) && s.IDs != nil {
					if err := s.appendTerminalEvent(tx, rec, rec.UpdatedAt); err != nil {
						return err
					}
				}
				reconciled++
			}
		}
		return nil
	})

	return reconciled, err
}

func (s *Supervisor) createDispatchForGeneration(tx port.Transaction, rec domain.SubagentRecord, now time.Time) error {
	if rec.TransportPeerID == "" || s.IDs == nil {
		return nil
	}
	if _, err := tx.SubagentDispatchByGeneration(rec.ID, rec.Attempt); err == nil {
		return nil
	} else if !errors.Is(err, port.ErrNotFound) {
		return err
	}
	requestID, err := s.IDs.NewID("subagent-dispatch")
	if err != nil {
		return err
	}
	return tx.CreateSubagentDispatch(domain.SubagentDispatch{SchemaVersion: domain.SchemaVersionV1, RequestID: domain.SubagentDispatchRequestID(requestID), SessionID: rec.ID, Attempt: rec.Attempt, PeerID: rec.TransportPeerID, Status: domain.SubagentDispatchPending, MaxSendAttempts: 3, AvailableAt: now, CreatedAt: now, UpdatedAt: now})
}

func (s *Supervisor) appendTerminalEvent(tx port.Transaction, rec domain.SubagentRecord, now time.Time) error {
	if s.IDs == nil {
		return nil
	}
	eventID, err := s.IDs.NewID("external-event")
	if err != nil {
		return err
	}
	result := rec.Result
	if rec.State == domain.SubagentStateError {
		result = rec.ErrorCode
	}
	event := domain.ExternalEvent{
		SchemaVersion:    domain.SchemaVersionV1,
		ID:               domain.ExternalEventID(eventID),
		DeduplicationKey: "subagent-terminal:" + rec.ID,
		Source:           "kernel.subagent.supervisor",
		SourceActorID:    "kernel",
		Kind:             domain.ExternalSubagentCompletion,
		MissionID:        domain.MissionID(rec.MissionID),
		CorrelationID:    rec.ID,
		Content: domain.ExternalContent{
			MediaType: "text/plain",
			Text:      string(rec.State) + ":" + result,
		},
		ReceivedAt: now,
	}
	disposition := domain.ExternalEventDisposition{
		SchemaVersion: domain.SchemaVersionV1,
		EventID:       event.ID,
		State:         domain.ExternalEventReceived,
		RecordedAt:    now,
	}
	if err := tx.CreateExternalEvent(event, disposition); err != nil && err != port.ErrConflict {
		return err
	}
	return nil
}
