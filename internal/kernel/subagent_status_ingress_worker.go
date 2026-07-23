package kernel

import (
	"context"
	"errors"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/retry"
)

// SubagentStatusIngressWorker applies a bounded batch of durable transport
// evidence to SessionManager. Supervisor remains the sole canonical writer.
type SubagentStatusIngressWorker struct {
	Store   port.Store
	Manager SessionManager
	Clock   interface{ Now() time.Time }
	Batch   int
	// LeaseTTL renews the active generation only after an authenticated RUNNING
	// receipt has been accepted by SessionManager. Zero keeps lease renewal off.
	LeaseTTL time.Duration
	// RetryPolicy is optional and applies only to the idempotent receipt CAS
	// transaction. SessionManager publication remains replay-safe and storage
	// adapters still perform exactly one Update attempt per invocation.
	RetryPolicy  retry.Policy
	RetrySleeper retry.Sleeper
	RetryJitter  retry.JitterSource
	RetryObserve func(retry.Report)
}

func (w *SubagentStatusIngressWorker) ApplyPending(ctx context.Context) (int, error) {
	if w == nil || w.Store == nil || w.Manager == nil || w.Clock == nil {
		return 0, errors.New("subagent status ingress worker dependencies are incomplete")
	}
	batch := w.Batch
	if batch <= 0 {
		batch = 4
	}
	var receipts []domain.SubagentStatusIngressReceipt
	if err := w.Store.View(ctx, func(r port.Reader) error {
		var err error
		receipts, err = r.PendingSubagentStatusIngressReceipts(batch)
		return err
	}); err != nil {
		return 0, err
	}
	processed := 0
	for _, receipt := range receipts {
		err := w.Manager.PublishStatus(ctx, ingressObservation(receipt))
		if err != nil {
			if !errors.Is(err, ErrSessionAttempt) && !errors.Is(err, ErrSessionTerminal) {
				return processed, err
			}
			rejectionCode := domain.SubagentStatusIngressRejectionAttemptMismatch
			if errors.Is(err, ErrSessionTerminal) {
				rejectionCode = domain.SubagentStatusIngressRejectionTerminalConflict
			}
			transitioned := false
			err = w.updateReceipt(ctx, func(tx port.Transaction) error {
				current, err := tx.SubagentStatusIngressReceipt(receipt.CallerPeerID, receipt.DeliveryID)
				if err != nil {
					return err
				}
				if current.Status == domain.SubagentStatusIngressRejected && current.Matches(receipt) {
					return nil
				}
				var next domain.SubagentStatusIngressReceipt
				if rejectionCode == domain.SubagentStatusIngressRejectionTerminalConflict {
					next, err = domain.RejectSubagentStatusIngressTerminalConflict(current, w.Clock.Now().UTC())
				} else {
					next, err = domain.RejectSubagentStatusIngressAttemptMismatch(current, w.Clock.Now().UTC())
				}
				if err != nil {
					return err
				}
				if err := tx.SaveSubagentStatusIngressReceipt(next, domain.SubagentStatusIngressPending); err != nil {
					return err
				}
				transitioned = true
				return nil
			})
			if err != nil {
				if errors.Is(err, port.ErrConflict) && !errors.Is(err, retry.ErrBudgetExhausted) {
					continue
				}
				return processed, err
			}
			if transitioned {
				processed++
			}
			continue
		}
		transitioned := false
		err = w.updateReceipt(ctx, func(tx port.Transaction) error {
			current, err := tx.SubagentStatusIngressReceipt(receipt.CallerPeerID, receipt.DeliveryID)
			if err != nil {
				return err
			}
			if current.Status == domain.SubagentStatusIngressApplied {
				if current.Matches(receipt) {
					return nil
				}
				return port.ErrConflict
			}
			now := w.Clock.Now().UTC()
			var record domain.SubagentRecord
			if receipt.State == string(SessionStateRunning) && w.LeaseTTL > 0 {
				record, err = tx.SubagentRecord(receipt.SessionID)
				if err != nil {
					return err
				}
				if record.Attempt != receipt.Attempt || (record.State != domain.SubagentStatePending && record.State != domain.SubagentStateRunning) {
					rejected, rejectErr := domain.RejectSubagentStatusIngressAttemptMismatch(current, now)
					if rejectErr != nil {
						return rejectErr
					}
					return tx.SaveSubagentStatusIngressReceipt(rejected, domain.SubagentStatusIngressPending)
				}
			}
			next, err := domain.MarkSubagentStatusIngressApplied(current, now)
			if err != nil {
				return err
			}
			if err := tx.SaveSubagentStatusIngressReceipt(next, domain.SubagentStatusIngressPending); err != nil {
				return err
			}
			transitioned = true
			if receipt.State == string(SessionStateRunning) && w.LeaseTTL > 0 {
				renewedUntil := now.Add(w.LeaseTTL)
				if renewedUntil.After(record.LeaseExpiresAt) {
					record.LeaseExpiresAt = renewedUntil
					record.UpdatedAt = now
					return tx.SaveSubagentRecord(record)
				}
			}
			return nil
		})
		if err != nil {
			if errors.Is(err, port.ErrConflict) && !errors.Is(err, retry.ErrBudgetExhausted) {
				continue
			}
			return processed, err
		}
		if transitioned {
			processed++
		}
	}
	return processed, nil
}

func (w *SubagentStatusIngressWorker) updateReceipt(ctx context.Context, update func(port.Transaction) error) error {
	operation := func(context.Context, int) error {
		return w.Store.Update(ctx, update)
	}
	if w.RetryPolicy.MaxAttempts == 0 {
		return operation(ctx, 1)
	}
	report, err := retry.Do(ctx, w.RetryPolicy, w.RetrySleeper, w.RetryJitter, func(err error) (string, bool) {
		if errors.Is(err, port.ErrConflict) {
			return "conflict", true
		}
		return "fatal", false
	}, operation)
	if w.RetryObserve != nil {
		w.RetryObserve(report)
	}
	return err
}

func ingressObservation(receipt domain.SubagentStatusIngressReceipt) SubagentObservation {
	return SubagentObservation{ID: SessionID(receipt.SessionID), Attempt: receipt.Attempt, State: SessionState(receipt.State), Result: receipt.Result, Failure: receipt.Failure}
}
