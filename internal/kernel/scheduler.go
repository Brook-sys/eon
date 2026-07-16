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
	// Cooldowns, when set, skip families that recently expanded without delta
	// and rotate toward other registered strategies (anti-fixation).
	Cooldowns *StrategyCooldownBook
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

// resolvePolicy prefers an explicit non-zero Policy, then the active durable
// HORIZON revision, then the built-in default. Explicit still wins so unit
// tests and callers can pin a horizon without mutating the store.
func (s Scheduler) resolvePolicy(ctx context.Context) (domain.HorizonPolicy, error) {
	return ResolveHorizonPolicy(ctx, s.Store, s.Policy)
}

func (s Scheduler) Step(ctx context.Context, missionRevision domain.MissionRevisionID) (Decision, error) {
	if s.Store == nil || s.Clock == nil || missionRevision == "" {
		return Decision{}, errors.New("invalid scheduler configuration")
	}
	policy, err := s.resolvePolicy(ctx)
	if err != nil {
		return Decision{}, fmt.Errorf("horizon policy: %w", err)
	}
	if err := policy.Validate(); err != nil {
		return Decision{}, fmt.Errorf("horizon policy: %w", err)
	}
	if decision, found, err := s.selectOrResume(ctx, missionRevision); err != nil || found {
		return decision, err
	}

	// Preventive replenishment: when ready work is at/below low_watermark but
	// the frontier still has OPEN opportunities, admit before family strategies.
	if s.IDs != nil {
		replenisher := Replenisher{Store: s.Store, Clock: s.Clock, IDs: s.IDs, Policy: policy}
		if _, _, err := replenisher.PreventivelyReplenish(ctx, missionRevision); err != nil {
			return Decision{}, err
		}
		if decision, found, err := s.selectOrResume(ctx, missionRevision); err != nil || found {
			if found {
				decision.Action = domain.ContinuityExpand
				decision.Strategy = "frontier_admission"
			}
			return decision, err
		}
	}

	strategies := s.strategies()
	tried := make([]string, 0, len(strategies))
	eliminated := make([]string, 0)
	now := s.Clock.Now().UTC()
	for index, strategy := range strategies {
		if strategy == nil || strategy.Name() == "" {
			return Decision{}, errors.New("continuity strategy must have a name")
		}
		if s.Cooldowns != nil && s.Cooldowns.Active(strategy.Name(), now) {
			eliminated = append(eliminated, "cooldown:"+strategy.Name())
			continue
		}
		horizon, err := s.observeHorizon(ctx, missionRevision, policy)
		if err != nil {
			return Decision{}, err
		}
		// remaining counts strategies not yet attempted in this step, including
		// the current one; cooled strategies do not count as expansion options.
		remaining := 0
		for j := index; j < len(strategies); j++ {
			if strategies[j] == nil {
				continue
			}
			if s.Cooldowns != nil && s.Cooldowns.Active(strategies[j].Name(), now) {
				continue
			}
			remaining++
		}
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
		if result.Admitted == 0 {
			if s.Cooldowns != nil {
				s.Cooldowns.MarkNoDelta(strategy.Name(), now, policy.StrategyCooldown)
			}
		} else if s.Cooldowns != nil {
			s.Cooldowns.Clear(strategy.Name())
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
	if len(tried) == 0 && len(eliminated) > 0 {
		// Every strategy is cooling down: still a continuity violation, but the
		// diagnosis records the cooldown barrier instead of pretending none ran.
		tried = append(tried, "all_strategies_in_cooldown")
	}
	diagnosis, err := s.persistDiagnosis(ctx, missionRevision, policy, tried, eliminated, horizon)
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

func (s Scheduler) persistDiagnosis(ctx context.Context, missionRevision domain.MissionRevisionID, policy domain.HorizonPolicy, tried, eliminated []string, horizon domain.ExecutableHorizon) (domain.ContinuityDiagnosis, error) {
	detail := "no executable work after all configured continuity strategies"
	strategiesTried := append([]string(nil), tried...)
	if len(strategiesTried) == 0 {
		detail = "no continuity strategies configured and no executable work"
		strategiesTried = []string{"none_configured"}
	}
	if len(eliminated) > 0 && strategiesTried[0] == "all_strategies_in_cooldown" {
		detail = "all continuity strategies are in no-delta cooldown"
	}
	recovery := []string{
		"admit work opportunity with expected delta",
		"restore blocked capability",
		"operator command or authorized source providing new scope",
	}
	if policy.StrategyCooldown > 0 {
		recovery = append(recovery, "wait strategy cooldown or clear no-delta cooldowns")
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
	if len(eliminated) > 0 {
		diagnosis.EliminatedAlternatives = append([]string(nil), eliminated...)
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
