package kernel

import (
	"context"
	"errors"
	"fmt"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// RemoteSubagentExecutor is the authority-free receiver execution boundary.
// Implementations return evidence text only; they cannot mutate canonical state.
type RemoteSubagentExecutor interface {
	ExecuteRemoteSubagent(context.Context, string, string) (string, error)
}

// RemoteSubagentExecutorFunc adapts a bounded function for tests and simple providers.
type RemoteSubagentExecutorFunc func(context.Context, string, string) (string, error)

func (f RemoteSubagentExecutorFunc) ExecuteRemoteSubagent(ctx context.Context, task, contextMode string) (string, error) {
	return f(ctx, task, contextMode)
}

// TerminalSubagentObservation converts a durable receiver execution outcome
// into the process-local projection used by SessionManager. Keeping this
// conversion in the kernel makes startup restore and live reconciliation obey
// the same generation/result semantics.
func TerminalSubagentObservation(receipt domain.SubagentSpawnReceipt) (SubagentObservation, error) {
	if err := receipt.Validate(); err != nil {
		return SubagentObservation{}, err
	}
	observation := SubagentObservation{
		ID:      SessionID(receipt.ReceiverSessionID),
		Attempt: receipt.ReceiverAttempt,
		State:   SessionStateComplete,
		Result:  receipt.Result,
	}
	switch receipt.Status {
	case domain.SubagentSpawnReceiptComplete:
		return observation, nil
	case domain.SubagentSpawnReceiptFailed:
		observation.State, observation.Result, observation.Failure = SessionStateFailed, "", receipt.Failure
		return observation, nil
	default:
		return SubagentObservation{}, domain.ErrInvalidSubagentSpawnRPC
	}
}

// RemoteSubagentWorker claims durable inbound spawn receipts and executes each
// admitted generation at most once concurrently. Terminal results are committed
// before any outbound status delivery so restart never re-executes them.
type RemoteSubagentWorker struct {
	Store    port.Store
	Manager  SessionManager
	Executor RemoteSubagentExecutor
	Clock    interface{ Now() time.Time }
	Owner    string
	Batch    int
	Lease    time.Duration
	Timeout  time.Duration
}

func (w *RemoteSubagentWorker) ExecuteDue(ctx context.Context) (int, error) {
	if w == nil || w.Store == nil || w.Manager == nil || w.Executor == nil || w.Clock == nil || w.Owner == "" {
		return 0, errors.New("remote subagent worker dependencies are incomplete")
	}
	batch := w.Batch
	if batch <= 0 {
		batch = 2
	}
	lease := w.Lease
	if lease <= 0 {
		lease = 2 * time.Minute
	}
	timeout := w.Timeout
	if timeout <= 0 || timeout >= lease {
		timeout = lease - time.Second
		if timeout <= 0 {
			timeout = lease / 2
		}
	}
	now := w.Clock.Now().UTC()
	var due []domain.SubagentSpawnReceipt
	if err := w.Store.View(ctx, func(r port.Reader) error {
		var err error
		due, err = r.DueSubagentSpawnReceipts(now, batch)
		return err
	}); err != nil {
		return 0, err
	}

	processed := 0
	for _, candidate := range due {
		if err := ctx.Err(); err != nil {
			return processed, err
		}
		// An expired lease means execution may have happened without a durable
		// terminal commit. Park it as failed rather than executing ambiguously.
		if candidate.Status == domain.SubagentSpawnReceiptLeased {
			if err := w.failExpired(ctx, candidate); err != nil {
				// A conflicting worker may already have committed the real terminal
				// outcome. Never publish failure from the stale due-list snapshot.
				if errors.Is(err, port.ErrConflict) {
					continue
				}
				return processed, err
			}
			if err := w.Manager.PublishStatus(ctx, SubagentObservation{ID: SessionID(candidate.ReceiverSessionID), Attempt: candidate.ReceiverAttempt, State: SessionStateFailed, Failure: "execution_lease_expired_effect_unknown"}); err != nil {
				// Durable failure is already committed. Generation fences are an
				// expected concurrent outcome; unknown manager failures remain visible
				// so the control cycle cannot falsely report successful convergence.
				if !errors.Is(err, ErrSessionTerminal) && !errors.Is(err, ErrSessionAttempt) && !errors.Is(err, ErrSessionNotFound) {
					return processed, fmt.Errorf("publish expired receiver lease failure: %w", err)
				}
			}
			processed++
			continue
		}

		var leased domain.SubagentSpawnReceipt
		fenced := false
		if err := w.Store.Update(ctx, func(tx port.Transaction) error {
			current, err := tx.SubagentSpawnReceipt(candidate.CallerPeerID, candidate.RequestID)
			if err != nil {
				return err
			}
			if !current.Due(w.Clock.Now().UTC()) || (current.Status != "" && current.Status != domain.SubagentSpawnReceiptPending) {
				return port.ErrConflict
			}
			expectedStatus, expectedUpdatedAt := receiptVersion(current)
			record, err := tx.SubagentRecord(current.ReceiverSessionID)
			if err != nil {
				return err
			}
			now := w.Clock.Now().UTC()
			active := receiverGenerationActive(record, current.ReceiverAttempt, now)
			if !active {
				failed, err := domain.FailPendingSubagentSpawnReceipt(current, "receiver_generation_inactive", now)
				if err != nil {
					return err
				}
				fenced = true
				return tx.SaveSubagentSpawnReceipt(failed, expectedStatus, expectedUpdatedAt)
			}
			leased, err = domain.LeaseSubagentSpawnReceipt(current, w.Owner, now, now.Add(lease))
			if err != nil {
				return err
			}
			// When canonical leasing is enabled, the execution receipt and receiver
			// generation must not have contradictory liveness windows. Renew in the
			// same transaction as the claim, but never arm leasing when the operator
			// selected deadline-only mode or shorten a longer canonical lease.
			if !record.LeaseExpiresAt.IsZero() && record.LeaseExpiresAt.Before(leased.LeaseUntil) {
				record.LeaseExpiresAt = leased.LeaseUntil
				record.UpdatedAt = now
				if err := tx.SaveSubagentRecord(record); err != nil {
					return err
				}
			}
			return tx.SaveSubagentSpawnReceipt(leased, expectedStatus, expectedUpdatedAt)
		}); err != nil {
			if errors.Is(err, port.ErrConflict) {
				continue
			}
			return processed, err
		}
		if fenced {
			processed++
			continue
		}

		if err := w.Manager.PublishStatus(ctx, SubagentObservation{ID: SessionID(leased.ReceiverSessionID), Attempt: leased.ReceiverAttempt, State: SessionStateRunning}); err != nil {
			// The canonical generation can become terminal or be replaced after
			// the durable claim but before RUNNING becomes process-visible. Do not
			// execute work which the receiver lifecycle no longer accepts.
			if errors.Is(err, ErrSessionTerminal) || errors.Is(err, ErrSessionAttempt) || errors.Is(err, ErrSessionNotFound) {
				if failErr := w.failLeased(ctx, leased, "receiver_generation_inactive_before_execution"); failErr != nil && !errors.Is(failErr, port.ErrConflict) {
					return processed, failErr
				}
				processed++
				continue
			}
			return processed, fmt.Errorf("publish receiver running status: %w", err)
		}
		execCtx, cancel := context.WithTimeout(ctx, timeout)
		result, execErr := w.Executor.ExecuteRemoteSubagent(execCtx, leased.Task, leased.ContextMode)
		cancel()
		finished := w.Clock.Now().UTC()
		terminalAccepted := false
		if err := w.Store.Update(ctx, func(tx port.Transaction) error {
			current, err := tx.SubagentSpawnReceipt(leased.CallerPeerID, leased.RequestID)
			if err != nil {
				return err
			}
			record, err := tx.SubagentRecord(current.ReceiverSessionID)
			if err != nil {
				return err
			}
			var next domain.SubagentSpawnReceipt
			active := receiverGenerationActive(record, current.ReceiverAttempt, finished)
			// The receipt lease is also an exclusive commit boundary. At or after
			// LeaseUntil the executor may have produced an effect, but this owner no
			// longer has authority to commit its result. Park that ambiguity with the
			// same fail-closed outcome used by crash recovery instead of attempting a
			// normal owner failure, which is intentionally invalid after expiry.
			if current.Status == domain.SubagentSpawnReceiptLeased && !finished.Before(current.LeaseUntil) {
				next, err = domain.FailExpiredSubagentSpawnReceipt(current, finished, "execution_lease_expired_effect_unknown")
			} else if !active {
				next, err = domain.FailSubagentSpawnReceipt(current, w.Owner, "receiver_generation_inactive_after_execution", finished)
			} else if execErr == nil {
				next, err = domain.CompleteSubagentSpawnReceipt(current, w.Owner, result, finished)
			} else {
				next, err = domain.FailSubagentSpawnReceipt(current, w.Owner, boundedFailure(execErr), finished)
			}
			if err != nil {
				return err
			}
			if err := tx.SaveSubagentSpawnReceipt(next, current.Status, current.UpdatedAt); err != nil {
				return err
			}
			terminalAccepted = active && finished.Before(current.LeaseUntil)
			return nil
		}); err != nil {
			return processed, err
		}
		if !terminalAccepted {
			processed++
			continue
		}
		terminal := SubagentObservation{ID: SessionID(leased.ReceiverSessionID), Attempt: leased.ReceiverAttempt, State: SessionStateComplete, Result: result}
		if execErr != nil {
			terminal.State, terminal.Result, terminal.Failure = SessionStateFailed, "", boundedFailure(execErr)
		}
		if err := w.Manager.PublishStatus(ctx, terminal); err != nil {
			return processed, fmt.Errorf("publish receiver terminal status: %w", err)
		}
		processed++
	}
	return processed, nil
}

func receiverGenerationActive(record domain.SubagentRecord, attempt int, now time.Time) bool {
	return record.Attempt == attempt &&
		(record.State == domain.SubagentStatePending || record.State == domain.SubagentStateRunning) &&
		(record.Deadline.IsZero() || now.Before(record.Deadline)) &&
		(record.LeaseExpiresAt.IsZero() || now.Before(record.LeaseExpiresAt))
}

func (w *RemoteSubagentWorker) failLeased(ctx context.Context, leased domain.SubagentSpawnReceipt, failure string) error {
	now := w.Clock.Now().UTC()
	return w.Store.Update(ctx, func(tx port.Transaction) error {
		current, err := tx.SubagentSpawnReceipt(leased.CallerPeerID, leased.RequestID)
		if err != nil {
			return err
		}
		next, err := domain.FailSubagentSpawnReceipt(current, w.Owner, failure, now)
		if err != nil {
			return err
		}
		return tx.SaveSubagentSpawnReceipt(next, current.Status, current.UpdatedAt)
	})
}

func (w *RemoteSubagentWorker) failExpired(ctx context.Context, candidate domain.SubagentSpawnReceipt) error {
	now := w.Clock.Now().UTC()
	return w.Store.Update(ctx, func(tx port.Transaction) error {
		current, err := tx.SubagentSpawnReceipt(candidate.CallerPeerID, candidate.RequestID)
		if err != nil {
			return err
		}
		next, err := domain.FailExpiredSubagentSpawnReceipt(current, now, "execution_lease_expired_effect_unknown")
		if err != nil {
			return err
		}
		return tx.SaveSubagentSpawnReceipt(next, current.Status, current.UpdatedAt)
	})
}

func receiptVersion(receipt domain.SubagentSpawnReceipt) (domain.SubagentSpawnReceiptStatus, time.Time) {
	if receipt.Status == "" && receipt.UpdatedAt.IsZero() {
		return domain.SubagentSpawnReceiptPending, receipt.RecordedAt
	}
	return receipt.Status, receipt.UpdatedAt
}

func boundedFailure(err error) string {
	if err == nil {
		return ""
	}
	failure := err.Error()
	if len(failure) > domain.MaxSubagentSpawnFailureBytes {
		failure = failure[:domain.MaxSubagentSpawnFailureBytes]
	}
	return failure
}
