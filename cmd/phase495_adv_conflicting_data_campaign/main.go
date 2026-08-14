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
		{"groq", "qwen/qwen3.6-27b", groqKey, "https://api.groq.com/openai/v1"},
		{"nim", "meta/llama-3.1-8b-instruct", nimKey, "https://integrate.api.nvidia.com/v1"},
	}

	results := make([]map[string]interface{}, 0)

	// Conflicting data test: Test if models get distracted by strong negations regarding a different subject, missing the target answer completely.
	// Task: Does User-A have administrative rights? (YES/NO).
	conflictPrompt := `Answer YES or NO if User-A has administrative rights.
Return ONLY this format:
ADMIN: <YES/NO>

Text: User-B does NOT have administrative rights. Under no circumstances should User-B be granted administrative rights, this is strictly forbidden. User-A has administrative rights.`

	for _, m := range models {
		prov, _ := openai.New(openai.Config{
			BaseURL: m.baseURL,
			APIKey:  m.key,
			Model:   m.model,
		})

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		start := time.Now()
		
		req := port.CompletionRequest{
			Prompt:          conflictPrompt,
			MaxOutputTokens: 16,
		}
		
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
			
			res["strict_yes"] = strings.Contains(resp.Text, "ADMIN: YES")
			res["distracted_no"] = strings.Contains(resp.Text, "ADMIN: NO")
		}
		results = append(results, res)
		time.Sleep(1 * time.Second)
	}

	resultsDir := filepath.Join("results", "phase495_adv_conflicting")
	os.MkdirAll(resultsDir, 0755)
	
	b, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile(filepath.Join(resultsDir, "results.json"), b, 0644)
	fmt.Println("Phase 495 Done.")
}
