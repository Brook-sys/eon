package kernel

import (
	"context"
	"errors"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// SubagentStatusIngressWorker applies a bounded batch of durable transport
// evidence to SessionManager. Supervisor remains the sole canonical writer.
type SubagentStatusIngressWorker struct {
	Store   port.Store
	Manager SessionManager
	Clock   interface{ Now() time.Time }
	Batch   int
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
		err := w.Manager.PublishStatus(ctx, SubagentObservation{ID: SessionID(receipt.SessionID), Attempt: receipt.Attempt, State: SessionState(receipt.State), Result: receipt.Result, Failure: receipt.Failure})
		if err != nil {
			if !errors.Is(err, ErrSessionAttempt) && !errors.Is(err, ErrSessionTerminal) {
				return processed, err
			}
			rejectionCode := domain.SubagentStatusIngressRejectionAttemptMismatch
			if errors.Is(err, ErrSessionTerminal) {
				rejectionCode = domain.SubagentStatusIngressRejectionTerminalConflict
			}
			err = w.Store.Update(ctx, func(tx port.Transaction) error {
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
				return tx.SaveSubagentStatusIngressReceipt(next, domain.SubagentStatusIngressPending)
			})
			if err != nil {
				if errors.Is(err, port.ErrConflict) {
					continue
				}
				return processed, err
			}
			processed++
			continue
		}
		err = w.Store.Update(ctx, func(tx port.Transaction) error {
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
			next, err := domain.MarkSubagentStatusIngressApplied(current, w.Clock.Now().UTC())
			if err != nil {
				return err
			}
			return tx.SaveSubagentStatusIngressReceipt(next, domain.SubagentStatusIngressPending)
		})
		if err != nil {
			if errors.Is(err, port.ErrConflict) {
				continue
			}
			return processed, err
		}
		processed++
	}
	return processed, nil
}
