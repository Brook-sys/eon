package spike

import (
	"context"
	"errors"
	"fmt"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// OfficialMutationRefs identifies every independently observable record that
// makes one canonical changeset application official. Crash recovery must see
// either none of these records or a complete, mutually consistent set.
type OfficialMutationRefs struct {
	EventID         domain.EventID           `json:"event_id"`
	CommitID        domain.CommitID          `json:"commit_id"`
	ReceiptID       domain.ReceiptID         `json:"receipt_id"`
	MissionRevision domain.MissionRevisionID `json:"mission_revision"`
	IdempotencyKey  domain.IdempotencyKey    `json:"idempotency_key"`
	CanonicalType   string                   `json:"canonical_type"`
	CanonicalID     string                   `json:"canonical_id"`
}

func (r OfficialMutationRefs) validate() error {
	if r.EventID == "" || r.CommitID == "" || r.ReceiptID == "" || r.MissionRevision == "" || r.IdempotencyKey == "" || r.CanonicalType == "" || r.CanonicalID == "" {
		return errors.New("official mutation references are incomplete")
	}
	return nil
}

// OfficialMutation is the self-contained reference mutation used by the
// storage spike. Its prerequisite graph and the six independently observable
// official records are committed by one Store.Update call so adapter
// failpoints exercise the same atomicity boundary as the runtime.
type OfficialMutation struct {
	SchemaVersion int                  `json:"schema_version"`
	Refs          OfficialMutationRefs `json:"refs"`
	OccurredAt    time.Time            `json:"occurred_at"`
}

func (m OfficialMutation) validate() error {
	if m.SchemaVersion != 1 {
		return fmt.Errorf("unsupported official mutation schema version %d", m.SchemaVersion)
	}
	if err := m.Refs.validate(); err != nil {
		return err
	}
	if m.OccurredAt.IsZero() {
		return errors.New("official mutation occurred_at is required")
	}
	return nil
}

// ApplyOfficialMutation installs a minimal valid lineage and makes one
// canonical changeset official. The fixture is intentionally dependency-free
// and deterministic so SQLite and Dolt receive identical logical work.
func ApplyOfficialMutation(ctx context.Context, store port.Store, mutation OfficialMutation) error {
	if err := mutation.validate(); err != nil {
		return err
	}
	r := mutation.Refs
	now := mutation.OccurredAt.UTC()
	return store.Update(ctx, func(tx port.Transaction) error {
		revision := domain.MissionRevision{SchemaVersion: 1, ID: r.MissionRevision, MissionID: "mission_spike", Revision: 1, OriginalText: "investigate storage atomicity", Purpose: "knowledge", Domains: []string{"storage"}, Policies: []string{"cite"}, Status: domain.MissionActive, Provenance: "storage-spike", AcceptedAt: now}
		if err := tx.AppendMissionRevision(revision); err != nil {
			return err
		}
		spec := domain.OperationSpec{SchemaVersion: 1, ID: "storage-spike-apply@1", ContractVersion: 1, TemplateVersion: 1, InputSchema: "refs", OutputSchema: "changeset", Budget: domain.Budget{ModelCalls: 1, Tokens: 1000, Attempts: 1}, MaxOutputTokens: 100, SafetyMargin: 50, Validators: []string{"schema"}, RetryPolicy: "none", FallbackPolicy: "fail", MaximumAuthority: domain.AuthorityProposeOnly}
		if err := tx.AppendOperationSpec(spec); err != nil {
			return err
		}
		question := domain.Question{SchemaVersion: 1, ID: "question_spike", MissionRevision: revision.ID, Text: "is the official mutation atomic?", Origin: "storage-spike", Relevance: "primary", AnswerCondition: "durable reopen"}
		if err := tx.CreateQuestion(question); err != nil {
			return err
		}
		candidate := domain.InquiryCandidate{SchemaVersion: 1, ID: "candidate_spike", MissionRevision: revision.ID, QuestionID: question.ID, DerivedFrom: []string{"storage-spike"}, ExpectedProgress: "atomicity evidence", Novelty: "measured", Risk: domain.RiskLow, SourcePlan: []string{"fixture"}, AnswerCondition: "durable reopen", StopCondition: "classified", ReviewAfter: now.Add(time.Hour)}
		if err := tx.CreateInquiryCandidate(candidate); err != nil {
			return err
		}
		inquiry := domain.Inquiry{SchemaVersion: 1, ID: "inquiry_spike", CandidateID: candidate.ID, MissionRevision: revision.ID, QuestionID: question.ID, AdmissionReason: "storage comparison", StopCondition: "classified", State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady}}
		if err := tx.CreateInquiry(inquiry); err != nil {
			return err
		}
		operation := domain.Operation{SchemaVersion: 1, ID: "operation_spike", InquiryID: inquiry.ID, MissionRevision: revision.ID, SpecID: spec.ID, ReadSet: []string{"fragment_spike"}, InputRefs: []string{"artifact_spike"}, ExpectedOutput: "changeset", IdempotencyKey: r.IdempotencyKey, State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady}}
		if err := tx.CreateOperation(operation); err != nil {
			return err
		}
		raw := domain.RawModelOutput{SchemaVersion: 1, ID: "artifact_spike_validation", OperationID: operation.ID, Model: "fixture", Content: "{}", ContentHash: "sha256:fixture", CreatedAt: now}
		if err := tx.AppendRawModelOutput(raw); err != nil {
			return err
		}
		proposal := domain.ProposedChangeSet{SchemaVersion: 1, ID: "changeset_spike", MissionRevision: revision.ID, OperationID: operation.ID, BaseCommitID: domain.GenesisCommitID, ReadSet: []string{"fragment_spike"}, Changes: []domain.Change{{Kind: domain.ChangeAdd, EntityType: r.CanonicalType, EntityID: r.CanonicalID, PayloadRef: "payload_spike"}}, ExpectedDelta: "one canonical entity", ValidatorIDs: []string{"schema"}, Provenance: "storage-spike", IdempotencyKey: r.IdempotencyKey}
		if err := tx.AppendProposedChangeSet(proposal); err != nil {
			return err
		}
		validation := domain.ValidationReceipt{SchemaVersion: 1, ID: "receipt_spike_validation", OperationID: operation.ID, ChangeSetID: proposal.ID, ValidatorID: "schema", Passed: true, ArtifactRef: raw.ID, ProducedAt: now}
		if err := tx.AppendValidationReceipt(validation); err != nil {
			return err
		}
		accepted := domain.AcceptedChangeSet{SchemaVersion: 1, ID: "accepted_spike", ProposedChangeSetID: proposal.ID, ValidationReceiptIDs: []domain.ReceiptID{validation.ID}, AcceptedAt: now, PolicyVersion: "policy@1"}
		if err := tx.AppendAcceptedChangeSet(accepted); err != nil {
			return err
		}
		if _, err := tx.ReserveIdempotency(domain.IdempotencyRecord{SchemaVersion: 1, Key: r.IdempotencyKey, OperationID: operation.ID, Intent: "apply changeset", Status: domain.IdempotencyReserved, ReservedAt: now}); err != nil {
			return err
		}
		commit := domain.Commit{SchemaVersion: 1, ID: r.CommitID, AcceptedChangeSetID: accepted.ID, MissionRevision: revision.ID, BaseCommitID: domain.GenesisCommitID, Version: 1, CommittedAt: now, ReceiptID: r.ReceiptID, IdempotencyKey: r.IdempotencyKey}
		receipt := domain.CommitReceipt{SchemaVersion: 1, ID: r.ReceiptID, CommitID: commit.ID, ChangeSetID: accepted.ID, OperationID: operation.ID, Version: 1, ProducedAt: now}
		if err := tx.ApplyCommit(commit, receipt, proposal.Changes); err != nil {
			return err
		}
		if _, err := tx.CompleteIdempotency(r.IdempotencyKey, r.ReceiptID, string(r.CommitID), now); err != nil {
			return err
		}
		_, err := tx.AppendEvent(domain.Event{SchemaVersion: 1, ID: r.EventID, Kind: "knowledge.commit.applied", OccurredAt: now, MissionRevision: revision.ID, OperationID: operation.ID, CommitID: commit.ID, PayloadRef: string(proposal.ID)})
		return err
	})
}

// InspectOfficialMutation classifies the compound visibility invariant used by
// the final crash harness. Merely finding the audit event is insufficient: the
// commit, receipt, head, idempotency completion and canonical entity must all
// exist and agree on the same logical commit.
func InspectOfficialMutation(ctx context.Context, store port.Store, refs OfficialMutationRefs) (CrashOutcome, error) {
	if err := refs.validate(); err != nil {
		return OutcomeInvalidPartial, err
	}
	present := 0
	consistent := true
	err := store.View(ctx, func(reader port.Reader) error {
		event, found, err := lookup(func() (domain.Event, error) { return reader.EventByID(refs.EventID) })
		if err != nil {
			return err
		}
		if found {
			present++
			consistent = consistent && event.CommitID == refs.CommitID
		}

		commit, found, err := lookup(func() (domain.Commit, error) { return reader.Commit(refs.CommitID) })
		if err != nil {
			return err
		}
		if found {
			present++
			consistent = consistent && commit.ReceiptID == refs.ReceiptID && commit.MissionRevision == refs.MissionRevision && commit.IdempotencyKey == refs.IdempotencyKey
		}

		receipt, found, err := lookup(func() (domain.CommitReceipt, error) { return reader.CommitReceipt(refs.ReceiptID) })
		if err != nil {
			return err
		}
		if found {
			present++
			consistent = consistent && receipt.CommitID == refs.CommitID
		}

		head, found, err := lookup(func() (domain.Commit, error) { return reader.HeadCommit(refs.MissionRevision) })
		if err != nil {
			return err
		}
		if found {
			present++
			consistent = consistent && head.ID == refs.CommitID
		}

		idempotency, found, err := lookup(func() (domain.IdempotencyRecord, error) { return reader.IdempotencyRecord(refs.IdempotencyKey) })
		if err != nil {
			return err
		}
		if found {
			present++
			consistent = consistent && idempotency.Status == domain.IdempotencyCompleted && idempotency.ReceiptID == refs.ReceiptID && idempotency.ResultRef == string(refs.CommitID)
		}

		entity, found, err := lookup(func() (domain.CanonicalEntity, error) {
			return reader.CanonicalEntity(refs.CanonicalType, refs.CanonicalID)
		})
		if err != nil {
			return err
		}
		if found {
			present++
			consistent = consistent && entity.CommitID == refs.CommitID
		}
		return nil
	})
	if err != nil {
		return OutcomeInvalidPartial, fmt.Errorf("inspect official mutation: %w", err)
	}
	if present == 0 {
		return OutcomeNotApplied, nil
	}
	if present == 6 && consistent {
		return OutcomeApplied, nil
	}
	return OutcomeInvalidPartial, nil
}

func lookup[T any](get func() (T, error)) (T, bool, error) {
	value, err := get()
	if err == nil {
		return value, true, nil
	}
	if errors.Is(err, port.ErrNotFound) {
		var zero T
		return zero, false, nil
	}
	var zero T
	return zero, false, err
}
