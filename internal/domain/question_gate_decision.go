package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type PersistedQuestionGateDecision string

const (
	PersistedQuestionAdmit    PersistedQuestionGateDecision = "ADMIT"
	PersistedQuestionSuppress PersistedQuestionGateDecision = "SUPPRESS"
	PersistedQuestionDefer    PersistedQuestionGateDecision = "DEFER"
)

func (d PersistedQuestionGateDecision) valid() bool {
	switch d {
	case PersistedQuestionAdmit, PersistedQuestionSuppress, PersistedQuestionDefer:
		return true
	default:
		return false
	}
}

type PersistedQuestionGateReason string

const (
	PersistedQuestionGateAllowed                  PersistedQuestionGateReason = "ALLOWED"
	PersistedQuestionGateDuplicatePending         PersistedQuestionGateReason = "DUPLICATE_PENDING"
	PersistedQuestionGateCooldown                 PersistedQuestionGateReason = "COOLDOWN"
	PersistedQuestionGatePendingLimit             PersistedQuestionGateReason = "PENDING_LIMIT"
	PersistedQuestionGateRateLimit                PersistedQuestionGateReason = "RATE_LIMIT"
	PersistedQuestionGateQuietHours               PersistedQuestionGateReason = "QUIET_HOURS"
	PersistedQuestionGatePriorityLow              PersistedQuestionGateReason = "PRIORITY_TOO_LOW"
	PersistedQuestionGateSafeDefault              PersistedQuestionGateReason = "SAFE_REVERSIBLE_DEFAULT"
	PersistedQuestionGateInsufficientAlternatives PersistedQuestionGateReason = "INSUFFICIENT_ALTERNATIVES"
)

func (r PersistedQuestionGateReason) valid() bool {
	switch r {
	case PersistedQuestionGateAllowed, PersistedQuestionGateDuplicatePending, PersistedQuestionGateCooldown,
		PersistedQuestionGatePendingLimit, PersistedQuestionGateRateLimit, PersistedQuestionGateQuietHours,
		PersistedQuestionGatePriorityLow, PersistedQuestionGateSafeDefault, PersistedQuestionGateInsufficientAlternatives:
		return true
	default:
		return false
	}
}

// QuestionGateDecisionRecord is the durable audit fact produced before any
// canonical question or delivery is created. PolicyVersion identifies the
// exact configured policy whose deterministic evaluation produced the result.
type QuestionGateDecisionRecord struct {
	SchemaVersion  int                           `json:"schema_version"`
	ID             QuestionGateDecisionID        `json:"gate_decision_id"`
	QuestionID     OperatorQuestionID            `json:"question_id"`
	MissionID      MissionID                     `json:"mission_id"`
	DedupSignature string                        `json:"dedup_signature"`
	Decision       PersistedQuestionGateDecision `json:"decision"`
	Reason         PersistedQuestionGateReason   `json:"reason"`
	PolicyVersion  string                        `json:"policy_version"`
	EvaluatedAt    time.Time                     `json:"evaluated_at"`
	RetryAfter     time.Time                     `json:"retry_after,omitempty"`
}

func (r QuestionGateDecisionRecord) Validate() error {
	if r.SchemaVersion != SchemaVersionV1 || r.ID == "" || r.QuestionID == "" || r.MissionID == "" ||
		strings.TrimSpace(r.DedupSignature) == "" || strings.TrimSpace(r.PolicyVersion) == "" || r.EvaluatedAt.IsZero() {
		return errors.New("question gate decision is incomplete or has unsupported schema version")
	}
	if !r.Decision.valid() || !r.Reason.valid() {
		return fmt.Errorf("question gate decision has unknown decision %q or reason %q", r.Decision, r.Reason)
	}
	if len(r.DedupSignature) > 512 || len(r.PolicyVersion) > 128 {
		return errors.New("question gate decision field exceeds byte limit")
	}
	if r.Decision == PersistedQuestionDefer {
		if !r.RetryAfter.IsZero() && !r.RetryAfter.After(r.EvaluatedAt) {
			return errors.New("deferred question retry must follow evaluation")
		}
	} else if !r.RetryAfter.IsZero() {
		return errors.New("only deferred question decisions may carry retry time")
	}
	if r.Decision == PersistedQuestionAdmit && r.Reason != PersistedQuestionGateAllowed {
		return errors.New("admitted question requires ALLOWED reason")
	}
	if r.Decision != PersistedQuestionAdmit && r.Reason == PersistedQuestionGateAllowed {
		return errors.New("non-admitted question cannot use ALLOWED reason")
	}
	return nil
}
