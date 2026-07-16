package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type QuestionDeliveryID string

type QuestionDeliveryStatus string

const (
	QuestionDeliveryPending       QuestionDeliveryStatus = "PENDING"
	QuestionDeliveryLeased        QuestionDeliveryStatus = "LEASED"
	QuestionDeliveryDelivered     QuestionDeliveryStatus = "DELIVERED"
	QuestionDeliveryRetry         QuestionDeliveryStatus = "RETRY"
	QuestionDeliveryEffectUnknown QuestionDeliveryStatus = "EFFECT_UNKNOWN"
	QuestionDeliveryDead          QuestionDeliveryStatus = "DEAD"
	QuestionDeliveryCancelled     QuestionDeliveryStatus = "CANCELLED"
)

const (
	// Failure codes recorded on delivery transitions. Keep them stable for operators.
	DeliveryFailureLeaseExpired       = "LEASE_EXPIRED"
	DeliveryFailureEffectUnknown      = "EFFECT_UNKNOWN"
	DeliveryFailureReconcileRequired  = "RECONCILE_REQUIRED"
	DeliveryFailureAmbiguousTransport = "AMBIGUOUS_TRANSPORT"
)

func (s QuestionDeliveryStatus) valid() bool {
	switch s {
	case QuestionDeliveryPending, QuestionDeliveryLeased, QuestionDeliveryDelivered, QuestionDeliveryRetry, QuestionDeliveryEffectUnknown, QuestionDeliveryDead, QuestionDeliveryCancelled:
		return true
	default:
		return false
	}
}

func (s QuestionDeliveryStatus) Terminal() bool {
	return s == QuestionDeliveryDelivered || s == QuestionDeliveryDead || s == QuestionDeliveryCancelled
}

// RequiresReconciliation reports whether the delivery must not be re-leased until
// an explicit reconciliation transition proves the previous attempt is safe.
func (s QuestionDeliveryStatus) RequiresReconciliation() bool {
	return s == QuestionDeliveryEffectUnknown
}

// QuestionDelivery is an adapter-neutral outbox item. Payload is reconstructed
// from the canonical question; the record stores routing and delivery facts,
// never bot tokens or other credentials.
type QuestionDelivery struct {
	SchemaVersion      int                    `json:"schema_version"`
	ID                 QuestionDeliveryID     `json:"delivery_id"`
	QuestionID         OperatorQuestionID     `json:"question_id"`
	QuestionRevision   uint64                 `json:"question_revision"`
	Channel            string                 `json:"channel"`
	DestinationRef     string                 `json:"destination_ref"`
	Status             QuestionDeliveryStatus `json:"status"`
	Attempt            uint32                 `json:"attempt"`
	MaxAttempts        uint32                 `json:"max_attempts"`
	AvailableAt        time.Time              `json:"available_at"`
	LeaseOwner         string                 `json:"lease_owner,omitempty"`
	LeaseUntil         time.Time              `json:"lease_until,omitempty"`
	TransportMessageID string                 `json:"transport_message_id,omitempty"`
	LastFailureCode    string                 `json:"last_failure_code,omitempty"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
}

func (d QuestionDelivery) Validate() error {
	if d.SchemaVersion != SchemaVersionV1 || d.ID == "" || d.QuestionID == "" || d.QuestionRevision == 0 || strings.TrimSpace(d.Channel) == "" || strings.TrimSpace(d.DestinationRef) == "" || d.CreatedAt.IsZero() || d.UpdatedAt.IsZero() || d.AvailableAt.IsZero() {
		return errors.New("question delivery is incomplete or has unsupported schema version")
	}
	if !d.Status.valid() {
		return fmt.Errorf("unknown question delivery status %q", d.Status)
	}
	if d.MaxAttempts == 0 || d.Attempt > d.MaxAttempts || d.UpdatedAt.Before(d.CreatedAt) {
		return errors.New("question delivery has invalid attempts or timestamps")
	}
	if len(d.Channel) > 64 || len(d.DestinationRef) > 512 || len(d.LeaseOwner) > 256 || len(d.TransportMessageID) > 512 || len(d.LastFailureCode) > 256 {
		return errors.New("question delivery field exceeds byte limit")
	}
	switch d.Status {
	case QuestionDeliveryPending, QuestionDeliveryRetry:
		if d.LeaseOwner != "" || !d.LeaseUntil.IsZero() || d.TransportMessageID != "" {
			return errors.New("available question delivery contains lease or transport result")
		}
	case QuestionDeliveryLeased:
		if d.LeaseOwner == "" || d.LeaseUntil.IsZero() || !d.LeaseUntil.After(d.UpdatedAt) || d.TransportMessageID != "" {
			return errors.New("leased question delivery has invalid lease")
		}
	case QuestionDeliveryEffectUnknown:
		// Ambiguous transport outcome: no active lease, no proven message id.
		// Must not auto-lease; reconciliation is explicit.
		if d.LeaseOwner != "" || !d.LeaseUntil.IsZero() || d.TransportMessageID != "" {
			return errors.New("effect-unknown delivery contains lease or transport result")
		}
		if d.LastFailureCode == "" {
			return errors.New("effect-unknown delivery requires a failure code")
		}
	case QuestionDeliveryDelivered:
		if d.TransportMessageID == "" || d.LeaseOwner != "" || !d.LeaseUntil.IsZero() || d.LastFailureCode != "" {
			return errors.New("delivered question delivery has invalid result fields")
		}
	case QuestionDeliveryDead:
		if d.LastFailureCode == "" || d.LeaseOwner != "" || !d.LeaseUntil.IsZero() || d.TransportMessageID != "" {
			return errors.New("dead question delivery has invalid failure fields")
		}
	case QuestionDeliveryCancelled:
		if d.LeaseOwner != "" || !d.LeaseUntil.IsZero() || d.TransportMessageID != "" {
			return errors.New("cancelled question delivery contains lease or transport result")
		}
	}
	return nil
}

// Due reports whether a delivery is eligible for automatic worker leasing.
// EFFECT_UNKNOWN is intentionally never due: expired leases must be reconciled
// before another external effect is attempted.
func (d QuestionDelivery) Due(now time.Time) bool {
	switch d.Status {
	case QuestionDeliveryPending, QuestionDeliveryRetry:
		return !now.Before(d.AvailableAt)
	case QuestionDeliveryLeased:
		// Expired leases surface in due queries so workers can park them as EFFECT_UNKNOWN.
		return !now.Before(d.LeaseUntil)
	default:
		return false
	}
}

func LeaseQuestionDelivery(current QuestionDelivery, owner string, now, until time.Time) (QuestionDelivery, error) {
	if err := current.Validate(); err != nil {
		return QuestionDelivery{}, err
	}
	if current.Status.RequiresReconciliation() {
		return QuestionDelivery{}, errors.New("question delivery requires reconciliation before lease")
	}
	if strings.TrimSpace(owner) == "" || now.IsZero() || until.IsZero() || !until.After(now) || !current.Due(now) {
		return QuestionDelivery{}, errors.New("question delivery cannot be leased with supplied facts")
	}
	if current.Status != QuestionDeliveryPending && current.Status != QuestionDeliveryRetry {
		return QuestionDelivery{}, errors.New("question delivery status is not leaseable")
	}
	next := current
	next.Status, next.LeaseOwner, next.LeaseUntil, next.UpdatedAt = QuestionDeliveryLeased, owner, until, now
	next.Attempt++
	return next, next.Validate()
}

func CompleteQuestionDelivery(current QuestionDelivery, owner, transportMessageID string, now time.Time) (QuestionDelivery, error) {
	if err := current.Validate(); err != nil {
		return QuestionDelivery{}, err
	}
	if current.Status != QuestionDeliveryLeased || current.LeaseOwner != owner || strings.TrimSpace(transportMessageID) == "" || now.IsZero() || now.Before(current.UpdatedAt) {
		return QuestionDelivery{}, errors.New("question delivery completion does not match active lease")
	}
	next := current
	next.Status, next.TransportMessageID, next.UpdatedAt = QuestionDeliveryDelivered, transportMessageID, now
	next.LeaseOwner, next.LeaseUntil, next.LastFailureCode = "", time.Time{}, ""
	return next, next.Validate()
}

func FailQuestionDelivery(current QuestionDelivery, owner, failureCode string, now, retryAt time.Time) (QuestionDelivery, error) {
	if err := current.Validate(); err != nil {
		return QuestionDelivery{}, err
	}
	if current.Status != QuestionDeliveryLeased || current.LeaseOwner != owner || strings.TrimSpace(failureCode) == "" || now.IsZero() || now.Before(current.UpdatedAt) {
		return QuestionDelivery{}, errors.New("question delivery failure does not match active lease")
	}
	next := current
	next.LeaseOwner, next.LeaseUntil, next.UpdatedAt, next.LastFailureCode = "", time.Time{}, now, failureCode
	if current.Attempt >= current.MaxAttempts {
		next.Status = QuestionDeliveryDead
	} else {
		if retryAt.IsZero() || retryAt.Before(now) {
			return QuestionDelivery{}, errors.New("retryable question delivery requires future retry time")
		}
		next.Status, next.AvailableAt = QuestionDeliveryRetry, retryAt
	}
	return next, next.Validate()
}

// PermanentlyFailQuestionDelivery records a non-retryable adapter failure
// without mutating the immutable attempt policy.
func PermanentlyFailQuestionDelivery(current QuestionDelivery, owner, failureCode string, now time.Time) (QuestionDelivery, error) {
	if err := current.Validate(); err != nil {
		return QuestionDelivery{}, err
	}
	if current.Status != QuestionDeliveryLeased || current.LeaseOwner != owner || strings.TrimSpace(failureCode) == "" || now.IsZero() || now.Before(current.UpdatedAt) {
		return QuestionDelivery{}, errors.New("question delivery permanent failure does not match active lease")
	}
	next := current
	next.Status, next.UpdatedAt, next.LastFailureCode = QuestionDeliveryDead, now, failureCode
	next.LeaseOwner, next.LeaseUntil, next.TransportMessageID = "", time.Time{}, ""
	return next, next.Validate()
}

// MarkQuestionDeliveryEffectUnknown parks an expired lease (or ambiguous send)
// so the worker will not re-lease until explicit reconciliation. This is the
// only automatic transition allowed after lease expiry.
func MarkQuestionDeliveryEffectUnknown(current QuestionDelivery, now time.Time, failureCode string) (QuestionDelivery, error) {
	if err := current.Validate(); err != nil {
		return QuestionDelivery{}, err
	}
	if now.IsZero() {
		return QuestionDelivery{}, errors.New("effect-unknown requires a timestamp")
	}
	code := strings.TrimSpace(failureCode)
	if code == "" {
		code = DeliveryFailureEffectUnknown
	}
	switch current.Status {
	case QuestionDeliveryLeased:
		if now.Before(current.LeaseUntil) {
			return QuestionDelivery{}, errors.New("question delivery lease is not expired")
		}
	case QuestionDeliveryEffectUnknown:
		// Idempotent re-mark preserves code if already parked.
		if current.LastFailureCode == code {
			return current, nil
		}
	default:
		return QuestionDelivery{}, errors.New("only leased or effect-unknown deliveries can be marked effect-unknown")
	}
	next := current
	next.Status = QuestionDeliveryEffectUnknown
	next.LeaseOwner, next.LeaseUntil = "", time.Time{}
	next.TransportMessageID = ""
	next.LastFailureCode = code
	next.UpdatedAt = now
	// Keep AvailableAt as the original due time for audit ordering; not leaseable.
	return next, next.Validate()
}

// ReclaimExpiredQuestionDelivery is kept as a named entry point for workers.
// It does NOT return a retryable due item; it parks the delivery as EFFECT_UNKNOWN.
func ReclaimExpiredQuestionDelivery(current QuestionDelivery, now time.Time) (QuestionDelivery, error) {
	return MarkQuestionDeliveryEffectUnknown(current, now, DeliveryFailureLeaseExpired)
}

// ResolveQuestionDeliveryEffectUnknown allows an explicit reconcilation action
// to re-enable retry after proving that re-sending is safe (no silent duplicate),
// or to dead-letter when attempts are exhausted.
func ResolveQuestionDeliveryEffectUnknown(current QuestionDelivery, now, retryAt time.Time) (QuestionDelivery, error) {
	if err := current.Validate(); err != nil {
		return QuestionDelivery{}, err
	}
	if current.Status != QuestionDeliveryEffectUnknown || now.IsZero() {
		return QuestionDelivery{}, errors.New("effect-unknown resolution requires EFFECT_UNKNOWN delivery and timestamp")
	}
	next := current
	next.UpdatedAt = now
	next.LeaseOwner, next.LeaseUntil, next.TransportMessageID = "", time.Time{}, ""
	if current.Attempt >= current.MaxAttempts {
		next.Status = QuestionDeliveryDead
		if next.LastFailureCode == "" {
			next.LastFailureCode = DeliveryFailureReconcileRequired
		}
		return next, next.Validate()
	}
	if retryAt.IsZero() || retryAt.Before(now) {
		return QuestionDelivery{}, errors.New("effect-unknown resolution requires future or present retry time")
	}
	next.Status = QuestionDeliveryRetry
	next.AvailableAt = retryAt
	// Keep last failure code for operator diagnosis until a successful delivery clears it.
	return next, next.Validate()
}

// CompleteQuestionDeliveryAfterReconcile records that reconciliation found a
// transport message id for a previously ambiguous attempt.
func CompleteQuestionDeliveryAfterReconcile(current QuestionDelivery, transportMessageID string, now time.Time) (QuestionDelivery, error) {
	if err := current.Validate(); err != nil {
		return QuestionDelivery{}, err
	}
	if current.Status != QuestionDeliveryEffectUnknown || strings.TrimSpace(transportMessageID) == "" || now.IsZero() || now.Before(current.UpdatedAt) {
		return QuestionDelivery{}, errors.New("reconcile completion requires EFFECT_UNKNOWN delivery and transport message id")
	}
	next := current
	next.Status = QuestionDeliveryDelivered
	next.TransportMessageID = transportMessageID
	next.LastFailureCode = ""
	next.LeaseOwner, next.LeaseUntil = "", time.Time{}
	next.UpdatedAt = now
	return next, next.Validate()
}

// MarkAmbiguousTransportAfterSend parks a leased delivery when the adapter
// cannot prove success or failure (timeout after send started, truncated reply, etc.).
func MarkAmbiguousTransportAfterSend(current QuestionDelivery, owner string, now time.Time) (QuestionDelivery, error) {
	if err := current.Validate(); err != nil {
		return QuestionDelivery{}, err
	}
	if current.Status != QuestionDeliveryLeased || current.LeaseOwner != owner || now.IsZero() || now.Before(current.UpdatedAt) {
		return QuestionDelivery{}, errors.New("ambiguous transport mark does not match active lease")
	}
	next := current
	next.Status = QuestionDeliveryEffectUnknown
	next.LeaseOwner, next.LeaseUntil, next.TransportMessageID = "", time.Time{}, ""
	next.LastFailureCode = DeliveryFailureAmbiguousTransport
	next.UpdatedAt = now
	return next, next.Validate()
}
