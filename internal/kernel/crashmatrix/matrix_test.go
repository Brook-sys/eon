package crashmatrix

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

// This package is intentionally a process-free invariant matrix. Abrupt process
// termination belongs to the repository-root integration tests; here we keep the
// four durable boundaries small enough to run in the core kernel suite.
func TestModelExecutorDurabilityInvariantMatrix(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 23, 8, 35, 0, 0, time.UTC)
	operationID := domain.OperationID("operation_crash_matrix")
	completion := domain.ModelCompletionResult{
		Text:         `{"matrix":"OK"}`,
		InputTokens:  12,
		OutputTokens: 4,
		Model:        "fixture-model",
		FinishReason: "stop",
	}
	hash, err := completion.Hash()
	if err != nil {
		t.Fatal(err)
	}
	reservation := domain.ModelCallReservation{
		SchemaVersion: domain.SchemaVersionV1,
		OperationID:   operationID,
		Attempt:       1,
		ModelCall:     1,
		BindingID:     "fixture-binding",
		ReservedAt:    now,
	}
	receipt := domain.ModelCompletionReceipt{
		SchemaVersion: domain.SchemaVersionV1,
		OperationID:   operationID,
		Attempt:       1,
		ModelCall:     1,
		Result:        completion,
		PayloadHash:   hash,
		RecordedAt:    now.Add(time.Second),
	}
	commit := domain.Commit{
		SchemaVersion:       domain.SchemaVersionV1,
		ID:                  "commit_crash_matrix",
		AcceptedChangeSetID: "changeset_crash_matrix",
		MissionRevision:     "revision_crash_matrix",
		BaseCommitID:        domain.GenesisCommitID,
		Version:             1,
		CommittedAt:         now.Add(2 * time.Second),
		ReceiptID:           "receipt_crash_matrix",
		IdempotencyKey:      "idem_crash_matrix",
	}
	commitReceipt := domain.CommitReceipt{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            commit.ReceiptID,
		CommitID:      commit.ID,
		ChangeSetID:   commit.AcceptedChangeSetID,
		OperationID:   operationID,
		Version:       commit.Version,
		ProducedAt:    commit.CommittedAt,
	}

	tests := []struct {
		name               string
		persistReservation bool
		persistReceipt     bool
		persistCommit      bool
		wantOutcome        string
	}{
		{name: "before_reservation", wantOutcome: "safe_to_dispatch"},
		{name: "after_reservation_before_receipt", persistReservation: true, wantOutcome: "burn_ambiguous_slot"},
		{name: "after_receipt_before_commit", persistReservation: true, persistReceipt: true, wantOutcome: "replay_without_provider"},
		{name: "after_commit", persistReservation: true, persistReceipt: true, persistCommit: true, wantOutcome: "terminal_skip"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			store := memory.New()
			if err := store.Update(context.Background(), func(tx port.Transaction) error {
				if test.persistReservation {
					if err := tx.AppendModelCallReservation(reservation); err != nil {
						return err
					}
				}
				if test.persistReceipt {
					if err := tx.AppendModelCompletionReceipt(receipt); err != nil {
						return err
					}
				}
				if test.persistCommit {
					// ApplyCommit requires the underlying mission revision in memory
					revision := domain.MissionRevision{
						SchemaVersion: domain.SchemaVersionV1,
						ID:            commit.MissionRevision,
						MissionID:     "mission_1",
						Revision:      1,
						OriginalText:  "matrix test",
						Purpose:       "test",
						Status:        domain.MissionActive,
						Provenance:    "user",
						AcceptedAt:    commit.CommittedAt,
					}
					if err := tx.AppendMissionRevision(revision); err != nil {
						return err
					}
					question := domain.Question{
						SchemaVersion:   domain.SchemaVersionV1,
						ID:              "q1",
						MissionRevision: commit.MissionRevision,
						Text:            "test",
						Origin:          "test",
						Relevance:       "test",
						AnswerCondition: "test",
					}
					if err := tx.CreateQuestion(question); err != nil {
						return err
					}
					candidate := domain.InquiryCandidate{
						SchemaVersion:    domain.SchemaVersionV1,
						ID:               "candidate_1",
						MissionRevision:  commit.MissionRevision,
						QuestionID:       "q1",
						DerivedFrom:      []string{"test"},
						ExpectedProgress: "test",
						Novelty:          "test",
						Risk:             domain.RiskLow,
						SourcePlan:       []string{"test"},
						AnswerCondition:  "test",
						StopCondition:    "test",
						ReviewAfter:      commit.CommittedAt,
					}
					if err := tx.CreateInquiryCandidate(candidate); err != nil {
						return err
					}
					inquiry := domain.Inquiry{
						SchemaVersion:   domain.SchemaVersionV1,
						ID:              "inquiry_1",
						CandidateID:     "candidate_1",
						MissionRevision: commit.MissionRevision,
						QuestionID:      "q1",
						AdmissionReason: "test",
						StopCondition:   "test",
						Budget: domain.Budget{
							ModelCalls: 1,
							Tokens:     100,
							Attempts:   1,
						},
						State: domain.StateRunning,
						Reevaluation: domain.ReevaluationCondition{
							Kind:      domain.ReevaluateLease,
							Reference: "lease:1",
						},
					}
					if err := tx.CreateInquiry(inquiry); err != nil {
						return err
					}
					spec := domain.OperationSpec{
						SchemaVersion:    domain.SchemaVersionV1,
						ID:               "spec_1",
						ContractVersion:  1,
						TemplateVersion:  1,
						InputSchema:      "test",
						OutputSchema:     "test",
						Budget:           domain.Budget{ModelCalls: 1, Tokens: 100, Attempts: 1},
						MaxOutputTokens:  10,
						SafetyMargin:     10,
						Validators:       []string{"schema"},
						RetryPolicy:      "none",
						FallbackPolicy:   "fail",
						MaximumAuthority: domain.AuthorityProposeOnly,
					}
					if err := tx.AppendOperationSpec(spec); err != nil {
						return err
					}
					op := domain.Operation{
						SchemaVersion:   domain.SchemaVersionV1,
						ID:              operationID,
						InquiryID:       "inquiry_1",
						MissionRevision: commit.MissionRevision,
						SpecID:          "spec_1",
						ExpectedOutput:  "changeset",
						IdempotencyKey:  "idem_crash_matrix",
						State:           domain.StateRunning,
						Reevaluation: domain.ReevaluationCondition{
							Kind:      domain.ReevaluateLease,
							Reference: "lease:1",
						},
					}
					if err := tx.CreateOperation(op); err != nil {
						return err
					}
					// ApplyCommit requires the underlying proposal and accepted state in memory
					proposal := domain.ProposedChangeSet{
						SchemaVersion:   domain.SchemaVersionV1,
						ID:              "proposed_crash_matrix",
						MissionRevision: commit.MissionRevision,
						OperationID:     operationID,
						BaseCommitID:    commit.BaseCommitID,
						ReadSet:         []string{"fragment_1"},
						Preconditions:   []string{},
						Changes: []domain.Change{
							{Kind: domain.ChangeAdd, EntityType: "observation", EntityID: "obs_matrix", PayloadRef: "payload_matrix"},
						},
						ExpectedDelta:  "one matrix observation",
						ValidatorIDs:   []string{"schema"},
						Provenance:     "model:fixture",
						IdempotencyKey: commit.IdempotencyKey,
					}
					if err := tx.AppendProposedChangeSet(proposal); err != nil {
						return err
					}
					rawOutput := domain.RawModelOutput{
						SchemaVersion: domain.SchemaVersionV1,
						ID:            "artifact_1",
						OperationID:   operationID,
						Model:         "fixture-model",
						Content:       "test",
						ContentHash:   "test",
						CreatedAt:     commit.CommittedAt,
					}
					if err := tx.AppendRawModelOutput(rawOutput); err != nil {
						return err
					}
					valReceipt := domain.ValidationReceipt{
						SchemaVersion: domain.SchemaVersionV1,
						ID:            "val_receipt_1",
						OperationID:   operationID,
						ChangeSetID:   proposal.ID,
						ValidatorID:   "schema",
						Passed:        true,
						ArtifactRef:   "artifact_1",
						ProducedAt:    commit.CommittedAt,
					}
					if err := tx.AppendValidationReceipt(valReceipt); err != nil {
						return err
					}
					accepted := domain.AcceptedChangeSet{
						SchemaVersion:        domain.SchemaVersionV1,
						ID:                   commit.AcceptedChangeSetID,
						ProposedChangeSetID:  proposal.ID,
						ValidationReceiptIDs: []domain.ReceiptID{"val_receipt_1"},
						AcceptedAt:           commit.CommittedAt,
						PolicyVersion:        "policy@matrix",
					}
					if err := tx.AppendAcceptedChangeSet(accepted); err != nil {
						return err
					}
					if err := tx.ApplyCommit(commit, commitReceipt, proposal.Changes); err != nil {
						// ApplyCommit checks for stale base commit on the mission revision head.
						// The memory store initializes headVersion to 0 if mission revision is missing.
						// As long as we use GenesisCommitID and Version 1 it succeeds without seeding a full mission tree.
						return err
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}

			got, err := inspectBoundary(context.Background(), store, operationID, commit.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.wantOutcome {
				t.Fatalf("outcome=%q, want %q", got, test.wantOutcome)
			}
		})
	}
}

func inspectBoundary(ctx context.Context, store port.Store, operationID domain.OperationID, commitID domain.CommitID) (string, error) {
	var outcome string
	err := store.View(ctx, func(r port.Reader) error {
		_, commitErr := r.Commit(commitID)
		if commitErr == nil {
			outcome = "terminal_skip"
			return nil
		}
		if !errors.Is(commitErr, port.ErrNotFound) {
			return commitErr
		}
		reservations, err := r.ModelCallReservations(operationID)
		if err != nil && !errors.Is(err, port.ErrNotFound) {
			return err
		}
		if len(reservations) == 0 {
			outcome = "safe_to_dispatch"
			return nil
		}
		if len(reservations) != 1 || reservations[0].ModelCall != 1 {
			return fmt.Errorf("reservations=%#v", reservations)
		}
		_, receiptErr := r.ModelCompletionReceipt(operationID, reservations[0].Attempt, reservations[0].ModelCall)
		switch {
		case receiptErr == nil:
			outcome = "replay_without_provider"
		case errors.Is(receiptErr, port.ErrNotFound):
			outcome = "burn_ambiguous_slot"
		default:
			return receiptErr
		}
		return nil
	})
	return outcome, err
}
