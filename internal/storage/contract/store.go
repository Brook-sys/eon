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
	t.Run("observations require a recoverable anchor and exact fragment quote", func(t *testing.T) {
		store := factory()
		now := time.Date(2026, 7, 15, 16, 40, 0, 0, time.UTC)
		content := []byte("bounded source text")
		digest := sha256.Sum256(content)
		hash := "sha256:" + hex.EncodeToString(digest[:])
		source := domain.Source{SchemaVersion: 1, ID: "source_1", Kind: "fixture", Locator: "source.txt", ObservedAt: now}
		version := domain.SourceVersion{SchemaVersion: 1, ID: "source_version_1", SourceID: source.ID, ContentHash: hash, ContentRef: hash, ObservedAt: now}
		snapshot := domain.SourceSnapshot{SchemaVersion: 1, SourceVersionID: version.ID, MediaType: "text/plain", Content: content}
		fragment := domain.SourceFragment{SchemaVersion: 1, ID: "fragment_1", SourceVersionID: version.ID, Location: "bytes:0-19", StartOffset: 0, EndOffset: 19, ContentHash: hash, ContentRef: hash}
		observation := domain.Observation{SchemaVersion: 1, ID: "observation_1", Statement: "the source contains bounded text", ExactQuote: string(content), Anchor: domain.ObservationAnchor{SourceFragmentID: fragment.ID}, Provenance: "extractor:test@1"}
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			if err := tx.AppendSource(source, version, snapshot); err != nil {
				return err
			}
			if err := tx.AppendSourceFragments(version.ID, []domain.SourceFragment{fragment}); err != nil {
				return err
			}
			return tx.AppendObservation(observation)
		}); err != nil {
			t.Fatalf("append observation: %v", err)
		}
		if err := store.View(context.Background(), func(r port.Reader) error {
			got, err := r.Observation(observation.ID)
			if err != nil {
				return err
			}
			if got != observation {
				t.Fatalf("observation = %+v, want %+v", got, observation)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}

		bad := observation
		bad.ID, bad.ExactQuote = "observation_2", "invented quote"
		err := store.Update(context.Background(), func(tx port.Transaction) error { return tx.AppendObservation(bad) })
		if !errors.Is(err, port.ErrConflict) {
			t.Fatalf("quote mismatch error = %v, want ErrConflict", err)
		}
		missing := observation
		missing.ID, missing.Anchor.SourceFragmentID = "observation_3", "fragment_missing"
		err = store.Update(context.Background(), func(tx port.Transaction) error { return tx.AppendObservation(missing) })
		if !errors.Is(err, port.ErrNotFound) {
			t.Fatalf("missing anchor error = %v, want ErrNotFound", err)
		}
	})
	t.Run("claims require qualifiers and evidence links resolve both endpoints", func(t *testing.T) {
		store := factory()
		now := time.Date(2026, 7, 15, 17, 0, 0, 0, time.UTC)
		content := []byte("bounded claim evidence")
		digest := sha256.Sum256(content)
		hash := "sha256:" + hex.EncodeToString(digest[:])
		source := domain.Source{SchemaVersion: 1, ID: "source_1", Kind: "fixture", Locator: "source.txt", ObservedAt: now}
		version := domain.SourceVersion{SchemaVersion: 1, ID: "source_version_1", SourceID: source.ID, ContentHash: hash, ContentRef: hash, ObservedAt: now}
		snapshot := domain.SourceSnapshot{SchemaVersion: 1, SourceVersionID: version.ID, MediaType: "text/plain", Content: content}
		fragment := domain.SourceFragment{SchemaVersion: 1, ID: "fragment_1", SourceVersionID: version.ID, Location: "bytes:0-22", StartOffset: 0, EndOffset: 22, ContentHash: hash, ContentRef: hash}
		observation := domain.Observation{SchemaVersion: 1, ID: "observation_1", Statement: "the source provides claim evidence", ExactQuote: string(content), Anchor: domain.ObservationAnchor{SourceFragmentID: fragment.ID}, Provenance: "extractor:test@1"}
		claim := domain.Claim{SchemaVersion: 1, ID: "claim_1", Proposition: "The fixture provides bounded evidence.", Qualifiers: map[string]string{"scope": "fixture"}, Version: 1}
		link := domain.EvidenceLink{SchemaVersion: 1, ID: "evidence_1", ObservationID: observation.ID, ClaimID: claim.ID, Relation: domain.EvidenceSupports, Rationale: "exact source anchor"}
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			if err := tx.AppendSource(source, version, snapshot); err != nil {
				return err
			}
			if err := tx.AppendSourceFragments(version.ID, []domain.SourceFragment{fragment}); err != nil {
				return err
			}
			if err := tx.AppendObservation(observation); err != nil {
				return err
			}
			return tx.AppendClaimWithEvidence(claim, []domain.EvidenceLink{link})
		}); err != nil {
			t.Fatalf("append claim and evidence: %v", err)
		}
		claim.Qualifiers["scope"] = "caller mutation"
		if err := store.View(context.Background(), func(r port.Reader) error {
			got, err := r.Claim("claim_1")
			if err != nil {
				return err
			}
			if got.Qualifiers["scope"] != "fixture" {
				t.Fatalf("stored claim aliased caller: %+v", got.Qualifiers)
			}
			links, err := r.EvidenceLinksForClaim(got.ID)
			if err != nil {
				return err
			}
			if len(links) != 1 || links[0] != link {
				t.Fatalf("evidence links = %+v, want %+v", links, link)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}

		orphan := domain.Claim{SchemaVersion: 1, ID: "claim_2", Proposition: "orphan", Qualifiers: map[string]string{"scope": "test"}, Version: 1}
		orphanLink := domain.EvidenceLink{SchemaVersion: 1, ID: "evidence_2", ObservationID: "observation_missing", ClaimID: orphan.ID, Relation: domain.EvidenceSupports}
		err := store.Update(context.Background(), func(tx port.Transaction) error {
			return tx.AppendClaimWithEvidence(orphan, []domain.EvidenceLink{orphanLink})
		})
		if !errors.Is(err, port.ErrNotFound) {
			t.Fatalf("orphan evidence error = %v, want ErrNotFound", err)
		}
		if err := store.View(context.Background(), func(r port.Reader) error {
			_, err := r.Claim(orphan.ID)
			if !errors.Is(err, port.ErrNotFound) {
				t.Fatalf("orphan claim survived rollback: %v", err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("evidence deltas and knowledge artifacts are append-only and isolated", func(t *testing.T) {
		store := factory()
		now := time.Date(2026, 7, 15, 17, 20, 0, 0, time.UTC)
		content := []byte("artifact evidence")
		digest := sha256.Sum256(content)
		hash := "sha256:" + hex.EncodeToString(digest[:])
		source := domain.Source{SchemaVersion: 1, ID: "source_1", Kind: "fixture", Locator: "source.txt", ObservedAt: now}
		version := domain.SourceVersion{SchemaVersion: 1, ID: "source_version_1", SourceID: source.ID, ContentHash: hash, ContentRef: hash, ObservedAt: now}
		snapshot := domain.SourceSnapshot{SchemaVersion: 1, SourceVersionID: version.ID, MediaType: "text/plain", Content: content}
		fragment := domain.SourceFragment{SchemaVersion: 1, ID: "fragment_1", SourceVersionID: version.ID, Location: "bytes:0-17", StartOffset: 0, EndOffset: 17, ContentHash: hash, ContentRef: hash}
		observation := domain.Observation{SchemaVersion: 1, ID: "observation_1", Statement: "artifact evidence exists", ExactQuote: string(content), Anchor: domain.ObservationAnchor{SourceFragmentID: fragment.ID}, Provenance: "extractor:test@1"}
		claim := domain.Claim{SchemaVersion: 1, ID: "claim_1", Proposition: "An artifact can cite evidence.", Qualifiers: map[string]string{"scope": "test"}, Version: 1}
		initial := domain.EvidenceLink{SchemaVersion: 1, ID: "evidence_1", ObservationID: observation.ID, ClaimID: claim.ID, Relation: domain.EvidenceSupports}
		delta := domain.EvidenceLink{SchemaVersion: 1, ID: "evidence_2", ObservationID: observation.ID, ClaimID: claim.ID, Relation: domain.EvidenceReplicates}
		artifact := domain.KnowledgeArtifact{SchemaVersion: 1, ID: "artifact_1", Kind: "cited_claim_view", BaseCommitID: domain.GenesisCommitID, Dependencies: []string{"claim:claim_1@1", "evidence_link:evidence_1"}, ContentRef: hash, Content: "# cited view", Stale: false}
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			if err := tx.AppendSource(source, version, snapshot); err != nil {
				return err
			}
			if err := tx.AppendSourceFragments(version.ID, []domain.SourceFragment{fragment}); err != nil {
				return err
			}
			if err := tx.AppendObservation(observation); err != nil {
				return err
			}
			if err := tx.AppendClaimWithEvidence(claim, []domain.EvidenceLink{initial}); err != nil {
				return err
			}
			if err := tx.AppendEvidenceLinks(claim.ID, []domain.EvidenceLink{delta}); err != nil {
				return err
			}
			return tx.AppendKnowledgeArtifact(artifact)
		}); err != nil {
			t.Fatal(err)
		}
		artifact.Dependencies[0] = "caller mutation"
		if err := store.View(context.Background(), func(r port.Reader) error {
			links, err := r.EvidenceLinksForClaim(claim.ID)
			if err != nil {
				return err
			}
			if len(links) != 2 || links[0].ID != initial.ID || links[1].ID != delta.ID {
				t.Fatalf("evidence links = %+v", links)
			}
			got, err := r.KnowledgeArtifact("artifact_1")
			if err != nil {
				return err
			}
			if got.Dependencies[0] == "caller mutation" {
				t.Fatal("stored artifact aliased caller dependencies")
			}
			got.Dependencies[0] = "reader mutation"
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		artifact.Dependencies = []string{"claim:claim_1@1", "evidence_link:evidence_1"}
		artifact.Stale = true
		if err := store.Update(context.Background(), func(tx port.Transaction) error { return tx.SaveKnowledgeArtifact(artifact) }); err != nil {
			t.Fatalf("mark artifact stale: %v", err)
		}
		artifact.Content = "mutated"
		err := store.Update(context.Background(), func(tx port.Transaction) error { return tx.SaveKnowledgeArtifact(artifact) })
		if !errors.Is(err, port.ErrConflict) {
			t.Fatalf("artifact mutation error = %v, want ErrConflict", err)
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

	t.Run("operator questions use optimistic revisions and deduplicate transport answers", func(t *testing.T) {
		store := factory()
		mission := missionRevision()
		question := operatorQuestionRecord()
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			if err := tx.AppendMissionRevision(mission); err != nil {
				return err
			}
			return tx.CreateOperatorQuestion(question)
		}); err != nil {
			t.Fatalf("create operator question: %v", err)
		}
		answer := operatorAnswerRecord(question)
		answered, err := domain.TransitionOperatorQuestion(question, domain.OperatorQuestionTransition{Event: domain.QuestionEventAnswer, OccurredAt: answer.ReceivedAt, AnswerID: answer.ID})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			return tx.AcceptUserAnswer(answer, answered, question.Revision)
		}); err != nil {
			t.Fatalf("accept operator answer: %v", err)
		}
		if err := store.View(context.Background(), func(r port.Reader) error {
			got, err := r.OperatorQuestion(question.ID)
			if err != nil {
				return err
			}
			if got.Status != domain.OperatorQuestionAnswered || got.AnswerID != answer.ID {
				t.Fatalf("question = %#v", got)
			}
			byTransport, err := r.UserAnswerByTransport(answer.Channel, answer.TransportEventID)
			if err != nil {
				return err
			}
			if byTransport.ID != answer.ID {
				t.Fatalf("answer = %#v", byTransport)
			}
			listed, err := r.OperatorQuestions(question.MissionID, domain.OperatorQuestionAnswered)
			if err != nil || len(listed) != 1 || listed[0].ID != question.ID {
				t.Fatalf("listed = %#v, err = %v", listed, err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}

		duplicate := answer
		duplicate.ID = "answer_2"
		duplicateQuestion := answered
		duplicateQuestion.AnswerID = duplicate.ID
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			return tx.AcceptUserAnswer(duplicate, duplicateQuestion, question.Revision)
		}); !errors.Is(err, port.ErrConflict) {
			t.Fatalf("duplicate transport error = %v", err)
		}
		if err := store.Update(context.Background(), func(tx port.Transaction) error { return tx.SaveOperatorQuestion(answered, question.Revision) }); !errors.Is(err, port.ErrConflict) {
			t.Fatalf("stale revision error = %v", err)
		}
	})

	t.Run("operator question answer and state update roll back together", func(t *testing.T) {
		store := factory()
		mission := missionRevision()
		question := operatorQuestionRecord()
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			if err := tx.AppendMissionRevision(mission); err != nil {
				return err
			}
			return tx.CreateOperatorQuestion(question)
		}); err != nil {
			t.Fatal(err)
		}
		answer := operatorAnswerRecord(question)
		sentinel := errors.New("abort operator answer")
		err := store.Update(context.Background(), func(tx port.Transaction) error {
			answered, err := domain.TransitionOperatorQuestion(question, domain.OperatorQuestionTransition{Event: domain.QuestionEventAnswer, OccurredAt: answer.ReceivedAt, AnswerID: answer.ID})
			if err != nil {
				return err
			}
			if err := tx.AcceptUserAnswer(answer, answered, question.Revision); err != nil {
				return err
			}
			return sentinel
		})
		if !errors.Is(err, sentinel) {
			t.Fatalf("rollback error = %v", err)
		}
		if err := store.View(context.Background(), func(r port.Reader) error {
			if _, err := r.UserAnswer(answer.ID); !errors.Is(err, port.ErrNotFound) {
				t.Fatalf("rolled-back answer exists: %v", err)
			}
			got, err := r.OperatorQuestion(question.ID)
			if err != nil {
				return err
			}
			if got.Status != domain.OperatorQuestionPending || got.Revision != 1 {
				t.Fatalf("question changed during rollback: %#v", got)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("question delivery outbox leases and completes optimistically", func(t *testing.T) {
		store := factory()
		mission := missionRevision()
		question := operatorQuestionRecord()
		delivery := questionDeliveryRecord(question)
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			if err := tx.AppendMissionRevision(mission); err != nil {
				return err
			}
			if err := tx.CreateOperatorQuestion(question); err != nil {
				return err
			}
			return tx.CreateQuestionDelivery(delivery)
		}); err != nil {
			t.Fatal(err)
		}
		dueAt := delivery.AvailableAt
		leased, err := domain.LeaseQuestionDelivery(delivery, "worker_1", dueAt, dueAt.Add(time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			return tx.SaveQuestionDelivery(leased, delivery.Status, delivery.Attempt)
		}); err != nil {
			t.Fatal(err)
		}
		if err := store.View(context.Background(), func(r port.Reader) error {
			due, err := r.DueQuestionDeliveries(dueAt, 10)
			if err != nil {
				return err
			}
			if len(due) != 0 {
				t.Fatalf("leased delivery remains due: %#v", due)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		delivered, err := domain.CompleteQuestionDelivery(leased, "worker_1", "message_1", dueAt.Add(time.Second))
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			return tx.SaveQuestionDelivery(delivered, leased.Status, leased.Attempt)
		}); err != nil {
			t.Fatal(err)
		}
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			return tx.SaveQuestionDelivery(delivered, leased.Status, leased.Attempt)
		}); !errors.Is(err, port.ErrConflict) {
			t.Fatalf("stale delivery error = %v", err)
		}
		duplicate := delivery
		duplicate.ID = "delivery_2"
		if err := store.Update(context.Background(), func(tx port.Transaction) error { return tx.CreateQuestionDelivery(duplicate) }); !errors.Is(err, port.ErrConflict) {
			t.Fatalf("duplicate route error = %v", err)
		}
	})

	t.Run("question delivery outbox exposes expired leases for recovery", func(t *testing.T) {
		store := factory()
		mission := missionRevision()
		question := operatorQuestionRecord()
		delivery := questionDeliveryRecord(question)
		leaseUntil := delivery.AvailableAt.Add(time.Minute)
		leased, err := domain.LeaseQuestionDelivery(delivery, "crashed_worker", delivery.AvailableAt, leaseUntil)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			if err := tx.AppendMissionRevision(mission); err != nil {
				return err
			}
			if err := tx.CreateOperatorQuestion(question); err != nil {
				return err
			}
			if err := tx.CreateQuestionDelivery(delivery); err != nil {
				return err
			}
			return tx.SaveQuestionDelivery(leased, delivery.Status, delivery.Attempt)
		}); err != nil {
			t.Fatal(err)
		}
		if err := store.View(context.Background(), func(r port.Reader) error {
			due, err := r.DueQuestionDeliveries(leaseUntil, 10)
			if err != nil {
				return err
			}
			if len(due) != 1 || due[0].ID != delivery.ID {
				t.Fatalf("expired lease due = %#v", due)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		reclaimed, err := domain.ReclaimExpiredQuestionDelivery(leased, leaseUntil)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			return tx.SaveQuestionDelivery(reclaimed, leased.Status, leased.Attempt)
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("question gate decisions are persisted and retrievable", func(t *testing.T) {
		store := factory()
		now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
		decision := domain.QuestionGateDecisionRecord{
			SchemaVersion:  domain.SchemaVersionV1,
			ID:             "gate_1",
			QuestionID:     "ask_1",
			MissionID:      "mission_1",
			DedupSignature: "presentation:artifact_1",
			Decision:       domain.PersistedQuestionAdmit,
			Reason:         domain.PersistedQuestionGateAllowed,
			PolicyVersion:  "default@1",
			EvaluatedAt:    now,
		}
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			return tx.CreateQuestionGateDecision(decision)
		}); err != nil {
			t.Fatalf("create gate decision: %v", err)
		}
		if err := store.View(context.Background(), func(r port.Reader) error {
			got, err := r.QuestionGateDecision(decision.ID)
			if err != nil {
				return err
			}
			if got.ID != decision.ID || got.Decision != domain.PersistedQuestionAdmit {
				t.Fatalf("decision = %#v", got)
			}
			byQ, err := r.QuestionGateDecisionByQuestion(decision.QuestionID)
			if err != nil || byQ.ID != decision.ID {
				t.Fatalf("by question = %#v, err = %v", byQ, err)
			}
			listed, err := r.QuestionGateDecisions(decision.MissionID)
			if err != nil || len(listed) != 1 || listed[0].ID != decision.ID {
				t.Fatalf("listed = %#v, err = %v", listed, err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			return tx.CreateQuestionGateDecision(decision)
		}); !errors.Is(err, port.ErrConflict) {
			t.Fatalf("duplicate gate ID error = %v", err)
		}
		duplicateQ := decision
		duplicateQ.ID = "gate_2"
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			return tx.CreateQuestionGateDecision(duplicateQ)
		}); !errors.Is(err, port.ErrConflict) {
			t.Fatalf("duplicate question error = %v", err)
		}
	})

	t.Run("operator commands and control state are durable with optimistic concurrency", func(t *testing.T) {
		store := factory()
		now := time.Date(2026, 7, 16, 4, 0, 0, 0, time.UTC)
		mission := missionRevision()
		revision := uint64(1)
		command := domain.OperatorCommand{
			SchemaVersion: domain.SchemaVersionV1, ID: "cmd_pause", IdempotencyKey: "idem_pause",
			ActorType: domain.ActorOperator, ActorID: "operator_1", Kind: domain.CommandPauseMission,
			Target: domain.CommandTarget{MissionID: mission.MissionID}, ExpectedRevision: &revision,
			Reason: "hold", SubmittedAt: now,
		}
		receipt := domain.CommandReceipt{
			SchemaVersion: domain.SchemaVersionV1, ID: "receipt_cmd_pause", CommandID: command.ID,
			State: domain.CommandReceived, RecordedAt: now,
		}
		control := domain.DefaultControlState(now)
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			if err := tx.AppendMissionRevision(mission); err != nil {
				return err
			}
			if err := tx.ActivateMissionRevision(mission.MissionID, mission.ID); err != nil {
				return err
			}
			if err := tx.CreateOperatorCommand(command, receipt); err != nil {
				return err
			}
			return tx.SaveControlState(control, 0)
		}); err != nil {
			t.Fatalf("seed control: %v", err)
		}
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			return tx.CreateOperatorCommand(command, receipt)
		}); err != nil {
			t.Fatalf("identical command replay: %v", err)
		}
		divergent := command
		divergent.ID = "cmd_pause_other"
		divergent.Reason = "other"
		divergentReceipt := receipt
		divergentReceipt.ID, divergentReceipt.CommandID = "receipt_other", divergent.ID
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			return tx.CreateOperatorCommand(divergent, divergentReceipt)
		}); !errors.Is(err, port.ErrConflict) {
			t.Fatalf("divergent idempotency error = %v", err)
		}
		nextControl, resultRef, err := domain.ApplyOperatorCommand(control, command, mission, now.Add(time.Minute))
		if err != nil || resultRef != "mission_1@1:PAUSED" {
			t.Fatalf("apply = %#v ref=%s err=%v", nextControl, resultRef, err)
		}
		validating := receipt
		validating.State, validating.RecordedAt = domain.CommandValidating, now.Add(time.Second)
		accepted := validating
		accepted.State, accepted.RecordedAt = domain.CommandAccepted, now.Add(2*time.Second)
		applying := accepted
		applying.State, applying.RecordedAt = domain.CommandApplying, now.Add(3*time.Second)
		applied := applying
		applied.State, applied.ResultRef, applied.RecordedAt = domain.CommandApplied, resultRef, now.Add(4*time.Second)
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			for _, step := range []domain.CommandReceipt{validating, accepted, applying, applied} {
				if err := tx.SaveOperatorCommandReceipt(step); err != nil {
					return err
				}
			}
			return tx.SaveControlState(nextControl, control.Revision)
		}); err != nil {
			t.Fatalf("apply control: %v", err)
		}
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			return tx.SaveControlState(nextControl, control.Revision)
		}); !errors.Is(err, port.ErrConflict) {
			t.Fatalf("stale control revision error = %v", err)
		}
		if err := store.View(context.Background(), func(r port.Reader) error {
			state, err := r.ControlState()
			if err != nil {
				return err
			}
			if state.AllowsDispatch(mission.MissionID) || state.Revision != 1 {
				t.Fatalf("control state = %#v", state)
			}
			pending, err := r.PendingOperatorCommands(10)
			if err != nil {
				return err
			}
			if len(pending) != 0 {
				t.Fatalf("pending after apply = %#v", pending)
			}
			gotReceipt, err := r.OperatorCommandReceipt(command.ID)
			if err != nil || gotReceipt.State != domain.CommandApplied {
				t.Fatalf("receipt = %#v err=%v", gotReceipt, err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("work opportunities and continuity diagnoses are durable with dedup and lineage", func(t *testing.T) {
		store := factory()
		now := time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC)
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			return tx.AppendMissionRevision(missionRevision())
		}); err != nil {
			t.Fatalf("seed mission: %v", err)
		}
		root := domain.WorkOpportunity{
			SchemaVersion: domain.SchemaVersionV1, ID: "opp_root", MissionRevision: "revision_1",
			Family: domain.FamilyGapScan, Status: domain.OpportunityOpen, Title: "cover gaps", Origin: "mission",
			ExpectedGain: "new inquiries", Novelty: "uncovered scopes", StopCondition: "coverage target",
			DedupSignature: "gap:root", Depth: 0, EstimatedCost: domain.Budget{Tokens: 10}, Risk: domain.RiskLow,
			Priority: 10, CreatedAt: now, UpdatedAt: now,
		}
		child, err := root.DeriveChild("opp_child", "define term", "decompose:root", "definition", "undefined", "definition accepted", "gap:term", domain.RiskLow, 8, now.Add(time.Minute), domain.Budget{Tokens: 3})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			if err := tx.CreateWorkOpportunity(root); err != nil {
				return err
			}
			return tx.CreateWorkOpportunity(child)
		}); err != nil {
			t.Fatalf("create opportunities: %v", err)
		}
		dup := root
		dup.ID = "opp_dup"
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			return tx.CreateWorkOpportunity(dup)
		}); !errors.Is(err, port.ErrConflict) {
			t.Fatalf("dedup error = %v", err)
		}
		admitted := root
		admitted.Status = domain.OpportunityAdmitted
		admitted.AdmittedInquiryID = "inquiry_1"
		admitted.UpdatedAt = now.Add(2 * time.Minute)
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			return tx.SaveWorkOpportunity(admitted)
		}); err != nil {
			t.Fatalf("admit: %v", err)
		}
		diagnosis := domain.ContinuityDiagnosis{
			SchemaVersion: domain.SchemaVersionV1, ID: "diag_1", MissionRevision: "revision_1", OccurredAt: now.Add(3 * time.Minute),
			StrategiesTried: []string{"gap_scan"}, OpenCandidateCount: 1, ReadyCount: 0,
			RecoveryConditions: []string{"new source"}, SafeDetail: "blocked", PolicyVersion: "horizon.v1",
		}
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			return tx.CreateContinuityDiagnosis(diagnosis)
		}); err != nil {
			t.Fatalf("diagnosis: %v", err)
		}
		if err := store.View(context.Background(), func(r port.Reader) error {
			open, err := r.WorkOpportunities("revision_1", domain.OpportunityOpen)
			if err != nil || len(open) != 1 || open[0].ID != "opp_child" {
				t.Fatalf("open = %#v err=%v", open, err)
			}
			got, err := r.LatestContinuityDiagnosis("revision_1")
			if err != nil || got.ID != diagnosis.ID {
				t.Fatalf("latest diagnosis = %+v err=%v", got, err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("external events are durable with disposition and dedup", func(t *testing.T) {
		store := factory()
		now := time.Date(2026, 7, 16, 5, 0, 0, 0, time.UTC)
		event := domain.ExternalEvent{
			SchemaVersion: domain.SchemaVersionV1, ID: "ext_msg_1", DeduplicationKey: "telegram:update:99",
			Source: "telegram", SourceActorID: "operator_1", Kind: domain.ExternalUserMessage,
			MissionID: "mission_1", CorrelationID: "thread_1",
			Content: domain.ExternalContent{MediaType: "text/plain", Text: "hello"}, ReceivedAt: now,
		}
		disposition := domain.ExternalEventDisposition{
			SchemaVersion: domain.SchemaVersionV1, EventID: event.ID, State: domain.ExternalEventReceived, RecordedAt: now,
		}
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			return tx.CreateExternalEvent(event, disposition)
		}); err != nil {
			t.Fatalf("create external event: %v", err)
		}
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			return tx.CreateExternalEvent(event, disposition)
		}); err != nil {
			t.Fatalf("identical external event replay: %v", err)
		}
		divergent := event
		divergent.ID = "ext_msg_2"
		divergent.Content.Text = "other"
		divergentDisposition := disposition
		divergentDisposition.EventID = divergent.ID
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			return tx.CreateExternalEvent(divergent, divergentDisposition)
		}); !errors.Is(err, port.ErrConflict) {
			t.Fatalf("divergent dedup error = %v", err)
		}
		if err := store.View(context.Background(), func(r port.Reader) error {
			pending, err := r.PendingExternalEvents(10)
			if err != nil || len(pending) != 1 || pending[0].ID != event.ID {
				t.Fatalf("pending = %#v err=%v", pending, err)
			}
			byKey, err := r.ExternalEventByDeduplicationKey(event.DeduplicationKey)
			if err != nil || byKey.ID != event.ID {
				t.Fatalf("by key = %#v err=%v", byKey, err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		applied := disposition
		applied.State, applied.ResultRef, applied.RecordedAt = domain.ExternalEventApplied, "wake:ext_msg_1:user.message:resumed=0", now.Add(time.Second)
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			return tx.SaveExternalEventDisposition(applied)
		}); err != nil {
			t.Fatalf("apply disposition: %v", err)
		}
		reverse := disposition
		reverse.RecordedAt = applied.RecordedAt.Add(time.Second)
		if err := store.Update(context.Background(), func(tx port.Transaction) error {
			return tx.SaveExternalEventDisposition(reverse)
		}); !errors.Is(err, port.ErrConflict) {
			t.Fatalf("terminal reverse disposition error = %v", err)
		}
		if err := store.View(context.Background(), func(r port.Reader) error {
			pending, err := r.PendingExternalEvents(10)
			if err != nil || len(pending) != 0 {
				t.Fatalf("pending after apply = %#v err=%v", pending, err)
			}
			got, err := r.ExternalEventDisposition(event.ID)
			if err != nil || got.State != domain.ExternalEventApplied {
				t.Fatalf("disposition = %#v err=%v", got, err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
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

func operatorQuestionRecord() domain.OperatorQuestion {
	created := time.Date(2026, 7, 15, 12, 1, 0, 0, time.UTC)
	return domain.OperatorQuestion{
		SchemaVersion: domain.SchemaVersionV1, ID: "ask_1", MissionID: "mission_1", MissionRevision: "revision_1", Revision: 1,
		Kind: domain.QuestionSingleChoice, Prompt: "Choose a presentation", Context: "Only the artifact presentation depends on it",
		Options: []domain.QuestionOption{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}, AllowContext: true, AllowSkip: true,
		BlockingScope:  []domain.QuestionBlockingTarget{{Kind: domain.QuestionBlockingArtifact, Reference: "artifact_1"}},
		FallbackPolicy: domain.QuestionContinueOtherWork, DedupSignature: "presentation:artifact_1", Priority: 50,
		Status: domain.OperatorQuestionPending, CreatedAt: created, ExpiresAt: created.Add(time.Hour),
	}
}

func operatorAnswerRecord(question domain.OperatorQuestion) domain.UserAnswer {
	return domain.UserAnswer{
		SchemaVersion: domain.SchemaVersionV1, ID: "answer_1", QuestionID: question.ID, ExpectedQuestionRevision: question.Revision,
		Kind: domain.AnswerOptions, OptionIDs: []string{"a"}, ActorID: "operator_1", Channel: "dashboard",
		TransportEventID: "request_1", ReceivedAt: question.CreatedAt.Add(time.Minute),
	}
}

func questionDeliveryRecord(question domain.OperatorQuestion) domain.QuestionDelivery {
	return domain.QuestionDelivery{
		SchemaVersion: domain.SchemaVersionV1, ID: "delivery_1", QuestionID: question.ID, QuestionRevision: question.Revision,
		Channel: "telegram", DestinationRef: "operator_primary", Status: domain.QuestionDeliveryPending, MaxAttempts: 3,
		AvailableAt: question.CreatedAt, CreatedAt: question.CreatedAt, UpdatedAt: question.CreatedAt,
	}
}
