// Package memory implements deterministic, transactional in-memory storage.
package memory

import (
	"context"
	"fmt"
	"sync"

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
	questions        map[domain.QuestionID]domain.Question
	candidates       map[domain.InquiryCandidateID]domain.InquiryCandidate
	inquiries        map[domain.InquiryID]domain.Inquiry
	operations       map[domain.OperationID]domain.Operation
}

func New() *Store { return &Store{state: newState()} }

func newState() state {
	return state{
		missionRevisions: make(map[domain.MissionRevisionID]domain.MissionRevision),
		activeMissions:   make(map[domain.MissionID]domain.MissionRevisionID),
		questions:        make(map[domain.QuestionID]domain.Question),
		candidates:       make(map[domain.InquiryCandidateID]domain.InquiryCandidate),
		inquiries:        make(map[domain.InquiryID]domain.Inquiry),
		operations:       make(map[domain.OperationID]domain.Operation),
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
func (t transaction) InquiryCandidate(id domain.InquiryCandidateID) (domain.InquiryCandidate, error) {
	return reader(t).InquiryCandidate(id)
}
func (t transaction) Inquiry(id domain.InquiryID) (domain.Inquiry, error) {
	return reader(t).Inquiry(id)
}
func (t transaction) Operation(id domain.OperationID) (domain.Operation, error) {
	return reader(t).Operation(id)
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
	t.state.questions[v.ID] = v
	return nil
}
func (t transaction) CreateInquiryCandidate(v domain.InquiryCandidate) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate inquiry candidate: %w", err)
	}
	if _, ok := t.state.candidates[v.ID]; ok {
		return conflict("inquiry candidate", v.ID)
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
	t.state.operations[v.ID] = cloneOperation(v)
	return nil
}
func (t transaction) SaveInquiry(v domain.Inquiry) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate inquiry: %w", err)
	}
	if _, ok := t.state.inquiries[v.ID]; !ok {
		return notFound("inquiry", v.ID)
	}
	t.state.inquiries[v.ID] = v
	return nil
}
func (t transaction) SaveOperation(v domain.Operation) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("validate operation: %w", err)
	}
	if _, ok := t.state.operations[v.ID]; !ok {
		return notFound("operation", v.ID)
	}
	t.state.operations[v.ID] = cloneOperation(v)
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

var _ port.Store = (*Store)(nil)
