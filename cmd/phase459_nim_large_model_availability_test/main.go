package main

// Phase 459 — NIM Large-Model Availability Investigation
//
// Hypothesis: Phase 458 timeouts for meta/llama-3.3-70b-instruct and
// google/gemma-4-31b-it were cold-start artifacts. With graduated per-request
// timeouts (warm-up probe + main trial), we should observe either successful
// completion or a faster failure classification.
//
// Bounds:
//   - 12 planned external calls (6 models × 2 rounds)
//   - max_tokens = 32 per call
//   - temperature = 0.0
//   - warm-up timeout: 15s; main trial timeout: 60s
//   - total budget ceiling: 12 calls × 32 tokens = 384 output tokens max
//
// Cessation criteria:
//   - Stop a model after 2 consecutive provider errors (timeout/5xx)
//   - Stop the campaign if 3 consecutive models all fail

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

type ModelTarget struct {
	Provider      string `json:"provider"`
	BaseURL       string `json:"base_url"`
	Model         string `json:"model"`
	APIKeyEnv     string `json:"api_key_env"`
	WarmupTimeout int    `json:"warmup_timeout_seconds"`
	MainTimeout   int    `json:"main_timeout_seconds"`
}

type TrialResult struct {
	Provider     string                `json:"provider"`
	Model        string                `json:"model"`
	Round        string                `json:"round"` // "warmup" or "main"
	Latency      time.Duration         `json:"latency_ms"`
	Response     port.CompletionResult `json:"response,omitempty"`
	Error        string                `json:"error,omitempty"`
	TimedOut     bool                  `json:"timed_out"`
	FinishReason string                `json:"finish_reason,omitempty"`
}

type CampaignSummary struct {
	Timestamp  string        `json:"timestamp"`
	Hypothesis string        `json:"hypothesis"`
	Bounds     string        `json:"bounds"`
	Planned    int           `json:"planned_calls"`
	Completed  int           `json:"completed_calls"`
	Results    []TrialResult `json:"results"`
}

func main() {
	ctx := context.Background()

	groqKey := os.Getenv("GROQ_API_KEY")
	nimKey := os.Getenv("NVIDIA_NIM_API_KEY")
	if groqKey == "" || nimKey == "" {
		fmt.Println("ERROR: Missing GROQ_API_KEY or NVIDIA_NIM_API_KEY")
		os.Exit(1)
	}

	// Prompt: simple structural classification suitable for any model size.
	// The prompt itself is authority-free — it asks for format confirmation only.
	promptText := `Analyze the following P2P network state snapshot and reply with a single line confirming each field:

NODE_ID: phase459-probe
MTLS: ENFORCED
SYNC_CAPABILITY: event_sync_v1
RETENTION_POLICY: MVP_APPEND_ONLY

Reply with exactly one line per field, prefixing the original field name.`

	targets := []ModelTarget{
		// Groq baselines (expected to succeed quickly)
		{Provider: "groq", BaseURL: "https://api.groq.com/openai/v1", Model: "llama-3.3-70b-versatile", APIKeyEnv: "GROQ_API_KEY", WarmupTimeout: 10, MainTimeout: 30},
		{Provider: "groq", BaseURL: "https://api.groq.com/openai/v1", Model: "llama-3.1-8b-instant", APIKeyEnv: "GROQ_API_KEY", WarmupTimeout: 10, MainTimeout: 30},
		{Provider: "groq", BaseURL: "https://api.groq.com/openai/v1", Model: "allam-2-7b", APIKeyEnv: "GROQ_API_KEY", WarmupTimeout: 10, MainTimeout: 30},

		// NIM large models from Phase 458 that timed out
		{Provider: "nvidia_nim", BaseURL: "https://integrate.api.nvidia.com/v1", Model: "meta/llama-3.3-70b-instruct", APIKeyEnv: "NVIDIA_NIM_API_KEY", WarmupTimeout: 15, MainTimeout: 60},
		{Provider: "nvidia_nim", BaseURL: "https://integrate.api.nvidia.com/v1", Model: "google/gemma-4-31b-it", APIKeyEnv: "NVIDIA_NIM_API_KEY", WarmupTimeout: 15, MainTimeout: 60},

		// NIM models not yet tested in recent phases
		{Provider: "nvidia_nim", BaseURL: "https://integrate.api.nvidia.com/v1", Model: "meta/llama-3.1-70b-instruct", APIKeyEnv: "NVIDIA_NIM_API_KEY", WarmupTimeout: 15, MainTimeout: 60},
	}

	var results []TrialResult
	completed := 0
	consecutiveModelFailures := 0

	for _, tgt := range targets {
		apiKey := groqKey
		if tgt.APIKeyEnv == "NVIDIA_NIM_API_KEY" {
			apiKey = nimKey
		}

		fmt.Printf("\n=== %s / %s ===\n", tgt.Provider, tgt.Model)

		providerClient, err := openai.New(openai.Config{
			BaseURL: tgt.BaseURL,
			APIKey:  apiKey,
			Model:   tgt.Model,
		})
		if err != nil {
			fmt.Printf("  CLIENT ERROR: %v\n", err)
			results = append(results, TrialResult{
				Provider: tgt.Provider, Model: tgt.Model, Round: "warmup",
				Error: fmt.Sprintf("create client: %v", err),
			})
			consecutiveModelFailures++
			if consecutiveModelFailures >= 3 {
				fmt.Println("  3 consecutive model failures — stopping campaign")
				break
			}
			continue
		}

		modelFailed := false

		// Round 1: warm-up probe (short timeout, minimal tokens)
		fmt.Printf("  [warmup] timeout=%ds... ", tgt.WarmupTimeout)
		warmupCtx, warmupCancel := context.WithTimeout(ctx, time.Duration(tgt.WarmupTimeout)*time.Second)
		start := time.Now()
		warmupReq := port.CompletionRequest{
			Prompt:          promptText,
			MaxOutputTokens: 32,
			Temperature:     0.0,
			Timeout:         time.Duration(tgt.WarmupTimeout) * time.Second,
		}
		warmupResp, warmupErr := providerClient.Complete(warmupCtx, warmupReq)
		warmupLatency := time.Since(start)
		warmupCancel()

		warmupResult := TrialResult{
			Provider: tgt.Provider,
			Model:    tgt.Model,
			Round:    "warmup",
			Latency:  warmupLatency,
		}

		if warmupErr != nil {
			warmupResult.Error = warmupErr.Error()
			warmupResult.TimedOut = isTimeout(warmupErr)
			fmt.Printf("FAIL (%v)\n", warmupErr)
			modelFailed = true
		} else {
			warmupResult.Response = warmupResp
			warmupResult.FinishReason = string(warmupResp.FinishReason)
			fmt.Printf("OK (%v, %d tokens, finish=%s)\n", warmupLatency, warmupResp.OutputTokens, warmupResp.FinishReason)
			completed++
		}
		results = append(results, warmupResult)

		// Round 2: main trial (full timeout) — only if warm-up failed OR always (to get latency under full timeout)
		// We always run main trial to get a clean latency measurement under the full timeout
		if !modelFailed {
			fmt.Printf("  [main]    timeout=%ds... ", tgt.MainTimeout)
			mainCtx, mainCancel := context.WithTimeout(ctx, time.Duration(tgt.MainTimeout)*time.Second)
			start = time.Now()
			mainReq := port.CompletionRequest{
				Prompt:          promptText,
				MaxOutputTokens: 32,
				Temperature:     0.0,
				Timeout:         time.Duration(tgt.MainTimeout) * time.Second,
			}
			mainResp, mainErr := providerClient.Complete(mainCtx, mainReq)
			mainLatency := time.Since(start)
			mainCancel()

			mainResult := TrialResult{
				Provider: tgt.Provider,
				Model:    tgt.Model,
				Round:    "main",
				Latency:  mainLatency,
			}
			if mainErr != nil {
				mainResult.Error = mainErr.Error()
				mainResult.TimedOut = isTimeout(mainErr)
				fmt.Printf("FAIL (%v)\n", mainErr)
				modelFailed = true
			} else {
				mainResult.Response = mainResp
				mainResult.FinishReason = string(mainResp.FinishReason)
				fmt.Printf("OK (%v, %d tokens, finish=%s)\n", mainLatency, mainResp.OutputTokens, mainResp.FinishReason)
				completed++
			}
			results = append(results, mainResult)
		}

		if modelFailed {
			consecutiveModelFailures++
		} else {
			consecutiveModelFailures = 0
		}

		if consecutiveModelFailures >= 3 {
			fmt.Println("\n  3 consecutive model failures — stopping campaign early")
			break
		}
	}

	summary := CampaignSummary{
		Timestamp:  time.Now().Format(time.RFC3339),
		Hypothesis: "Phase 458 NIM timeouts (llama-3.3-70b-instruct, gemma-4-31b-it) were cold-start artifacts; graduated timeouts should recover or classify faster.",
		Bounds:     "12 planned calls, 32 max_tokens each, temp=0.0, warmup=10-15s, main=30-60s, stop after 3 consecutive model failures",
		Planned:    12,
		Completed:  completed,
		Results:    results,
	}

	summaryDir := filepath.Join("results", "phase459-nim_large_model_availability")
	if err := os.MkdirAll(summaryDir, 0755); err != nil {
		fmt.Printf("failed to create results dir: %v\n", err)
	}
	summaryPath := filepath.Join(summaryDir, "summary.json")
	bytes, _ := json.MarshalIndent(summary, "", "  ")
	if err := os.WriteFile(summaryPath, bytes, 0644); err != nil {
		fmt.Printf("failed to write summary: %v\n", err)
	} else {
		fmt.Printf("\nSummary written to %s (%d/%d calls completed)\n", summaryPath, completed, 12)
	}

	// Print verdict
	fmt.Println("\n=== VERDICT ===")
	for _, r := range results {
		status := "OK"
		if r.Error != "" {
			status = "FAIL"
			if r.TimedOut {
				status = "TIMEOUT"
			}
		}
		fmt.Printf("  %s/%s [%s]: %s (%v)\n", r.Provider, r.Model, r.Round, status, r.Latency)
	}
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// context.DeadlineExceeded and net/http timeout indicators
	return contains(msg, "deadline exceeded") || contains(msg, "timeout") || contains(msg, "Client.Timeout")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || containsFold(s, substr))
}

func containsFold(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			a, b := s[i+j], substr[j]
			if a >= 'A' && a <= 'Z' {
				a += 32
			}
			if b >= 'A' && b <= 'Z' {
				b += 32
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
