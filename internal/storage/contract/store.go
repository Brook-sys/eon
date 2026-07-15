// Package contract contains reusable tests for storage backends.
package contract

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

type Factory func() port.Store

func TestStore(t *testing.T, factory Factory) {
	t.Helper()
	t.Run("source ingestion is immutable, content addressed and atomic", func(t *testing.T) {
		store := factory()
		now := time.Date(2026, 7, 15, 16, 0, 0, 0, time.UTC)
		content := []byte("fixture evidence")
		digest := sha256.Sum256(content)
		hash := "sha256:" + hex.EncodeToString(digest[:])
		source := domain.Source{SchemaVersion: 1, ID: "source_1", Kind: "fixture", Locator: "testdata/source.txt", ObservedAt: now}
		version := domain.SourceVersion{SchemaVersion: 1, ID: "source_version_1", SourceID: source.ID, ContentHash: hash, ContentRef: hash, ObservedAt: now}
		snapshot := domain.SourceSnapshot{SchemaVersion: 1, SourceVersionID: version.ID, MediaType: "text/plain", Content: content}
		if err := store.Update(context.Background(), func(tx port.Transaction) error { return tx.AppendSource(source, version, snapshot) }); err != nil {
			t.Fatalf("append source: %v", err)
		}
		content[0] = 'X'
		snapshot.Content[1] = 'X'
		if err := store.View(context.Background(), func(r port.Reader) error {
			got, err := r.SourceSnapshot(version.ID)
			if err != nil {
				return err
			}
			if string(got.Content) != "fixture evidence" {
				t.Fatalf("stored snapshot aliased caller: %q", got.Content)
			}
			got.Content[0] = 'X'
			again, err := r.SourceSnapshot(version.ID)
			if err == nil && string(again.Content) != "fixture evidence" {
				t.Fatalf("read snapshot aliased store: %q", again.Content)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}

		badSource := source
		badSource.ID = "source_2"
		badVersion := version
		badVersion.ID, badVersion.SourceID, badVersion.ContentHash, badVersion.ContentRef = "source_version_2", badSource.ID, "sha256:wrong", "sha256:wrong"
		badSnapshot := domain.SourceSnapshot{SchemaVersion: 1, SourceVersionID: badVersion.ID, MediaType: "text/plain", Content: []byte("other")}
		err := store.Update(context.Background(), func(tx port.Transaction) error { return tx.AppendSource(badSource, badVersion, badSnapshot) })
		if !errors.Is(err, port.ErrConflict) {
			t.Fatalf("hash mismatch error = %v, want ErrConflict", err)
		}
		if err := store.View(context.Background(), func(r port.Reader) error {
			_, err := r.Source(badSource.ID)
			if !errors.Is(err, port.ErrNotFound) {
				t.Fatalf("invalid source survived rollback: %v", err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("source fragments require exact ordered coverage and round trip", func(t *testing.T) {
		store := factory()
		now := time.Date(2026, 7, 15, 16, 20, 0, 0, time.UTC)
		content := []byte("abcdef")
		digest := sha256.Sum256(content)
		hash := "sha256:" + hex.EncodeToString(digest[:])
		source := domain.Source{SchemaVersion: 1, ID: "source_1", Kind: "fixture", Locator: "source.txt", ObservedAt: now}
		version := domain.SourceVersion{SchemaVersion: 1, ID: "source_version_1", SourceID: source.ID, ContentHash: hash, ContentRef: hash, ObservedAt: now}
		snapshot := domain.SourceSnapshot{SchemaVersion: 1, SourceVersionID: version.ID, MediaType: "text/plain", Content: content}
		fragment := func(id domain.SourceFragmentID, start, end uint64) domain.SourceFragment {
			digest := sha256.Sum256(content[start:end])
			hash := "sha256:" + hex.EncodeToString(digest[:])
			return domain.SourceFragment{SchemaVersion: 1, ID: id, SourceVersionID: version.ID, Location: fmt.Sprintf("bytes:%d-%d", start, end), StartOffset: start, EndOffset: end, ContentHash: hash, ContentRef: hash}
		}
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			if err := tx.AppendSource(source, version, snapshot); err != nil {
				return err
			}
			return tx.AppendSourceFragments(version.ID, []domain.SourceFragment{fragment("fragment_1", 0, 3), fragment("fragment_2", 3, 6)})
		}); err != nil {
			t.Fatalf("append fragments: %v", err)
		}
		if err := store.View(context.Background(), func(r port.Reader) error {
			fragments, err := r.SourceFragments(version.ID)
			if err != nil {
				return err
			}
			var roundTrip []byte
			for _, got := range fragments {
				roundTrip = append(roundTrip, content[got.StartOffset:got.EndOffset]...)
			}
			if string(roundTrip) != string(content) {
				t.Fatalf("fragment round trip = %q", roundTrip)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}

		other := domain.Source{SchemaVersion: 1, ID: "source_2", Kind: "fixture", Locator: "other.txt", ObservedAt: now}
		otherVersion := domain.SourceVersion{SchemaVersion: 1, ID: "source_version_2", SourceID: other.ID, ContentHash: hash, ContentRef: hash, ObservedAt: now}
		otherSnapshot := domain.SourceSnapshot{SchemaVersion: 1, SourceVersionID: otherVersion.ID, MediaType: "text/plain", Content: content}
		gap := fragment("fragment_gap", 1, 6)
		gap.SourceVersionID = otherVersion.ID
		err := store.Update(context.Background(), func(tx port.Transaction) error {
			if err := tx.AppendSource(other, otherVersion, otherSnapshot); err != nil {
				return err
			}
			return tx.AppendSourceFragments(otherVersion.ID, []domain.SourceFragment{gap})
		})
		if !errors.Is(err, port.ErrConflict) {
			t.Fatalf("gap error = %v, want ErrConflict", err)
		}
		if err := store.View(context.Background(), func(r port.Reader) error {
			_, err := r.Source(other.ID)
			if !errors.Is(err, port.ErrNotFound) {
				t.Fatalf("failed fragment transaction partially committed: %v", err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("rest round trips and requires an existing mission revision", func(t *testing.T) {
		store := factory()
		now := time.Date(2026, 7, 15, 15, 40, 0, 0, time.UTC)
		notBefore := now.Add(time.Hour)
		wantNotBefore := notBefore
		rest := domain.Rest{SchemaVersion: 1, MissionRevision: "revision_1", Reason: "no executable work", EnteredAt: now, Active: true, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateNotBefore, NotBefore: &notBefore}}
		err := store.Update(context.Background(), func(tx port.Transaction) error { return tx.SaveRest(rest) })
		if !errors.Is(err, port.ErrNotFound) {
			t.Fatalf("rest without mission error = %v, want ErrNotFound", err)
		}
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			if err := tx.AppendMissionRevision(missionRevision()); err != nil {
				return err
			}
			return tx.SaveRest(rest)
		}); err != nil {
			t.Fatalf("save rest: %v", err)
		}
		*rest.Reevaluation.NotBefore = now.Add(24 * time.Hour)
		if err := store.View(context.Background(), func(r port.Reader) error {
			got, err := r.Rest("revision_1")
			if err == nil && !got.Reevaluation.NotBefore.Equal(wantNotBefore) {
				t.Fatalf("stored rest aliased caller: %v", got.Reevaluation.NotBefore)
			}
			return err
		}); err != nil {
			t.Fatalf("read rest: %v", err)
		}
	})
	t.Run("mission revisions are immutable and activation is explicit", func(t *testing.T) {
		store := factory()
		revision := missionRevision()
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			if err := tx.AppendMissionRevision(revision); err != nil {
				return err
			}
			return tx.ActivateMissionRevision(revision.MissionID, revision.ID)
		}); err != nil {
			t.Fatalf("seed mission: %v", err)
		}

		revision.Domains[0] = "mutated by caller"
		var got domain.MissionRevision
		if err := store.View(context.Background(), func(r port.Reader) error {
			var err error
			got, err = r.ActiveMissionRevision("mission_1")
			return err
		}); err != nil {
			t.Fatalf("read active mission: %v", err)
		}
		if got.Domains[0] != "science" {
			t.Fatalf("stored slice aliased caller: %q", got.Domains[0])
		}
		got.Domains[0] = "mutated after read"
		if err := store.View(context.Background(), func(r port.Reader) error {
			again, err := r.MissionRevision("revision_1")
			if err == nil && again.Domains[0] != "science" {
				t.Fatalf("read result aliased store: %q", again.Domains[0])
			}
			return err
		}); err != nil {
			t.Fatalf("reread mission: %v", err)
		}

		err := store.Update(context.Background(), func(tx port.Transaction) error { return tx.AppendMissionRevision(missionRevision()) })
		if !errors.Is(err, port.ErrConflict) {
			t.Fatalf("duplicate append error = %v, want ErrConflict", err)
		}
	})

	t.Run("agenda records round trip and mutable records require prior create", func(t *testing.T) {
		store := factory()
		q, candidate, inquiry, operation := agendaRecords()
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			if err := tx.AppendMissionRevision(missionRevision()); err != nil {
				return err
			}
			if err := tx.AppendOperationSpec(operationSpec()); err != nil {
				return err
			}
			if err := tx.CreateQuestion(q); err != nil {
				return err
			}
			if err := tx.CreateInquiryCandidate(candidate); err != nil {
				return err
			}
			if err := tx.CreateInquiry(inquiry); err != nil {
				return err
			}
			return tx.CreateOperation(operation)
		}); err != nil {
			t.Fatalf("create agenda: %v", err)
		}

		operation.ReadSet[0] = "caller mutation"
		if err := store.View(context.Background(), func(r port.Reader) error {
			spec, err := r.OperationSpec("extract@1")
			if err != nil {
				return err
			}
			spec.Validators[0] = "caller mutation"
			again, err := r.OperationSpec("extract@1")
			if err != nil {
				return err
			}
			if again.Validators[0] != "schema" {
				t.Fatalf("operation spec slice aliased store: %q", again.Validators[0])
			}
			got, err := r.Operation("operation_1")
			if err == nil && got.ReadSet[0] != "fragment_1" {
				t.Fatalf("operation slice aliased caller: %q", got.ReadSet[0])
			}
			return err
		}); err != nil {
			t.Fatalf("read operation: %v", err)
		}

		missing := operation
		missing.ID = "operation_missing"
		err := store.Update(context.Background(), func(tx port.Transaction) error { return tx.SaveOperation(missing) })
		if !errors.Is(err, port.ErrNotFound) {
			t.Fatalf("save missing error = %v, want ErrNotFound", err)
		}

		changed := operation
		changed.SpecID = "other@1"
		err = store.Update(context.Background(), func(tx port.Transaction) error { return tx.SaveOperation(changed) })
		if !errors.Is(err, port.ErrConflict) {
			t.Fatalf("mutate immutable operation error = %v, want ErrConflict", err)
		}
		changedInquiry := inquiry
		changedInquiry.QuestionID = "other_question"
		err = store.Update(context.Background(), func(tx port.Transaction) error { return tx.SaveInquiry(changedInquiry) })
		if !errors.Is(err, port.ErrConflict) {
			t.Fatalf("mutate immutable inquiry error = %v, want ErrConflict", err)
		}
	})

	t.Run("agenda lineage and operation spec references fail closed", func(t *testing.T) {
		store := factory()
		q, candidate, inquiry, operation := agendaRecords()
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			if err := tx.AppendMissionRevision(missionRevision()); err != nil {
				return err
			}
			return tx.CreateQuestion(q)
		}); err != nil {
			t.Fatal(err)
		}
		orphan := candidate
		orphan.QuestionID = "question_missing"
		err := store.Update(context.Background(), func(tx port.Transaction) error { return tx.CreateInquiryCandidate(orphan) })
		if !errors.Is(err, port.ErrNotFound) {
			t.Fatalf("orphan candidate error = %v, want ErrNotFound", err)
		}
		err = store.Update(context.Background(), func(tx port.Transaction) error {
			if err := tx.CreateInquiryCandidate(candidate); err != nil {
				return err
			}
			if err := tx.CreateInquiry(inquiry); err != nil {
				return err
			}
			return tx.CreateOperation(operation)
		})
		if !errors.Is(err, port.ErrNotFound) {
			t.Fatalf("missing spec error = %v, want ErrNotFound", err)
		}
		if err := store.View(context.Background(), func(r port.Reader) error {
			_, err := r.Inquiry(inquiry.ID)
			if !errors.Is(err, port.ErrNotFound) {
				t.Fatalf("failed lineage transaction partially committed: %v", err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("failed transaction rolls back all writes", func(t *testing.T) {
		store := factory()
		sentinel := errors.New("inject failure")
		err := store.Update(context.Background(), func(tx port.Transaction) error {
			if err := tx.AppendMissionRevision(missionRevision()); err != nil {
				return err
			}
			if err := tx.CreateQuestion(agendaQuestion()); err != nil {
				return err
			}
			return sentinel
		})
		if !errors.Is(err, sentinel) {
			t.Fatalf("update error = %v, want sentinel", err)
		}
		if err := store.View(context.Background(), func(r port.Reader) error {
			_, missionErr := r.MissionRevision("revision_1")
			_, questionErr := r.Question("question_1")
			if !errors.Is(missionErr, port.ErrNotFound) || !errors.Is(questionErr, port.ErrNotFound) {
				t.Fatalf("rollback left data: mission=%v question=%v", missionErr, questionErr)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("event log is ordered append-only and transactional", func(t *testing.T) {
		store := factory()
		first := event("event_1", "operation.ready")
		second := event("event_2", "operation.started")
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			appended, err := tx.AppendEvent(first)
			if err != nil {
				return err
			}
			if appended.Sequence != 1 {
				t.Fatalf("first sequence = %d, want 1", appended.Sequence)
			}
			_, err = tx.AppendEvent(second)
			return err
		}); err != nil {
			t.Fatalf("append events: %v", err)
		}
		if err := store.View(context.Background(), func(r port.Reader) error {
			events, err := r.Events(0, 1)
			if err != nil {
				return err
			}
			if len(events) != 1 || events[0].ID != first.ID || events[0].Sequence != 1 {
				t.Fatalf("first page = %#v", events)
			}
			events, err = r.Events(1, 10)
			if err != nil {
				return err
			}
			if len(events) != 1 || events[0].ID != second.ID || events[0].Sequence != 2 {
				t.Fatalf("second page = %#v", events)
			}
			byID, err := r.EventByID(second.ID)
			if err == nil && byID.Sequence != 2 {
				t.Fatalf("event by id sequence = %d", byID.Sequence)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		err := store.Update(context.Background(), func(tx port.Transaction) error { _, err := tx.AppendEvent(first); return err })
		if !errors.Is(err, port.ErrConflict) {
			t.Fatalf("duplicate event error = %v, want ErrConflict", err)
		}
		sentinel := errors.New("rollback event")
		err = store.Update(context.Background(), func(tx port.Transaction) error {
			if _, err := tx.AppendEvent(event("event_3", "operation.failed")); err != nil {
				return err
			}
			return sentinel
		})
		if !errors.Is(err, sentinel) {
			t.Fatalf("rollback error = %v", err)
		}
		if err := store.View(context.Background(), func(r port.Reader) error {
			_, err := r.EventByID("event_3")
			if !errors.Is(err, port.ErrNotFound) {
				t.Fatalf("rolled-back event error = %v", err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("idempotency reservation and completion are replay safe", func(t *testing.T) {
		store := factory()
		reservedAt := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
		record := domain.IdempotencyRecord{SchemaVersion: 1, Key: "idem_1", OperationID: "operation_1", Intent: "fetch source_1", Status: domain.IdempotencyReserved, ReservedAt: reservedAt}
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			first, err := tx.ReserveIdempotency(record)
			if err != nil {
				return err
			}
			replay, err := tx.ReserveIdempotency(record)
			if err == nil && replay != first {
				t.Fatalf("reservation replay changed record: %#v != %#v", replay, first)
			}
			return err
		}); err != nil {
			t.Fatalf("reserve idempotency: %v", err)
		}
		conflicting := record
		conflicting.OperationID = "operation_2"
		err := store.Update(context.Background(), func(tx port.Transaction) error { _, err := tx.ReserveIdempotency(conflicting); return err })
		if !errors.Is(err, port.ErrConflict) {
			t.Fatalf("conflicting reservation error = %v, want ErrConflict", err)
		}
		completedAt := reservedAt.Add(time.Minute)
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			first, err := tx.CompleteIdempotency(record.Key, "receipt_1", "artifact_1", completedAt)
			if err != nil {
				return err
			}
			replay, err := tx.CompleteIdempotency(record.Key, "receipt_1", "artifact_1", completedAt)
			if err == nil && replay != first {
				t.Fatalf("completion replay changed record: %#v != %#v", replay, first)
			}
			return err
		}); err != nil {
			t.Fatalf("complete idempotency: %v", err)
		}
		err = store.Update(context.Background(), func(tx port.Transaction) error {
			_, err := tx.CompleteIdempotency(record.Key, "receipt_2", "artifact_2", completedAt)
			return err
		})
		if !errors.Is(err, port.ErrConflict) {
			t.Fatalf("conflicting completion error = %v, want ErrConflict", err)
		}
		if err := store.View(context.Background(), func(r port.Reader) error {
			got, err := r.IdempotencyRecord(record.Key)
			if err == nil && (got.Status != domain.IdempotencyCompleted || got.ReceiptID != "receipt_1") {
				t.Fatalf("completed record = %#v", got)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("knowledge commit is atomic, versioned, and replay safe", func(t *testing.T) {
		store := factory()
		q, candidate, inquiry, operation := agendaRecords()
		now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
		proposal := domain.ProposedChangeSet{SchemaVersion: 1, ID: "changeset_1", MissionRevision: "revision_1", OperationID: operation.ID, BaseCommitID: domain.GenesisCommitID, ReadSet: []string{"fragment_1"}, Preconditions: []string{}, Changes: []domain.Change{{Kind: domain.ChangeAdd, EntityType: "observation", EntityID: "observation_1", PayloadRef: "payload_1"}}, ExpectedDelta: "one observation", ValidatorIDs: []string{"schema"}, Provenance: "model:fake", IdempotencyKey: operation.IdempotencyKey}
		raw := domain.RawModelOutput{SchemaVersion: 1, ID: "artifact_raw_1", OperationID: operation.ID, Model: "fake", Content: "{}", ContentHash: "hash", CreatedAt: now}
		validation := domain.ValidationReceipt{SchemaVersion: 1, ID: "validation_1", OperationID: operation.ID, ChangeSetID: proposal.ID, ValidatorID: "schema", Passed: true, ArtifactRef: raw.ID, ProducedAt: now}
		accepted := domain.AcceptedChangeSet{SchemaVersion: 1, ID: "accepted_1", ProposedChangeSetID: proposal.ID, ValidationReceiptIDs: []domain.ReceiptID{validation.ID}, AcceptedAt: now, PolicyVersion: "policy@1"}
		commit := domain.Commit{SchemaVersion: 1, ID: "commit_1", AcceptedChangeSetID: accepted.ID, MissionRevision: proposal.MissionRevision, BaseCommitID: proposal.BaseCommitID, Version: 1, CommittedAt: now, ReceiptID: "commit_receipt_1", IdempotencyKey: proposal.IdempotencyKey}
		commitReceipt := domain.CommitReceipt{SchemaVersion: 1, ID: commit.ReceiptID, CommitID: commit.ID, ChangeSetID: accepted.ID, OperationID: operation.ID, Version: commit.Version, ProducedAt: now}
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			if err := tx.AppendMissionRevision(missionRevision()); err != nil {
				return err
			}
			if err := tx.AppendOperationSpec(operationSpec()); err != nil {
				return err
			}
			if err := tx.CreateQuestion(q); err != nil {
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
			if err := tx.AppendProposedChangeSet(proposal); err != nil {
				return err
			}
			if err := tx.AppendValidationReceipt(validation); err != nil {
				return err
			}
			if err := tx.AppendAcceptedChangeSet(accepted); err != nil {
				return err
			}
			return tx.ApplyCommit(commit, commitReceipt, proposal.Changes)
		}); err != nil {
			t.Fatalf("apply commit: %v", err)
		}
		if err := store.Update(context.Background(), func(tx port.Transaction) error { return tx.ApplyCommit(commit, commitReceipt, proposal.Changes) }); err != nil {
			t.Fatalf("replay identical commit: %v", err)
		}
		if err := store.View(context.Background(), func(r port.Reader) error {
			entity, err := r.CanonicalEntity("observation", "observation_1")
			if err != nil {
				return err
			}
			if entity.CommitID != commit.ID || entity.Version != 1 {
				t.Fatalf("entity = %#v", entity)
			}
			receipt, err := r.CommitReceipt(commit.ReceiptID)
			if err == nil && receipt.CommitID != commit.ID {
				t.Fatalf("receipt = %#v", receipt)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}

		staleProposal := proposal
		staleProposal.ID = "changeset_2"
		staleProposal.Changes = []domain.Change{{Kind: domain.ChangeAdd, EntityType: "observation", EntityID: "observation_2", PayloadRef: "payload_2"}}
		staleValidation := validation
		staleValidation.ID, staleValidation.ChangeSetID = "validation_2", staleProposal.ID
		staleAccepted := accepted
		staleAccepted.ID, staleAccepted.ProposedChangeSetID, staleAccepted.ValidationReceiptIDs = "accepted_2", staleProposal.ID, []domain.ReceiptID{staleValidation.ID}
		staleCommit := commit
		staleCommit.ID, staleCommit.AcceptedChangeSetID, staleCommit.ReceiptID, staleCommit.IdempotencyKey = "commit_2", staleAccepted.ID, "commit_receipt_2", staleProposal.IdempotencyKey
		staleCommit.Version = 2
		staleReceipt := domain.CommitReceipt{SchemaVersion: 1, ID: staleCommit.ReceiptID, CommitID: staleCommit.ID, ChangeSetID: staleAccepted.ID, OperationID: operation.ID, Version: 2, ProducedAt: now}
		sentinel := errors.New("abort after staged stale records")
		err := store.Update(context.Background(), func(tx port.Transaction) error {
			if err := tx.AppendProposedChangeSet(staleProposal); err != nil {
				return err
			}
			if err := tx.AppendValidationReceipt(staleValidation); err != nil {
				return err
			}
			if err := tx.AppendAcceptedChangeSet(staleAccepted); err != nil {
				return err
			}
			if err := tx.ApplyCommit(staleCommit, staleReceipt, staleProposal.Changes); !errors.Is(err, port.ErrConflict) {
				return fmt.Errorf("stale commit error = %v", err)
			}
			return sentinel
		})
		if !errors.Is(err, sentinel) {
			t.Fatalf("rollback error = %v", err)
		}
		if err := store.View(context.Background(), func(r port.Reader) error {
			_, err := r.ProposedChangeSet(staleProposal.ID)
			if !errors.Is(err, port.ErrNotFound) {
				t.Fatalf("staged proposal survived rollback: %v", err)
			}
			_, err = r.CanonicalEntity("observation", "observation_2")
			if !errors.Is(err, port.ErrNotFound) {
				t.Fatalf("stale entity exists: %v", err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("invalid data and cancelled contexts do not commit", func(t *testing.T) {
		store := factory()
		err := store.Update(context.Background(), func(tx port.Transaction) error { return tx.CreateQuestion(domain.Question{}) })
		if err == nil {
			t.Fatal("invalid question was accepted")
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err = store.Update(ctx, func(tx port.Transaction) error { t.Fatal("callback ran for cancelled context"); return nil })
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled update error = %v", err)
		}
	})
}

func event(id domain.EventID, kind string) domain.Event {
	return domain.Event{SchemaVersion: 1, ID: id, Kind: kind, OccurredAt: time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC), OperationID: "operation_1"}
}

func missionRevision() domain.MissionRevision {
	return domain.MissionRevision{SchemaVersion: 1, ID: "revision_1", MissionID: "mission_1", Revision: 1, OriginalText: "investigate", Purpose: "build knowledge", Domains: []string{"science"}, Policies: []string{"cite sources"}, Status: domain.MissionActive, Provenance: "user", AcceptedAt: time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)}
}
func agendaQuestion() domain.Question {
	return domain.Question{SchemaVersion: 1, ID: "question_1", MissionRevision: "revision_1", Text: "What is true?", Origin: "mission", Relevance: "primary", AnswerCondition: "two sources"}
}
func agendaRecords() (domain.Question, domain.InquiryCandidate, domain.Inquiry, domain.Operation) {
	q := agendaQuestion()
	candidate := domain.InquiryCandidate{SchemaVersion: 1, ID: "candidate_1", MissionRevision: "revision_1", QuestionID: q.ID, DerivedFrom: []string{"gap_1"}, ExpectedProgress: "reduce uncertainty", Novelty: "not duplicate", Risk: domain.RiskLow, SourcePlan: []string{"primary sources"}, AnswerCondition: "two sources", StopCondition: "budget", ReviewAfter: time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)}
	inquiry := domain.Inquiry{SchemaVersion: 1, ID: "inquiry_1", CandidateID: candidate.ID, MissionRevision: "revision_1", QuestionID: q.ID, AdmissionReason: "priority", StopCondition: "answered", State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady}}
	operation := domain.Operation{SchemaVersion: 1, ID: "operation_1", InquiryID: inquiry.ID, MissionRevision: "revision_1", SpecID: "extract@1", ReadSet: []string{"fragment_1"}, InputRefs: []string{"artifact_1"}, ExpectedOutput: "proposed_change_set", IdempotencyKey: "idem_1", State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady}}
	return q, candidate, inquiry, operation
}

func operationSpec() domain.OperationSpec {
	return domain.OperationSpec{SchemaVersion: 1, ID: "extract@1", ContractVersion: 1, TemplateVersion: 1, InputSchema: "fragment refs", OutputSchema: "proposed change set", Budget: domain.Budget{ModelCalls: 1, Tokens: 1000, Attempts: 1}, MaxOutputTokens: 100, SafetyMargin: 50, Validators: []string{"schema"}, RetryPolicy: "no retry", FallbackPolicy: "fail", MaximumAuthority: domain.AuthorityProposeOnly}
}
