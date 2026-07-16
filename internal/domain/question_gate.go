package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// QuestionJustification is supplied with a proposed question so deterministic
// policy can decide whether interrupting the operator is warranted.
type QuestionJustification struct {
	MissingInformation string   `json:"missing_information"`
	DecisionImpact     string   `json:"decision_impact"`
	AlternativesTried  []string `json:"alternatives_tried"`
	ExpectedGain       string   `json:"expected_gain"`
	CostOfSilence      string   `json:"cost_of_silence"`
	HasSafeDefault     bool     `json:"has_safe_default"`
	DefaultReversible  bool     `json:"default_reversible"`
}

func (j QuestionJustification) Validate() error {
	if strings.TrimSpace(j.MissingInformation) == "" || strings.TrimSpace(j.DecisionImpact) == "" ||
		strings.TrimSpace(j.ExpectedGain) == "" || strings.TrimSpace(j.CostOfSilence) == "" {
		return errors.New("question justification is incomplete")
	}
	if len(j.MissingInformation) > 4096 || len(j.DecisionImpact) > 4096 || len(j.ExpectedGain) > 4096 || len(j.CostOfSilence) > 4096 {
		return errors.New("question justification exceeds byte limit")
	}
	if len(j.AlternativesTried) > 16 {
		return errors.New("question justification has too many alternatives")
	}
	if err := validateDistinctNonEmpty(j.AlternativesTried, "alternatives tried"); err != nil {
		return err
	}
	if j.DefaultReversible && !j.HasSafeDefault {
		return errors.New("reversible default requires a safe default")
	}
	return nil
}

type OperatorQuestionProposal struct {
	SchemaVersion int                   `json:"schema_version"`
	Question      OperatorQuestion      `json:"question"`
	Justification QuestionJustification `json:"justification"`
	ProposedBy    string                `json:"proposed_by"`
	ProposedAt    time.Time             `json:"proposed_at"`
}

func (p OperatorQuestionProposal) Validate() error {
	if p.SchemaVersion != SchemaVersionV1 || strings.TrimSpace(p.ProposedBy) == "" || p.ProposedAt.IsZero() {
		return errors.New("operator question proposal is incomplete or has unsupported schema version")
	}
	if err := p.Question.Validate(); err != nil {
		return fmt.Errorf("validate proposed question: %w", err)
	}
	if p.Question.Status != OperatorQuestionPending {
		return errors.New("proposed question must be pending")
	}
	if p.ProposedAt.After(p.Question.CreatedAt) {
		return errors.New("proposal time must not follow persisted question creation time")
	}
	return p.Justification.Validate()
}
