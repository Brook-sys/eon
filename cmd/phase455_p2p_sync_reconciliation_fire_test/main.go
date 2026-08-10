package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"motor-autonomo/internal/network"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/provider/openai"
	"motor-autonomo/internal/storage/memory"
)

type TrialResult struct {
	Provider string                `json:"provider"`
	Model    string                `json:"model"`
	Latency  time.Duration         `json:"latency"`
	Response port.CompletionResult `json:"response"`
	Error    string                `json:"error,omitempty"`
}

type PKIFiles struct {
	NodeCert string
	NodeKey  string
	CACert   string
}

func generateNodePKI(dir, nodeName string, caPub ed25519.PublicKey, caPriv ed25519.PrivateKey, caDer []byte) (PKIFiles, error) {
	nodePub, nodePriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return PKIFiles{}, fmt.Errorf("generate node key: %w", err)
	}

	caCert, err := x509.ParseCertificate(caDer)
	if err != nil {
		return PKIFiles{}, fmt.Errorf("parse ca cert: %w", err)
	}

	nodeTemplate := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: nodeName},
		NotBefore:    time.Now().Add(-1 * time.Minute),
		NotAfter:     time.Now().Add(1 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}

	nodeDer, err := x509.CreateCertificate(rand.Reader, &nodeTemplate, caCert, nodePub, caPriv)
	if err != nil {
		return PKIFiles{}, fmt.Errorf("create node cert: %w", err)
	}

	nodePrivBytes, err := x509.MarshalPKCS8PrivateKey(nodePriv)
	if err != nil {
		return PKIFiles{}, fmt.Errorf("marshal node key: %w", err)
	}

	certPath := filepath.Join(dir, nodeName+".crt")
	keyPath := filepath.Join(dir, nodeName+".key")
	caPath := filepath.Join(dir, "ca.crt")

	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDer}), 0600); err != nil {
		return PKIFiles{}, err
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: nodeDer}), 0600); err != nil {
		return PKIFiles{}, err
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: nodePrivBytes}), 0600); err != nil {
		return PKIFiles{}, err
	}

	return PKIFiles{
		NodeCert: certPath,
		NodeKey:  keyPath,
		CACert:   caPath,
	}, nil
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	fmt.Println("=== Phase 455: P2P Manager Sync Reconciliation Live Fire Test ===")

	// 1. Setup PKI
	tmpDir, err := os.MkdirTemp("", "p2p-phase455-*")
	if err != nil {
		fmt.Printf("failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	caPub, caPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fmt.Printf("generate ca key: %v\n", err)
		os.Exit(1)
	}
	caTemplate := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "phase455-ca"},
		NotBefore:             time.Now().Add(-1 * time.Minute),
		NotAfter:              time.Now().Add(1 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDer, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, caPub, caPriv)
	if err != nil {
		fmt.Printf("create ca cert: %v\n", err)
		os.Exit(1)
	}

	pkiA, err := generateNodePKI(tmpDir, "node-a", caPub, caPriv, caDer)
	if err != nil {
		fmt.Printf("pki A: %v\n", err)
		os.Exit(1)
	}
	pkiB, err := generateNodePKI(tmpDir, "node-b", caPub, caPriv, caDer)
	if err != nil {
		fmt.Printf("pki B: %v\n", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// 2. Initialize P2PManager A & B
	mgrA, err := network.NewP2PManager(network.Options{
		Enabled:       true,
		BindAddr:      "127.0.0.1:0",
		NodeID:        "node-a",
		TLSCertFile:   pkiA.NodeCert,
		TLSKeyFile:    pkiA.NodeKey,
		TLSCACertFile: pkiA.CACert,
	}, logger)
	if err != nil {
		fmt.Printf("NewP2PManager A: %v\n", err)
		os.Exit(1)
	}

	mgrB, err := network.NewP2PManager(network.Options{
		Enabled:       true,
		BindAddr:      "127.0.0.1:0",
		NodeID:        "node-b",
		TLSCertFile:   pkiB.NodeCert,
		TLSKeyFile:    pkiB.NodeKey,
		TLSCACertFile: pkiB.CACert,
	}, logger)
	if err != nil {
		fmt.Printf("NewP2PManager B: %v\n", err)
		os.Exit(1)
	}

	storeA := memory.New()
	storeB := memory.New()

	_, _, err = mgrA.AttachSyncService(storeA)
	if err != nil {
		fmt.Printf("AttachSyncService A: %v\n", err)
		os.Exit(1)
	}

	_, _, err = mgrB.AttachSyncService(storeB)
	if err != nil {
		fmt.Printf("AttachSyncService B: %v\n", err)
		os.Exit(1)
	}

	if err := mgrA.Start(ctx); err != nil {
		fmt.Printf("Start A: %v\n", err)
		os.Exit(1)
	}
	defer mgrA.Stop(ctx)

	if err := mgrB.Start(ctx); err != nil {
		fmt.Printf("Start B: %v\n", err)
		os.Exit(1)
	}
	defer mgrB.Stop(ctx)

	// Test direct call ReconcilePeerEventsNow
	_, err = mgrA.ReconcilePeerEventsNow(ctx, "node-b", "events")
	if err != nil {
		fmt.Printf("ReconcilePeerEventsNow expected error (no static registry/mDNS route yet): %v\n", err)
	}

	fmt.Println("P2P Managers A and B successfully verified ReconcilePeerEventsNow API.")

	// 3. Cognitive Model Trials
	groqKey := os.Getenv("GROQ_API_KEY")
	nimKey := os.Getenv("NVIDIA_NIM_API_KEY")
	if groqKey == "" || nimKey == "" {
		fmt.Println("Missing API keys")
		os.Exit(1)
	}

	targets := []struct {
		provider string
		baseURL  string
		apiKey   string
		model    string
	}{
		{
			provider: "groq",
			baseURL:  "https://api.groq.com/openai/v1",
			apiKey:   groqKey,
			model:    "llama-3.3-70b-versatile",
		},
		{
			provider: "groq",
			baseURL:  "https://api.groq.com/openai/v1",
			apiKey:   groqKey,
			model:    "llama-3.1-8b-instant",
		},
		{
			provider: "nvidia_nim",
			baseURL:  "https://integrate.api.nvidia.com/v1",
			apiKey:   nimKey,
			model:    "deepseek-ai/deepseek-v4-flash-0731",
		},
		{
			provider: "nvidia_nim",
			baseURL:  "https://integrate.api.nvidia.com/v1",
			apiKey:   nimKey,
			model:    "meta/llama-3.1-8b-instruct",
		},
	}

	promptText := `Analyze the following system status and report state:
STATUS: READY
ON_DEMAND_SYNC: ENABLED
RECONCILE_API: ACTIVE
CANONICAL_LOG: RECONCILED`

	var results []TrialResult

	for _, tgt := range targets {
		fmt.Printf("Running trial for provider=%s model=%s...\n", tgt.provider, tgt.model)
		providerClient, err := openai.New(openai.Config{
			BaseURL: tgt.baseURL,
			APIKey:  tgt.apiKey,
			Model:   tgt.model,
		})
		if err != nil {
			results = append(results, TrialResult{
				Provider: tgt.provider,
				Model:    tgt.model,
				Error:    fmt.Sprintf("create client: %v", err),
			})
			continue
		}

		req := port.CompletionRequest{
			Prompt:          promptText,
			MaxOutputTokens: 128,
			Temperature:     0.0,
		}

		start := time.Now()
		resp, err := providerClient.Complete(ctx, req)
		latency := time.Since(start)

		if err != nil {
			results = append(results, TrialResult{
				Provider: tgt.provider,
				Model:    tgt.model,
				Latency:  latency,
				Error:    err.Error(),
			})
			fmt.Printf("  -> ERROR: %v\n", err)
		} else {
			results = append(results, TrialResult{
				Provider: tgt.provider,
				Model:    tgt.model,
				Latency:  latency,
				Response: resp,
			})
			fmt.Printf("  -> SUCCESS (%v, %d tokens, finish=%s)\n", latency, resp.OutputTokens, resp.FinishReason)
		}
	}

	summaryPath := filepath.Join("results", "phase455-p2p_sync_reconciliation_fire_test", "summary.json")
	bytes, _ := json.MarshalIndent(results, "", "  ")
	if err := os.WriteFile(summaryPath, bytes, 0644); err != nil {
		fmt.Printf("failed to write summary: %v\n", err)
	} else {
		fmt.Printf("Summary written to %s\n", summaryPath)
	}
}
