// Package contract contains reusable tests for storage backends.
package contract

import (
	"context"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

type Factory func() port.Store

func TestStore(t *testing.T, factory Factory) {
	t.Helper()
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
