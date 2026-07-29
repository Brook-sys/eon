package network

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// PeerConfig holds TLS artifacts to establish mutual TLS between nodes.
type PeerConfig struct {
	NodeCert string
	NodeKey  string
	CACert   string
}

// LoadMTLSConfig parses the PEM material and produces a strongly verified TLS config.
func LoadMTLSConfig(cfg PeerConfig) (*tls.Config, error) {
	if cfg.NodeCert == "" || cfg.NodeKey == "" || cfg.CACert == "" {
		return nil, fmt.Errorf("missing peer mtls path configuration")
	}

	cert, err := tls.LoadX509KeyPair(cfg.NodeCert, cfg.NodeKey)
	if err != nil {
		return nil, fmt.Errorf("load node keypair: %w", err)
	}

	caBytes, err := os.ReadFile(cfg.CACert)
	if err != nil {
		return nil, fmt.Errorf("load ca cert: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caBytes) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		ClientCAs:    caPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}, nil
}
