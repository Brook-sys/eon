package subagentspawn

import (
	"context"
	"encoding/json"
	"errors"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
)

const Capability = "subagent.spawn.v1"

var ErrInvalidRequest = domain.ErrInvalidSubagentSpawnRPC

type Request = domain.SubagentSpawnRequest
type Acknowledgement = domain.SubagentSpawnAcknowledgement

type Service struct{ manager kernel.SessionManager }
type Handler interface {
	Handle(context.Context, string, []byte) ([]byte, error)
}

func NewService(manager kernel.SessionManager) (*Service, error) {
	if manager == nil {
		return nil, errors.New("subagent spawn manager is required")
	}
	return &Service{manager: manager}, nil
}

func (s *Service) Handle(ctx context.Context, callerID string, payload []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	request, err := domain.DecodeSubagentSpawnRequest(payload)
	if err != nil || !domain.ValidSubagentRPCField(callerID) {
		return nil, ErrInvalidRequest
	}
	id, err := s.manager.Spawn(ctx, kernel.SubagentSpec{Task: request.Task, ContextMode: request.ContextMode, Labels: map[string]string{"task_id": request.RequestID, "source_peer_id": callerID, "source_session_id": request.SessionID}})
	if err != nil {
		return nil, err
	}
	return json.Marshal(Acknowledgement{RequestID: request.RequestID, SessionID: request.SessionID, Attempt: request.Attempt, ReceiverSessionID: string(id), Accepted: true})
}

func EncodeRequest(request Request) ([]byte, error) {
	return domain.EncodeSubagentSpawnRequest(request)
}
func DecodeRequest(payload []byte) (Request, error) {
	return domain.DecodeSubagentSpawnRequest(payload)
}
func DecodeAcknowledgement(payload []byte) (Acknowledgement, error) {
	return domain.DecodeSubagentSpawnAcknowledgement(payload)
}
