package domain

import (
	"errors"
	"fmt"
	"time"
)

const SchemaVersionV1 = 1

// Budget is a persistable upper bound. Zero means no units are authorized,
// never "unlimited". Durations are serialized by encoding/json as nanoseconds;
// external schemas may provide a friendlier representation at their boundary.
type Budget struct {
	ModelCalls int           `json:"model_calls"`
	Tokens     int           `json:"tokens"`
	Bytes      int64         `json:"bytes"`
	Attempts   int           `json:"attempts"`
	Duration   time.Duration `json:"duration"`
}

func (b Budget) Validate() error {
	if b.ModelCalls < 0 || b.Tokens < 0 || b.Bytes < 0 || b.Attempts < 0 || b.Duration < 0 {
		return errors.New("budget values must not be negative")
	}
	return nil
}

type MissionStatus string

const (
	MissionActive    MissionStatus = "ACTIVE"
	MissionPaused    MissionStatus = "PAUSED"
	MissionCancelled MissionStatus = "CANCELLED"
)

// MissionRevision is an immutable persisted revision by contract. Repositories
// must append revisions rather than overwrite an accepted value (FR-AUTH-001).
type MissionRevision struct {
	SchemaVersion int               `json:"schema_version"`
	ID            MissionRevisionID `json:"id"`
	MissionID     MissionID         `json:"mission_id"`
	Revision      uint64            `json:"revision"`
	OriginalText  string            `json:"original_text"`
	Purpose       string            `json:"purpose"`
	Domains       []string          `json:"domains"`
	Policies      []string          `json:"policies"`
	Budget        Budget            `json:"budget"`
	Status        MissionStatus     `json:"status"`
	Provenance    string            `json:"provenance"`
	AcceptedAt    time.Time         `json:"accepted_at"`
}

func (m MissionRevision) Validate() error {
	if m.SchemaVersion != SchemaVersionV1 {
		return fmt.Errorf("unsupported mission schema version %d", m.SchemaVersion)
	}
	if m.ID == "" || m.MissionID == "" || m.Revision == 0 || m.OriginalText == "" || m.Purpose == "" || m.Provenance == "" || m.AcceptedAt.IsZero() {
		return errors.New("mission revision is missing required identity, text, purpose, provenance, or acceptance time")
	}
	switch m.Status {
	case MissionActive, MissionPaused, MissionCancelled:
	default:
		return fmt.Errorf("unknown mission status %q", m.Status)
	}
	return m.Budget.Validate()
}

type Question struct {
	SchemaVersion   int               `json:"schema_version"`
	ID              QuestionID        `json:"id"`
	MissionRevision MissionRevisionID `json:"mission_revision_id"`
	Text            string            `json:"text"`
	Origin          string            `json:"origin"`
	Relevance       string            `json:"relevance"`
	AnswerCondition string            `json:"answer_condition"`
}

func (q Question) Validate() error {
	if q.SchemaVersion != SchemaVersionV1 || q.ID == "" || q.MissionRevision == "" || q.Text == "" || q.Origin == "" || q.Relevance == "" || q.AnswerCondition == "" {
		return errors.New("question is incomplete or has unsupported schema version")
	}
	return nil
}

type RiskLevel string

const (
	RiskLow    RiskLevel = "LOW"
	RiskMedium RiskLevel = "MEDIUM"
	RiskHigh   RiskLevel = "HIGH"
)

type InquiryCandidate struct {
	SchemaVersion    int                `json:"schema_version"`
	ID               InquiryCandidateID `json:"id"`
	MissionRevision  MissionRevisionID  `json:"mission_revision_id"`
	QuestionID       QuestionID         `json:"question_id"`
	DerivedFrom      []string           `json:"derived_from"`
	ExpectedProgress string             `json:"expected_progress"`
	Novelty          string             `json:"novelty"`
	EstimatedCost    Budget             `json:"estimated_cost"`
	Risk             RiskLevel          `json:"risk"`
	SourcePlan       []string           `json:"source_plan"`
	AnswerCondition  string             `json:"answer_condition"`
	StopCondition    string             `json:"stop_condition"`
	ReviewAfter      time.Time          `json:"review_after"`
}

func (c InquiryCandidate) Validate() error {
	if c.SchemaVersion != SchemaVersionV1 || c.ID == "" || c.MissionRevision == "" || c.QuestionID == "" || len(c.DerivedFrom) == 0 || c.ExpectedProgress == "" || c.Novelty == "" || len(c.SourcePlan) == 0 || c.AnswerCondition == "" || c.StopCondition == "" || c.ReviewAfter.IsZero() {
		return errors.New("inquiry candidate is incomplete or has unsupported schema version")
	}
	switch c.Risk {
	case RiskLow, RiskMedium, RiskHigh:
	default:
		return fmt.Errorf("unknown risk level %q", c.Risk)
	}
	return c.EstimatedCost.Validate()
}

type Inquiry struct {
	SchemaVersion   int                   `json:"schema_version"`
	ID              InquiryID             `json:"id"`
	CandidateID     InquiryCandidateID    `json:"candidate_id"`
	MissionRevision MissionRevisionID     `json:"mission_revision_id"`
	QuestionID      QuestionID            `json:"question_id"`
	AdmissionReason string                `json:"admission_reason"`
	Budget          Budget                `json:"budget"`
	StopCondition   string                `json:"stop_condition"`
	State           OperationalState      `json:"state"`
	Reevaluation    ReevaluationCondition `json:"reevaluation"`
}

func (i Inquiry) Validate() error {
	if i.SchemaVersion != SchemaVersionV1 || i.ID == "" || i.CandidateID == "" || i.MissionRevision == "" || i.QuestionID == "" || i.AdmissionReason == "" || i.StopCondition == "" {
		return errors.New("inquiry is incomplete or has unsupported schema version")
	}
	if err := i.Budget.Validate(); err != nil {
		return err
	}
	return i.Reevaluation.ValidateFor(i.State)
}

type Authority string

const (
	AuthorityProposeOnly Authority = "PROPOSE_ONLY"
	AuthorityReadOnly    Authority = "READ_ONLY"
	AuthorityKernelWrite Authority = "KERNEL_WRITE"
)

// OperationSpec is the versioned contract instantiated by an Operation.
type OperationSpec struct {
	SchemaVersion    int             `json:"schema_version"`
	ID               OperationSpecID `json:"id"`
	ContractVersion  uint64          `json:"contract_version"`
	InputSchema      string          `json:"input_schema"`
	OutputSchema     string          `json:"output_schema"`
	Budget           Budget          `json:"budget"`
	Validators       []string        `json:"validators"`
	RetryPolicy      string          `json:"retry_policy"`
	FallbackPolicy   string          `json:"fallback_policy"`
	MaximumAuthority Authority       `json:"maximum_authority"`
}

func (s OperationSpec) Validate() error {
	if s.SchemaVersion != SchemaVersionV1 || s.ID == "" || s.ContractVersion == 0 || s.InputSchema == "" || s.OutputSchema == "" || len(s.Validators) == 0 || s.RetryPolicy == "" || s.FallbackPolicy == "" {
		return errors.New("operation spec is incomplete or has unsupported schema version")
	}
	switch s.MaximumAuthority {
	case AuthorityProposeOnly, AuthorityReadOnly, AuthorityKernelWrite:
	default:
		return fmt.Errorf("unknown authority %q", s.MaximumAuthority)
	}
	return s.Budget.Validate()
}

type Operation struct {
	SchemaVersion   int                   `json:"schema_version"`
	ID              OperationID           `json:"id"`
	InquiryID       InquiryID             `json:"inquiry_id"`
	MissionRevision MissionRevisionID     `json:"mission_revision_id"`
	SpecID          OperationSpecID       `json:"spec_id"`
	ReadSet         []string              `json:"read_set"`
	InputRefs       []string              `json:"input_refs"`
	ExpectedOutput  string                `json:"expected_output"`
	Attempt         uint32                `json:"attempt"`
	IdempotencyKey  IdempotencyKey        `json:"idempotency_key"`
	State           OperationalState      `json:"state"`
	Reevaluation    ReevaluationCondition `json:"reevaluation"`
}

func (o Operation) Validate() error {
	if o.SchemaVersion != SchemaVersionV1 || o.ID == "" || o.InquiryID == "" || o.MissionRevision == "" || o.SpecID == "" || o.ExpectedOutput == "" || o.IdempotencyKey == "" {
		return errors.New("operation is incomplete or has unsupported schema version")
	}
	return o.Reevaluation.ValidateFor(o.State)
}
