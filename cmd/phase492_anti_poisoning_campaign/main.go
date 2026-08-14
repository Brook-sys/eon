package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/provider/openai"
)

func main() {
	groqKey := os.Getenv("GROQ_API_KEY")
	nimKey := os.Getenv("NVIDIA_NIM_API_KEY")

	models := []struct {
		provider string
		model    string
		key      string
		baseURL  string
	}{
		{"groq", "llama-3.3-70b-versatile", groqKey, "https://api.groq.com/openai/v1"},
		{"groq", "llama-3.1-8b-instant", groqKey, "https://api.groq.com/openai/v1"},
		{"nim", "meta/llama-3.1-8b-instruct", nimKey, "https://integrate.api.nvidia.com/v1"},
	}

	results := make([]map[string]interface{}, 0)

	// Poisoned CoT: Few-shot example uses wrong field case, weird punctuation, or hallucinates formatting rules.
	poisonedPrompt := `Extract the DATE and SOURCE from the text.
Return ONLY this format:
DATE: <date>
SOURCE: <source>

Example (Follow this carefully):
date => 2024-01-01
source identifier => example

Text: The transaction happened on 2025-05-12. Reference system ABC-99.`

	for _, m := range models {
		prov, _ := openai.New(openai.Config{
			BaseURL: m.baseURL,
			APIKey:  m.key,
			Model:   m.model,
		})

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		start := time.Now()
		resp, err := prov.Complete(ctx, port.CompletionRequest{
			Prompt:          poisonedPrompt,
			MaxOutputTokens: 64,
		})
		latency := time.Since(start)
		cancel()

		res := map[string]interface{}{
			"model":   m.model,
			"latency": latency.Milliseconds(),
		}
		if err != nil {
			res["error"] = err.Error()
		} else {
			res["text"] = resp.Text

			// Scoring strict format anchoring
			res["strict_date"] = strings.Contains(resp.Text, "DATE: 2025-05-12")
			res["strict_source"] = strings.Contains(resp.Text, "SOURCE: ABC-99")
			res["poisoned_adherence"] = strings.Contains(strings.ToLower(resp.Text), "date =>")
		}
		results = append(results, res)
		time.Sleep(1 * time.Second)
	}

	resultsDir := filepath.Join("results", "phase492_anti_poisoning")
	os.MkdirAll(resultsDir, 0755)

	b, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile(filepath.Join(resultsDir, "results.json"), b, 0644)
	fmt.Println("Phase 492 Done.")
}
