package peerhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"motor-autonomo/internal/domain"
)

type mockCaller struct {
	callFunc func(context.Context, domain.PeerRPCRequest) (domain.PeerRPCResponse, error)
}

func (m *mockCaller) Call(ctx context.Context, req domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
	return m.callFunc(ctx, req)
}

func TestServerHandler_Valid(t *testing.T) {
	caller := &mockCaller{
		callFunc: func(ctx context.Context, req domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
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

	handler, err := NewServerHandler(caller)
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
	handler, _ := NewServerHandler(caller)

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
