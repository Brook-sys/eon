package spike

import (
	"context"
	"errors"
	"fmt"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

type CrashOutcome string

const (
	OutcomeNotApplied     CrashOutcome = "NOT_APPLIED"
	OutcomeApplied        CrashOutcome = "APPLIED"
	OutcomeInvalidPartial CrashOutcome = "INVALID_PARTIAL"
)

type CrashIntent struct {
	Event domain.Event `json:"event"`
}

// ApplyCrashIntent is the smallest official mutation used by the subprocess
// harness. The adapter failpoint must terminate the worker at a durability
// boundary; returning an error from this callback is not a crash simulation.
func ApplyCrashIntent(ctx context.Context, store port.Store, intent CrashIntent) error {
	return store.Update(ctx, func(tx port.Transaction) error {
		_, err := tx.AppendEvent(intent.Event)
		return err
	})
}

func InspectCrashIntent(ctx context.Context, store port.Store, intent CrashIntent) (CrashOutcome, error) {
	var found bool
	err := store.View(ctx, func(reader port.Reader) error {
		_, err := reader.EventByID(intent.Event.ID)
		switch {
		case err == nil:
			found = true
			return nil
		case errors.Is(err, port.ErrNotFound):
			return nil
		default:
			return err
		}
	})
	if err != nil {
		return OutcomeInvalidPartial, fmt.Errorf("inspect crash intent: %w", err)
	}
	if found {
		return OutcomeApplied, nil
	}
	return OutcomeNotApplied, nil
}
