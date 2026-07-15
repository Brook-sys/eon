package domain

import (
	"errors"
	"fmt"
	"time"
)

// OperationalState is execution state, not epistemic acceptance state.
type OperationalState string

const (
	StateNew               OperationalState = "NEW"
	StateReady             OperationalState = "READY"
	StateRunning           OperationalState = "RUNNING"
	StateVerifying         OperationalState = "VERIFYING"
	StateWaitingTime       OperationalState = "WAITING_TIME"
	StateWaitingEvent      OperationalState = "WAITING_EVENT"
	StateWaitingApproval   OperationalState = "WAITING_APPROVAL"
	StateThrottled         OperationalState = "THROTTLED"
	StateBlockedDependency OperationalState = "BLOCKED_DEPENDENCY"
	StateReplanning        OperationalState = "REPLANNING"
	StateSucceeded         OperationalState = "SUCCEEDED"
	StateExhausted         OperationalState = "EXHAUSTED"
	StateFailed            OperationalState = "FAILED"
	StateCancelled         OperationalState = "CANCELLED"
)

func (s OperationalState) Valid() bool {
	switch s {
	case StateNew, StateReady, StateRunning, StateVerifying, StateWaitingTime,
		StateWaitingEvent, StateWaitingApproval, StateThrottled,
		StateBlockedDependency, StateReplanning, StateSucceeded,
		StateExhausted, StateFailed, StateCancelled:
		return true
	default:
		return false
	}
}

func (s OperationalState) Terminal() bool {
	switch s {
	case StateSucceeded, StateExhausted, StateFailed, StateCancelled:
		return true
	default:
		return false
	}
}

// ReevaluationKind makes the wake-up condition of every non-terminal unit
// explicit and serializable (FR-DUR-001, INV-DUR-001).
type ReevaluationKind string

const (
	ReevaluateNormalize  ReevaluationKind = "NORMALIZE"
	ReevaluateReady      ReevaluationKind = "READY"
	ReevaluateLease      ReevaluationKind = "LEASE"
	ReevaluateNotBefore  ReevaluationKind = "NOT_BEFORE"
	ReevaluateEvent      ReevaluationKind = "EVENT"
	ReevaluateApproval   ReevaluationKind = "APPROVAL"
	ReevaluateCapacity   ReevaluationKind = "CAPACITY"
	ReevaluateDependency ReevaluationKind = "DEPENDENCY"
	ReevaluateReplanning ReevaluationKind = "REPLANNING"
)

// ReevaluationCondition uses only the fields selected by Kind. It is persisted
// with Inquiry and Operation rather than reconstructed from process memory.
type ReevaluationCondition struct {
	Kind      ReevaluationKind `json:"kind"`
	NotBefore *time.Time       `json:"not_before,omitempty"`
	EventType string           `json:"event_type,omitempty"`
	Reference string           `json:"reference,omitempty"`
}

func (c ReevaluationCondition) ValidateFor(state OperationalState) error {
	if !state.Valid() {
		return fmt.Errorf("unknown operational state %q", state)
	}
	if state.Terminal() {
		if c != (ReevaluationCondition{}) {
			return errors.New("terminal state must not have a reevaluation condition")
		}
		return nil
	}

	expected := map[OperationalState]ReevaluationKind{
		StateNew:               ReevaluateNormalize,
		StateReady:             ReevaluateReady,
		StateRunning:           ReevaluateLease,
		StateVerifying:         ReevaluateLease,
		StateWaitingTime:       ReevaluateNotBefore,
		StateWaitingEvent:      ReevaluateEvent,
		StateWaitingApproval:   ReevaluateApproval,
		StateThrottled:         ReevaluateCapacity,
		StateBlockedDependency: ReevaluateDependency,
		StateReplanning:        ReevaluateReplanning,
	}
	if c.Kind != expected[state] {
		return fmt.Errorf("state %s requires reevaluation kind %s, got %s", state, expected[state], c.Kind)
	}
	if c.Kind == ReevaluateNotBefore && c.NotBefore == nil {
		return errors.New("NOT_BEFORE condition requires an instant")
	}
	if c.Kind != ReevaluateNotBefore && c.NotBefore != nil {
		return errors.New("only NOT_BEFORE condition may carry an instant")
	}
	if c.Kind == ReevaluateEvent && c.EventType == "" {
		return errors.New("EVENT condition requires an event type")
	}
	requiresReference := map[ReevaluationKind]bool{
		ReevaluateLease:      true,
		ReevaluateApproval:   true,
		ReevaluateCapacity:   true,
		ReevaluateDependency: true,
		ReevaluateReplanning: true,
	}
	if requiresReference[c.Kind] && c.Reference == "" {
		return fmt.Errorf("%s condition requires a reference", c.Kind)
	}
	return nil
}
