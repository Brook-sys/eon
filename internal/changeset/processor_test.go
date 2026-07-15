package changeset

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func TestDecodeStrictRejectsNonCanonicalJSON(t *testing.T) {
	valid := proposalText(t, proposal("operation_1", "idem_1", domain.GenesisCommitID, "entity_1"))
	tests := []struct {
		name string
		text string
	}{
		{name: "unknown field", text: strings.Replace(valid, `"expected_delta":`, `"unknown":true,"expected_delta":`, 1)},
		{name: "case variant", text: strings.Replace(valid, `"schema_version"`, `"Schema_Version"`, 1)},
		{name: "duplicate key", text: strings.Replace(valid, `"schema_version":1`, `"schema_version":1,"schema_version":1`, 1)},
		{name: "trailing value", text: valid + ` {}`},
		{name: "markdown fence", text: "```json\n" + valid + "\n```"},
		{name: "unsupported link", text: strings.Replace(valid, `"kind":"ADD"`, `"kind":"LINK"`, 1)},
		{name: "null required array", text: strings.Replace(valid, `"preconditions":[]`, `"preconditions":null`, 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeStrict(test.text, 1<<20); err == nil {
				t.Fatal("invalid output was accepted")
			}
		})
	}
	if _, err := DecodeStrict(valid, int64(len(valid)-1)); err == nil {
		t.Fatal("oversized output was accepted")
	}
}

func TestProcessorPreservesValidatesAndCommitsAtomically(t *testing.T) {
	store := memory.New()
	seed(t, store)
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	processor, err := New(Config{Store: store, Clock: source.NewManualClock(now), IDs: source.NewSequenceIDGenerator(1), PolicyVersion: "policy@1"})
	if err != nil {
		t.Fatal(err)
	}
	text := proposalText(t, proposal("operation_1", "idem_1", domain.GenesisCommitID, "entity_1"))
	result := port.CompletionResult{Text: text, Model: "fake-model"}

	commit, err := processor.Process(context.Background(), "operation_1", result)
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if commit.Version != 1 || commit.BaseCommitID != domain.GenesisCommitID {
		t.Fatalf("commit = %#v", commit)
	}

	if err := store.View(context.Background(), func(r port.Reader) error {
		raw, err := r.RawModelOutput("artifact_0000000000000001")
		if err != nil {
			return err
		}
		if raw.Content != text {
			t.Fatal("raw model text was changed")
		}
		proposal, err := r.ProposedChangeSet("changeset_1")
		if err != nil {
			return err
		}
		if proposal.OperationID != "operation_1" {
			t.Fatalf("proposal = %#v", proposal)
		}
		accepted, err := r.AcceptedChangeSet(commit.AcceptedChangeSetID)
		if err != nil {
			return err
		}
		if len(accepted.ValidationReceiptIDs) != 1 {
			t.Fatalf("accepted = %#v", accepted)
		}
		validation, err := r.ValidationReceipt(accepted.ValidationReceiptIDs[0])
		if err != nil {
			return err
		}
		if validation.ValidatorID != "schema" || !validation.Passed {
			t.Fatalf("validation = %#v", validation)
		}
		commitReceipt, err := r.CommitReceipt(commit.ReceiptID)
		if err != nil {
			return err
		}
		if commitReceipt.CommitID != commit.ID {
			t.Fatalf("commit receipt = %#v", commitReceipt)
		}
		entity, err := r.CanonicalEntity("observation", "entity_1")
		if err != nil {
			return err
		}
		if entity.PayloadRef != "payload_1" || entity.CommitID != commit.ID {
			t.Fatalf("entity = %#v", entity)
		}
		events, err := r.Events(0, 10)
		if err != nil {
			return err
		}
		if len(events) != 1 || events[0].CommitID != commit.ID {
			t.Fatalf("events = %#v", events)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	replay, err := processor.Process(context.Background(), "operation_1", result)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay != commit {
		t.Fatalf("replay = %#v, want %#v", replay, commit)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		events, err := r.Events(0, 10)
		if err == nil && len(events) != 1 {
			t.Fatalf("replay added event: %#v", events)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestProcessorPreservesInvalidRawOutputWithoutOfficialEffect(t *testing.T) {
	store := memory.New()
	seed(t, store)
	processor, err := New(Config{Store: store, Clock: source.NewManualClock(time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)), IDs: source.NewSequenceIDGenerator(1), PolicyVersion: "policy@1"})
	if err != nil {
		t.Fatal(err)
	}
	bad := `{"schema_version":1,"unknown":true}`
	if _, err := processor.Process(context.Background(), "operation_1", port.CompletionResult{Text: bad, Model: "fake-model"}); err == nil {
		t.Fatal("invalid output was accepted")
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		raw, err := r.RawModelOutput("artifact_0000000000000001")
		if err != nil {
			return err
		}
		if raw.Content != bad {
			t.Fatalf("raw = %#v", raw)
		}
		_, proposalErr := r.ProposedChangeSet("changeset_1")
		if !errors.Is(proposalErr, port.ErrNotFound) {
			t.Fatalf("proposal error = %v", proposalErr)
		}
		_, entityErr := r.CanonicalEntity("observation", "entity_1")
		if !errors.Is(entityErr, port.ErrNotFound) {
			t.Fatalf("entity error = %v", entityErr)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestProcessorRollsBackStaleCommitChain(t *testing.T) {
	store := memory.New()
	seed(t, store)
	clock := source.NewManualClock(time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC))
	processor, err := New(Config{Store: store, Clock: clock, IDs: source.NewSequenceIDGenerator(1), PolicyVersion: "policy@1"})
	if err != nil {
		t.Fatal(err)
	}
	first := proposalText(t, proposal("operation_1", "idem_1", domain.GenesisCommitID, "entity_1"))
	if _, err := processor.Process(context.Background(), "operation_1", port.CompletionResult{Text: first, Model: "fake-model"}); err != nil {
		t.Fatal(err)
	}

	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		op, err := tx.Operation("operation_1")
		if err != nil {
			return err
		}
		op.ID, op.IdempotencyKey = "operation_2", "idem_2"
		return tx.CreateOperation(op)
	}); err != nil {
		t.Fatal(err)
	}
	staleProposal := proposal("operation_2", "idem_2", domain.GenesisCommitID, "entity_2")
	staleProposal.ID = "changeset_2"
	if _, err := processor.Process(context.Background(), "operation_2", port.CompletionResult{Text: proposalText(t, staleProposal), Model: "fake-model"}); !errors.Is(err, port.ErrConflict) {
		t.Fatalf("stale process error = %v", err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		_, proposalErr := r.ProposedChangeSet("changeset_2")
		if !errors.Is(proposalErr, port.ErrNotFound) {
			t.Fatalf("stale proposal partially committed: %v", proposalErr)
		}
		_, entityErr := r.CanonicalEntity("observation", "entity_2")
		if !errors.Is(entityErr, port.ErrNotFound) {
			t.Fatalf("stale entity partially committed: %v", entityErr)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func proposal(operationID domain.OperationID, key domain.IdempotencyKey, base domain.CommitID, entityID string) domain.ProposedChangeSet {
	return domain.ProposedChangeSet{SchemaVersion: 1, ID: "changeset_1", MissionRevision: "revision_1", OperationID: operationID, BaseCommitID: base, ReadSet: []string{"fragment_1"}, Preconditions: []string{}, Changes: []domain.Change{{Kind: domain.ChangeAdd, EntityType: "observation", EntityID: entityID, PayloadRef: "payload_1"}}, ExpectedDelta: "one observation", ValidatorIDs: []string{"schema"}, Provenance: "model:fake-model", IdempotencyKey: key}
}

func proposalText(t *testing.T, proposal domain.ProposedChangeSet) string {
	t.Helper()
	encoded, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func seed(t *testing.T, store port.Store) {
	t.Helper()
	err := store.Update(context.Background(), func(tx port.Transaction) error {
		revision := domain.MissionRevision{SchemaVersion: 1, ID: "revision_1", MissionID: "mission_1", Revision: 1, OriginalText: "investigate", Purpose: "knowledge", Domains: []string{"science"}, Policies: []string{"cite"}, Status: domain.MissionActive, Provenance: "user", AcceptedAt: time.Date(2026, 7, 15, 11, 0, 0, 0, time.UTC)}
		if err := tx.AppendMissionRevision(revision); err != nil {
			return err
		}
		spec := domain.OperationSpec{SchemaVersion: 1, ID: "extract@1", ContractVersion: 1, TemplateVersion: 1, InputSchema: "refs", OutputSchema: "proposed changeset", Budget: domain.Budget{ModelCalls: 1, Tokens: 1000, Attempts: 1}, MaxOutputTokens: 100, SafetyMargin: 50, Validators: []string{"schema"}, RetryPolicy: "none", FallbackPolicy: "fail", MaximumAuthority: domain.AuthorityProposeOnly}
		if err := tx.AppendOperationSpec(spec); err != nil {
			return err
		}
		question := domain.Question{SchemaVersion: 1, ID: "question_1", MissionRevision: revision.ID, Text: "what?", Origin: "mission", Relevance: "primary", AnswerCondition: "evidence"}
		if err := tx.CreateQuestion(question); err != nil {
			return err
		}
		candidate := domain.InquiryCandidate{SchemaVersion: 1, ID: "candidate_1", MissionRevision: revision.ID, QuestionID: question.ID, DerivedFrom: []string{"gap_1"}, ExpectedProgress: "reduce uncertainty", Novelty: "new", Risk: domain.RiskLow, SourcePlan: []string{"fixtures"}, AnswerCondition: "evidence", StopCondition: "done", ReviewAfter: time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)}
		if err := tx.CreateInquiryCandidate(candidate); err != nil {
			return err
		}
		inquiry := domain.Inquiry{SchemaVersion: 1, ID: "inquiry_1", CandidateID: candidate.ID, MissionRevision: revision.ID, QuestionID: question.ID, AdmissionReason: "priority", StopCondition: "done", State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady}}
		if err := tx.CreateInquiry(inquiry); err != nil {
			return err
		}
		operation := domain.Operation{SchemaVersion: 1, ID: "operation_1", InquiryID: inquiry.ID, MissionRevision: revision.ID, SpecID: spec.ID, ReadSet: []string{"fragment_1"}, InputRefs: []string{"artifact_1"}, ExpectedOutput: "proposed_change_set", IdempotencyKey: "idem_1", State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady}}
		return tx.CreateOperation(operation)
	})
	if err != nil {
		t.Fatal(err)
	}
}
