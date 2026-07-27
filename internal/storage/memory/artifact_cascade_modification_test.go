package memory_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

func TestApplyCommitCascadesStaleOnParentArtifactReplacement(t *testing.T) {
	t.Parallel()
	store := memory.New()
	now := time.Date(2026, 7, 23, 14, 0, 0, 0, time.UTC)

	parentContentV1 := []byte("v1_content")
	parentHashV1 := "sha256:" + hex.EncodeToString(func(b [32]byte) []byte { return b[:] }(sha256.Sum256(parentContentV1)))

	const (
		revisionID   = "revision_1"
		operationID1 = "operation_1"
		operationID2 = "operation_2"
		idemKey1     = "idem_1"
		idemKey2     = "idem_2"
		commitID1    = "commit_1"
		commitID2    = "commit_2"
		parentEntity = "source_doc"
	)

	parentV1 := domain.KnowledgeArtifact{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            parentEntity,
		Kind:          "source_document",
		BaseCommitID:  domain.GenesisCommitID,
		Dependencies:  []string{"artifact:dummy"},
		ContentRef:    parentHashV1,
		Content:       "# v1",
	}

	derivedPlan := domain.KnowledgeArtifact{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            "derived_plan",
		Kind:          "plan_document",
		BaseCommitID:  domain.GenesisCommitID,
		Dependencies:  []string{"artifact:" + parentEntity},
		ContentRef:    "sha256:dummy",
		Content:       "# plan depending on v1",
	}

	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		revision := domain.MissionRevision{
			SchemaVersion: domain.SchemaVersionV1, ID: revisionID, MissionID: "mission_1", Revision: 1,
			OriginalText: "cascade", Purpose: "knowledge", Domains: []string{"test"}, Policies: []string{"cite"},
			Status: domain.MissionActive, Provenance: "test", AcceptedAt: now,
		}
		if err := tx.AppendMissionRevision(revision); err != nil {
			return err
		}
		spec := domain.OperationSpec{
			SchemaVersion: domain.SchemaVersionV1, ID: "extract@1", ContractVersion: 1, TemplateVersion: 1,
			InputSchema: "refs", OutputSchema: "changeset",
			Budget:          domain.Budget{ModelCalls: 1, Tokens: 1000, Attempts: 1},
			MaxOutputTokens: 100, SafetyMargin: 50, Validators: []string{"schema"},
			RetryPolicy: "none", FallbackPolicy: "fail", MaximumAuthority: domain.AuthorityProposeOnly,
		}
		if err := tx.AppendOperationSpec(spec); err != nil {
			return err
		}
		question := domain.Question{
			SchemaVersion: domain.SchemaVersionV1, ID: "question_1", MissionRevision: revision.ID,
			Text: "what?", Origin: "mission", Relevance: "primary", AnswerCondition: "evidence",
		}
		if err := tx.CreateQuestion(question); err != nil {
			return err
		}
		candidate := domain.InquiryCandidate{
			SchemaVersion: domain.SchemaVersionV1, ID: "candidate_1", MissionRevision: revision.ID,
			QuestionID: question.ID, DerivedFrom: []string{"gap"}, ExpectedProgress: "progress",
			Novelty: "new", Risk: domain.RiskLow, SourcePlan: []string{"fixture"},
			AnswerCondition: "evidence", StopCondition: "done", ReviewAfter: now.Add(time.Hour),
		}
		if err := tx.CreateInquiryCandidate(candidate); err != nil {
			return err
		}
		inquiry := domain.Inquiry{
			SchemaVersion: domain.SchemaVersionV1, ID: "inquiry_1", CandidateID: candidate.ID,
			MissionRevision: revision.ID, QuestionID: question.ID, AdmissionReason: "priority",
			StopCondition: "done", State: domain.StateReady,
			Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
		}
		if err := tx.CreateInquiry(inquiry); err != nil {
			return err
		}
		operation1 := domain.Operation{
			SchemaVersion: domain.SchemaVersionV1, ID: operationID1, InquiryID: inquiry.ID,
			MissionRevision: revision.ID, SpecID: spec.ID, ReadSet: []string{"seed"},
			InputRefs: []string{}, ExpectedOutput: "changeset",
			IdempotencyKey: idemKey1, State: domain.StateReady,
			Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
		}
		if err := tx.CreateOperation(operation1); err != nil {
			return err
		}
		operation2 := domain.Operation{
			SchemaVersion: domain.SchemaVersionV1, ID: operationID2, InquiryID: inquiry.ID,
			MissionRevision: revision.ID, SpecID: spec.ID, ReadSet: []string{"seed"},
			InputRefs: []string{}, ExpectedOutput: "changeset",
			IdempotencyKey: idemKey2, State: domain.StateReady,
			Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
		}
		if err := tx.CreateOperation(operation2); err != nil {
			return err
		}

		if err := tx.AppendKnowledgeArtifact(parentV1); err != nil {
			return err
		}
		if err := tx.AppendKnowledgeArtifact(derivedPlan); err != nil {
			return err
		}

		raw0 := domain.RawModelOutput{
			SchemaVersion: domain.SchemaVersionV1, ID: "raw_0", OperationID: operationID1,
			Model: "fixture", Content: "{}", ContentHash: "hash0", CreatedAt: now,
		}
		if err := tx.AppendRawModelOutput(raw0); err != nil {
			return err
		}

		priorProposal := domain.ProposedChangeSet{
			SchemaVersion: domain.SchemaVersionV1, ID: "changeset_0", MissionRevision: revision.ID,
			OperationID: operationID1, BaseCommitID: domain.GenesisCommitID,
			ReadSet: []string{"seed"}, Preconditions: []string{},
			Changes: []domain.Change{{
				Kind: domain.ChangeAdd, EntityType: "artifact", EntityID: parentEntity, PayloadRef: "payload_0",
			}},
			ExpectedDelta: "add parent", ValidatorIDs: []string{"schema"},
			Provenance: "test", IdempotencyKey: idemKey1,
		}
		if err := tx.AppendProposedChangeSet(priorProposal); err != nil {
			return err
		}

		val0 := domain.ValidationReceipt{
			SchemaVersion: domain.SchemaVersionV1, ID: "receipt_val_0", OperationID: operationID1,
			ChangeSetID: priorProposal.ID, ValidatorID: "schema", Passed: true, ArtifactRef: raw0.ID, ProducedAt: now,
		}
		if err := tx.AppendValidationReceipt(val0); err != nil {
			return err
		}

		priorAccepted := domain.AcceptedChangeSet{
			SchemaVersion: domain.SchemaVersionV1, ID: "accepted_0",
			ProposedChangeSetID: priorProposal.ID, ValidationReceiptIDs: []domain.ReceiptID{val0.ID},
			AcceptedAt: now, PolicyVersion: "test@1",
		}
		if err := tx.AppendAcceptedChangeSet(priorAccepted); err != nil {
			return err
		}
		priorCommit := domain.Commit{
			SchemaVersion: domain.SchemaVersionV1, ID: commitID1, AcceptedChangeSetID: priorAccepted.ID,
			MissionRevision: revision.ID, BaseCommitID: domain.GenesisCommitID, Version: 1,
			CommittedAt: now, ReceiptID: "commit_receipt_0", IdempotencyKey: idemKey1,
		}
		if err := tx.ApplyCommit(priorCommit, domain.CommitReceipt{SchemaVersion: domain.SchemaVersionV1, ID: "commit_receipt_0", CommitID: priorCommit.ID, ChangeSetID: priorAccepted.ID, OperationID: operationID1, Version: 1, ProducedAt: now}, priorProposal.Changes); err != nil {
			return err
		}

		raw := domain.RawModelOutput{
			SchemaVersion: domain.SchemaVersionV1, ID: "raw_1", OperationID: operationID2,
			Model: "fixture", Content: "{}", ContentHash: "hash1", CreatedAt: now,
		}
		if err := tx.AppendRawModelOutput(raw); err != nil {
			return err
		}
		proposal := domain.ProposedChangeSet{
			SchemaVersion: domain.SchemaVersionV1, ID: "changeset_1", MissionRevision: revision.ID,
			OperationID: operationID2, BaseCommitID: priorCommit.ID,
			ReadSet: []string{"seed"}, Preconditions: []string{},
			Changes: []domain.Change{{
				Kind: domain.ChangeReplace, EntityType: "artifact", EntityID: parentEntity, PayloadRef: "payload_1",
			}},
			ExpectedDelta: "modify parent artifact", ValidatorIDs: []string{"schema"},
			Provenance: "test", IdempotencyKey: idemKey2,
		}
		if err := tx.AppendProposedChangeSet(proposal); err != nil {
			return err
		}
		validation := domain.ValidationReceipt{
			SchemaVersion: domain.SchemaVersionV1, ID: "receipt_val_1", OperationID: operationID2,
			ChangeSetID: proposal.ID, ValidatorID: "schema", Passed: true, ArtifactRef: raw.ID, ProducedAt: now,
		}
		if err := tx.AppendValidationReceipt(validation); err != nil {
			return err
		}
		accepted := domain.AcceptedChangeSet{
			SchemaVersion: domain.SchemaVersionV1, ID: "accepted_1",
			ProposedChangeSetID: proposal.ID, ValidationReceiptIDs: []domain.ReceiptID{validation.ID},
			AcceptedAt: now, PolicyVersion: "test@1",
		}
		if err := tx.AppendAcceptedChangeSet(accepted); err != nil {
			return err
		}
		commit := domain.Commit{
			SchemaVersion: domain.SchemaVersionV1, ID: commitID2, AcceptedChangeSetID: accepted.ID,
			MissionRevision: revision.ID, BaseCommitID: priorCommit.ID, Version: 2,
			CommittedAt: now, ReceiptID: "commit_receipt_1", IdempotencyKey: idemKey2,
		}
		receipt := domain.CommitReceipt{
			SchemaVersion: domain.SchemaVersionV1, ID: commit.ReceiptID, CommitID: commit.ID,
			ChangeSetID: accepted.ID, OperationID: operationID2, Version: 2, ProducedAt: now,
		}
		return tx.ApplyCommit(commit, receipt, proposal.Changes)
	}); err != nil {
		t.Fatalf("apply commit: %v", err)
	}

	if err := store.View(context.Background(), func(r port.Reader) error {
		derived, err := r.KnowledgeArtifact(derivedPlan.ID)
		if err != nil {
			return err
		}
		if !derived.Stale {
			t.Fatal("derived plan was not marked stale after parent artifact was replaced")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
