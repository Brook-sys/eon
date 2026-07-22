package kernel

import (
	"context"
	"errors"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

const (
	subagentDeadlineExceeded  = "deadline_exceeded"
	EventSubagentLeaseEvicted = "subagent.lease_evicted"
)

// Supervisor manages persistent subagent lifecycles across crashes, matching domain states
// to SessionManager instances.
type Supervisor struct {
	Store   port.Store
	Manager SessionManager
	Clock   interface{ Now() time.Time }
	IDs     interface{ NewID(string) (string, error) }
	// LeaseTTL arms each retried generation with a fresh liveness fence. Zero
	// preserves deadline-only behavior.
	LeaseTTL time.Duration
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
			status, statusErr := s.Manager.Status(ctx, SessionID(rec.ID))
			// Receiver execution commits its terminal receipt before publishing to
			// the process-local manager. Repair that split during normal operation,
			// not only at bootstrap, so a transient publication failure cannot hold a
			// concurrency slot until restart or let the local deadline defeat an
			// already durable terminal outcome. The lookup is generation-scoped.
			// Publish only when the manager still exposes that exact active generation:
			// a manager terminal is authoritative process-local evidence, and replacing
			// it from a conflicting durable receipt would turn deterministic storage
			// election into silent terminal corruption.
			terminal, terminalErr := tx.TerminalSubagentSpawnReceiptForReceiver(rec.ID, rec.Attempt)
			if terminalErr == nil {
				managerGenerationActive := statusErr == nil && status.Attempt == rec.Attempt && (status.State == SessionStatePending || status.State == SessionStateRunning)
				if managerGenerationActive {
					observation, err := TerminalSubagentObservation(terminal)
					if err != nil {
						return err
					}
					if err := s.Manager.PublishStatus(ctx, observation); err != nil {
						return err
					}
					status, statusErr = s.Manager.Status(ctx, SessionID(rec.ID))
				}
			} else if !errors.Is(terminalErr, port.ErrNotFound) {
				return terminalErr
			}
			// Retry is deliberately issued before its durable generation is
			// published. If that transaction rolls back, the transport can already
			// be anywhere in attempt+1 (not only PENDING). Treat the immediately
			// advanced generation as current so restart/reconcile cannot lose a fast
			// RUNNING/COMPLETE/FAILED observation or leak it at the deadline.
			recoveredRetry := statusErr == nil && rec.State == domain.SubagentStateRunning && status.Attempt == rec.Attempt+1 && rec.Attempt < rec.MaxAttempts-1
			statusForCurrentAttempt := statusErr == nil && (status.Attempt == rec.Attempt || recoveredRetry)
			terminalForCurrentAttempt := statusForCurrentAttempt && (status.State == SessionStateComplete || status.State == SessionStateFailed)
			deadlineFailureForCurrentAttempt := terminalForCurrentAttempt && status.State == SessionStateFailed && status.Error != nil && status.Error.Error() == subagentDeadlineExceeded
			// Positive terminal evidence for the active generation wins over the
			// local deadline. This matters when authenticated remote completion was
			// already durable before the deadline but only became process-visible in
			// the current control cycle.
			leaseExpired := !rec.LeaseExpiresAt.IsZero() && !now.Before(rec.LeaseExpiresAt)
			if (!terminalForCurrentAttempt || deadlineFailureForCurrentAttempt) && (!rec.Deadline.IsZero() && !now.Before(rec.Deadline) || leaseExpired) {
				// Fence a process-visible active generation before publishing the
				// durable timeout. Otherwise the record leaves the active scan while the
				// manager keeps consuming a concurrency slot forever. Publishing first
				// is crash-safe: if the store commit fails, the next cycle recognizes
				// this exact failure as a deadline terminal rather than retrying it.
				if statusForCurrentAttempt && (status.State == SessionStatePending || status.State == SessionStateRunning) {
					attempt := rec.Attempt
					if recoveredRetry {
						attempt = status.Attempt
					}
					failureCode := subagentDeadlineExceeded
					if leaseExpired && (rec.Deadline.IsZero() || now.Before(rec.Deadline)) {
						failureCode = "lease_expired"
					}
					if err := s.Manager.PublishStatus(ctx, SubagentObservation{ID: SessionID(rec.ID), Attempt: attempt, State: SessionStateFailed, Failure: failureCode}); err != nil {
						return err
					}
				}
				if recoveredRetry {
					rec.Attempt = status.Attempt
				}
				rec.State = domain.SubagentStateError
				rec.ErrorCode = subagentDeadlineExceeded
				if leaseExpired && (rec.Deadline.IsZero() || now.Before(rec.Deadline)) {
					rec.ErrorCode = "lease_expired"
				}
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

			if statusErr != nil {
				continue
			}
			if status.Attempt < rec.Attempt || status.Attempt > rec.Attempt+1 {
				continue
			}

			changed := false
			if recoveredRetry {
				rec.Attempt = status.Attempt
				s.armLease(&rec, now)
				changed = true
			}
			switch status.State {
			case SessionStatePending:
				// A previous retry may have re-armed the transport immediately before
				// the durable transaction rolled back. Complete that split observation
				// without issuing another transport retry.
				if recoveredRetry {
					rec.State = domain.SubagentStatePending
					rec.Result = ""
					rec.ErrorCode = ""
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
					s.armLease(&rec, s.Clock.Now())
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

func (s *Supervisor) armLease(rec *domain.SubagentRecord, now time.Time) {
	if s.LeaseTTL > 0 {
		rec.LeaseExpiresAt = now.Add(s.LeaseTTL)
		return
	}
	rec.LeaseExpiresAt = time.Time{}
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
	if rec.ErrorCode != "lease_expired" {
		return nil
	}
	auditID, err := s.IDs.NewID("event")
	if err != nil {
		return err
	}
	_, err = tx.AppendEvent(domain.Event{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            domain.EventID(auditID),
		Kind:          EventSubagentLeaseEvicted,
		OccurredAt:    now,
		PayloadRef:    "subagent=" + rec.ID + ";reason=lease_expired",
	})
	return err
}
