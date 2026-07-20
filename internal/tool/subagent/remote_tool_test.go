package subagent

import (
	"context"
	"errors"
	"testing"

	"motor-autonomo/internal/domain"
)

type dummyIDGen struct {
	id  string
	err error
}

func (g *dummyIDGen) NewID(prefix string) (string, error) {
	return g.id, g.err
}

func TestRemoteTool_Execute(t *testing.T) {
	ctx := context.Background()
	idGen := &dummyIDGen{id: "req-xyz"}

	t.Run("success", func(t *testing.T) {
		caller := &dummyPeerCaller{
			t: t,
			resp: domain.PeerRPCResponse{
				RequestID: "req-xyz",
				PeerID:    "peer-456",
				Payload:   []byte(`{"result":"done"}`),
			},
		}
		delegator := &SubagentDelegator{Caller: caller}
		tool := &RemoteTool{
			Delegator: delegator,
			CallerID:  "my-agent-1",
			IDGen:     idGen,
		}

		def := tool.Definition()
		if def.Name != "sessions_spawn_remote" {
			t.Errorf("unexpected tool name: %s", def.Name)
		}

		input := []byte(`{"peer_id":"peer-456","capability":"do_work","payload":{"target":"sys"}}`)
		res, err := tool.Execute(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if res != `{"result":"done"}` {
			t.Errorf("unexpected result: %s", res)
		}
		if caller.req.RequestID != "req-xyz" {
			t.Errorf("unexpected request ID generated: %s", caller.req.RequestID)
		}
		if caller.req.PeerID != "peer-456" {
			t.Errorf("unexpected peer ID: %s", caller.req.PeerID)
		}
		if caller.req.CallerID != "my-agent-1" {
			t.Errorf("unexpected caller ID: %s", caller.req.CallerID)
		}
		if caller.req.Capability != "do_work" {
			t.Errorf("unexpected capability: %s", caller.req.Capability)
		}
		if string(caller.req.Payload) != `{"target":"sys"}` {
			t.Errorf("unexpected payload: %s", caller.req.Payload)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		tool := &RemoteTool{}
		input := []byte(`{bad json}`)
		_, err := tool.Execute(ctx, input)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("missing required fields", func(t *testing.T) {
		tool := &RemoteTool{}
		input := []byte(`{"peer_id":"peer-456","capability":""}`)
		_, err := tool.Execute(ctx, input)
		if err == nil || err.Error() != "missing required fields in input" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("id generation failure", func(t *testing.T) {
		tool := &RemoteTool{
			IDGen: &dummyIDGen{err: errors.New("rng error")},
		}
		input := []byte(`{"peer_id":"peer-456","capability":"do_work","payload":{"a":1}}`)
		_, err := tool.Execute(ctx, input)
		if err == nil || err.Error() != "failed to generate request id: rng error" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("delegation failure", func(t *testing.T) {
		caller := &dummyPeerCaller{
			t:   t,
			err: errors.New("timeout"),
		}
		delegator := &SubagentDelegator{Caller: caller}
		tool := &RemoteTool{
			Delegator: delegator,
			CallerID:  "my-agent-1",
			IDGen:     idGen,
		}

		input := []byte(`{"peer_id":"peer-456","capability":"do_work","payload":{"a":1}}`)
		_, err := tool.Execute(ctx, input)
		if err == nil || err.Error() != "delegation failed: peer call failed: timeout" {
			t.Errorf("unexpected error: %v", err)
		}
	})
}
