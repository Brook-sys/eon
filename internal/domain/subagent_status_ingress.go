package domain

import (
	"errors"
	"strings"
	"time"
)

var ErrInvalidSubagentStatusIngress = errors.New("invalid subagent status ingress")

type SubagentStatusIngressState string

const (
	SubagentStatusIngressPending SubagentStatusIngressState = "PENDING"
	SubagentStatusIngressApplied SubagentStatusIngressState = "APPLIED"
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
}

func (r SubagentStatusIngressReceipt) Validate() error {
	if r.SchemaVersion != SchemaVersionV1 || !ValidSubagentRPCField(r.CallerPeerID) || !ValidSubagentRPCField(r.DeliveryID) || !ValidSubagentRPCField(r.SessionID) || r.Attempt < 0 || r.RecordedAt.IsZero() || len(r.Result) > MaxSubagentSpawnResultBytes || len(r.Failure) > MaxSubagentSpawnFailureBytes {
		return ErrInvalidSubagentStatusIngress
	}
	switch r.State {
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
		if !r.AppliedAt.IsZero() {
			return ErrInvalidSubagentStatusIngress
		}
	case SubagentStatusIngressApplied:
		if r.AppliedAt.IsZero() || r.AppliedAt.Before(r.RecordedAt) {
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
