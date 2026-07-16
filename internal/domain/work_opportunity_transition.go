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
func PlanFrontierHygiene(open []WorkOpportunity, policy HorizonPolicy, now time.Time) ([]FrontierHygieneAction, error) {
	if err := policy.Validate(); err != nil {
		return nil, fmt.Errorf("validate horizon policy: %w", err)
	}
	if now.IsZero() {
		return nil, errors.New("frontier hygiene requires occurrence time")
	}
	now = now.UTC()

	var actions []FrontierHygieneAction
	remaining := make([]WorkOpportunity, 0, len(open))
	for _, opp := range open {
		if opp.Status != OpportunityOpen {
			// Plan only OPEN; deferred already parked, admitted is agenda.
			continue
		}
		if err := opp.Validate(); err != nil {
			return nil, fmt.Errorf("validate work opportunity %s: %w", opp.ID, err)
		}
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
		remaining = append(remaining, opp)
	}

	excess := len(remaining) - policy.MaxCandidates
	if excess <= 0 {
		return actions, nil
	}

	// Stable order: lowest priority first, then oldest UpdatedAt, then ID.
	sort.SliceStable(remaining, func(i, j int) bool {
		a, b := remaining[i], remaining[j]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.UpdatedAt.Before(b.UpdatedAt)
		}
		return string(a.ID) < string(b.ID)
	})

	for i := 0; i < excess; i++ {
		opp := remaining[i]
		actions = append(actions, FrontierHygieneAction{
			OpportunityID: opp.ID,
			Transition: WorkOpportunityTransition{
				Event:      OppEventDefer,
				OccurredAt: now,
				Reason:     fmt.Sprintf("max_candidates=%d open_after_depth_trim=%d", policy.MaxCandidates, len(remaining)),
			},
		})
	}
	return actions, nil
}
