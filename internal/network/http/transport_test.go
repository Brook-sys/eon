package peerhttp

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
)

func generateTestPKI() (*tls.Config, *tls.Config) {
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTemplate := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caDER, _ := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &caKey.PublicKey, caKey)
	caCert, _ := x509.ParseCertificate(caDER)

	serverKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serverTemplate := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "Server"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}
	serverDER, _ := x509.CreateCertificate(rand.Reader, &serverTemplate, caCert, &serverKey.PublicKey, caKey)

	clientKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	clientTemplate := x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "Client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}
	clientDER, _ := x509.CreateCertificate(rand.Reader, &clientTemplate, caCert, &clientKey.PublicKey, caKey)

	pool := x509.NewCertPool()
	pool.AddCert(caCert)

	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{serverDER}, PrivateKey: serverKey}},
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}
	clientTLS := &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{clientDER}, PrivateKey: clientKey}},
		RootCAs:      pool,
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}
	return serverTLS, clientTLS
}

func TestTransport_Valid(t *testing.T) {
	serverTLS, clientTLS := generateTestPKI()
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != RPCPath || r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		var f frame
		if err := json.NewDecoder(r.Body).Decode(&f); err != nil || f.RequestID != "req-1" || f.Capability != "echo" {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(frame{RequestID: f.RequestID, PeerID: "server-id", Payload: append([]byte("echo-"), f.Payload...)})
	}))
	server.TLS = serverTLS
	server.StartTLS()
	defer server.Close()

	port := server.Listener.Addr().((*net.TCPAddr)).Port
	transport, err := NewTransport(clientTLS, time.Second)
	if err != nil {
		t.Fatal(err)
	}

	req := domain.PeerRPCRequest{RequestID: "req-1", PeerID: "server-id", Capability: "echo", Payload: []byte("ping")}
	resp, err := transport.Invoke(context.Background(), domain.PeerRecord{Address: domain.PeerAddress{Host: "127.0.0.1", Port: port}}, req)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if resp.RequestID != "req-1" || resp.PeerID != "server-id" || string(resp.Payload) != "echo-ping" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestTransport_Errors(t *testing.T) {
	serverTLS, clientTLS := generateTestPKI()
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"request_id":"req-err","peer_id":"server-id","payload":"","unknown":"field"}`))
	}))
	server.TLS = serverTLS
	server.StartTLS()
	defer server.Close()

	port := server.Listener.Addr().((*net.TCPAddr)).Port
	transport, _ := NewTransport(clientTLS, time.Second)

	_, err := transport.Invoke(context.Background(), domain.PeerRecord{Address: domain.PeerAddress{Host: "127.0.0.1", Port: port}}, domain.PeerRPCRequest{})
	if err != ErrInvalidFrame {
		t.Errorf("expected ErrInvalidFrame for strict decoding unknown fields, got %v", err)
	}

	_, err = NewTransport(nil, time.Second)
	if err == nil {
		t.Error("expected error for nil config")
	}

	insecureTLS := clientTLS.Clone()
	insecureTLS.ClientAuth = tls.NoClientCert
	_, err = NewTransport(insecureTLS, time.Second)
	if err == nil {
		t.Error("expected error for insecure config")
	}
}
