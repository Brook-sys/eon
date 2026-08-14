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

	// Language degradation test: Instruction is in English, structure is strictly English keys, but context is in PT-BR with heavy slang/regionalisms.
	// Task: Extract SYSTEM_STATUS and FAILURE_REASON.
	langPrompt := `Extract the SYSTEM_STATUS and FAILURE_REASON from the text.
Return ONLY this format:
STATUS: <status>
REASON: <reason>

Text: O sistema pifou geral ontem de noite, a gambiarra que o Zé fez no banco de dados não segurou a onda e deu timeout na requisição principal. O treco tá todo fora do ar agora.`

	for _, m := range models {
		prov, _ := openai.New(openai.Config{
			BaseURL: m.baseURL,
			APIKey:  m.key,
			Model:   m.model,
		})

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		start := time.Now()

		req := port.CompletionRequest{
			Prompt:          langPrompt,
			MaxOutputTokens: 64,
		}

		if strings.Contains(m.model, "qwen") {
			req.ReasoningEffort = "none"
		}

		resp, err := prov.Complete(ctx, req)
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

			res["strict_status"] = strings.Contains(resp.Text, "STATUS:")
			res["strict_reason"] = strings.Contains(resp.Text, "REASON:")

			// Weak test to see if reason carries the context semantics
			txtLower := strings.ToLower(resp.Text)
			res["captured_semantics"] = strings.Contains(txtLower, "timeout") || strings.Contains(txtLower, "banco") || strings.Contains(txtLower, "database") || strings.Contains(txtLower, "fora")
		}
		results = append(results, res)
		time.Sleep(1 * time.Second)
	}

	resultsDir := filepath.Join("results", "phase494_adv_lang_deg")
	os.MkdirAll(resultsDir, 0755)

	b, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile(filepath.Join(resultsDir, "results.json"), b, 0644)
	fmt.Println("Phase 494 Done.")
}
