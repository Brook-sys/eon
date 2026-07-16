package kernel

import (
	"reflect"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
)

func blockingQuestion() domain.OperatorQuestion {
	q := gateProposal(time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)).Question
	q.BlockingScope = []domain.QuestionBlockingTarget{{Kind: domain.QuestionBlockingOperation, Reference: "operation_1"}}
	return q
}

func readyOperation(id domain.OperationID) domain.Operation {
	return domain.Operation{
		SchemaVersion: domain.SchemaVersionV1, ID: id, InquiryID: "inquiry_1", MissionRevision: "revision_1", SpecID: "spec_1",
		ReadSet: []string{"read"}, InputRefs: []string{"input"}, ExpectedOutput: "output", IdempotencyKey: domain.IdempotencyKey("idem:" + id),
		State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
	}
}

func TestQuestionWaitBlocksOnlyDeclaredOperation(t *testing.T) {
	question := blockingQuestion()
	operations := []domain.Operation{readyOperation("operation_1"), readyOperation("operation_2")}
	before := cloneOperations(operations)
	blocked, err := ApplyQuestionWait(question, operations)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(operations, before) {
		t.Fatal("input operations were mutated")
	}
	if blocked[0].State != domain.StateWaitingEvent || blocked[0].Reevaluation.EventType != OperatorQuestionAnsweredEventType || blocked[0].Reevaluation.Reference != string(question.ID) {
		t.Fatalf("target = %#v", blocked[0])
	}
	if !reflect.DeepEqual(blocked[1], operations[1]) {
		t.Fatalf("independent operation changed: %#v", blocked[1])
	}
}

func TestQuestionResolutionResumesOnlyMatchingWait(t *testing.T) {
	question := blockingQuestion()
	blocked, err := ApplyQuestionWait(question, []domain.Operation{readyOperation("operation_1"), readyOperation("operation_2")})
	if err != nil {
		t.Fatal(err)
	}
	other := readyOperation("operation_3")
	snapshot, err := domain.Transition(domain.OperationalSnapshot{State: other.State, Reevaluation: other.Reevaluation}, domain.TransitionInput{Event: domain.EventWaitEvent, EventType: OperatorQuestionAnsweredEventType, Reference: "ask_other"})
	if err != nil {
		t.Fatal(err)
	}
	other.State, other.Reevaluation = snapshot.State, snapshot.Reevaluation
	blocked = append(blocked, other)
	terminal, err := domain.TransitionOperatorQuestion(question, domain.OperatorQuestionTransition{Event: domain.QuestionEventCancel, OccurredAt: question.CreatedAt.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := ResumeQuestionWait(terminal, blocked)
	if err != nil {
		t.Fatal(err)
	}
	if resumed[0].State != domain.StateReady || resumed[0].Reevaluation.Kind != domain.ReevaluateReady {
		t.Fatalf("matching operation not resumed: %#v", resumed[0])
	}
	if resumed[1].State != domain.StateReady {
		t.Fatalf("independent ready operation changed: %#v", resumed[1])
	}
	if !reflect.DeepEqual(resumed[2], other) {
		t.Fatalf("unrelated question wait changed: %#v", resumed[2])
	}
}

func TestQuestionWaitFailsClosedForMissingOrForeignTarget(t *testing.T) {
	question := blockingQuestion()
	if _, err := ApplyQuestionWait(question, []domain.Operation{readyOperation("operation_2")}); err == nil {
		t.Fatal("missing blocking target accepted")
	}
	foreign := readyOperation("operation_1")
	foreign.MissionRevision = "revision_2"
	if _, err := ApplyQuestionWait(question, []domain.Operation{foreign}); err == nil {
		t.Fatal("foreign mission target accepted")
	}
}
