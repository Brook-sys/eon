package domain

import (
	"testing"
	"time"
)

func TestReevaluationConditionValidateFor(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		state     OperationalState
		condition ReevaluationCondition
		wantError bool
	}{
		{name: "ready", state: StateReady, condition: ReevaluationCondition{Kind: ReevaluateReady}},
		{name: "temporal wait", state: StateWaitingTime, condition: ReevaluationCondition{Kind: ReevaluateNotBefore, NotBefore: &now}},
		{name: "event wait", state: StateWaitingEvent, condition: ReevaluationCondition{Kind: ReevaluateEvent, EventType: "source.available"}},
		{name: "terminal has no wakeup", state: StateSucceeded},
		{name: "terminal rejects wakeup", state: StateFailed, condition: ReevaluationCondition{Kind: ReevaluateReady}, wantError: true},
		{name: "time missing", state: StateWaitingTime, condition: ReevaluationCondition{Kind: ReevaluateNotBefore}, wantError: true},
		{name: "wrong kind", state: StateThrottled, condition: ReevaluationCondition{Kind: ReevaluateReady}, wantError: true},
		{name: "unknown state", state: OperationalState("UNKNOWN"), wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.condition.ValidateFor(test.state)
			if (err != nil) != test.wantError {
				t.Fatalf("ValidateFor() error = %v, wantError %v", err, test.wantError)
			}
		})
	}
}

func TestInquiryValidationRejectsOrphanedNonTerminalState(t *testing.T) {
	inquiry := Inquiry{
		SchemaVersion: SchemaVersionV1,
		ID:            "inquiry_1", CandidateID: "candidate_1", MissionRevision: "revision_1",
		QuestionID: "question_1", AdmissionReason: "mission priority", StopCondition: "answered",
		State: StateWaitingEvent,
	}
	if err := inquiry.Validate(); err == nil {
		t.Fatal("Validate() accepted a non-terminal inquiry without its event condition")
	}
}

func TestObservationRequiresExactlyOneAnchor(t *testing.T) {
	base := Observation{SchemaVersion: SchemaVersionV1, ID: "observation_1", Statement: "source states X", Provenance: "extractor@1"}
	tests := []struct {
		name      string
		anchor    ObservationAnchor
		wantError bool
	}{
		{name: "fragment", anchor: ObservationAnchor{SourceFragmentID: "fragment_1"}},
		{name: "receipt", anchor: ObservationAnchor{ReceiptID: "receipt_1"}},
		{name: "neither", wantError: true},
		{name: "both", anchor: ObservationAnchor{SourceFragmentID: "fragment_1", ReceiptID: "receipt_1"}, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observation := base
			observation.Anchor = test.anchor
			err := observation.Validate()
			if (err != nil) != test.wantError {
				t.Fatalf("Validate() error = %v, wantError %v", err, test.wantError)
			}
		})
	}
}

func TestEvidenceLinkRejectsUnknownRelation(t *testing.T) {
	link := EvidenceLink{
		SchemaVersion: SchemaVersionV1,
		ID:            "evidence_1", ObservationID: "observation_1", ClaimID: "claim_1",
		Relation: EvidenceRelation("TRUSTS"),
	}
	if err := link.Validate(); err == nil {
		t.Fatal("Validate() accepted an unknown evidence relation")
	}
}
