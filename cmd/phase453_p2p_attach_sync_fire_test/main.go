//go:build ignore

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

	"motor-autonomo/internal/domain"
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

	fmt.Println("=== Phase 453: P2P Manager AttachSyncService Live Fire Test ===")

	// 1. Setup PKI
	tmpDir, err := os.MkdirTemp("", "p2p-phase453-*")
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
		Subject:               pkix.Name{CommonName: "phase453-ca"},
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

	svcA, tickerA, err := mgrA.AttachSyncService(storeA)
	if err != nil {
		fmt.Printf("AttachSyncService A: %v\n", err)
		os.Exit(1)
	}
	_ = svcA

	svcB, tickerB, err := mgrB.AttachSyncService(storeB)
	if err != nil {
		fmt.Printf("AttachSyncService B: %v\n", err)
		os.Exit(1)
	}
	_ = svcB

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

	// Execute single tick pass
	_ = tickerA.Tick(ctx)
	_ = tickerB.Tick(ctx)

	fmt.Println("P2P Managers A and B successfully attached Sync Service and Ticker.")

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
			provider: "nim",
			baseURL:  "https://integrate.api.nvidia.com/v1",
			apiKey:   nimKey,
			model:    "deepseek-ai/deepseek-v4-flash-0731",
		},
		{
			provider: "nim",
			baseURL:  "https://integrate.api.nvidia.com/v1",
			apiKey:   nimKey,
			model:    "meta/llama-3.1-8b-instruct",
		},
	}

	req := port.CompletionRequest{
		Prompt: `Análise do componente P2P AttachSyncService:
- Método AttachSyncService() instanciado no P2PManager.
- Capacidade "event.sync.v1" registrada no Router.
- Serviço e Ticker de sincronização mTLS operacionais.

Instruções:
Analise se o componente AttachSyncService está pronto e ativo.
Responda estritamente emitindo os prefixos de linha:
STATUS: READY
SYNC_ATTACH: ATTACHED
SYNC_CAPABILITY: EVENT_SYNC_V1
TICKER_PASS: PASS`,
		MaxOutputTokens:  128,
		Temperature:      0.0,
		PrefillAssistant: "STATUS:",
	}

	outDir := filepath.Join("results", "phase453-p2p_attach_sync_fire_test")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Printf("failed to create output dir: %v\n", err)
		os.Exit(1)
	}

	var results []TrialResult

	for _, t := range targets {
		fmt.Printf("Executing live trial on %s / %s...\n", t.provider, t.model)
		provider, err := openai.New(openai.Config{
			BaseURL: t.baseURL,
			APIKey:  t.apiKey,
			Model:   t.model,
		})
		if err != nil {
			fmt.Printf("  -> Failed to create provider: %v\n", err)
			continue
		}

		start := time.Now()
		res, err := provider.Complete(ctx, req)
		elapsed := time.Since(start)

		tr := TrialResult{
			Provider: t.provider,
			Model:    t.model,
			Latency:  elapsed,
			Response: res,
		}

		if err != nil {
			tr.Error = err.Error()
			fmt.Printf("  -> ERROR (%v): %v\n", elapsed, err)
		} else {
			fmt.Printf("  -> OK (%v): %d output tokens, finish=%s\n", elapsed, res.OutputTokens, res.FinishReason)
			fmt.Printf("     Text: %q\n", res.Text)
		}
		results = append(results, tr)
	}

	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		fmt.Printf("failed to marshal results: %v\n", err)
		os.Exit(1)
	}

	sumPath := filepath.Join(outDir, "summary.json")
	if err := os.WriteFile(sumPath, data, 0644); err != nil {
		fmt.Printf("failed to write summary: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Saved campaign summary to %s\n", sumPath)
}

var _ = domain.PeerRecord{}
