package subagent

import (
	"context"
	"fmt"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// SubagentDelegator coordena a chamada de capability remota em rede P2P.
type SubagentDelegator struct {
	Caller port.PeerCaller
}

// Delegate submete o comando para o peer remoto via PeerCaller interface.
func (d *SubagentDelegator) Delegate(ctx context.Context, requestID, peerID, callerID, capName string, payload []byte) ([]byte, error) {
	if requestID == "" {
		return nil, fmt.Errorf("missing requestID")
	}
	if peerID == "" {
		return nil, fmt.Errorf("missing peerID")
	}
	if callerID == "" {
		return nil, fmt.Errorf("missing callerID")
	}
	if capName == "" {
		return nil, fmt.Errorf("missing capability name")
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("missing payload")
	}

	req := domain.PeerRPCRequest{
		RequestID:  requestID,
		PeerID:     peerID,
		CallerID:   callerID,
		Capability: capName,
		Payload:    payload,
	}

	resp, err := d.Caller.Call(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("peer call failed: %w", err)
	}

	if resp.RequestID != requestID {
		return nil, fmt.Errorf("response correlation mismatch: expected %s, got %s", requestID, resp.RequestID)
	}

	return resp.Payload, nil
}
