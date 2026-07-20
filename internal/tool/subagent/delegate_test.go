package subagent

import (
	"context"
	"errors"
	"testing"

	"motor-autonomo/internal/domain"
)

type dummyPeerCaller struct {
	t        *testing.T
	req      *domain.PeerRPCRequest
	resp     domain.PeerRPCResponse
	err      error
}

func (d *dummyPeerCaller) Call(ctx context.Context, req domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
	d.req = &req
	return d.resp, d.err
}

func TestSubagentDelegator_Delegate(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		caller := &dummyPeerCaller{
			t: t,
			resp: domain.PeerRPCResponse{
				RequestID: "req-123",
				PeerID:    "peer123",
				Payload:   []byte(`{"status":"ok"}`),
			},
		}
		delegator := &SubagentDelegator{Caller: caller}

		payload, err := delegator.Delegate(ctx, "req-123", "peer123", "caller-1", "exec_tool", []byte(`{"arg":1}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(payload) != `{"status":"ok"}` {
			t.Errorf("unexpected payload: %s", payload)
		}
		if caller.req.RequestID != "req-123" {
			t.Errorf("unexpected request id: %s", caller.req.RequestID)
		}
		if caller.req.PeerID != "peer123" {
			t.Errorf("unexpected peer id: %s", caller.req.PeerID)
		}
		if caller.req.CallerID != "caller-1" {
			t.Errorf("unexpected caller id: %s", caller.req.CallerID)
		}
		if caller.req.Capability != "exec_tool" {
			t.Errorf("unexpected capability: %s", caller.req.Capability)
		}
		if string(caller.req.Payload) != `{"arg":1}` {
			t.Errorf("unexpected request payload: %s", caller.req.Payload)
		}
	})

	t.Run("correlation mismatch", func(t *testing.T) {
		caller := &dummyPeerCaller{
			t: t,
			resp: domain.PeerRPCResponse{
				RequestID: "req-999", // divergent
				PeerID:    "peer123",
				Payload:   []byte(`{"status":"ok"}`),
			},
		}
		delegator := &SubagentDelegator{Caller: caller}

		_, err := delegator.Delegate(ctx, "req-123", "peer123", "caller-1", "exec_tool", []byte(`{"arg":1}`))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if err.Error() != "response correlation mismatch: expected req-123, got req-999" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("caller error", func(t *testing.T) {
		caller := &dummyPeerCaller{
			t:   t,
			err: errors.New("network error"),
		}
		delegator := &SubagentDelegator{Caller: caller}

		_, err := delegator.Delegate(ctx, "req-123", "peer123", "caller-1", "exec_tool", []byte(`{"arg":1}`))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if err.Error() != "peer call failed: network error" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("missing parameters", func(t *testing.T) {
		caller := &dummyPeerCaller{t: t}
		delegator := &SubagentDelegator{Caller: caller}

		_, err := delegator.Delegate(ctx, "", "peer123", "caller-1", "exec_tool", []byte(`{"arg":1}`))
		if err == nil || err.Error() != "missing requestID" {
			t.Errorf("unexpected error: %v", err)
		}

		_, err = delegator.Delegate(ctx, "req-123", "", "caller-1", "exec_tool", []byte(`{"arg":1}`))
		if err == nil || err.Error() != "missing peerID" {
			t.Errorf("unexpected error: %v", err)
		}

		_, err = delegator.Delegate(ctx, "req-123", "peer123", "", "exec_tool", []byte(`{"arg":1}`))
		if err == nil || err.Error() != "missing callerID" {
			t.Errorf("unexpected error: %v", err)
		}

		_, err = delegator.Delegate(ctx, "req-123", "peer123", "caller-1", "", []byte(`{"arg":1}`))
		if err == nil || err.Error() != "missing capability name" {
			t.Errorf("unexpected error: %v", err)
		}

		_, err = delegator.Delegate(ctx, "req-123", "peer123", "caller-1", "exec_tool", nil)
		if err == nil || err.Error() != "missing payload" {
			t.Errorf("unexpected error: %v", err)
		}
	})
}
