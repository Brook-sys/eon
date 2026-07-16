package kernel

import (
	"context"
	"errors"
	"fmt"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
)

// Event kinds for lease reconciliation (derived diagnostics; transitions are
// authoritative via domain.Transition).
const (
	EventOperationLeaseExpired = "operation.lease_expired"
	EventOperationReconciling  = "operation.reconciling"
)

// LeaseReaper scans RUNNING/VERIFYING operations whose lease deadline has
// elapsed and moves them through RECONCILE → REPLANNING → RESUME → READY with
// EffectUnknown (FR-DUR-006). Ambiguous model/worker effects never auto-retry
// into SUCCEEDED.
type LeaseReaper struct {
	Store port.Store
	Clock source.Clock
	IDs   source.IDGenerator
}

// ReconcileResult summarizes one Reconcile pass.
type ReconcileResult struct {
	Scanned    int
	Reconciled int
	Skipped    int
}

func (r LeaseReaper) validateDeps() error {
	if r.Store == nil || r.Clock == nil {
		return errors.New("lease reaper dependencies are incomplete")
	}
	return nil
}

// Reconcile expired operation leases for one mission revision.
func (r LeaseReaper) Reconcile(ctx context.Context, missionRevision domain.MissionRevisionID) (ReconcileResult, error) {
	if err := r.validateDeps(); err != nil {
		return ReconcileResult{}, err
	}
	if missionRevision == "" {
		return ReconcileResult{}, errors.New("mission revision is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var result ReconcileResult
	err := r.Store.Update(ctx, func(tx port.Transaction) error {
		operations, err := tx.Operations(missionRevision)
		if err != nil {
			return err
		}
		now := r.Clock.Now().UTC()
		for _, operation := range operations {
			if operation.State != domain.StateRunning && operation.State != domain.StateVerifying {
				continue
			}
			result.Scanned++
			if !LeaseExpired(operation.Reevaluation, now) {
				result.Skipped++
				continue
			}
			if err := r.reconcileOne(tx, operation, now); err != nil {
				return fmt.Errorf("reconcile operation %s: %w", operation.ID, err)
			}
			result.Reconciled++
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	return result, nil
}

func (r LeaseReaper) reconcileOne(tx port.Transaction, operation domain.Operation, now time.Time) error {
	leaseRef := operation.Reevaluation.Reference
	snap := domain.OperationalSnapshot{State: operation.State, Reevaluation: operation.Reevaluation}
	// UNKNOWN effect: do not invent success; replan then resume to READY.
	replanning, err := domain.Transition(snap, domain.TransitionInput{
		Event:       domain.EventReconcile,
		EffectState: domain.EffectUnknown,
		Reference:   leaseRef,
	})
	if err != nil {
		return fmt.Errorf("reconcile transition: %w", err)
	}
	ready, err := domain.Transition(replanning, domain.TransitionInput{Event: domain.EventResume})
	if err != nil {
		return fmt.Errorf("resume after reconcile: %w", err)
	}
	operation.State = ready.State
	operation.Reevaluation = ready.Reevaluation
	if err := tx.SaveOperation(operation); err != nil {
		return err
	}

	events := []domain.Event{
		{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(fmt.Sprintf("%s:lease_expired:%d:%d", operation.ID, operation.Attempt, now.UnixNano())),
			Kind:            EventOperationLeaseExpired,
			OccurredAt:      now,
			MissionRevision: operation.MissionRevision,
			InquiryID:       operation.InquiryID,
			OperationID:     operation.ID,
			PayloadRef:      leaseRef,
		},
		{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(fmt.Sprintf("%s:reconciling:%d:%d", operation.ID, operation.Attempt, now.UnixNano())),
			Kind:            EventOperationReconciling,
			OccurredAt:      now,
			MissionRevision: operation.MissionRevision,
			InquiryID:       operation.InquiryID,
			OperationID:     operation.ID,
			PayloadRef:      "effect=UNKNOWN;next=READY",
		},
	}
	for _, event := range events {
		if _, err := tx.AppendEvent(event); err != nil {
			return err
		}
	}
	return nil
}
