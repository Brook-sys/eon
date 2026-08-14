package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/provider/openai"
)

type TrialResult struct {
	Model     string `json:"model"`
	Strategy  string `json:"strategy"`
	LatencyMs int64  `json:"latency"`
	Error     string `json:"error,omitempty"`
	Status    int    `json:"status,omitempty"`
}

func main() {
	if os.Getenv("GROQ_API_KEY") == "" {
		log.Fatal("GROQ_API_KEY is required")
	}

	outDir := filepath.Join("results", "phase524_qwen_parameter_fire_test")
	os.MkdirAll(outDir, 0755)

	strategies := []struct {
		Name    string
		Model   string
		Request port.CompletionRequest
	}{
		{
			Name:  "qwen/qwen3.6-27b_minimal",
			Model: "qwen/qwen3.6-27b",
			Request: port.CompletionRequest{
				Prompt: "Respond with OK.",
			},
		},
		{
			Name:  "qwen/qwen3.6-27b_system",
			Model: "qwen/qwen3.6-27b",
			Request: port.CompletionRequest{
				Prompt:       "Respond with OK.",
				SystemPrompt: "You are a helpful assistant.",
			},
		},
		{
			Name:  "qwen/qwen3.6-27b_prefill",
			Model: "qwen/qwen3.6-27b",
			Request: port.CompletionRequest{
				Prompt:           "Respond with OK.",
				PrefillAssistant: "O",
			},
		},
		{
			Name:  "qwen/qwen3.6-27b_reasoning_none",
			Model: "qwen/qwen3.6-27b",
			Request: port.CompletionRequest{
				Prompt:          "Respond with OK.",
				ReasoningEffort: "none",
			},
		},
	}

	var results []TrialResult

	for _, strategy := range strategies {
		log.Printf("Running %s (%s)", strategy.Model, strategy.Name)

		provider, _ := openai.New(openai.Config{
			BaseURL: "https://api.groq.com/openai/v1",
			APIKey:  os.Getenv("GROQ_API_KEY"),
			Model:   strategy.Model,
		})

		start := time.Now()
		_, err := provider.Complete(context.Background(), strategy.Request)
		latency := time.Since(start).Milliseconds()

		res := TrialResult{
			Model:     strategy.Model,
			Strategy:  strategy.Name,
			LatencyMs: latency,
		}

		if err != nil {
			res.Error = err.Error()
		}

		results = append(results, res)
		time.Sleep(1 * time.Second)
	}

	b, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile(filepath.Join(outDir, "results.json"), b, 0644)
	log.Println("Phase 524 Done.")
}
