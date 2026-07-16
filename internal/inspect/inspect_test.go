package inspect_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"motor-autonomo/internal/control"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/inspect"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func TestProjectorOverviewAndEventPagination(t *testing.T) {
	store, mission, operation, now := seedRuntime(t)
	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "motor-autonomo", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	projector.Clock = func() time.Time { return now }

	overview, err := projector.BuildOverview(context.Background(), mission.MissionID)
	if err != nil {
		t.Fatal(err)
	}
	if overview.ProcessMode != domain.ProcessRunning {
		t.Fatalf("process mode = %s", overview.ProcessMode)
	}
	if overview.EventHeadSequence == 0 {
		t.Fatal("expected event head after seed")
	}
	if overview.Mission == nil || overview.Mission.ActiveRevisionID != mission.ID {
		t.Fatalf("mission overview = %#v", overview.Mission)
	}
	if overview.Mission.Agenda.Total != 1 || overview.Mission.Agenda.Ready != 1 {
		t.Fatalf("agenda = %#v", overview.Mission.Agenda)
	}
	if len(overview.Mission.Operations) != 1 || overview.Mission.Operations[0].ID != operation.ID {
		t.Fatalf("operations = %#v", overview.Mission.Operations)
	}
	if overview.Mission.Horizon == nil {
		t.Fatal("expected executable horizon projection")
	}
	if overview.Mission.Horizon.ReadyCount != 1 || overview.Mission.Horizon.TargetReady <= 0 {
		t.Fatalf("horizon = %#v", overview.Mission.Horizon)
	}
	if overview.Mission.Horizon.OpenCandidates != 1 {
		t.Fatalf("open candidates = %d", overview.Mission.Horizon.OpenCandidates)
	}
	if overview.Mission.Frontier == nil {
		t.Fatal("expected frontier projection")
	}
	if overview.Mission.Frontier.Total != 1 || overview.Mission.Frontier.Open != 1 {
		t.Fatalf("frontier = %#v", overview.Mission.Frontier)
	}
	if len(overview.Mission.Frontier.ByFamily) != 1 || overview.Mission.Frontier.ByFamily[0].Family != domain.FamilyGapScan {
		t.Fatalf("by_family = %#v", overview.Mission.Frontier.ByFamily)
	}
	if overview.Mission.LatestDiagnosis == nil || overview.Mission.LatestDiagnosis.ID != "diag_inspect_1" {
		t.Fatalf("latest diagnosis = %#v", overview.Mission.LatestDiagnosis)
	}

	page, err := projector.ListEvents(context.Background(), inspect.EventFilter{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 1 || page.NextSequence != page.Events[0].Sequence {
		t.Fatalf("page = %#v", page)
	}
	next, err := projector.ListEvents(context.Background(), inspect.EventFilter{AfterSequence: page.NextSequence, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Events) == 0 {
		t.Fatal("expected remaining events after first page")
	}
	if next.Events[0].Sequence <= page.NextSequence {
		t.Fatalf("pagination regressed: first=%d second=%d", page.NextSequence, next.Events[0].Sequence)
	}

	filtered, err := projector.ListEvents(context.Background(), inspect.EventFilter{
		OperationID: operation.ID,
		Limit:       10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Events) == 0 {
		t.Fatal("expected operation-correlated events")
	}
	for _, event := range filtered.Events {
		if event.OperationID != operation.ID {
			t.Fatalf("filter leak: %#v", event)
		}
	}
}

func TestOperationInspectorProjectsModelRecoverySummary(t *testing.T) {
	store, mission, operation, now := seedRuntime(t)
	lease := "lease_rec:until=2026-07-16T20:00:00Z"
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		events := []domain.Event{
			{
				SchemaVersion: domain.SchemaVersionV1, ID: "ev_inv_1", Kind: "operation.model_invoked",
				OccurredAt: now, MissionRevision: mission.ID, InquiryID: operation.InquiryID, OperationID: operation.ID,
				PayloadRef: lease + ";model=primary;call=1",
			},
			{
				SchemaVersion: domain.SchemaVersionV1, ID: "ev_dec_1", Kind: "operation.model_recovery_decision",
				OccurredAt: now.Add(time.Second), MissionRevision: mission.ID, InquiryID: operation.InquiryID, OperationID: operation.ID,
				PayloadRef: lease + ";disposition=SHORT_CORRECT;stage=SHORT_CORRECTION;reason=validation_failed_short_correction_available;calls=1",
			},
			{
				SchemaVersion: domain.SchemaVersionV1, ID: "ev_inv_2", Kind: "operation.model_invoked",
				OccurredAt: now.Add(2 * time.Second), MissionRevision: mission.ID, InquiryID: operation.InquiryID, OperationID: operation.ID,
				PayloadRef: lease + ";model=primary;call=2;recovery=1",
			},
			{
				SchemaVersion: domain.SchemaVersionV1, ID: "ev_dec_2", Kind: "operation.model_recovery_decision",
				OccurredAt: now.Add(3 * time.Second), MissionRevision: mission.ID, InquiryID: operation.InquiryID, OperationID: operation.ID,
				PayloadRef: lease + ";disposition=FALLBACK_MODEL;stage=FALLBACK_MODEL;reason=validation_failed_fallback_model_available;calls=2",
			},
			{
				SchemaVersion: domain.SchemaVersionV1, ID: "ev_inv_3", Kind: "operation.model_invoked",
				OccurredAt: now.Add(4 * time.Second), MissionRevision: mission.ID, InquiryID: operation.InquiryID, OperationID: operation.ID,
				PayloadRef: lease + ";model=fallback;call=3;recovery=1;fallback=1",
			},
			{
				SchemaVersion: domain.SchemaVersionV1, ID: "ev_exh", Kind: "operation.model_exhausted",
				OccurredAt: now.Add(5 * time.Second), MissionRevision: mission.ID, InquiryID: operation.InquiryID, OperationID: operation.ID,
				PayloadRef: lease + ";reason=model_recovery_budget_exhausted;disposition=EXHAUST",
			},
		}
		for _, event := range events {
			if _, err := tx.AppendEvent(event); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "motor-autonomo", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := projector.OperationInspector(context.Background(), operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.ModelRecovery == nil {
		t.Fatal("expected model_recovery summary")
	}
	sum := detail.ModelRecovery
	if sum.Invocations != 3 {
		t.Fatalf("invocations = %d", sum.Invocations)
	}
	if sum.RecoveryInvocations != 2 {
		t.Fatalf("recovery invocations = %d", sum.RecoveryInvocations)
	}
	if sum.FallbackInvocations != 1 {
		t.Fatalf("fallback invocations = %d", sum.FallbackInvocations)
	}
	if !sum.Exhausted {
		t.Fatal("expected exhausted")
	}
	if sum.LastDisposition != "EXHAUST" {
		t.Fatalf("last disposition = %q", sum.LastDisposition)
	}
	if len(sum.Decisions) != 2 {
		t.Fatalf("decisions = %#v", sum.Decisions)
	}
	if sum.Decisions[0].Disposition != "SHORT_CORRECT" || sum.Decisions[1].Stage != "FALLBACK_MODEL" {
		t.Fatalf("decision order/content = %#v", sum.Decisions)
	}
	if len(sum.StagesTried) != 2 || sum.StagesTried[0] != "SHORT_CORRECTION" || sum.StagesTried[1] != "FALLBACK_MODEL" {
		t.Fatalf("stages_tried = %#v", sum.StagesTried)
	}

	// HTTP surface includes the projection under redaction envelope.
	api, err := inspect.NewAPI(projector)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)
	resp, err := http.Get(server.URL + "/operations/" + string(operation.ID))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	rec, ok := body["model_recovery"].(map[string]any)
	if !ok || rec == nil {
		t.Fatalf("http model_recovery missing: %#v", body["model_recovery"])
	}
	if rec["exhausted"] != true {
		t.Fatalf("http exhausted = %#v", rec["exhausted"])
	}
}

func TestOperationInspectorOmitsModelRecoveryWithoutSignal(t *testing.T) {
	store, _, operation, _ := seedRuntime(t)
	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "motor-autonomo", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := projector.OperationInspector(context.Background(), operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.ModelRecovery != nil {
		t.Fatalf("expected nil model_recovery without recovery events, got %#v", detail.ModelRecovery)
	}
}

func TestOperationInspectorCorrelatesCommitChain(t *testing.T) {
	store, mission, operation, now := seedRuntime(t)
	commit := domain.Commit{
		SchemaVersion: domain.SchemaVersionV1, ID: "commit_1", AcceptedChangeSetID: "accepted_1",
		MissionRevision: mission.ID, BaseCommitID: domain.GenesisCommitID, Version: 1,
		CommittedAt: now, ReceiptID: "commit_receipt_1", IdempotencyKey: operation.IdempotencyKey,
	}
	accepted := domain.AcceptedChangeSet{
		SchemaVersion: domain.SchemaVersionV1, ID: "accepted_1", ProposedChangeSetID: "proposed_1",
		ValidationReceiptIDs: []domain.ReceiptID{"validation_1"}, AcceptedAt: now, PolicyVersion: "v1",
	}
	proposed := domain.ProposedChangeSet{
		SchemaVersion: domain.SchemaVersionV1, ID: "proposed_1", MissionRevision: mission.ID,
		OperationID: operation.ID, BaseCommitID: domain.GenesisCommitID, ReadSet: []string{"fragment_1"},
		Preconditions: []string{}, Changes: []domain.Change{{Kind: domain.ChangeAdd, EntityType: "observation", EntityID: "observation_1", PayloadRef: "payload_1"}},
		ExpectedDelta: "one observation", ValidatorIDs: []string{"schema"}, Provenance: "model:fake", IdempotencyKey: operation.IdempotencyKey,
	}
	raw := domain.RawModelOutput{
		SchemaVersion: domain.SchemaVersionV1, ID: "artifact_validation_1", OperationID: operation.ID,
		Model: "fake", Content: `{"ok":true}`, ContentHash: "hash_validation_1", CreatedAt: now,
	}
	validation := domain.ValidationReceipt{
		SchemaVersion: domain.SchemaVersionV1, ID: "validation_1", OperationID: operation.ID,
		ChangeSetID: proposed.ID, ValidatorID: "schema", Passed: true, ArtifactRef: raw.ID,
		ProducedAt: now,
	}
	commitReceipt := domain.CommitReceipt{
		SchemaVersion: domain.SchemaVersionV1, ID: "commit_receipt_1", CommitID: commit.ID,
		ChangeSetID: accepted.ID, OperationID: operation.ID, Version: 1, ProducedAt: now,
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.AppendRawModelOutput(raw); err != nil {
			return err
		}
		if err := tx.AppendProposedChangeSet(proposed); err != nil {
			return err
		}
		if err := tx.AppendValidationReceipt(validation); err != nil {
			return err
		}
		if err := tx.AppendAcceptedChangeSet(accepted); err != nil {
			return err
		}
		if err := tx.ApplyCommit(commit, commitReceipt, proposed.Changes); err != nil {
			return err
		}
		_, err := tx.AppendEvent(domain.Event{
			SchemaVersion: domain.SchemaVersionV1, ID: "event_commit", Kind: "knowledge.commit.applied",
			OccurredAt: now, MissionRevision: mission.ID, OperationID: operation.ID, CommitID: commit.ID,
			PayloadRef: string(proposed.ID),
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "motor-autonomo", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := projector.OperationInspector(context.Background(), operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Spec == nil || detail.Inquiry == nil || detail.Question == nil {
		t.Fatalf("missing lineage: %#v", detail)
	}
	if len(detail.Commits) != 1 || detail.Commits[0].ID != commit.ID {
		t.Fatalf("commits = %#v", detail.Commits)
	}
	if len(detail.Proposed) != 1 || detail.Proposed[0].ID != proposed.ID {
		t.Fatalf("proposed = %#v", detail.Proposed)
	}
	if len(detail.RawOutputs) != 1 || detail.RawOutputs[0].ID != raw.ID {
		t.Fatalf("raw outputs = %#v", detail.RawOutputs)
	}
	if len(detail.Validations) != 1 || detail.Validations[0].ID != validation.ID {
		t.Fatalf("validations = %#v", detail.Validations)
	}
	if len(detail.Events) == 0 {
		t.Fatal("expected correlated events")
	}

	commitDetail, err := projector.CommitInspector(context.Background(), commit.ID)
	if err != nil {
		t.Fatal(err)
	}
	if commitDetail.Proposed == nil || commitDetail.Accepted == nil || commitDetail.Receipt == nil {
		t.Fatalf("commit detail incomplete: %#v", commitDetail)
	}
}

func TestCommandInspectorAndHTTPReadOnlySurface(t *testing.T) {
	store, mission, operation, now := seedRuntime(t)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(100)
	inbox, err := control.NewCommandInbox(store, control.FixedReceiptFactory("receipt_cmd", now))
	if err != nil {
		t.Fatal(err)
	}
	revision := mission.Revision
	pause := domain.OperatorCommand{
		SchemaVersion: domain.SchemaVersionV1, ID: "cmd_pause", IdempotencyKey: "idem_pause_inspect",
		ActorType: domain.ActorOperator, ActorID: "operator_1", Kind: domain.CommandPauseMission,
		Target: domain.CommandTarget{MissionID: mission.MissionID}, ExpectedRevision: &revision,
		Reason: "inspect test", SubmittedAt: now,
	}
	if _, err := inbox.SubmitCommand(pause); err != nil {
		t.Fatal(err)
	}
	processor, err := kernel.NewCommandProcessor(store, clock, ids)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := processor.ProcessNext(context.Background()); err != nil {
		t.Fatal(err)
	}

	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{Name: "motor-autonomo", Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	projector.Clock = func() time.Time { return now }
	commandDetail, err := projector.CommandInspector(context.Background(), pause.ID)
	if err != nil {
		t.Fatal(err)
	}
	if commandDetail.Receipt.State != domain.CommandApplied {
		t.Fatalf("receipt = %#v", commandDetail.Receipt)
	}
	if len(commandDetail.Events) == 0 {
		t.Fatal("expected command audit events")
	}

	api, err := inspect.NewAPI(projector)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)

	// Health and version.
	mustStatus(t, server.URL+"/health", http.StatusOK)
	mustStatus(t, server.URL+"/version", http.StatusOK)

	// Overview with mission.
	resp := mustGET(t, server.URL+"/overview?mission_id="+string(mission.MissionID))
	defer resp.Body.Close()
	var overview inspect.Overview
	if err := json.NewDecoder(resp.Body).Decode(&overview); err != nil {
		t.Fatal(err)
	}
	if overview.Mission == nil {
		t.Fatal("expected mission in overview")
	}
	if overview.Mission.DispatchAllowsNew {
		t.Fatalf("paused mission still allows dispatch: %#v", overview.Mission)
	}

	// Events resume by sequence.
	eventsResp := mustGET(t, server.URL+"/events?limit=1")
	defer eventsResp.Body.Close()
	var page inspect.EventPage
	if err := json.NewDecoder(eventsResp.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 1 {
		t.Fatalf("events page = %#v", page)
	}
	resume := mustGET(t, server.URL+"/events?after_sequence="+itoa(page.NextSequence)+"&limit=50")
	defer resume.Body.Close()
	var page2 inspect.EventPage
	if err := json.NewDecoder(resume.Body).Decode(&page2); err != nil {
		t.Fatal(err)
	}
	if len(page2.Events) == 0 {
		t.Fatal("resume page empty")
	}

	// Operation inspector endpoint.
	opResp := mustGET(t, server.URL+"/operations/"+string(operation.ID))
	defer opResp.Body.Close()
	if opResp.StatusCode != http.StatusOK {
		t.Fatalf("operation status = %d", opResp.StatusCode)
	}

	// Command endpoint.
	cmdResp := mustGET(t, server.URL+"/commands/"+string(pause.ID))
	defer cmdResp.Body.Close()
	if cmdResp.StatusCode != http.StatusOK {
		t.Fatalf("command status = %d", cmdResp.StatusCode)
	}

	// Missing resource.
	missing := mustGET(t, server.URL+"/operations/operation_missing")
	defer missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("missing status = %d", missing.StatusCode)
	}

	// Read-only: POST is not registered.
	post, err := http.Post(server.URL+"/overview", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer post.Body.Close()
	if post.StatusCode != http.StatusMethodNotAllowed && post.StatusCode != http.StatusNotFound {
		t.Fatalf("unexpected write status = %d", post.StatusCode)
	}
}

func seedRuntime(t *testing.T) (*memory.Store, domain.MissionRevision, domain.Operation, time.Time) {
	t.Helper()
	store := memory.New()
	now := time.Date(2026, 7, 16, 4, 0, 0, 0, time.UTC)
	mission := domain.MissionRevision{
		SchemaVersion: domain.SchemaVersionV1, ID: "revision_1", MissionID: "mission_1", Revision: 1,
		OriginalText: "inspect mission", Purpose: "inspect", Status: domain.MissionActive,
		Provenance: "fixture", AcceptedAt: now,
	}
	spec := domain.OperationSpec{
		SchemaVersion: domain.SchemaVersionV1, ID: "spec_1", ContractVersion: 1, TemplateVersion: 1,
		InputSchema: "input", OutputSchema: "output", Budget: domain.Budget{Tokens: 100, ModelCalls: 1, Attempts: 1},
		MaxOutputTokens: 20, SafetyMargin: 5, Validators: []string{"schema"}, RetryPolicy: "none",
		FallbackPolicy: "none", MaximumAuthority: domain.AuthorityProposeOnly,
	}
	question := domain.Question{
		SchemaVersion: domain.SchemaVersionV1, ID: "question_1", MissionRevision: mission.ID,
		Text: "What is known?", Origin: "fixture", Relevance: "core", AnswerCondition: "cited claim",
	}
	candidate := domain.InquiryCandidate{
		SchemaVersion: domain.SchemaVersionV1, ID: "candidate_1", MissionRevision: mission.ID, QuestionID: question.ID,
		DerivedFrom: []string{"mission"}, ExpectedProgress: "one claim", Novelty: "new", EstimatedCost: domain.Budget{Tokens: 50},
		Risk: domain.RiskLow, SourcePlan: []string{"fixture"}, AnswerCondition: "cited claim", StopCondition: "done", ReviewAfter: now.Add(time.Hour),
	}
	inquiry := domain.Inquiry{
		SchemaVersion: domain.SchemaVersionV1, ID: "inquiry_1", CandidateID: candidate.ID, MissionRevision: mission.ID,
		QuestionID: question.ID, AdmissionReason: "seed", Budget: domain.Budget{Tokens: 50}, StopCondition: "done",
		State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
	}
	operation := domain.Operation{
		SchemaVersion: domain.SchemaVersionV1, ID: "operation_1", InquiryID: inquiry.ID, MissionRevision: mission.ID,
		SpecID: spec.ID, ReadSet: []string{"fragment_1"}, InputRefs: []string{"artifact_1"}, ExpectedOutput: "proposed_change_set",
		IdempotencyKey: "idem_op_1", State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
	}
	opp := domain.WorkOpportunity{
		SchemaVersion: domain.SchemaVersionV1, ID: "opp_inspect_gap", MissionRevision: mission.ID,
		Family: domain.FamilyGapScan, Status: domain.OpportunityOpen, Title: "inspect frontier seed",
		Origin: "fixture", ExpectedGain: "visible frontier row", Novelty: "overview frontier projection",
		StopCondition: "projected", DedupSignature: "inspect:frontier:gap", Depth: 0,
		EstimatedCost: domain.Budget{Tokens: 10, Attempts: 1}, Risk: domain.RiskLow,
		Priority: 12, CreatedAt: now, UpdatedAt: now,
	}
	diag := domain.ContinuityDiagnosis{
		SchemaVersion: domain.SchemaVersionV1, ID: "diag_inspect_1", MissionRevision: mission.ID,
		OccurredAt: now.Add(-time.Minute), StrategiesTried: []string{"gap_scan@v2"},
		OpenCandidateCount: 1, ReadyCount: 0, RecoveryConditions: []string{"admit open opportunity"},
		SafeDetail: "no ready work under policy; catalog=continuity-catalog.v2", PolicyVersion: "horizon.v1",
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
		if err := tx.CreateOperation(operation); err != nil {
			return err
		}
		if err := tx.CreateWorkOpportunity(opp); err != nil {
			return err
		}
		if err := tx.CreateContinuityDiagnosis(diag); err != nil {
			return err
		}
		if _, err := tx.AppendEvent(domain.Event{
			SchemaVersion: domain.SchemaVersionV1, ID: "event_mission", Kind: "mission.revision_activated",
			OccurredAt: now, MissionRevision: mission.ID, PayloadRef: string(mission.ID),
		}); err != nil {
			return err
		}
		_, err := tx.AppendEvent(domain.Event{
			SchemaVersion: domain.SchemaVersionV1, ID: "event_agenda", Kind: "agenda.work_created",
			OccurredAt: now.Add(time.Second), MissionRevision: mission.ID, InquiryID: inquiry.ID,
			OperationID: operation.ID, PayloadRef: string(question.ID),
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return store, mission, operation, now
}

func mustGET(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func mustStatus(t *testing.T, url string, want int) {
	t.Helper()
	resp := mustGET(t, url)
	defer resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("%s status = %d want %d", url, resp.StatusCode, want)
	}
}

func itoa(v uint64) string {
	return strconvFormatUint(v)
}

func strconvFormatUint(v uint64) string {
	// local tiny helper to avoid importing strconv in multiple places of helpers
	const digits = "0123456789"
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = digits[v%10]
		v /= 10
	}
	return string(buf[i:])
}
