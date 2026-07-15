// Package port defines backend-independent runtime boundaries.
package port

import (
	"context"
	"errors"
	"time"

	"motor-autonomo/internal/domain"
)

var (
	ErrNotFound = errors.New("storage entity not found")
	ErrConflict = errors.New("storage conflict")
)

// MissionReader exposes immutable mission revisions and the active pointer.
type MissionReader interface {
	MissionRevision(domain.MissionRevisionID) (domain.MissionRevision, error)
	ActiveMissionRevision(domain.MissionID) (domain.MissionRevision, error)
}

// MissionWriter appends immutable revisions and changes only the active pointer.
type MissionWriter interface {
	AppendMissionRevision(domain.MissionRevision) error
	ActivateMissionRevision(domain.MissionID, domain.MissionRevisionID) error
}

// AgendaReader exposes persisted planning and execution units.
type AgendaReader interface {
	OperationSpec(domain.OperationSpecID) (domain.OperationSpec, error)
	Question(domain.QuestionID) (domain.Question, error)
	InquiryCandidate(domain.InquiryCandidateID) (domain.InquiryCandidate, error)
	Inquiry(domain.InquiryID) (domain.Inquiry, error)
	Operation(domain.OperationID) (domain.Operation, error)
}

// AgendaWriter creates immutable agenda records and saves mutable execution
// records. A duplicate create is a conflict rather than an implicit overwrite.
type AgendaWriter interface {
	AppendOperationSpec(domain.OperationSpec) error
	CreateQuestion(domain.Question) error
	CreateInquiryCandidate(domain.InquiryCandidate) error
	CreateInquiry(domain.Inquiry) error
	CreateOperation(domain.Operation) error
	SaveInquiry(domain.Inquiry) error
	SaveOperation(domain.Operation) error
}

// EventReader returns immutable events in ascending storage sequence. A zero
// afterSequence starts at the beginning; limit must be positive.
type EventReader interface {
	Events(afterSequence uint64, limit int) ([]domain.Event, error)
	EventByID(domain.EventID) (domain.Event, error)
}

type EventWriter interface {
	AppendEvent(domain.Event) (domain.Event, error)
}

// IdempotencyReader exposes the durable result of a logical intent.
type IdempotencyReader interface {
	IdempotencyRecord(domain.IdempotencyKey) (domain.IdempotencyRecord, error)
}

type IdempotencyWriter interface {
	ReserveIdempotency(domain.IdempotencyRecord) (domain.IdempotencyRecord, error)
	CompleteIdempotency(domain.IdempotencyKey, domain.ReceiptID, string, time.Time) (domain.IdempotencyRecord, error)
}

type KnowledgeReader interface {
	RawModelOutput(domain.ArtifactID) (domain.RawModelOutput, error)
	ProposedChangeSet(domain.ChangeSetID) (domain.ProposedChangeSet, error)
	AcceptedChangeSet(domain.ChangeSetID) (domain.AcceptedChangeSet, error)
	ValidationReceipt(domain.ReceiptID) (domain.ValidationReceipt, error)
	CommitReceipt(domain.ReceiptID) (domain.CommitReceipt, error)
	Commit(domain.CommitID) (domain.Commit, error)
	CommitByIdempotencyKey(domain.IdempotencyKey) (domain.Commit, error)
	HeadCommit(domain.MissionRevisionID) (domain.Commit, error)
	CanonicalEntity(entityType, entityID string) (domain.CanonicalEntity, error)
}

type KnowledgeWriter interface {
	AppendRawModelOutput(domain.RawModelOutput) error
	AppendProposedChangeSet(domain.ProposedChangeSet) error
	AppendAcceptedChangeSet(domain.AcceptedChangeSet) error
	AppendValidationReceipt(domain.ValidationReceipt) error
	ApplyCommit(domain.Commit, domain.CommitReceipt, []domain.Change) error
}

type Reader interface {
	MissionReader
	AgendaReader
	EventReader
	IdempotencyReader
	KnowledgeReader
}

type Transaction interface {
	Reader
	MissionWriter
	AgendaWriter
	EventWriter
	IdempotencyWriter
	KnowledgeWriter
}

// Store provides serializable, rollback-capable local transactions. Callbacks
// must not retain Reader or Transaction values after they return.
type Store interface {
	View(context.Context, func(Reader) error) error
	Update(context.Context, func(Transaction) error) error
}
