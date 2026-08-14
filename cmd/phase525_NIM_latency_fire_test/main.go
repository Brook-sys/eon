package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/provider/openai"
)

type TrialResult struct {
	Model     string `json:"model"`
	LatencyMs int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
	Status    int    `json:"status,omitempty"`
	Tokens    int    `json:"tokens,omitempty"`
}

func main() {
	nimKey := os.Getenv("NVIDIA_NIM_API_KEY")
	if nimKey == "" {
		log.Fatal("NVIDIA_NIM_API_KEY is required")
	}

	models := []string{
		"meta/llama-3.1-8b-instruct",
		"nvidia/nemotron-4-340b-instruct",
		"meta/llama-3.3-70b-instruct",
		"mistralai/mixtral-8x22b-instruct-v0.1",
	}

	results := []TrialResult{}

	for _, model := range models {
		log.Printf("Probing %s...", model)

		nimProviderConfigured, err := openai.New(openai.Config{
			BaseURL: "https://integrate.api.nvidia.com/v1",
			APIKey:  nimKey,
			Model:   model,
		})
		if err != nil {
			log.Fatalf("Failed to create provider for model: %v", err)
		}

		req := port.CompletionRequest{
			SystemPrompt:    "You are a helpful assistant.",
			Prompt:          "Respond with 'OK' and nothing else.",
			MaxOutputTokens: 64,
			Temperature:     0.0,
		}

		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		resp, err := nimProviderConfigured.Complete(ctx, req)
		cancel()

		latency := time.Since(start).Milliseconds()

		res := TrialResult{
			Model:     model,
			LatencyMs: latency,
		}

		if err != nil {
			log.Printf("Error: %v", err)
			res.Error = err.Error()
		} else {
			log.Printf("Success. Latency: %dms, Response: %q", latency, resp.Text)
			res.Status = 200
			res.Tokens = resp.OutputTokens
		}

		results = append(results, res)
		time.Sleep(1 * time.Second)
	}

	outDir := filepath.Join("results", "phase525_NIM_latency_fire_test")
	os.MkdirAll(outDir, 0755)

	outPath := filepath.Join(outDir, "results.json")
	b, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile(outPath, b, 0644)
	log.Printf("Results saved to %s", outPath)

	fmt.Println("Phase 525 Done.")
}
