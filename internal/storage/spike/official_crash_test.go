package spike

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

func TestInspectOfficialMutationRequiresCompleteConsistentVisibility(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	refs := OfficialMutationRefs{
		EventID: "event_official_1", CommitID: "commit_1", ReceiptID: "receipt_commit_1",
		MissionRevision: "revision_1", IdempotencyKey: "idem_1", CanonicalType: "observation", CanonicalID: "observation_1",
	}
	outcome, err := InspectOfficialMutation(ctx, store, refs)
	if err != nil || outcome != OutcomeNotApplied {
		t.Fatalf("empty store outcome=%s err=%v", outcome, err)
	}

	if err := store.Update(ctx, func(tx port.Transaction) error {
		_, err := tx.AppendEvent(domain.Event{SchemaVersion: 1, ID: refs.EventID, Kind: "knowledge.commit.applied", OccurredAt: fixedOfficialTime(), CommitID: refs.CommitID})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	outcome, err = InspectOfficialMutation(ctx, store, refs)
	if err != nil || outcome != OutcomeInvalidPartial {
		t.Fatalf("event-only outcome=%s err=%v", outcome, err)
	}

	store = memory.New()
	seedOfficialMutation(t, store, refs)
	outcome, err = InspectOfficialMutation(ctx, store, refs)
	if err != nil || outcome != OutcomeApplied {
		t.Fatalf("complete outcome=%s err=%v", outcome, err)
	}
}

func TestInspectOfficialMutationRejectsCrossLinkedRecords(t *testing.T) {
	store := memory.New()
	refs := OfficialMutationRefs{EventID: "event_official_1", CommitID: "commit_1", ReceiptID: "receipt_commit_1", MissionRevision: "revision_1", IdempotencyKey: "idem_1", CanonicalType: "observation", CanonicalID: "observation_1"}
	seedOfficialMutation(t, store, refs)
	refs.EventID = "event_wrong_link"
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		_, err := tx.AppendEvent(domain.Event{SchemaVersion: 1, ID: refs.EventID, Kind: "knowledge.commit.applied", OccurredAt: fixedOfficialTime(), CommitID: "commit_other"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	outcome, err := InspectOfficialMutation(context.Background(), store, refs)
	if err != nil || outcome != OutcomeInvalidPartial {
		t.Fatalf("cross-linked outcome=%s err=%v", outcome, err)
	}
}

func TestApplyOfficialMutationProducesCompleteVisibleSet(t *testing.T) {
	store := memory.New()
	refs := OfficialMutationRefs{EventID: "event_apply_1", CommitID: "commit_apply_1", ReceiptID: "receipt_apply_1", MissionRevision: "revision_apply_1", IdempotencyKey: "idem_apply_1", CanonicalType: "observation", CanonicalID: "observation_apply_1"}
	mutation := OfficialMutation{SchemaVersion: 1, Refs: refs, OccurredAt: fixedOfficialTime()}
	if err := ApplyOfficialMutation(context.Background(), store, mutation); err != nil {
		t.Fatal(err)
	}
	outcome, err := InspectOfficialMutation(context.Background(), store, refs)
	if err != nil || outcome != OutcomeApplied {
		t.Fatalf("outcome=%s err=%v", outcome, err)
	}
}

func seedOfficialMutation(t *testing.T, store port.Store, refs OfficialMutationRefs) {
	t.Helper()
	now := fixedOfficialTime()
	err := store.Update(context.Background(), func(tx port.Transaction) error {
		revision := domain.MissionRevision{SchemaVersion: 1, ID: refs.MissionRevision, MissionID: "mission_1", Revision: 1, OriginalText: "investigate", Purpose: "knowledge", Domains: []string{"science"}, Policies: []string{"cite"}, Status: domain.MissionActive, Provenance: "fixture", AcceptedAt: now}
		if err := tx.AppendMissionRevision(revision); err != nil {
			return err
		}
		spec := domain.OperationSpec{SchemaVersion: 1, ID: "extract@1", ContractVersion: 1, TemplateVersion: 1, InputSchema: "refs", OutputSchema: "changeset", Budget: domain.Budget{ModelCalls: 1, Tokens: 1000, Attempts: 1}, MaxOutputTokens: 100, SafetyMargin: 50, Validators: []string{"schema"}, RetryPolicy: "none", FallbackPolicy: "fail", MaximumAuthority: domain.AuthorityProposeOnly}
		if err := tx.AppendOperationSpec(spec); err != nil {
			return err
		}
		question := domain.Question{SchemaVersion: 1, ID: "question_1", MissionRevision: revision.ID, Text: "what?", Origin: "mission", Relevance: "primary", AnswerCondition: "evidence"}
		if err := tx.CreateQuestion(question); err != nil {
			return err
		}
		candidate := domain.InquiryCandidate{SchemaVersion: 1, ID: "candidate_1", MissionRevision: revision.ID, QuestionID: question.ID, DerivedFrom: []string{"gap"}, ExpectedProgress: "progress", Novelty: "new", Risk: domain.RiskLow, SourcePlan: []string{"fixture"}, AnswerCondition: "evidence", StopCondition: "done", ReviewAfter: now.Add(time.Hour)}
		if err := tx.CreateInquiryCandidate(candidate); err != nil {
			return err
		}
		inquiry := domain.Inquiry{SchemaVersion: 1, ID: "inquiry_1", CandidateID: candidate.ID, MissionRevision: revision.ID, QuestionID: question.ID, AdmissionReason: "priority", StopCondition: "done", State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady}}
		if err := tx.CreateInquiry(inquiry); err != nil {
			return err
		}
		operation := domain.Operation{SchemaVersion: 1, ID: "operation_1", InquiryID: inquiry.ID, MissionRevision: revision.ID, SpecID: spec.ID, ReadSet: []string{"fragment_1"}, InputRefs: []string{"artifact_1"}, ExpectedOutput: "changeset", IdempotencyKey: refs.IdempotencyKey, State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady}}
		if err := tx.CreateOperation(operation); err != nil {
			return err
		}
		raw := domain.RawModelOutput{SchemaVersion: 1, ID: "artifact_validation_1", OperationID: operation.ID, Model: "fixture", Content: "{}", ContentHash: "sha256:fixture", CreatedAt: now}
		if err := tx.AppendRawModelOutput(raw); err != nil {
			return err
		}
		proposal := domain.ProposedChangeSet{SchemaVersion: 1, ID: "changeset_1", MissionRevision: revision.ID, OperationID: operation.ID, BaseCommitID: domain.GenesisCommitID, ReadSet: []string{"fragment_1"}, Changes: []domain.Change{{Kind: domain.ChangeAdd, EntityType: refs.CanonicalType, EntityID: refs.CanonicalID, PayloadRef: "payload_1"}}, ExpectedDelta: "one entity", ValidatorIDs: []string{"schema"}, Provenance: "fixture", IdempotencyKey: refs.IdempotencyKey}
		if err := tx.AppendProposedChangeSet(proposal); err != nil {
			return err
		}
		validation := domain.ValidationReceipt{SchemaVersion: 1, ID: "receipt_validation_1", OperationID: operation.ID, ChangeSetID: proposal.ID, ValidatorID: "schema", Passed: true, ArtifactRef: raw.ID, ProducedAt: now}
		if err := tx.AppendValidationReceipt(validation); err != nil {
			return err
		}
		accepted := domain.AcceptedChangeSet{SchemaVersion: 1, ID: "accepted_1", ProposedChangeSetID: proposal.ID, ValidationReceiptIDs: []domain.ReceiptID{validation.ID}, AcceptedAt: now, PolicyVersion: "policy@1"}
		if err := tx.AppendAcceptedChangeSet(accepted); err != nil {
			return err
		}
		if _, err := tx.ReserveIdempotency(domain.IdempotencyRecord{SchemaVersion: 1, Key: refs.IdempotencyKey, OperationID: operation.ID, Intent: "apply changeset", Status: domain.IdempotencyReserved, ReservedAt: now}); err != nil {
			return err
		}
		commit := domain.Commit{SchemaVersion: 1, ID: refs.CommitID, AcceptedChangeSetID: accepted.ID, MissionRevision: revision.ID, BaseCommitID: domain.GenesisCommitID, Version: 1, CommittedAt: now, ReceiptID: refs.ReceiptID, IdempotencyKey: refs.IdempotencyKey}
		receipt := domain.CommitReceipt{SchemaVersion: 1, ID: refs.ReceiptID, CommitID: commit.ID, ChangeSetID: accepted.ID, OperationID: operation.ID, Version: 1, ProducedAt: now}
		if err := tx.ApplyCommit(commit, receipt, proposal.Changes); err != nil {
			return err
		}
		if _, err := tx.CompleteIdempotency(refs.IdempotencyKey, refs.ReceiptID, string(refs.CommitID), now); err != nil {
			return err
		}
		_, err := tx.AppendEvent(domain.Event{SchemaVersion: 1, ID: refs.EventID, Kind: "knowledge.commit.applied", OccurredAt: now, MissionRevision: revision.ID, OperationID: operation.ID, CommitID: commit.ID, PayloadRef: string(proposal.ID)})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func fixedOfficialTime() time.Time { return time.Date(2026, 7, 15, 20, 0, 0, 0, time.UTC) }
