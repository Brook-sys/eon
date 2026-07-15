package spike

import (
	"context"
	"errors"
	"fmt"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// OfficialMutationRefs identifies every independently observable record that
// makes one canonical changeset application official. Crash recovery must see
// either none of these records or a complete, mutually consistent set.
type OfficialMutationRefs struct {
	EventID         domain.EventID
	CommitID        domain.CommitID
	ReceiptID       domain.ReceiptID
	MissionRevision domain.MissionRevisionID
	IdempotencyKey  domain.IdempotencyKey
	CanonicalType   string
	CanonicalID     string
}

func (r OfficialMutationRefs) validate() error {
	if r.EventID == "" || r.CommitID == "" || r.ReceiptID == "" || r.MissionRevision == "" || r.IdempotencyKey == "" || r.CanonicalType == "" || r.CanonicalID == "" {
		return errors.New("official mutation references are incomplete")
	}
	return nil
}

// InspectOfficialMutation classifies the compound visibility invariant used by
// the final crash harness. Merely finding the audit event is insufficient: the
// commit, receipt, head, idempotency completion and canonical entity must all
// exist and agree on the same logical commit.
func InspectOfficialMutation(ctx context.Context, store port.Store, refs OfficialMutationRefs) (CrashOutcome, error) {
	if err := refs.validate(); err != nil {
		return OutcomeInvalidPartial, err
	}
	present := 0
	consistent := true
	err := store.View(ctx, func(reader port.Reader) error {
		event, found, err := lookup(func() (domain.Event, error) { return reader.EventByID(refs.EventID) })
		if err != nil {
			return err
		}
		if found {
			present++
			consistent = consistent && event.CommitID == refs.CommitID
		}

		commit, found, err := lookup(func() (domain.Commit, error) { return reader.Commit(refs.CommitID) })
		if err != nil {
			return err
		}
		if found {
			present++
			consistent = consistent && commit.ReceiptID == refs.ReceiptID && commit.MissionRevision == refs.MissionRevision && commit.IdempotencyKey == refs.IdempotencyKey
		}

		receipt, found, err := lookup(func() (domain.CommitReceipt, error) { return reader.CommitReceipt(refs.ReceiptID) })
		if err != nil {
			return err
		}
		if found {
			present++
			consistent = consistent && receipt.CommitID == refs.CommitID
		}

		head, found, err := lookup(func() (domain.Commit, error) { return reader.HeadCommit(refs.MissionRevision) })
		if err != nil {
			return err
		}
		if found {
			present++
			consistent = consistent && head.ID == refs.CommitID
		}

		idempotency, found, err := lookup(func() (domain.IdempotencyRecord, error) { return reader.IdempotencyRecord(refs.IdempotencyKey) })
		if err != nil {
			return err
		}
		if found {
			present++
			consistent = consistent && idempotency.Status == domain.IdempotencyCompleted && idempotency.ReceiptID == refs.ReceiptID && idempotency.ResultRef == string(refs.CommitID)
		}

		entity, found, err := lookup(func() (domain.CanonicalEntity, error) {
			return reader.CanonicalEntity(refs.CanonicalType, refs.CanonicalID)
		})
		if err != nil {
			return err
		}
		if found {
			present++
			consistent = consistent && entity.CommitID == refs.CommitID
		}
		return nil
	})
	if err != nil {
		return OutcomeInvalidPartial, fmt.Errorf("inspect official mutation: %w", err)
	}
	if present == 0 {
		return OutcomeNotApplied, nil
	}
	if present == 6 && consistent {
		return OutcomeApplied, nil
	}
	return OutcomeInvalidPartial, nil
}

func lookup[T any](get func() (T, error)) (T, bool, error) {
	value, err := get()
	if err == nil {
		return value, true, nil
	}
	if errors.Is(err, port.ErrNotFound) {
		var zero T
		return zero, false, nil
	}
	var zero T
	return zero, false, err
}
