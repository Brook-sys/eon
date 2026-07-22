package network

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func generateTestPKI(t *testing.T, dir string) PeerConfig {
	t.Helper()

	// Generate CA
	caPub, caPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ca key: %v", err)
	}
	caTemplate := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "motor-autonomo-ca"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDer, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, caPub, caPriv)
	if err != nil {
		t.Fatalf("create ca cert: %v", err)
	}

	caPath := filepath.Join(dir, "ca.crt")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDer}), 0600); err != nil {
		t.Fatalf("write ca cert: %v", err)
	}

	// Generate Node
	nodePub, nodePriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate node key: %v", err)
	}
	nodeTemplate := x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "node-1"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	nodeDer, err := x509.CreateCertificate(rand.Reader, &nodeTemplate, &caTemplate, nodePub, caPriv)
	if err != nil {
		t.Fatalf("create node cert: %v", err)
	}
	nodePrivBytes, err := x509.MarshalPKCS8PrivateKey(nodePriv)
	if err != nil {
		t.Fatalf("marshal node key: %v", err)
	}

	certPath := filepath.Join(dir, "node.crt")
	keyPath := filepath.Join(dir, "node.key")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: nodeDer}), 0600); err != nil {
		t.Fatalf("write node cert: %v", err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: nodePrivBytes}), 0600); err != nil {
		t.Fatalf("write node key: %v", err)
	}

	return PeerConfig{
		NodeCert: certPath,
		NodeKey:  keyPath,
		CACert:   caPath,
	}
}

func TestLoadMTLSConfig_Valid(t *testing.T) {
	dir := t.TempDir()
	cfg := generateTestPKI(t, dir)

	tlsCfg, err := LoadMTLSConfig(cfg)
	if err != nil {
		t.Fatalf("load mtls config: %v", err)
	}
	if tlsCfg.MinVersion != tls.VersionTLS13 {
		t.Errorf("min version = %d, want TLS 1.3", tlsCfg.MinVersion)
	}
	if tlsCfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Errorf("client auth = %d, want RequireAndVerifyClientCert", tlsCfg.ClientAuth)
	}
	if len(tlsCfg.Certificates) != 1 {
		t.Errorf("certs length = %d, want 1", len(tlsCfg.Certificates))
	}
}

func TestLoadMTLSConfig_MissingFiles(t *testing.T) {
	dir := t.TempDir()
	cfg := generateTestPKI(t, dir)

	cfg.NodeCert = filepath.Join(dir, "missing.crt")
	if _, err := LoadMTLSConfig(cfg); err == nil {
		t.Error("expected error for missing cert, got nil")
	}

	cfg = generateTestPKI(t, dir)
	cfg.CACert = filepath.Join(dir, "missing.ca")
	if _, err := LoadMTLSConfig(cfg); err == nil {
		t.Error("expected error for missing CA, got nil")
	}
}
