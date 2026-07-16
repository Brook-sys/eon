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
	DecisionDispatch          DecisionKind = "DISPATCH"
	DecisionExpand            DecisionKind = "EXPAND"
	DecisionDiagnose          DecisionKind = "DIAGNOSE"
	DecisionContinuityBlocked DecisionKind = "CONTINUITY_BLOCKED"
)

type Decision struct {
	Kind               DecisionKind
	Operation          domain.OperationID
	Action             domain.ContinuityAction
	Strategy           string
	StrategiesTried    []string
	ContinuityFailure  string
	Horizon            domain.ExecutableHorizon
	DiagnosisID        domain.ContinuityDiagnosisID
	UnavailableCaps    []string
	RecoveryConditions []string
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
	Registry   *StrategyRegistry
	Policy     domain.HorizonPolicy
	// IDs is optional; when set, ContinuityBlocked diagnosis ids are generated with it.
	IDs source.IDGenerator
}

func (s Scheduler) strategies() []ContinuityStrategy {
	if s.Registry != nil && s.Registry.Len() > 0 {
		return s.Registry.Strategies()
	}
	return s.Strategies
}

func (s Scheduler) policy() domain.HorizonPolicy {
	if s.Policy.Version == "" && s.Policy.SchemaVersion == 0 {
		return domain.DefaultHorizonPolicy()
	}
	return s.Policy
}

func (s Scheduler) Step(ctx context.Context, missionRevision domain.MissionRevisionID) (Decision, error) {
	if s.Store == nil || s.Clock == nil || missionRevision == "" {
		return Decision{}, errors.New("invalid scheduler configuration")
	}
	policy := s.policy()
	if err := policy.Validate(); err != nil {
		return Decision{}, fmt.Errorf("horizon policy: %w", err)
	}
	if decision, found, err := s.selectOrResume(ctx, missionRevision); err != nil || found {
		return decision, err
	}

	strategies := s.strategies()
	tried := make([]string, 0, len(strategies))
	for index, strategy := range strategies {
		if strategy == nil || strategy.Name() == "" {
			return Decision{}, errors.New("continuity strategy must have a name")
		}
		horizon, err := s.observeHorizon(ctx, missionRevision, policy)
		if err != nil {
			return Decision{}, err
		}
		remaining := len(strategies) - index
		plan, err := PlanContinuityAction(horizon, remaining, strategy.Name())
		if err != nil {
			return Decision{}, err
		}
		if plan.Action != domain.ContinuityExpand {
			break
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
			if found {
				decision.Action = domain.ContinuityExpand
				decision.Strategy = strategy.Name()
				decision.StrategiesTried = append([]string(nil), tried...)
				decision.Horizon = horizon
			}
			return decision, err
		}
	}

	horizon, err := s.observeHorizon(ctx, missionRevision, policy)
	if err != nil {
		return Decision{}, err
	}
	diagnosis, err := s.persistDiagnosis(ctx, missionRevision, policy, tried, horizon)
	if err != nil {
		return Decision{}, err
	}
	return Decision{
		Kind:               DecisionContinuityBlocked,
		Action:             domain.ContinuityDiagnose,
		StrategiesTried:    tried,
		ContinuityFailure:  diagnosis.SafeDetail,
		Horizon:            horizon,
		DiagnosisID:        diagnosis.ID,
		UnavailableCaps:    append([]string(nil), diagnosis.UnavailableCapabilities...),
		RecoveryConditions: append([]string(nil), diagnosis.RecoveryConditions...),
	}, nil
}

func (s Scheduler) observeHorizon(ctx context.Context, missionRevision domain.MissionRevisionID, policy domain.HorizonPolicy) (domain.ExecutableHorizon, error) {
	var horizon domain.ExecutableHorizon
	err := s.Store.View(ctx, func(r port.Reader) error {
		operations, err := r.Operations(missionRevision)
		if err != nil {
			return err
		}
		ready := 0
		for _, operation := range operations {
			if operation.State == domain.StateReady {
				ready++
			}
		}
		open := 0
		opportunities, err := r.WorkOpportunities(missionRevision, domain.OpportunityOpen)
		if err != nil {
			return err
		}
		open += len(opportunities)
		deferred, err := r.WorkOpportunities(missionRevision, domain.OpportunityDeferred)
		if err != nil {
			return err
		}
		open += len(deferred)
		horizon = domain.ExecutableHorizon{
			SchemaVersion:   domain.SchemaVersionV1,
			MissionRevision: missionRevision,
			PolicyVersion:   policy.Version,
			ReadyCount:      ready,
			OpenCandidates:  open,
			TargetReady:     policy.TargetReady,
			LowWatermark:    policy.LowWatermark,
			MaxReady:        policy.MaxReady,
			ObservedAt:      s.Clock.Now().UTC(),
		}
		return horizon.Validate()
	})
	if err != nil {
		return domain.ExecutableHorizon{}, fmt.Errorf("observe executable horizon: %w", err)
	}
	return horizon, nil
}

func (s Scheduler) persistDiagnosis(ctx context.Context, missionRevision domain.MissionRevisionID, policy domain.HorizonPolicy, tried []string, horizon domain.ExecutableHorizon) (domain.ContinuityDiagnosis, error) {
	detail := "no executable work after all configured continuity strategies"
	strategiesTried := append([]string(nil), tried...)
	if len(strategiesTried) == 0 {
		detail = "no continuity strategies configured and no executable work"
		strategiesTried = []string{"none_configured"}
	}
	recovery := []string{
		"admit work opportunity with expected delta",
		"restore blocked capability",
		"operator command or authorized source providing new scope",
	}
	diagnosis := domain.ContinuityDiagnosis{
		SchemaVersion:      domain.SchemaVersionV1,
		ID:                 "continuity_blocked_ephemeral",
		MissionRevision:    missionRevision,
		OccurredAt:         s.Clock.Now().UTC(),
		StrategiesTried:    strategiesTried,
		OpenCandidateCount: horizon.OpenCandidates,
		ReadyCount:         horizon.ReadyCount,
		RecoveryConditions: recovery,
		SafeDetail:         detail,
		PolicyVersion:      policy.Version,
	}
	if s.IDs != nil {
		id, idErr := s.IDs.NewID("continuity")
		if idErr != nil {
			return domain.ContinuityDiagnosis{}, fmt.Errorf("generate continuity diagnosis id: %w", idErr)
		}
		diagnosis.ID = domain.ContinuityDiagnosisID(id)
	} else {
		diagnosis.ID = domain.ContinuityDiagnosisID(fmt.Sprintf("continuity_%s_%d", missionRevision, horizon.ObservedAt.UnixNano()))
	}
	if err := diagnosis.Validate(); err != nil {
		return domain.ContinuityDiagnosis{}, err
	}
	err := s.Store.Update(ctx, func(tx port.Transaction) error {
		if err := tx.CreateContinuityDiagnosis(diagnosis); err != nil {
			return err
		}
		_, err := tx.AppendEvent(domain.Event{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(string(diagnosis.ID) + ":event"),
			Kind:            domain.EventContinuityBlocked,
			OccurredAt:      diagnosis.OccurredAt,
			MissionRevision: missionRevision,
			PayloadRef:      string(diagnosis.ID),
		})
		return err
	})
	if err != nil {
		return domain.ContinuityDiagnosis{}, fmt.Errorf("persist continuity diagnosis: %w", err)
	}
	return diagnosis, nil
}

func (s Scheduler) selectOrResume(ctx context.Context, missionRevision domain.MissionRevisionID) (Decision, bool, error) {
	var selected domain.Operation
	err := s.Store.Update(ctx, func(tx port.Transaction) error {
		operations, err := tx.Operations(missionRevision)
		if err != nil {
			return err
		}
		now := s.Clock.Now().UTC()
		allowsDispatch, err := s.allowsDispatch(tx, missionRevision, now)
		if err != nil {
			return err
		}
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
			if allowsDispatch && selected.ID == "" && operation.State == domain.StateReady {
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

// allowsDispatch consults durable control state. Missing control state means
// the process has never received an operator command and remains running.
func (s Scheduler) allowsDispatch(tx port.Transaction, missionRevision domain.MissionRevisionID, now time.Time) (bool, error) {
	state, err := tx.ControlState()
	if err != nil {
		if errors.Is(err, port.ErrNotFound) {
			return true, nil
		}
		return false, err
	}
	missionID := domain.MissionID("")
	if revision, revErr := tx.MissionRevision(missionRevision); revErr == nil {
		missionID = revision.MissionID
	} else if !errors.Is(revErr, port.ErrNotFound) {
		return false, revErr
	}
	if missionID == "" {
		return state.ProcessMode == domain.ProcessRunning, nil
	}
	_ = now
	return state.AllowsDispatch(missionID), nil
}
