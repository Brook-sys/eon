// Package memory implements deterministic, transactional in-memory storage.
package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

type Store struct {
	mu    sync.RWMutex
	state state
}

type state struct {
	missionRevisions  map[domain.MissionRevisionID]domain.MissionRevision
	activeMissions    map[domain.MissionID]domain.MissionRevisionID
	operationSpecs    map[domain.OperationSpecID]domain.OperationSpec
	questions         map[domain.QuestionID]domain.Question
	operatorQuestions map[domain.OperatorQuestionID]domain.OperatorQuestion
	operatorAnswers   map[domain.OperatorAnswerID]domain.UserAnswer
	answerByTransport map[string]domain.OperatorAnswerID
	candidates        map[domain.InquiryCandidateID]domain.InquiryCandidate
	inquiries         map[domain.InquiryID]domain.Inquiry
	operations        map[domain.OperationID]domain.Operation
	events            []domain.Event
	eventIDs          map[domain.EventID]uint64
	idempotency       map[domain.IdempotencyKey]domain.IdempotencyRecord
	sources           map[domain.SourceID]domain.Source
	sourceVersions    map[domain.SourceVersionID]domain.SourceVersion
	sourceSnapshots   map[domain.SourceVersionID]domain.SourceSnapshot
	sourceFragments   map[domain.SourceFragmentID]domain.SourceFragment
	observations      map[domain.ObservationID]domain.Observation
	claims            map[domain.ClaimID]domain.Claim
	evidenceLinks     map[domain.EvidenceLinkID]domain.EvidenceLink
	artifacts         map[domain.ArtifactID]domain.KnowledgeArtifact
	rawModelOutputs   map[domain.ArtifactID]domain.RawModelOutput
	proposedChanges   map[domain.ChangeSetID]domain.ProposedChangeSet
	acceptedChanges   map[domain.ChangeSetID]domain.AcceptedChangeSet
	receipts          map[domain.ReceiptID]domain.ValidationReceipt
	commitReceipts    map[domain.ReceiptID]domain.CommitReceipt
	commits           map[domain.CommitID]domain.Commit
	commitByIntent    map[domain.IdempotencyKey]domain.CommitID
	headCommits       map[domain.MissionRevisionID]domain.CommitID
	canonical         map[string]domain.CanonicalEntity
}

func New() *Store { return &Store{state: newState()} }

func newState() state {
	return state{
		missionRevisions:  make(map[domain.MissionRevisionID]domain.MissionRevision),
		activeMissions:    make(map[domain.MissionID]domain.MissionRevisionID),
		operationSpecs:    make(map[domain.OperationSpecID]domain.OperationSpec),
		questions:         make(map[domain.QuestionID]domain.Question),
		operatorQuestions: make(map[domain.OperatorQuestionID]domain.OperatorQuestion),
		operatorAnswers:   make(map[domain.OperatorAnswerID]domain.UserAnswer),
		answerByTransport: make(map[string]domain.OperatorAnswerID),
		candidates:        make(map[domain.InquiryCandidateID]domain.InquiryCandidate),
		inquiries:         make(map[domain.InquiryID]domain.Inquiry),
		operations:        make(map[domain.OperationID]domain.Operation),
		eventIDs:          make(map[domain.EventID]uint64),
		idempotency:       make(map[domain.IdempotencyKey]domain.IdempotencyRecord),
		sources:           make(map[domain.SourceID]domain.Source),
		sourceVersions:    make(map[domain.SourceVersionID]domain.SourceVersion),
		sourceSnapshots:   make(map[domain.SourceVersionID]domain.SourceSnapshot),
		sourceFragments:   make(map[domain.SourceFragmentID]domain.SourceFragment),
		observations:      make(map[domain.ObservationID]domain.Observation),
		claims:            make(map[domain.ClaimID]domain.Claim),
		evidenceLinks:     make(map[domain.EvidenceLinkID]domain.EvidenceLink),
		artifacts:         make(map[domain.ArtifactID]domain.KnowledgeArtifact),
		rawModelOutputs:   make(map[domain.ArtifactID]domain.RawModelOutput),
		proposedChanges:   make(map[domain.ChangeSetID]domain.ProposedChangeSet),
		acceptedChanges:   make(map[domain.ChangeSetID]domain.AcceptedChangeSet),
		receipts:          make(map[domain.ReceiptID]domain.ValidationReceipt),
		commitReceipts:    make(map[domain.ReceiptID]domain.CommitReceipt),
		commits:           make(map[domain.CommitID]domain.Commit),
		commitByIntent:    make(map[domain.IdempotencyKey]domain.CommitID),
		headCommits:       make(map[domain.MissionRevisionID]domain.CommitID),
		canonical:         make(map[string]domain.CanonicalEntity),
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
func (t transaction) SourceVersion(id domain.SourceVersionID) (domain.SourceVersion, error) {
	return reader(t).SourceVersion(id)
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
func (t transaction) Claim(id domain.ClaimID) (domain.Claim, error) {
	return reader(t).Claim(id)
}
func (t transaction) EvidenceLink(id domain.EvidenceLinkID) (domain.EvidenceLink, error) {
	return reader(t).EvidenceLink(id)
}
func (t transaction) EvidenceLinksForClaim(id domain.ClaimID) ([]domain.EvidenceLink, error) {
	return reader(t).EvidenceLinksForClaim(id)
}
func (t transaction) KnowledgeArtifact(id domain.ArtifactID) (domain.KnowledgeArtifact, error) {
	return reader(t).KnowledgeArtifact(id)
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
func (r reader) SourceVersion(id domain.SourceVersionID) (domain.SourceVersion, error) {
	v, ok := r.state.sourceVersions[id]
	if !ok {
		return domain.SourceVersion{}, notFound("source version", id)
	}
	return v, nil
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
func (r reader) Claim(id domain.ClaimID) (domain.Claim, error) {
	v, ok := r.state.claims[id]
	if !ok {
		return domain.Claim{}, notFound("claim", id)
	}
	return cloneClaim(v), nil
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
func (r reader) KnowledgeArtifact(id domain.ArtifactID) (domain.KnowledgeArtifact, error) {
	v, ok := r.state.artifacts[id]
	if !ok {
		return domain.KnowledgeArtifact{}, notFound("knowledge artifact", id)
	}
	return cloneKnowledgeArtifact(v), nil
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

func cloneState(src state) state {
	dst := newState()
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
	for k, v := range src.candidates {
		dst.candidates[k] = cloneCandidate(v)
	}
	for k, v := range src.inquiries {
		dst.inquiries[k] = v
	}
	for k, v := range src.operations {
		dst.operations[k] = cloneOperation(v)
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
	return dst
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
