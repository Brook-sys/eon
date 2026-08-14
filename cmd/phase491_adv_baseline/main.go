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

func main() {
	groqKey := os.Getenv("GROQ_API_KEY")
	if groqKey == "" {
		fmt.Println("GROQ_API_KEY not set")
		os.Exit(1)
	}

	nimKey := os.Getenv("NVIDIA_NIM_API_KEY")
	if nimKey == "" {
		fmt.Println("NVIDIA_NIM_API_KEY not set")
		os.Exit(1)
	}

	fmt.Println("Starting phase491_adv_baseline sweep...")

	models := []struct {
		provider  string
		model     string
		key       string
		baseURL   string
		reasoning string
	}{
		{"groq", "llama-3.3-70b-versatile", groqKey, "https://api.groq.com/openai/v1", ""},
		{"groq", "qwen/qwen3.6-27b", groqKey, "https://api.groq.com/openai/v1", "none"},
		{"nim", "meta/llama-3.1-8b-instruct", nimKey, "https://integrate.api.nvidia.com/v1", ""},
	}

	results := make([]map[string]interface{}, 0)

	for _, m := range models {
		fmt.Printf("Testing %s %s...\n", m.provider, m.model)
		prov, err := openai.New(openai.Config{
			BaseURL: m.baseURL,
			APIKey:  m.key,
			Model:   m.model,
		})
		if err != nil {
			fmt.Printf("Failed to init provider %s %s: %v\n", m.provider, m.model, err)
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		start := time.Now()
		resp, err := prov.Complete(ctx, port.CompletionRequest{
			Prompt:          "Reply with exactly: READY",
			MaxOutputTokens: 16,
			ReasoningEffort: m.reasoning,
		})
		latency := time.Since(start)
		cancel()

		res := map[string]interface{}{
			"provider":   m.provider,
			"model":      m.model,
			"latency_ms": latency.Milliseconds(),
		}
		if err != nil {
			res["error"] = err.Error()
			fmt.Printf("Error: %v\n", err)
		} else {
			res["text"] = resp.Text
			res["finish_reason"] = resp.FinishReason
			fmt.Printf("Success. Latency: %dms, Reply: %q\n", latency.Milliseconds(), resp.Text)
		}
		results = append(results, res)
		time.Sleep(1 * time.Second) // rate limit pace
	}

	resultsDir := filepath.Join("results", "phase491_adv_baseline")
	os.MkdirAll(resultsDir, 0755)

	b, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile(filepath.Join(resultsDir, "results.json"), b, 0644)
	fmt.Println("Done.")
}
