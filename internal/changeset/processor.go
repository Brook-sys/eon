// Package changeset turns untrusted model text into an auditable proposal and
// applies it only through deterministic kernel validation.
package changeset

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/modeltext"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
)

const defaultMaxOutputBytes int64 = 1 << 20

type Processor struct {
	store          port.Store
	clock          source.Clock
	ids            source.IDGenerator
	policyVersion  string
	maxOutputBytes int64
	checkpoint     Checkpoint
}

// Boundary names durable processing frontiers used by crash/restart tests.
// A checkpoint error models abrupt worker loss: callers must assume that
// writes before the boundary may already be durable and recover from storage.
type Boundary string

const (
	BoundaryRawPersisted     Boundary = "RAW_PERSISTED"
	BoundaryProposalStaged   Boundary = "PROPOSAL_STAGED"
	BoundaryValidationStaged Boundary = "VALIDATION_STAGED"
	BoundaryAcceptanceStaged Boundary = "ACCEPTANCE_STAGED"
	BoundaryCommitStaged     Boundary = "COMMIT_STAGED"
	BoundaryEventStaged      Boundary = "EVENT_STAGED"
	BoundaryCommitDurable    Boundary = "COMMIT_DURABLE"
)

type Checkpoint func(Boundary) error

type Config struct {
	Store          port.Store
	Clock          source.Clock
	IDs            source.IDGenerator
	PolicyVersion  string
	MaxOutputBytes int64
	Checkpoint     Checkpoint
}

func New(config Config) (*Processor, error) {
	if config.Store == nil || config.Clock == nil || config.IDs == nil || strings.TrimSpace(config.PolicyVersion) == "" {
		return nil, errors.New("store, clock, ID generator, and policy version are required")
	}
	limit := config.MaxOutputBytes
	if limit == 0 {
		limit = defaultMaxOutputBytes
	}
	if limit < 1 {
		return nil, errors.New("max output bytes must be positive")
	}
	return &Processor{store: config.Store, clock: config.Clock, ids: config.IDs, policyVersion: config.PolicyVersion, maxOutputBytes: limit, checkpoint: config.Checkpoint}, nil
}

// DecodeStrict accepts exactly one JSON object after deterministic local
// normalization (FR-MODEL-004 ladder: trim, fence strip, object extract).
// Unknown fields, trailing data, duplicate keys, and oversized output are
// still rejected. Callers MUST preserve the original raw text separately.
func DecodeStrict(text string, maxBytes int64) (domain.ProposedChangeSet, error) {
	if maxBytes < 1 || int64(len(text)) > maxBytes {
		return domain.ProposedChangeSet{}, errors.New("model output exceeds changeset limit")
	}
	// Normalize against the raw size budget: refuse to expand authority by
	// accepting a payload whose pre-normalized form already exceeds the limit.
	normalized := modeltext.BestJSONCandidate(text)
	if int64(len(normalized)) > maxBytes {
		return domain.ProposedChangeSet{}, errors.New("model output exceeds changeset limit")
	}
	text = normalized
	if err := validateJSONShape(text); err != nil {
		return domain.ProposedChangeSet{}, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &fields); err != nil {
		return domain.ProposedChangeSet{}, fmt.Errorf("decode proposed changeset fields: %w", err)
	}
	for _, name := range []string{"read_set", "preconditions", "changes", "validator_ids"} {
		value, ok := fields[name]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return domain.ProposedChangeSet{}, fmt.Errorf("required array %q is missing or null", name)
		}
	}
	decoder := json.NewDecoder(bytes.NewBufferString(text))
	decoder.DisallowUnknownFields()
	var proposal domain.ProposedChangeSet
	if err := decoder.Decode(&proposal); err != nil {
		return domain.ProposedChangeSet{}, fmt.Errorf("decode proposed changeset: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return domain.ProposedChangeSet{}, errors.New("decode proposed changeset: trailing JSON value")
		}
		return domain.ProposedChangeSet{}, fmt.Errorf("decode proposed changeset trailing data: %w", err)
	}
	if err := proposal.Validate(); err != nil {
		return domain.ProposedChangeSet{}, fmt.Errorf("validate proposed changeset: %w", err)
	}
	return proposal, nil
}

// Process first preserves exact provider text, then parses and validates it.
// Invalid text remains inspectable but cannot create a typed proposal or
// canonical effect. All accepted-change, commit, entity, and event writes are
// one storage transaction.
func (p *Processor) Process(ctx context.Context, operationID domain.OperationID, result port.CompletionResult) (domain.Commit, error) {
	raw, err := p.rawOutput(operationID, result)
	if err != nil {
		return domain.Commit{}, err
	}
	if err := p.store.Update(ctx, func(tx port.Transaction) error { return tx.AppendRawModelOutput(raw) }); err != nil {
		return domain.Commit{}, fmt.Errorf("preserve raw model output: %w", err)
	}
	if err := p.reach(BoundaryRawPersisted); err != nil {
		return domain.Commit{}, err
	}

	// Decode from a normalized view of the preserved raw text. The artifact
	// still holds the exact provider bytes; only the typed proposal is repaired.
	proposal, err := DecodeStrict(result.Text, p.maxOutputBytes)
	if err != nil {
		return domain.Commit{}, err
	}
	if proposal.OperationID != operationID {
		return domain.Commit{}, errors.New("proposed changeset operation does not match request")
	}

	var replay domain.Commit
	err = p.store.View(ctx, func(r port.Reader) error {
		commit, lookupErr := r.CommitByIdempotencyKey(proposal.IdempotencyKey)
		if lookupErr == nil {
			existing, proposalErr := r.ProposedChangeSet(proposal.ID)
			if proposalErr != nil || !equalProposal(existing, proposal) || commit.MissionRevision != proposal.MissionRevision || commit.BaseCommitID != proposal.BaseCommitID {
				return fmt.Errorf("%w: idempotency key is already committed for another proposal", port.ErrConflict)
			}
			replay = commit
			return nil
		}
		if !errors.Is(lookupErr, port.ErrNotFound) {
			return lookupErr
		}
		return nil
	})
	if err != nil {
		return domain.Commit{}, err
	}
	if replay.ID != "" {
		return replay, nil
	}

	acceptedID, err := p.newID("accepted_changeset")
	if err != nil {
		return domain.Commit{}, err
	}
	commitID, err := p.newID("commit")
	if err != nil {
		return domain.Commit{}, err
	}
	commitReceiptID, err := p.newID("receipt")
	if err != nil {
		return domain.Commit{}, err
	}
	eventID, err := p.newID("event")
	if err != nil {
		return domain.Commit{}, err
	}
	now := p.clock.Now()
	var committed domain.Commit
	err = p.store.Update(ctx, func(tx port.Transaction) error {
		operation, err := tx.Operation(proposal.OperationID)
		if err != nil {
			return err
		}
		spec, err := tx.OperationSpec(operation.SpecID)
		if err != nil {
			return err
		}
		if err := validateLineage(operation, spec, proposal); err != nil {
			return err
		}
		if err := tx.AppendProposedChangeSet(proposal); err != nil {
			return err
		}
		if err := p.reach(BoundaryProposalStaged); err != nil {
			return err
		}

		receiptIDs := make([]domain.ReceiptID, 0, len(proposal.ValidatorIDs))
		for _, validatorID := range proposal.ValidatorIDs {
			if err := runValidator(validatorID, proposal); err != nil {
				return err
			}
			receiptID, err := p.newID("receipt")
			if err != nil {
				return err
			}
			receipt := domain.ValidationReceipt{SchemaVersion: 1, ID: domain.ReceiptID(receiptID), OperationID: proposal.OperationID, ChangeSetID: proposal.ID, ValidatorID: validatorID, Passed: true, ArtifactRef: raw.ID, ProducedAt: now}
			if err := tx.AppendValidationReceipt(receipt); err != nil {
				return err
			}
			receiptIDs = append(receiptIDs, receipt.ID)
		}
		if err := p.reach(BoundaryValidationStaged); err != nil {
			return err
		}
		accepted := domain.AcceptedChangeSet{SchemaVersion: 1, ID: domain.ChangeSetID(acceptedID), ProposedChangeSetID: proposal.ID, ValidationReceiptIDs: receiptIDs, AcceptedAt: now, PolicyVersion: p.policyVersion}
		if err := tx.AppendAcceptedChangeSet(accepted); err != nil {
			return err
		}
		if err := p.reach(BoundaryAcceptanceStaged); err != nil {
			return err
		}

		version := uint64(1)
		if head, headErr := tx.HeadCommit(proposal.MissionRevision); headErr == nil {
			version = head.Version + 1
		} else if !errors.Is(headErr, port.ErrNotFound) {
			return headErr
		}
		committed = domain.Commit{SchemaVersion: 1, ID: domain.CommitID(commitID), AcceptedChangeSetID: accepted.ID, MissionRevision: proposal.MissionRevision, BaseCommitID: proposal.BaseCommitID, Version: version, CommittedAt: now, ReceiptID: domain.ReceiptID(commitReceiptID), IdempotencyKey: proposal.IdempotencyKey}
		commitReceipt := domain.CommitReceipt{SchemaVersion: 1, ID: committed.ReceiptID, CommitID: committed.ID, ChangeSetID: accepted.ID, OperationID: proposal.OperationID, Version: committed.Version, ProducedAt: now}
		if err := tx.ApplyCommit(committed, commitReceipt, proposal.Changes); err != nil {
			return err
		}
		if err := p.reach(BoundaryCommitStaged); err != nil {
			return err
		}
		_, err = tx.AppendEvent(domain.Event{SchemaVersion: 1, ID: domain.EventID(eventID), Kind: "knowledge.commit.applied", OccurredAt: now, MissionRevision: proposal.MissionRevision, OperationID: proposal.OperationID, CommitID: committed.ID, PayloadRef: string(proposal.ID)})
		if err != nil {
			return err
		}
		return p.reach(BoundaryEventStaged)
	})
	if err != nil {
		return domain.Commit{}, fmt.Errorf("apply proposed changeset: %w", err)
	}
	if err := p.reach(BoundaryCommitDurable); err != nil {
		return domain.Commit{}, err
	}
	return committed, nil
}

func (p *Processor) reach(boundary Boundary) error {
	if p.checkpoint == nil {
		return nil
	}
	if err := p.checkpoint(boundary); err != nil {
		return fmt.Errorf("processing interrupted at %s: %w", boundary, err)
	}
	return nil
}

func (p *Processor) rawOutput(operationID domain.OperationID, result port.CompletionResult) (domain.RawModelOutput, error) {
	id, err := p.newID("artifact")
	if err != nil {
		return domain.RawModelOutput{}, err
	}
	hash := sha256.Sum256([]byte(result.Text))
	return domain.RawModelOutput{SchemaVersion: 1, ID: domain.ArtifactID(id), OperationID: operationID, Model: result.Model, Content: result.Text, ContentHash: hex.EncodeToString(hash[:]), CreatedAt: p.clock.Now()}, nil
}

func (p *Processor) newID(prefix string) (string, error) {
	id, err := p.ids.NewID(prefix)
	if err != nil {
		return "", fmt.Errorf("generate %s ID: %w", prefix, err)
	}
	return id, nil
}

func validateLineage(operation domain.Operation, spec domain.OperationSpec, proposal domain.ProposedChangeSet) error {
	if operation.MissionRevision != proposal.MissionRevision || operation.IdempotencyKey != proposal.IdempotencyKey {
		return fmt.Errorf("%w: proposal differs from operation lineage", port.ErrConflict)
	}
	if !slices.Equal(operation.ReadSet, proposal.ReadSet) {
		return fmt.Errorf("%w: proposal read set differs from operation", port.ErrConflict)
	}
	if len(proposal.Preconditions) != 0 {
		return errors.New("preconditions are not supported until a typed precondition language is defined")
	}
	want := append([]string(nil), spec.Validators...)
	got := append([]string(nil), proposal.ValidatorIDs...)
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(want, got) {
		return fmt.Errorf("%w: proposal validators differ from operation spec", port.ErrConflict)
	}
	return nil
}

func equalProposal(a, b domain.ProposedChangeSet) bool {
	return a.SchemaVersion == b.SchemaVersion && a.ID == b.ID && a.MissionRevision == b.MissionRevision &&
		a.OperationID == b.OperationID && a.BaseCommitID == b.BaseCommitID &&
		slices.Equal(a.ReadSet, b.ReadSet) && slices.Equal(a.Preconditions, b.Preconditions) &&
		slices.Equal(a.Changes, b.Changes) && a.ExpectedDelta == b.ExpectedDelta &&
		slices.Equal(a.ValidatorIDs, b.ValidatorIDs) && a.Provenance == b.Provenance &&
		a.IdempotencyKey == b.IdempotencyKey
}

func runValidator(id string, proposal domain.ProposedChangeSet) error {
	switch id {
	case "schema":
		return proposal.Validate()
	default:
		return fmt.Errorf("unknown deterministic changeset validator %q", id)
	}
}

var allowedJSONKeys = map[string]struct{}{
	"schema_version": {}, "id": {}, "mission_revision_id": {}, "operation_id": {},
	"base_commit_id": {}, "read_set": {}, "preconditions": {}, "changes": {},
	"expected_delta": {}, "validator_ids": {}, "provenance": {}, "idempotency_key": {},
	"kind": {}, "entity_type": {}, "entity_id": {}, "payload_ref": {},
}

// validateJSONShape rejects duplicate keys and non-canonical key spellings
// before encoding/json performs typed per-object validation.
func validateJSONShape(text string) error {
	decoder := json.NewDecoder(strings.NewReader(text))
	if err := validateJSONValue(decoder, true); err != nil {
		return fmt.Errorf("validate proposed changeset JSON shape: %w", err)
	}
	if token, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("validate proposed changeset JSON shape: trailing token %v", token)
		}
		return fmt.Errorf("validate proposed changeset JSON shape: %w", err)
	}
	return nil
}

func validateJSONValue(decoder *json.Decoder, requireObject bool) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, isDelim := token.(json.Delim)
	if requireObject && (!isDelim || delim != '{') {
		return errors.New("top-level value must be an object")
	}
	if !isDelim {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if _, ok := allowedJSONKeys[key]; !ok {
				return fmt.Errorf("unknown or non-canonical field %q", key)
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate field %q", key)
			}
			seen[key] = struct{}{}
			if err := validateJSONValue(decoder, false); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("unterminated object")
		}
	case '[':
		for decoder.More() {
			if err := validateJSONValue(decoder, false); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return errors.New("unterminated array")
		}
	default:
		return fmt.Errorf("unexpected delimiter %q", delim)
	}
	return nil
}
