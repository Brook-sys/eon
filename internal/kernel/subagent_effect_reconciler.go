package kernel

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

const subagentReconcileCapability = "subagent.reconcile.v1"

// SubagentEffectReconciler resolves ambiguous remote effects only from positive
// durable evidence. Negative, conflicting, malformed, or unavailable evidence
// leaves the local row parked and never triggers retry or cancellation.
type SubagentEffectReconciler struct {
	Store       port.Store
	Caller      port.PeerCaller
	Clock       interface{ Now() time.Time }
	Batch       int
	RPCTimeout  time.Duration
	BackoffBase time.Duration
	BackoffMax  time.Duration
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
	now := r.Clock.Now().UTC()
	var dispatches []domain.SubagentDispatch
	var statuses []domain.SubagentSpawnReceipt
	if err := r.Store.View(ctx, func(reader port.Reader) error {
		var err error
		dispatches, err = reader.EffectUnknownSubagentDispatches(now, batch)
		if err != nil {
			return err
		}
		statuses, err = reader.SubagentStatusDeliveriesRequiringReconciliation(now, batch)
		return err
	}); err != nil {
		return 0, err
	}
	candidates := make([]subagentReconcileCandidate, 0, len(dispatches)+len(statuses))
	for i := range dispatches {
		candidates = append(candidates, subagentReconcileCandidate{kind: domain.SubagentReconcileSpawn, updatedAt: dispatches[i].UpdatedAt, key: string(dispatches[i].RequestID), dispatch: &dispatches[i]})
	}
	for i := range statuses {
		candidates = append(candidates, subagentReconcileCandidate{kind: domain.SubagentReconcileStatus, updatedAt: statuses[i].UpdatedAt, key: statuses[i].CallerPeerID + "\x00" + statuses[i].RequestID, status: &statuses[i]})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].updatedAt.Equal(candidates[j].updatedAt) {
			if candidates[i].kind == candidates[j].kind {
				return candidates[i].key < candidates[j].key
			}
			return candidates[i].kind < candidates[j].kind
		}
		return candidates[i].updatedAt.Before(candidates[j].updatedAt)
	})
	if len(candidates) > batch {
		candidates = candidates[:batch]
	}
	processed := 0
	for _, candidate := range candidates {
		if candidate.dispatch == nil {
			receipt := *candidate.status
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
				if err := r.deferStatus(ctx, receipt); err != nil && !errors.Is(err, port.ErrConflict) {
					return processed, err
				}
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
			continue
		}

		dispatch := *candidate.dispatch
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
			if err := r.deferDispatch(ctx, dispatch); err != nil && !errors.Is(err, port.ErrConflict) {
				return processed, err
			}
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
	return processed, nil
}

func (r *SubagentEffectReconciler) backoff(attempt uint32) time.Duration {
	base, maximum := r.BackoffBase, r.BackoffMax
	if base <= 0 {
		base = time.Second
	}
	if maximum <= 0 {
		maximum = time.Minute
	}
	delay := base
	for i := uint32(0); i < attempt && delay < maximum; i++ {
		if delay > maximum/2 {
			return maximum
		}
		delay *= 2
	}
	if delay > maximum {
		return maximum
	}
	return delay
}

func (r *SubagentEffectReconciler) deferDispatch(ctx context.Context, observed domain.SubagentDispatch) error {
	return r.Store.Update(ctx, func(tx port.Transaction) error {
		current, err := tx.SubagentDispatch(observed.RequestID)
		if err != nil {
			return err
		}
		if current.Status != domain.SubagentDispatchEffectUnknown {
			return port.ErrConflict
		}
		if !current.UpdatedAt.Equal(observed.UpdatedAt) || current.ReconcileAttempts != observed.ReconcileAttempts || !current.ReconcileAfter.Equal(observed.ReconcileAfter) {
			return port.ErrConflict
		}
		now := r.Clock.Now().UTC()
		next, err := domain.DeferSubagentDispatchReconciliation(current, now, now.Add(r.backoff(current.ReconcileAttempts)))
		if err != nil {
			return err
		}
		return tx.SaveSubagentDispatch(next, current.Status, current.SendAttempt)
	})
}

func (r *SubagentEffectReconciler) deferStatus(ctx context.Context, observed domain.SubagentSpawnReceipt) error {
	return r.Store.Update(ctx, func(tx port.Transaction) error {
		current, err := tx.SubagentSpawnReceipt(observed.CallerPeerID, observed.RequestID)
		if err != nil {
			return err
		}
		if current.StatusDelivery != observed.StatusDelivery || !current.UpdatedAt.Equal(observed.UpdatedAt) || current.ReconcileAttempts != observed.ReconcileAttempts || !current.ReconcileAfter.Equal(observed.ReconcileAfter) {
			return port.ErrConflict
		}
		now := r.Clock.Now().UTC()
		next, err := domain.DeferSubagentSpawnReceiptStatusReconciliation(current, now, now.Add(r.backoff(current.ReconcileAttempts)))
		if err != nil {
			return err
		}
		return tx.SaveSubagentSpawnReceipt(next, current.Status, current.UpdatedAt)
	})
}

type subagentReconcileCandidate struct {
	kind      domain.SubagentReconcileKind
	updatedAt time.Time
	key       string
	dispatch  *domain.SubagentDispatch
	status    *domain.SubagentSpawnReceipt
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
