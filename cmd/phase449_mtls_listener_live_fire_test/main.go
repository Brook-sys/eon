//go:build ignore

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/provider/openai"
)

type TrialResult struct {
	Provider string                `json:"provider"`
	Model    string                `json:"model"`
	Latency  time.Duration         `json:"latency"`
	Response port.CompletionResult `json:"response"`
	Error    string                `json:"error,omitempty"`
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

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
		Prompt: `Analyse the following Go code configuration:
- cert, err := tls.LoadX509KeyPair(certFile, keyFile)
- caPool.AppendCertsFromPEM(caBytes)
- tlsConfig = &tls.Config{Certificates: []tls.Certificate{cert}, ClientCAs: caPool, ClientAuth: tls.RequireAndVerifyClientCert, MinVersion: tls.VersionTLS13}
- listener = tls.NewListener(tcpListener, tlsConfig)

Evaluate if mutual TLS (mTLS) is enforced for incoming connections.
Emit strictly line prefixes:
STATUS: READY
ENFORCED: YES
CLIENT_AUTH: REQUIRED
TLS_MIN: TLS13`,
		MaxOutputTokens:  128,
		Temperature:      0.0,
		PrefillAssistant: "STATUS:",
	}

	outDir := filepath.Join("results", "phase449-mtls_listener_live_fire")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Printf("failed to create outDir: %v\n", err)
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
			fmt.Printf("Failed to init provider %s/%s: %v\n", t.provider, t.model, err)
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

	outFile := filepath.Join(outDir, "summary.json")
	data, _ := json.MarshalIndent(results, "", "  ")
	if err := os.WriteFile(outFile, data, 0644); err != nil {
		fmt.Printf("Failed to save summary: %v\n", err)
	} else {
		fmt.Printf("Saved campaign summary to %s\n", outFile)
	}
}
