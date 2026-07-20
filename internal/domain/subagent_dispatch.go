package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type SubagentDispatchRequestID string

type SubagentDispatchStatus string

const (
	SubagentDispatchPending       SubagentDispatchStatus = "PENDING"
	SubagentDispatchLeased        SubagentDispatchStatus = "LEASED"
	SubagentDispatchDelivered     SubagentDispatchStatus = "DELIVERED"
	SubagentDispatchRetry         SubagentDispatchStatus = "RETRY"
	SubagentDispatchEffectUnknown SubagentDispatchStatus = "EFFECT_UNKNOWN"
	SubagentDispatchDead          SubagentDispatchStatus = "DEAD"
	SubagentDispatchCancelled     SubagentDispatchStatus = "CANCELLED"
)

const (
	SubagentDispatchFailureLeaseExpired      = "LEASE_EXPIRED"
	SubagentDispatchFailureEffectUnknown     = "EFFECT_UNKNOWN"
	SubagentDispatchFailureReconcileRequired = "RECONCILE_REQUIRED"
	SubagentDispatchFailureAmbiguousSend     = "AMBIGUOUS_SEND"
)

func (s SubagentDispatchStatus) valid() bool {
	switch s {
	case SubagentDispatchPending, SubagentDispatchLeased, SubagentDispatchDelivered, SubagentDispatchRetry, SubagentDispatchEffectUnknown, SubagentDispatchDead, SubagentDispatchCancelled:
		return true
	default:
		return false
	}
}

func (s SubagentDispatchStatus) Terminal() bool {
	return s == SubagentDispatchDelivered || s == SubagentDispatchDead || s == SubagentDispatchCancelled
}

func (s SubagentDispatchStatus) RequiresReconciliation() bool {
	return s == SubagentDispatchEffectUnknown
}

// SubagentDispatch is the durable outbox row for exactly one subagent
// generation. RequestID and (SessionID, Attempt) are immutable; SendAttempt is
// transport retry state and never changes generation identity.
type SubagentDispatch struct {
	SchemaVersion int                       `json:"schema_version"`
	RequestID     SubagentDispatchRequestID `json:"request_id"`
	SessionID     string                    `json:"session_id"`
	Attempt       int                       `json:"attempt"`
	PeerID        string                    `json:"peer_id"`
	// ReceiverSessionID is populated only after a correlated accepted ACK. It
	// is diagnostic/reconciliation identity; origin lifecycle correlation
	// continues to use SessionID and Attempt.
	ReceiverSessionID string                 `json:"receiver_session_id,omitempty"`
	Status            SubagentDispatchStatus `json:"status"`
	SendAttempt       uint32                 `json:"send_attempt"`
	MaxSendAttempts   uint32                 `json:"max_send_attempts"`
	AvailableAt       time.Time              `json:"available_at"`
	LeaseOwner        string                 `json:"lease_owner,omitempty"`
	LeaseUntil        time.Time              `json:"lease_until,omitempty"`
	LastFailureCode   string                 `json:"last_failure_code,omitempty"`
	ReconcileAttempts uint32                 `json:"reconcile_attempts,omitempty"`
	ReconcileAfter    time.Time              `json:"reconcile_after,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

func (d SubagentDispatch) Validate() error {
	if d.SchemaVersion != SchemaVersionV1 || strings.TrimSpace(string(d.RequestID)) == "" || strings.TrimSpace(d.SessionID) == "" || strings.TrimSpace(d.PeerID) == "" || d.AvailableAt.IsZero() || d.CreatedAt.IsZero() || d.UpdatedAt.IsZero() {
		return errors.New("subagent dispatch is incomplete or has unsupported schema version")
	}
	if !d.Status.valid() {
		return fmt.Errorf("unknown subagent dispatch status %q", d.Status)
	}
	if d.Attempt < 0 || d.MaxSendAttempts == 0 || d.SendAttempt > d.MaxSendAttempts || d.UpdatedAt.Before(d.CreatedAt) {
		return errors.New("subagent dispatch has invalid attempts or timestamps")
	}
	if len(d.RequestID) > 128 || len(d.SessionID) > 128 || len(d.PeerID) > 128 || len(d.ReceiverSessionID) > 128 || len(d.LeaseOwner) > 128 || len(d.LastFailureCode) > 128 {
		return errors.New("subagent dispatch field exceeds byte limit")
	}
	if d.Status != SubagentDispatchEffectUnknown && (d.ReconcileAttempts != 0 || !d.ReconcileAfter.IsZero()) {
		return errors.New("subagent dispatch has reconciliation schedule outside EFFECT_UNKNOWN")
	}
	switch d.Status {
	case SubagentDispatchPending, SubagentDispatchRetry:
		if d.LeaseOwner != "" || !d.LeaseUntil.IsZero() || d.ReceiverSessionID != "" {
			return errors.New("available subagent dispatch contains lease")
		}
	case SubagentDispatchLeased:
		if strings.TrimSpace(d.LeaseOwner) == "" || d.LeaseUntil.IsZero() || !d.LeaseUntil.After(d.UpdatedAt) || d.ReceiverSessionID != "" {
			return errors.New("leased subagent dispatch has invalid lease")
		}
	case SubagentDispatchEffectUnknown:
		if d.LeaseOwner != "" || !d.LeaseUntil.IsZero() || strings.TrimSpace(d.LastFailureCode) == "" || d.ReceiverSessionID != "" {
			return errors.New("effect-unknown subagent dispatch has invalid fields")
		}
		if !d.ReconcileAfter.IsZero() && d.ReconcileAfter.Before(d.UpdatedAt) {
			return errors.New("effect-unknown subagent dispatch has invalid reconciliation schedule")
		}
	case SubagentDispatchDelivered:
		// ReceiverSessionID was introduced after the outbox. Empty remains valid
		// for checkpoints delivered by older binaries; every new completion path
		// requires and persists it.
		if d.LeaseOwner != "" || !d.LeaseUntil.IsZero() || d.LastFailureCode != "" || (d.ReceiverSessionID != "" && !ValidSubagentRPCField(d.ReceiverSessionID)) {
			return errors.New("delivered subagent dispatch has invalid fields")
		}
	case SubagentDispatchDead:
		if d.LeaseOwner != "" || !d.LeaseUntil.IsZero() || strings.TrimSpace(d.LastFailureCode) == "" || d.ReceiverSessionID != "" {
			return errors.New("dead subagent dispatch has invalid fields")
		}
	case SubagentDispatchCancelled:
		if d.LeaseOwner != "" || !d.LeaseUntil.IsZero() || d.ReceiverSessionID != "" {
			return errors.New("cancelled subagent dispatch contains lease")
		}
	}
	return nil
}

func (d SubagentDispatch) Due(now time.Time) bool {
	switch d.Status {
	case SubagentDispatchPending, SubagentDispatchRetry:
		return !now.Before(d.AvailableAt)
	case SubagentDispatchLeased:
		return !now.Before(d.LeaseUntil)
	default:
		return false
	}
}

func LeaseSubagentDispatch(current SubagentDispatch, owner string, now, until time.Time) (SubagentDispatch, error) {
	if err := current.Validate(); err != nil {
		return SubagentDispatch{}, err
	}
	if current.Status.RequiresReconciliation() || (current.Status != SubagentDispatchPending && current.Status != SubagentDispatchRetry) || strings.TrimSpace(owner) == "" || now.IsZero() || !until.After(now) || !current.Due(now) {
		return SubagentDispatch{}, errors.New("subagent dispatch cannot be leased with supplied facts")
	}
	next := current
	next.Status, next.LeaseOwner, next.LeaseUntil, next.UpdatedAt = SubagentDispatchLeased, owner, until, now
	next.SendAttempt++
	return next, next.Validate()
}

func CompleteSubagentDispatch(current SubagentDispatch, owner, receiverSessionID string, now time.Time) (SubagentDispatch, error) {
	if err := current.Validate(); err != nil {
		return SubagentDispatch{}, err
	}
	if current.Status != SubagentDispatchLeased || current.LeaseOwner != owner || !ValidSubagentRPCField(receiverSessionID) || now.IsZero() || now.Before(current.UpdatedAt) {
		return SubagentDispatch{}, errors.New("subagent dispatch completion does not match active lease")
	}
	next := current
	next.Status, next.UpdatedAt, next.LastFailureCode, next.ReceiverSessionID = SubagentDispatchDelivered, now, "", receiverSessionID
	next.LeaseOwner, next.LeaseUntil = "", time.Time{}
	next.ReconcileAttempts, next.ReconcileAfter = 0, time.Time{}
	return next, next.Validate()
}

func FailSubagentDispatch(current SubagentDispatch, owner, failureCode string, now, retryAt time.Time) (SubagentDispatch, error) {
	if err := current.Validate(); err != nil {
		return SubagentDispatch{}, err
	}
	if current.Status != SubagentDispatchLeased || current.LeaseOwner != owner || strings.TrimSpace(failureCode) == "" || now.IsZero() || now.Before(current.UpdatedAt) {
		return SubagentDispatch{}, errors.New("subagent dispatch failure does not match active lease")
	}
	next := current
	next.LeaseOwner, next.LeaseUntil, next.UpdatedAt, next.LastFailureCode = "", time.Time{}, now, failureCode
	next.ReconcileAttempts, next.ReconcileAfter = 0, time.Time{}
	if current.SendAttempt >= current.MaxSendAttempts {
		next.Status = SubagentDispatchDead
	} else {
		if retryAt.IsZero() || retryAt.Before(now) {
			return SubagentDispatch{}, errors.New("retryable subagent dispatch requires future retry time")
		}
		next.Status, next.AvailableAt = SubagentDispatchRetry, retryAt
	}
	return next, next.Validate()
}

func MarkSubagentDispatchEffectUnknown(current SubagentDispatch, now time.Time, failureCode string) (SubagentDispatch, error) {
	if err := current.Validate(); err != nil {
		return SubagentDispatch{}, err
	}
	if now.IsZero() {
		return SubagentDispatch{}, errors.New("effect-unknown requires a timestamp")
	}
	code := strings.TrimSpace(failureCode)
	if code == "" {
		code = SubagentDispatchFailureEffectUnknown
	}
	switch current.Status {
	case SubagentDispatchLeased:
		if now.Before(current.LeaseUntil) {
			return SubagentDispatch{}, errors.New("subagent dispatch lease is not expired")
		}
	case SubagentDispatchEffectUnknown:
		if current.LastFailureCode == code {
			return current, nil
		}
	default:
		return SubagentDispatch{}, errors.New("only leased or effect-unknown dispatches can be marked effect-unknown")
	}
	next := current
	next.Status, next.LeaseOwner, next.LeaseUntil, next.LastFailureCode, next.UpdatedAt = SubagentDispatchEffectUnknown, "", time.Time{}, code, now
	next.ReconcileAttempts, next.ReconcileAfter = 0, now
	return next, next.Validate()
}

func ReclaimExpiredSubagentDispatch(current SubagentDispatch, now time.Time) (SubagentDispatch, error) {
	return MarkSubagentDispatchEffectUnknown(current, now, SubagentDispatchFailureLeaseExpired)
}

func MarkAmbiguousSubagentDispatch(current SubagentDispatch, owner string, now time.Time) (SubagentDispatch, error) {
	if err := current.Validate(); err != nil {
		return SubagentDispatch{}, err
	}
	if current.Status != SubagentDispatchLeased || current.LeaseOwner != owner || now.IsZero() || now.Before(current.UpdatedAt) {
		return SubagentDispatch{}, errors.New("ambiguous dispatch mark does not match active lease")
	}
	next := current
	next.Status, next.LeaseOwner, next.LeaseUntil, next.LastFailureCode, next.UpdatedAt = SubagentDispatchEffectUnknown, "", time.Time{}, SubagentDispatchFailureAmbiguousSend, now
	next.ReconcileAttempts, next.ReconcileAfter = 0, now
	return next, next.Validate()
}

// DeferSubagentDispatchReconciliation schedules another read-only evidence
// lookup. It does not authorize retry, cancellation, or any remote effect.
func DeferSubagentDispatchReconciliation(current SubagentDispatch, now, retryAt time.Time) (SubagentDispatch, error) {
	if err := current.Validate(); err != nil {
		return SubagentDispatch{}, err
	}
	if current.Status != SubagentDispatchEffectUnknown || now.IsZero() || retryAt.Before(now) || now.Before(current.UpdatedAt) {
		return SubagentDispatch{}, errors.New("dispatch reconciliation deferral requires EFFECT_UNKNOWN and a valid schedule")
	}
	next := current
	if next.ReconcileAttempts < ^uint32(0) {
		next.ReconcileAttempts++
	}
	next.ReconcileAfter, next.UpdatedAt = retryAt, now
	return next, next.Validate()
}

func ResolveSubagentDispatchEffectUnknown(current SubagentDispatch, now, retryAt time.Time) (SubagentDispatch, error) {
	if err := current.Validate(); err != nil {
		return SubagentDispatch{}, err
	}
	if current.Status != SubagentDispatchEffectUnknown || now.IsZero() || now.Before(current.UpdatedAt) {
		return SubagentDispatch{}, errors.New("effect-unknown resolution requires EFFECT_UNKNOWN dispatch and timestamp")
	}
	next := current
	next.UpdatedAt = now
	next.ReconcileAttempts, next.ReconcileAfter = 0, time.Time{}
	if current.SendAttempt >= current.MaxSendAttempts {
		next.Status = SubagentDispatchDead
		if next.LastFailureCode == "" {
			next.LastFailureCode = SubagentDispatchFailureReconcileRequired
		}
	} else {
		if retryAt.IsZero() || retryAt.Before(now) {
			return SubagentDispatch{}, errors.New("effect-unknown resolution requires future or present retry time")
		}
		next.Status, next.AvailableAt = SubagentDispatchRetry, retryAt
	}
	return next, next.Validate()
}

func CompleteSubagentDispatchAfterReconcile(current SubagentDispatch, receiverSessionID string, now time.Time) (SubagentDispatch, error) {
	if err := current.Validate(); err != nil {
		return SubagentDispatch{}, err
	}
	if current.Status != SubagentDispatchEffectUnknown || !ValidSubagentRPCField(receiverSessionID) || now.IsZero() || now.Before(current.UpdatedAt) {
		return SubagentDispatch{}, errors.New("reconcile completion requires EFFECT_UNKNOWN dispatch")
	}
	next := current
	next.Status, next.UpdatedAt, next.LastFailureCode, next.ReceiverSessionID = SubagentDispatchDelivered, now, "", receiverSessionID
	next.ReconcileAttempts, next.ReconcileAfter = 0, time.Time{}
	return next, next.Validate()
}

func CancelSubagentDispatch(current SubagentDispatch, now time.Time) (SubagentDispatch, error) {
	if err := current.Validate(); err != nil {
		return SubagentDispatch{}, err
	}
	// LEASED and EFFECT_UNKNOWN may already have produced a remote execution.
	// Cancellation is safe only before a send starts or after explicit
	// reconciliation has returned the row to RETRY.
	if current.Status.Terminal() || current.Status == SubagentDispatchLeased || current.Status.RequiresReconciliation() || now.IsZero() || now.Before(current.UpdatedAt) {
		return SubagentDispatch{}, errors.New("subagent dispatch cannot be cancelled")
	}
	next := current
	next.Status, next.UpdatedAt = SubagentDispatchCancelled, now
	next.LeaseOwner, next.LeaseUntil = "", time.Time{}
	next.ReconcileAttempts, next.ReconcileAfter = 0, time.Time{}
	return next, next.Validate()
}
