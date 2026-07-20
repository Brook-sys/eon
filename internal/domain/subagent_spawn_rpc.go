package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

const MaxSubagentSpawnPayloadBytes = 72 << 10
const maxSubagentSpawnTaskBytes = 64 << 10
const MaxSubagentSpawnResultBytes = 64 << 10
const MaxSubagentSpawnFailureBytes = 4 << 10

var ErrInvalidSubagentSpawnRPC = errors.New("invalid subagent spawn rpc")

type SubagentSpawnRequest struct {
	RequestID   string `json:"request_id"`
	SessionID   string `json:"session_id"`
	Attempt     int    `json:"attempt"`
	Task        string `json:"task"`
	ContextMode string `json:"context_mode"`
}

type SubagentSpawnAcknowledgement struct {
	RequestID         string `json:"request_id"`
	SessionID         string `json:"session_id"`
	Attempt           int    `json:"attempt"`
	ReceiverSessionID string `json:"receiver_session_id"`
	Accepted          bool   `json:"accepted"`
	Code              string `json:"code,omitempty"`
	Retryable         bool   `json:"retryable,omitempty"`
}

type SubagentSpawnReceiptStatus string

const (
	SubagentSpawnReceiptPending  SubagentSpawnReceiptStatus = "PENDING"
	SubagentSpawnReceiptLeased   SubagentSpawnReceiptStatus = "LEASED"
	SubagentSpawnReceiptComplete SubagentSpawnReceiptStatus = "COMPLETE"
	SubagentSpawnReceiptFailed   SubagentSpawnReceiptStatus = "FAILED"
)

func (s SubagentSpawnReceiptStatus) valid() bool {
	switch s {
	case SubagentSpawnReceiptPending, SubagentSpawnReceiptLeased, SubagentSpawnReceiptComplete, SubagentSpawnReceiptFailed:
		return true
	default:
		return false
	}
}

type SubagentStatusDeliveryState string

const (
	SubagentStatusDeliveryPending       SubagentStatusDeliveryState = "PENDING"
	SubagentStatusDeliveryInFlight      SubagentStatusDeliveryState = "IN_FLIGHT"
	SubagentStatusDeliveryDelivered     SubagentStatusDeliveryState = "DELIVERED"
	SubagentStatusDeliveryEffectUnknown SubagentStatusDeliveryState = "EFFECT_UNKNOWN"
)

func (s SubagentStatusDeliveryState) valid() bool {
	switch s {
	case SubagentStatusDeliveryPending, SubagentStatusDeliveryInFlight, SubagentStatusDeliveryDelivered, SubagentStatusDeliveryEffectUnknown:
		return true
	default:
		return false
	}
}

// SubagentSpawnReceipt is the receiver-side durable idempotency boundary for
// one authenticated subagent.spawn.v1 request. It stores both the complete
// request identity and the exact successful acknowledgement so a response lost
// after commit can be replayed after process restart without spawning again.
type SubagentSpawnReceipt struct {
	SchemaVersion     int                         `json:"schema_version"`
	CallerPeerID      string                      `json:"caller_peer_id"`
	RequestID         string                      `json:"request_id"`
	SourceSessionID   string                      `json:"source_session_id"`
	Attempt           int                         `json:"attempt"`
	Task              string                      `json:"task"`
	ContextMode       string                      `json:"context_mode"`
	ReceiverSessionID string                      `json:"receiver_session_id"`
	RecordedAt        time.Time                   `json:"recorded_at"`
	Status            SubagentSpawnReceiptStatus  `json:"status,omitempty"`
	LeaseOwner        string                      `json:"lease_owner,omitempty"`
	LeaseUntil        time.Time                   `json:"lease_until,omitempty"`
	UpdatedAt         time.Time                   `json:"updated_at,omitempty"`
	Result            string                      `json:"result,omitempty"`
	Failure           string                      `json:"failure,omitempty"`
	StatusDelivery    SubagentStatusDeliveryState `json:"status_delivery,omitempty"`
}

func (r SubagentSpawnReceipt) Validate() error {
	request := SubagentSpawnRequest{RequestID: r.RequestID, SessionID: r.SourceSessionID, Attempt: r.Attempt, Task: r.Task, ContextMode: r.ContextMode}
	if r.SchemaVersion != SchemaVersionV1 || !ValidSubagentRPCField(r.CallerPeerID) || ValidateSubagentSpawnRequest(request) != nil || !ValidSubagentRPCField(r.ReceiverSessionID) || r.RecordedAt.IsZero() {
		return ErrInvalidSubagentSpawnRPC
	}
	// Receipts written before the receiver queue was introduced had none of the
	// queue fields. Keep those checkpoints readable; queue operations interpret
	// such a receipt as PENDING with UpdatedAt equal to RecordedAt.
	if r.legacyQueueState() {
		return nil
	}
	if !r.Status.valid() || r.UpdatedAt.IsZero() || r.UpdatedAt.Before(r.RecordedAt) || len(r.LeaseOwner) > 128 || len(r.Result) > MaxSubagentSpawnResultBytes || len(r.Failure) > MaxSubagentSpawnFailureBytes {
		return ErrInvalidSubagentSpawnRPC
	}
	if r.StatusDelivery != "" && !r.StatusDelivery.valid() {
		return ErrInvalidSubagentSpawnRPC
	}
	switch r.Status {
	case SubagentSpawnReceiptPending:
		if r.LeaseOwner != "" || !r.LeaseUntil.IsZero() || r.Result != "" || r.Failure != "" || r.StatusDelivery != "" {
			return ErrInvalidSubagentSpawnRPC
		}
	case SubagentSpawnReceiptLeased:
		if !ValidSubagentRPCField(r.LeaseOwner) || !r.LeaseUntil.After(r.UpdatedAt) || r.Result != "" || r.Failure != "" || r.StatusDelivery != "" {
			return ErrInvalidSubagentSpawnRPC
		}
	case SubagentSpawnReceiptComplete:
		if r.LeaseOwner != "" || !r.LeaseUntil.IsZero() || r.Failure != "" {
			return ErrInvalidSubagentSpawnRPC
		}
	case SubagentSpawnReceiptFailed:
		if r.LeaseOwner != "" || !r.LeaseUntil.IsZero() || r.Result != "" || strings.TrimSpace(r.Failure) == "" {
			return ErrInvalidSubagentSpawnRPC
		}
	}
	return nil
}

func (r SubagentSpawnReceipt) legacyQueueState() bool {
	return r.Status == "" && r.LeaseOwner == "" && r.LeaseUntil.IsZero() && r.UpdatedAt.IsZero() && r.Result == "" && r.Failure == ""
}

func (r SubagentSpawnReceipt) normalizedQueueState() SubagentSpawnReceipt {
	if r.legacyQueueState() {
		r.Status = SubagentSpawnReceiptPending
		r.UpdatedAt = r.RecordedAt
	}
	return r
}

// Due reports whether the receipt is available to a receiver worker. Expired
// leases are due so a worker can first recover them to PENDING.
func (r SubagentSpawnReceipt) Due(now time.Time) bool {
	r = r.normalizedQueueState()
	if now.IsZero() {
		return false
	}
	switch r.Status {
	case SubagentSpawnReceiptPending:
		return true
	case SubagentSpawnReceiptLeased:
		return !now.Before(r.LeaseUntil)
	default:
		return false
	}
}

func LeaseSubagentSpawnReceipt(current SubagentSpawnReceipt, owner string, now, until time.Time) (SubagentSpawnReceipt, error) {
	if err := current.Validate(); err != nil {
		return SubagentSpawnReceipt{}, err
	}
	current = current.normalizedQueueState()
	if current.Status != SubagentSpawnReceiptPending || !ValidSubagentRPCField(owner) || now.IsZero() || now.Before(current.UpdatedAt) || !until.After(now) {
		return SubagentSpawnReceipt{}, ErrInvalidSubagentSpawnRPC
	}
	next := current
	next.Status, next.LeaseOwner, next.LeaseUntil, next.UpdatedAt = SubagentSpawnReceiptLeased, owner, until, now
	return next, next.Validate()
}

func CompleteSubagentSpawnReceipt(current SubagentSpawnReceipt, owner, result string, now time.Time) (SubagentSpawnReceipt, error) {
	if err := current.Validate(); err != nil {
		return SubagentSpawnReceipt{}, err
	}
	current = current.normalizedQueueState()
	if current.Status != SubagentSpawnReceiptLeased || current.LeaseOwner != owner || now.IsZero() || now.Before(current.UpdatedAt) || !now.Before(current.LeaseUntil) || len(result) > MaxSubagentSpawnResultBytes {
		return SubagentSpawnReceipt{}, ErrInvalidSubagentSpawnRPC
	}
	next := current
	next.Status, next.UpdatedAt, next.Result, next.StatusDelivery = SubagentSpawnReceiptComplete, now, result, SubagentStatusDeliveryPending
	next.LeaseOwner, next.LeaseUntil = "", time.Time{}
	return next, next.Validate()
}

func FailSubagentSpawnReceipt(current SubagentSpawnReceipt, owner, failure string, now time.Time) (SubagentSpawnReceipt, error) {
	if err := current.Validate(); err != nil {
		return SubagentSpawnReceipt{}, err
	}
	current = current.normalizedQueueState()
	if current.Status != SubagentSpawnReceiptLeased || current.LeaseOwner != owner || now.IsZero() || now.Before(current.UpdatedAt) || !now.Before(current.LeaseUntil) || strings.TrimSpace(failure) == "" || len(failure) > MaxSubagentSpawnFailureBytes {
		return SubagentSpawnReceipt{}, ErrInvalidSubagentSpawnRPC
	}
	next := current
	next.Status, next.UpdatedAt, next.Failure, next.StatusDelivery = SubagentSpawnReceiptFailed, now, failure, SubagentStatusDeliveryPending
	next.LeaseOwner, next.LeaseUntil = "", time.Time{}
	return next, next.Validate()
}

func RecoverExpiredSubagentSpawnReceipt(current SubagentSpawnReceipt, now time.Time) (SubagentSpawnReceipt, error) {
	if err := current.Validate(); err != nil {
		return SubagentSpawnReceipt{}, err
	}
	current = current.normalizedQueueState()
	if current.Status != SubagentSpawnReceiptLeased || now.IsZero() || now.Before(current.LeaseUntil) || now.Before(current.UpdatedAt) {
		return SubagentSpawnReceipt{}, ErrInvalidSubagentSpawnRPC
	}
	next := current
	next.Status, next.UpdatedAt = SubagentSpawnReceiptPending, now
	next.LeaseOwner, next.LeaseUntil = "", time.Time{}
	return next, next.Validate()
}

// MarkSubagentSpawnReceiptStatusDelivered records the origin's acknowledgement
// of a terminal status publication. The terminal result/failure is retained so
// request replay remains safe after publication acknowledgement.
func BeginSubagentSpawnReceiptStatusDelivery(current SubagentSpawnReceipt, now time.Time) (SubagentSpawnReceipt, error) {
	if err := current.Validate(); err != nil {
		return SubagentSpawnReceipt{}, err
	}
	current = current.normalizedQueueState()
	if (current.Status != SubagentSpawnReceiptComplete && current.Status != SubagentSpawnReceiptFailed) || (current.StatusDelivery != "" && current.StatusDelivery != SubagentStatusDeliveryPending) || now.IsZero() || now.Before(current.UpdatedAt) {
		return SubagentSpawnReceipt{}, ErrInvalidSubagentSpawnRPC
	}
	next := current
	next.StatusDelivery, next.UpdatedAt = SubagentStatusDeliveryInFlight, now
	return next, next.Validate()
}

func MarkSubagentSpawnReceiptStatusDelivered(current SubagentSpawnReceipt, now time.Time) (SubagentSpawnReceipt, error) {
	if err := current.Validate(); err != nil {
		return SubagentSpawnReceipt{}, err
	}
	current = current.normalizedQueueState()
	if current.StatusDelivery != SubagentStatusDeliveryInFlight || now.IsZero() || now.Before(current.UpdatedAt) {
		return SubagentSpawnReceipt{}, ErrInvalidSubagentSpawnRPC
	}
	next := current
	next.StatusDelivery, next.UpdatedAt = SubagentStatusDeliveryDelivered, now
	return next, next.Validate()
}

func MarkSubagentSpawnReceiptStatusEffectUnknown(current SubagentSpawnReceipt, now time.Time) (SubagentSpawnReceipt, error) {
	if err := current.Validate(); err != nil {
		return SubagentSpawnReceipt{}, err
	}
	current = current.normalizedQueueState()
	if current.StatusDelivery != SubagentStatusDeliveryInFlight || now.IsZero() || now.Before(current.UpdatedAt) {
		return SubagentSpawnReceipt{}, ErrInvalidSubagentSpawnRPC
	}
	next := current
	next.StatusDelivery, next.UpdatedAt = SubagentStatusDeliveryEffectUnknown, now
	return next, next.Validate()
}

// FailExpiredSubagentSpawnReceipt parks an expired execution lease as a
// terminal failure. Re-executing after lease loss would be unsafe because the
// prior worker may have produced an external/model effect before crashing.
func FailExpiredSubagentSpawnReceipt(current SubagentSpawnReceipt, now time.Time, failure string) (SubagentSpawnReceipt, error) {
	if err := current.Validate(); err != nil {
		return SubagentSpawnReceipt{}, err
	}
	current = current.normalizedQueueState()
	if current.Status != SubagentSpawnReceiptLeased || now.IsZero() || now.Before(current.LeaseUntil) || now.Before(current.UpdatedAt) || strings.TrimSpace(failure) == "" || len(failure) > MaxSubagentSpawnFailureBytes {
		return SubagentSpawnReceipt{}, ErrInvalidSubagentSpawnRPC
	}
	next := current
	next.Status, next.UpdatedAt, next.Failure, next.StatusDelivery = SubagentSpawnReceiptFailed, now, failure, SubagentStatusDeliveryPending
	next.LeaseOwner, next.LeaseUntil = "", time.Time{}
	return next, next.Validate()
}

func (r SubagentSpawnReceipt) Matches(callerPeerID string, request SubagentSpawnRequest) bool {
	return r.CallerPeerID == callerPeerID && r.RequestID == request.RequestID && r.SourceSessionID == request.SessionID && r.Attempt == request.Attempt && r.Task == request.Task && r.ContextMode == request.ContextMode
}

func (r SubagentSpawnReceipt) Acknowledgement() SubagentSpawnAcknowledgement {
	return SubagentSpawnAcknowledgement{RequestID: r.RequestID, SessionID: r.SourceSessionID, Attempt: r.Attempt, ReceiverSessionID: r.ReceiverSessionID, Accepted: true}
}

func EncodeSubagentSpawnRequest(request SubagentSpawnRequest) ([]byte, error) {
	if err := ValidateSubagentSpawnRequest(request); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(request)
	if err != nil || len(payload) > MaxSubagentSpawnPayloadBytes {
		return nil, ErrInvalidSubagentSpawnRPC
	}
	return payload, nil
}

func DecodeSubagentSpawnRequest(payload []byte) (SubagentSpawnRequest, error) {
	if len(payload) == 0 || len(payload) > MaxSubagentSpawnPayloadBytes {
		return SubagentSpawnRequest{}, ErrInvalidSubagentSpawnRPC
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var request SubagentSpawnRequest
	if err := decoder.Decode(&request); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return SubagentSpawnRequest{}, ErrInvalidSubagentSpawnRPC
	}
	return request, ValidateSubagentSpawnRequest(request)
}

func DecodeSubagentSpawnAcknowledgement(payload []byte) (SubagentSpawnAcknowledgement, error) {
	if len(payload) == 0 || len(payload) > MaxSubagentSpawnPayloadBytes {
		return SubagentSpawnAcknowledgement{}, ErrInvalidSubagentSpawnRPC
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var ack SubagentSpawnAcknowledgement
	if err := decoder.Decode(&ack); err != nil || decoder.Decode(&struct{}{}) != io.EOF || !ValidSubagentRPCField(ack.RequestID) || !ValidSubagentRPCField(ack.SessionID) || ack.Attempt < 0 {
		return SubagentSpawnAcknowledgement{}, ErrInvalidSubagentSpawnRPC
	}
	if ack.Accepted {
		if !ValidSubagentRPCField(ack.ReceiverSessionID) || ack.Code != "" || ack.Retryable {
			return SubagentSpawnAcknowledgement{}, ErrInvalidSubagentSpawnRPC
		}
	} else if !ValidSubagentRPCField(ack.Code) || ack.ReceiverSessionID != "" {
		return SubagentSpawnAcknowledgement{}, ErrInvalidSubagentSpawnRPC
	}
	return ack, nil
}

func ValidateSubagentSpawnRequest(request SubagentSpawnRequest) error {
	if !ValidSubagentRPCField(request.RequestID) || !ValidSubagentRPCField(request.SessionID) || request.Attempt < 0 || request.Task == "" || len(request.Task) > maxSubagentSpawnTaskBytes || (request.ContextMode != "isolated" && request.ContextMode != "fork") {
		return ErrInvalidSubagentSpawnRPC
	}
	return nil
}

func ValidSubagentRPCField(value string) bool {
	return value != "" && len(value) <= 128 && strings.TrimSpace(value) == value
}
