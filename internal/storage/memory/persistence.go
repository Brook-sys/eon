package memory

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"errors"
	"fmt"
	"io"

	"motor-autonomo/internal/domain"
)

const (
	checkpointFormatV1      = 1
	CheckpointFormatVersion = 2
)

var ErrUnsupportedCheckpointFormat = errors.New("unsupported checkpoint format version")
var ErrCheckpointIntegrity = errors.New("checkpoint integrity verification failed")
var ErrCheckpointFraming = errors.New("checkpoint contains trailing or malformed gob data")
var ErrCheckpointFormatMismatch = errors.New("checkpoint table and payload format versions disagree")

type checkpointEnvelope struct {
	FormatVersion int
	PayloadDigest [sha256.Size]byte
	Payload       []byte
}

type checkpointEnvelopeV1 struct {
	FormatVersion int
	State         persistedState
}

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
	Memories                  map[string]domain.LongTermMemory
	Events                    []domain.Event
	EventIDs                  map[domain.EventID]uint64
	PeerSyncInbox             map[string]domain.PeerSyncInboxRecord
	PeerSyncCursors           map[string]domain.PeerSyncCursor
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
	SubagentRecords           map[string]domain.SubagentRecord
	ConfigDrafts              map[domain.ConfigDraftID]domain.ConfigDraft
	ConfigRevisions           map[domain.ConfigRevisionID]domain.ConfigRevision
	ActiveConfig              map[domain.ConfigScope]domain.ConfigRevisionID
	ConfigApplyReceipts       map[domain.ConfigDraftID]domain.ConfigApplyReceipt
	ChannelCursors            map[string]domain.ChannelCursor
	ResourceUsages            map[domain.ResourceID]domain.ResourceUsage
	ModelContextPressures     map[string]domain.ModelContextPressure
}

// MarshalBinary returns an isolated, versioned checkpoint of the reference
// store. NewFromBinary still accepts the legacy unwrapped v0 gob so existing
// MVP databases can be upgraded on their next successful write.
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
		Memories: cloned.memories, Events: cloned.events, EventIDs: cloned.eventIDs,
		PeerSyncInbox: cloned.peerSyncInbox, PeerSyncCursors: cloned.peerSyncCursors, Idempotency: cloned.idempotency,
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
		SubagentRecords: cloned.subagentRecords,
		ConfigDrafts:    cloned.configDrafts, ConfigRevisions: cloned.configRevisions,
		ActiveConfig: cloned.activeConfig, ConfigApplyReceipts: cloned.configApplyReceipts,
		ChannelCursors:        cloned.channelCursors,
		ResourceUsages:        cloned.resourceUsages,
		ModelContextPressures: cloned.modelContextPressures,
	}
	var state bytes.Buffer
	if err := gob.NewEncoder(&state).Encode(p); err != nil {
		return nil, fmt.Errorf("encode memory checkpoint state: %w", err)
	}
	payload := state.Bytes()
	envelope := checkpointEnvelope{
		FormatVersion: CheckpointFormatVersion,
		PayloadDigest: sha256.Sum256(payload),
		Payload:       payload,
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(envelope); err != nil {
		return nil, fmt.Errorf("encode memory checkpoint: %w", err)
	}
	return buf.Bytes(), nil
}

// NewFromBinary restores a checkpoint produced by MarshalBinary.
func NewFromBinary(data []byte) (*Store, error) {
	var envelope checkpointEnvelope
	envelopeErr := decodeSingleGob(data, &envelope)
	if envelope.FormatVersion != 0 {
		if envelopeErr != nil {
			return nil, fmt.Errorf("decode memory checkpoint envelope: %w", envelopeErr)
		}
		switch envelope.FormatVersion {
		case CheckpointFormatVersion:
			if len(envelope.Payload) == 0 || sha256.Sum256(envelope.Payload) != envelope.PayloadDigest {
				return nil, ErrCheckpointIntegrity
			}
			var p persistedState
			if err := decodeSingleGob(envelope.Payload, &p); err != nil {
				return nil, fmt.Errorf("decode memory checkpoint state: %w", err)
			}
			return newFromPersistedState(p)
		case checkpointFormatV1:
			var legacy checkpointEnvelopeV1
			if err := decodeSingleGob(data, &legacy); err != nil {
				return nil, fmt.Errorf("decode v1 memory checkpoint: %w", err)
			}
			return newFromPersistedState(legacy.State)
		default:
			return nil, fmt.Errorf("%w: got %d, support %d", ErrUnsupportedCheckpointFormat, envelope.FormatVersion, CheckpointFormatVersion)
		}
	}

	// Compatibility path for checkpoints written before the envelope existed.
	var p persistedState
	if err := decodeSingleGob(data, &p); err != nil {
		return nil, fmt.Errorf("decode memory checkpoint: %w", err)
	}
	return newFromPersistedState(p)
}

// ValidateExternalCheckpoint verifies that the durable table-level version
// agrees with the payload's own framing before restore. External v1 remains a
// migration umbrella for the legacy unwrapped payload (v0) and v1 envelope;
// external v2 requires the integrity-protected v2 envelope.
func ValidateExternalCheckpoint(version int, data []byte) error {
	if !SupportsExternalCheckpointFormat(version) {
		return fmt.Errorf("%w: got %d, support %d", ErrUnsupportedCheckpointFormat, version, CheckpointFormatVersion)
	}
	detected, err := checkpointPayloadFormat(data)
	if err != nil {
		return err
	}
	compatible := version == CheckpointFormatVersion && detected == CheckpointFormatVersion ||
		version == checkpointFormatV1 && (detected == 0 || detected == checkpointFormatV1)
	if !compatible {
		return fmt.Errorf("%w: table=%d payload=%d", ErrCheckpointFormatMismatch, version, detected)
	}
	return nil
}

func checkpointPayloadFormat(data []byte) (int, error) {
	var envelope checkpointEnvelope
	envelopeErr := decodeSingleGob(data, &envelope)
	if envelope.FormatVersion != 0 {
		if envelopeErr != nil {
			return 0, fmt.Errorf("decode checkpoint envelope: %w", envelopeErr)
		}
		return envelope.FormatVersion, nil
	}
	var legacy persistedState
	if err := decodeSingleGob(data, &legacy); err != nil {
		return 0, fmt.Errorf("decode checkpoint framing: %w", err)
	}
	return 0, nil
}

func decodeSingleGob(data []byte, target any) error {
	decoder := gob.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("%w: %v", ErrCheckpointFraming, err)
	}
	return ErrCheckpointFraming
}

// SupportsExternalCheckpointFormat reports whether a durable adapter may pass
// a checkpoint with this table-level version to NewFromBinary. Version 1 is
// retained because pre-envelope databases already wrote format_version=1 and
// may contain either the unwrapped payload or the v1 envelope.
func SupportsExternalCheckpointFormat(version int) bool {
	return version == checkpointFormatV1 || version == CheckpointFormatVersion
}

func newFromPersistedState(p persistedState) (*Store, error) {
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
	base.memories = nonNil(p.Memories, base.memories)
	base.events = append([]domain.Event(nil), p.Events...)
	base.eventIDs = nonNil(p.EventIDs, base.eventIDs)
	base.peerSyncInbox = nonNil(p.PeerSyncInbox, base.peerSyncInbox)
	base.peerSyncCursors = nonNil(p.PeerSyncCursors, base.peerSyncCursors)
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
	base.subagentRecords = nonNil(p.SubagentRecords, base.subagentRecords)
	base.configDrafts = nonNil(p.ConfigDrafts, base.configDrafts)
	base.configRevisions = nonNil(p.ConfigRevisions, base.configRevisions)
	base.activeConfig = nonNil(p.ActiveConfig, base.activeConfig)
	base.configApplyReceipts = nonNil(p.ConfigApplyReceipts, base.configApplyReceipts)
	base.channelCursors = nonNil(p.ChannelCursors, base.channelCursors)
	base.resourceUsages = nonNil(p.ResourceUsages, base.resourceUsages)
	base.modelContextPressures = nonNil(p.ModelContextPressures, base.modelContextPressures)
	return &Store{state: base}, nil
}

func nonNil[K comparable, V any](got, fallback map[K]V) map[K]V {
	if got == nil {
		return fallback
	}
	return got
}
