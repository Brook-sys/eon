package memory

import (
	"bytes"
	"encoding/gob"
	"fmt"

	"motor-autonomo/internal/domain"
)

// persistedState is an internal, versioned-independent transport used by disk
// adapters to checkpoint the complete in-memory reference model. Domain schema
// versions remain authoritative for individual records.
type persistedState struct {
	MissionRevisions          map[domain.MissionRevisionID]domain.MissionRevision
	ActiveMissions            map[domain.MissionID]domain.MissionRevisionID
	OperationSpecs            map[domain.OperationSpecID]domain.OperationSpec
	Questions                 map[domain.QuestionID]domain.Question
	OperatorQuestions         map[domain.OperatorQuestionID]domain.OperatorQuestion
	OperatorAnswers           map[domain.OperatorAnswerID]domain.UserAnswer
	AnswerByTransport         map[string]domain.OperatorAnswerID
	QuestionDeliveries        map[domain.QuestionDeliveryID]domain.QuestionDelivery
	DeliveryByRoute           map[string]domain.QuestionDeliveryID
	DeliveryByTransport       map[string]domain.QuestionDeliveryID
	QuestionGateDecisions     map[domain.QuestionGateDecisionID]domain.QuestionGateDecisionRecord
	GateDecisionByQuestion    map[domain.OperatorQuestionID]domain.QuestionGateDecisionID
	Candidates                map[domain.InquiryCandidateID]domain.InquiryCandidate
	Inquiries                 map[domain.InquiryID]domain.Inquiry
	Operations                map[domain.OperationID]domain.Operation
	Events                    []domain.Event
	EventIDs                  map[domain.EventID]uint64
	Idempotency               map[domain.IdempotencyKey]domain.IdempotencyRecord
	Sources                   map[domain.SourceID]domain.Source
	SourceVersions            map[domain.SourceVersionID]domain.SourceVersion
	SourceSnapshots           map[domain.SourceVersionID]domain.SourceSnapshot
	SourceFragments           map[domain.SourceFragmentID]domain.SourceFragment
	Observations              map[domain.ObservationID]domain.Observation
	Claims                    map[domain.ClaimID]domain.Claim
	EvidenceLinks             map[domain.EvidenceLinkID]domain.EvidenceLink
	Artifacts                 map[domain.ArtifactID]domain.KnowledgeArtifact
	RawModelOutputs           map[domain.ArtifactID]domain.RawModelOutput
	ProposedChanges           map[domain.ChangeSetID]domain.ProposedChangeSet
	AcceptedChanges           map[domain.ChangeSetID]domain.AcceptedChangeSet
	Receipts                  map[domain.ReceiptID]domain.ValidationReceipt
	CommitReceipts            map[domain.ReceiptID]domain.CommitReceipt
	Commits                   map[domain.CommitID]domain.Commit
	CommitByIntent            map[domain.IdempotencyKey]domain.CommitID
	HeadCommits               map[domain.MissionRevisionID]domain.CommitID
	Canonical                 map[string]domain.CanonicalEntity
	HasControlState           bool
	ControlState              domain.ControlState
	OperatorCommands          map[domain.CommandID]domain.OperatorCommand
	OperatorCommandByIdem     map[domain.IdempotencyKey]domain.CommandID
	OperatorCommandReceipts   map[domain.CommandID]domain.CommandReceipt
	ExternalEvents            map[domain.ExternalEventID]domain.ExternalEvent
	ExternalEventByDedup      map[string]domain.ExternalEventID
	ExternalEventDispositions map[domain.ExternalEventID]domain.ExternalEventDisposition
	WorkOpportunities         map[domain.WorkOpportunityID]domain.WorkOpportunity
	ContinuityDiagnoses       map[domain.ContinuityDiagnosisID]domain.ContinuityDiagnosis
	ConfigDrafts              map[domain.ConfigDraftID]domain.ConfigDraft
	ConfigRevisions           map[domain.ConfigRevisionID]domain.ConfigRevision
	ActiveConfig              map[domain.ConfigScope]domain.ConfigRevisionID
	ConfigApplyReceipts       map[domain.ConfigDraftID]domain.ConfigApplyReceipt
	ChannelCursors            map[string]domain.ChannelCursor
}

// MarshalBinary returns an isolated checkpoint of the reference store.
func (s *Store) MarshalBinary() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cloned := cloneState(s.state)
	p := persistedState{
		MissionRevisions: cloned.missionRevisions, ActiveMissions: cloned.activeMissions,
		OperationSpecs: cloned.operationSpecs, Questions: cloned.questions, OperatorQuestions: cloned.operatorQuestions,
		OperatorAnswers: cloned.operatorAnswers, AnswerByTransport: cloned.answerByTransport,
		QuestionDeliveries: cloned.questionDeliveries, DeliveryByRoute: cloned.deliveryByRoute,
		DeliveryByTransport:   cloned.deliveryByTransport,
		QuestionGateDecisions: cloned.questionGateDecisions, GateDecisionByQuestion: cloned.gateDecisionByQuestion,
		Candidates: cloned.candidates, Inquiries: cloned.inquiries, Operations: cloned.operations,
		Events: cloned.events, EventIDs: cloned.eventIDs, Idempotency: cloned.idempotency,
		Sources: cloned.sources, SourceVersions: cloned.sourceVersions, SourceSnapshots: cloned.sourceSnapshots,
		SourceFragments: cloned.sourceFragments, Observations: cloned.observations, Claims: cloned.claims,
		EvidenceLinks: cloned.evidenceLinks, Artifacts: cloned.artifacts, RawModelOutputs: cloned.rawModelOutputs,
		ProposedChanges: cloned.proposedChanges, AcceptedChanges: cloned.acceptedChanges, Receipts: cloned.receipts,
		CommitReceipts: cloned.commitReceipts, Commits: cloned.commits, CommitByIntent: cloned.commitByIntent,
		HeadCommits: cloned.headCommits, Canonical: cloned.canonical,
		HasControlState: cloned.hasControlState, ControlState: cloned.controlState,
		OperatorCommands: cloned.operatorCommands, OperatorCommandByIdem: cloned.operatorCommandByIdem,
		OperatorCommandReceipts: cloned.operatorCommandReceipts,
		ExternalEvents:          cloned.externalEvents, ExternalEventByDedup: cloned.externalEventByDedup,
		ExternalEventDispositions: cloned.externalEventDispositions,
		WorkOpportunities:         cloned.workOpportunities, ContinuityDiagnoses: cloned.continuityDiagnoses,
		ConfigDrafts: cloned.configDrafts, ConfigRevisions: cloned.configRevisions,
		ActiveConfig: cloned.activeConfig, ConfigApplyReceipts: cloned.configApplyReceipts,
		ChannelCursors: cloned.channelCursors,
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(p); err != nil {
		return nil, fmt.Errorf("encode memory checkpoint: %w", err)
	}
	return buf.Bytes(), nil
}

// NewFromBinary restores a checkpoint produced by MarshalBinary.
func NewFromBinary(data []byte) (*Store, error) {
	var p persistedState
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&p); err != nil {
		return nil, fmt.Errorf("decode memory checkpoint: %w", err)
	}
	base := newState()
	base.missionRevisions = nonNil(p.MissionRevisions, base.missionRevisions)
	base.activeMissions = nonNil(p.ActiveMissions, base.activeMissions)
	base.operationSpecs = nonNil(p.OperationSpecs, base.operationSpecs)
	base.questions = nonNil(p.Questions, base.questions)
	base.operatorQuestions = nonNil(p.OperatorQuestions, base.operatorQuestions)
	base.operatorAnswers = nonNil(p.OperatorAnswers, base.operatorAnswers)
	base.answerByTransport = nonNil(p.AnswerByTransport, base.answerByTransport)
	base.questionDeliveries = nonNil(p.QuestionDeliveries, base.questionDeliveries)
	base.deliveryByRoute = nonNil(p.DeliveryByRoute, base.deliveryByRoute)
	base.deliveryByTransport = nonNil(p.DeliveryByTransport, base.deliveryByTransport)
	// Older checkpoints may omit the transport index; rebuild from delivered rows.
	if len(base.deliveryByTransport) == 0 && len(base.questionDeliveries) > 0 {
		for id, delivery := range base.questionDeliveries {
			if delivery.TransportMessageID == "" {
				continue
			}
			base.deliveryByTransport[deliveryTransportKey(delivery.Channel, delivery.TransportMessageID)] = id
		}
	}
	base.questionGateDecisions = nonNil(p.QuestionGateDecisions, base.questionGateDecisions)
	base.gateDecisionByQuestion = nonNil(p.GateDecisionByQuestion, base.gateDecisionByQuestion)
	base.candidates = nonNil(p.Candidates, base.candidates)
	base.inquiries = nonNil(p.Inquiries, base.inquiries)
	base.operations = nonNil(p.Operations, base.operations)
	base.events = append([]domain.Event(nil), p.Events...)
	base.eventIDs = nonNil(p.EventIDs, base.eventIDs)
	base.idempotency = nonNil(p.Idempotency, base.idempotency)
	base.sources = nonNil(p.Sources, base.sources)
	base.sourceVersions = nonNil(p.SourceVersions, base.sourceVersions)
	base.sourceSnapshots = nonNil(p.SourceSnapshots, base.sourceSnapshots)
	base.sourceFragments = nonNil(p.SourceFragments, base.sourceFragments)
	base.observations = nonNil(p.Observations, base.observations)
	base.claims = nonNil(p.Claims, base.claims)
	base.evidenceLinks = nonNil(p.EvidenceLinks, base.evidenceLinks)
	base.artifacts = nonNil(p.Artifacts, base.artifacts)
	base.rawModelOutputs = nonNil(p.RawModelOutputs, base.rawModelOutputs)
	base.proposedChanges = nonNil(p.ProposedChanges, base.proposedChanges)
	base.acceptedChanges = nonNil(p.AcceptedChanges, base.acceptedChanges)
	base.receipts = nonNil(p.Receipts, base.receipts)
	base.commitReceipts = nonNil(p.CommitReceipts, base.commitReceipts)
	base.commits = nonNil(p.Commits, base.commits)
	base.commitByIntent = nonNil(p.CommitByIntent, base.commitByIntent)
	base.headCommits = nonNil(p.HeadCommits, base.headCommits)
	base.canonical = nonNil(p.Canonical, base.canonical)
	base.hasControlState = p.HasControlState
	base.controlState = p.ControlState
	base.operatorCommands = nonNil(p.OperatorCommands, base.operatorCommands)
	base.operatorCommandByIdem = nonNil(p.OperatorCommandByIdem, base.operatorCommandByIdem)
	base.operatorCommandReceipts = nonNil(p.OperatorCommandReceipts, base.operatorCommandReceipts)
	base.externalEvents = nonNil(p.ExternalEvents, base.externalEvents)
	base.externalEventByDedup = nonNil(p.ExternalEventByDedup, base.externalEventByDedup)
	base.externalEventDispositions = nonNil(p.ExternalEventDispositions, base.externalEventDispositions)
	base.workOpportunities = nonNil(p.WorkOpportunities, base.workOpportunities)
	base.continuityDiagnoses = nonNil(p.ContinuityDiagnoses, base.continuityDiagnoses)
	base.configDrafts = nonNil(p.ConfigDrafts, base.configDrafts)
	base.configRevisions = nonNil(p.ConfigRevisions, base.configRevisions)
	base.activeConfig = nonNil(p.ActiveConfig, base.activeConfig)
	base.configApplyReceipts = nonNil(p.ConfigApplyReceipts, base.configApplyReceipts)
	base.channelCursors = nonNil(p.ChannelCursors, base.channelCursors)
	return &Store{state: base}, nil
}

func nonNil[K comparable, V any](got, fallback map[K]V) map[K]V {
	if got == nil {
		return fallback
	}
	return got
}
