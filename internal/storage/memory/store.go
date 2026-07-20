// Package memory implements deterministic, transactional in-memory storage.
package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

type Store struct {
	mu    sync.RWMutex
	state state
}

func (s *Store) LongTermMemory(key string) (domain.LongTermMemory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return reader{&s.state}.LongTermMemory(key)
}

func (s *Store) ListMemoriesByScope(scope domain.MemoryScope) ([]domain.LongTermMemory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return reader{&s.state}.ListMemoriesByScope(scope)
}

func (s *Store) ListExpiredMemories(now time.Time) ([]domain.LongTermMemory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return reader{&s.state}.ListExpiredMemories(now)
}

// SetActiveConfig sets an active config revision directly for testing.
func (s *Store) SetActiveConfig(ctx context.Context, scope domain.ConfigScope, version string, payload interface{}) error {
	return s.Update(ctx, func(tx port.Transaction) error {
		rev := domain.ConfigRevision{
			SchemaVersion: domain.SchemaVersionV1,
			ID:            domain.ConfigRevisionID(scope) + "@" + domain.ConfigRevisionID(version),
			Scope:         scope,
			Revision:      1,
			Applicability: domain.ConfigHot,
			ContentHash:   "test",
			ActorType:     domain.ActorKernel,
			ActorID:       "test",
			Reason:        "test",
			DraftID:       "draft_test",
			AcceptedAt:    time.Now().UTC(),
		}
		if m, ok := payload.(*domain.ModelsConfig); ok {
			rev.Models = m
		}
		draft := domain.ConfigDraft{
			SchemaVersion: domain.SchemaVersionV1,
			ID:            "draft_test",
			Scope:         scope,
			Applicability: domain.ConfigHot,
			Status:        domain.ConfigDraftOpen,
			ActorType:     domain.ActorKernel,
			ActorID:       "test",
			Reason:        "test",
			CreatedAt:     time.Now().UTC(),
			Models:        rev.Models,
		}
		if err := tx.CreateConfigDraft(draft); err != nil {
			return err
		}
		draft.Status = domain.ConfigDraftValidated
		draft.ValidatedAt = time.Now().UTC()
		if err := tx.SaveConfigDraft(draft); err != nil {
			return err
		}
		draft.Status = domain.ConfigDraftApplied
		if err := tx.SaveConfigDraft(draft); err != nil {
			return err
		}
		hash, _ := domain.ConfigPayloadHash(scope, nil, nil, nil, nil, nil, rev.Models)
		rev.ContentHash = hash
		if err := tx.AppendConfigRevision(rev); err != nil {
			return err
		}
		return tx.ActivateConfigRevision(scope, rev.ID)
	})
}

type state struct {
	subagentRecords        map[string]domain.SubagentRecord
	subagentDispatches     map[domain.SubagentDispatchRequestID]domain.SubagentDispatch
	dispatchByGeneration   map[string]domain.SubagentDispatchRequestID
	subagentSpawnReceipts  map[string]domain.SubagentSpawnReceipt
	missionRevisions       map[domain.MissionRevisionID]domain.MissionRevision
	activeMissions         map[domain.MissionID]domain.MissionRevisionID
	operationSpecs         map[domain.OperationSpecID]domain.OperationSpec
	questions              map[domain.QuestionID]domain.Question
	operatorQuestions      map[domain.OperatorQuestionID]domain.OperatorQuestion
	operatorAnswers        map[domain.OperatorAnswerID]domain.UserAnswer
	answerByTransport      map[string]domain.OperatorAnswerID
	questionDeliveries     map[domain.QuestionDeliveryID]domain.QuestionDelivery
	deliveryByRoute        map[string]domain.QuestionDeliveryID
	deliveryByTransport    map[string]domain.QuestionDeliveryID
	questionGateDecisions  map[domain.QuestionGateDecisionID]domain.QuestionGateDecisionRecord
	gateDecisionByQuestion map[domain.OperatorQuestionID]domain.QuestionGateDecisionID
	candidates             map[domain.InquiryCandidateID]domain.InquiryCandidate
	inquiries              map[domain.InquiryID]domain.Inquiry
	operations             map[domain.OperationID]domain.Operation
	events                 []domain.Event
	eventIDs               map[domain.EventID]uint64
	peerSyncInbox          map[string]domain.PeerSyncInboxRecord
	peerSyncCursors        map[string]domain.PeerSyncCursor
	idempotency            map[domain.IdempotencyKey]domain.IdempotencyRecord
	sources                map[domain.SourceID]domain.Source
	sourceVersions         map[domain.SourceVersionID]domain.SourceVersion
	sourceSnapshots        map[domain.SourceVersionID]domain.SourceSnapshot
	sourceFragments        map[domain.SourceFragmentID]domain.SourceFragment
	observations           map[domain.ObservationID]domain.Observation
	claims                 map[domain.ClaimID]domain.Claim
	evidenceLinks          map[domain.EvidenceLinkID]domain.EvidenceLink
	artifacts              map[domain.ArtifactID]domain.KnowledgeArtifact
	rawModelOutputs        map[domain.ArtifactID]domain.RawModelOutput
	proposedChanges        map[domain.ChangeSetID]domain.ProposedChangeSet
	acceptedChanges        map[domain.ChangeSetID]domain.AcceptedChangeSet
	receipts               map[domain.ReceiptID]domain.ValidationReceipt
	memories               map[string]domain.LongTermMemory

	commitReceipts            map[domain.ReceiptID]domain.CommitReceipt
	commits                   map[domain.CommitID]domain.Commit
	commitByIntent            map[domain.IdempotencyKey]domain.CommitID
	headCommits               map[domain.MissionRevisionID]domain.CommitID
	canonical                 map[string]domain.CanonicalEntity
	controlState              domain.ControlState
	hasControlState           bool
	operatorCommands          map[domain.CommandID]domain.OperatorCommand
	operatorCommandByIdem     map[domain.IdempotencyKey]domain.CommandID
	operatorCommandReceipts   map[domain.CommandID]domain.CommandReceipt
	externalEvents            map[domain.ExternalEventID]domain.ExternalEvent
	externalEventByDedup      map[string]domain.ExternalEventID
	externalEventDispositions map[domain.ExternalEventID]domain.ExternalEventDisposition
	workOpportunities         map[domain.WorkOpportunityID]domain.WorkOpportunity
	continuityDiagnoses       map[domain.ContinuityDiagnosisID]domain.ContinuityDiagnosis
	configDrafts              map[domain.ConfigDraftID]domain.ConfigDraft
	configRevisions           map[domain.ConfigRevisionID]domain.ConfigRevision
	activeConfig              map[domain.ConfigScope]domain.ConfigRevisionID
	configApplyReceipts       map[domain.ConfigDraftID]domain.ConfigApplyReceipt
	channelCursors            map[string]domain.ChannelCursor
	resourceUsages            map[domain.ResourceID]domain.ResourceUsage
	modelContextPressures     map[string]domain.ModelContextPressure
}

func New() *Store { return &Store{state: newState()} }

func newState() state {
	return state{

		subagentRecords:       make(map[string]domain.SubagentRecord),
		subagentDispatches:    make(map[domain.SubagentDispatchRequestID]domain.SubagentDispatch),
		dispatchByGeneration:  make(map[string]domain.SubagentDispatchRequestID),
		subagentSpawnReceipts: make(map[string]domain.SubagentSpawnReceipt),

		missionRevisions:          make(map[domain.MissionRevisionID]domain.MissionRevision),
		activeMissions:            make(map[domain.MissionID]domain.MissionRevisionID),
		operationSpecs:            make(map[domain.OperationSpecID]domain.OperationSpec),
		questions:                 make(map[domain.QuestionID]domain.Question),
		operatorQuestions:         make(map[domain.OperatorQuestionID]domain.OperatorQuestion),
		operatorAnswers:           make(map[domain.OperatorAnswerID]domain.UserAnswer),
		answerByTransport:         make(map[string]domain.OperatorAnswerID),
		questionDeliveries:        make(map[domain.QuestionDeliveryID]domain.QuestionDelivery),
		deliveryByRoute:           make(map[string]domain.QuestionDeliveryID),
		deliveryByTransport:       make(map[string]domain.QuestionDeliveryID),
		questionGateDecisions:     make(map[domain.QuestionGateDecisionID]domain.QuestionGateDecisionRecord),
		gateDecisionByQuestion:    make(map[domain.OperatorQuestionID]domain.QuestionGateDecisionID),
		candidates:                make(map[domain.InquiryCandidateID]domain.InquiryCandidate),
		inquiries:                 make(map[domain.InquiryID]domain.Inquiry),
		operations:                make(map[domain.OperationID]domain.Operation),
		eventIDs:                  make(map[domain.EventID]uint64),
		peerSyncInbox:             make(map[string]domain.PeerSyncInboxRecord),
		peerSyncCursors:           make(map[string]domain.PeerSyncCursor),
		idempotency:               make(map[domain.IdempotencyKey]domain.IdempotencyRecord),
		sources:                   make(map[domain.SourceID]domain.Source),
		sourceVersions:            make(map[domain.SourceVersionID]domain.SourceVersion),
		sourceSnapshots:           make(map[domain.SourceVersionID]domain.SourceSnapshot),
		sourceFragments:           make(map[domain.SourceFragmentID]domain.SourceFragment),
		observations:              make(map[domain.ObservationID]domain.Observation),
		claims:                    make(map[domain.ClaimID]domain.Claim),
		evidenceLinks:             make(map[domain.EvidenceLinkID]domain.EvidenceLink),
		artifacts:                 make(map[domain.ArtifactID]domain.KnowledgeArtifact),
		rawModelOutputs:           make(map[domain.ArtifactID]domain.RawModelOutput),
		proposedChanges:           make(map[domain.ChangeSetID]domain.ProposedChangeSet),
		acceptedChanges:           make(map[domain.ChangeSetID]domain.AcceptedChangeSet),
		receipts:                  make(map[domain.ReceiptID]domain.ValidationReceipt),
		commitReceipts:            make(map[domain.ReceiptID]domain.CommitReceipt),
		commits:                   make(map[domain.CommitID]domain.Commit),
		commitByIntent:            make(map[domain.IdempotencyKey]domain.CommitID),
		headCommits:               make(map[domain.MissionRevisionID]domain.CommitID),
		canonical:                 make(map[string]domain.CanonicalEntity),
		operatorCommands:          make(map[domain.CommandID]domain.OperatorCommand),
		operatorCommandByIdem:     make(map[domain.IdempotencyKey]domain.CommandID),
		operatorCommandReceipts:   make(map[domain.CommandID]domain.CommandReceipt),
		externalEvents:            make(map[domain.ExternalEventID]domain.ExternalEvent),
		externalEventByDedup:      make(map[string]domain.ExternalEventID),
		externalEventDispositions: make(map[domain.ExternalEventID]domain.ExternalEventDisposition),
		workOpportunities:         make(map[domain.WorkOpportunityID]domain.WorkOpportunity),
		continuityDiagnoses:       make(map[domain.ContinuityDiagnosisID]domain.ContinuityDiagnosis),
		configDrafts:              make(map[domain.ConfigDraftID]domain.ConfigDraft),
		configRevisions:           make(map[domain.ConfigRevisionID]domain.ConfigRevision),
		memories:                  make(map[string]domain.LongTermMemory),
		activeConfig:              make(map[domain.ConfigScope]domain.ConfigRevisionID),
		configApplyReceipts:       make(map[domain.ConfigDraftID]domain.ConfigApplyReceipt),
		channelCursors:            make(map[string]domain.ChannelCursor),
		resourceUsages:            make(map[domain.ResourceID]domain.ResourceUsage),
		modelContextPressures:     make(map[string]domain.ModelContextPressure),
	}
}

func (s *Store) View(ctx context.Context, fn func(port.Reader) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn(reader{state: &s.state})
}

func (s *Store) Update(ctx context.Context, fn func(port.Transaction) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}

	working := cloneState(s.state)
	if err := fn(transaction{state: &working}); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.state = working
	return nil
}

type reader struct{ state *state }
type transaction struct{ state *state }

func (t transaction) MissionRevision(id domain.MissionRevisionID) (domain.MissionRevision, error) {
	return reader(t).MissionRevision(id)
}
func (t transaction) ActiveMissionRevision(id domain.MissionID) (domain.MissionRevision, error) {
	return reader(t).ActiveMissionRevision(id)
}
func (t transaction) Question(id domain.QuestionID) (domain.Question, error) {
	return reader(t).Question(id)
}
func (t transaction) OperatorQuestion(id domain.OperatorQuestionID) (domain.OperatorQuestion, error) {
	return reader(t).OperatorQuestion(id)
}
func (t transaction) OperatorQuestions(id domain.MissionID, status domain.OperatorQuestionStatus) ([]domain.OperatorQuestion, error) {
	return reader(t).OperatorQuestions(id, status)
}
func (t transaction) UserAnswer(id domain.OperatorAnswerID) (domain.UserAnswer, error) {
	return reader(t).UserAnswer(id)
}
func (t transaction) UserAnswerByTransport(channel, transportEventID string) (domain.UserAnswer, error) {
	return reader(t).UserAnswerByTransport(channel, transportEventID)
}
func (t transaction) QuestionDelivery(id domain.QuestionDeliveryID) (domain.QuestionDelivery, error) {
	return reader(t).QuestionDelivery(id)
}
func (t transaction) QuestionDeliveries(id domain.OperatorQuestionID) ([]domain.QuestionDelivery, error) {
	return reader(t).QuestionDeliveries(id)
}
func (t transaction) QuestionDeliveryByTransport(channel, transportMessageID string) (domain.QuestionDelivery, error) {
	return reader(t).QuestionDeliveryByTransport(channel, transportMessageID)
}
func (t transaction) DueQuestionDeliveries(now time.Time, limit int) ([]domain.QuestionDelivery, error) {
	return reader(t).DueQuestionDeliveries(now, limit)
}
func (t transaction) QuestionGateDecision(id domain.QuestionGateDecisionID) (domain.QuestionGateDecisionRecord, error) {
	return reader(t).QuestionGateDecision(id)
}
func (t transaction) QuestionGateDecisionByQuestion(id domain.OperatorQuestionID) (domain.QuestionGateDecisionRecord, error) {
	return reader(t).QuestionGateDecisionByQuestion(id)
}
func (t transaction) QuestionGateDecisions(id domain.MissionID) ([]domain.QuestionGateDecisionRecord, error) {
	return reader(t).QuestionGateDecisions(id)
}
func (t transaction) ControlState() (domain.ControlState, error) {
	return reader(t).ControlState()
}
func (t transaction) ChannelCursor(channel string) (domain.ChannelCursor, error) {
	return reader(t).ChannelCursor(channel)
}
func (t transaction) ResourceUsage(id domain.ResourceID) (domain.ResourceUsage, error) {
	return reader(t).ResourceUsage(id)
}
func (t transaction) ResourceUsages() ([]domain.ResourceUsage, error) {
	return reader(t).ResourceUsages()
}
func (t transaction) ModelContextPressure(bindingID string) (domain.ModelContextPressure, error) {
	return reader(t).ModelContextPressure(bindingID)
}
func (t transaction) ModelContextPressures() ([]domain.ModelContextPressure, error) {
	return reader(t).ModelContextPressures()
}
func (t transaction) OperatorCommand(id domain.CommandID) (domain.OperatorCommand, error) {
	return reader(t).OperatorCommand(id)
}
func (t transaction) OperatorCommandByIdempotency(key domain.IdempotencyKey) (domain.OperatorCommand, error) {
	return reader(t).OperatorCommandByIdempotency(key)
}
func (t transaction) OperatorCommandReceipt(id domain.CommandID) (domain.CommandReceipt, error) {
	return reader(t).OperatorCommandReceipt(id)
}
func (t transaction) PendingOperatorCommands(limit int) ([]domain.OperatorCommand, error) {
	return reader(t).PendingOperatorCommands(limit)
}
func (t transaction) ExternalEvent(id domain.ExternalEventID) (domain.ExternalEvent, error) {
	return reader(t).ExternalEvent(id)
}
func (t transaction) ExternalEventByDeduplicationKey(key string) (domain.ExternalEvent, error) {
	return reader(t).ExternalEventByDeduplicationKey(key)
}
func (t transaction) ExternalEventDisposition(id domain.ExternalEventID) (domain.ExternalEventDisposition, error) {
	return reader(t).ExternalEventDisposition(id)
}
func (t transaction) PendingExternalEvents(limit int) ([]domain.ExternalEvent, error) {
	return reader(t).PendingExternalEvents(limit)
}
func (t transaction) WorkOpportunity(id domain.WorkOpportunityID) (domain.WorkOpportunity, error) {
	return reader(t).WorkOpportunity(id)
}
func (t transaction) WorkOpportunities(id domain.MissionRevisionID, status domain.WorkOpportunityStatus) ([]domain.WorkOpportunity, error) {
	return reader(t).WorkOpportunities(id, status)
}
func (t transaction) ContinuityDiagnosis(id domain.ContinuityDiagnosisID) (domain.ContinuityDiagnosis, error) {
	return reader(t).ContinuityDiagnosis(id)
}
func (t transaction) LatestContinuityDiagnosis(id domain.MissionRevisionID) (domain.ContinuityDiagnosis, error) {
	return reader(t).LatestContinuityDiagnosis(id)
}
func (t transaction) ConfigDraft(id domain.ConfigDraftID) (domain.ConfigDraft, error) {
	return reader(t).ConfigDraft(id)
}
func (t transaction) ConfigDrafts(scope domain.ConfigScope, status domain.ConfigDraftStatus) ([]domain.ConfigDraft, error) {
	return reader(t).ConfigDrafts(scope, status)
}
func (t transaction) ConfigRevision(id domain.ConfigRevisionID) (domain.ConfigRevision, error) {
	return reader(t).ConfigRevision(id)
}
func (t transaction) ActiveConfigRevision(scope domain.ConfigScope) (domain.ConfigRevision, error) {
	return reader(t).ActiveConfigRevision(scope)
}
func (t transaction) ConfigRevisions(scope domain.ConfigScope) ([]domain.ConfigRevision, error) {
	return reader(t).ConfigRevisions(scope)
}
func (t transaction) ConfigApplyReceipt(id domain.ConfigDraftID) (domain.ConfigApplyReceipt, error) {
	return reader(t).ConfigApplyReceipt(id)
}
func (t transaction) OperationSpec(id domain.OperationSpecID) (domain.OperationSpec, error) {
	return reader(t).OperationSpec(id)
}
func (t transaction) InquiryCandidate(id domain.InquiryCandidateID) (domain.InquiryCandidate, error) {
	return reader(t).InquiryCandidate(id)
}
func (t transaction) Inquiry(id domain.InquiryID) (domain.Inquiry, error) {
	return reader(t).Inquiry(id)
}
func (t transaction) Operation(id domain.OperationID) (domain.Operation, error) {
	return reader(t).Operation(id)
}
func (t transaction) Operations(id domain.MissionRevisionID) ([]domain.Operation, error) {
	return reader(t).Operations(id)
}
func (t transaction) Events(afterSequence uint64, limit int) ([]domain.Event, error) {
	return reader(t).Events(afterSequence, limit)
}
func (t transaction) EventByID(id domain.EventID) (domain.Event, error) {
	return reader(t).EventByID(id)
}
func (t transaction) PeerSyncInboxRecord(peerID, originID, messageID string) (domain.PeerSyncInboxRecord, error) {
	return reader(t).PeerSyncInboxRecord(peerID, originID, messageID)
}

func (t transaction) PendingPeerSyncInboxRecords(peerID string, limit int) ([]domain.PeerSyncInboxRecord, error) {
	return reader(t).PendingPeerSyncInboxRecords(peerID, limit)
}
func (t transaction) PeerSyncCursor(peerID, originID, streamID string, direction domain.PeerSyncCursorDirection) (domain.PeerSyncCursor, error) {
	return reader(t).PeerSyncCursor(peerID, originID, streamID, direction)
}
func (t transaction) IdempotencyRecord(key domain.IdempotencyKey) (domain.IdempotencyRecord, error) {
	return reader(t).IdempotencyRecord(key)
}
func (t transaction) RawModelOutput(id domain.ArtifactID) (domain.RawModelOutput, error) {
	return reader(t).RawModelOutput(id)
}
func (t transaction) ProposedChangeSet(id domain.ChangeSetID) (domain.ProposedChangeSet, error) {
	return reader(t).ProposedChangeSet(id)
}
func (t transaction) AcceptedChangeSet(id domain.ChangeSetID) (domain.AcceptedChangeSet, error) {
	return reader(t).AcceptedChangeSet(id)
}
func (t transaction) ValidationReceipt(id domain.ReceiptID) (domain.ValidationReceipt, error) {
	return reader(t).ValidationReceipt(id)
}
func (t transaction) CommitReceipt(id domain.ReceiptID) (domain.CommitReceipt, error) {
	return reader(t).CommitReceipt(id)
}
func (t transaction) Commit(id domain.CommitID) (domain.Commit, error) {
	return reader(t).Commit(id)
}
func (t transaction) CommitByIdempotencyKey(key domain.IdempotencyKey) (domain.Commit, error) {
	return reader(t).CommitByIdempotencyKey(key)
}
func (t transaction) HeadCommit(id domain.MissionRevisionID) (domain.Commit, error) {
	return reader(t).HeadCommit(id)
}
func (t transaction) CanonicalEntity(entityType, entityID string) (domain.CanonicalEntity, error) {
	return reader(t).CanonicalEntity(entityType, entityID)
}
func (t transaction) Source(id domain.SourceID) (domain.Source, error) {
	return reader(t).Source(id)
}
func (t transaction) Sources() ([]domain.Source, error) {
	return reader(t).Sources()
}
func (t transaction) SourceVersion(id domain.SourceVersionID) (domain.SourceVersion, error) {
	return reader(t).SourceVersion(id)
}
func (t transaction) SourceVersions(id domain.SourceID) ([]domain.SourceVersion, error) {
	return reader(t).SourceVersions(id)
}
func (t transaction) SourceSnapshot(id domain.SourceVersionID) (domain.SourceSnapshot, error) {
	return reader(t).SourceSnapshot(id)
}
func (t transaction) SourceFragment(id domain.SourceFragmentID) (domain.SourceFragment, error) {
	return reader(t).SourceFragment(id)
}
func (t transaction) SourceFragments(id domain.SourceVersionID) ([]domain.SourceFragment, error) {
	return reader(t).SourceFragments(id)
}
func (t transaction) Observation(id domain.ObservationID) (domain.Observation, error) {
	return reader(t).Observation(id)
}
func (t transaction) Observations() ([]domain.Observation, error) {
	return reader(t).Observations()
}
func (t transaction) Claim(id domain.ClaimID) (domain.Claim, error) {
	return reader(t).Claim(id)
}
func (t transaction) Claims() ([]domain.Claim, error) {
	return reader(t).Claims()
}
func (t transaction) EvidenceLink(id domain.EvidenceLinkID) (domain.EvidenceLink, error) {
	return reader(t).EvidenceLink(id)
}
func (t transaction) EvidenceLinksForClaim(id domain.ClaimID) ([]domain.EvidenceLink, error) {
	return reader(t).EvidenceLinksForClaim(id)
}
func (t transaction) EvidenceLinks() ([]domain.EvidenceLink, error) {
	return reader(t).EvidenceLinks()
}
func (t transaction) KnowledgeArtifact(id domain.ArtifactID) (domain.KnowledgeArtifact, error) {
	return reader(t).KnowledgeArtifact(id)
}
func (t transaction) KnowledgeArtifacts() ([]domain.KnowledgeArtifact, error) {
	return reader(t).KnowledgeArtifacts()
}

func (r reader) MissionRevision(id domain.MissionRevisionID) (domain.MissionRevision, error) {
	v, ok := r.state.missionRevisions[id]
	if !ok {
		return domain.MissionRevision{}, notFound("mission revision", id)
	}
	return cloneMission(v), nil
}
func (r reader) ActiveMissionRevision(id domain.MissionID) (domain.MissionRevision, error) {
	revisionID, ok := r.state.activeMissions[id]
	if !ok {
		return domain.MissionRevision{}, notFound("active mission", id)
	}
	return r.MissionRevision(revisionID)
}
func (r reader) Question(id domain.QuestionID) (domain.Question, error) {
	v, ok := r.state.questions[id]
	if !ok {
		return domain.Question{}, notFound("question", id)
	}
	return v, nil
}
func (r reader) OperatorQuestion(id domain.OperatorQuestionID) (domain.OperatorQuestion, error) {
	v, ok := r.state.operatorQuestions[id]
	if !ok {
		return domain.OperatorQuestion{}, notFound("operator question", id)
	}
	return cloneOperatorQuestion(v), nil
}
func (r reader) OperatorQuestions(missionID domain.MissionID, status domain.OperatorQuestionStatus) ([]domain.OperatorQuestion, error) {
	questions := make([]domain.OperatorQuestion, 0)
	for _, question := range r.state.operatorQuestions {
		if question.MissionID == missionID && (status == "" || question.Status == status) {
			questions = append(questions, cloneOperatorQuestion(question))
		}
	}
	sort.Slice(questions, func(i, j int) bool {
		if questions[i].CreatedAt.Equal(questions[j].CreatedAt) {
			return questions[i].ID < questions[j].ID
		}
		return questions[i].CreatedAt.Before(questions[j].CreatedAt)
	})
	return questions, nil
}
func (r reader) UserAnswer(id domain.OperatorAnswerID) (domain.UserAnswer, error) {
	v, ok := r.state.operatorAnswers[id]
	if !ok {
		return domain.UserAnswer{}, notFound("user answer", id)
	}
	return cloneUserAnswer(v), nil
}
func (r reader) UserAnswerByTransport(channel, transportEventID string) (domain.UserAnswer, error) {
	id, ok := r.state.answerByTransport[transportAnswerKey(channel, transportEventID)]
	if !ok {
		return domain.UserAnswer{}, notFound("user answer transport", transportAnswerKey(channel, transportEventID))
	}
	return r.UserAnswer(id)
}
func (r reader) QuestionDelivery(id domain.QuestionDeliveryID) (domain.QuestionDelivery, error) {
	v, ok := r.state.questionDeliveries[id]
	if !ok {
		return domain.QuestionDelivery{}, notFound("question delivery", id)
	}
	return v, nil
}
func (r reader) QuestionDeliveries(questionID domain.OperatorQuestionID) ([]domain.QuestionDelivery, error) {
	result := make([]domain.QuestionDelivery, 0)
	for _, delivery := range r.state.questionDeliveries {
		if delivery.QuestionID == questionID {
			result = append(result, delivery)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}
func (r reader) QuestionDeliveryByTransport(channel, transportMessageID string) (domain.QuestionDelivery, error) {
	if strings.TrimSpace(channel) == "" || strings.TrimSpace(transportMessageID) == "" {
		return domain.QuestionDelivery{}, fmt.Errorf("question delivery transport lookup requires channel and message id")
	}
	id, ok := r.state.deliveryByTransport[deliveryTransportKey(channel, transportMessageID)]
	if !ok {
		return domain.QuestionDelivery{}, notFound("question delivery transport", deliveryTransportKey(channel, transportMessageID))
	}
	return r.QuestionDelivery(id)
}
func (r reader) DueQuestionDeliveries(now time.Time, limit int) ([]domain.QuestionDelivery, error) {
	if now.IsZero() || limit <= 0 {
		return nil, fmt.Errorf("question delivery query requires time and positive limit")
	}
	result := make([]domain.QuestionDelivery, 0, limit)
	for _, delivery := range r.state.questionDeliveries {
		if delivery.Due(now) {
			result = append(result, delivery)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].AvailableAt.Equal(result[j].AvailableAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].AvailableAt.Before(result[j].AvailableAt)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}
func (r reader) QuestionGateDecision(id domain.QuestionGateDecisionID) (domain.QuestionGateDecisionRecord, error) {
	v, ok := r.state.questionGateDecisions[id]
	if !ok {
		return domain.QuestionGateDecisionRecord{}, notFound("question gate decision", id)
	}
	return v, nil
}
func (r reader) QuestionGateDecisionByQuestion(questionID domain.OperatorQuestionID) (domain.QuestionGateDecisionRecord, error) {
	id, ok := r.state.gateDecisionByQuestion[questionID]
	if !ok {
		return domain.QuestionGateDecisionRecord{}, notFound("question gate decision by question", questionID)
	}
	return r.QuestionGateDecision(id)
}
func (r reader) QuestionGateDecisions(missionID domain.MissionID) ([]domain.QuestionGateDecisionRecord, error) {
	result := make([]domain.QuestionGateDecisionRecord, 0)
	for _, decision := range r.state.questionGateDecisions {
		if decision.MissionID == missionID {
			result = append(result, decision)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].EvaluatedAt.Equal(result[j].EvaluatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].EvaluatedAt.Before(result[j].EvaluatedAt)
	})
	return result, nil
}
func (r reader) ControlState() (domain.ControlState, error) {
	if !r.state.hasControlState {
		return domain.ControlState{}, notFound("control state", "singleton")
	}
	return cloneControlState(r.state.controlState), nil
}
func (r reader) ChannelCursor(channel string) (domain.ChannelCursor, error) {
	key := strings.TrimSpace(channel)
	if key == "" {
		return domain.ChannelCursor{}, fmt.Errorf("channel cursor requires channel")
	}
	v, ok := r.state.channelCursors[key]
	if !ok {
		return domain.ChannelCursor{}, notFound("channel cursor", key)
	}
	return v, nil
}
func (r reader) ResourceUsage(id domain.ResourceID) (domain.ResourceUsage, error) {
	key := domain.ResourceID(strings.TrimSpace(string(id)))
	if key == "" {
		return domain.ResourceUsage{}, fmt.Errorf("resource usage requires resource id")
	}
	v, ok := r.state.resourceUsages[key]
	if !ok {
		return domain.ResourceUsage{}, notFound("resource usage", key)
	}
	return cloneResourceUsage(v), nil
}
func (r reader) ResourceUsages() ([]domain.ResourceUsage, error) {
	out := make([]domain.ResourceUsage, 0, len(r.state.resourceUsages))
	for _, v := range r.state.resourceUsages {
		out = append(out, cloneResourceUsage(v))
	}
	sort.Slice(out, func(i, j int) bool {
		return string(out[i].Resource) < string(out[j].Resource)
	})
	return out, nil
}

func (r reader) ModelContextPressure(bindingID string) (domain.ModelContextPressure, error) {
	key := strings.TrimSpace(bindingID)
	if key == "" {
		return domain.ModelContextPressure{}, fmt.Errorf("model context pressure requires binding id")
	}
	v, ok := r.state.modelContextPressures[key]
	if !ok {
		return domain.ModelContextPressure{}, notFound("model context pressure", key)
	}
	return v, nil
}
func (r reader) ModelContextPressures() ([]domain.ModelContextPressure, error) {
	out := make([]domain.ModelContextPressure, 0, len(r.state.modelContextPressures))
	for _, v := range r.state.modelContextPressures {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].BindingID < out[j].BindingID
	})
	return out, nil
}
func (r reader) OperatorCommand(id domain.CommandID) (domain.OperatorCommand, error) {
	v, ok := r.state.operatorCommands[id]
	if !ok {
		return domain.OperatorCommand{}, notFound("operator command", id)
	}
	return cloneOperatorCommand(v), nil
}
func (r reader) OperatorCommandByIdempotency(key domain.IdempotencyKey) (domain.OperatorCommand, error) {
	id, ok := r.state.operatorCommandByIdem[key]
	if !ok {
		return domain.OperatorCommand{}, notFound("operator command idempotency", key)
	}
	return r.OperatorCommand(id)
}
func (r reader) OperatorCommandReceipt(id domain.CommandID) (domain.CommandReceipt, error) {
	v, ok := r.state.operatorCommandReceipts[id]
	if !ok {
		return domain.CommandReceipt{}, notFound("operator command receipt", id)
	}
	return v, nil
}
func (r reader) PendingOperatorCommands(limit int) ([]domain.OperatorCommand, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("pending operator command query requires positive limit")
	}
	result := make([]domain.OperatorCommand, 0, limit)
	for _, command := range r.state.operatorCommands {
		receipt, ok := r.state.operatorCommandReceipts[command.ID]
		if !ok || receipt.State.Terminal() {
			continue
		}
		result = append(result, cloneOperatorCommand(command))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].SubmittedAt.Equal(result[j].SubmittedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].SubmittedAt.Before(result[j].SubmittedAt)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}
func (r reader) ExternalEvent(id domain.ExternalEventID) (domain.ExternalEvent, error) {
	v, ok := r.state.externalEvents[id]
	if !ok {
		return domain.ExternalEvent{}, notFound("external event", id)
	}
	return cloneExternalEvent(v), nil
}
func (r reader) ExternalEventByDeduplicationKey(key string) (domain.ExternalEvent, error) {
	if key == "" {
		return domain.ExternalEvent{}, fmt.Errorf("external event deduplication key is required")
	}
	id, ok := r.state.externalEventByDedup[key]
	if !ok {
		return domain.ExternalEvent{}, notFound("external event deduplication", key)
	}
	return r.ExternalEvent(id)
}
func (r reader) ExternalEventDisposition(id domain.ExternalEventID) (domain.ExternalEventDisposition, error) {
	v, ok := r.state.externalEventDispositions[id]
	if !ok {
		return domain.ExternalEventDisposition{}, notFound("external event disposition", id)
	}
	return v, nil
}
func (r reader) PendingExternalEvents(limit int) ([]domain.ExternalEvent, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("pending external event query requires positive limit")
	}
	result := make([]domain.ExternalEvent, 0, limit)
	for _, event := range r.state.externalEvents {
		disposition, ok := r.state.externalEventDispositions[event.ID]
		if !ok || disposition.State.Terminal() {
			continue
		}
		result = append(result, cloneExternalEvent(event))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ReceivedAt.Equal(result[j].ReceivedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].ReceivedAt.Before(result[j].ReceivedAt)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}
func (r reader) Operations(missionRevision domain.MissionRevisionID) ([]domain.Operation, error) {
	operations := make([]domain.Operation, 0)
	for _, operation := range r.state.operations {
		if operation.MissionRevision == missionRevision {
			operations = append(operations, cloneOperation(operation))
		}
	}
	sort.Slice(operations, func(i, j int) bool { return operations[i].ID < operations[j].ID })
	return operations, nil
}
func (r reader) WorkOpportunity(id domain.WorkOpportunityID) (domain.WorkOpportunity, error) {
	v, ok := r.state.workOpportunities[id]
	if !ok {
		return domain.WorkOpportunity{}, notFound("work opportunity", id)
	}
	return cloneWorkOpportunity(v), nil
}
func (r reader) WorkOpportunities(missionRevision domain.MissionRevisionID, status domain.WorkOpportunityStatus) ([]domain.WorkOpportunity, error) {
	if missionRevision == "" {
		return nil, fmt.Errorf("work opportunity query requires mission revision")
	}
	if status != "" && !status.Valid() {
		return nil, fmt.Errorf("unknown work opportunity status filter %q", status)
	}
	result := make([]domain.WorkOpportunity, 0)
	for _, opportunity := range r.state.workOpportunities {
		if opportunity.MissionRevision != missionRevision {
			continue
		}
		if status != "" && opportunity.Status != status {
			continue
		}
		result = append(result, cloneWorkOpportunity(opportunity))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Priority == result[j].Priority {
			if result[i].CreatedAt.Equal(result[j].CreatedAt) {
				return result[i].ID < result[j].ID
			}
			return result[i].CreatedAt.Before(result[j].CreatedAt)
		}
		return result[i].Priority > result[j].Priority
	})
	return result, nil
}
func (r reader) ContinuityDiagnosis(id domain.ContinuityDiagnosisID) (domain.ContinuityDiagnosis, error) {
	v, ok := r.state.continuityDiagnoses[id]
	if !ok {
		return domain.ContinuityDiagnosis{}, notFound("continuity diagnosis", id)
	}
	return cloneContinuityDiagnosis(v), nil
}
func (r reader) LatestContinuityDiagnosis(missionRevision domain.MissionRevisionID) (domain.ContinuityDiagnosis, error) {
	if missionRevision == "" {
		return domain.ContinuityDiagnosis{}, fmt.Errorf("latest continuity diagnosis requires mission revision")
	}
	var latest domain.ContinuityDiagnosis
	found := false
	for _, diagnosis := range r.state.continuityDiagnoses {
		if diagnosis.MissionRevision != missionRevision {
			continue
		}
		if !found || diagnosis.OccurredAt.After(latest.OccurredAt) || (diagnosis.OccurredAt.Equal(latest.OccurredAt) && diagnosis.ID > latest.ID) {
			latest = diagnosis
			found = true
		}
	}
	if !found {
		return domain.ContinuityDiagnosis{}, notFound("continuity diagnosis", missionRevision)
	}
	return cloneContinuityDiagnosis(latest), nil
}
func (r reader) ConfigDraft(id domain.ConfigDraftID) (domain.ConfigDraft, error) {
	v, ok := r.state.configDrafts[id]
	if !ok {
		return domain.ConfigDraft{}, notFound("config draft", id)
	}
	return cloneConfigDraft(v), nil
}
func (r reader) ConfigDrafts(scope domain.ConfigScope, status domain.ConfigDraftStatus) ([]domain.ConfigDraft, error) {
	if !scope.Valid() {
		return nil, fmt.Errorf("config draft query requires valid scope")
	}
	if status != "" && !status.Valid() {
		return nil, fmt.Errorf("config draft query has invalid status %q", status)
	}
	result := make([]domain.ConfigDraft, 0)
	for _, draft := range r.state.configDrafts {
		if draft.Scope != scope {
			continue
		}
		if status != "" && draft.Status != status {
			continue
		}
		result = append(result, cloneConfigDraft(draft))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID > result[j].ID
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result, nil
}
func (r reader) ConfigRevision(id domain.ConfigRevisionID) (domain.ConfigRevision, error) {
	v, ok := r.state.configRevisions[id]
	if !ok {
		return domain.ConfigRevision{}, notFound("config revision", id)
	}
	return cloneConfigRevision(v), nil
}
func (r reader) ActiveConfigRevision(scope domain.ConfigScope) (domain.ConfigRevision, error) {
	id, ok := r.state.activeConfig[scope]
	if !ok {
		return domain.ConfigRevision{}, notFound("active config revision", scope)
	}
	return r.ConfigRevision(id)
}
func (r reader) ConfigRevisions(scope domain.ConfigScope) ([]domain.ConfigRevision, error) {
	if !scope.Valid() {
		return nil, fmt.Errorf("config revision query requires valid scope")
	}
	result := make([]domain.ConfigRevision, 0)
	for _, revision := range r.state.configRevisions {
		if revision.Scope == scope {
			result = append(result, cloneConfigRevision(revision))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Revision == result[j].Revision {
			return result[i].ID < result[j].ID
		}
		return result[i].Revision < result[j].Revision
	})
	return result, nil
}
func (r reader) ConfigApplyReceipt(id domain.ConfigDraftID) (domain.ConfigApplyReceipt, error) {
	v, ok := r.state.configApplyReceipts[id]
	if !ok {
		return domain.ConfigApplyReceipt{}, notFound("config apply receipt", id)
	}
	return v, nil
}
func (r reader) OperationSpec(id domain.OperationSpecID) (domain.OperationSpec, error) {
	v, ok := r.state.operationSpecs[id]
	if !ok {
		return domain.OperationSpec{}, notFound("operation spec", id)
	}
	return cloneOperationSpec(v), nil
}
func (r reader) InquiryCandidate(id domain.InquiryCandidateID) (domain.InquiryCandidate, error) {
	v, ok := r.state.candidates[id]
	if !ok {
		return domain.InquiryCandidate{}, notFound("inquiry candidate", id)
	}
	return cloneCandidate(v), nil
}
func (r reader) Inquiry(id domain.InquiryID) (domain.Inquiry, error) {
	v, ok := r.state.inquiries[id]
	if !ok {
		return domain.Inquiry{}, notFound("inquiry", id)
	}
	return v, nil
}
func (r reader) Operation(id domain.OperationID) (domain.Operation, error) {
	v, ok := r.state.operations[id]
	if !ok {
		return domain.Operation{}, notFound("operation", id)
	}
	return cloneOperation(v), nil
}
func (r reader) Events(afterSequence uint64, limit int) ([]domain.Event, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("event limit must be positive")
	}
	if afterSequence >= uint64(len(r.state.events)) {
		return []domain.Event{}, nil
	}
	start := int(afterSequence)
	end := min(start+limit, len(r.state.events))
	return append([]domain.Event(nil), r.state.events[start:end]...), nil
}
func (r reader) EventByID(id domain.EventID) (domain.Event, error) {
	sequence, ok := r.state.eventIDs[id]
	if !ok {
		return domain.Event{}, notFound("event", id)
	}
	return r.state.events[sequence-1], nil
}

func peerSyncInboxKey(peerID, originID, messageID string) string {
	return strings.TrimSpace(peerID) + "\x00" + strings.TrimSpace(originID) + "\x00" + strings.TrimSpace(messageID)
}

func peerSyncCursorKey(peerID, originID, streamID string, direction domain.PeerSyncCursorDirection) string {
	return strings.TrimSpace(peerID) + "\x00" + strings.TrimSpace(originID) + "\x00" + strings.TrimSpace(streamID) + "\x00" + string(direction)
}

func (r reader) PeerSyncInboxRecord(peerID, originID, messageID string) (domain.PeerSyncInboxRecord, error) {
	key := peerSyncInboxKey(peerID, originID, messageID)
	if strings.TrimSpace(peerID) == "" || strings.TrimSpace(originID) == "" || strings.TrimSpace(messageID) == "" {
		return domain.PeerSyncInboxRecord{}, fmt.Errorf("peer sync inbox lookup requires identity")
	}
	v, ok := r.state.peerSyncInbox[key]
	if !ok {
		return domain.PeerSyncInboxRecord{}, notFound("peer sync inbox record", key)
	}
	return clonePeerSyncInboxRecord(v), nil
}

func (r reader) PendingPeerSyncInboxRecords(peerID string, limit int) ([]domain.PeerSyncInboxRecord, error) {
	if strings.TrimSpace(peerID) == "" {
		return nil, fmt.Errorf("pending peer sync inbox lookup requires peer identity")
	}
	var records []domain.PeerSyncInboxRecord
	for _, v := range r.state.peerSyncInbox {
		if v.PeerID == peerID {
			records = append(records, clonePeerSyncInboxRecord(v))
		}
	}
	// Sort by ReceivedAt to ensure deterministic reconciliation order.
	// We want oldest events first so they are processed in order of receipt.
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].ReceivedAt.Before(records[j].ReceivedAt)
	})
	if limit > 0 && len(records) > limit {
		records = records[:limit]
	}
	return records, nil
}

func (r reader) PeerSyncCursor(peerID, originID, streamID string, direction domain.PeerSyncCursorDirection) (domain.PeerSyncCursor, error) {
	key := peerSyncCursorKey(peerID, originID, streamID, direction)
	if strings.TrimSpace(peerID) == "" || strings.TrimSpace(originID) == "" || strings.TrimSpace(streamID) == "" {
		return domain.PeerSyncCursor{}, fmt.Errorf("peer sync cursor requires scope")
	}
	v, ok := r.state.peerSyncCursors[key]
	if !ok {
		return domain.PeerSyncCursor{}, notFound("peer sync cursor", key)
	}
	return v, nil
}
func (r reader) IdempotencyRecord(key domain.IdempotencyKey) (domain.IdempotencyRecord, error) {
	v, ok := r.state.idempotency[key]
	if !ok {
		return domain.IdempotencyRecord{}, notFound("idempotency key", key)
	}
	return v, nil
}
func (r reader) Source(id domain.SourceID) (domain.Source, error) {
	v, ok := r.state.sources[id]
	if !ok {
		return domain.Source{}, notFound("source", id)
	}
	return v, nil
}
func (r reader) Sources() ([]domain.Source, error) {
	out := make([]domain.Source, 0, len(r.state.sources))
	for _, source := range r.state.sources {
		out = append(out, source)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (r reader) SourceVersion(id domain.SourceVersionID) (domain.SourceVersion, error) {
	v, ok := r.state.sourceVersions[id]
	if !ok {
		return domain.SourceVersion{}, notFound("source version", id)
	}
	return v, nil
}
func (r reader) SourceVersions(sourceID domain.SourceID) ([]domain.SourceVersion, error) {
	if sourceID != "" {
		if _, ok := r.state.sources[sourceID]; !ok {
			return nil, notFound("source", sourceID)
		}
	}
	out := make([]domain.SourceVersion, 0)
	for _, version := range r.state.sourceVersions {
		if sourceID != "" && version.SourceID != sourceID {
			continue
		}
		out = append(out, version)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ObservedAt.Equal(out[j].ObservedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].ObservedAt.Before(out[j].ObservedAt)
	})
	return out, nil
}
func (r reader) SourceSnapshot(id domain.SourceVersionID) (domain.SourceSnapshot, error) {
	v, ok := r.state.sourceSnapshots[id]
	if !ok {
		return domain.SourceSnapshot{}, notFound("source snapshot", id)
	}
	return cloneSourceSnapshot(v), nil
}
func (r reader) SourceFragment(id domain.SourceFragmentID) (domain.SourceFragment, error) {
	v, ok := r.state.sourceFragments[id]
	if !ok {
		return domain.SourceFragment{}, notFound("source fragment", id)
	}
	return v, nil
}
func (r reader) SourceFragments(id domain.SourceVersionID) ([]domain.SourceFragment, error) {
	if _, ok := r.state.sourceVersions[id]; !ok {
		return nil, notFound("source version", id)
	}
	fragments := make([]domain.SourceFragment, 0)
	for _, fragment := range r.state.sourceFragments {
		if fragment.SourceVersionID == id {
			fragments = append(fragments, fragment)
		}
	}
	sort.Slice(fragments, func(i, j int) bool {
		if fragments[i].StartOffset == fragments[j].StartOffset {
			return fragments[i].ID < fragments[j].ID
		}
		return fragments[i].StartOffset < fragments[j].StartOffset
	})
	return fragments, nil
}
func (r reader) Observation(id domain.ObservationID) (domain.Observation, error) {
	v, ok := r.state.observations[id]
	if !ok {
		return domain.Observation{}, notFound("observation", id)
	}
	return v, nil
}
func (r reader) Observations() ([]domain.Observation, error) {
	out := make([]domain.Observation, 0, len(r.state.observations))
	for _, observation := range r.state.observations {
		out = append(out, observation)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (r reader) Claim(id domain.ClaimID) (domain.Claim, error) {
	v, ok := r.state.claims[id]
	if !ok {
		return domain.Claim{}, notFound("claim", id)
	}
	return cloneClaim(v), nil
}
func (r reader) Claims() ([]domain.Claim, error) {
	out := make([]domain.Claim, 0, len(r.state.claims))
	for _, claim := range r.state.claims {
		out = append(out, cloneClaim(claim))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (r reader) EvidenceLink(id domain.EvidenceLinkID) (domain.EvidenceLink, error) {
	v, ok := r.state.evidenceLinks[id]
	if !ok {
		return domain.EvidenceLink{}, notFound("evidence link", id)
	}
	return v, nil
}
func (r reader) EvidenceLinksForClaim(id domain.ClaimID) ([]domain.EvidenceLink, error) {
	if _, ok := r.state.claims[id]; !ok {
		return nil, notFound("claim", id)
	}
	links := make([]domain.EvidenceLink, 0)
	for _, link := range r.state.evidenceLinks {
		if link.ClaimID == id {
			links = append(links, link)
		}
	}
	sort.Slice(links, func(i, j int) bool { return links[i].ID < links[j].ID })
	return links, nil
}
func (r reader) EvidenceLinks() ([]domain.EvidenceLink, error) {
	out := make([]domain.EvidenceLink, 0, len(r.state.evidenceLinks))
	for _, link := range r.state.evidenceLinks {
		out = append(out, link)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (r reader) KnowledgeArtifact(id domain.ArtifactID) (domain.KnowledgeArtifact, error) {
	v, ok := r.state.artifacts[id]
	if !ok {
		return domain.KnowledgeArtifact{}, notFound("knowledge artifact", id)
	}
	return cloneKnowledgeArtifact(v), nil
}
func (r reader) KnowledgeArtifacts() ([]domain.KnowledgeArtifact, error) {
	out := make([]domain.KnowledgeArtifact, 0, len(r.state.artifacts))
	for _, artifact := range r.state.artifacts {
		out = append(out, cloneKnowledgeArtifact(artifact))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (r reader) RawModelOutput(id domain.ArtifactID) (domain.RawModelOutput, error) {
	v, ok := r.state.rawModelOutputs[id]
	if !ok {
		return domain.RawModelOutput{}, notFound("raw model output", id)
	}
	return v, nil
}
func (r reader) ProposedChangeSet(id domain.ChangeSetID) (domain.ProposedChangeSet, error) {
	v, ok := r.state.proposedChanges[id]
	if !ok {
		return domain.ProposedChangeSet{}, notFound("proposed changeset", id)
	}
	return cloneProposedChangeSet(v), nil
}
func (r reader) AcceptedChangeSet(id domain.ChangeSetID) (domain.AcceptedChangeSet, error) {
	v, ok := r.state.acceptedChanges[id]
	if !ok {
		return domain.AcceptedChangeSet{}, notFound("accepted changeset", id)
	}
	return cloneAcceptedChangeSet(v), nil
}
func (r reader) ValidationReceipt(id domain.ReceiptID) (domain.ValidationReceipt, error) {
	v, ok := r.state.receipts[id]
	if !ok {
		return domain.ValidationReceipt{}, notFound("validation receipt", id)
	}
	return v, nil
}
func (r reader) CommitReceipt(id domain.ReceiptID) (domain.CommitReceipt, error) {
	v, ok := r.state.commitReceipts[id]
	if !ok {
		return domain.CommitReceipt{}, notFound("commit receipt", id)
	}
	return v, nil
}
func (r reader) Commit(id domain.CommitID) (domain.Commit, error) {
	v, ok := r.state.commits[id]
	if !ok {
		return domain.Commit{}, notFound("commit", id)
	}
	return v, nil
}
func (r reader) Commits() ([]domain.Commit, error) {
	out := make([]domain.Commit, 0, len(r.state.commits))
	for _, commit := range r.state.commits {
		out = append(out, commit)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Version != out[j].Version {
			return out[i].Version < out[j].Version
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}
func (t transaction) Commits() ([]domain.Commit, error) {
	return reader(t).Commits()
}
func (r reader) CommitByIdempotencyKey(key domain.IdempotencyKey) (domain.Commit, error) {
	id, ok := r.state.commitByIntent[key]
	if !ok {
		return domain.Commit{}, notFound("commit idempotency key", key)
	}
	return r.Commit(id)
}
func (r reader) HeadCommit(id domain.MissionRevisionID) (domain.Commit, error) {
	commitID, ok := r.state.headCommits[id]
	if !ok {
		return domain.Commit{}, notFound("head commit", id)
	}
	return r.Commit(commitID)
}
func (r reader) CanonicalEntity(entityType, entityID string) (domain.CanonicalEntity, error) {
	v, ok := r.state.canonical[canonicalKey(entityType, entityID)]
	if !ok {
		return domain.CanonicalEntity{}, notFound("canonical entity", entityType+"/"+entityID)
	}
	return v, nil
}

func (t transaction) AppendMissionRevision(v domain.MissionRevision) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate mission revision: %w", err)
	}
	if _, exists := t.state.missionRevisions[v.ID]; exists {
		return conflict("mission revision", v.ID)
	}
	for _, existing := range t.state.missionRevisions {
		if existing.MissionID == v.MissionID && existing.Revision == v.Revision {
			return conflict("mission revision number", v.Revision)
		}
	}
	t.state.missionRevisions[v.ID] = cloneMission(v)
	return nil
}
func (t transaction) ActivateMissionRevision(missionID domain.MissionID, revisionID domain.MissionRevisionID) error {
	v, ok := t.state.missionRevisions[revisionID]
	if !ok {
		return notFound("mission revision", revisionID)
	}
	if v.MissionID != missionID {
		return fmt.Errorf("%w: revision %s belongs to mission %s", port.ErrConflict, revisionID, v.MissionID)
	}
	t.state.activeMissions[missionID] = revisionID
	return nil
}
func (t transaction) CreateQuestion(v domain.Question) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate question: %w", err)
	}
	if _, ok := t.state.questions[v.ID]; ok {
		return conflict("question", v.ID)
	}
	if _, ok := t.state.missionRevisions[v.MissionRevision]; !ok {
		return notFound("mission revision", v.MissionRevision)
	}
	t.state.questions[v.ID] = v
	return nil
}

func (t transaction) CreateOperatorQuestion(v domain.OperatorQuestion) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate operator question: %w", err)
	}
	if v.Status != domain.OperatorQuestionPending || v.Revision != 1 {
		return fmt.Errorf("%w: operator question must be created pending at revision 1", port.ErrConflict)
	}
	if _, ok := t.state.missionRevisions[v.MissionRevision]; !ok {
		return notFound("mission revision", v.MissionRevision)
	}
	if _, exists := t.state.operatorQuestions[v.ID]; exists {
		return conflict("operator question", v.ID)
	}
	for _, existing := range t.state.operatorQuestions {
		if existing.MissionID == v.MissionID && !existing.Status.Terminal() && existing.DedupSignature == v.DedupSignature {
			return fmt.Errorf("%w: pending operator question duplicates semantic signature", port.ErrConflict)
		}
	}
	t.state.operatorQuestions[v.ID] = cloneOperatorQuestion(v)
	return nil
}

func (t transaction) SaveOperatorQuestion(v domain.OperatorQuestion, expectedRevision uint64) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate operator question: %w", err)
	}
	current, ok := t.state.operatorQuestions[v.ID]
	if !ok {
		return notFound("operator question", v.ID)
	}
	if current.Revision != expectedRevision || v.Revision != expectedRevision+1 {
		return fmt.Errorf("%w: stale operator question revision", port.ErrConflict)
	}
	if current.MissionID != v.MissionID || current.MissionRevision != v.MissionRevision || current.CreatedAt != v.CreatedAt || current.Kind != v.Kind || current.DedupSignature != v.DedupSignature {
		return fmt.Errorf("%w: immutable operator question fields changed", port.ErrConflict)
	}
	t.state.operatorQuestions[v.ID] = cloneOperatorQuestion(v)
	return nil
}

func (t transaction) AcceptUserAnswer(v domain.UserAnswer, answered domain.OperatorQuestion, expectedRevision uint64) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate user answer: %w", err)
	}
	question, ok := t.state.operatorQuestions[v.QuestionID]
	if !ok {
		return notFound("operator question", v.QuestionID)
	}
	if err := v.ValidateForQuestion(question); err != nil {
		return fmt.Errorf("%w: correlate user answer: %v", port.ErrConflict, err)
	}
	if _, exists := t.state.operatorAnswers[v.ID]; exists {
		return conflict("user answer", v.ID)
	}
	key := transportAnswerKey(v.Channel, v.TransportEventID)
	if _, exists := t.state.answerByTransport[key]; exists {
		return fmt.Errorf("%w: transport event already accepted", port.ErrConflict)
	}
	if answered.ID != question.ID || answered.Status != domain.OperatorQuestionAnswered || answered.AnswerID != v.ID || !answered.AnsweredAt.Equal(v.ReceivedAt) {
		return fmt.Errorf("%w: answered question does not match user answer", port.ErrConflict)
	}
	if err := t.SaveOperatorQuestion(answered, expectedRevision); err != nil {
		return err
	}
	t.state.operatorAnswers[v.ID] = cloneUserAnswer(v)
	t.state.answerByTransport[key] = v.ID
	return nil
}

func (t transaction) CreateQuestionDelivery(v domain.QuestionDelivery) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate question delivery: %w", err)
	}
	if v.Status != domain.QuestionDeliveryPending || v.Attempt != 0 {
		return fmt.Errorf("%w: question delivery must be created pending before attempts", port.ErrConflict)
	}
	question, ok := t.state.operatorQuestions[v.QuestionID]
	if !ok {
		return notFound("operator question", v.QuestionID)
	}
	if question.Revision != v.QuestionRevision || question.Status.Terminal() {
		return fmt.Errorf("%w: delivery references stale or terminal operator question", port.ErrConflict)
	}
	if _, exists := t.state.questionDeliveries[v.ID]; exists {
		return conflict("question delivery", v.ID)
	}
	key := questionDeliveryRouteKey(v.QuestionID, v.QuestionRevision, v.Channel, v.DestinationRef)
	if _, exists := t.state.deliveryByRoute[key]; exists {
		return fmt.Errorf("%w: question delivery route already exists", port.ErrConflict)
	}
	t.state.questionDeliveries[v.ID] = v
	t.state.deliveryByRoute[key] = v.ID
	return nil
}

func (t transaction) SaveQuestionDelivery(v domain.QuestionDelivery, expectedStatus domain.QuestionDeliveryStatus, expectedAttempt uint32) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate question delivery: %w", err)
	}
	current, ok := t.state.questionDeliveries[v.ID]
	if !ok {
		return notFound("question delivery", v.ID)
	}
	if current.Status != expectedStatus || current.Attempt != expectedAttempt {
		return fmt.Errorf("%w: stale question delivery state", port.ErrConflict)
	}
	if current.QuestionID != v.QuestionID || current.QuestionRevision != v.QuestionRevision || current.Channel != v.Channel || current.DestinationRef != v.DestinationRef || current.CreatedAt != v.CreatedAt || current.MaxAttempts != v.MaxAttempts {
		return fmt.Errorf("%w: immutable question delivery fields changed", port.ErrConflict)
	}
	if v.Status == domain.QuestionDeliveryLeased {
		question, ok := t.state.operatorQuestions[v.QuestionID]
		if !ok {
			return notFound("operator question", v.QuestionID)
		}
		if question.Revision != v.QuestionRevision || question.Status.Terminal() {
			return fmt.Errorf("%w: cannot lease delivery for stale or terminal operator question", port.ErrConflict)
		}
	}
	if current.TransportMessageID != "" && current.TransportMessageID != v.TransportMessageID {
		delete(t.state.deliveryByTransport, deliveryTransportKey(current.Channel, current.TransportMessageID))
	}
	if v.TransportMessageID != "" {
		key := deliveryTransportKey(v.Channel, v.TransportMessageID)
		if owner, exists := t.state.deliveryByTransport[key]; exists && owner != v.ID {
			return fmt.Errorf("%w: question delivery transport already bound", port.ErrConflict)
		}
		t.state.deliveryByTransport[key] = v.ID
	}
	t.state.questionDeliveries[v.ID] = v
	return nil
}

func (t transaction) CreateQuestionGateDecision(v domain.QuestionGateDecisionRecord) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate question gate decision: %w", err)
	}
	if _, exists := t.state.questionGateDecisions[v.ID]; exists {
		return conflict("question gate decision", v.ID)
	}
	if _, exists := t.state.gateDecisionByQuestion[v.QuestionID]; exists {
		return fmt.Errorf("%w: question already has a gate decision", port.ErrConflict)
	}
	t.state.questionGateDecisions[v.ID] = v
	t.state.gateDecisionByQuestion[v.QuestionID] = v.ID
	return nil
}

func (t transaction) CreateOperatorCommand(command domain.OperatorCommand, receipt domain.CommandReceipt) error {
	if err := command.Validate(); err != nil {
		return fmt.Errorf("validate operator command: %w", err)
	}
	if err := receipt.Validate(); err != nil {
		return fmt.Errorf("validate command receipt: %w", err)
	}
	if receipt.CommandID != command.ID || receipt.State != domain.CommandReceived {
		return fmt.Errorf("%w: command must be created with RECEIVED receipt", port.ErrConflict)
	}
	if existing, ok := t.state.operatorCommands[command.ID]; ok {
		if equalOperatorCommands(existing, command) {
			currentReceipt := t.state.operatorCommandReceipts[command.ID]
			if currentReceipt == receipt {
				return nil
			}
			return fmt.Errorf("%w: operator command receipt diverges on replay", port.ErrConflict)
		}
		return conflict("operator command", command.ID)
	}
	if existingID, ok := t.state.operatorCommandByIdem[command.IdempotencyKey]; ok {
		existing := t.state.operatorCommands[existingID]
		if equalOperatorCommands(existing, command) {
			return nil
		}
		return fmt.Errorf("%w: operator command idempotency key reused with different content", port.ErrConflict)
	}
	if _, ok := t.state.operatorCommandReceipts[command.ID]; ok {
		return conflict("operator command receipt", command.ID)
	}
	t.state.operatorCommands[command.ID] = cloneOperatorCommand(command)
	t.state.operatorCommandByIdem[command.IdempotencyKey] = command.ID
	t.state.operatorCommandReceipts[command.ID] = receipt
	return nil
}

func (t transaction) SaveOperatorCommandReceipt(receipt domain.CommandReceipt) error {
	if err := receipt.Validate(); err != nil {
		return fmt.Errorf("validate command receipt: %w", err)
	}
	if _, ok := t.state.operatorCommands[receipt.CommandID]; !ok {
		return notFound("operator command", receipt.CommandID)
	}
	current, ok := t.state.operatorCommandReceipts[receipt.CommandID]
	if !ok {
		return notFound("operator command receipt", receipt.CommandID)
	}
	if err := domain.AdvanceCommandReceipt(current, receipt); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return fmt.Errorf("%w: %v", port.ErrConflict, err)
		}
		return err
	}
	t.state.operatorCommandReceipts[receipt.CommandID] = receipt
	return nil
}

func (t transaction) CreateExternalEvent(event domain.ExternalEvent, disposition domain.ExternalEventDisposition) error {
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validate external event: %w", err)
	}
	if err := disposition.Validate(); err != nil {
		return fmt.Errorf("validate external event disposition: %w", err)
	}
	if disposition.EventID != event.ID || disposition.State != domain.ExternalEventReceived {
		return fmt.Errorf("%w: external event must be created with RECEIVED disposition", port.ErrConflict)
	}
	if existing, ok := t.state.externalEvents[event.ID]; ok {
		if equalExternalEvents(existing, event) {
			current := t.state.externalEventDispositions[event.ID]
			if current == disposition {
				return nil
			}
			return fmt.Errorf("%w: external event disposition diverges on replay", port.ErrConflict)
		}
		return conflict("external event", event.ID)
	}
	if existingID, ok := t.state.externalEventByDedup[event.DeduplicationKey]; ok {
		existing := t.state.externalEvents[existingID]
		if equalExternalEvents(existing, event) {
			return nil
		}
		return fmt.Errorf("%w: external event deduplication key reused with different content", port.ErrConflict)
	}
	if _, ok := t.state.externalEventDispositions[event.ID]; ok {
		return conflict("external event disposition", event.ID)
	}
	t.state.externalEvents[event.ID] = cloneExternalEvent(event)
	t.state.externalEventByDedup[event.DeduplicationKey] = event.ID
	t.state.externalEventDispositions[event.ID] = disposition
	return nil
}

func (t transaction) SaveExternalEventDisposition(disposition domain.ExternalEventDisposition) error {
	if err := disposition.Validate(); err != nil {
		return fmt.Errorf("validate external event disposition: %w", err)
	}
	if _, ok := t.state.externalEvents[disposition.EventID]; !ok {
		return notFound("external event", disposition.EventID)
	}
	current, ok := t.state.externalEventDispositions[disposition.EventID]
	if !ok {
		return notFound("external event disposition", disposition.EventID)
	}
	if err := domain.AdvanceExternalEventDisposition(current, disposition); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return fmt.Errorf("%w: %v", port.ErrConflict, err)
		}
		return err
	}
	t.state.externalEventDispositions[disposition.EventID] = disposition
	return nil
}

func (t transaction) CreateWorkOpportunity(v domain.WorkOpportunity) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate work opportunity: %w", err)
	}
	if v.Status != domain.OpportunityOpen && v.Status != domain.OpportunityDeferred {
		return fmt.Errorf("%w: work opportunity must be created open or deferred", port.ErrConflict)
	}
	if _, ok := t.state.missionRevisions[v.MissionRevision]; !ok {
		return notFound("mission revision", v.MissionRevision)
	}
	if v.ParentID != "" {
		parent, ok := t.state.workOpportunities[v.ParentID]
		if !ok {
			return notFound("parent work opportunity", v.ParentID)
		}
		if parent.MissionRevision != v.MissionRevision {
			return fmt.Errorf("%w: child work opportunity mission revision diverges", port.ErrConflict)
		}
		if v.Depth != parent.Depth+1 {
			return fmt.Errorf("%w: child work opportunity depth must be parent+1", port.ErrConflict)
		}
	}
	if _, exists := t.state.workOpportunities[v.ID]; exists {
		return conflict("work opportunity", v.ID)
	}
	for _, existing := range t.state.workOpportunities {
		if existing.MissionRevision == v.MissionRevision && existing.Status.Active() && existing.DedupSignature == v.DedupSignature {
			return fmt.Errorf("%w: active work opportunity duplicates semantic signature", port.ErrConflict)
		}
	}
	t.state.workOpportunities[v.ID] = cloneWorkOpportunity(v)
	return nil
}

func (t transaction) SaveWorkOpportunity(v domain.WorkOpportunity) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate work opportunity: %w", err)
	}
	current, ok := t.state.workOpportunities[v.ID]
	if !ok {
		return notFound("work opportunity", v.ID)
	}
	if current.MissionRevision != v.MissionRevision || current.Family != v.Family || current.DedupSignature != v.DedupSignature || current.ParentID != v.ParentID || current.Depth != v.Depth || !current.CreatedAt.Equal(v.CreatedAt) {
		return fmt.Errorf("%w: immutable work opportunity fields changed", port.ErrConflict)
	}
	if v.UpdatedAt.Before(current.UpdatedAt) {
		return fmt.Errorf("%w: work opportunity update time must not go backwards", port.ErrConflict)
	}
	for _, existing := range t.state.workOpportunities {
		if existing.ID == v.ID || existing.MissionRevision != v.MissionRevision || !existing.Status.Active() || !v.Status.Active() {
			continue
		}
		if existing.DedupSignature == v.DedupSignature {
			return fmt.Errorf("%w: active work opportunity duplicates semantic signature", port.ErrConflict)
		}
	}
	t.state.workOpportunities[v.ID] = cloneWorkOpportunity(v)
	return nil
}

func (t transaction) CreateContinuityDiagnosis(v domain.ContinuityDiagnosis) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate continuity diagnosis: %w", err)
	}
	if _, ok := t.state.missionRevisions[v.MissionRevision]; !ok {
		return notFound("mission revision", v.MissionRevision)
	}
	if _, exists := t.state.continuityDiagnoses[v.ID]; exists {
		return conflict("continuity diagnosis", v.ID)
	}
	t.state.continuityDiagnoses[v.ID] = cloneContinuityDiagnosis(v)
	return nil
}

func (t transaction) CreateConfigDraft(v domain.ConfigDraft) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate config draft: %w", err)
	}
	if v.Status != domain.ConfigDraftOpen {
		return fmt.Errorf("%w: config draft must be created OPEN", port.ErrConflict)
	}
	if _, exists := t.state.configDrafts[v.ID]; exists {
		return conflict("config draft", v.ID)
	}
	t.state.configDrafts[v.ID] = cloneConfigDraft(v)
	return nil
}

func (t transaction) SaveConfigDraft(v domain.ConfigDraft) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate config draft: %w", err)
	}
	current, ok := t.state.configDrafts[v.ID]
	if !ok {
		return notFound("config draft", v.ID)
	}
	if current.Scope != v.Scope || current.BasedOnRevision != v.BasedOnRevision || current.Applicability != v.Applicability || current.ActorType != v.ActorType || current.ActorID != v.ActorID || current.Reason != v.Reason || !current.CreatedAt.Equal(v.CreatedAt) {
		return fmt.Errorf("%w: immutable config draft fields changed", port.ErrConflict)
	}
	if !equalConfigPayloads(current, v) {
		return fmt.Errorf("%w: config draft payload is immutable", port.ErrConflict)
	}
	if current.Status.Terminal() && current.Status != v.Status {
		return fmt.Errorf("%w: terminal config draft cannot change status", port.ErrConflict)
	}
	if current.Status == v.Status {
		if current.ValidatedAt.Equal(v.ValidatedAt) {
			return nil
		}
		return fmt.Errorf("%w: config draft status unchanged with different validation time", port.ErrConflict)
	}
	switch {
	case current.Status == domain.ConfigDraftOpen && (v.Status == domain.ConfigDraftValidated || v.Status == domain.ConfigDraftRejected):
	case current.Status == domain.ConfigDraftValidated && (v.Status == domain.ConfigDraftApplied || v.Status == domain.ConfigDraftRejected):
	default:
		return fmt.Errorf("%w: illegal config draft status transition %s → %s", port.ErrConflict, current.Status, v.Status)
	}
	t.state.configDrafts[v.ID] = cloneConfigDraft(v)
	return nil
}

func (t transaction) AppendConfigRevision(v domain.ConfigRevision) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate config revision: %w", err)
	}
	if _, exists := t.state.configRevisions[v.ID]; exists {
		return conflict("config revision", v.ID)
	}
	var max uint64
	for _, existing := range t.state.configRevisions {
		if existing.Scope != v.Scope {
			continue
		}
		if existing.Revision > max {
			max = existing.Revision
		}
		if existing.Revision == v.Revision {
			return fmt.Errorf("%w: config revision number already used for scope", port.ErrConflict)
		}
	}
	if v.Revision != max+1 {
		return fmt.Errorf("%w: config revision number must be sequential", port.ErrConflict)
	}
	if v.ParentID != "" {
		parent, ok := t.state.configRevisions[v.ParentID]
		if !ok {
			return notFound("parent config revision", v.ParentID)
		}
		if parent.Scope != v.Scope || parent.Revision+1 != v.Revision {
			return fmt.Errorf("%w: config revision parent lineage is invalid", port.ErrConflict)
		}
	} else if v.Revision != 1 {
		return fmt.Errorf("%w: first config revision must omit parent", port.ErrConflict)
	}
	draft, ok := t.state.configDrafts[v.DraftID]
	if !ok {
		return notFound("config draft", v.DraftID)
	}
	if draft.Scope != v.Scope {
		return fmt.Errorf("%w: config draft scope disagrees with revision", port.ErrConflict)
	}
	t.state.configRevisions[v.ID] = cloneConfigRevision(v)
	return nil
}

func (t transaction) ActivateConfigRevision(scope domain.ConfigScope, id domain.ConfigRevisionID) error {
	if !scope.Valid() {
		return fmt.Errorf("activate config requires valid scope")
	}
	revision, ok := t.state.configRevisions[id]
	if !ok {
		return notFound("config revision", id)
	}
	if revision.Scope != scope {
		return fmt.Errorf("%w: config revision scope disagrees with activation", port.ErrConflict)
	}
	if current, ok := t.state.activeConfig[scope]; ok {
		if current == id {
			return nil
		}
		existing := t.state.configRevisions[current]
		if revision.Revision <= existing.Revision {
			return fmt.Errorf("%w: active config can only move forward", port.ErrConflict)
		}
	}
	t.state.activeConfig[scope] = id
	return nil
}

func (t transaction) SaveConfigApplyReceipt(receipt domain.ConfigApplyReceipt) error {
	if err := receipt.Validate(); err != nil {
		return fmt.Errorf("validate config apply receipt: %w", err)
	}
	if _, ok := t.state.configDrafts[receipt.DraftID]; !ok {
		return notFound("config draft", receipt.DraftID)
	}
	current, ok := t.state.configApplyReceipts[receipt.DraftID]
	if !ok {
		if receipt.State != domain.ConfigApplyReceived {
			return fmt.Errorf("%w: config apply receipt must start RECEIVED", port.ErrConflict)
		}
		t.state.configApplyReceipts[receipt.DraftID] = receipt
		return nil
	}
	if err := domain.AdvanceConfigApplyReceipt(current, receipt); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return fmt.Errorf("%w: %v", port.ErrConflict, err)
		}
		return err
	}
	t.state.configApplyReceipts[receipt.DraftID] = receipt
	return nil
}

func (t transaction) SaveControlState(next domain.ControlState, expectedRevision uint64) error {
	if err := next.Validate(); err != nil {
		return fmt.Errorf("validate control state: %w", err)
	}
	if !t.state.hasControlState {
		if expectedRevision != 0 || next.Revision != 0 {
			return fmt.Errorf("%w: initial control state must start at revision 0", port.ErrConflict)
		}
		t.state.controlState = cloneControlState(next)
		t.state.hasControlState = true
		return nil
	}
	current := t.state.controlState
	if current.Revision != expectedRevision {
		return fmt.Errorf("%w: stale control state revision", port.ErrConflict)
	}
	if next.Revision != expectedRevision+1 {
		return fmt.Errorf("%w: control state revision must advance by one", port.ErrConflict)
	}
	if next.SchemaVersion != current.SchemaVersion {
		return fmt.Errorf("%w: control state schema is immutable", port.ErrConflict)
	}
	t.state.controlState = cloneControlState(next)
	t.state.hasControlState = true
	return nil
}

func (t transaction) SaveChannelCursor(next domain.ChannelCursor, expectedRevision uint64) error {
	if err := next.Validate(); err != nil {
		return fmt.Errorf("validate channel cursor: %w", err)
	}
	key := strings.TrimSpace(next.Channel)
	next.Channel = key
	current, ok := t.state.channelCursors[key]
	if !ok {
		if expectedRevision != 0 {
			return fmt.Errorf("%w: expected existing channel cursor revision %d", port.ErrConflict, expectedRevision)
		}
		if next.Revision != 0 {
			return fmt.Errorf("%w: initial channel cursor must start at revision 0", port.ErrConflict)
		}
		t.state.channelCursors[key] = next
		return nil
	}
	if current.Revision != expectedRevision {
		return fmt.Errorf("%w: stale channel cursor revision", port.ErrConflict)
	}
	if next.Revision < current.Revision {
		return fmt.Errorf("%w: channel cursor revision must not decrease", port.ErrConflict)
	}
	if next.Revision == current.Revision {
		// Pure replay of identical position is allowed without mutation.
		if next.Cursor != current.Cursor || next.SchemaVersion != current.SchemaVersion {
			return fmt.Errorf("%w: channel cursor content changed without revision advance", port.ErrConflict)
		}
		return nil
	}
	if next.Revision != expectedRevision+1 {
		return fmt.Errorf("%w: channel cursor revision must advance by one", port.ErrConflict)
	}
	if next.SchemaVersion != current.SchemaVersion {
		return fmt.Errorf("%w: channel cursor schema is immutable", port.ErrConflict)
	}
	if next.Cursor < current.Cursor {
		return fmt.Errorf("%w: channel cursor must not decrease", port.ErrConflict)
	}
	t.state.channelCursors[key] = next
	return nil
}

func (t transaction) PutPeerSyncInboxRecord(next domain.PeerSyncInboxRecord) (domain.PeerSyncInboxRecord, bool, error) {
	if err := next.Validate(); err != nil {
		return domain.PeerSyncInboxRecord{}, false, fmt.Errorf("validate peer sync inbox record: %w", err)
	}
	key := peerSyncInboxKey(next.PeerID, next.OriginID, next.MessageID)
	if current, ok := t.state.peerSyncInbox[key]; ok {
		if !peerSyncInboxRecordsEqual(current, next) {
			return domain.PeerSyncInboxRecord{}, false, fmt.Errorf("%w: peer sync message id reused with divergent content", port.ErrConflict)
		}
		return clonePeerSyncInboxRecord(current), false, nil
	}
	t.state.peerSyncInbox[key] = clonePeerSyncInboxRecord(next)
	return clonePeerSyncInboxRecord(next), true, nil
}

func (t transaction) SavePeerSyncCursor(next domain.PeerSyncCursor, expectedRevision uint64) error {
	if err := next.Validate(); err != nil {
		return fmt.Errorf("validate peer sync cursor: %w", err)
	}
	key := peerSyncCursorKey(next.PeerID, next.OriginID, next.StreamID, next.Direction)
	current, ok := t.state.peerSyncCursors[key]
	if !ok {
		if expectedRevision != 0 || next.Revision != 0 {
			return fmt.Errorf("%w: initial peer sync cursor must start at revision 0", port.ErrConflict)
		}
		t.state.peerSyncCursors[key] = next
		return nil
	}
	if current.Revision != expectedRevision {
		return fmt.Errorf("%w: stale peer sync cursor revision", port.ErrConflict)
	}
	if next.NextSequence == current.NextSequence && next.Revision == current.Revision {
		return nil
	}
	if next.Revision != current.Revision+1 || next.NextSequence < current.NextSequence || next.SchemaVersion != current.SchemaVersion {
		return fmt.Errorf("%w: invalid peer sync cursor advance", port.ErrConflict)
	}
	t.state.peerSyncCursors[key] = next
	return nil
}

func (t transaction) SaveResourceUsage(next domain.ResourceUsage) error {
	if err := next.Validate(); err != nil {
		return fmt.Errorf("validate resource usage: %w", err)
	}
	key := domain.ResourceID(strings.TrimSpace(string(next.Resource)))
	if key == "" {
		return fmt.Errorf("resource usage requires resource id")
	}
	next.Resource = key
	t.state.resourceUsages[key] = cloneResourceUsage(next)
	return nil
}

func (t transaction) SaveModelContextPressure(next domain.ModelContextPressure) error {
	if err := next.Validate(); err != nil {
		return fmt.Errorf("validate model context pressure: %w", err)
	}
	next.BindingID = strings.TrimSpace(next.BindingID)
	next.UpdatedAt = next.UpdatedAt.UTC()
	t.state.modelContextPressures[next.BindingID] = next
	return nil
}

func (t transaction) AppendOperationSpec(v domain.OperationSpec) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate operation spec: %w", err)
	}
	if _, ok := t.state.operationSpecs[v.ID]; ok {
		return conflict("operation spec", v.ID)
	}
	t.state.operationSpecs[v.ID] = cloneOperationSpec(v)
	return nil
}
func (t transaction) CreateInquiryCandidate(v domain.InquiryCandidate) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate inquiry candidate: %w", err)
	}
	if _, ok := t.state.candidates[v.ID]; ok {
		return conflict("inquiry candidate", v.ID)
	}
	question, ok := t.state.questions[v.QuestionID]
	if !ok {
		return notFound("question", v.QuestionID)
	}
	if question.MissionRevision != v.MissionRevision {
		return fmt.Errorf("%w: candidate and question mission revisions differ", port.ErrConflict)
	}
	t.state.candidates[v.ID] = cloneCandidate(v)
	return nil
}
func (t transaction) CreateInquiry(v domain.Inquiry) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate inquiry: %w", err)
	}
	if _, ok := t.state.inquiries[v.ID]; ok {
		return conflict("inquiry", v.ID)
	}
	candidate, ok := t.state.candidates[v.CandidateID]
	if !ok {
		return notFound("inquiry candidate", v.CandidateID)
	}
	if candidate.QuestionID != v.QuestionID || candidate.MissionRevision != v.MissionRevision {
		return fmt.Errorf("%w: inquiry lineage differs from candidate", port.ErrConflict)
	}
	t.state.inquiries[v.ID] = v
	return nil
}
func (t transaction) CreateOperation(v domain.Operation) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate operation: %w", err)
	}
	if _, ok := t.state.operations[v.ID]; ok {
		return conflict("operation", v.ID)
	}
	inquiry, ok := t.state.inquiries[v.InquiryID]
	if !ok {
		return notFound("inquiry", v.InquiryID)
	}
	if inquiry.MissionRevision != v.MissionRevision {
		return fmt.Errorf("%w: operation and inquiry mission revisions differ", port.ErrConflict)
	}
	if _, ok := t.state.operationSpecs[v.SpecID]; !ok {
		return notFound("operation spec", v.SpecID)
	}
	t.state.operations[v.ID] = cloneOperation(v)
	return nil
}
func (t transaction) SaveInquiry(v domain.Inquiry) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate inquiry: %w", err)
	}
	existing, ok := t.state.inquiries[v.ID]
	if !ok {
		return notFound("inquiry", v.ID)
	}
	if existing.SchemaVersion != v.SchemaVersion || existing.CandidateID != v.CandidateID || existing.MissionRevision != v.MissionRevision || existing.QuestionID != v.QuestionID || existing.AdmissionReason != v.AdmissionReason || existing.Budget != v.Budget || existing.StopCondition != v.StopCondition {
		return fmt.Errorf("%w: immutable inquiry fields changed", port.ErrConflict)
	}
	t.state.inquiries[v.ID] = v
	return nil
}
func (t transaction) SaveOperation(v domain.Operation) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate operation: %w", err)
	}
	existing, ok := t.state.operations[v.ID]
	if !ok {
		return notFound("operation", v.ID)
	}
	if existing.SchemaVersion != v.SchemaVersion || existing.InquiryID != v.InquiryID || existing.MissionRevision != v.MissionRevision || existing.SpecID != v.SpecID || !equalStrings(existing.ReadSet, v.ReadSet) || !equalStrings(existing.InputRefs, v.InputRefs) || existing.ExpectedOutput != v.ExpectedOutput || existing.IdempotencyKey != v.IdempotencyKey {
		return fmt.Errorf("%w: immutable operation fields changed", port.ErrConflict)
	}
	t.state.operations[v.ID] = cloneOperation(v)
	return nil
}
func (t transaction) AppendEvent(v domain.Event) (domain.Event, error) {
	if err := v.ValidateForAppend(); err != nil {
		return domain.Event{}, fmt.Errorf("validate event: %w", err)
	}
	if _, exists := t.state.eventIDs[v.ID]; exists {
		return domain.Event{}, conflict("event", v.ID)
	}
	v.Sequence = uint64(len(t.state.events) + 1)
	t.state.events = append(t.state.events, v)
	t.state.eventIDs[v.ID] = v.Sequence
	return v, nil
}
func (t transaction) ReserveIdempotency(v domain.IdempotencyRecord) (domain.IdempotencyRecord, error) {
	if err := v.Validate(); err != nil {
		return domain.IdempotencyRecord{}, fmt.Errorf("validate idempotency reservation: %w", err)
	}
	if v.Status != domain.IdempotencyReserved {
		return domain.IdempotencyRecord{}, fmt.Errorf("idempotency reservation must have status %s", domain.IdempotencyReserved)
	}
	if existing, ok := t.state.idempotency[v.Key]; ok {
		if existing.OperationID == v.OperationID && existing.Intent == v.Intent {
			return existing, nil
		}
		return domain.IdempotencyRecord{}, conflict("idempotency key", v.Key)
	}
	t.state.idempotency[v.Key] = v
	return v, nil
}
func (t transaction) CompleteIdempotency(key domain.IdempotencyKey, receiptID domain.ReceiptID, resultRef string, completedAt time.Time) (domain.IdempotencyRecord, error) {
	v, ok := t.state.idempotency[key]
	if !ok {
		return domain.IdempotencyRecord{}, notFound("idempotency key", key)
	}
	if v.Status == domain.IdempotencyCompleted {
		if v.ReceiptID == receiptID && v.ResultRef == resultRef && v.CompletedAt.Equal(completedAt) {
			return v, nil
		}
		return domain.IdempotencyRecord{}, conflict("idempotency completion", key)
	}
	v.Status = domain.IdempotencyCompleted
	v.ReceiptID = receiptID
	v.ResultRef = resultRef
	v.CompletedAt = completedAt
	if err := v.Validate(); err != nil {
		return domain.IdempotencyRecord{}, fmt.Errorf("validate idempotency completion: %w", err)
	}
	t.state.idempotency[key] = v
	return v, nil
}

func (t transaction) AppendSource(v domain.Source, version domain.SourceVersion, snapshot domain.SourceSnapshot) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate source: %w", err)
	}
	if err := version.Validate(); err != nil {
		return fmt.Errorf("validate source version: %w", err)
	}
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("validate source snapshot: %w", err)
	}
	if version.SourceID != v.ID || snapshot.SourceVersionID != version.ID {
		return fmt.Errorf("%w: source ingestion lineage differs", port.ErrConflict)
	}
	digest := sha256.Sum256(snapshot.Content)
	wantHash := "sha256:" + hex.EncodeToString(digest[:])
	if version.ContentHash != wantHash || version.ContentRef != wantHash {
		return fmt.Errorf("%w: source snapshot hash or content reference differs", port.ErrConflict)
	}
	if _, ok := t.state.sources[v.ID]; ok {
		return conflict("source", v.ID)
	}
	if _, ok := t.state.sourceVersions[version.ID]; ok {
		return conflict("source version", version.ID)
	}
	if _, ok := t.state.sourceSnapshots[version.ID]; ok {
		return conflict("source snapshot", version.ID)
	}
	t.state.sources[v.ID] = v
	t.state.sourceVersions[version.ID] = version
	t.state.sourceSnapshots[version.ID] = cloneSourceSnapshot(snapshot)
	return nil
}

func (t transaction) AppendSourceFragments(versionID domain.SourceVersionID, fragments []domain.SourceFragment) error {
	snapshot, ok := t.state.sourceSnapshots[versionID]
	if !ok {
		return notFound("source snapshot", versionID)
	}
	if len(fragments) == 0 {
		return fmt.Errorf("source fragmentation requires at least one fragment")
	}
	if existing, _ := reader(t).SourceFragments(versionID); len(existing) != 0 {
		return conflict("source fragments", versionID)
	}
	next := uint64(0)
	for _, fragment := range fragments {
		if err := fragment.Validate(); err != nil {
			return fmt.Errorf("validate source fragment: %w", err)
		}
		if fragment.SourceVersionID != versionID || fragment.StartOffset != next || fragment.EndOffset > uint64(len(snapshot.Content)) {
			return fmt.Errorf("%w: source fragment coverage or lineage differs", port.ErrConflict)
		}
		if fragment.Location != fmt.Sprintf("bytes:%d-%d", fragment.StartOffset, fragment.EndOffset) {
			return fmt.Errorf("%w: source fragment location differs from offsets", port.ErrConflict)
		}
		content := snapshot.Content[fragment.StartOffset:fragment.EndOffset]
		digest := sha256.Sum256(content)
		wantHash := "sha256:" + hex.EncodeToString(digest[:])
		if fragment.ContentHash != wantHash || fragment.ContentRef != wantHash {
			return fmt.Errorf("%w: source fragment hash or content reference differs", port.ErrConflict)
		}
		if _, exists := t.state.sourceFragments[fragment.ID]; exists {
			return conflict("source fragment", fragment.ID)
		}
		next = fragment.EndOffset
	}
	if next != uint64(len(snapshot.Content)) {
		return fmt.Errorf("%w: source fragments do not cover snapshot", port.ErrConflict)
	}
	for _, fragment := range fragments {
		t.state.sourceFragments[fragment.ID] = fragment
	}
	return nil
}

func (t transaction) AppendObservation(v domain.Observation) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate observation: %w", err)
	}
	if _, exists := t.state.observations[v.ID]; exists {
		return conflict("observation", v.ID)
	}
	if v.Anchor.SourceFragmentID != "" {
		fragment, ok := t.state.sourceFragments[v.Anchor.SourceFragmentID]
		if !ok {
			return notFound("source fragment", v.Anchor.SourceFragmentID)
		}
		snapshot := t.state.sourceSnapshots[fragment.SourceVersionID]
		if v.ExactQuote != string(snapshot.Content[fragment.StartOffset:fragment.EndOffset]) {
			return fmt.Errorf("%w: observation exact quote differs from anchored fragment", port.ErrConflict)
		}
	} else {
		if _, ok := t.state.receipts[v.Anchor.ReceiptID]; !ok {
			return notFound("evidence receipt", v.Anchor.ReceiptID)
		}
	}
	t.state.observations[v.ID] = v
	return nil
}

func (t transaction) AppendClaimWithEvidence(claim domain.Claim, links []domain.EvidenceLink) error {
	if err := claim.Validate(); err != nil {
		return fmt.Errorf("validate claim: %w", err)
	}
	if _, exists := t.state.claims[claim.ID]; exists {
		return conflict("claim", claim.ID)
	}
	if len(links) == 0 {
		return fmt.Errorf("claim proposal requires at least one evidence link")
	}
	seen := make(map[domain.EvidenceLinkID]struct{}, len(links))
	for _, link := range links {
		if err := link.Validate(); err != nil {
			return fmt.Errorf("validate evidence link: %w", err)
		}
		if link.ClaimID != claim.ID {
			return fmt.Errorf("%w: evidence link targets another claim", port.ErrConflict)
		}
		if _, ok := t.state.observations[link.ObservationID]; !ok {
			return notFound("observation", link.ObservationID)
		}
		if _, duplicate := seen[link.ID]; duplicate {
			return conflict("evidence link", link.ID)
		}
		if _, exists := t.state.evidenceLinks[link.ID]; exists {
			return conflict("evidence link", link.ID)
		}
		seen[link.ID] = struct{}{}
	}
	t.state.claims[claim.ID] = cloneClaim(claim)
	for _, link := range links {
		t.state.evidenceLinks[link.ID] = link
	}
	return nil
}

func (t transaction) AppendEvidenceLinks(claimID domain.ClaimID, links []domain.EvidenceLink) error {
	if _, ok := t.state.claims[claimID]; !ok {
		return notFound("claim", claimID)
	}
	if len(links) == 0 {
		return fmt.Errorf("evidence delta must contain at least one link")
	}
	seen := make(map[domain.EvidenceLinkID]struct{}, len(links))
	for _, link := range links {
		if err := link.Validate(); err != nil {
			return fmt.Errorf("validate evidence link: %w", err)
		}
		if link.ClaimID != claimID {
			return fmt.Errorf("%w: evidence link targets another claim", port.ErrConflict)
		}
		if _, ok := t.state.observations[link.ObservationID]; !ok {
			return notFound("observation", link.ObservationID)
		}
		if _, duplicate := seen[link.ID]; duplicate {
			return conflict("evidence link", link.ID)
		}
		if _, exists := t.state.evidenceLinks[link.ID]; exists {
			return conflict("evidence link", link.ID)
		}
		seen[link.ID] = struct{}{}
	}
	for _, link := range links {
		t.state.evidenceLinks[link.ID] = link
	}
	// FR-KNOW-005: evidence deltas invalidate derived views that depend on the claim graph.
	if err := t.markDependentArtifactsStale(domain.EvidenceDeltaDependencyKeys(claimID, links)); err != nil {
		return err
	}
	return nil
}

func (t transaction) AppendKnowledgeArtifact(v domain.KnowledgeArtifact) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate knowledge artifact: %w", err)
	}
	if _, exists := t.state.artifacts[v.ID]; exists {
		return conflict("knowledge artifact", v.ID)
	}
	t.state.artifacts[v.ID] = cloneKnowledgeArtifact(v)
	return nil
}

func (t transaction) SaveKnowledgeArtifact(v domain.KnowledgeArtifact) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate knowledge artifact: %w", err)
	}
	previous, exists := t.state.artifacts[v.ID]
	if !exists {
		return notFound("knowledge artifact", v.ID)
	}
	if previous.SchemaVersion != v.SchemaVersion || previous.ID != v.ID || previous.Kind != v.Kind || previous.BaseCommitID != v.BaseCommitID || previous.ContentRef != v.ContentRef || previous.Content != v.Content || !equalStrings(previous.Dependencies, v.Dependencies) || previous.Stale || !v.Stale {
		return fmt.Errorf("%w: knowledge artifact is immutable except for false-to-true stale transition", port.ErrConflict)
	}
	t.state.artifacts[v.ID] = cloneKnowledgeArtifact(v)
	return nil
}

func (t transaction) AppendRawModelOutput(v domain.RawModelOutput) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate raw model output: %w", err)
	}
	if _, ok := t.state.operations[v.OperationID]; !ok {
		return notFound("operation", v.OperationID)
	}
	if _, ok := t.state.rawModelOutputs[v.ID]; ok {
		return conflict("raw model output", v.ID)
	}
	t.state.rawModelOutputs[v.ID] = v
	return nil
}

func (t transaction) AppendProposedChangeSet(v domain.ProposedChangeSet) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate proposed changeset: %w", err)
	}
	operation, ok := t.state.operations[v.OperationID]
	if !ok {
		return notFound("operation", v.OperationID)
	}
	if operation.MissionRevision != v.MissionRevision || operation.IdempotencyKey != v.IdempotencyKey {
		return fmt.Errorf("%w: proposed changeset differs from operation lineage", port.ErrConflict)
	}
	if _, ok := t.state.proposedChanges[v.ID]; ok {
		return conflict("proposed changeset", v.ID)
	}
	t.state.proposedChanges[v.ID] = cloneProposedChangeSet(v)
	return nil
}

func (t transaction) AppendValidationReceipt(v domain.ValidationReceipt) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate validation receipt: %w", err)
	}
	proposal, ok := t.state.proposedChanges[v.ChangeSetID]
	if !ok {
		return notFound("proposed changeset", v.ChangeSetID)
	}
	if proposal.OperationID != v.OperationID {
		return fmt.Errorf("%w: validation receipt differs from changeset operation", port.ErrConflict)
	}
	if _, ok := t.state.rawModelOutputs[v.ArtifactRef]; !ok {
		return notFound("raw model output", v.ArtifactRef)
	}
	if _, ok := t.state.receipts[v.ID]; ok {
		return conflict("validation receipt", v.ID)
	}
	t.state.receipts[v.ID] = v
	return nil
}

func (t transaction) AppendAcceptedChangeSet(v domain.AcceptedChangeSet) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate accepted changeset: %w", err)
	}
	proposal, ok := t.state.proposedChanges[v.ProposedChangeSetID]
	if !ok {
		return notFound("proposed changeset", v.ProposedChangeSetID)
	}
	seenValidators := make(map[string]struct{}, len(v.ValidationReceiptIDs))
	for _, receiptID := range v.ValidationReceiptIDs {
		receipt, ok := t.state.receipts[receiptID]
		if !ok {
			return notFound("validation receipt", receiptID)
		}
		if receipt.ChangeSetID != proposal.ID || receipt.OperationID != proposal.OperationID || !receipt.Passed {
			return fmt.Errorf("%w: invalid validation receipt lineage", port.ErrConflict)
		}
		seenValidators[receipt.ValidatorID] = struct{}{}
	}
	for _, validatorID := range proposal.ValidatorIDs {
		if _, ok := seenValidators[validatorID]; !ok {
			return fmt.Errorf("%w: required validator %s has no receipt", port.ErrConflict, validatorID)
		}
	}
	if _, ok := t.state.acceptedChanges[v.ID]; ok {
		return conflict("accepted changeset", v.ID)
	}
	t.state.acceptedChanges[v.ID] = cloneAcceptedChangeSet(v)
	return nil
}

func (t transaction) ApplyCommit(v domain.Commit, receipt domain.CommitReceipt, changes []domain.Change) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate commit: %w", err)
	}
	if err := receipt.Validate(); err != nil {
		return fmt.Errorf("validate commit receipt: %w", err)
	}
	if receipt.ID != v.ReceiptID || receipt.CommitID != v.ID || receipt.ChangeSetID != v.AcceptedChangeSetID || receipt.Version != v.Version {
		return fmt.Errorf("%w: commit receipt differs from commit", port.ErrConflict)
	}
	accepted, ok := t.state.acceptedChanges[v.AcceptedChangeSetID]
	if !ok {
		return notFound("accepted changeset", v.AcceptedChangeSetID)
	}
	proposal := t.state.proposedChanges[accepted.ProposedChangeSetID]
	if proposal.MissionRevision != v.MissionRevision || proposal.BaseCommitID != v.BaseCommitID || proposal.IdempotencyKey != v.IdempotencyKey || !equalChanges(proposal.Changes, changes) {
		return fmt.Errorf("%w: commit differs from accepted proposal", port.ErrConflict)
	}
	if existingID, ok := t.state.commitByIntent[v.IdempotencyKey]; ok {
		existing := t.state.commits[existingID]
		if existing == v {
			return nil
		}
		return conflict("commit idempotency key", v.IdempotencyKey)
	}
	headID := domain.GenesisCommitID
	headVersion := uint64(0)
	if existingHead, ok := t.state.headCommits[v.MissionRevision]; ok {
		headID = existingHead
		headVersion = t.state.commits[existingHead].Version
	}
	if v.BaseCommitID != headID || v.Version != headVersion+1 {
		return fmt.Errorf("%w: stale base commit or non-sequential version", port.ErrConflict)
	}
	if _, ok := t.state.commits[v.ID]; ok {
		return conflict("commit", v.ID)
	}
	if _, ok := t.state.commitReceipts[receipt.ID]; ok {
		return conflict("commit receipt", receipt.ID)
	}
	for _, change := range changes {
		key := canonicalKey(change.EntityType, change.EntityID)
		current, exists := t.state.canonical[key]
		switch change.Kind {
		case domain.ChangeAdd:
			if exists {
				return fmt.Errorf("%w: add target %s already exists", port.ErrConflict, key)
			}
			t.state.canonical[key] = domain.CanonicalEntity{EntityType: change.EntityType, EntityID: change.EntityID, PayloadRef: change.PayloadRef, Version: v.Version, CommitID: v.ID}
		case domain.ChangeReplace:
			if !exists || current.Deprecated {
				return fmt.Errorf("%w: replace target %s is missing or deprecated", port.ErrConflict, key)
			}
			current.PayloadRef, current.Version, current.CommitID = change.PayloadRef, v.Version, v.ID
			t.state.canonical[key] = current
		case domain.ChangeDeprecate:
			if !exists || current.Deprecated {
				return fmt.Errorf("%w: deprecate target %s is missing or deprecated", port.ErrConflict, key)
			}
			current.Deprecated, current.Version, current.CommitID = true, v.Version, v.ID
			t.state.canonical[key] = current
		default:
			return fmt.Errorf("unsupported change kind %q", change.Kind)
		}
	}
	// FR-KNOW-005: official commits mark dependent derived artifacts stale in the same transaction.
	if err := t.markDependentArtifactsStale(domain.ChangeDependencyKeys(changes)); err != nil {
		return err
	}
	t.state.commits[v.ID] = v
	t.state.commitReceipts[receipt.ID] = receipt
	t.state.commitByIntent[v.IdempotencyKey] = v.ID
	t.state.headCommits[v.MissionRevision] = v.ID
	return nil
}

func notFound(kind string, id any) error { return fmt.Errorf("%w: %s %v", port.ErrNotFound, kind, id) }
func conflict(kind string, id any) error {
	return fmt.Errorf("%w: %s %v already exists", port.ErrConflict, kind, id)
}

// markDependentArtifactsStale applies FR-KNOW-005 cascade: only false→true Stale
// on non-audit artifacts whose Dependencies intersect changedKeys.
func (t transaction) markDependentArtifactsStale(changedKeys []string) error {
	if len(changedKeys) == 0 || len(t.state.artifacts) == 0 {
		return nil
	}
	artifacts := make([]domain.KnowledgeArtifact, 0, len(t.state.artifacts))
	for _, artifact := range t.state.artifacts {
		artifacts = append(artifacts, artifact)
	}
	for _, id := range domain.PlanArtifactInvalidation(artifacts, changedKeys, domain.IsLocalAuditArtifactKind) {
		artifact, ok := t.state.artifacts[id]
		if !ok || artifact.Stale {
			continue
		}
		artifact.Stale = true
		if err := artifact.Validate(); err != nil {
			return fmt.Errorf("validate knowledge artifact %s after stale mark: %w", id, err)
		}
		t.state.artifacts[id] = cloneKnowledgeArtifact(artifact)
	}
	return nil
}

func cloneState(src state) state {
	dst := newState()
	for k, v := range src.subagentRecords {
		dst.subagentRecords[k] = v
	}
	for k, v := range src.subagentDispatches {
		dst.subagentDispatches[k] = v
	}
	for k, v := range src.subagentSpawnReceipts {
		dst.subagentSpawnReceipts[k] = v
	}
	for k, v := range src.dispatchByGeneration {
		dst.dispatchByGeneration[k] = v
	}
	for k, v := range src.missionRevisions {
		dst.missionRevisions[k] = cloneMission(v)
	}
	for k, v := range src.activeMissions {
		dst.activeMissions[k] = v
	}
	for k, v := range src.operationSpecs {
		dst.operationSpecs[k] = cloneOperationSpec(v)
	}
	for k, v := range src.questions {
		dst.questions[k] = v
	}
	for k, v := range src.operatorQuestions {
		dst.operatorQuestions[k] = cloneOperatorQuestion(v)
	}
	for k, v := range src.operatorAnswers {
		dst.operatorAnswers[k] = cloneUserAnswer(v)
	}
	for k, v := range src.answerByTransport {
		dst.answerByTransport[k] = v
	}
	for k, v := range src.questionDeliveries {
		dst.questionDeliveries[k] = v
	}
	for k, v := range src.deliveryByRoute {
		dst.deliveryByRoute[k] = v
	}
	for k, v := range src.deliveryByTransport {
		dst.deliveryByTransport[k] = v
	}
	for k, v := range src.questionGateDecisions {
		dst.questionGateDecisions[k] = v
	}
	for k, v := range src.gateDecisionByQuestion {
		dst.gateDecisionByQuestion[k] = v
	}
	for k, v := range src.candidates {
		dst.candidates[k] = cloneCandidate(v)
	}
	for k, v := range src.inquiries {
		dst.inquiries[k] = v
	}
	for k, v := range src.operations {
		dst.operations[k] = cloneOperation(v)
	}
	for k, v := range src.memories {
		dst.memories[k] = v
	}
	dst.events = append([]domain.Event(nil), src.events...)
	for k, v := range src.eventIDs {
		dst.eventIDs[k] = v
	}
	for k, v := range src.idempotency {
		dst.idempotency[k] = v
	}
	for k, v := range src.sources {
		dst.sources[k] = v
	}
	for k, v := range src.sourceVersions {
		dst.sourceVersions[k] = v
	}
	for k, v := range src.sourceSnapshots {
		dst.sourceSnapshots[k] = cloneSourceSnapshot(v)
	}
	for k, v := range src.sourceFragments {
		dst.sourceFragments[k] = v
	}
	for k, v := range src.observations {
		dst.observations[k] = v
	}
	for k, v := range src.claims {
		dst.claims[k] = cloneClaim(v)
	}
	for k, v := range src.evidenceLinks {
		dst.evidenceLinks[k] = v
	}
	for k, v := range src.artifacts {
		dst.artifacts[k] = cloneKnowledgeArtifact(v)
	}
	for k, v := range src.rawModelOutputs {
		dst.rawModelOutputs[k] = v
	}
	for k, v := range src.proposedChanges {
		dst.proposedChanges[k] = cloneProposedChangeSet(v)
	}
	for k, v := range src.acceptedChanges {
		dst.acceptedChanges[k] = cloneAcceptedChangeSet(v)
	}
	for k, v := range src.receipts {
		dst.receipts[k] = v
	}
	for k, v := range src.commitReceipts {
		dst.commitReceipts[k] = v
	}
	for k, v := range src.commits {
		dst.commits[k] = v
	}
	for k, v := range src.commitByIntent {
		dst.commitByIntent[k] = v
	}
	for k, v := range src.headCommits {
		dst.headCommits[k] = v
	}
	for k, v := range src.canonical {
		dst.canonical[k] = v
	}
	if src.hasControlState {
		dst.controlState = cloneControlState(src.controlState)
		dst.hasControlState = true
	}
	for k, v := range src.operatorCommands {
		dst.operatorCommands[k] = cloneOperatorCommand(v)
	}
	for k, v := range src.operatorCommandByIdem {
		dst.operatorCommandByIdem[k] = v
	}
	for k, v := range src.operatorCommandReceipts {
		dst.operatorCommandReceipts[k] = v
	}
	for k, v := range src.externalEvents {
		dst.externalEvents[k] = cloneExternalEvent(v)
	}
	for k, v := range src.externalEventByDedup {
		dst.externalEventByDedup[k] = v
	}
	for k, v := range src.externalEventDispositions {
		dst.externalEventDispositions[k] = v
	}
	for k, v := range src.workOpportunities {
		dst.workOpportunities[k] = cloneWorkOpportunity(v)
	}
	for k, v := range src.continuityDiagnoses {
		dst.continuityDiagnoses[k] = cloneContinuityDiagnosis(v)
	}
	for k, v := range src.configDrafts {
		dst.configDrafts[k] = cloneConfigDraft(v)
	}
	for k, v := range src.configRevisions {
		dst.configRevisions[k] = cloneConfigRevision(v)
	}
	for k, v := range src.activeConfig {
		dst.activeConfig[k] = v
	}
	for k, v := range src.configApplyReceipts {
		dst.configApplyReceipts[k] = v
	}
	for k, v := range src.channelCursors {
		dst.channelCursors[k] = v
	}
	for k, v := range src.peerSyncInbox {
		dst.peerSyncInbox[k] = clonePeerSyncInboxRecord(v)
	}
	for k, v := range src.peerSyncCursors {
		dst.peerSyncCursors[k] = v
	}
	for k, v := range src.resourceUsages {
		dst.resourceUsages[k] = cloneResourceUsage(v)
	}
	for k, v := range src.modelContextPressures {
		dst.modelContextPressures[k] = v
	}
	return dst
}

func clonePeerSyncInboxRecord(v domain.PeerSyncInboxRecord) domain.PeerSyncInboxRecord {
	out := v
	out.Message.Events = append([]domain.Event(nil), v.Message.Events...)
	return out
}

func peerSyncInboxRecordsEqual(a, b domain.PeerSyncInboxRecord) bool {
	if a.SchemaVersion != b.SchemaVersion || a.PeerID != b.PeerID || a.OriginID != b.OriginID || a.MessageID != b.MessageID || a.StreamID != b.StreamID {
		return false
	}
	return fmt.Sprintf("%#v", a.Message) == fmt.Sprintf("%#v", b.Message)
}

func cloneResourceUsage(v domain.ResourceUsage) domain.ResourceUsage {
	out := v
	if v.CircuitOpenUntil != nil {
		t := v.CircuitOpenUntil.UTC()
		out.CircuitOpenUntil = &t
	}
	if v.LastFailureAt != nil {
		t := v.LastFailureAt.UTC()
		out.LastFailureAt = &t
	}
	return out
}

func cloneWorkOpportunity(v domain.WorkOpportunity) domain.WorkOpportunity {
	v.Dependencies = append([]string(nil), v.Dependencies...)
	return v
}

func cloneContinuityDiagnosis(v domain.ContinuityDiagnosis) domain.ContinuityDiagnosis {
	v.StrategiesTried = append([]string(nil), v.StrategiesTried...)
	v.UnavailableCapabilities = append([]string(nil), v.UnavailableCapabilities...)
	v.EliminatedAlternatives = append([]string(nil), v.EliminatedAlternatives...)
	v.RecoveryConditions = append([]string(nil), v.RecoveryConditions...)
	return v
}

func cloneConfigDraft(v domain.ConfigDraft) domain.ConfigDraft {
	v.Runtime = cloneRuntimePtr(v.Runtime)
	v.Scheduler = cloneSchedulerPtr(v.Scheduler)
	v.Horizon = cloneHorizonPtr(v.Horizon)
	v.Interruption = cloneInterruptionPtr(v.Interruption)
	v.Channels = cloneChannelsPtr(v.Channels)
	v.Models = cloneModelsPtr(v.Models)
	return v
}

func cloneConfigRevision(v domain.ConfigRevision) domain.ConfigRevision {
	v.Runtime = cloneRuntimePtr(v.Runtime)
	v.Scheduler = cloneSchedulerPtr(v.Scheduler)
	v.Horizon = cloneHorizonPtr(v.Horizon)
	v.Interruption = cloneInterruptionPtr(v.Interruption)
	v.Channels = cloneChannelsPtr(v.Channels)
	v.Models = cloneModelsPtr(v.Models)
	return v
}

func cloneRuntimePtr(v *domain.RuntimeProcessConfig) *domain.RuntimeProcessConfig {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
}

func cloneSchedulerPtr(v *domain.SchedulerCadenceConfig) *domain.SchedulerCadenceConfig {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
}

func cloneHorizonPtr(v *domain.HorizonPolicy) *domain.HorizonPolicy {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
}

func cloneInterruptionPtr(v *domain.InterruptionRuntimePolicy) *domain.InterruptionRuntimePolicy {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
}

func cloneChannelsPtr(v *domain.ChannelsConfig) *domain.ChannelsConfig {
	if v == nil {
		return nil
	}
	cp := *v
	cp.Routes = append([]domain.ChannelRouteConfig(nil), v.Routes...)
	return &cp
}

func cloneModelsPtr(v *domain.ModelsConfig) *domain.ModelsConfig {
	if v == nil {
		return nil
	}
	cp := *v
	cp.Providers = append([]domain.ModelProviderConfig(nil), v.Providers...)
	cp.Bindings = append([]domain.ModelBindingConfig(nil), v.Bindings...)
	return &cp
}

func equalConfigPayloads(a, b domain.ConfigDraft) bool {
	ha, errA := domain.ConfigPayloadHash(a.Scope, a.Runtime, a.Scheduler, a.Horizon, a.Interruption, a.Channels, a.Models)
	hb, errB := domain.ConfigPayloadHash(b.Scope, b.Runtime, b.Scheduler, b.Horizon, b.Interruption, b.Channels, b.Models)
	if errA != nil || errB != nil {
		return false
	}
	return ha == hb
}

func cloneExternalEvent(event domain.ExternalEvent) domain.ExternalEvent {
	event.Content.Structured = append([]byte(nil), event.Content.Structured...)
	return event
}

func equalExternalEvents(a, b domain.ExternalEvent) bool {
	if a.SchemaVersion != b.SchemaVersion || a.ID != b.ID || a.DeduplicationKey != b.DeduplicationKey || a.Source != b.Source || a.SourceActorID != b.SourceActorID || a.Kind != b.Kind || a.MissionID != b.MissionID || a.CorrelationID != b.CorrelationID || a.TransportMessageID != b.TransportMessageID || !a.ReceivedAt.Equal(b.ReceivedAt) {
		return false
	}
	if a.Content.MediaType != b.Content.MediaType || a.Content.Text != b.Content.Text || a.Content.Reference != b.Content.Reference {
		return false
	}
	return string(a.Content.Structured) == string(b.Content.Structured)
}
func cloneKnowledgeArtifact(v domain.KnowledgeArtifact) domain.KnowledgeArtifact {
	v.Dependencies = append([]string(nil), v.Dependencies...)
	return v
}
func cloneMission(v domain.MissionRevision) domain.MissionRevision {
	v.Domains = append([]string(nil), v.Domains...)
	v.Policies = append([]string(nil), v.Policies...)
	return v
}
func cloneCandidate(v domain.InquiryCandidate) domain.InquiryCandidate {
	v.DerivedFrom = append([]string(nil), v.DerivedFrom...)
	v.SourcePlan = append([]string(nil), v.SourcePlan...)
	return v
}
func cloneOperation(v domain.Operation) domain.Operation {
	v.ReadSet = append([]string(nil), v.ReadSet...)
	v.InputRefs = append([]string(nil), v.InputRefs...)
	return v
}
func cloneOperatorQuestion(v domain.OperatorQuestion) domain.OperatorQuestion {
	v.Options = append([]domain.QuestionOption(nil), v.Options...)
	v.BlockingScope = append([]domain.QuestionBlockingTarget(nil), v.BlockingScope...)
	return v
}
func cloneUserAnswer(v domain.UserAnswer) domain.UserAnswer {
	v.OptionIDs = append([]string(nil), v.OptionIDs...)
	return v
}
func transportAnswerKey(channel, eventID string) string { return channel + "\x00" + eventID }
func deliveryTransportKey(channel, messageID string) string {
	return channel + "\x00" + messageID
}
func questionDeliveryRouteKey(questionID domain.OperatorQuestionID, revision uint64, channel, destination string) string {
	return fmt.Sprintf("%s\x00%d\x00%s\x00%s", questionID, revision, channel, destination)
}
func cloneSourceSnapshot(v domain.SourceSnapshot) domain.SourceSnapshot {
	v.Content = append([]byte(nil), v.Content...)
	return v
}

func cloneClaim(v domain.Claim) domain.Claim {
	qualifiers := make(map[string]string, len(v.Qualifiers))
	for key, value := range v.Qualifiers {
		qualifiers[key] = value
	}
	v.Qualifiers = qualifiers
	return v
}

func cloneProposedChangeSet(v domain.ProposedChangeSet) domain.ProposedChangeSet {
	v.ReadSet = append([]string(nil), v.ReadSet...)
	v.Preconditions = append([]string(nil), v.Preconditions...)
	v.Changes = append([]domain.Change(nil), v.Changes...)
	v.ValidatorIDs = append([]string(nil), v.ValidatorIDs...)
	return v
}

func cloneAcceptedChangeSet(v domain.AcceptedChangeSet) domain.AcceptedChangeSet {
	v.ValidationReceiptIDs = append([]domain.ReceiptID(nil), v.ValidationReceiptIDs...)
	return v
}

func canonicalKey(entityType, entityID string) string { return entityType + "\x00" + entityID }

func equalChanges(a, b []domain.Change) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func cloneOperationSpec(v domain.OperationSpec) domain.OperationSpec {
	v.Validators = append([]string(nil), v.Validators...)
	return v
}

func cloneOperatorCommand(v domain.OperatorCommand) domain.OperatorCommand {
	if v.ExpectedRevision != nil {
		revision := *v.ExpectedRevision
		v.ExpectedRevision = &revision
	}
	return v
}

func cloneControlState(v domain.ControlState) domain.ControlState {
	if v.Missions != nil {
		missions := make(map[domain.MissionID]domain.MissionControl, len(v.Missions))
		for id, mission := range v.Missions {
			missions[id] = mission
		}
		v.Missions = missions
	}
	return v
}

func equalOperatorCommands(a, b domain.OperatorCommand) bool {
	if a.SchemaVersion != b.SchemaVersion || a.ID != b.ID || a.IdempotencyKey != b.IdempotencyKey || a.ActorType != b.ActorType || a.ActorID != b.ActorID || a.Kind != b.Kind || a.Target != b.Target || a.Reason != b.Reason || !a.SubmittedAt.Equal(b.SubmittedAt) {
		return false
	}
	switch {
	case a.ExpectedRevision == nil && b.ExpectedRevision == nil:
		return true
	case a.ExpectedRevision == nil || b.ExpectedRevision == nil:
		return false
	default:
		return *a.ExpectedRevision == *b.ExpectedRevision
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

var _ port.Store = (*Store)(nil)

func (t transaction) DeletePeerSyncInboxRecord(peerID, originID, messageID string) error {
	key := peerSyncInboxKey(peerID, originID, messageID)
	if _, ok := t.state.peerSyncInbox[key]; !ok {
		return port.ErrNotFound
	}
	delete(t.state.peerSyncInbox, key)
	return nil
}

func (t transaction) CreateSubagentRecord(record domain.SubagentRecord) error {
	if err := record.Validate(); err != nil {
		return fmt.Errorf("validate subagent record: %w", err)
	}
	if _, exists := t.state.subagentRecords[record.ID]; exists {
		return fmt.Errorf("%w: subagent record %s already exists", port.ErrConflict, record.ID)
	}
	t.state.subagentRecords[record.ID] = record
	return nil
}

func (t transaction) SaveSubagentRecord(record domain.SubagentRecord) error {
	if err := record.Validate(); err != nil {
		return fmt.Errorf("validate subagent record: %w", err)
	}
	if _, exists := t.state.subagentRecords[record.ID]; !exists {
		return port.ErrNotFound
	}
	t.state.subagentRecords[record.ID] = record
	return nil
}

func (t transaction) SubagentRecord(id string) (domain.SubagentRecord, error) {
	return reader{t.state}.SubagentRecord(id)
}
func (r reader) SubagentRecord(id string) (domain.SubagentRecord, error) {
	record, exists := r.state.subagentRecords[id]
	if !exists {
		return domain.SubagentRecord{}, port.ErrNotFound
	}
	return record, nil
}

func (t transaction) SubagentRecordsByState(state domain.SubagentState, limit int) ([]domain.SubagentRecord, error) {
	return reader{t.state}.SubagentRecordsByState(state, limit)
}

func subagentDispatchGenerationKey(sessionID string, attempt int) string {
	return fmt.Sprintf("%s\x00%d", sessionID, attempt)
}

func subagentSpawnReceiptKey(callerPeerID, requestID string) string {
	return callerPeerID + "\x00" + requestID
}

func (t transaction) SubagentSpawnReceipt(callerPeerID, requestID string) (domain.SubagentSpawnReceipt, error) {
	return reader(t).SubagentSpawnReceipt(callerPeerID, requestID)
}
func (r reader) SubagentSpawnReceipt(callerPeerID, requestID string) (domain.SubagentSpawnReceipt, error) {
	v, ok := r.state.subagentSpawnReceipts[subagentSpawnReceiptKey(callerPeerID, requestID)]
	if !ok {
		return domain.SubagentSpawnReceipt{}, notFound("subagent spawn receipt", requestID)
	}
	return v, nil
}
func (t transaction) CreateSubagentSpawnReceipt(v domain.SubagentSpawnReceipt) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate subagent spawn receipt: %w", err)
	}
	key := subagentSpawnReceiptKey(v.CallerPeerID, v.RequestID)
	if existing, ok := t.state.subagentSpawnReceipts[key]; ok {
		if existing == v {
			return nil
		}
		return conflict("subagent spawn receipt", v.RequestID)
	}
	if _, ok := t.state.subagentRecords[v.ReceiverSessionID]; !ok {
		return notFound("subagent record", v.ReceiverSessionID)
	}
	t.state.subagentSpawnReceipts[key] = v
	return nil
}

func (t transaction) SubagentDispatch(id domain.SubagentDispatchRequestID) (domain.SubagentDispatch, error) {
	return reader(t).SubagentDispatch(id)
}
func (r reader) SubagentDispatch(id domain.SubagentDispatchRequestID) (domain.SubagentDispatch, error) {
	v, ok := r.state.subagentDispatches[id]
	if !ok {
		return domain.SubagentDispatch{}, notFound("subagent dispatch", id)
	}
	return v, nil
}
func (t transaction) SubagentDispatchByGeneration(sessionID string, attempt int) (domain.SubagentDispatch, error) {
	return reader(t).SubagentDispatchByGeneration(sessionID, attempt)
}
func (r reader) SubagentDispatchByGeneration(sessionID string, attempt int) (domain.SubagentDispatch, error) {
	id, ok := r.state.dispatchByGeneration[subagentDispatchGenerationKey(sessionID, attempt)]
	if !ok {
		return domain.SubagentDispatch{}, notFound("subagent dispatch generation", sessionID)
	}
	return r.SubagentDispatch(id)
}
func (t transaction) DueSubagentDispatches(now time.Time, limit int) ([]domain.SubagentDispatch, error) {
	return reader(t).DueSubagentDispatches(now, limit)
}
func (r reader) DueSubagentDispatches(now time.Time, limit int) ([]domain.SubagentDispatch, error) {
	if now.IsZero() || limit <= 0 {
		return nil, fmt.Errorf("subagent dispatch query requires time and positive limit")
	}
	result := make([]domain.SubagentDispatch, 0, limit)
	for _, dispatch := range r.state.subagentDispatches {
		if dispatch.Due(now) {
			result = append(result, dispatch)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].AvailableAt.Equal(result[j].AvailableAt) {
			return result[i].RequestID < result[j].RequestID
		}
		return result[i].AvailableAt.Before(result[j].AvailableAt)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}
func (t transaction) CreateSubagentDispatch(v domain.SubagentDispatch) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate subagent dispatch: %w", err)
	}
	if v.Status != domain.SubagentDispatchPending || v.SendAttempt != 0 {
		return fmt.Errorf("%w: subagent dispatch must be created pending before sends", port.ErrConflict)
	}
	if _, exists := t.state.subagentDispatches[v.RequestID]; exists {
		return conflict("subagent dispatch", v.RequestID)
	}
	key := subagentDispatchGenerationKey(v.SessionID, v.Attempt)
	if _, exists := t.state.dispatchByGeneration[key]; exists {
		return fmt.Errorf("%w: subagent generation already has a dispatch", port.ErrConflict)
	}
	record, exists := t.state.subagentRecords[v.SessionID]
	if !exists {
		return notFound("subagent record", v.SessionID)
	}
	if record.Attempt != v.Attempt || record.TransportPeerID != v.PeerID {
		return fmt.Errorf("%w: dispatch does not match subagent generation or peer", port.ErrConflict)
	}
	t.state.subagentDispatches[v.RequestID] = v
	t.state.dispatchByGeneration[key] = v.RequestID
	return nil
}
func (t transaction) SaveSubagentDispatch(v domain.SubagentDispatch, expectedStatus domain.SubagentDispatchStatus, expectedSendAttempt uint32) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate subagent dispatch: %w", err)
	}
	current, ok := t.state.subagentDispatches[v.RequestID]
	if !ok {
		return notFound("subagent dispatch", v.RequestID)
	}
	if current.Status != expectedStatus || current.SendAttempt != expectedSendAttempt {
		return fmt.Errorf("%w: stale subagent dispatch state", port.ErrConflict)
	}
	if current.SessionID != v.SessionID || current.Attempt != v.Attempt || current.PeerID != v.PeerID || current.RequestID != v.RequestID || current.CreatedAt != v.CreatedAt || current.MaxSendAttempts != v.MaxSendAttempts {
		return fmt.Errorf("%w: immutable subagent dispatch fields changed", port.ErrConflict)
	}
	t.state.subagentDispatches[v.RequestID] = v
	return nil
}
func (r reader) SubagentRecordsByState(state domain.SubagentState, limit int) ([]domain.SubagentRecord, error) {
	var result []domain.SubagentRecord
	for _, record := range r.state.subagentRecords {
		if record.State == state {
			result = append(result, record)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].UpdatedAt.Equal(result[j].UpdatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].UpdatedAt.Before(result[j].UpdatedAt)
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}
