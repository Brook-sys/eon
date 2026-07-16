// Package kernel contains deterministic runtime coordination.
package kernel

import (
	"context"
	"errors"
	"fmt"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
)

type DecisionKind string

const (
	DecisionDispatch          DecisionKind = "DISPATCH"
	DecisionContinuityBlocked DecisionKind = "CONTINUITY_BLOCKED"
)

type Decision struct {
	Kind              DecisionKind
	Operation         domain.OperationID
	StrategiesTried   []string
	ContinuityFailure string
}

type ContinuityResult struct {
	Admitted int
	Changed  bool
}

// ContinuityStrategy derives a bounded family of useful work from mission
// state. It must not poll, retry without budget, or report admission without
// persisting the corresponding units.
type ContinuityStrategy interface {
	Name() string
	Replenish(context.Context, domain.MissionRevisionID) (ContinuityResult, error)
}

type Scheduler struct {
	Store      port.Store
	Clock      source.Clock
	Strategies []ContinuityStrategy
}

func (s Scheduler) Step(ctx context.Context, missionRevision domain.MissionRevisionID) (Decision, error) {
	if s.Store == nil || s.Clock == nil || missionRevision == "" {
		return Decision{}, errors.New("invalid scheduler configuration")
	}
	if decision, found, err := s.selectOrResume(ctx, missionRevision); err != nil || found {
		return decision, err
	}

	tried := make([]string, 0, len(s.Strategies))
	for _, strategy := range s.Strategies {
		if strategy == nil || strategy.Name() == "" {
			return Decision{}, errors.New("continuity strategy must have a name")
		}
		tried = append(tried, strategy.Name())
		result, err := strategy.Replenish(ctx, missionRevision)
		if err != nil {
			return Decision{}, fmt.Errorf("continuity strategy %s: %w", strategy.Name(), err)
		}
		if result.Admitted < 0 {
			return Decision{}, fmt.Errorf("continuity strategy %s returned negative admission count", strategy.Name())
		}
		if decision, found, err := s.selectOrResume(ctx, missionRevision); err != nil || found {
			return decision, err
		}
	}

	return Decision{
		Kind:              DecisionContinuityBlocked,
		StrategiesTried:   tried,
		ContinuityFailure: "no executable work after all configured continuity strategies",
	}, nil
}

func (s Scheduler) selectOrResume(ctx context.Context, missionRevision domain.MissionRevisionID) (Decision, bool, error) {
	var selected domain.Operation
	err := s.Store.Update(ctx, func(tx port.Transaction) error {
		operations, err := tx.Operations(missionRevision)
		if err != nil {
			return err
		}
		now := s.Clock.Now().UTC()
		for _, operation := range operations {
			if operation.State == domain.StateWaitingTime && operation.Reevaluation.NotBefore != nil && !operation.Reevaluation.NotBefore.After(now) {
				next, err := domain.Transition(domain.OperationalSnapshot{State: operation.State, Reevaluation: operation.Reevaluation}, domain.TransitionInput{Event: domain.EventResume})
				if err != nil {
					return err
				}
				operation.State, operation.Reevaluation = next.State, next.Reevaluation
				if err := tx.SaveOperation(operation); err != nil {
					return err
				}
			}
			if selected.ID == "" && operation.State == domain.StateReady {
				selected = operation
			}
		}
		return nil
	})
	if err != nil {
		return Decision{}, false, fmt.Errorf("select work: %w", err)
	}
	if selected.ID == "" {
		return Decision{}, false, nil
	}
	return Decision{Kind: DecisionDispatch, Operation: selected.ID}, true, nil
}
