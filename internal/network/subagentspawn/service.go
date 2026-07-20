package subagentspawn

import (
	"context"
	"encoding/json"
	"errors"

	"motor-autonomo/internal/domain"
)

const Capability = "subagent.spawn.v1"

var ErrInvalidRequest = domain.ErrInvalidSubagentSpawnRPC

type Request = domain.SubagentSpawnRequest
type Acknowledgement = domain.SubagentSpawnAcknowledgement

type remoteSpawnManager interface {
	AcceptRemoteSpawn(context.Context, string, domain.SubagentSpawnRequest) (domain.SubagentSpawnAcknowledgement, error)
}
type Service struct{ manager remoteSpawnManager }
type Handler interface {
	Handle(context.Context, string, []byte) ([]byte, error)
}

func NewService(manager remoteSpawnManager) (*Service, error) {
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
	ack, err := s.manager.AcceptRemoteSpawn(ctx, callerID, request)
	if err != nil {
		return nil, err
	}
	return json.Marshal(ack)
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
