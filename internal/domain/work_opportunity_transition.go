package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// WorkOpportunityTransitionEvent is a pure lifecycle event for frontier units.
// Admission into the agenda still goes through Admitter (needs inquiry binding);
// these events cover reservoir hygiene only.
type WorkOpportunityTransitionEvent string

const (
	// OppEventDefer parks an OPEN opportunity without destroying it.
	OppEventDefer WorkOpportunityTransitionEvent = "DEFER"
	// OppEventReopen returns a DEFERRED opportunity to OPEN.
	OppEventReopen WorkOpportunityTransitionEvent = "REOPEN"
	// OppEventAbandon eliminates an active opportunity with a durable reason.
	OppEventAbandon WorkOpportunityTransitionEvent = "ABANDON"
	// OppEventSupersede marks an active opportunity replaced by a better unit.
	// The successor id is recorded in AbandonReason as "superseded_by:<id>" when set.
	OppEventSupersede WorkOpportunityTransitionEvent = "SUPERSEDE"
)

// WorkOpportunityTransition is the pure input for TransitionWorkOpportunity.
type WorkOpportunityTransition struct {
	Event      WorkOpportunityTransitionEvent
	OccurredAt time.Time
	// Reason is required for ABANDON and optional detail for SUPERSEDE/DEFER.
	Reason string
	// SupersededBy is required for SUPERSEDE (distinct successor opportunity id).
	SupersededBy WorkOpportunityID
}

// TransitionWorkOpportunity applies a pure optimistic transition. Callers
// persist the result with SaveWorkOpportunity and an audit event atomically.
func TransitionWorkOpportunity(current WorkOpportunity, transition WorkOpportunityTransition) (WorkOpportunity, error) {
	if err := current.Validate(); err != nil {
		return WorkOpportunity{}, fmt.Errorf("validate current work opportunity: %w", err)
	}
	if transition.OccurredAt.IsZero() {
		return WorkOpportunity{}, errors.New("work opportunity transition requires occurrence time")
	}
	if transition.OccurredAt.Before(current.CreatedAt) {
		return WorkOpportunity{}, errors.New("work opportunity transition precedes creation")
	}
	// Terminal statuses cannot move.
	switch current.Status {
	case OpportunityAbandoned, OpportunitySuperseded, OpportunityAdmitted:
		return WorkOpportunity{}, fmt.Errorf("work opportunity in status %s cannot transition via hygiene events", current.Status)
	}

	next := current
	next.UpdatedAt = transition.OccurredAt.UTC()
	reason := strings.TrimSpace(transition.Reason)

	switch transition.Event {
	case OppEventDefer:
		if current.Status != OpportunityOpen {
			return WorkOpportunity{}, errors.New("only OPEN work opportunities may be deferred")
		}
		if transition.SupersededBy != "" {
			return WorkOpportunity{}, errors.New("defer transition must not set superseded_by")
		}
		next.Status = OpportunityDeferred
		if reason != "" {
			next.AbandonReason = reason
		}
	case OppEventReopen:
		if current.Status != OpportunityDeferred {
			return WorkOpportunity{}, errors.New("only DEFERRED work opportunities may be reopened")
		}
		if transition.SupersededBy != "" {
			return WorkOpportunity{}, errors.New("reopen transition must not set superseded_by")
		}
		next.Status = OpportunityOpen
		// Clear parking detail so reopen looks clean in inspect.
		next.AbandonReason = ""
	case OppEventAbandon:
		if !current.Status.Active() {
			return WorkOpportunity{}, errors.New("only active work opportunities may be abandoned")
		}
		if reason == "" {
			return WorkOpportunity{}, errors.New("abandon transition requires a reason")
		}
		if transition.SupersededBy != "" {
			return WorkOpportunity{}, errors.New("abandon transition must not set superseded_by")
		}
		if len(reason) > 2048 {
			return WorkOpportunity{}, errors.New("abandon reason exceeds byte limit")
		}
		next.Status = OpportunityAbandoned
		next.AbandonReason = reason
	case OppEventSupersede:
		if !current.Status.Active() {
			return WorkOpportunity{}, errors.New("only active work opportunities may be superseded")
		}
		if transition.SupersededBy == "" || transition.SupersededBy == current.ID {
			return WorkOpportunity{}, errors.New("supersede transition requires a distinct successor")
		}
		detail := "superseded_by:" + string(transition.SupersededBy)
		if reason != "" {
			detail = detail + " " + reason
		}
		if len(detail) > 2048 {
			return WorkOpportunity{}, errors.New("supersede detail exceeds byte limit")
		}
		next.Status = OpportunitySuperseded
		next.AbandonReason = detail
	default:
		return WorkOpportunity{}, fmt.Errorf("unknown work opportunity transition %q", transition.Event)
	}

	if err := next.Validate(); err != nil {
		return WorkOpportunity{}, fmt.Errorf("validate transitioned work opportunity: %w", err)
	}
	return next, nil
}

// FrontierHygieneAction is one pure transition PlanFrontierHygiene decided to apply.
// Kernel/store layers persist each action with SaveWorkOpportunity + audit event.
type FrontierHygieneAction struct {
	OpportunityID WorkOpportunityID
	Transition    WorkOpportunityTransition
}

// EventKindForOpportunityTransition maps a hygiene event to the append-only log kind.
func EventKindForOpportunityTransition(event WorkOpportunityTransitionEvent) (string, error) {
	switch event {
	case OppEventDefer:
		return EventWorkOpportunityDeferred, nil
	case OppEventReopen:
		return EventWorkOpportunityReopened, nil
	case OppEventAbandon:
		return EventWorkOpportunityAbandoned, nil
	case OppEventSupersede:
		return EventWorkOpportunitySuperseded, nil
	default:
		return "", fmt.Errorf("unknown work opportunity transition %q", event)
	}
}

// PlanFrontierHygiene returns deterministic reservoir hygiene actions for OPEN
// opportunities. It never mutates input. Policy-driven rules:
//  1. abandon OPEN units whose depth exceeds MaxDepth (illegal residual growth);
//  2. when remaining OPEN count exceeds MaxCandidates, DEFER lowest-priority
//     (then oldest, then id) units until the active open count fits the mark.
//
// Deferred units stay recoverable via REOPEN; ADMITTED is never touched here.
// Prefer PlanFrontierReservoirHygiene when DEFERRED units and signature merge
// should participate in the same deterministic plan.
func PlanFrontierHygiene(open []WorkOpportunity, policy HorizonPolicy, now time.Time) ([]FrontierHygieneAction, error) {
	return PlanFrontierReservoirHygiene(open, nil, policy, now)
}

// PlanFrontierReservoirHygiene is the full pure hygiene planner for the frontier
// reservoir. It never mutates input and never touches ADMITTED units.
//
// Deterministic order (each step sees simulated residual of prior steps):
//  1. SUPERSEDE exact DedupSignature duplicates among OPEN∪DEFERRED (keep winner);
//  2. ABANDON OPEN units whose depth exceeds MaxDepth;
//  3. DEFER lowest-priority OPEN units when open residual exceeds MaxCandidates;
//  4. REOPEN highest-priority DEFERRED units while open residual is under MaxCandidates.
//
// Winner among a signature group: OPEN before DEFERRED, then higher Priority,
// then newer UpdatedAt, then smaller ID. Model output never selects winners.
func PlanFrontierReservoirHygiene(open, deferred []WorkOpportunity, policy HorizonPolicy, now time.Time) ([]FrontierHygieneAction, error) {
	if err := policy.Validate(); err != nil {
		return nil, fmt.Errorf("validate horizon policy: %w", err)
	}
	if now.IsZero() {
		return nil, errors.New("frontier hygiene requires occurrence time")
	}
	now = now.UTC()

	openPool, err := filterStatus(open, OpportunityOpen)
	if err != nil {
		return nil, err
	}
	deferredPool, err := filterStatus(deferred, OpportunityDeferred)
	if err != nil {
		return nil, err
	}

	var actions []FrontierHygieneAction

	// 1) Signature merge / supersede.
	active := make([]WorkOpportunity, 0, len(openPool)+len(deferredPool))
	active = append(active, openPool...)
	active = append(active, deferredPool...)
	supersedeActions, survivors := planSignatureSupersede(active, now)
	actions = append(actions, supersedeActions...)

	openPool = openPool[:0]
	deferredPool = deferredPool[:0]
	for _, opp := range survivors {
		switch opp.Status {
		case OpportunityOpen:
			openPool = append(openPool, opp)
		case OpportunityDeferred:
			deferredPool = append(deferredPool, opp)
		}
	}

	// 2) Depth abandon on OPEN.
	remainingOpen := make([]WorkOpportunity, 0, len(openPool))
	for _, opp := range openPool {
		if opp.Depth > policy.MaxDepth {
			actions = append(actions, FrontierHygieneAction{
				OpportunityID: opp.ID,
				Transition: WorkOpportunityTransition{
					Event:      OppEventAbandon,
					OccurredAt: now,
					Reason:     fmt.Sprintf("depth_exceeds_policy max_depth=%d depth=%d", policy.MaxDepth, opp.Depth),
				},
			})
			continue
		}
		remainingOpen = append(remainingOpen, opp)
	}

	// 3) Defer excess OPEN under MaxCandidates.
	excess := len(remainingOpen) - policy.MaxCandidates
	if excess > 0 {
		sort.SliceStable(remainingOpen, func(i, j int) bool {
			return opportunityWorseFirst(remainingOpen[i], remainingOpen[j])
		})
		for i := 0; i < excess; i++ {
			opp := remainingOpen[i]
			actions = append(actions, FrontierHygieneAction{
				OpportunityID: opp.ID,
				Transition: WorkOpportunityTransition{
					Event:      OppEventDefer,
					OccurredAt: now,
					Reason:     fmt.Sprintf("max_candidates=%d open_after_depth_trim=%d", policy.MaxCandidates, len(remainingOpen)),
				},
			})
			// Simulate parking for inventory only; same-cycle units are not reopened.
			deferredPool = append(deferredPool, opp)
		}
		remainingOpen = remainingOpen[excess:]
	}

	// 4) Reopen deferred into free OPEN slots (never above MaxCandidates).
	if len(deferredPool) > 0 && len(remainingOpen) < policy.MaxCandidates {
		sort.SliceStable(deferredPool, func(i, j int) bool {
			return opportunityBetterFirst(deferredPool[i], deferredPool[j])
		})
		for _, opp := range deferredPool {
			if len(remainingOpen) >= policy.MaxCandidates {
				break
			}
			// Units deferred in this same plan stay parked until a later cycle.
			if justDeferredIn(actions, opp.ID) {
				continue
			}
			actions = append(actions, FrontierHygieneAction{
				OpportunityID: opp.ID,
				Transition: WorkOpportunityTransition{
					Event:      OppEventReopen,
					OccurredAt: now,
					Reason:     fmt.Sprintf("reopen_under_max_candidates open=%d max_candidates=%d", len(remainingOpen), policy.MaxCandidates),
				},
			})
			remainingOpen = append(remainingOpen, opp)
		}
	}

	return actions, nil
}

func filterStatus(items []WorkOpportunity, want WorkOpportunityStatus) ([]WorkOpportunity, error) {
	out := make([]WorkOpportunity, 0, len(items))
	for _, opp := range items {
		if opp.Status != want {
			continue
		}
		if err := opp.Validate(); err != nil {
			return nil, fmt.Errorf("validate work opportunity %s: %w", opp.ID, err)
		}
		out = append(out, opp)
	}
	return out, nil
}

// planSignatureSupersede groups by exact DedupSignature and supersedes losers.
// Survivors retain their original status (OPEN/DEFERRED).
func planSignatureSupersede(active []WorkOpportunity, now time.Time) ([]FrontierHygieneAction, []WorkOpportunity) {
	if len(active) == 0 {
		return nil, nil
	}
	groups := map[string][]WorkOpportunity{}
	order := make([]string, 0)
	for _, opp := range active {
		sig := opp.DedupSignature
		if _, ok := groups[sig]; !ok {
			order = append(order, sig)
		}
		groups[sig] = append(groups[sig], opp)
	}
	sort.Strings(order)

	var actions []FrontierHygieneAction
	survivors := make([]WorkOpportunity, 0, len(active))
	for _, sig := range order {
		group := groups[sig]
		if len(group) == 1 {
			survivors = append(survivors, group[0])
			continue
		}
		// Winner first: OPEN > DEFERRED, higher priority, newer UpdatedAt, smaller ID.
		sort.SliceStable(group, func(i, j int) bool {
			return opportunityBetterFirst(group[i], group[j])
		})
		winner := group[0]
		survivors = append(survivors, winner)
		for _, loser := range group[1:] {
			actions = append(actions, FrontierHygieneAction{
				OpportunityID: loser.ID,
				Transition: WorkOpportunityTransition{
					Event:        OppEventSupersede,
					OccurredAt:   now,
					SupersededBy: winner.ID,
					Reason:       fmt.Sprintf("duplicate_signature=%s", sig),
				},
			})
		}
	}
	return actions, survivors
}

// opportunityBetterFirst ranks the preferred survivor / reopen candidate.
func opportunityBetterFirst(a, b WorkOpportunity) bool {
	if a.Status != b.Status {
		if a.Status == OpportunityOpen {
			return true
		}
		if b.Status == OpportunityOpen {
			return false
		}
	}
	if a.Priority != b.Priority {
		return a.Priority > b.Priority
	}
	if !a.UpdatedAt.Equal(b.UpdatedAt) {
		return a.UpdatedAt.After(b.UpdatedAt)
	}
	return string(a.ID) < string(b.ID)
}

// opportunityWorseFirst ranks first-to-defer / first-to-lose under pressure.
func opportunityWorseFirst(a, b WorkOpportunity) bool {
	if a.Priority != b.Priority {
		return a.Priority < b.Priority
	}
	if !a.UpdatedAt.Equal(b.UpdatedAt) {
		return a.UpdatedAt.Before(b.UpdatedAt)
	}
	return string(a.ID) < string(b.ID)
}

func justDeferredIn(actions []FrontierHygieneAction, id WorkOpportunityID) bool {
	for _, action := range actions {
		if action.OpportunityID == id && action.Transition.Event == OppEventDefer {
			return true
		}
	}
	return false
}
