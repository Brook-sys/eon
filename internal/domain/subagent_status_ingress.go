package domain

import (
	"errors"
	"strings"
	"time"
)

var ErrInvalidSubagentStatusIngress = errors.New("invalid subagent status ingress")

type SubagentStatusIngressState string

const (
	SubagentStatusIngressPending  SubagentStatusIngressState = "PENDING"
	SubagentStatusIngressApplied  SubagentStatusIngressState = "APPLIED"
	SubagentStatusIngressRejected SubagentStatusIngressState = "REJECTED"
)

const (
	SubagentStatusIngressRejectionAttemptMismatch  = "ATTEMPT_MISMATCH"
	SubagentStatusIngressRejectionTerminalConflict = "TERMINAL_CONFLICT"
)

type SubagentStatusIngressReceipt struct {
	SchemaVersion int                        `json:"schema_version"`
	CallerPeerID  string                     `json:"caller_peer_id"`
	DeliveryID    string                     `json:"delivery_id"`
	SessionID     string                     `json:"session_id"`
	Attempt       int                        `json:"attempt"`
	State         string                     `json:"state"`
	Result        string                     `json:"result,omitempty"`
	Failure       string                     `json:"failure,omitempty"`
	Status        SubagentStatusIngressState `json:"status"`
	RecordedAt    time.Time                  `json:"recorded_at"`
	AppliedAt     time.Time                  `json:"applied_at,omitempty"`
	RejectedAt    time.Time                  `json:"rejected_at,omitempty"`
	RejectionCode string                     `json:"rejection_code,omitempty"`
}

func (r SubagentStatusIngressReceipt) Validate() error {
	if r.SchemaVersion != SchemaVersionV1 || !ValidSubagentRPCField(r.CallerPeerID) || !ValidSubagentRPCField(r.DeliveryID) || !ValidSubagentRPCField(r.SessionID) || r.Attempt < 0 || r.RecordedAt.IsZero() || len(r.Result) > MaxSubagentSpawnResultBytes || len(r.Failure) > MaxSubagentSpawnFailureBytes {
		return ErrInvalidSubagentStatusIngress
	}
	switch r.State {
	case "RUNNING":
		if r.Result != "" || r.Failure != "" {
			return ErrInvalidSubagentStatusIngress
		}
	case "COMPLETE":
		if r.Failure != "" {
			return ErrInvalidSubagentStatusIngress
		}
	case "FAILED":
		if r.Result != "" || strings.TrimSpace(r.Failure) == "" {
			return ErrInvalidSubagentStatusIngress
		}
	default:
		return ErrInvalidSubagentStatusIngress
	}
	switch r.Status {
	case SubagentStatusIngressPending:
		if !r.AppliedAt.IsZero() || !r.RejectedAt.IsZero() || r.RejectionCode != "" {
			return ErrInvalidSubagentStatusIngress
		}
	case SubagentStatusIngressApplied:
		if r.AppliedAt.IsZero() || r.AppliedAt.Before(r.RecordedAt) || !r.RejectedAt.IsZero() || r.RejectionCode != "" {
			return ErrInvalidSubagentStatusIngress
		}
	case SubagentStatusIngressRejected:
		if !r.AppliedAt.IsZero() || r.RejectedAt.IsZero() || r.RejectedAt.Before(r.RecordedAt) || !validSubagentStatusIngressRejectionCode(r.RejectionCode) {
			return ErrInvalidSubagentStatusIngress
		}
	default:
		return ErrInvalidSubagentStatusIngress
	}
	return nil
}

func (r SubagentStatusIngressReceipt) Matches(candidate SubagentStatusIngressReceipt) bool {
	return r.CallerPeerID == candidate.CallerPeerID && r.DeliveryID == candidate.DeliveryID && r.SessionID == candidate.SessionID && r.Attempt == candidate.Attempt && r.State == candidate.State && r.Result == candidate.Result && r.Failure == candidate.Failure
}

func MarkSubagentStatusIngressApplied(current SubagentStatusIngressReceipt, now time.Time) (SubagentStatusIngressReceipt, error) {
	if current.Validate() != nil || current.Status != SubagentStatusIngressPending || now.IsZero() || now.Before(current.RecordedAt) {
		return SubagentStatusIngressReceipt{}, ErrInvalidSubagentStatusIngress
	}
	next := current
	next.Status = SubagentStatusIngressApplied
	next.AppliedAt = now
	return next, next.Validate()
}

// RejectSubagentStatusIngressAttemptMismatch quarantines durable evidence that
// cannot belong to the active generation. Keeping the receipt preserves the
// authenticated evidence and replay contract while removing it from the
// pending queue so one stale/future observation cannot poison every cycle.
func RejectSubagentStatusIngressAttemptMismatch(current SubagentStatusIngressReceipt, now time.Time) (SubagentStatusIngressReceipt, error) {
	return rejectSubagentStatusIngress(current, now, SubagentStatusIngressRejectionAttemptMismatch)
}

// RejectSubagentStatusIngressTerminalConflict quarantines authenticated
// evidence that contradicts an already-terminal session at the same attempt.
// The evidence remains durable for audit and replay, but leaves the pending
// queue so it cannot starve later independent receipts.
func RejectSubagentStatusIngressTerminalConflict(current SubagentStatusIngressReceipt, now time.Time) (SubagentStatusIngressReceipt, error) {
	return rejectSubagentStatusIngress(current, now, SubagentStatusIngressRejectionTerminalConflict)
}

func rejectSubagentStatusIngress(current SubagentStatusIngressReceipt, now time.Time, code string) (SubagentStatusIngressReceipt, error) {
	if current.Validate() != nil || current.Status != SubagentStatusIngressPending || now.IsZero() || now.Before(current.RecordedAt) {
		return SubagentStatusIngressReceipt{}, ErrInvalidSubagentStatusIngress
	}
	next := current
	next.Status = SubagentStatusIngressRejected
	next.RejectedAt = now
	next.RejectionCode = code
	return next, next.Validate()
}

func validSubagentStatusIngressRejectionCode(code string) bool {
	return code == SubagentStatusIngressRejectionAttemptMismatch || code == SubagentStatusIngressRejectionTerminalConflict
}
