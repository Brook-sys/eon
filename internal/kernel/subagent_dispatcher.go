package kernel

import (
	"context"
	"errors"
	"fmt"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// SubagentDispatcher drains a bounded durable outbox. A response that may have
// been lost after remote admission is never retried automatically.
type SubagentDispatcher struct {
	Store      port.Store
	Caller     port.PeerCaller
	Clock      interface{ Now() time.Time }
	Owner      string
	Batch      int
	Lease      time.Duration
	RetryDelay time.Duration
	RPCTimeout time.Duration
}

func (d *SubagentDispatcher) DispatchDue(ctx context.Context) (int, error) {
	if d == nil || d.Store == nil || d.Caller == nil || d.Clock == nil || d.Owner == "" {
		return 0, errors.New("subagent dispatcher dependencies are incomplete")
	}
	batch := d.Batch
	if batch <= 0 {
		batch = 4
	}
	lease := d.Lease
	if lease <= 0 {
		lease = 30 * time.Second
	}
	retryDelay := d.RetryDelay
	if retryDelay <= 0 {
		retryDelay = 15 * time.Second
	}
	timeout := d.RPCTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	now := d.Clock.Now().UTC()
	var due []domain.SubagentDispatch
	if err := d.Store.View(ctx, func(r port.Reader) error {
		var err error
		due, err = r.DueSubagentDispatches(now, batch)
		return err
	}); err != nil {
		return 0, err
	}

	processed := 0
	for _, candidate := range due {
		if err := ctx.Err(); err != nil {
			return processed, err
		}
		if candidate.Status == domain.SubagentDispatchLeased {
			if err := d.Store.Update(ctx, func(tx port.Transaction) error {
				current, err := tx.SubagentDispatch(candidate.RequestID)
				if err != nil {
					return err
				}
				next, err := domain.ReclaimExpiredSubagentDispatch(current, d.Clock.Now().UTC())
				if err != nil {
					return err
				}
				return tx.SaveSubagentDispatch(next, current.Status, current.SendAttempt)
			}); err != nil && !errors.Is(err, port.ErrConflict) {
				return processed, err
			}
			processed++
			continue
		}

		var leased domain.SubagentDispatch
		var record domain.SubagentRecord
		if err := d.Store.Update(ctx, func(tx port.Transaction) error {
			current, err := tx.SubagentDispatch(candidate.RequestID)
			if err != nil {
				return err
			}
			record, err = tx.SubagentRecord(current.SessionID)
			if err != nil {
				return err
			}
			if record.Attempt != current.Attempt || record.TransportPeerID != current.PeerID {
				return fmt.Errorf("dispatch generation no longer matches session")
			}
			leased, err = domain.LeaseSubagentDispatch(current, d.Owner, d.Clock.Now().UTC(), d.Clock.Now().UTC().Add(lease))
			if err != nil {
				return err
			}
			return tx.SaveSubagentDispatch(leased, current.Status, current.SendAttempt)
		}); err != nil {
			if errors.Is(err, port.ErrConflict) {
				continue
			}
			return processed, err
		}

		payload, err := domain.EncodeSubagentSpawnRequest(domain.SubagentSpawnRequest{RequestID: string(leased.RequestID), SessionID: leased.SessionID, Attempt: leased.Attempt, Task: record.Task, ContextMode: record.ContextMode})
		if err != nil {
			return processed, err
		}
		rpcCtx, cancel := context.WithTimeout(ctx, timeout)
		response, callErr := d.Caller.Call(rpcCtx, domain.PeerRPCRequest{RequestID: string(leased.RequestID), PeerID: leased.PeerID, Capability: "subagent.spawn.v1", Payload: payload})
		cancel()
		finished := d.Clock.Now().UTC()
		var ack domain.SubagentSpawnAcknowledgement
		if callErr == nil {
			var decodeErr error
			ack, decodeErr = domain.DecodeSubagentSpawnAcknowledgement(response.Payload)
			if decodeErr != nil || ack.RequestID != string(leased.RequestID) || ack.SessionID != leased.SessionID || ack.Attempt != leased.Attempt {
				callErr = domain.ErrInvalidSubagentSpawnRPC
			}
		}
		if err := d.Store.Update(ctx, func(tx port.Transaction) error {
			current, err := tx.SubagentDispatch(leased.RequestID)
			if err != nil {
				return err
			}
			var next domain.SubagentDispatch
			if callErr == nil && ack.Accepted {
				next, err = domain.CompleteSubagentDispatch(current, d.Owner, ack.ReceiverSessionID, finished)
			} else if callErr != nil {
				next, err = domain.MarkAmbiguousSubagentDispatch(current, d.Owner, finished)
			} else {
				code := ack.Code
				if !ack.Retryable {
					current.MaxSendAttempts = current.SendAttempt
				}
				next, err = domain.FailSubagentDispatch(current, d.Owner, code, finished, finished.Add(retryDelay))
			}
			if err != nil {
				return err
			}
			return tx.SaveSubagentDispatch(next, current.Status, current.SendAttempt)
		}); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}
