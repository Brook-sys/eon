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

func TestApplyCommitCascadesDependentArtifactStale(t *testing.T) {
	t.Parallel()
	store := memory.New()
	now := time.Date(2026, 7, 17, 1, 40, 0, 0, time.UTC)
	content := []byte("cascade evidence")
	digest := sha256.Sum256(content)
	hash := "sha256:" + hex.EncodeToString(digest[:])

	const (
		revisionID  = "revision_1"
		operationID = "operation_1"
		idemKey     = "idem_cascade_1"
		commitID    = "commit_1"
		entityID    = "entity_obs"
	)

	dependent := domain.KnowledgeArtifact{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            "artifact_dep",
		Kind:          "cited_claim_view",
		BaseCommitID:  domain.GenesisCommitID,
		Dependencies:  []string{"observation:" + entityID},
		ContentRef:    hash,
		Content:       "# dependent",
	}
	unrelated := domain.KnowledgeArtifact{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            "artifact_other",
		Kind:          "cited_claim_view",
		BaseCommitID:  domain.GenesisCommitID,
		Dependencies:  []string{"claim:claim_other@1"},
		ContentRef:    hash,
		Content:       "# other",
	}
	audit := domain.KnowledgeArtifact{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            "artifact_audit",
		Kind:          "artifact_refresh_report",
		BaseCommitID:  domain.GenesisCommitID,
		Dependencies:  []string{"observation:" + entityID},
		ContentRef:    hash,
		Content:       "# audit",
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
		operation := domain.Operation{
			SchemaVersion: domain.SchemaVersionV1, ID: operationID, InquiryID: inquiry.ID,
			MissionRevision: revision.ID, SpecID: spec.ID, ReadSet: []string{"seed"},
			InputRefs: []string{"artifact_dep"}, ExpectedOutput: "changeset",
			IdempotencyKey: idemKey, State: domain.StateReady,
			Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
		}
		if err := tx.CreateOperation(operation); err != nil {
			return err
		}
		if err := tx.AppendKnowledgeArtifact(dependent); err != nil {
			return err
		}
		if err := tx.AppendKnowledgeArtifact(unrelated); err != nil {
			return err
		}
		if err := tx.AppendKnowledgeArtifact(audit); err != nil {
			return err
		}
		raw := domain.RawModelOutput{
			SchemaVersion: domain.SchemaVersionV1, ID: "raw_1", OperationID: operation.ID,
			Model: "fixture", Content: "{}", ContentHash: hash, CreatedAt: now,
		}
		if err := tx.AppendRawModelOutput(raw); err != nil {
			return err
		}
		proposal := domain.ProposedChangeSet{
			SchemaVersion: domain.SchemaVersionV1, ID: "changeset_1", MissionRevision: revision.ID,
			OperationID: operation.ID, BaseCommitID: domain.GenesisCommitID,
			ReadSet: []string{"seed"}, Preconditions: []string{},
			Changes: []domain.Change{{
				Kind: domain.ChangeAdd, EntityType: "observation", EntityID: entityID, PayloadRef: "payload_1",
			}},
			ExpectedDelta: "one observation", ValidatorIDs: []string{"schema"},
			Provenance: "test", IdempotencyKey: idemKey,
		}
		if err := tx.AppendProposedChangeSet(proposal); err != nil {
			return err
		}
		validation := domain.ValidationReceipt{
			SchemaVersion: domain.SchemaVersionV1, ID: "receipt_val_1", OperationID: operation.ID,
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
			SchemaVersion: domain.SchemaVersionV1, ID: commitID, AcceptedChangeSetID: accepted.ID,
			MissionRevision: revision.ID, BaseCommitID: domain.GenesisCommitID, Version: 1,
			CommittedAt: now, ReceiptID: "commit_receipt_1", IdempotencyKey: idemKey,
		}
		receipt := domain.CommitReceipt{
			SchemaVersion: domain.SchemaVersionV1, ID: commit.ReceiptID, CommitID: commit.ID,
			ChangeSetID: accepted.ID, OperationID: operation.ID, Version: 1, ProducedAt: now,
		}
		return tx.ApplyCommit(commit, receipt, proposal.Changes)
	}); err != nil {
		t.Fatalf("apply commit: %v", err)
	}

	if err := store.View(context.Background(), func(r port.Reader) error {
		dep, err := r.KnowledgeArtifact(dependent.ID)
		if err != nil {
			return err
		}
		if !dep.Stale {
			t.Fatal("dependent artifact not marked stale by ApplyCommit cascade")
		}
		other, err := r.KnowledgeArtifact(unrelated.ID)
		if err != nil {
			return err
		}
		if other.Stale {
			t.Fatal("unrelated artifact incorrectly staled")
		}
		report, err := r.KnowledgeArtifact(audit.ID)
		if err != nil {
			return err
		}
		if report.Stale {
			t.Fatal("audit artifact must not cascade-stale")
		}
		head, err := r.HeadCommit(revisionID)
		if err != nil {
			return err
		}
		if head.ID != commitID {
			t.Fatalf("head = %s, want %s", head.ID, commitID)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
