package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"motor-autonomo/internal/control"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

// TestSmokeQuestionPathEndToEnd covers:
// gate admit → outbox → telegram delivery → correlated USER_ANSWER →
// kernel apply → operation wait resume.
func TestSmokeQuestionPathEndToEnd(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	store := memory.New()
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)

	mission, operation := seedMissionWithReadyOperation(t, store, now)

	// Block operation on a future operator question (same identity the gate will create).
	questionID := domain.OperatorQuestionID("ask_e2e_1")
	proposal := domain.OperatorQuestionProposal{
		SchemaVersion: domain.SchemaVersionV1,
		Question: domain.OperatorQuestion{
			SchemaVersion: domain.SchemaVersionV1, ID: questionID, MissionID: mission.MissionID, MissionRevision: mission.ID, Revision: 1,
			Kind: domain.QuestionSingleChoice, Prompt: "Pick presentation", Context: "Affects artifact only",
			Options:      []domain.QuestionOption{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}},
			AllowContext: true, AllowSkip: true,
			BlockingScope:  []domain.QuestionBlockingTarget{{Kind: domain.QuestionBlockingOperation, Reference: string(operation.ID)}},
			FallbackPolicy: domain.QuestionContinueOtherWork, DedupSignature: "e2e:presentation", Priority: 50,
			Status: domain.OperatorQuestionPending, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
		},
		Justification: domain.QuestionJustification{
			MissingInformation: "operator preference", DecisionImpact: "presentation",
			AlternativesTried: []string{"mission policies"}, ExpectedGain: "avoid rework", CostOfSilence: "neutral layout",
		},
		ProposedBy: "model:small", ProposedAt: now,
	}

	// 1) Gate admits → canonical question + PENDING outbox delivery.
	gate, err := kernel.NewQuestionGateProcessor(
		store, clock, ids,
		kernel.QuestionGatePolicy{MinPriority: 20, MaxPending: 5, MaxDeliveredPerWindow: 5, Window: time.Hour, Cooldown: time.Hour, QuietStartHour: 23, QuietEndHour: 7, UrgentPriority: 90, MinAlternativesTried: 1},
		"default@1",
		[]kernel.QuestionRoute{{Channel: ChannelName, DestinationRef: "operator_primary", MaxAttempts: 3}},
	)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := gate.Process(context.Background(), proposal)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Decision != domain.PersistedQuestionAdmit {
		t.Fatalf("gate decision = %#v", decision)
	}

	// Apply local wait now that the question exists.
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		q, err := tx.OperatorQuestion(questionID)
		if err != nil {
			return err
		}
		ops, err := tx.Operations(mission.ID)
		if err != nil {
			return err
		}
		blocked, err := kernel.ApplyQuestionWait(q, ops)
		if err != nil {
			return err
		}
		for _, op := range blocked {
			if err := tx.SaveOperation(op); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		op, err := r.Operation(operation.ID)
		if err != nil {
			return err
		}
		if op.State != domain.StateWaitingEvent {
			t.Fatalf("operation should wait, got %#v", op)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// 2) Telegram worker leases and delivers.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":777}}`))
	}))
	t.Cleanup(server.Close)
	adapter := testAdapter(t, server)
	worker := DeliveryWorker{
		Store: store, Adapter: adapter, Clock: clock, Owner: "worker_e2e",
		LeaseDuration: time.Minute, RetryDelay: time.Minute,
	}
	processed, err := worker.ProcessDue(context.Background(), 10)
	if err != nil || processed != 1 {
		t.Fatalf("delivery process = %d err=%v", processed, err)
	}
	var delivered domain.QuestionDelivery
	var question domain.OperatorQuestion
	if err := store.View(context.Background(), func(r port.Reader) error {
		var err error
		question, err = r.OperatorQuestion(questionID)
		if err != nil {
			return err
		}
		deliveries, err := r.QuestionDeliveries(questionID)
		if err != nil {
			return err
		}
		if len(deliveries) != 1 || deliveries[0].Status != domain.QuestionDeliveryDelivered || deliveries[0].TransportMessageID != "777" {
			t.Fatalf("deliveries = %#v", deliveries)
		}
		delivered = deliveries[0]
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// 3) Operator answers via Telegram callback → ExternalEvent.
	update := Update{
		UpdateID: 9001,
		CallbackQuery: &CallbackQuery{
			ID:   "cb_e2e_1",
			From: User{ID: 7},
			Message: &Message{
				MessageID: 777,
				Chat:      Chat{ID: 100},
			},
			Data: "o:1", // second option ("b") per SendQuestion encoding
		},
	}
	answerIDs := source.NewSequenceIDGenerator(5000)
	event, err := adapter.ExternalAnswer(update, question, delivered, answerIDs, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("ExternalAnswer: %v", err)
	}

	inbox, err := control.NewExternalEventInbox(store, control.FixedDispositionFactory(now.Add(2*time.Minute)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inbox.SubmitExternalEvent(event); err != nil {
		t.Fatal(err)
	}

	// 4) Kernel applies answer and resumes wait.
	processor, err := kernel.NewExternalEventProcessor(store, source.NewManualClock(now.Add(2*time.Minute)), source.NewSequenceIDGenerator(9000))
	if err != nil {
		t.Fatal(err)
	}
	disposition, ok, err := processor.ProcessNext(context.Background())
	if err != nil || !ok || disposition.State != domain.ExternalEventApplied {
		t.Fatalf("process answer = %#v ok=%v err=%v", disposition, ok, err)
	}

	if err := store.View(context.Background(), func(r port.Reader) error {
		q, err := r.OperatorQuestion(questionID)
		if err != nil {
			return err
		}
		if q.Status != domain.OperatorQuestionAnswered {
			t.Fatalf("question status = %#v", q)
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
			t.Fatalf("pending external = %#v err=%v", pending, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// 5) Idempotent replay of the same transport event does not re-apply.
	if again, err := inbox.SubmitExternalEvent(event); err != nil {
		t.Fatal(err)
	} else if again.State != domain.ExternalEventApplied && again.State != domain.ExternalEventReceived {
		// Durable inbox may return terminal disposition on exact replay.
		t.Logf("replay submit disposition = %#v", again)
	}
	againDisp, err := processor.Process(context.Background(), event.ID)
	if err != nil {
		t.Fatal(err)
	}
	if againDisp.State != domain.ExternalEventApplied {
		t.Fatalf("replay process = %#v", againDisp)
	}
}

func seedMissionWithReadyOperation(t *testing.T, store port.Store, now time.Time) (domain.MissionRevision, domain.Operation) {
	t.Helper()
	mission := domain.MissionRevision{
		SchemaVersion: domain.SchemaVersionV1, ID: "revision_1", MissionID: "mission_1", Revision: 1,
		OriginalText: "e2e", Purpose: "e2e smoke", Status: domain.MissionActive, Provenance: "fixture", AcceptedAt: now,
	}
	spec := domain.OperationSpec{
		SchemaVersion: 1, ID: "extract@1", ContractVersion: 1, TemplateVersion: 1, InputSchema: "fragment refs",
		OutputSchema: "proposed change set", Budget: domain.Budget{ModelCalls: 1, Tokens: 1000, Attempts: 1},
		MaxOutputTokens: 100, SafetyMargin: 50, Validators: []string{"schema"}, RetryPolicy: "no retry",
		FallbackPolicy: "fail", MaximumAuthority: domain.AuthorityProposeOnly,
	}
	q := domain.Question{
		SchemaVersion: 1, ID: "question_1", MissionRevision: mission.ID, Text: "What is true?", Origin: "mission",
		Relevance: "primary", AnswerCondition: "two sources",
	}
	candidate := domain.InquiryCandidate{
		SchemaVersion: 1, ID: "candidate_1", MissionRevision: mission.ID, QuestionID: q.ID,
		DerivedFrom: []string{"gap_1"}, ExpectedProgress: "reduce uncertainty", Novelty: "not duplicate",
		Risk: domain.RiskLow, SourcePlan: []string{"primary sources"}, AnswerCondition: "two sources",
		StopCondition: "budget", ReviewAfter: now.Add(time.Hour),
	}
	inquiry := domain.Inquiry{
		SchemaVersion: 1, ID: "inquiry_1", CandidateID: candidate.ID, MissionRevision: mission.ID, QuestionID: q.ID,
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
		if err := tx.CreateQuestion(q); err != nil {
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
