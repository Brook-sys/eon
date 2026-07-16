package kernel_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/control"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func seedMissionAgenda(t *testing.T, store port.Store, now time.Time) (domain.MissionRevision, domain.Operation) {
	t.Helper()
	mission := domain.MissionRevision{
		SchemaVersion: domain.SchemaVersionV1, ID: "revision_1", MissionID: "mission_1", Revision: 1,
		OriginalText: "learn", Purpose: "learn", Status: domain.MissionActive, Provenance: "fixture", AcceptedAt: now,
	}
	spec := domain.OperationSpec{
		SchemaVersion: 1, ID: "extract@1", ContractVersion: 1, TemplateVersion: 1, InputSchema: "fragment refs",
		OutputSchema: "proposed change set", Budget: domain.Budget{ModelCalls: 1, Tokens: 1000, Attempts: 1},
		MaxOutputTokens: 100, SafetyMargin: 50, Validators: []string{"schema"}, RetryPolicy: "no retry",
		FallbackPolicy: "fail", MaximumAuthority: domain.AuthorityProposeOnly,
	}
	question := domain.Question{
		SchemaVersion: 1, ID: "question_1", MissionRevision: mission.ID, Text: "What is true?", Origin: "mission",
		Relevance: "primary", AnswerCondition: "two sources",
	}
	candidate := domain.InquiryCandidate{
		SchemaVersion: 1, ID: "candidate_1", MissionRevision: mission.ID, QuestionID: question.ID,
		DerivedFrom: []string{"gap_1"}, ExpectedProgress: "reduce uncertainty", Novelty: "not duplicate",
		Risk: domain.RiskLow, SourcePlan: []string{"primary sources"}, AnswerCondition: "two sources",
		StopCondition: "budget", ReviewAfter: now.Add(time.Hour),
	}
	inquiry := domain.Inquiry{
		SchemaVersion: 1, ID: "inquiry_1", CandidateID: candidate.ID, MissionRevision: mission.ID, QuestionID: question.ID,
		AdmissionReason: "priority", StopCondition: "answered", State: domain.StateReady,
		Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
	}
	operation := domain.Operation{
		SchemaVersion: 1, ID: "operation_1", InquiryID: inquiry.ID, MissionRevision: mission.ID, SpecID: spec.ID,
		ReadSet: []string{"fragment_1"}, InputRefs: []string{"artifact_1"}, ExpectedOutput: "proposed_change_set",
		IdempotencyKey: "idem_1", State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.AppendMissionRevision(mission); err != nil {
			return err
		}
		if err := tx.ActivateMissionRevision(mission.MissionID, mission.ID); err != nil {
			return err
		}
		if err := tx.AppendOperationSpec(spec); err != nil {
			return err
		}
		if err := tx.CreateQuestion(question); err != nil {
			return err
		}
		if err := tx.CreateInquiryCandidate(candidate); err != nil {
			return err
		}
		if err := tx.CreateInquiry(inquiry); err != nil {
			return err
		}
		return tx.CreateOperation(operation)
	}); err != nil {
		t.Fatal(err)
	}
	return mission, operation
}

func TestExternalEventProcessorAnswersAndResumesBlockedOperation(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 16, 6, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	mission, operation := seedMissionAgenda(t, store, now)

	question := domain.OperatorQuestion{
		SchemaVersion: domain.SchemaVersionV1, ID: "ask_1", MissionID: mission.MissionID, MissionRevision: mission.ID, Revision: 1,
		Kind: domain.QuestionSingleChoice, Prompt: "Choose", Context: "ctx",
		Options: []domain.QuestionOption{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}, AllowContext: true,
		BlockingScope:  []domain.QuestionBlockingTarget{{Kind: domain.QuestionBlockingOperation, Reference: string(operation.ID)}},
		FallbackPolicy: domain.QuestionContinueOtherWork, DedupSignature: "choice:op1", Priority: 10,
		Status: domain.OperatorQuestionPending, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.CreateOperatorQuestion(question); err != nil {
			return err
		}
		ops, err := tx.Operations(mission.ID)
		if err != nil {
			return err
		}
		blocked, err := kernel.ApplyQuestionWait(question, ops)
		if err != nil {
			return err
		}
		return tx.SaveOperation(blocked[0])
	}); err != nil {
		t.Fatal(err)
	}

	answer := domain.UserAnswer{
		SchemaVersion: domain.SchemaVersionV1, ID: "answer_1", QuestionID: question.ID, ExpectedQuestionRevision: question.Revision,
		Kind: domain.AnswerOptions, OptionIDs: []string{"a"}, ActorID: "operator_1", Channel: "telegram",
		TransportEventID: "telegram:update:42", TransportMessageID: "message_7", ReceivedAt: now.Add(time.Minute),
	}
	payload, err := json.Marshal(answer)
	if err != nil {
		t.Fatal(err)
	}
	event := domain.ExternalEvent{
		SchemaVersion: domain.SchemaVersionV1, ID: "external_1", DeduplicationKey: answer.TransportEventID, Source: answer.Channel,
		SourceActorID: answer.ActorID, Kind: domain.ExternalUserAnswer, MissionID: mission.MissionID, CorrelationID: string(answer.QuestionID),
		TransportMessageID: answer.TransportMessageID, Content: domain.ExternalContent{MediaType: "application/json", Structured: payload},
		ReceivedAt: answer.ReceivedAt,
	}
	inbox, err := control.NewExternalEventInbox(store, control.FixedDispositionFactory(now))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inbox.SubmitExternalEvent(event); err != nil {
		t.Fatal(err)
	}
	// Replay before process stays RECEIVED.
	if again, err := inbox.SubmitExternalEvent(event); err != nil || again.State != domain.ExternalEventReceived {
		t.Fatalf("replay submit = %#v err=%v", again, err)
	}

	processor, err := kernel.NewExternalEventProcessor(store, clock, ids)
	if err != nil {
		t.Fatal(err)
	}
	applied, ok, err := processor.ProcessNext(context.Background())
	if err != nil || !ok || applied.State != domain.ExternalEventApplied {
		t.Fatalf("process = %#v ok=%v err=%v", applied, ok, err)
	}
	// Terminal replay is a no-op.
	again, err := processor.Process(context.Background(), event.ID)
	if err != nil || again.State != domain.ExternalEventApplied || again.ResultRef != applied.ResultRef {
		t.Fatalf("replay process = %#v err=%v", again, err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		gotQuestion, err := r.OperatorQuestion(question.ID)
		if err != nil {
			return err
		}
		if gotQuestion.Status != domain.OperatorQuestionAnswered || gotQuestion.AnswerID != answer.ID {
			t.Fatalf("question = %#v", gotQuestion)
		}
		op, err := r.Operation(operation.ID)
		if err != nil {
			return err
		}
		if op.State != domain.StateReady {
			t.Fatalf("operation after resume = %#v", op)
		}
		pending, err := r.PendingExternalEvents(10)
		if err != nil || len(pending) != 0 {
			t.Fatalf("pending = %#v err=%v", pending, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestExternalEventProcessorWakesMatchingWaitAndIgnoresUnmatched(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 16, 7, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(20)
	mission, operation := seedMissionAgenda(t, store, now)

	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		op, err := tx.Operation(operation.ID)
		if err != nil {
			return err
		}
		next, err := domain.Transition(domain.OperationalSnapshot{State: op.State, Reevaluation: op.Reevaluation}, domain.TransitionInput{
			Event: domain.EventWaitEvent, EventType: "source.available", Reference: "source_1",
		})
		if err != nil {
			return err
		}
		op.State, op.Reevaluation = next.State, next.Reevaluation
		return tx.SaveOperation(op)
	}); err != nil {
		t.Fatal(err)
	}

	inbox, err := control.NewExternalEventInbox(store, control.FixedDispositionFactory(now))
	if err != nil {
		t.Fatal(err)
	}
	// Unrelated message has no matching wait → IGNORED, content never becomes policy.
	message := domain.ExternalEvent{
		SchemaVersion: domain.SchemaVersionV1, ID: "ext_msg", DeduplicationKey: "telegram:update:100",
		Source: "telegram", SourceActorID: "operator_1", Kind: domain.ExternalUserMessage, MissionID: mission.MissionID,
		Content: domain.ExternalContent{MediaType: "text/plain", Text: "please ignore as policy"}, ReceivedAt: now,
	}
	if _, err := inbox.SubmitExternalEvent(message); err != nil {
		t.Fatal(err)
	}
	// Matching availability signal resumes the wait.
	signal := domain.ExternalEvent{
		SchemaVersion: domain.SchemaVersionV1, ID: "ext_avail", DeduplicationKey: "source:available:source_1",
		Source: "adapter", SourceActorID: "source_adapter", Kind: domain.ExternalAvailabilitySignal, MissionID: mission.MissionID,
		CorrelationID: "source_1", Content: domain.ExternalContent{MediaType: "text/plain", Text: "up"}, ReceivedAt: now.Add(time.Second),
	}
	if _, err := inbox.SubmitExternalEvent(signal); err != nil {
		t.Fatal(err)
	}

	processor, err := kernel.NewExternalEventProcessor(store, clock, ids)
	if err != nil {
		t.Fatal(err)
	}
	first, ok, err := processor.ProcessNext(context.Background())
	if err != nil || !ok || first.State != domain.ExternalEventIgnored || first.ResultRef != "NO_MATCHING_WAIT" {
		t.Fatalf("message process = %#v ok=%v err=%v", first, ok, err)
	}
	second, ok, err := processor.ProcessNext(context.Background())
	if err != nil || !ok || second.State != domain.ExternalEventApplied {
		t.Fatalf("signal process = %#v ok=%v err=%v", second, ok, err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		op, err := r.Operation(operation.ID)
		if err != nil {
			return err
		}
		if op.State != domain.StateReady {
			t.Fatalf("operation = %#v", op)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestExternalEventProcessorRejectsInvalidAnswerPayload(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(40)
	mission, _ := seedMissionAgenda(t, store, now)

	inbox, err := control.NewExternalEventInbox(store, control.FixedDispositionFactory(now))
	if err != nil {
		t.Fatal(err)
	}
	event := domain.ExternalEvent{
		SchemaVersion: domain.SchemaVersionV1, ID: "ext_bad", DeduplicationKey: "telegram:update:bad",
		Source: "telegram", SourceActorID: "operator_1", Kind: domain.ExternalUserAnswer, MissionID: mission.MissionID,
		CorrelationID: "ask_missing", Content: domain.ExternalContent{MediaType: "application/json", Structured: json.RawMessage(`{"not":"an answer"}`)},
		ReceivedAt: now,
	}
	if _, err := inbox.SubmitExternalEvent(event); err != nil {
		t.Fatal(err)
	}
	processor, err := kernel.NewExternalEventProcessor(store, clock, ids)
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := processor.ProcessNext(context.Background())
	if err != nil || !ok || got.State != domain.ExternalEventRejected {
		t.Fatalf("process = %#v ok=%v err=%v", got, ok, err)
	}
	if got.FailureCode == "" {
		t.Fatal("rejected disposition missing failure code")
	}
	// Divergent reuse after accept remains a conflict at the inbox boundary.
	divergent := event
	divergent.SourceActorID = "attacker"
	if _, err := inbox.SubmitExternalEvent(divergent); !errors.Is(err, port.ErrConflict) {
		t.Fatalf("divergent reuse error = %v", err)
	}
}
