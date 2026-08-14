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
		{"groq", "qwen/qwen3.6-27b", groqKey, "https://api.groq.com/openai/v1"},
		{"nim", "meta/llama-3.1-8b-instruct", nimKey, "https://integrate.api.nvidia.com/v1"},
		{"nim", "mistralai/mistral-large-2-instruct", nimKey, "https://integrate.api.nvidia.com/v1"},
	}

	results := make([]map[string]interface{}, 0)

	// Ambiguity test: Give the model multiple dates/sources and an ambiguous instruction on which to pick.
	// Task: Extract the PRIMARY EVENT DATE and SOURCE.
	// Conflicting context: "The plan started on 2025-01-01 (Source: SYS-A), but the actual primary execution recorded by SYS-B occurred on 2025-05-12, followed by a post-mortem on 2025-06-01 (Source: SYS-C)."
	ambiguousPrompt := `Extract the PRIMARY EXECUTION DATE and SOURCE from the text.
Return ONLY this format:
DATE: <YYYY-MM-DD>
SOURCE: <ID>

Text: The plan started on 2025-01-01 (Source: SYS-A), but the actual primary execution recorded by SYS-B occurred on 2025-05-12, followed by a post-mortem on 2025-06-01 (Source: SYS-C).`

	for _, m := range models {
		prov, _ := openai.New(openai.Config{
			BaseURL: m.baseURL,
			APIKey:  m.key,
			Model:   m.model,
		})

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		start := time.Now()
		
		req := port.CompletionRequest{
			Prompt:          ambiguousPrompt,
			MaxOutputTokens: 64,
		}
		
		// If Qwen, disable reasoning effort so it emits directly.
		if strings.Contains(m.model, "qwen") {
			req.ReasoningEffort = "none"
		}
		
		resp, err := prov.Complete(ctx, req)
		latency := time.Since(start)
		cancel()

		res := map[string]interface{}{
			"model": m.model,
			"latency": latency.Milliseconds(),
		}
		if err != nil {
			res["error"] = err.Error()
		} else {
			res["text"] = resp.Text
			
			// Expected primary execution values: DATE: 2025-05-12, SOURCE: SYS-B
			res["semantic_date_correct"] = strings.Contains(resp.Text, "2025-05-12")
			res["semantic_source_correct"] = strings.Contains(resp.Text, "SYS-B")
			res["strict_format"] = strings.Contains(resp.Text, "DATE: 2025-05-12") && strings.Contains(resp.Text, "SOURCE: SYS-B")
		}
		results = append(results, res)
		time.Sleep(1 * time.Second)
	}

	resultsDir := filepath.Join("results", "phase493_adv_ambiguity")
	os.MkdirAll(resultsDir, 0755)
	
	b, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile(filepath.Join(resultsDir, "results.json"), b, 0644)
	fmt.Println("Phase 493 Done.")
}
