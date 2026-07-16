package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	MaxOperatorQuestionTextBytes    = 8 << 10
	MaxOperatorQuestionContextBytes = 16 << 10
	MaxOperatorQuestionOptions      = 12
	MaxOperatorAnswerTextBytes      = 64 << 10
	MaxBlockingScopeItems           = 32
)

type OperatorQuestionKind string

const (
	QuestionSingleChoice          OperatorQuestionKind = "SINGLE_CHOICE"
	QuestionMultipleChoice        OperatorQuestionKind = "MULTIPLE_CHOICE"
	QuestionSingleChoiceWithOther OperatorQuestionKind = "SINGLE_CHOICE_WITH_OTHER"
	QuestionFreeText              OperatorQuestionKind = "FREE_TEXT"
	QuestionConfirmation          OperatorQuestionKind = "CONFIRMATION"
	QuestionClarification         OperatorQuestionKind = "CLARIFICATION"
)

func (k OperatorQuestionKind) valid() bool {
	switch k {
	case QuestionSingleChoice, QuestionMultipleChoice, QuestionSingleChoiceWithOther,
		QuestionFreeText, QuestionConfirmation, QuestionClarification:
		return true
	default:
		return false
	}
}

type OperatorQuestionStatus string

const (
	OperatorQuestionPending                OperatorQuestionStatus = "PENDING"
	OperatorQuestionClarificationRequested OperatorQuestionStatus = "CLARIFICATION_REQUESTED"
	OperatorQuestionAnswered               OperatorQuestionStatus = "ANSWERED"
	OperatorQuestionExpired                OperatorQuestionStatus = "EXPIRED"
	OperatorQuestionSuperseded             OperatorQuestionStatus = "SUPERSEDED"
	OperatorQuestionCancelled              OperatorQuestionStatus = "CANCELLED"
)

func (s OperatorQuestionStatus) valid() bool {
	switch s {
	case OperatorQuestionPending, OperatorQuestionClarificationRequested, OperatorQuestionAnswered, OperatorQuestionExpired,
		OperatorQuestionSuperseded, OperatorQuestionCancelled:
		return true
	default:
		return false
	}
}

func (s OperatorQuestionStatus) Terminal() bool {
	return s.valid() && s != OperatorQuestionPending && s != OperatorQuestionClarificationRequested
}

type QuestionFallbackPolicy string

const (
	QuestionContinueOtherWork QuestionFallbackPolicy = "CONTINUE_OTHER_WORK"
	QuestionWaitLocally       QuestionFallbackPolicy = "WAIT_LOCALLY"
	QuestionApplyDefault      QuestionFallbackPolicy = "APPLY_DEFAULT"
)

func (p QuestionFallbackPolicy) valid() bool {
	switch p {
	case QuestionContinueOtherWork, QuestionWaitLocally, QuestionApplyDefault:
		return true
	default:
		return false
	}
}

type QuestionBlockingTargetKind string

const (
	QuestionBlockingOperation QuestionBlockingTargetKind = "OPERATION"
	QuestionBlockingInquiry   QuestionBlockingTargetKind = "INQUIRY"
	QuestionBlockingArtifact  QuestionBlockingTargetKind = "ARTIFACT"
	QuestionBlockingDecision  QuestionBlockingTargetKind = "DECISION"
)

func (k QuestionBlockingTargetKind) valid() bool {
	switch k {
	case QuestionBlockingOperation, QuestionBlockingInquiry, QuestionBlockingArtifact, QuestionBlockingDecision:
		return true
	default:
		return false
	}
}

type QuestionBlockingTarget struct {
	Kind      QuestionBlockingTargetKind `json:"kind"`
	Reference string                     `json:"reference"`
}

func (t QuestionBlockingTarget) Validate() error {
	if !t.Kind.valid() || strings.TrimSpace(t.Reference) == "" {
		return errors.New("question blocking target requires known kind and reference")
	}
	if len(t.Reference) > 512 {
		return errors.New("question blocking target reference exceeds byte limit")
	}
	return nil
}

func (t QuestionBlockingTarget) key() string { return string(t.Kind) + ":" + t.Reference }

type QuestionOption struct {
	ID          string `json:"option_id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

func (o QuestionOption) Validate() error {
	if strings.TrimSpace(o.ID) == "" || strings.TrimSpace(o.Label) == "" {
		return errors.New("question option requires ID and label")
	}
	if len(o.ID) > 128 || len(o.Label) > 512 || len(o.Description) > 2048 {
		return errors.New("question option exceeds byte limit")
	}
	return nil
}

// OperatorQuestion is the channel-independent, durable representation of an
// interruption request. BlockingScope lists only the local units that may wait
// for its answer; it never denotes a global mission wait.
type OperatorQuestion struct {
	SchemaVersion   int                      `json:"schema_version"`
	ID              OperatorQuestionID       `json:"question_id"`
	MissionID       MissionID                `json:"mission_id"`
	MissionRevision MissionRevisionID        `json:"mission_revision_id"`
	InquiryID       InquiryID                `json:"inquiry_id,omitempty"`
	OperationID     OperationID              `json:"operation_id,omitempty"`
	Revision        uint64                   `json:"revision"`
	Kind            OperatorQuestionKind     `json:"kind"`
	Prompt          string                   `json:"prompt"`
	Context         string                   `json:"context"`
	Options         []QuestionOption         `json:"options,omitempty"`
	AllowOther      bool                     `json:"allow_other"`
	AllowContext    bool                     `json:"allow_clarification"`
	AllowSkip       bool                     `json:"allow_skip"`
	BlockingScope   []QuestionBlockingTarget `json:"blocking_scope,omitempty"`
	FallbackPolicy  QuestionFallbackPolicy   `json:"fallback_policy"`
	DefaultOptionID string                   `json:"default_option_id,omitempty"`
	DedupSignature  string                   `json:"dedup_signature"`
	Priority        uint8                    `json:"priority"`
	Status          OperatorQuestionStatus   `json:"status"`
	CreatedAt       time.Time                `json:"created_at"`
	ExpiresAt       time.Time                `json:"expires_at,omitempty"`
	AnsweredAt      time.Time                `json:"answered_at,omitempty"`
	AnswerID        OperatorAnswerID         `json:"answer_id,omitempty"`
	SupersededBy    OperatorQuestionID       `json:"superseded_by,omitempty"`
}

func (q OperatorQuestion) Validate() error {
	if q.SchemaVersion != SchemaVersionV1 || q.ID == "" || q.MissionID == "" || q.MissionRevision == "" || q.Revision == 0 ||
		strings.TrimSpace(q.Prompt) == "" || strings.TrimSpace(q.Context) == "" || strings.TrimSpace(q.DedupSignature) == "" || q.CreatedAt.IsZero() {
		return errors.New("operator question is incomplete or has unsupported schema version")
	}
	if !q.Kind.valid() {
		return fmt.Errorf("unknown operator question kind %q", q.Kind)
	}
	if !q.FallbackPolicy.valid() {
		return fmt.Errorf("unknown question fallback policy %q", q.FallbackPolicy)
	}
	if !q.Status.valid() {
		return fmt.Errorf("unknown operator question status %q", q.Status)
	}
	if len(q.Prompt) > MaxOperatorQuestionTextBytes || len(q.Context) > MaxOperatorQuestionContextBytes || len(q.DedupSignature) > 512 {
		return errors.New("operator question text exceeds byte limit")
	}
	if q.Priority == 0 {
		return errors.New("operator question priority must be positive")
	}
	if len(q.Options) > MaxOperatorQuestionOptions {
		return errors.New("operator question has too many options")
	}
	if len(q.BlockingScope) > MaxBlockingScopeItems {
		return errors.New("operator question blocking scope exceeds item limit")
	}
	blockingKeys := make([]string, len(q.BlockingScope))
	for i, target := range q.BlockingScope {
		if err := target.Validate(); err != nil {
			return fmt.Errorf("validate blocking target %d: %w", i, err)
		}
		blockingKeys[i] = target.key()
	}
	if err := validateDistinctNonEmpty(blockingKeys, "blocking scope"); err != nil {
		return err
	}
	optionIDs := make([]string, len(q.Options))
	for i, option := range q.Options {
		if err := option.Validate(); err != nil {
			return fmt.Errorf("validate option %d: %w", i, err)
		}
		optionIDs[i] = option.ID
	}
	if err := validateDistinctNonEmpty(optionIDs, "option IDs"); err != nil {
		return err
	}
	switch q.Kind {
	case QuestionSingleChoice, QuestionMultipleChoice, QuestionSingleChoiceWithOther:
		if len(q.Options) < 2 {
			return errors.New("choice question requires at least two options")
		}
	case QuestionConfirmation:
		if len(q.Options) != 0 {
			return errors.New("confirmation question uses implicit confirm/decline options")
		}
	case QuestionFreeText, QuestionClarification:
		if len(q.Options) != 0 {
			return errors.New("free-text question must not carry options")
		}
	}
	if q.Kind == QuestionSingleChoiceWithOther && !q.AllowOther {
		return errors.New("single choice with other must allow other text")
	}
	if q.DefaultOptionID != "" {
		if q.FallbackPolicy != QuestionApplyDefault {
			return errors.New("default option requires APPLY_DEFAULT fallback")
		}
		if !containsString(optionIDs, q.DefaultOptionID) {
			return errors.New("default option does not exist")
		}
	} else if q.FallbackPolicy == QuestionApplyDefault {
		return errors.New("APPLY_DEFAULT fallback requires default option")
	}
	if !q.ExpiresAt.IsZero() && !q.ExpiresAt.After(q.CreatedAt) {
		return errors.New("question expiration must follow creation")
	}
	switch q.Status {
	case OperatorQuestionPending, OperatorQuestionClarificationRequested:
		if !q.AnsweredAt.IsZero() || q.AnswerID != "" || q.SupersededBy != "" {
			return errors.New("open question contains terminal fields")
		}
	case OperatorQuestionAnswered:
		if q.AnsweredAt.IsZero() || q.AnswerID == "" || q.AnsweredAt.Before(q.CreatedAt) || q.SupersededBy != "" {
			return errors.New("answered question has invalid answer fields")
		}
	case OperatorQuestionSuperseded:
		if q.SupersededBy == "" || q.SupersededBy == q.ID || !q.AnsweredAt.IsZero() || q.AnswerID != "" {
			return errors.New("superseded question has invalid successor or answer fields")
		}
	case OperatorQuestionExpired, OperatorQuestionCancelled:
		if !q.AnsweredAt.IsZero() || q.AnswerID != "" || q.SupersededBy != "" {
			return errors.New("closed unanswered question contains answer or successor fields")
		}
	}
	return nil
}

type OperatorAnswerKind string

const (
	AnswerOptions      OperatorAnswerKind = "OPTIONS"
	AnswerFreeText     OperatorAnswerKind = "FREE_TEXT"
	AnswerOther        OperatorAnswerKind = "OTHER"
	AnswerNeedContext  OperatorAnswerKind = "NEED_CONTEXT"
	AnswerSkip         OperatorAnswerKind = "SKIP"
	AnswerNoPreference OperatorAnswerKind = "NO_PREFERENCE"
	AnswerConfirm      OperatorAnswerKind = "CONFIRM"
	AnswerDecline      OperatorAnswerKind = "DECLINE"
)

func (k OperatorAnswerKind) valid() bool {
	switch k {
	case AnswerOptions, AnswerFreeText, AnswerOther, AnswerNeedContext, AnswerSkip,
		AnswerNoPreference, AnswerConfirm, AnswerDecline:
		return true
	default:
		return false
	}
}

// UserAnswer is untrusted until correlated to the exact question revision and
// validated by the kernel. Transport identifiers are evidence, not authority.
type UserAnswer struct {
	SchemaVersion            int                `json:"schema_version"`
	ID                       OperatorAnswerID   `json:"answer_id"`
	QuestionID               OperatorQuestionID `json:"question_id"`
	ExpectedQuestionRevision uint64             `json:"expected_question_revision"`
	Kind                     OperatorAnswerKind `json:"kind"`
	OptionIDs                []string           `json:"option_ids,omitempty"`
	Text                     string             `json:"text,omitempty"`
	ActorID                  string             `json:"actor_id"`
	Channel                  string             `json:"channel"`
	TransportEventID         string             `json:"transport_event_id"`
	TransportMessageID       string             `json:"transport_message_id,omitempty"`
	ReceivedAt               time.Time          `json:"received_at"`
}

func (a UserAnswer) Validate() error {
	if a.SchemaVersion != SchemaVersionV1 || a.ID == "" || a.QuestionID == "" || a.ExpectedQuestionRevision == 0 ||
		strings.TrimSpace(a.ActorID) == "" || strings.TrimSpace(a.Channel) == "" || strings.TrimSpace(a.TransportEventID) == "" || a.ReceivedAt.IsZero() {
		return errors.New("user answer is incomplete or has unsupported schema version")
	}
	if !a.Kind.valid() {
		return fmt.Errorf("unknown operator answer kind %q", a.Kind)
	}
	if len(a.Text) > MaxOperatorAnswerTextBytes {
		return errors.New("user answer text exceeds byte limit")
	}
	if err := validateDistinctNonEmpty(a.OptionIDs, "answer option IDs"); err != nil {
		return err
	}
	switch a.Kind {
	case AnswerOptions:
		if len(a.OptionIDs) == 0 || a.Text != "" {
			return errors.New("option answer requires option IDs and no text")
		}
	case AnswerFreeText, AnswerOther, AnswerNeedContext:
		if strings.TrimSpace(a.Text) == "" || len(a.OptionIDs) != 0 {
			return errors.New("textual answer requires text and no options")
		}
	case AnswerSkip, AnswerNoPreference, AnswerConfirm, AnswerDecline:
		if a.Text != "" || len(a.OptionIDs) != 0 {
			return errors.New("action answer must not contain text or options")
		}
	}
	return nil
}

// ValidateForQuestion performs semantic correlation after both records pass
// structural validation. It deliberately does not mutate either record.
func (a UserAnswer) ValidateForQuestion(q OperatorQuestion) error {
	if err := q.Validate(); err != nil {
		return fmt.Errorf("validate question: %w", err)
	}
	if err := a.Validate(); err != nil {
		return fmt.Errorf("validate answer: %w", err)
	}
	if q.Status != OperatorQuestionPending && q.Status != OperatorQuestionClarificationRequested {
		return errors.New("operator question is not open")
	}
	if a.QuestionID != q.ID || a.ExpectedQuestionRevision != q.Revision {
		return errors.New("answer does not match question identity and revision")
	}
	if a.ReceivedAt.Before(q.CreatedAt) {
		return errors.New("answer predates question")
	}
	if !q.ExpiresAt.IsZero() && a.ReceivedAt.After(q.ExpiresAt) {
		return errors.New("answer arrived after question expiration")
	}
	optionIDs := make([]string, len(q.Options))
	for i, option := range q.Options {
		optionIDs[i] = option.ID
	}
	switch a.Kind {
	case AnswerOptions:
		if q.Kind != QuestionSingleChoice && q.Kind != QuestionMultipleChoice && q.Kind != QuestionSingleChoiceWithOther {
			return errors.New("question does not accept option answers")
		}
		if q.Kind != QuestionMultipleChoice && len(a.OptionIDs) != 1 {
			return errors.New("single-choice question accepts exactly one option")
		}
		for _, id := range a.OptionIDs {
			if !containsString(optionIDs, id) {
				return fmt.Errorf("unknown answer option %q", id)
			}
		}
	case AnswerFreeText:
		if q.Kind != QuestionFreeText && q.Kind != QuestionClarification {
			return errors.New("question does not accept a primary free-text answer")
		}
	case AnswerOther:
		if !q.AllowOther {
			return errors.New("question does not allow other text")
		}
	case AnswerNeedContext:
		if !q.AllowContext {
			return errors.New("question does not allow context requests")
		}
	case AnswerSkip, AnswerNoPreference:
		if !q.AllowSkip {
			return errors.New("question does not allow skip or no preference")
		}
	case AnswerConfirm, AnswerDecline:
		if q.Kind != QuestionConfirmation {
			return errors.New("question is not a confirmation")
		}
	}
	return nil
}

func validateDistinctNonEmpty(values []string, label string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s contains an empty value", label)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("%s contains duplicate %q", label, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
