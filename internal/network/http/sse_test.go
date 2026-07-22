package peerhttp_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"testing"

	"motor-autonomo/internal/domain"
	peerhttp "motor-autonomo/internal/network/http"
)

type mockSSEHandler struct {
	handled   bool
	namespace string
}

func (m *mockSSEHandler) Handle(ctx context.Context, req domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
	return domain.PeerRPCResponse{}, nil
}

func (m *mockSSEHandler) HandleSSE(w http.ResponseWriter, r *http.Request) {
	m.handled = true
	m.namespace = r.URL.Query().Get("namespace")
	w.WriteHeader(http.StatusOK)
}

func TestServerHandler_RoutesSSEToHandlerMethod(t *testing.T) {
	mock := &mockSSEHandler{}
	srv, err := peerhttp.NewAuthenticatedServerHandler(mock, func(c *x509.Certificate) (string, error) { return "test-peer-123", nil })
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/events/stream", nil)
	req.TLS = &tls.ConnectionState{
		VerifiedChains: [][]*x509.Certificate{{{}}},
	}
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if !mock.handled {
		t.Error("expected HandleSSE to be called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if mock.namespace != "test-peer-123" {
		t.Errorf("expected namespace test-peer-123, got %s", mock.namespace)
	}
}
