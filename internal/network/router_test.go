package network

import (
	"context"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
)

type transportFunc func(context.Context, domain.PeerRecord, domain.PeerRPCRequest) (domain.PeerRPCResponse, error)

func (f transportFunc) Invoke(ctx context.Context, peer domain.PeerRecord, request domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
	return f(ctx, peer, request)
}

func newTestRouter(t *testing.T, transport transportFunc) *Router {
	t.Helper()
	registry, err := NewStaticRegistry(domain.PeerRegistryPolicy{MaxPeers: 2, EvictionTimeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(context.Background(), testPeer("node-a")); err != nil {
		t.Fatal(err)
	}
	router, err := NewRouter(registry, transport)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func TestRouterResolvesAuthorizesAndIsolatesRPC(t *testing.T) {
	requestPayload := []byte("request")
	remotePayload := []byte("response")
	router := newTestRouter(t, func(_ context.Context, peer domain.PeerRecord, request domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
		if peer.Identity.ID != "node-a" || request.Capability != "subagent.spawn" {
			t.Fatalf("unexpected route: %#v %#v", peer, request)
		}
		request.Payload[0] = 'X'
		return domain.PeerRPCResponse{RequestID: request.RequestID, PeerID: peer.Identity.ID, Payload: remotePayload}, nil
	})

	response, err := router.Call(context.Background(), domain.PeerRPCRequest{
		RequestID: "request-1", PeerID: "node-a", Capability: "subagent.spawn", Payload: requestPayload,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(requestPayload) != "request" {
		t.Fatal("transport mutated caller request")
	}
	remotePayload[0] = 'X'
	if string(response.Payload) != "response" {
		t.Fatal("response leaked transport-owned bytes")
	}
}

func TestRouterFailsClosedBeforeTransport(t *testing.T) {
	calls := 0
	router := newTestRouter(t, func(_ context.Context, _ domain.PeerRecord, request domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
		calls++
		return domain.PeerRPCResponse{RequestID: request.RequestID, PeerID: request.PeerID}, nil
	})

	for _, request := range []domain.PeerRPCRequest{
		{RequestID: "", PeerID: "node-a", Capability: "subagent.spawn"},
		{RequestID: "request-1", PeerID: "node-a", Capability: "missing"},
		{RequestID: "request-2", PeerID: "missing", Capability: "subagent.spawn"},
		{RequestID: "request-3", PeerID: "node-a", Capability: "subagent.spawn", Payload: make([]byte, maxPeerRPCPayload+1)},
	} {
		if _, err := router.Call(context.Background(), request); err == nil {
			t.Fatalf("request unexpectedly accepted: %#v", request)
		}
	}
	if calls != 0 {
		t.Fatalf("transport calls = %d, want 0", calls)
	}
}

func TestRouterRejectsMismatchedResponseAndPropagatesCancellation(t *testing.T) {
	router := newTestRouter(t, func(_ context.Context, _ domain.PeerRecord, request domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
		return domain.PeerRPCResponse{RequestID: request.RequestID + "-wrong", PeerID: request.PeerID}, nil
	})
	if _, err := router.Call(context.Background(), domain.PeerRPCRequest{RequestID: "request-1", PeerID: "node-a", Capability: "subagent.spawn"}); !errors.Is(err, ErrInvalidRPCResponse) {
		t.Fatalf("response error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := router.Call(ctx, domain.PeerRPCRequest{RequestID: "request-2", PeerID: "node-a", Capability: "subagent.spawn"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error = %v", err)
	}
}
