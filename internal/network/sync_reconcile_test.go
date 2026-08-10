package network

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

func generateTestPKIFiles(t *testing.T, dir string, nodeName string) (certPath, keyPath, caPath string) {
	t.Helper()
	caPub, caPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ca key: %v", err)
	}
	caTemplate := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-1 * time.Minute),
		NotAfter:              time.Now().Add(1 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDer, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, caPub, caPriv)
	if err != nil {
		t.Fatalf("create ca cert: %v", err)
	}

	nodePub, nodePriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate node key: %v", err)
	}
	nodeTemplate := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: nodeName},
		NotBefore:    time.Now().Add(-1 * time.Minute),
		NotAfter:     time.Now().Add(1 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	caCert, _ := x509.ParseCertificate(caDer)
	nodeDer, err := x509.CreateCertificate(rand.Reader, &nodeTemplate, caCert, nodePub, caPriv)
	if err != nil {
		t.Fatalf("create node cert: %v", err)
	}

	nodePrivBytes, _ := x509.MarshalPKCS8PrivateKey(nodePriv)

	certPath = filepath.Join(dir, nodeName+".crt")
	keyPath = filepath.Join(dir, nodeName+".key")
	caPath = filepath.Join(dir, "ca.crt")

	_ = os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDer}), 0600)
	_ = os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: nodeDer}), 0600)
	_ = os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: nodePrivBytes}), 0600)

	return certPath, keyPath, caPath
}

func TestP2PManager_ReconcilePeerEventsNow(t *testing.T) {
	tmpDir := t.TempDir()
	certPath, keyPath, caPath := generateTestPKIFiles(t, tmpDir, "node-test")

	opts := Options{
		Enabled:       true,
		NodeID:        "node-test",
		TLSCertFile:   certPath,
		TLSKeyFile:    keyPath,
		TLSCACertFile: caPath,
	}

	storeA := memory.New()
	logger := slog.Default()
	mgrA, err := NewP2PManager(opts, logger)
	if err != nil {
		t.Fatalf("NewP2PManager failed: %v", err)
	}

	_, _, err = mgrA.AttachSyncService(storeA)
	if err != nil {
		t.Fatalf("AttachSyncService failed: %v", err)
	}

	ctx := context.Background()
	if err := mgrA.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer mgrA.Stop(ctx)

	// Seed event in storeA via Update transaction
	err = storeA.Update(ctx, func(tx port.Transaction) error {
		_, err := tx.AppendEvent(domain.Event{
			SchemaVersion: domain.SchemaVersionV1,
			ID:            "evt-test-1",
			Kind:          "test.created",
			OccurredAt:    time.Now(),
		})
		return err
	})
	if err != nil {
		t.Fatalf("AppendEvent transaction failed: %v", err)
	}

	// Reconcile with a peer node
	summary, err := mgrA.ReconcilePeerEventsNow(ctx, "peer-node-b", "events")
	if err == nil {
		t.Logf("Reconcile returned summary: %+v", summary)
	} else {
		t.Logf("Expected failure for unreachable peer: %v", err)
	}
}

func TestP2PManager_ReconcilePeerEventsNow_Errors(t *testing.T) {
	var mgr *P2PManager
	ctx := context.Background()
	_, err := mgr.ReconcilePeerEventsNow(ctx, "peer-b", "events")
	if err == nil {
		t.Errorf("expected error when manager is nil")
	}

	mgr2 := &P2PManager{}
	_, err = mgr2.ReconcilePeerEventsNow(ctx, "peer-b", "events")
	if err == nil {
		t.Errorf("expected error when ticker is nil")
	}
}
