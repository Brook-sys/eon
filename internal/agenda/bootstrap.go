// Package agenda installs deterministic units of epistemic work.
package agenda

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

const EventAgendaWorkCreated = "agenda.work_created"

type Clock interface{ Now() time.Time }
type IDGenerator interface {
	NewID(prefix string) (string, error)
}

// WorkSpec contains policy-approved inputs. Model output must be validated and
// admitted before reaching this boundary.
type WorkSpec struct {
	MissionRevision  domain.MissionRevisionID
	QuestionText     string
	QuestionOrigin   string
	Relevance        string
	AnswerCondition  string
	DerivedFrom      []string
	ExpectedProgress string
	Novelty          string
	EstimatedCost    domain.Budget
	Risk             domain.RiskLevel
	SourcePlan       []string
	StopCondition    string
	ReviewAfter      time.Time
	AdmissionReason  string
	InquiryBudget    domain.Budget
	OperationSpecID  domain.OperationSpecID
	ReadSet          []string
	InputRefs        []string
	ExpectedOutput   string
}

func (s WorkSpec) validate() error {
	if s.MissionRevision == "" || strings.TrimSpace(s.QuestionText) == "" || strings.TrimSpace(s.QuestionOrigin) == "" || strings.TrimSpace(s.Relevance) == "" || strings.TrimSpace(s.AnswerCondition) == "" {
		return errors.New("question inputs are incomplete")
	}
	if len(s.DerivedFrom) == 0 || strings.TrimSpace(s.ExpectedProgress) == "" || strings.TrimSpace(s.Novelty) == "" || len(s.SourcePlan) == 0 || strings.TrimSpace(s.StopCondition) == "" || s.ReviewAfter.IsZero() || strings.TrimSpace(s.AdmissionReason) == "" {
		return errors.New("inquiry inputs are incomplete")
	}
	if s.OperationSpecID == "" || strings.TrimSpace(s.ExpectedOutput) == "" {
		return errors.New("operation inputs are incomplete")
	}
	if err := s.EstimatedCost.Validate(); err != nil {
		return fmt.Errorf("estimated cost: %w", err)
	}
	return s.InquiryBudget.Validate()
}

type WorkSet struct {
	Question  domain.Question
	Candidate domain.InquiryCandidate
	Inquiry   domain.Inquiry
	Operation domain.Operation
}

type Bootstrapper struct {
	Store port.Store
	Clock Clock
	IDs   IDGenerator
}

// Create persists a question, its admitted inquiry, the first operation and
// all reevaluation conditions in one transaction (FR-AGENDA-001/002,
// FR-DUR-001, FR-OBS-001).
func (b Bootstrapper) Create(ctx context.Context, spec WorkSpec) (WorkSet, error) {
	if b.Store == nil || b.Clock == nil || b.IDs == nil {
		return WorkSet{}, errors.New("agenda bootstrapper dependencies are incomplete")
	}
	if err := spec.validate(); err != nil {
		return WorkSet{}, fmt.Errorf("validate work spec: %w", err)
	}
	questionID, err := b.newID("question")
	if err != nil {
		return WorkSet{}, err
	}
	candidateID, err := b.newID("inquiry_candidate")
	if err != nil {
		return WorkSet{}, err
	}
	inquiryID, err := b.newID("inquiry")
	if err != nil {
		return WorkSet{}, err
	}
	operationID, err := b.newID("operation")
	if err != nil {
		return WorkSet{}, err
	}
	idempotencyKey, err := b.newID("idempotency")
	if err != nil {
		return WorkSet{}, err
	}
	eventID, err := b.newID("event")
	if err != nil {
		return WorkSet{}, err
	}

	work := WorkSet{
		Question:  domain.Question{SchemaVersion: 1, ID: domain.QuestionID(questionID), MissionRevision: spec.MissionRevision, Text: spec.QuestionText, Origin: spec.QuestionOrigin, Relevance: spec.Relevance, AnswerCondition: spec.AnswerCondition},
		Candidate: domain.InquiryCandidate{SchemaVersion: 1, ID: domain.InquiryCandidateID(candidateID), MissionRevision: spec.MissionRevision, QuestionID: domain.QuestionID(questionID), DerivedFrom: append([]string(nil), spec.DerivedFrom...), ExpectedProgress: spec.ExpectedProgress, Novelty: spec.Novelty, EstimatedCost: spec.EstimatedCost, Risk: spec.Risk, SourcePlan: append([]string(nil), spec.SourcePlan...), AnswerCondition: spec.AnswerCondition, StopCondition: spec.StopCondition, ReviewAfter: spec.ReviewAfter},
		Inquiry:   domain.Inquiry{SchemaVersion: 1, ID: domain.InquiryID(inquiryID), CandidateID: domain.InquiryCandidateID(candidateID), MissionRevision: spec.MissionRevision, QuestionID: domain.QuestionID(questionID), AdmissionReason: spec.AdmissionReason, Budget: spec.InquiryBudget, StopCondition: spec.StopCondition, State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady}},
		Operation: domain.Operation{SchemaVersion: 1, ID: domain.OperationID(operationID), InquiryID: domain.InquiryID(inquiryID), MissionRevision: spec.MissionRevision, SpecID: spec.OperationSpecID, ReadSet: append([]string(nil), spec.ReadSet...), InputRefs: append([]string(nil), spec.InputRefs...), ExpectedOutput: spec.ExpectedOutput, IdempotencyKey: domain.IdempotencyKey(idempotencyKey), State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady}},
	}
	if err := validateWork(work); err != nil {
		return WorkSet{}, err
	}
	now := b.Clock.Now().UTC()
	err = b.Store.Update(ctx, func(tx port.Transaction) error {
		if _, err := tx.MissionRevision(spec.MissionRevision); err != nil {
			return err
		}
		if _, err := tx.OperationSpec(spec.OperationSpecID); err != nil {
			return err
		}
		if err := tx.CreateQuestion(work.Question); err != nil {
			return err
		}
		if err := tx.CreateInquiryCandidate(work.Candidate); err != nil {
			return err
		}
		if err := tx.CreateInquiry(work.Inquiry); err != nil {
			return err
		}
		if err := tx.CreateOperation(work.Operation); err != nil {
			return err
		}
		_, err := tx.AppendEvent(domain.Event{SchemaVersion: 1, ID: domain.EventID(eventID), Kind: EventAgendaWorkCreated, OccurredAt: now, MissionRevision: spec.MissionRevision, InquiryID: work.Inquiry.ID, OperationID: work.Operation.ID, PayloadRef: string(work.Question.ID)})
		return err
	})
	if err != nil {
		return WorkSet{}, fmt.Errorf("persist agenda work: %w", err)
	}
	return work, nil
}

func (b Bootstrapper) newID(prefix string) (string, error) {
	id, err := b.IDs.NewID(prefix)
	if err != nil {
		return "", fmt.Errorf("generate %s ID: %w", prefix, err)
	}
	return id, nil
}

func validateWork(work WorkSet) error {
	if err := work.Question.Validate(); err != nil {
		return fmt.Errorf("build question: %w", err)
	}
	if err := work.Candidate.Validate(); err != nil {
		return fmt.Errorf("build candidate: %w", err)
	}
	if err := work.Inquiry.Validate(); err != nil {
		return fmt.Errorf("build inquiry: %w", err)
	}
	if err := work.Operation.Validate(); err != nil {
		return fmt.Errorf("build operation: %w", err)
	}
	return nil
}
