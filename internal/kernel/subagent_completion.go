package kernel

import (
	"context"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// SubagentCompletionProcessor is responsible for discovering child subagent
// sessions that have reached a terminal state (COMPLETE or FAILED) and
// ingesting those results as ExternalEvents into the parent mission log.
type SubagentCompletionProcessor struct {
	Manager SessionManager
	Store   port.Store
	Clock   interface{ Now() time.Time }
	IDs     interface{ NewID(string) (string, error) }
}

// ProcessCompletedSessions checks pending subagent work.
func (p SubagentCompletionProcessor) ProcessCompletedSessions(ctx context.Context, revID domain.MissionRevisionID) (int, error) {
	var resumedCount int
	var checkErr error

	p.Store.Update(ctx, func(tx port.Transaction) error {
		ops, err := tx.Operations(revID)
		if err != nil {
			checkErr = err
			return err
		}

		for _, op := range ops {
			if op.State != domain.StateWaitingEvent || op.Reevaluation.EventType != "subagent.completion" {
				continue
			}
			// Operation was paused for child completion. Retrieve session ID which was set as reference.
			sessionID := SessionID(op.Reevaluation.Reference)
			status, err := p.Manager.Status(ctx, sessionID)
			if err != nil {
				continue // Ignore missing or temporary errors in manager during poll.
			}

			if status.State == SessionStateComplete || status.State == SessionStateFailed {
				eventID, err := p.IDs.NewID("event")
				if err != nil { return err }
				ev := domain.ExternalEvent{
					SchemaVersion:    1,
					ID:               domain.ExternalEventID(eventID),
					DeduplicationKey: "subagent_completion:" + string(sessionID),
					Source:           "kernel.subagent",
					SourceActorID:    "kernel",
					Kind:             domain.ExternalSubagentCompletion,
					MissionID:        domain.MissionID(op.MissionRevision),
					CorrelationID:    string(sessionID),
					ReceivedAt:       p.Clock.Now(),
				}
				disp := domain.ExternalEventDisposition{
					EventID: ev.ID,
					State: domain.ExternalEventReceived,
				}
				// If already exists due to deduplication, CreateExternalEvent may err or just skip, handle gracefully
				err = tx.CreateExternalEvent(ev, disp)
				if err != nil && err != port.ErrConflict {
					return err
				}
				if err == nil {
					resumedCount++
				}
			}
		}
		return nil
	})

	return resumedCount, checkErr
}
