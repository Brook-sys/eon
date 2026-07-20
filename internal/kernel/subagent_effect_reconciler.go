package kernel

import (
	"context"
	"errors"
	"fmt"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

const subagentReconcileCapability = "subagent.reconcile.v1"

// SubagentEffectReconciler resolves ambiguous remote effects only from positive
// durable evidence. Negative, conflicting, malformed, or unavailable evidence
// leaves the local row parked and never triggers retry or cancellation.
type SubagentEffectReconciler struct {
	Store      port.Store
	Caller     port.PeerCaller
	Clock      interface{ Now() time.Time }
	Batch      int
	RPCTimeout time.Duration
}

func (r *SubagentEffectReconciler) Reconcile(ctx context.Context) (int, error) {
	if r == nil || r.Store == nil || r.Caller == nil || r.Clock == nil {
		return 0, errors.New("subagent effect reconciler dependencies are incomplete")
	}
	batch := r.Batch
	if batch <= 0 {
		batch = 4
	}
	timeout := r.RPCTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	var dispatches []domain.SubagentDispatch
	var statuses []domain.SubagentSpawnReceipt
	if err := r.Store.View(ctx, func(reader port.Reader) error {
		var err error
		dispatches, err = reader.EffectUnknownSubagentDispatches(batch)
		if err != nil {
			return err
		}
		remaining := batch - len(dispatches)
		if remaining > 0 {
			statuses, err = reader.SubagentStatusDeliveriesRequiringReconciliation(remaining)
		}
		return err
	}); err != nil {
		return 0, err
	}
	processed := 0
	for _, dispatch := range dispatches {
		var record domain.SubagentRecord
		if err := r.Store.View(ctx, func(reader port.Reader) error {
			var err error
			record, err = reader.SubagentRecord(dispatch.SessionID)
			return err
		}); err != nil {
			return processed, err
		}
		digest, err := domain.SubagentSpawnRequestDigest(domain.SubagentSpawnRequest{RequestID: string(dispatch.RequestID), SessionID: dispatch.SessionID, Attempt: dispatch.Attempt, Task: record.Task, ContextMode: record.ContextMode})
		if err != nil {
			return processed, err
		}
		request := domain.SubagentReconcileRequest{Kind: domain.SubagentReconcileSpawn, DeliveryID: string(dispatch.RequestID), SessionID: dispatch.SessionID, Attempt: dispatch.Attempt, Digest: digest}
		response, ok := r.lookup(ctx, timeout, dispatch.PeerID, request)
		if !ok || response.State != domain.SubagentReconcileFound {
			continue
		}
		if err := r.Store.Update(ctx, func(tx port.Transaction) error {
			current, err := tx.SubagentDispatch(dispatch.RequestID)
			if err != nil {
				return err
			}
			if current.Status != domain.SubagentDispatchEffectUnknown {
				return port.ErrConflict
			}
			next, err := domain.CompleteSubagentDispatchAfterReconcile(current, response.ReceiverSessionID, r.Clock.Now().UTC())
			if err != nil {
				return err
			}
			return tx.SaveSubagentDispatch(next, current.Status, current.SendAttempt)
		}); err != nil && !errors.Is(err, port.ErrConflict) {
			return processed, err
		}
		processed++
	}
	for _, receipt := range statuses {
		state := "COMPLETE"
		if receipt.Status == domain.SubagentSpawnReceiptFailed {
			state = "FAILED"
		}
		digest, err := domain.SubagentTerminalStatusDigest(domain.SubagentTerminalStatus{DeliveryID: receipt.RequestID, SessionID: receipt.SourceSessionID, Attempt: receipt.Attempt, State: state, Result: receipt.Result, Failure: receipt.Failure})
		if err != nil {
			return processed, err
		}
		request := domain.SubagentReconcileRequest{Kind: domain.SubagentReconcileStatus, DeliveryID: receipt.RequestID, SessionID: receipt.SourceSessionID, Attempt: receipt.Attempt, Digest: digest}
		response, ok := r.lookup(ctx, timeout, receipt.CallerPeerID, request)
		if !ok || response.State != domain.SubagentReconcileFound {
			continue
		}
		if err := r.Store.Update(ctx, func(tx port.Transaction) error {
			current, err := tx.SubagentSpawnReceipt(receipt.CallerPeerID, receipt.RequestID)
			if err != nil {
				return err
			}
			if current.StatusDelivery != domain.SubagentStatusDeliveryEffectUnknown && current.StatusDelivery != domain.SubagentStatusDeliveryInFlight {
				return port.ErrConflict
			}
			next, err := domain.ResolveSubagentSpawnReceiptStatusFound(current, r.Clock.Now().UTC())
			if err != nil {
				return err
			}
			return tx.SaveSubagentSpawnReceipt(next, current.Status, current.UpdatedAt)
		}); err != nil && !errors.Is(err, port.ErrConflict) {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func (r *SubagentEffectReconciler) lookup(ctx context.Context, timeout time.Duration, peerID string, request domain.SubagentReconcileRequest) (domain.SubagentReconcileResponse, bool) {
	payload, err := domain.EncodeSubagentReconcileRequest(request)
	if err != nil {
		return domain.SubagentReconcileResponse{}, false
	}
	rpcCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	response, err := r.Caller.Call(rpcCtx, domain.PeerRPCRequest{RequestID: fmt.Sprintf("reconcile:%s:%s", request.Kind, request.DeliveryID), PeerID: peerID, Capability: subagentReconcileCapability, Payload: payload})
	if err != nil {
		return domain.SubagentReconcileResponse{}, false
	}
	decoded, err := domain.DecodeSubagentReconcileResponse(response.Payload)
	if err != nil || decoded.Kind != request.Kind || decoded.DeliveryID != request.DeliveryID || decoded.SessionID != request.SessionID || decoded.Attempt != request.Attempt {
		return domain.SubagentReconcileResponse{}, false
	}
	return decoded, true
}
