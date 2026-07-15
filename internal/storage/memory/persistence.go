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
	MissionRevisions map[domain.MissionRevisionID]domain.MissionRevision
	ActiveMissions   map[domain.MissionID]domain.MissionRevisionID
	OperationSpecs   map[domain.OperationSpecID]domain.OperationSpec
	Questions        map[domain.QuestionID]domain.Question
	Candidates       map[domain.InquiryCandidateID]domain.InquiryCandidate
	Inquiries        map[domain.InquiryID]domain.Inquiry
	Operations       map[domain.OperationID]domain.Operation
	Rests            map[domain.MissionRevisionID]domain.Rest
	Events           []domain.Event
	EventIDs         map[domain.EventID]uint64
	Idempotency      map[domain.IdempotencyKey]domain.IdempotencyRecord
	Sources          map[domain.SourceID]domain.Source
	SourceVersions   map[domain.SourceVersionID]domain.SourceVersion
	SourceSnapshots  map[domain.SourceVersionID]domain.SourceSnapshot
	SourceFragments  map[domain.SourceFragmentID]domain.SourceFragment
	Observations     map[domain.ObservationID]domain.Observation
	Claims           map[domain.ClaimID]domain.Claim
	EvidenceLinks    map[domain.EvidenceLinkID]domain.EvidenceLink
	Artifacts        map[domain.ArtifactID]domain.KnowledgeArtifact
	RawModelOutputs  map[domain.ArtifactID]domain.RawModelOutput
	ProposedChanges  map[domain.ChangeSetID]domain.ProposedChangeSet
	AcceptedChanges  map[domain.ChangeSetID]domain.AcceptedChangeSet
	Receipts         map[domain.ReceiptID]domain.ValidationReceipt
	CommitReceipts   map[domain.ReceiptID]domain.CommitReceipt
	Commits          map[domain.CommitID]domain.Commit
	CommitByIntent   map[domain.IdempotencyKey]domain.CommitID
	HeadCommits      map[domain.MissionRevisionID]domain.CommitID
	Canonical        map[string]domain.CanonicalEntity
}

// MarshalBinary returns an isolated checkpoint of the reference store.
func (s *Store) MarshalBinary() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cloned := cloneState(s.state)
	p := persistedState{
		MissionRevisions: cloned.missionRevisions, ActiveMissions: cloned.activeMissions,
		OperationSpecs: cloned.operationSpecs, Questions: cloned.questions, Candidates: cloned.candidates,
		Inquiries: cloned.inquiries, Operations: cloned.operations, Rests: cloned.rests,
		Events: cloned.events, EventIDs: cloned.eventIDs, Idempotency: cloned.idempotency,
		Sources: cloned.sources, SourceVersions: cloned.sourceVersions, SourceSnapshots: cloned.sourceSnapshots,
		SourceFragments: cloned.sourceFragments, Observations: cloned.observations, Claims: cloned.claims,
		EvidenceLinks: cloned.evidenceLinks, Artifacts: cloned.artifacts, RawModelOutputs: cloned.rawModelOutputs,
		ProposedChanges: cloned.proposedChanges, AcceptedChanges: cloned.acceptedChanges, Receipts: cloned.receipts,
		CommitReceipts: cloned.commitReceipts, Commits: cloned.commits, CommitByIntent: cloned.commitByIntent,
		HeadCommits: cloned.headCommits, Canonical: cloned.canonical,
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
	base.candidates = nonNil(p.Candidates, base.candidates)
	base.inquiries = nonNil(p.Inquiries, base.inquiries)
	base.operations = nonNil(p.Operations, base.operations)
	base.rests = nonNil(p.Rests, base.rests)
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
	return &Store{state: base}, nil
}

func nonNil[K comparable, V any](got, fallback map[K]V) map[K]V {
	if got == nil {
		return fallback
	}
	return got
}
