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
	Operations(domain.MissionRevisionID) ([]domain.Operation, error)
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

// OperatorQuestionReader exposes the channel-independent interruption state.
// Answers remain immutable after acceptance; question revisions use optimistic
// replacement through SaveOperatorQuestion.
type OperatorQuestionReader interface {
	OperatorQuestion(domain.OperatorQuestionID) (domain.OperatorQuestion, error)
	OperatorQuestions(domain.MissionID, domain.OperatorQuestionStatus) ([]domain.OperatorQuestion, error)
	UserAnswer(domain.OperatorAnswerID) (domain.UserAnswer, error)
	UserAnswerByTransport(channel, transportEventID string) (domain.UserAnswer, error)
	QuestionDelivery(domain.QuestionDeliveryID) (domain.QuestionDelivery, error)
	QuestionDeliveries(domain.OperatorQuestionID) ([]domain.QuestionDelivery, error)
	// QuestionDeliveryByTransport resolves a delivered outbox row by channel and
	// transport message id. Used by non-authoritative adapters to bind inbound
	// replies/callbacks to the durable question delivery without scanning all rows.
	QuestionDeliveryByTransport(channel, transportMessageID string) (domain.QuestionDelivery, error)
	DueQuestionDeliveries(time.Time, int) ([]domain.QuestionDelivery, error)
	QuestionGateDecision(domain.QuestionGateDecisionID) (domain.QuestionGateDecisionRecord, error)
	QuestionGateDecisionByQuestion(domain.OperatorQuestionID) (domain.QuestionGateDecisionRecord, error)
	QuestionGateDecisions(domain.MissionID) ([]domain.QuestionGateDecisionRecord, error)
}

type OperatorQuestionWriter interface {
	CreateOperatorQuestion(domain.OperatorQuestion) error
	SaveOperatorQuestion(domain.OperatorQuestion, uint64) error
	AcceptUserAnswer(domain.UserAnswer, domain.OperatorQuestion, uint64) error
	CreateQuestionDelivery(domain.QuestionDelivery) error
	SaveQuestionDelivery(domain.QuestionDelivery, domain.QuestionDeliveryStatus, uint32) error
	CreateQuestionGateDecision(domain.QuestionGateDecisionRecord) error
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

// PeerSyncReader exposes only remote, authority-free sync evidence and its
// resumable source-local cursor.
type PeerSyncReader interface {
	PeerSyncInboxRecord(peerID, originID, messageID string) (domain.PeerSyncInboxRecord, error)
	PendingPeerSyncInboxRecords(peerID string, limit int) ([]domain.PeerSyncInboxRecord, error)
	PeerSyncCursor(peerID, originID, streamID string, direction domain.PeerSyncCursorDirection) (domain.PeerSyncCursor, error)
}

type PeerSyncWriter interface {
	PutPeerSyncInboxRecord(domain.PeerSyncInboxRecord) (domain.PeerSyncInboxRecord, bool, error)
	DeletePeerSyncInboxRecord(peerID, originID, messageID string) error
	SavePeerSyncCursor(domain.PeerSyncCursor, uint64) error
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
	Source(domain.SourceID) (domain.Source, error)
	// Sources returns all sources sorted by ID.
	Sources() ([]domain.Source, error)
	SourceVersion(domain.SourceVersionID) (domain.SourceVersion, error)
	// SourceVersions returns versions for a source sorted by ObservedAt then ID.
	// Empty sourceID lists every version in the store.
	SourceVersions(domain.SourceID) ([]domain.SourceVersion, error)
	SourceSnapshot(domain.SourceVersionID) (domain.SourceSnapshot, error)
	SourceFragment(domain.SourceFragmentID) (domain.SourceFragment, error)
	SourceFragments(domain.SourceVersionID) ([]domain.SourceFragment, error)
	Observation(domain.ObservationID) (domain.Observation, error)
	// Observations returns all observations sorted by ID.
	Observations() ([]domain.Observation, error)
	Claim(domain.ClaimID) (domain.Claim, error)
	// Claims returns all claims sorted by ID.
	Claims() ([]domain.Claim, error)
	EvidenceLink(domain.EvidenceLinkID) (domain.EvidenceLink, error)
	EvidenceLinksForClaim(domain.ClaimID) ([]domain.EvidenceLink, error)
	// EvidenceLinks returns all evidence links sorted by ID.
	EvidenceLinks() ([]domain.EvidenceLink, error)
	KnowledgeArtifact(domain.ArtifactID) (domain.KnowledgeArtifact, error)
	// KnowledgeArtifacts returns all knowledge artifacts sorted by ID.
	KnowledgeArtifacts() ([]domain.KnowledgeArtifact, error)
	RawModelOutput(domain.ArtifactID) (domain.RawModelOutput, error)
	ProposedChangeSet(domain.ChangeSetID) (domain.ProposedChangeSet, error)
	AcceptedChangeSet(domain.ChangeSetID) (domain.AcceptedChangeSet, error)
	ValidationReceipt(domain.ReceiptID) (domain.ValidationReceipt, error)
	CommitReceipt(domain.ReceiptID) (domain.CommitReceipt, error)
	Commit(domain.CommitID) (domain.Commit, error)
	// Commits returns all commits sorted by Version then ID (stable browse order).
	Commits() ([]domain.Commit, error)
	CommitByIdempotencyKey(domain.IdempotencyKey) (domain.Commit, error)
	HeadCommit(domain.MissionRevisionID) (domain.Commit, error)
	CanonicalEntity(entityType, entityID string) (domain.CanonicalEntity, error)
}

type KnowledgeWriter interface {
	AppendSource(domain.Source, domain.SourceVersion, domain.SourceSnapshot) error
	AppendSourceFragments(domain.SourceVersionID, []domain.SourceFragment) error
	AppendObservation(domain.Observation) error
	AppendClaimWithEvidence(domain.Claim, []domain.EvidenceLink) error
	AppendEvidenceLinks(domain.ClaimID, []domain.EvidenceLink) error
	AppendKnowledgeArtifact(domain.KnowledgeArtifact) error
	SaveKnowledgeArtifact(domain.KnowledgeArtifact) error
	AppendRawModelOutput(domain.RawModelOutput) error
	AppendProposedChangeSet(domain.ProposedChangeSet) error
	AppendAcceptedChangeSet(domain.AcceptedChangeSet) error
	AppendValidationReceipt(domain.ValidationReceipt) error
	ApplyCommit(domain.Commit, domain.CommitReceipt, []domain.Change) error
}

// ControlReader exposes durable process/mission control, operator commands, and
// untrusted external stimuli. Transports must not obtain writers that mutate
// control state directly.
type ControlReader interface {
	ControlState() (domain.ControlState, error)
	OperatorCommand(domain.CommandID) (domain.OperatorCommand, error)
	OperatorCommandByIdempotency(domain.IdempotencyKey) (domain.OperatorCommand, error)
	OperatorCommandReceipt(domain.CommandID) (domain.CommandReceipt, error)
	PendingOperatorCommands(limit int) ([]domain.OperatorCommand, error)
	ExternalEvent(domain.ExternalEventID) (domain.ExternalEvent, error)
	ExternalEventByDeduplicationKey(string) (domain.ExternalEvent, error)
	ExternalEventDisposition(domain.ExternalEventID) (domain.ExternalEventDisposition, error)
	PendingExternalEvents(limit int) ([]domain.ExternalEvent, error)
	// ChannelCursor returns the durable non-authoritative transport position for a
	// channel key (for example Telegram getUpdates offset). Missing keys return ErrNotFound.
	ChannelCursor(channel string) (domain.ChannelCursor, error)
}

// ControlWriter persists operator commands, external events, and kernel-applied
// control effects. External event content remains untrusted data.
type ControlWriter interface {
	CreateOperatorCommand(domain.OperatorCommand, domain.CommandReceipt) error
	SaveOperatorCommandReceipt(domain.CommandReceipt) error
	CreateExternalEvent(domain.ExternalEvent, domain.ExternalEventDisposition) error
	SaveExternalEventDisposition(domain.ExternalEventDisposition) error
	SaveControlState(domain.ControlState, uint64) error
	// SaveChannelCursor persists a transport cursor with optimistic concurrency.
	// expectedRevision is the revision observed by the writer (0 when creating).
	// Cursors are non-authoritative and must never grant capability or model power.
	SaveChannelCursor(domain.ChannelCursor, uint64) error
}

// ContinuityReader exposes the work frontier and continuity diagnoses.
type ContinuityReader interface {
	WorkOpportunity(domain.WorkOpportunityID) (domain.WorkOpportunity, error)
	WorkOpportunities(domain.MissionRevisionID, domain.WorkOpportunityStatus) ([]domain.WorkOpportunity, error)
	ContinuityDiagnosis(domain.ContinuityDiagnosisID) (domain.ContinuityDiagnosis, error)
	LatestContinuityDiagnosis(domain.MissionRevisionID) (domain.ContinuityDiagnosis, error)
}

// ContinuityWriter persists frontier opportunities and continuity diagnoses.
type ContinuityWriter interface {
	CreateWorkOpportunity(domain.WorkOpportunity) error
	SaveWorkOpportunity(domain.WorkOpportunity) error
	CreateContinuityDiagnosis(domain.ContinuityDiagnosis) error
}

// ResourceReader exposes ResourceGate usage snapshots (FR-RES-001).
// Missing resources return ErrNotFound; zero usage is not auto-created on read.
type ResourceReader interface {
	ResourceUsage(domain.ResourceID) (domain.ResourceUsage, error)
	// ResourceUsages returns all persisted usage rows sorted by ResourceID.
	ResourceUsages() ([]domain.ResourceUsage, error)
}

// ResourceWriter persists ResourceGate usage after acquire/report decisions.
// Full-record replace under the serializable transaction is intentional for MVP.
type ResourceWriter interface {
	SaveResourceUsage(domain.ResourceUsage) error
}

// ModelContextReader exposes durable, binding-local context pressure. Missing
// bindings return ErrNotFound and are interpreted by the kernel as zero pressure.
type ModelContextReader interface {
	ModelContextPressure(string) (domain.ModelContextPressure, error)
	// ModelContextPressures returns all persisted pressure rows sorted by BindingID.
	ModelContextPressures() ([]domain.ModelContextPressure, error)
}

// ModelContextWriter replaces one binding's bounded context-pressure record.
type ModelContextWriter interface {
	SaveModelContextPressure(domain.ModelContextPressure) error
}

// ConfigReader exposes versioned operator configuration drafts and revisions.
// Active revision is the last applied pointer per scope.
type ConfigReader interface {
	ConfigDraft(domain.ConfigDraftID) (domain.ConfigDraft, error)
	// ConfigDrafts returns drafts for a scope, newest first by CreatedAt then ID.
	// Empty status returns all drafts; otherwise filters by exact status.
	ConfigDrafts(domain.ConfigScope, domain.ConfigDraftStatus) ([]domain.ConfigDraft, error)
	ConfigRevision(domain.ConfigRevisionID) (domain.ConfigRevision, error)
	ActiveConfigRevision(domain.ConfigScope) (domain.ConfigRevision, error)
	ConfigRevisions(domain.ConfigScope) ([]domain.ConfigRevision, error)
	ConfigApplyReceipt(domain.ConfigDraftID) (domain.ConfigApplyReceipt, error)
}

// ConfigWriter persists drafts, immutable revisions, apply receipts, and the
// active pointer. Only the kernel apply path may promote a draft to active.
type ConfigWriter interface {
	CreateConfigDraft(domain.ConfigDraft) error
	SaveConfigDraft(domain.ConfigDraft) error
	AppendConfigRevision(domain.ConfigRevision) error
	ActivateConfigRevision(domain.ConfigScope, domain.ConfigRevisionID) error
	SaveConfigApplyReceipt(domain.ConfigApplyReceipt) error
}

type Reader interface {
	MissionReader
	AgendaReader
	OperatorQuestionReader
	ControlReader
	ContinuityReader
	ConfigReader
	ResourceReader
	ModelContextReader
	EventReader
	PeerSyncReader
	IdempotencyReader
	KnowledgeReader
	MemoryReader
}

type Transaction interface {
	Reader
	MissionWriter
	AgendaWriter
	OperatorQuestionWriter
	ControlWriter
	ContinuityWriter
	ConfigWriter
	ResourceWriter
	ModelContextWriter
	EventWriter
	PeerSyncWriter
	IdempotencyWriter
	KnowledgeWriter
	MemoryWriter
}

// Store provides serializable, rollback-capable local transactions. Callbacks
// must not retain Reader or Transaction values after they return.
type Store interface {
	View(context.Context, func(Reader) error) error
	Update(context.Context, func(Transaction) error) error
	MemoryReader
}

// ReadStore is the least-authority storage boundary for projections, health
// checks, and other consumers that must never mutate canonical state.
type ReadStore interface {
	View(context.Context, func(Reader) error) error
}
