// Package kernel contains deterministic runtime coordination.
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

type DecisionKind string

const (
	DecisionDispatch DecisionKind = "DISPATCH"
	DecisionRest     DecisionKind = "REST"
)

type Decision struct {
	Kind      DecisionKind
	Operation domain.OperationID
	Rest      domain.Rest
}

type Replenisher interface {
	Replenish(context.Context, domain.MissionRevisionID) (admitted bool, err error)
}

type Scheduler struct {
	Store            port.Store
	Clock            source.Clock
	Replenisher      Replenisher
	MaxReplenishment int
	RestInterval     time.Duration
}

func (s Scheduler) Step(ctx context.Context, missionRevision domain.MissionRevisionID) (Decision, error) {
	if s.Store == nil || s.Clock == nil || missionRevision == "" || s.MaxReplenishment < 0 || s.RestInterval <= 0 {
		return Decision{}, errors.New("invalid scheduler configuration")
	}
	if decision, found, err := s.selectOrResume(ctx, missionRevision); err != nil || found {
		return decision, err
	}
	for attempt := 0; attempt < s.MaxReplenishment && s.Replenisher != nil; attempt++ {
		admitted, err := s.Replenisher.Replenish(ctx, missionRevision)
		if err != nil {
			return Decision{}, fmt.Errorf("replenish agenda: %w", err)
		}
		if !admitted {
			break
		}
		if decision, found, err := s.selectOrResume(ctx, missionRevision); err != nil || found {
			return decision, err
		}
	}

	now := s.Clock.Now().UTC()
	notBefore := now.Add(s.RestInterval)
	rest := domain.Rest{SchemaVersion: 1, MissionRevision: missionRevision, Reason: "no executable work after bounded replenishment", EnteredAt: now, Active: true, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateNotBefore, NotBefore: &notBefore}}
	if err := s.Store.Update(ctx, func(tx port.Transaction) error { return tx.SaveRest(rest) }); err != nil {
		return Decision{}, fmt.Errorf("persist rest: %w", err)
	}
	return Decision{Kind: DecisionRest, Rest: rest}, nil
}

func (s Scheduler) Wait(ctx context.Context, rest domain.Rest) error {
	if err := rest.Validate(); err != nil || !rest.Active || rest.Reevaluation.NotBefore == nil {
		return errors.New("scheduler can only wait on active temporal rest")
	}
	if err := s.Clock.WaitUntil(ctx, *rest.Reevaluation.NotBefore); err != nil {
		return err
	}
	wokenAt := s.Clock.Now().UTC()
	rest.Active = false
	rest.Reevaluation = domain.ReevaluationCondition{}
	rest.WokenAt = &wokenAt
	return s.Store.Update(ctx, func(tx port.Transaction) error { return tx.SaveRest(rest) })
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
