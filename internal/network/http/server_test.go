package peerhttp

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"motor-autonomo/internal/domain"
)

func testIdentity(*x509.Certificate) (string, error) { return "caller-a", nil }

type mockCaller struct {
	callFunc func(context.Context, domain.PeerRPCRequest) (domain.PeerRPCResponse, error)
}

func (m *mockCaller) Call(ctx context.Context, req domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
	return m.callFunc(ctx, req)
}

func TestServerHandler_Valid(t *testing.T) {
	caller := &mockCaller{
		callFunc: func(ctx context.Context, req domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
			if req.CallerID != "caller-a" {
				t.Fatalf("caller ID = %q, want caller-a", req.CallerID)
			}
			if req.Capability != "echo" {
				return domain.PeerRPCResponse{}, errors.New("unsupported capability")
			}
			return domain.PeerRPCResponse{
				RequestID: req.RequestID,
				PeerID:    req.PeerID,
				Payload:   append([]byte("echo-"), req.Payload...),
			}, nil
		},
	}

	handler, err := NewAuthenticatedServerHandler(caller, testIdentity)
	if err != nil {
		t.Fatal(err)
	}

	reqBody, _ := json.Marshal(frame{
		RequestID:  "req-1",
		PeerID:     "node-a",
		Capability: "echo",
		Payload:    []byte("hello"),
	})

	req := httptest.NewRequest(http.MethodPost, RPCPath, bytes.NewReader(reqBody))
	req.TLS = verifiedTestTLSState()
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("unexpected content type: %s", w.Header().Get("Content-Type"))
	}

	var resp frame
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	if resp.RequestID != "req-1" || resp.PeerID != "node-a" || string(resp.Payload) != "echo-hello" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestServerHandler_Errors(t *testing.T) {
	caller := &mockCaller{
		callFunc: func(ctx context.Context, req domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
			return domain.PeerRPCResponse{}, errors.New("rpc failed")
		},
	}
	handler, _ := NewAuthenticatedServerHandler(caller, testIdentity)

	// Bad Method
	req := httptest.NewRequest(http.MethodGet, RPCPath, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad method, got %d", w.Code)
	}

	// Bad Path
	req = httptest.NewRequest(http.MethodPost, "/bad", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad path, got %d", w.Code)
	}

	// Bad Content-Type
	req = httptest.NewRequest(http.MethodPost, RPCPath, bytes.NewReader([]byte("{}")))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad content type, got %d", w.Code)
	}

	// Unknown field
	reqBody := []byte(`{"request_id":"1","peer_id":"a","unknown":"bad","payload":"` + `xA==` + `"}`)
	req = httptest.NewRequest(http.MethodPost, RPCPath, bytes.NewReader(reqBody))
	req.TLS = verifiedTestTLSState()
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown field, got %d", w.Code)
	}

	// RPC failure
	reqBody, _ = json.Marshal(frame{
		RequestID:  "req-err",
		PeerID:     "node-a",
		Capability: "echo",
	})
	req = httptest.NewRequest(http.MethodPost, RPCPath, bytes.NewReader(reqBody))
	req.TLS = verifiedTestTLSState()
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for rpc failure, got %d", w.Code)
	}
}

func TestServerHandler_NilCaller(t *testing.T) {
	if _, err := NewServerHandler(nil); err == nil {
		t.Error("expected error for nil caller, got nil")
	}
}

func TestServerHandler_RejectsMissingVerifiedCertificate(t *testing.T) {
	handler, err := NewAuthenticatedServerHandler(&mockCaller{callFunc: func(context.Context, domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
		t.Fatal("caller must not run for unauthenticated request")
		return domain.PeerRPCResponse{}, nil
	}}, testIdentity)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, RPCPath, bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestServerHandler_RejectsInvalidCallerResponse(t *testing.T) {
	for _, test := range []struct {
		name string
		resp domain.PeerRPCResponse
	}{
		{name: "request mismatch", resp: domain.PeerRPCResponse{RequestID: "other", PeerID: "node-a"}},
		{name: "peer mismatch", resp: domain.PeerRPCResponse{RequestID: "req-1", PeerID: "other"}},
		{name: "payload oversize", resp: domain.PeerRPCResponse{RequestID: "req-1", PeerID: "node-a", Payload: make([]byte, maxPeerPayloadBytes+1)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler, err := NewAuthenticatedServerHandler(&mockCaller{callFunc: func(context.Context, domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
				return test.resp, nil
			}}, testIdentity)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := json.Marshal(frame{RequestID: "req-1", PeerID: "node-a", Capability: "echo"})
			req := httptest.NewRequest(http.MethodPost, RPCPath, bytes.NewReader(body))
			req.TLS = verifiedTestTLSState()
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500", w.Code)
			}
		})
	}
}

func TestPeerIDFromCertificate(t *testing.T) {
	valid, err := url.Parse("spiffe://motor-autonomo/peer/node-a")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := PeerIDFromCertificate(&x509.Certificate{URIs: []*url.URL{valid}}); err != nil || got != "node-a" {
		t.Fatalf("valid identity = %q, %v", got, err)
	}
	for _, raw := range []string{
		"spiffe://other/peer/node-a",
		"spiffe://motor-autonomo/node-a",
		"spiffe://motor-autonomo/peer/node/a",
		"https://motor-autonomo/peer/node-a",
	} {
		uri, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := PeerIDFromCertificate(&x509.Certificate{URIs: []*url.URL{uri}}); err == nil {
			t.Errorf("accepted invalid URI SAN %q", raw)
		}
	}
	if _, err := PeerIDFromCertificate(&x509.Certificate{}); err == nil {
		t.Error("accepted certificate without URI SAN")
	}
}

func verifiedTestTLSState() *tls.ConnectionState {
	return &tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{{}}}}
}
