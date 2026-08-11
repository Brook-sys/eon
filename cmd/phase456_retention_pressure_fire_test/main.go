package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"motor-autonomo/internal/domain"
	peersync "motor-autonomo/internal/network/sync"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/provider/openai"
	"motor-autonomo/internal/storage/memory"
)

// TrialResult records the outcome of a single model trial.
type TrialResult struct {
	Provider       string                `json:"provider"`
	Model          string                `json:"model"`
	Latency        time.Duration         `json:"latency"`
	Response       port.CompletionResult `json:"response"`
	Error          string                `json:"error,omitempty"`
	PruneAuthorized bool                 `json:"prune_authorized"`
	HeadPressure   string               `json:"head_pressure"`
}

func main() {
	ctx := context.Background()

	// 1. Verify RetentionReporter with live store
	store := memory.New()
	policy := domain.DefaultStoreRetentionPolicy()
	reporter, err := peersync.NewRetentionReporter(policy, store)
	if err != nil {
		fmt.Printf("Failed to create retention reporter: %v\n", err)
		os.Exit(1)
	}

	report, err := reporter.Report(ctx)
	if err != nil {
		fmt.Printf("Failed to generate retention report: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Retention report: %s\n", report.String())
	if report.PruneAuthorized {
		fmt.Println("ERROR: prune must not be authorized in MVP")
		os.Exit(1)
	}

	// 2. Cognitive Model Trials — rotated providers
	groqKey := os.Getenv("GROQ_API_KEY")
	nimKey := os.Getenv("NVIDIA_NIM_API_KEY")
	if groqKey == "" || nimKey == "" {
		fmt.Println("Missing API keys")
		os.Exit(1)
	}

	// Rotation: emphasize Groq, include NIM cross-provider
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
			model:    "openai/gpt-oss-20b",
		},
		{
			provider: "groq",
			baseURL:  "https://api.groq.com/openai/v1",
			apiKey:   groqKey,
			model:    "qwen/qwen3.6-27b",
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

	promptText := `Analyze the following retention pressure system snapshot and confirm its state:
EVENT_LOG: APPEND_ONLY
PRUNE_AUTHORIZED: false
PRESSURE_MONITOR: ACTIVE
RETENTION_INVARIANT: PRESERVED
Reply with a single line confirming each field's value.`

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

		tr := TrialResult{
			Provider:       tgt.provider,
			Model:          tgt.model,
			Latency:        latency,
			PruneAuthorized: false,
			HeadPressure:   report.EventHeadPressure,
		}

		if err != nil {
			tr.Error = err.Error()
			fmt.Printf("  -> ERROR: %v\n", err)
		} else {
			tr.Response = resp
			fmt.Printf("  -> SUCCESS (%v, %d tokens, finish=%s)\n", latency, resp.OutputTokens, resp.FinishReason)
		}
		results = append(results, tr)
	}

	// Write summary
	summaryDir := filepath.Join("results", "phase456-retention_pressure_fire_test")
	if err := os.MkdirAll(summaryDir, 0755); err != nil {
		fmt.Printf("failed to create results dir: %v\n", err)
	}
	summaryPath := filepath.Join(summaryDir, "summary.json")
	bytes, _ := json.MarshalIndent(results, "", "  ")
	if err := os.WriteFile(summaryPath, bytes, 0644); err != nil {
		fmt.Printf("failed to write summary: %v\n", err)
	} else {
		fmt.Printf("Summary written to %s\n", summaryPath)
	}

	// Final retention report
	finalReport, _ := reporter.Report(ctx)
	fmt.Printf("Final retention report: %s\n", finalReport.String())
}
