package peerhttp

import (
	"context"
	"net"
	"net/http/httptest"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
)

func TestIntegration_TransportAndServer(t *testing.T) {
	serverTLS, clientTLS := generateTestPKI()

	caller := &mockCaller{
		callFunc: func(ctx context.Context, req domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
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

	handler, err := NewServerHandler(caller)
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
