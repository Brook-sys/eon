package kernel

import (
	"context"
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

			changed := false
			switch status.State {
			case SessionStateComplete:
				rec.State = domain.SubagentStateComplete
				rec.Result = status.Result
				changed = true
			case SessionStateFailed:
				if rec.Attempt < rec.MaxAttempts-1 {
					rec.State = domain.SubagentStatePending
					rec.Attempt++
					// clear previous result/error
					rec.Result = ""
					rec.ErrorCode = ""
				} else {
					rec.State = domain.SubagentStateError
					if status.Error != nil {
						rec.ErrorCode = status.Error.Error()
					}
				}
				changed = true
			case SessionStateRunning:
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
