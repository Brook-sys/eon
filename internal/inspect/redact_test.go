package inspect_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/inspect"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

func TestRedactSensitiveTextMasksSecrets(t *testing.T) {
	in := "Authorization: Bearer sk-abc1234567890xyz password=super-secret-value TELEGRAM_BOT_TOKEN=123456789:AAHxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx ok"
	out, n := inspect.RedactSensitiveText(in)
	if n == 0 {
		t.Fatalf("expected matches, got 0 for %q", out)
	}
	if strings.Contains(out, "sk-abc") || strings.Contains(out, "super-secret-value") || strings.Contains(out, "AAHxxxx") {
		t.Fatalf("secrets leaked: %q", out)
	}
	if !strings.Contains(out, "[redacted]") {
		t.Fatalf("missing redaction marker: %q", out)
	}
}

func TestRedactRawModelOutputTruncates(t *testing.T) {
	raw := domain.RawModelOutput{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            "artifact_1",
		OperationID:   "operation_1",
		Model:         "fake",
		Content:       strings.Repeat("a", 100),
		ContentHash:   "hash",
		CreatedAt:     time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC),
	}
	got, report := inspect.RedactRawModelOutput(raw, 40)
	if !report.Applied || report.TruncatedBytes == 0 {
		t.Fatalf("report = %#v content=%q", report, got.Content)
	}
	if len(got.Content) > 40+len("\n…[truncated]") {
		t.Fatalf("content too large: %d", len(got.Content))
	}
	if got.ContentHash != raw.ContentHash {
		t.Fatalf("hash must remain integrity anchor: %q vs %q", got.ContentHash, raw.ContentHash)
	}
}

func TestOperationInspectorLoadsRawOutputsAndHTTPRedacts(t *testing.T) {
	store := memory.New()
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	mission := domain.MissionRevision{
		SchemaVersion: domain.SchemaVersionV1, ID: "revision_1", MissionID: "mission_1", Revision: 1,
		OriginalText: "inspect", Purpose: "inspect", Status: domain.MissionActive,
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
		Text: "What?", Origin: "fixture", Relevance: "core", AnswerCondition: "done",
	}
	candidate := domain.InquiryCandidate{
		SchemaVersion: domain.SchemaVersionV1, ID: "candidate_1", MissionRevision: mission.ID, QuestionID: question.ID,
		DerivedFrom: []string{"mission"}, ExpectedProgress: "one", Novelty: "new", EstimatedCost: domain.Budget{Tokens: 50},
		Risk: domain.RiskLow, SourcePlan: []string{"fixture"}, AnswerCondition: "done", StopCondition: "done", ReviewAfter: now.Add(time.Hour),
	}
	inquiry := domain.Inquiry{
		SchemaVersion: domain.SchemaVersionV1, ID: "inquiry_1", CandidateID: candidate.ID, MissionRevision: mission.ID,
		QuestionID: question.ID, AdmissionReason: "seed", Budget: domain.Budget{Tokens: 50}, StopCondition: "done",
		State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
	}
	operation := domain.Operation{
		SchemaVersion: domain.SchemaVersionV1, ID: "operation_1", InquiryID: inquiry.ID, MissionRevision: mission.ID,
		SpecID: spec.ID, ReadSet: []string{"fragment_1"}, InputRefs: []string{"artifact_1"}, ExpectedOutput: "proposed_change_set",
		IdempotencyKey: "idem_op_raw", State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
	}
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
		Model: "fake", Content: `{"ok":true,"authorization":"Bearer sk-abcdefghijklmnopqrstuvwxyz"}`, ContentHash: "hash_validation_1", CreatedAt: now,
	}
	validation := domain.ValidationReceipt{
		SchemaVersion: domain.SchemaVersionV1, ID: "validation_1", OperationID: operation.ID,
		ChangeSetID: proposed.ID, ValidatorID: "schema", Passed: true, ArtifactRef: raw.ID, ProducedAt: now,
	}
	commitReceipt := domain.CommitReceipt{
		SchemaVersion: domain.SchemaVersionV1, ID: "commit_receipt_1", CommitID: commit.ID,
		ChangeSetID: accepted.ID, OperationID: operation.ID, Version: 1, ProducedAt: now,
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
	if len(detail.RawOutputs) != 1 || detail.RawOutputs[0].ID != raw.ID {
		t.Fatalf("raw outputs = %#v", detail.RawOutputs)
	}
	if !strings.Contains(detail.RawOutputs[0].Content, "sk-") {
		t.Fatalf("projector must preserve durable content before HTTP redaction: %#v", detail.RawOutputs[0])
	}

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
	var body inspect.OperationDetailResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.RawOutputs) != 1 {
		t.Fatalf("http raw outputs = %#v", body.RawOutputs)
	}
	if strings.Contains(body.RawOutputs[0].Content, "sk-") || strings.Contains(body.RawOutputs[0].Content, "Bearer sk-") {
		t.Fatalf("secret leaked over HTTP: %q", body.RawOutputs[0].Content)
	}
	if !body.Redaction.Applied || body.Redaction.SecretMatches == 0 {
		t.Fatalf("redaction report = %#v", body.Redaction)
	}
	if body.RawOutputs[0].ContentHash != raw.ContentHash {
		t.Fatalf("content hash changed: %q", body.RawOutputs[0].ContentHash)
	}

	// Durable store still holds the original content.
	var stored domain.RawModelOutput
	if err := store.View(context.Background(), func(r port.Reader) error {
		var err error
		stored, err = r.RawModelOutput(raw.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stored.Content, "sk-") {
		t.Fatalf("store content should remain unrewritten: %q", stored.Content)
	}

	commitResp, err := http.Get(server.URL + "/commits/" + string(commit.ID))
	if err != nil {
		t.Fatal(err)
	}
	defer commitResp.Body.Close()
	if commitResp.StatusCode != http.StatusOK {
		t.Fatalf("commit status = %d", commitResp.StatusCode)
	}
	var commitBody inspect.CommitDetailResponse
	if err := json.NewDecoder(commitResp.Body).Decode(&commitBody); err != nil {
		t.Fatal(err)
	}
	if commitBody.Proposed == nil || commitBody.Accepted == nil {
		t.Fatalf("commit body incomplete: %#v", commitBody)
	}
}
