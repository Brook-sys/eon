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
				rec.State = domain.SubagentStateError
				if status.Error != nil {
					rec.ErrorCode = status.Error.Error()
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
				reconciled++
			}
		}
		return nil
	})

	return reconciled, err
}
