package domain

import (
	"errors"
	"fmt"
	"time"
)

// Event is an immutable fact in the local append-only log. Sequence is assigned
// by the store when the event is appended; callers must provide it as zero.
type Event struct {
	SchemaVersion   int               `json:"schema_version"`
	ID              EventID           `json:"id"`
	Sequence        uint64            `json:"sequence"`
	Kind            string            `json:"kind"`
	OccurredAt      time.Time         `json:"occurred_at"`
	Namespace       string            `json:"namespace,omitempty"`
	RequestID       string            `json:"request_id,omitempty"`
	MissionRevision MissionRevisionID `json:"mission_revision_id,omitempty"`
	InquiryID       InquiryID         `json:"inquiry_id,omitempty"`
	OperationID     OperationID       `json:"operation_id,omitempty"`
	CommitID        CommitID          `json:"commit_id,omitempty"`
	PayloadRef      string            `json:"payload_ref,omitempty"`
}

func (e Event) ValidateForAppend() error {
	if e.SchemaVersion != SchemaVersionV1 || e.ID == "" || e.Kind == "" || e.OccurredAt.IsZero() {
		return errors.New("event is incomplete or has unsupported schema version")
	}
	if e.Sequence != 0 {
		return errors.New("event sequence must be assigned by storage")
	}
	return nil
}

func (e Event) ValidatePersisted() error {
	copy := e
	copy.Sequence = 0
	if err := copy.ValidateForAppend(); err != nil {
		return err
	}
	if e.Sequence == 0 {
		return errors.New("persisted event is missing sequence")
	}
	return nil
}

type IdempotencyStatus string

const (
	IdempotencyReserved  IdempotencyStatus = "RESERVED"
	IdempotencyCompleted IdempotencyStatus = "COMPLETED"
)

// IdempotencyRecord binds one stable key to one logical operation and intent.
// Re-reserving the same binding is a replay; binding the key to another intent
// is a conflict. Completion is monotonic and may be repeated only identically.
type IdempotencyRecord struct {
	SchemaVersion int               `json:"schema_version"`
	Key           IdempotencyKey    `json:"key"`
	OperationID   OperationID       `json:"operation_id"`
	Intent        string            `json:"intent"`
	Status        IdempotencyStatus `json:"status"`
	ReservedAt    time.Time         `json:"reserved_at"`
	CompletedAt   time.Time         `json:"completed_at,omitempty"`
	ReceiptID     ReceiptID         `json:"receipt_id,omitempty"`
	ResultRef     string            `json:"result_ref,omitempty"`
}

func (r IdempotencyRecord) Validate() error {
	if r.SchemaVersion != SchemaVersionV1 || r.Key == "" || r.OperationID == "" || r.Intent == "" || r.ReservedAt.IsZero() {
		return errors.New("idempotency record is incomplete or has unsupported schema version")
	}
	switch r.Status {
	case IdempotencyReserved:
		if !r.CompletedAt.IsZero() || r.ReceiptID != "" || r.ResultRef != "" {
			return errors.New("reserved idempotency record contains completion fields")
		}
	case IdempotencyCompleted:
		if r.CompletedAt.IsZero() || r.ReceiptID == "" || r.ResultRef == "" {
			return errors.New("completed idempotency record is missing completion fields")
		}
		if r.CompletedAt.Before(r.ReservedAt) {
			return fmt.Errorf("completion time %s precedes reservation time %s", r.CompletedAt, r.ReservedAt)
		}
	default:
		return fmt.Errorf("unknown idempotency status %q", r.Status)
	}
	return nil
}
