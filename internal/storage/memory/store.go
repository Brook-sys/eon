// Package memory implements deterministic, transactional in-memory storage.
package memory

import (
	"context"
	"fmt"
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
	missionRevisions map[domain.MissionRevisionID]domain.MissionRevision
	activeMissions   map[domain.MissionID]domain.MissionRevisionID
	operationSpecs   map[domain.OperationSpecID]domain.OperationSpec
	questions        map[domain.QuestionID]domain.Question
	candidates       map[domain.InquiryCandidateID]domain.InquiryCandidate
	inquiries        map[domain.InquiryID]domain.Inquiry
	operations       map[domain.OperationID]domain.Operation
	events           []domain.Event
	eventIDs         map[domain.EventID]uint64
	idempotency      map[domain.IdempotencyKey]domain.IdempotencyRecord
}

func New() *Store { return &Store{state: newState()} }

func newState() state {
	return state{
		missionRevisions: make(map[domain.MissionRevisionID]domain.MissionRevision),
		activeMissions:   make(map[domain.MissionID]domain.MissionRevisionID),
		operationSpecs:   make(map[domain.OperationSpecID]domain.OperationSpec),
		questions:        make(map[domain.QuestionID]domain.Question),
		candidates:       make(map[domain.InquiryCandidateID]domain.InquiryCandidate),
		inquiries:        make(map[domain.InquiryID]domain.Inquiry),
		operations:       make(map[domain.OperationID]domain.Operation),
		eventIDs:         make(map[domain.EventID]uint64),
		idempotency:      make(map[domain.IdempotencyKey]domain.IdempotencyRecord),
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
func (t transaction) Events(afterSequence uint64, limit int) ([]domain.Event, error) {
	return reader(t).Events(afterSequence, limit)
}
func (t transaction) EventByID(id domain.EventID) (domain.Event, error) {
	return reader(t).EventByID(id)
}
func (t transaction) IdempotencyRecord(key domain.IdempotencyKey) (domain.IdempotencyRecord, error) {
	return reader(t).IdempotencyRecord(key)
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
	return dst
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
