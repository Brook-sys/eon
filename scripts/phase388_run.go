package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/provider/openai"
)

type TrialResult struct {
	Model      string `json:"model"`
	Format     string `json:"format"`
	LatencyMs  int64  `json:"latency_ms"`
	StatusCode int    `json:"status_code"`
	Success    bool   `json:"success"`
	Tokens     int    `json:"tokens"`
	Error      string `json:"error,omitempty"`
}

func main() {
	groqKey := os.Getenv("GROQ_API_KEY")
	if groqKey == "" {
		fmt.Println("Missing GROQ_API_KEY")
		os.Exit(1)
	}

	models := []string{
		"llama-3.1-8b-instant",
		"llama-3.3-70b-versatile",
		"qwen/qwen3.6-27b",
	}

	var results []TrialResult

	// Phase 388 specific: test deep budget starvation (max_tokens=20) on a simple instruction
	req := port.CompletionRequest{
		Prompt:          "You are a helpful assistant. Be concise.\n\nReply with exactly: READY",
		MaxOutputTokens:  20,
		Temperature:      0.0,
		ReasoningEffort:  "none",
	}

	for _, modelName := range models {
		config := openai.Config{
			BaseURL: "https://api.groq.com/openai/v1",
			APIKey:  groqKey,
			Model:   modelName,
			MaxOutputField: openai.MaxOutputTokensLegacy,
		}
		
		provider, err := openai.New(config)
		if err != nil {
			fmt.Printf("Failed to init provider for %s: %v\n", modelName, err)
			continue
		}

		for i := 0; i < 3; i++ {
			start := time.Now()
			
			resp, err := provider.Complete(context.Background(), req)
			
			lat := time.Since(start).Milliseconds()
			
			trial := TrialResult{
				Model:     modelName,
				Format:    "exact_match",
				LatencyMs: lat,
			}
			
			if err != nil {
				trial.Success = false
				trial.Error = err.Error()
			} else {
				trial.Success = (resp.Text == "READY")
				trial.StatusCode = 200 // adapter success
				trial.Tokens = resp.OutputTokens
				if !trial.Success {
					trial.Error = fmt.Sprintf("content mismatch: %q", resp.Text)
				}
			}
			
			fmt.Printf("[%s] trial %d: success=%v lat=%dms\n", modelName, i, trial.Success, trial.LatencyMs)
			results = append(results, trial)
			
			time.Sleep(500 * time.Millisecond) // rate limit backoff
		}
	}

	b, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile("results/phase388-resilience-audit/results.json", b, 0644)
}
