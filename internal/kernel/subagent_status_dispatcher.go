package kernel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

const subagentStatusCapability = "subagent.status.v1"

type outboundSubagentStatus struct {
	DeliveryID string       `json:"delivery_id"`
	SessionID  string       `json:"session_id"`
	Attempt    int          `json:"attempt"`
	State      SessionState `json:"state"`
	Result     string       `json:"result,omitempty"`
	Failure    string       `json:"failure,omitempty"`
}

type subagentStatusAcknowledgement struct {
	SessionID string       `json:"session_id"`
	State     SessionState `json:"state"`
}

// SubagentStatusDispatcher durably drains terminal receiver receipts back to
// their authenticated origin. Terminal receipts remain until an ACK is seen;
// replay is safe because origin status ingress is monotonic and replay-safe.
type SubagentStatusDispatcher struct {
	Store      port.Store
	Caller     port.PeerCaller
	Clock      interface{ Now() time.Time }
	Batch      int
	RPCTimeout time.Duration
}

func (d *SubagentStatusDispatcher) DispatchTerminal(ctx context.Context) (int, error) {
	if d == nil || d.Store == nil || d.Caller == nil || d.Clock == nil {
		return 0, errors.New("subagent status dispatcher dependencies are incomplete")
	}
	batch := d.Batch
	if batch <= 0 {
		batch = 4
	}
	timeout := d.RPCTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	var receipts []domain.SubagentSpawnReceipt
	if err := d.Store.View(ctx, func(r port.Reader) error {
		var err error
		receipts, err = r.TerminalUndeliveredSubagentSpawnReceipts(batch)
		return err
	}); err != nil {
		return 0, err
	}
	processed := 0
	for _, candidate := range receipts {
		var receipt domain.SubagentSpawnReceipt
		if err := d.Store.Update(ctx, func(tx port.Transaction) error {
			current, err := tx.SubagentSpawnReceipt(candidate.CallerPeerID, candidate.RequestID)
			if err != nil {
				return err
			}
			receipt, err = domain.BeginSubagentSpawnReceiptStatusDelivery(current, d.Clock.Now().UTC())
			if err != nil {
				return err
			}
			return tx.SaveSubagentSpawnReceipt(receipt, current.Status, current.UpdatedAt)
		}); err != nil {
			if errors.Is(err, port.ErrConflict) || errors.Is(err, domain.ErrInvalidSubagentSpawnRPC) {
				continue
			}
			return processed, err
		}
		observation := outboundSubagentStatus{DeliveryID: receipt.RequestID, SessionID: receipt.SourceSessionID, Attempt: receipt.Attempt}
		switch receipt.Status {
		case domain.SubagentSpawnReceiptComplete:
			observation.State, observation.Result = SessionStateComplete, receipt.Result
		case domain.SubagentSpawnReceiptFailed:
			observation.State, observation.Failure = SessionStateFailed, receipt.Failure
		default:
			continue
		}
		payload, err := json.Marshal(observation)
		if err != nil || len(payload) > domain.MaxSubagentSpawnPayloadBytes {
			return processed, fmt.Errorf("encode subagent status: %w", err)
		}
		rpcCtx, cancel := context.WithTimeout(ctx, timeout)
		requestID := derivedSubagentRPCRequestID("subagent-status", receipt.CallerPeerID, receipt.RequestID, receipt.SourceSessionID, fmt.Sprint(receipt.Attempt))
		response, callErr := d.Caller.Call(rpcCtx, domain.PeerRPCRequest{RequestID: requestID, PeerID: receipt.CallerPeerID, Capability: subagentStatusCapability, Payload: payload})
		cancel()
		if callErr != nil {
			// The origin may have accepted the observation before the transport
			// failed. Park the delivery rather than automatically replaying an
			// ambiguous effect; reconciliation can resolve it explicitly later.
			if err := d.Store.Update(ctx, func(tx port.Transaction) error {
				current, err := tx.SubagentSpawnReceipt(receipt.CallerPeerID, receipt.RequestID)
				if err != nil {
					return err
				}
				next, err := domain.MarkSubagentSpawnReceiptStatusEffectUnknown(current, d.Clock.Now().UTC())
				if err != nil {
					return err
				}
				return tx.SaveSubagentSpawnReceipt(next, current.Status, current.UpdatedAt)
			}); err != nil {
				return processed, err
			}
			processed++
			continue
		}
		decoder := json.NewDecoder(bytes.NewReader(response.Payload))
		decoder.DisallowUnknownFields()
		var ack subagentStatusAcknowledgement
		if err := decoder.Decode(&ack); err != nil || decoder.Decode(&struct{}{}) != io.EOF || ack.SessionID != receipt.SourceSessionID || ack.State != observation.State {
			return processed, fmt.Errorf("invalid subagent status acknowledgement")
		}
		if err := d.Store.Update(ctx, func(tx port.Transaction) error {
			current, err := tx.SubagentSpawnReceipt(receipt.CallerPeerID, receipt.RequestID)
			if err != nil {
				return err
			}
			next, err := domain.MarkSubagentSpawnReceiptStatusDelivered(current, d.Clock.Now().UTC())
			if err != nil {
				return err
			}
			return tx.SaveSubagentSpawnReceipt(next, current.Status, current.UpdatedAt)
		}); err != nil {
			if errors.Is(err, port.ErrConflict) {
				continue
			}
			return processed, err
		}
		processed++
	}
	return processed, nil
}
