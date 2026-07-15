package domain

import (
	"reflect"
	"testing"
	"time"
)

func TestTransitionLegalPaths(t *testing.T) {
	now := time.Date(2026, 7, 15, 13, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		from  OperationalSnapshot
		input TransitionInput
		want  OperationalSnapshot
	}{
		{
			name:  "new normalizes to ready",
			from:  snapshotFor(StateNew, "ingest"),
			input: TransitionInput{Event: EventNormalize},
			want:  snapshotFor(StateReady, ""),
		},
		{
			name:  "ready dispatches under lease",
			from:  snapshotFor(StateReady, ""),
			input: TransitionInput{Event: EventDispatch, Reference: "lease_1"},
			want:  snapshotFor(StateRunning, "lease_1"),
		},
		{
			name:  "running enters verification",
			from:  snapshotFor(StateRunning, "lease_1"),
			input: TransitionInput{Event: EventBeginVerify, Reference: "lease_1"},
			want:  snapshotFor(StateVerifying, "lease_1"),
		},
		{
			name:  "verified succeeds",
			from:  snapshotFor(StateVerifying, "lease_1"),
			input: TransitionInput{Event: EventSucceed},
			want:  terminalSnapshot(StateSucceeded),
		},
		{
			name:  "running waits until explicit instant",
			from:  snapshotFor(StateRunning, "lease_1"),
			input: TransitionInput{Event: EventWaitUntil, NotBefore: &now, Reference: "retry-policy-v1"},
			want:  OperationalSnapshot{State: StateWaitingTime, Reevaluation: ReevaluationCondition{Kind: ReevaluateNotBefore, NotBefore: &now, Reference: "retry-policy-v1"}},
		},
		{
			name:  "ready waits for event",
			from:  snapshotFor(StateReady, ""),
			input: TransitionInput{Event: EventWaitEvent, EventType: "source.available", Reference: "source_1"},
			want:  OperationalSnapshot{State: StateWaitingEvent, Reevaluation: ReevaluationCondition{Kind: ReevaluateEvent, EventType: "source.available", Reference: "source_1"}},
		},
		{
			name:  "dependency resolution resumes ready",
			from:  snapshotFor(StateBlockedDependency, "operation_2"),
			input: TransitionInput{Event: EventResume},
			want:  snapshotFor(StateReady, ""),
		},
		{
			name:  "known non-effect permits retry",
			from:  snapshotFor(StateVerifying, "lease_1"),
			input: TransitionInput{Event: EventRetry, EffectState: EffectNotApplied},
			want:  snapshotFor(StateReady, ""),
		},
		{
			name:  "uncertain effect enters reconciliation",
			from:  snapshotFor(StateRunning, "lease_1"),
			input: TransitionInput{Event: EventReconcile, Reference: "receipt_lookup", EffectState: EffectUnknown},
			want:  snapshotFor(StateReplanning, "receipt_lookup"),
		},
		{
			name:  "cancel is accepted from any nonterminal state",
			from:  snapshotFor(StateWaitingApproval, "approval_1"),
			input: TransitionInput{Event: EventCancel},
			want:  terminalSnapshot(StateCancelled),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Transition(test.from, test.input)
			if err != nil {
				t.Fatalf("Transition() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Transition() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestTransitionRejectsIllegalOrUnsafeChanges(t *testing.T) {
	tests := []struct {
		name  string
		from  OperationalSnapshot
		input TransitionInput
	}{
		{name: "cannot dispatch terminal", from: terminalSnapshot(StateSucceeded), input: TransitionInput{Event: EventDispatch}},
		{name: "cannot succeed without verification", from: snapshotFor(StateRunning, "lease_1"), input: TransitionInput{Event: EventSucceed}},
		{name: "cannot resume ready", from: snapshotFor(StateReady, ""), input: TransitionInput{Event: EventResume}},
		{name: "wait until requires instant", from: snapshotFor(StateRunning, "lease_1"), input: TransitionInput{Event: EventWaitUntil}},
		{name: "event wait requires event type", from: snapshotFor(StateReady, ""), input: TransitionInput{Event: EventWaitEvent}},
		{name: "unknown effect cannot retry", from: snapshotFor(StateVerifying, "lease_1"), input: TransitionInput{Event: EventRetry, EffectState: EffectUnknown}},
		{name: "partial effect cannot retry", from: snapshotFor(StateVerifying, "lease_1"), input: TransitionInput{Event: EventRetry, EffectState: EffectPartial}},
		{name: "applied effect cannot retry", from: snapshotFor(StateVerifying, "lease_1"), input: TransitionInput{Event: EventRetry, EffectState: EffectApplied}},
		{name: "known non-effect cannot reconcile", from: snapshotFor(StateRunning, "lease_1"), input: TransitionInput{Event: EventReconcile, EffectState: EffectNotApplied}},
		{name: "unrelated event rejects effect state", from: snapshotFor(StateReady, ""), input: TransitionInput{Event: EventDispatch, EffectState: EffectNotApplied}},
		{name: "unrelated event rejects instant", from: snapshotFor(StateReady, ""), input: TransitionInput{Event: EventDispatch, NotBefore: ptrTime(time.Now())}},
		{name: "invalid current snapshot fails closed", from: OperationalSnapshot{State: StateWaitingEvent, Reevaluation: ReevaluationCondition{Kind: ReevaluateReady}}, input: TransitionInput{Event: EventResume}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Transition(test.from, test.input); err == nil {
				t.Fatal("Transition() accepted an illegal or unsafe transition")
			}
		})
	}
}

func ptrTime(value time.Time) *time.Time { return &value }

func TestTransitionIsPure(t *testing.T) {
	from := snapshotFor(StateRunning, "lease_1")
	before := from
	_, err := Transition(from, TransitionInput{Event: EventRequestReplan, Reference: "failure_1"})
	if err != nil {
		t.Fatalf("Transition() error = %v", err)
	}
	if !reflect.DeepEqual(from, before) {
		t.Fatalf("Transition() mutated input: got %#v, want %#v", from, before)
	}
}
