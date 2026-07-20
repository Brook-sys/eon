package subagentreconcile

import (
	"context"
	"errors"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

const Capability = "subagent.reconcile.v1"

type Service struct{ store port.Store }
type Handler interface {
	Handle(context.Context, string, []byte) ([]byte, error)
}

func NewService(store port.Store) (*Service, error) {
	if store == nil {
		return nil, errors.New("subagent reconcile store is required")
	}
	return &Service{store: store}, nil
}

func (s *Service) Handle(ctx context.Context, callerID string, payload []byte) ([]byte, error) {
	if !domain.ValidSubagentRPCField(callerID) {
		return nil, domain.ErrInvalidSubagentReconcileRPC
	}
	request, err := domain.DecodeSubagentReconcileRequest(payload)
	if err != nil {
		return nil, err
	}
	response := domain.SubagentReconcileResponse{Kind: request.Kind, DeliveryID: request.DeliveryID, SessionID: request.SessionID, Attempt: request.Attempt, State: domain.SubagentReconcileNotFound}
	err = s.store.View(ctx, func(r port.Reader) error {
		switch request.Kind {
		case domain.SubagentReconcileSpawn:
			receipt, getErr := r.SubagentSpawnReceipt(callerID, request.DeliveryID)
			if errors.Is(getErr, port.ErrNotFound) {
				return nil
			}
			if getErr != nil {
				return getErr
			}
			digest, digestErr := domain.SubagentSpawnRequestDigest(domain.SubagentSpawnRequest{RequestID: receipt.RequestID, SessionID: receipt.SourceSessionID, Attempt: receipt.Attempt, Task: receipt.Task, ContextMode: receipt.ContextMode})
			if digestErr != nil {
				return digestErr
			}
			if receipt.SourceSessionID != request.SessionID || receipt.Attempt != request.Attempt || digest != request.Digest {
				response.State = domain.SubagentReconcileConflict
				return nil
			}
			response.State, response.ReceiverSessionID = domain.SubagentReconcileFound, receipt.ReceiverSessionID
		case domain.SubagentReconcileStatus:
			receipt, getErr := r.SubagentStatusIngressReceipt(callerID, request.DeliveryID)
			if errors.Is(getErr, port.ErrNotFound) {
				return nil
			}
			if getErr != nil {
				return getErr
			}
			digest, digestErr := domain.SubagentTerminalStatusDigest(domain.SubagentTerminalStatus{DeliveryID: receipt.DeliveryID, SessionID: receipt.SessionID, Attempt: receipt.Attempt, State: receipt.State, Result: receipt.Result, Failure: receipt.Failure})
			if digestErr != nil {
				return digestErr
			}
			if receipt.SessionID != request.SessionID || receipt.Attempt != request.Attempt || digest != request.Digest {
				response.State = domain.SubagentReconcileConflict
				return nil
			}
			response.State = domain.SubagentReconcileFound
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return domain.EncodeSubagentReconcileResponse(response)
}
