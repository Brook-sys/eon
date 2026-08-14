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

	models := []struct {
		provider string
		model    string
		key      string
		baseURL  string
	}{
		{"groq", "llama-3.3-70b-versatile", groqKey, "https://api.groq.com/openai/v1"},
		{"groq", "llama-3.1-8b-instant", groqKey, "https://api.groq.com/openai/v1"},
		{"groq", "qwen/qwen3.6-27b", groqKey, "https://api.groq.com/openai/v1"},
	}

	results := make([]map[string]interface{}, 0)

	// Injected test prompt simulating deep boundary testing.
	// Scenario: A very large instruction set at the beginning, followed by thousands of words of distracting text, 
	// followed by the actual data payload. We are testing if the instruction boundaries hold under token pressure.
	
	distractor := strings.Repeat("Lorem ipsum dolor sit amet, consectetur adipiscing elit. ", 300)

	promptStr := `TEMPLATE v1

TASK
Extract ONLY the numerical STATUS CODE from the logs below. Return 0 if not found.

CONSTRAINTS
- Output MUST be exactly "STATUS: <code>"
- Do not output any other text or explanation.

FACTS
F1 [1]:
<data>
` + distractor + `
The system encountered an error. Status Code 502 was returned by the upstream server.
</data>

ALLOWED OUTPUTS
A: Text

ANSWER
STATUS: <code>`

	for _, m := range models {
		prov, _ := openai.New(openai.Config{
			BaseURL: m.baseURL,
			APIKey:  m.key,
			Model:   m.model,
		})

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		start := time.Now()
		
		req := port.CompletionRequest{
			SystemPrompt:    "You are a strict data extraction parser.",
			Prompt:          promptStr,
			MaxOutputTokens: 64,
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
		}
		results = append(results, res)
		time.Sleep(1 * time.Second)
	}

	resultsDir := filepath.Join("results", "phase497_prompt_boundary_campaign")
	os.MkdirAll(resultsDir, 0755)
	
	b, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile(filepath.Join(resultsDir, "results.json"), b, 0644)
	fmt.Println("Phase 497 Done.")
}
