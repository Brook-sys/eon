package domain

import "time"

type FailureClass string
type FailureLocus string
type RetryDisposition string
type EffectState string
type FailureScope string

const (
	FailureValidation FailureClass = "VALIDATION"
	FailureAuthority  FailureClass = "AUTHORITY"
	FailureResource   FailureClass = "RESOURCE"
	FailureDependency FailureClass = "DEPENDENCY"
	FailureConflict   FailureClass = "CONFLICT"
	FailureIntegrity  FailureClass = "INTEGRITY"
	FailureSecurity   FailureClass = "SECURITY"
	FailureProgress   FailureClass = "PROGRESS"
	FailureInternal   FailureClass = "INTERNAL"

	NoRetry         RetryDisposition = "NO_RETRY"
	RetryNow        RetryDisposition = "RETRY_NOW"
	RetryAfter      RetryDisposition = "RETRY_AFTER"
	Replan          RetryDisposition = "REPLAN"
	Reconcile       RetryDisposition = "RECONCILE"
	RequireApproval RetryDisposition = "REQUIRE_APPROVAL"
	Quarantine      RetryDisposition = "QUARANTINE"
	PauseMission    RetryDisposition = "PAUSE_MISSION"

	EffectNotStarted EffectState = "NOT_STARTED"
	EffectNotApplied EffectState = "NOT_APPLIED"
	EffectApplied    EffectState = "APPLIED"
	EffectUnknown    EffectState = "UNKNOWN"
	EffectPartial    EffectState = "PARTIAL"

	ScopeAttempt   FailureScope = "ATTEMPT"
	ScopeOperation FailureScope = "OPERATION"
	ScopeInquiry   FailureScope = "INQUIRY"
	ScopeMission   FailureScope = "MISSION"
	ScopeRuntime   FailureScope = "RUNTIME"
)

type FailureRecord struct {
	SchemaVersion      int               `json:"schema_version"`
	ID                 FailureID         `json:"id"`
	Code               string            `json:"code"`
	Class              FailureClass      `json:"class"`
	Locus              FailureLocus      `json:"locus"`
	RetryDisposition   RetryDisposition  `json:"retry_disposition"`
	EffectState        EffectState       `json:"effect_state"`
	Scope              FailureScope      `json:"scope"`
	MissionRevision    MissionRevisionID `json:"mission_revision_id"`
	InquiryID          InquiryID         `json:"inquiry_id,omitempty"`
	OperationID        OperationID       `json:"operation_id,omitempty"`
	Attempt            uint32            `json:"attempt"`
	OccurredAt         time.Time         `json:"occurred_at"`
	RetryAt            *time.Time        `json:"retry_at,omitempty"`
	SafeDetail         string            `json:"safe_detail"`
	CauseRef           string            `json:"cause_ref,omitempty"`
	EvidenceReceiptIDs []ReceiptID       `json:"evidence_receipt_ids,omitempty"`
	PolicyVersion      string            `json:"policy_version"`
}
