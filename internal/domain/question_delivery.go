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
	QuestionDeliveryPending   QuestionDeliveryStatus = "PENDING"
	QuestionDeliveryLeased    QuestionDeliveryStatus = "LEASED"
	QuestionDeliveryDelivered QuestionDeliveryStatus = "DELIVERED"
	QuestionDeliveryRetry     QuestionDeliveryStatus = "RETRY"
	QuestionDeliveryDead      QuestionDeliveryStatus = "DEAD"
	QuestionDeliveryCancelled QuestionDeliveryStatus = "CANCELLED"
)

func (s QuestionDeliveryStatus) valid() bool {
	switch s {
	case QuestionDeliveryPending, QuestionDeliveryLeased, QuestionDeliveryDelivered, QuestionDeliveryRetry, QuestionDeliveryDead, QuestionDeliveryCancelled:
		return true
	default:
		return false
	}
}

func (s QuestionDeliveryStatus) Terminal() bool {
	return s == QuestionDeliveryDelivered || s == QuestionDeliveryDead || s == QuestionDeliveryCancelled
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

func (d QuestionDelivery) Due(now time.Time) bool {
	if d.Status != QuestionDeliveryPending && d.Status != QuestionDeliveryRetry {
		return false
	}
	return !now.Before(d.AvailableAt)
}

func LeaseQuestionDelivery(current QuestionDelivery, owner string, now, until time.Time) (QuestionDelivery, error) {
	if err := current.Validate(); err != nil {
		return QuestionDelivery{}, err
	}
	if strings.TrimSpace(owner) == "" || now.IsZero() || until.IsZero() || !until.After(now) || !current.Due(now) {
		return QuestionDelivery{}, errors.New("question delivery cannot be leased with supplied facts")
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
