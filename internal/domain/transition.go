package domain

import (
	"errors"
	"fmt"
	"time"
)

// TransitionEvent is a validated kernel fact, not an instruction emitted by a
// model. Transition is pure: callers persist the returned snapshot separately.
type TransitionEvent string

const (
	EventNormalize       TransitionEvent = "NORMALIZE"
	EventDispatch        TransitionEvent = "DISPATCH"
	EventBeginVerify     TransitionEvent = "BEGIN_VERIFY"
	EventSucceed         TransitionEvent = "SUCCEED"
	EventWaitUntil       TransitionEvent = "WAIT_UNTIL"
	EventWaitEvent       TransitionEvent = "WAIT_EVENT"
	EventAwaitApproval   TransitionEvent = "AWAIT_APPROVAL"
	EventThrottle        TransitionEvent = "THROTTLE"
	EventBlockDependency TransitionEvent = "BLOCK_DEPENDENCY"
	EventRequestReplan   TransitionEvent = "REQUEST_REPLAN"
	EventResume          TransitionEvent = "RESUME"
	EventRetry           TransitionEvent = "RETRY"
	EventReconcile       TransitionEvent = "RECONCILE"
	EventExhaust         TransitionEvent = "EXHAUST"
	EventFail            TransitionEvent = "FAIL"
	EventCancel          TransitionEvent = "CANCEL"
)

// TransitionInput contains only facts needed to compute the next operational
// snapshot. EffectState is required by RETRY so ambiguous effects cannot be
// made READY accidentally (FR-DUR-006, INV-DUR-003).
type TransitionInput struct {
	Event       TransitionEvent
	NotBefore   *time.Time
	EventType   string
	Reference   string
	EffectState EffectState
}

type OperationalSnapshot struct {
	State        OperationalState
	Reevaluation ReevaluationCondition
}

// Transition computes one legal state change without reading time, storage or
// process-local state. The caller is responsible for validating success
// criteria before issuing SUCCEED and for atomically persisting the result.
func Transition(current OperationalSnapshot, input TransitionInput) (OperationalSnapshot, error) {
	if err := current.Reevaluation.ValidateFor(current.State); err != nil {
		return OperationalSnapshot{}, fmt.Errorf("invalid current snapshot: %w", err)
	}
	if err := input.validate(); err != nil {
		return OperationalSnapshot{}, fmt.Errorf("invalid transition input: %w", err)
	}
	if current.State.Terminal() {
		return OperationalSnapshot{}, fmt.Errorf("terminal state %s cannot transition", current.State)
	}

	if input.Event == EventCancel {
		return terminalSnapshot(StateCancelled), nil
	}
	if input.Event == EventFail {
		return terminalSnapshot(StateFailed), nil
	}
	if input.Event == EventExhaust {
		return terminalSnapshot(StateExhausted), nil
	}

	if next, ok := waitingTransition(input); ok {
		if !canEnterWaitingState(current.State) {
			return OperationalSnapshot{}, fmt.Errorf("event %s is not legal from %s", input.Event, current.State)
		}
		if err := next.Reevaluation.ValidateFor(next.State); err != nil {
			return OperationalSnapshot{}, fmt.Errorf("invalid transition payload: %w", err)
		}
		return next, nil
	}

	nextState, ok := directTransitions[transitionKey{current.State, input.Event}]
	if !ok {
		return OperationalSnapshot{}, fmt.Errorf("event %s is not legal from %s", input.Event, current.State)
	}
	next := snapshotFor(nextState, input.Reference)
	if err := next.Reevaluation.ValidateFor(next.State); err != nil {
		return OperationalSnapshot{}, fmt.Errorf("computed invalid snapshot: %w", err)
	}
	return next, nil
}

func (input TransitionInput) validate() error {
	if input.Event == "" {
		return errors.New("transition event is required")
	}
	if input.Event != EventWaitUntil && input.NotBefore != nil {
		return errors.New("only WAIT_UNTIL may carry not_before")
	}
	if input.Event != EventWaitEvent && input.EventType != "" {
		return errors.New("only WAIT_EVENT may carry event_type")
	}
	switch input.Event {
	case EventRetry:
		switch input.EffectState {
		case EffectNotStarted, EffectNotApplied:
			return nil
		default:
			return errors.New("retry requires a known non-effect; UNKNOWN or PARTIAL must reconcile first")
		}
	case EventReconcile:
		switch input.EffectState {
		case EffectUnknown, EffectPartial:
			return nil
		default:
			return errors.New("reconciliation requires UNKNOWN or PARTIAL effect")
		}
	default:
		if input.EffectState != "" {
			return errors.New("effect state is only valid for RETRY or RECONCILE")
		}
	}
	return nil
}

type transitionKey struct {
	state OperationalState
	event TransitionEvent
}

var directTransitions = map[transitionKey]OperationalState{
	{StateNew, EventNormalize}:            StateReady,
	{StateReady, EventDispatch}:           StateRunning,
	{StateRunning, EventBeginVerify}:      StateVerifying,
	{StateRunning, EventRequestReplan}:    StateReplanning,
	{StateRunning, EventReconcile}:        StateReplanning,
	{StateVerifying, EventSucceed}:        StateSucceeded,
	{StateVerifying, EventRetry}:          StateReady,
	{StateVerifying, EventRequestReplan}:  StateReplanning,
	{StateVerifying, EventReconcile}:      StateReplanning,
	{StateWaitingTime, EventResume}:       StateReady,
	{StateWaitingEvent, EventResume}:      StateReady,
	{StateWaitingApproval, EventResume}:   StateReady,
	{StateThrottled, EventResume}:         StateReady,
	{StateBlockedDependency, EventResume}: StateReady,
	{StateReplanning, EventResume}:        StateReady,
}

func canEnterWaitingState(state OperationalState) bool {
	switch state {
	case StateReady, StateRunning, StateVerifying, StateReplanning:
		return true
	default:
		return false
	}
}

func waitingTransition(input TransitionInput) (OperationalSnapshot, bool) {
	switch input.Event {
	case EventWaitUntil:
		return OperationalSnapshot{
			State:        StateWaitingTime,
			Reevaluation: ReevaluationCondition{Kind: ReevaluateNotBefore, NotBefore: input.NotBefore, Reference: input.Reference},
		}, true
	case EventWaitEvent:
		return OperationalSnapshot{
			State:        StateWaitingEvent,
			Reevaluation: ReevaluationCondition{Kind: ReevaluateEvent, EventType: input.EventType, Reference: input.Reference},
		}, true
	case EventAwaitApproval:
		return snapshotFor(StateWaitingApproval, input.Reference), true
	case EventThrottle:
		return snapshotFor(StateThrottled, input.Reference), true
	case EventBlockDependency:
		return snapshotFor(StateBlockedDependency, input.Reference), true
	default:
		return OperationalSnapshot{}, false
	}
}

func snapshotFor(state OperationalState, reference string) OperationalSnapshot {
	if state.Terminal() {
		return terminalSnapshot(state)
	}
	kinds := map[OperationalState]ReevaluationKind{
		StateNew:               ReevaluateNormalize,
		StateReady:             ReevaluateReady,
		StateRunning:           ReevaluateLease,
		StateVerifying:         ReevaluateLease,
		StateWaitingApproval:   ReevaluateApproval,
		StateThrottled:         ReevaluateCapacity,
		StateBlockedDependency: ReevaluateDependency,
		StateReplanning:        ReevaluateReplanning,
	}
	return OperationalSnapshot{
		State: state,
		Reevaluation: ReevaluationCondition{
			Kind:      kinds[state],
			Reference: reference,
		},
	}
}

func terminalSnapshot(state OperationalState) OperationalSnapshot {
	return OperationalSnapshot{State: state}
}
