package control_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"motor-autonomo/internal/control"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func TestControlAPISubmitCommandIdempotentAndLookup(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 16, 5, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	seedMission(t, store, now)

	api := newControlAPI(t, store, clock, ids)
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)

	body := map[string]any{
		"schema_version":    1,
		"idempotency_key":   "idem_pause_http",
		"kind":              "PAUSE_MISSION",
		"target":            map[string]any{"mission_id": "mission_1"},
		"expected_revision": 1,
		"reason":            "http test",
	}
	first := mustPOSTJSON(t, server.URL+"/commands", body)
	defer first.Body.Close()
	if first.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", first.StatusCode, readBody(t, first))
	}
	var resp1 map[string]any
	decodeJSON(t, first.Body, &resp1)
	if resp1["accepted"] != true {
		t.Fatalf("accepted = %#v", resp1["accepted"])
	}
	commandID, _ := resp1["command_id"].(string)
	if commandID == "" {
		t.Fatalf("missing command_id: %#v", resp1)
	}
	receipt1, ok := resp1["receipt"].(map[string]any)
	if !ok || receipt1["state"] != string(domain.CommandReceived) {
		t.Fatalf("receipt = %#v", resp1["receipt"])
	}

	// Identical retry by idempotency key returns the same receipt/state.
	body["command_id"] = commandID
	second := mustPOSTJSON(t, server.URL+"/commands", body)
	defer second.Body.Close()
	if second.StatusCode != http.StatusAccepted {
		t.Fatalf("retry status = %d body=%s", second.StatusCode, readBody(t, second))
	}
	var resp2 map[string]any
	decodeJSON(t, second.Body, &resp2)
	if resp2["command_id"] != commandID {
		t.Fatalf("retry command_id = %#v want %s", resp2["command_id"], commandID)
	}
	receipt2 := resp2["receipt"].(map[string]any)
	if receipt2["receipt_id"] != receipt1["receipt_id"] || receipt2["state"] != receipt1["state"] {
		t.Fatalf("retry receipt diverged: first=%#v second=%#v", receipt1, receipt2)
	}

	// Lookup command + receipt.
	got := mustGET(t, server.URL+"/commands/"+commandID)
	defer got.Body.Close()
	if got.StatusCode != http.StatusOK {
		t.Fatalf("get command status = %d", got.StatusCode)
	}
	var lookup map[string]any
	decodeJSON(t, got.Body, &lookup)
	if lookup["command"] == nil || lookup["receipt"] == nil {
		t.Fatalf("lookup = %#v", lookup)
	}

	// Process and observe terminal receipt via dedicated receipt endpoint.
	processor, err := kernel.NewCommandProcessor(store, clock, ids)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := processor.ProcessNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	receiptResp := mustGET(t, server.URL+"/commands/"+commandID+"/receipt")
	defer receiptResp.Body.Close()
	var applied domain.CommandReceipt
	decodeJSON(t, receiptResp.Body, &applied)
	if applied.State != domain.CommandApplied {
		t.Fatalf("applied receipt = %#v", applied)
	}

	// Divergent reuse conflicts.
	body["reason"] = "other"
	conflict := mustPOSTJSON(t, server.URL+"/commands", body)
	defer conflict.Body.Close()
	if conflict.StatusCode != http.StatusConflict {
		t.Fatalf("conflict status = %d body=%s", conflict.StatusCode, readBody(t, conflict))
	}
}

func TestControlAPISubmitExternalEventAndDisposition(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 16, 5, 10, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(50)
	seedMission(t, store, now)

	api := newControlAPI(t, store, clock, ids)
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)

	body := map[string]any{
		"schema_version":       1,
		"deduplication_key":    "dashboard:msg:1",
		"kind":                 "USER_MESSAGE",
		"mission_id":           "mission_1",
		"content":              map[string]any{"media_type": "text/plain", "text": "hello from dashboard"},
		"source":               "operator-dashboard",
		"source_actor_id":      "operator_1",
		"transport_message_id": "ui-1",
	}
	first := mustPOSTJSON(t, server.URL+"/external-events", body)
	defer first.Body.Close()
	if first.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", first.StatusCode, readBody(t, first))
	}
	var resp map[string]any
	decodeJSON(t, first.Body, &resp)
	eventID, _ := resp["event_id"].(string)
	if eventID == "" {
		t.Fatalf("missing event_id: %#v", resp)
	}
	disp := resp["disposition"].(map[string]any)
	if disp["state"] != string(domain.ExternalEventReceived) {
		t.Fatalf("disposition = %#v", disp)
	}

	// Replay identical delivery.
	second := mustPOSTJSON(t, server.URL+"/external-events", body)
	defer second.Body.Close()
	if second.StatusCode != http.StatusAccepted {
		t.Fatalf("replay status = %d", second.StatusCode)
	}
	var resp2 map[string]any
	decodeJSON(t, second.Body, &resp2)
	if resp2["event_id"] != eventID {
		t.Fatalf("replay event_id = %#v", resp2["event_id"])
	}

	got := mustGET(t, server.URL+"/external-events/"+eventID)
	defer got.Body.Close()
	if got.StatusCode != http.StatusOK {
		t.Fatalf("get event status = %d", got.StatusCode)
	}

	// Kernel processes wake with no matching wait → IGNORED, not authority.
	processor, err := kernel.NewExternalEventProcessor(store, clock, ids)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := processor.ProcessNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	dispResp := mustGET(t, server.URL+"/external-events/"+eventID+"/disposition")
	defer dispResp.Body.Close()
	var terminal domain.ExternalEventDisposition
	decodeJSON(t, dispResp.Body, &terminal)
	if terminal.State != domain.ExternalEventIgnored {
		t.Fatalf("terminal disposition = %#v", terminal)
	}
}

func TestControlAPIQuestionsListGetSubmitAndProcessAnswer(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 16, 5, 15, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(70)
	seedMission(t, store, now)
	question := domain.OperatorQuestion{
		SchemaVersion: domain.SchemaVersionV1, ID: "ask_dashboard", MissionID: "mission_1", MissionRevision: "revision_1",
		Revision: 1, Kind: domain.QuestionSingleChoiceWithOther, Prompt: "Choose a style", Context: "Affects artifact presentation only.",
		Options:    []domain.QuestionOption{{ID: "minimal", Label: "Minimal"}, {ID: "modern", Label: "Modern"}},
		AllowOther: true, AllowContext: true, AllowSkip: true, FallbackPolicy: domain.QuestionContinueOtherWork,
		DedupSignature: "style:artifact", Priority: 50, Status: domain.OperatorQuestionPending, CreatedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour),
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error { return tx.CreateOperatorQuestion(question) }); err != nil {
		t.Fatal(err)
	}

	api := newControlAPI(t, store, clock, ids)
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)

	listed := mustGET(t, server.URL+"/questions?mission_id=mission_1&status=PENDING")
	defer listed.Body.Close()
	if listed.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d body=%s", listed.StatusCode, readBody(t, listed))
	}
	var listBody struct {
		Questions []domain.OperatorQuestion `json:"questions"`
	}
	decodeJSON(t, listed.Body, &listBody)
	if len(listBody.Questions) != 1 || listBody.Questions[0].ID != question.ID {
		t.Fatalf("questions = %#v", listBody.Questions)
	}

	got := mustGET(t, server.URL+"/questions/"+string(question.ID))
	defer got.Body.Close()
	if got.StatusCode != http.StatusOK {
		t.Fatalf("get question status = %d", got.StatusCode)
	}

	body := map[string]any{
		"schema_version": 1, "idempotency_key": "dashboard:answer:1", "expected_question_revision": 1,
		"kind": "OPTIONS", "option_ids": []string{"minimal"}, "actor_id": "operator_1",
	}
	first := mustPOSTJSON(t, server.URL+"/questions/"+string(question.ID)+"/answers", body)
	defer first.Body.Close()
	if first.StatusCode != http.StatusAccepted {
		t.Fatalf("answer status = %d body=%s", first.StatusCode, readBody(t, first))
	}
	var accepted map[string]any
	decodeJSON(t, first.Body, &accepted)
	answerID := accepted["answer_id"]
	eventID := accepted["event_id"]
	if answerID == "" || eventID == "" {
		t.Fatalf("accepted answer = %#v", accepted)
	}

	// A retry may omit the server-generated answer ID and timestamp; the durable
	// event identity and answer identity remain stable by idempotency key.
	retry := mustPOSTJSON(t, server.URL+"/questions/"+string(question.ID)+"/answers", body)
	defer retry.Body.Close()
	if retry.StatusCode != http.StatusAccepted {
		t.Fatalf("retry status = %d body=%s", retry.StatusCode, readBody(t, retry))
	}
	var replay map[string]any
	decodeJSON(t, retry.Body, &replay)
	if replay["answer_id"] != answerID || replay["event_id"] != eventID {
		t.Fatalf("replay diverged: first=%#v replay=%#v", accepted, replay)
	}

	processor, err := kernel.NewExternalEventProcessor(store, clock, ids)
	if err != nil {
		t.Fatal(err)
	}
	if disposition, processed, err := processor.ProcessNext(context.Background()); err != nil {
		t.Fatal(err)
	} else if !processed || disposition.State != domain.ExternalEventApplied {
		t.Fatalf("processed=%v disposition=%#v", processed, disposition)
	}
	answered := mustGET(t, server.URL+"/questions/"+string(question.ID))
	defer answered.Body.Close()
	var terminal domain.OperatorQuestion
	decodeJSON(t, answered.Body, &terminal)
	if terminal.Status != domain.OperatorQuestionAnswered || string(terminal.AnswerID) != answerID {
		t.Fatalf("terminal question = %#v", terminal)
	}
}

func TestControlAPIQuestionAnswerRejectsStaleRevisionAndDivergentReplay(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 16, 5, 18, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(80)
	seedMission(t, store, now)
	question := domain.OperatorQuestion{
		SchemaVersion: 1, ID: "ask_confirm", MissionID: "mission_1", MissionRevision: "revision_1", Revision: 1,
		Kind: domain.QuestionConfirmation, Prompt: "Proceed?", Context: "Affects one reversible action.",
		FallbackPolicy: domain.QuestionContinueOtherWork, DedupSignature: "confirm:1", Priority: 50,
		Status: domain.OperatorQuestionPending, CreatedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour),
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error { return tx.CreateOperatorQuestion(question) }); err != nil {
		t.Fatal(err)
	}
	api := newControlAPI(t, store, clock, ids)
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)

	stale := mustPOSTJSON(t, server.URL+"/questions/ask_confirm/answers", map[string]any{
		"schema_version": 1, "idempotency_key": "answer:stale", "expected_question_revision": 2, "kind": "CONFIRM",
	})
	defer stale.Body.Close()
	if stale.StatusCode != http.StatusConflict {
		t.Fatalf("stale status = %d body=%s", stale.StatusCode, readBody(t, stale))
	}

	validBody := map[string]any{"schema_version": 1, "idempotency_key": "answer:same", "expected_question_revision": 1, "kind": "CONFIRM"}
	valid := mustPOSTJSON(t, server.URL+"/questions/ask_confirm/answers", validBody)
	defer valid.Body.Close()
	if valid.StatusCode != http.StatusAccepted {
		t.Fatalf("valid status = %d body=%s", valid.StatusCode, readBody(t, valid))
	}
	validBody["kind"] = "DECLINE"
	conflict := mustPOSTJSON(t, server.URL+"/questions/ask_confirm/answers", validBody)
	defer conflict.Body.Close()
	if conflict.StatusCode != http.StatusConflict {
		t.Fatalf("divergent replay status = %d body=%s", conflict.StatusCode, readBody(t, conflict))
	}
}

func TestControlAPIRejectsInvalidBodiesAndUnknownFields(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 16, 5, 20, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(90)
	api := newControlAPI(t, store, clock, ids)
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)

	// Unknown field.
	unknown := mustPOSTJSON(t, server.URL+"/commands", map[string]any{
		"schema_version": 1,
		"kind":           "PAUSE_MISSION",
		"reason":         "x",
		"unexpected":     true,
	})
	defer unknown.Body.Close()
	if unknown.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d body=%s", unknown.StatusCode, readBody(t, unknown))
	}

	// Missing required semantic fields after defaults.
	invalid := mustPOSTJSON(t, server.URL+"/commands", map[string]any{
		"schema_version": 1,
		"kind":           "PAUSE_MISSION",
		// no mission target / expected revision / reason
	})
	defer invalid.Body.Close()
	if invalid.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid status = %d body=%s", invalid.StatusCode, readBody(t, invalid))
	}

	// Oversized body.
	huge := strings.Repeat("a", domain.MaxControlPayloadBytes+32)
	resp, err := http.Post(server.URL+"/external-events", "application/json", strings.NewReader(`{"schema_version":1,"kind":"USER_MESSAGE","content":{"media_type":"text/plain","text":"`+huge+`"}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge && resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("oversize status = %d body=%s", resp.StatusCode, readBody(t, resp))
	}

	// Missing resources.
	missingCmd := mustGET(t, server.URL+"/commands/cmd_missing")
	defer missingCmd.Body.Close()
	if missingCmd.StatusCode != http.StatusNotFound {
		t.Fatalf("missing command status = %d", missingCmd.StatusCode)
	}
	missingEvt := mustGET(t, server.URL+"/external-events/ext_missing")
	defer missingEvt.Body.Close()
	if missingEvt.StatusCode != http.StatusNotFound {
		t.Fatalf("missing event status = %d", missingEvt.StatusCode)
	}
}

func newControlAPI(t *testing.T, store port.Store, clock source.Clock, ids source.IDGenerator) *control.API {
	t.Helper()
	commands, err := control.NewCommandInbox(store, control.ReceiptFactoryFrom(clock, ids))
	if err != nil {
		t.Fatal(err)
	}
	events, err := control.NewExternalEventInbox(store, control.DispositionFactoryFrom(clock))
	if err != nil {
		t.Fatal(err)
	}
	api, err := control.NewAPI(commands, events, clock, ids)
	if err != nil {
		t.Fatal(err)
	}
	return api
}

func newControlAPIWithConfig(t *testing.T, store port.Store, clock source.Clock, ids source.IDGenerator) *control.API {
	t.Helper()
	api := newControlAPI(t, store, clock, ids)
	applier, err := kernel.NewConfigApplier(store, clock, ids)
	if err != nil {
		t.Fatal(err)
	}
	api.ConfigValidate = applier
	api.ConfigApply = applier
	api.ConfigRollback = applier
	return api
}

func TestControlAPIConfigDraftLifecycle(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 16, 19, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(90)
	api := newControlAPIWithConfig(t, store, clock, ids)
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)

	policy := domain.DefaultInterruptionRuntimePolicy()
	policy.MaxPending = 5
	createBody := map[string]any{
		"schema_version": 1,
		"scope":          "INTERRUPTION",
		"reason":         "raise pending via admin",
		"interruption":   policy,
	}
	created := mustPOSTJSON(t, server.URL+"/config/drafts", createBody)
	defer created.Body.Close()
	if created.StatusCode != http.StatusAccepted {
		t.Fatalf("create status = %d body=%s", created.StatusCode, readBody(t, created))
	}
	var createResp struct {
		Draft domain.ConfigDraft `json:"draft"`
	}
	decodeJSON(t, created.Body, &createResp)
	if createResp.Draft.ID == "" || createResp.Draft.Status != domain.ConfigDraftOpen {
		t.Fatalf("create draft = %#v", createResp.Draft)
	}
	draftID := string(createResp.Draft.ID)

	listed := mustGET(t, server.URL+"/config/drafts?scope=INTERRUPTION&status=OPEN")
	defer listed.Body.Close()
	if listed.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d body=%s", listed.StatusCode, readBody(t, listed))
	}
	var listBody struct {
		Drafts []domain.ConfigDraft `json:"drafts"`
	}
	decodeJSON(t, listed.Body, &listBody)
	if len(listBody.Drafts) != 1 || string(listBody.Drafts[0].ID) != draftID {
		t.Fatalf("listed drafts = %#v", listBody.Drafts)
	}

	got := mustGET(t, server.URL+"/config/drafts/"+draftID)
	defer got.Body.Close()
	if got.StatusCode != http.StatusOK {
		t.Fatalf("get draft status = %d", got.StatusCode)
	}

	validated := mustPOSTJSON(t, server.URL+"/config/drafts/"+draftID+"/validate", map[string]any{})
	defer validated.Body.Close()
	if validated.StatusCode != http.StatusOK {
		t.Fatalf("validate status = %d body=%s", validated.StatusCode, readBody(t, validated))
	}
	var validateResp struct {
		Draft   domain.ConfigDraft         `json:"draft"`
		Preview domain.ConfigImpactPreview `json:"preview"`
	}
	decodeJSON(t, validated.Body, &validateResp)
	if validateResp.Draft.Status != domain.ConfigDraftValidated || validateResp.Preview.Blocked {
		t.Fatalf("validate resp = %#v", validateResp)
	}

	clock.Advance(time.Second)
	applied := mustPOSTJSON(t, server.URL+"/config/drafts/"+draftID+"/apply", map[string]any{})
	defer applied.Body.Close()
	if applied.StatusCode != http.StatusOK {
		t.Fatalf("apply status = %d body=%s", applied.StatusCode, readBody(t, applied))
	}
	var applyResp struct {
		Revision domain.ConfigRevision     `json:"revision"`
		Receipt  domain.ConfigApplyReceipt `json:"receipt"`
	}
	decodeJSON(t, applied.Body, &applyResp)
	if applyResp.Revision.Revision != 1 || applyResp.Receipt.State != domain.ConfigApplyApplied {
		t.Fatalf("apply resp = %#v", applyResp)
	}

	// Replay apply is pure.
	replay := mustPOSTJSON(t, server.URL+"/config/drafts/"+draftID+"/apply", map[string]any{})
	defer replay.Body.Close()
	if replay.StatusCode != http.StatusOK {
		t.Fatalf("replay apply status = %d body=%s", replay.StatusCode, readBody(t, replay))
	}

	active := mustGET(t, server.URL+"/config/revisions/active?scope=INTERRUPTION")
	defer active.Body.Close()
	if active.StatusCode != http.StatusOK {
		t.Fatalf("active status = %d body=%s", active.StatusCode, readBody(t, active))
	}
	var activeBody struct {
		Revision domain.ConfigRevision `json:"revision"`
	}
	decodeJSON(t, active.Body, &activeBody)
	if activeBody.Revision.ID != applyResp.Revision.ID || activeBody.Revision.Interruption == nil || activeBody.Revision.Interruption.MaxPending != 5 {
		t.Fatalf("active revision = %#v", activeBody.Revision)
	}

	revisions := mustGET(t, server.URL+"/config/revisions?scope=INTERRUPTION")
	defer revisions.Body.Close()
	if revisions.StatusCode != http.StatusOK {
		t.Fatalf("revisions status = %d", revisions.StatusCode)
	}
	var revBody struct {
		Revisions []domain.ConfigRevision `json:"revisions"`
	}
	decodeJSON(t, revisions.Body, &revBody)
	if len(revBody.Revisions) != 1 {
		t.Fatalf("revisions = %#v", revBody.Revisions)
	}

	receipt := mustGET(t, server.URL+"/config/drafts/"+draftID+"/receipt")
	defer receipt.Body.Close()
	if receipt.StatusCode != http.StatusOK {
		t.Fatalf("receipt status = %d", receipt.StatusCode)
	}
	var receiptBody domain.ConfigApplyReceipt
	decodeJSON(t, receipt.Body, &receiptBody)
	if receiptBody.State != domain.ConfigApplyApplied {
		t.Fatalf("receipt = %#v", receiptBody)
	}

	// Second revision so rollback has an ancestor distinct from active.
	clock.Advance(time.Second)
	policy2 := policy
	policy2.MaxPending = 9
	secondBody := map[string]any{
		"schema_version":    1,
		"scope":             "INTERRUPTION",
		"based_on_revision": 1,
		"reason":            "raise pending again",
		"interruption":      policy2,
	}
	created2 := mustPOSTJSON(t, server.URL+"/config/drafts", secondBody)
	defer created2.Body.Close()
	if created2.StatusCode != http.StatusAccepted {
		t.Fatalf("create2 status = %d body=%s", created2.StatusCode, readBody(t, created2))
	}
	var createResp2 struct {
		Draft domain.ConfigDraft `json:"draft"`
	}
	decodeJSON(t, created2.Body, &createResp2)
	draftID2 := string(createResp2.Draft.ID)
	validated2 := mustPOSTJSON(t, server.URL+"/config/drafts/"+draftID2+"/validate", map[string]any{})
	defer validated2.Body.Close()
	if validated2.StatusCode != http.StatusOK {
		t.Fatalf("validate2 status = %d body=%s", validated2.StatusCode, readBody(t, validated2))
	}
	clock.Advance(time.Second)
	applied2 := mustPOSTJSON(t, server.URL+"/config/drafts/"+draftID2+"/apply", map[string]any{})
	defer applied2.Body.Close()
	if applied2.StatusCode != http.StatusOK {
		t.Fatalf("apply2 status = %d body=%s", applied2.StatusCode, readBody(t, applied2))
	}

	// Semantic rollback to first revision payload.
	clock.Advance(time.Second)
	rb := mustPOSTJSON(t, server.URL+"/config/revisions/rollback", map[string]any{
		"schema_version":     1,
		"scope":              "INTERRUPTION",
		"target_revision_id": string(applyResp.Revision.ID),
		"reason":             "restore first policy",
	})
	defer rb.Body.Close()
	if rb.StatusCode != http.StatusOK {
		t.Fatalf("rollback status = %d body=%s", rb.StatusCode, readBody(t, rb))
	}
	var rbBody struct {
		Revision domain.ConfigRevision     `json:"revision"`
		Receipt  domain.ConfigApplyReceipt `json:"receipt"`
		Draft    domain.ConfigDraft        `json:"draft"`
	}
	decodeJSON(t, rb.Body, &rbBody)
	if rbBody.Revision.Revision != 3 || rbBody.Receipt.State != domain.ConfigApplyApplied || rbBody.Draft.Status != domain.ConfigDraftApplied {
		t.Fatalf("rollback body = %#v", rbBody)
	}
	if rbBody.Revision.Interruption == nil || rbBody.Revision.Interruption.MaxPending != 5 {
		t.Fatalf("rollback payload = %#v", rbBody.Revision.Interruption)
	}
	active2 := mustGET(t, server.URL+"/config/revisions/active?scope=INTERRUPTION")
	defer active2.Body.Close()
	var activeBody2 struct {
		Revision domain.ConfigRevision `json:"revision"`
	}
	decodeJSON(t, active2.Body, &activeBody2)
	if activeBody2.Revision.ID != rbBody.Revision.ID {
		t.Fatalf("active after rollback = %#v", activeBody2.Revision)
	}
}

func TestControlAPIConfigValidateApplyRequireWiring(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 16, 19, 30, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(100)
	api := newControlAPI(t, store, clock, ids)
	server := httptest.NewServer(api.Handler())
	t.Cleanup(server.Close)

	policy := domain.DefaultInterruptionRuntimePolicy()
	createBody := map[string]any{
		"schema_version": 1,
		"scope":          "INTERRUPTION",
		"reason":         "unwired validate",
		"interruption":   policy,
	}
	created := mustPOSTJSON(t, server.URL+"/config/drafts", createBody)
	defer created.Body.Close()
	if created.StatusCode != http.StatusAccepted {
		t.Fatalf("create status = %d body=%s", created.StatusCode, readBody(t, created))
	}
	var createResp struct {
		Draft domain.ConfigDraft `json:"draft"`
	}
	decodeJSON(t, created.Body, &createResp)
	draftID := string(createResp.Draft.ID)

	validated := mustPOSTJSON(t, server.URL+"/config/drafts/"+draftID+"/validate", map[string]any{})
	defer validated.Body.Close()
	if validated.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("validate without wiring status = %d body=%s", validated.StatusCode, readBody(t, validated))
	}
	rb := mustPOSTJSON(t, server.URL+"/config/revisions/rollback", map[string]any{
		"schema_version": 1, "scope": "INTERRUPTION", "target_revision_id": "cfgrev_missing",
	})
	defer rb.Body.Close()
	if rb.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("rollback without wiring status = %d body=%s", rb.StatusCode, readBody(t, rb))
	}
	applied := mustPOSTJSON(t, server.URL+"/config/drafts/"+draftID+"/apply", map[string]any{})
	defer applied.Body.Close()
	if applied.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("apply without wiring status = %d body=%s", applied.StatusCode, readBody(t, applied))
	}

	missingScope := mustGET(t, server.URL+"/config/drafts")
	defer missingScope.Body.Close()
	if missingScope.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing scope status = %d", missingScope.StatusCode)
	}
}

func seedMission(t *testing.T, store port.Store, now time.Time) {
	t.Helper()
	mission := domain.MissionRevision{
		SchemaVersion: domain.SchemaVersionV1, ID: "revision_1", MissionID: "mission_1", Revision: 1,
		OriginalText: "control api mission", Purpose: "test", Status: domain.MissionActive,
		Provenance: "fixture", AcceptedAt: now,
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.AppendMissionRevision(mission); err != nil {
			return err
		}
		return tx.ActivateMissionRevision(mission.MissionID, mission.ID)
	}); err != nil {
		t.Fatal(err)
	}
}

func mustPOSTJSON(t *testing.T, url string, payload any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func mustGET(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func decodeJSON(t *testing.T, r io.Reader, dest any) {
	t.Helper()
	if err := json.NewDecoder(r).Decode(dest); err != nil {
		t.Fatal(err)
	}
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
