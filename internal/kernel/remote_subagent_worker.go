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
			active := record.Attempt == current.ReceiverAttempt &&
				(record.State == domain.SubagentStatePending || record.State == domain.SubagentStateRunning) &&
				(record.Deadline.IsZero() || now.Before(record.Deadline))
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
			active := record.Attempt == current.ReceiverAttempt &&
				(record.State == domain.SubagentStatePending || record.State == domain.SubagentStateRunning) &&
				(record.Deadline.IsZero() || finished.Before(record.Deadline))
			if !active {
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
			terminalAccepted = active
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
