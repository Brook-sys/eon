package network

import (
	"context"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/network/subagentstatus"
	peersync "motor-autonomo/internal/network/sync"
	"motor-autonomo/internal/storage/memory"
)

type routerIDs struct{ n int }

func (i *routerIDs) NewID(prefix string) (string, error) { i.n++; return prefix + "-id", nil }

type transportFunc func(context.Context, domain.PeerRecord, domain.PeerRPCRequest) (domain.PeerRPCResponse, error)

func (f transportFunc) Invoke(ctx context.Context, peer domain.PeerRecord, request domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
	return f(ctx, peer, request)
}

func TestRouterDispatchesAuthenticatedSubagentStatusWithoutOutboundTransport(t *testing.T) {
	transportCalls := 0
	router := newTestRouter(t, func(_ context.Context, _ domain.PeerRecord, _ domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
		transportCalls++
		return domain.PeerRPCResponse{}, nil
	})
	local := kernel.NewLocalSessionManager(routerClock{})
	store := memory.New()
	manager, err := kernel.NewPersistentSessionManager(local, store, routerClock{}, &routerIDs{}, kernel.PersistentSessionPolicy{MissionID: "mission-1", MaxAttempts: 2, Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	id, err := manager.Spawn(context.Background(), kernel.SubagentSpec{Task: "remote work", ContextMode: "isolated", Labels: map[string]string{subagentstatus.TransportPeerKey: "node-a"}})
	if err != nil {
		t.Fatal(err)
	}
	service, err := subagentstatus.NewService(manager)
	if err != nil {
		t.Fatal(err)
	}
	if err := router.AttachSubagentStatuses(service); err != nil {
		t.Fatal(err)
	}
	payload, err := subagentstatus.Encode(subagentstatus.Observation{DeliveryID: "delivery-1", SessionID: string(id), Attempt: 0, State: kernel.SessionStateComplete, Result: "done"})
	if err != nil {
		t.Fatal(err)
	}
	response, err := router.Handle(context.Background(), domain.PeerRPCRequest{RequestID: "request-status", PeerID: "node-local", CallerID: "node-a", Capability: subagentstatus.Capability, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	ack, err := subagentstatus.DecodeAcknowledgement(response.Payload)
	if err != nil || ack.SessionID != string(id) || ack.State != kernel.SessionStateComplete {
		t.Fatalf("ack=%+v err=%v", ack, err)
	}
	if transportCalls != 0 {
		t.Fatalf("status ingress used outbound transport %d times", transportCalls)
	}
}

type routerClock struct{}

func (routerClock) Now() time.Time { return time.Unix(100, 0).UTC() }

func TestRouterHandlesAuthenticatedSyncWithoutTransport(t *testing.T) {
	transportCalls := 0
	router := newTestRouter(t, func(_ context.Context, _ domain.PeerRecord, _ domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
		transportCalls++
		return domain.PeerRPCResponse{}, nil
	})
	service, err := peersync.NewService(memory.New(), func() time.Time { return time.Unix(100, 0).UTC() })
	if err != nil {
		t.Fatal(err)
	}
	if err := router.AttachSync("node-local", service); err != nil {
		t.Fatal(err)
	}
	payload, err := peersync.Encode(domain.PeerSyncMessage{SchemaVersion: domain.SchemaVersionV1, StreamID: "stream", MessageID: "message", Kind: domain.PeerSyncHello, OriginID: "node-a"})
	if err != nil {
		t.Fatal(err)
	}
	response, err := router.Handle(context.Background(), domain.PeerRPCRequest{RequestID: "request-sync", PeerID: "node-local", CallerID: "node-a", Capability: peersync.Capability, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	message, err := peersync.Decode(response.Payload)
	if err != nil || message.Kind != domain.PeerSyncAck || message.OriginID != "node-local" {
		t.Fatalf("response = %+v, err=%v", message, err)
	}
	if transportCalls != 0 {
		t.Fatalf("sync unexpectedly used outbound transport %d times", transportCalls)
	}

	if _, err := router.Handle(context.Background(), domain.PeerRPCRequest{RequestID: "request-spoof", PeerID: "node-local", CallerID: "node-b", Capability: peersync.Capability, Payload: payload}); !errors.Is(err, peersync.ErrInvalidPeerIdentity) {
		t.Fatalf("spoof error = %v", err)
	}
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
	router.localID = "node-local"
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
