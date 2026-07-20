package peerhttp

import (
	"context"
	"crypto/x509"
	"net"
	"net/http/httptest"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/network/subagentspawn"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

type integrationClock struct{ now time.Time }

func (c integrationClock) Now() time.Time { return c.now }

type integrationIDs struct{ next int }

func (i *integrationIDs) NewID(prefix string) (string, error) {
	i.next++
	return prefix + "-integration", nil
}

type unusedTransport struct{}

func (unusedTransport) Invoke(context.Context, domain.PeerRecord, domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
	return domain.PeerRPCResponse{}, nil
}

type spawnRPCHandler struct{ spawn subagentspawn.Handler }

func (h spawnRPCHandler) Handle(ctx context.Context, request domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
	if request.PeerID != "node-b" || request.CallerID != "node-a" || request.Capability != subagentspawn.Capability {
		return domain.PeerRPCResponse{}, domain.ErrPeerNotFound
	}
	payload, err := h.spawn.Handle(ctx, request.CallerID, request.Payload)
	if err != nil {
		return domain.PeerRPCResponse{}, err
	}
	return domain.PeerRPCResponse{RequestID: request.RequestID, PeerID: request.PeerID, Payload: payload}, nil
}

func TestIntegration_TransportAndServer(t *testing.T) {
	serverTLS, clientTLS := generateTestPKI()

	caller := &mockCaller{
		callFunc: func(ctx context.Context, req domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
			if req.CallerID != "node-a" {
				t.Fatalf("authenticated caller = %q, want node-a", req.CallerID)
			}
			if req.Capability != "echo" {
				return domain.PeerRPCResponse{}, domain.ErrPeerNotFound
			}
			return domain.PeerRPCResponse{
				RequestID: req.RequestID,
				PeerID:    req.PeerID,
				Payload:   append([]byte("hello-"), req.Payload...),
			}, nil
		},
	}

	handler, err := NewAuthenticatedServerHandler(caller, func(*x509.Certificate) (string, error) { return "node-a", nil })
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewUnstartedServer(handler)
	server.TLS = serverTLS
	server.StartTLS()
	defer server.Close()

	port := server.Listener.Addr().((*net.TCPAddr)).Port

	transport, err := NewTransport(clientTLS, time.Second*5)
	if err != nil {
		t.Fatal(err)
	}

	req := domain.PeerRPCRequest{
		RequestID:  "int-1",
		PeerID:     "node-b",
		Capability: "echo",
		Payload:    []byte("world"),
	}
	peer := domain.PeerRecord{
		Address: domain.PeerAddress{Host: "127.0.0.1", Port: port},
	}

	resp, err := transport.Invoke(context.Background(), peer, req)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	if resp.RequestID != "int-1" || resp.PeerID != "node-b" || string(resp.Payload) != "hello-world" {
		t.Errorf("unexpected payload in response: %+v", resp)
	}
}

func TestIntegration_SubagentSpawnReceiptReplaysAcrossMTLSReceiverRestart(t *testing.T) {
	serverTLS, clientTLS := generateTestPKI()
	clock := integrationClock{now: time.Date(2026, 7, 20, 12, 40, 0, 0, time.UTC)}
	store := memory.New()

	startReceiver := func(store port.Store) (*httptest.Server, error) {
		local := kernel.NewLocalSessionManager(clock)
		manager, err := kernel.NewPersistentSessionManager(local, store, clock, &integrationIDs{}, kernel.PersistentSessionPolicy{MissionID: "mission-receiver", MaxAttempts: 2, Timeout: time.Minute})
		if err != nil {
			return nil, err
		}
		spawnService, err := subagentspawn.NewService(manager)
		if err != nil {
			return nil, err
		}
		handler, err := NewAuthenticatedServerHandler(spawnRPCHandler{spawn: spawnService}, func(*x509.Certificate) (string, error) { return "node-a", nil })
		if err != nil {
			return nil, err
		}
		server := httptest.NewUnstartedServer(handler)
		server.TLS = serverTLS
		server.StartTLS()
		return server, nil
	}

	requestPayload, err := subagentspawn.EncodeRequest(subagentspawn.Request{RequestID: "dispatch-1", SessionID: "source-1", Attempt: 1, Task: "remote work", ContextMode: "isolated"})
	if err != nil {
		t.Fatal(err)
	}
	invoke := func(server *httptest.Server) subagentspawn.Acknowledgement {
		t.Helper()
		transport, err := NewTransport(clientTLS, 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		port := server.Listener.Addr().((*net.TCPAddr)).Port
		response, err := transport.Invoke(context.Background(), domain.PeerRecord{Address: domain.PeerAddress{Host: "127.0.0.1", Port: port}}, domain.PeerRPCRequest{RequestID: "rpc-1", PeerID: "node-b", Capability: subagentspawn.Capability, Payload: requestPayload})
		if err != nil {
			t.Fatal(err)
		}
		ack, err := subagentspawn.DecodeAcknowledgement(response.Payload)
		if err != nil {
			t.Fatal(err)
		}
		return ack
	}

	firstServer, err := startReceiver(store)
	if err != nil {
		t.Fatal(err)
	}
	first := invoke(firstServer)
	firstServer.Close()
	checkpoint, err := store.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	restartedStore, err := memory.NewFromBinary(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	secondServer, err := startReceiver(restartedStore)
	if err != nil {
		t.Fatal(err)
	}
	defer secondServer.Close()
	second := invoke(secondServer)
	if first != second || first.ReceiverSessionID == "" {
		t.Fatalf("mTLS restart replay acknowledgements differ: %+v %+v", first, second)
	}
}
