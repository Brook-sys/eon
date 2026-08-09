package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"motor-autonomo/internal/prompt"
)

type TrialResult struct {
	Model      string        `json:"model"`
	Provider   string        `json:"provider"`
	Scenario   string        `json:"scenario"`
	Tokens     int           `json:"tokens_used"`
	Latency    time.Duration `json:"latency"`
	Success    bool          `json:"success"`
	Extracted  string        `json:"extracted_decision,omitempty"`
	Error      string        `json:"error,omitempty"`
	RawContent string        `json:"raw_content,omitempty"`
}

func main() {
	groqKey := os.Getenv("GROQ_API_KEY")
	nimKey := os.Getenv("NVIDIA_NIM_API_KEY")

	if groqKey == "" || nimKey == "" {
		fmt.Println("Error: Missing GROQ_API_KEY or NVIDIA_NIM_API_KEY")
		os.Exit(1)
	}

	scenarios := []struct {
		Name             string
		Prompt           string
		ExpectedDecision string
		MaxTokens        int
	}{
		{
			Name: "English Intent Routing",
			Prompt: `The user says: "Please reset my password." Route this to one of: BILLING, TECH_SUPPORT, SALES.
Emit your decision using exactly this format:
DECISION: [ROUTING_KEY]
REASON: [Brief explanation]`,
			ExpectedDecision: "TECH_SUPPORT",
			MaxTokens:        32,
		},
		{
			Name: "Portuguese Intent Routing",
			Prompt: `O usuário diz: "Quero cancelar minha assinatura." Encaminhe para: BILLING, TECH_SUPPORT, SALES.
Emita sua decisão usando exatamente este formato:
DECISION: [ROUTING_KEY]
REASON: [Explicação breve]`,
			ExpectedDecision: "BILLING",
			MaxTokens:        32,
		},
	}

	models := []struct {
		ID       string
		Provider string
		Endpoint string
		Key      string
	}{
		{"llama-3.1-8b-instant", "groq", "https://api.groq.com/openai/v1/chat/completions", groqKey},
		{"llama-3.3-70b-versatile", "groq", "https://api.groq.com/openai/v1/chat/completions", groqKey},
		{"meta/llama-3.1-8b-instruct", "nim", "https://integrate.api.nvidia.com/v1/chat/completions", nimKey},
	}

	var results []TrialResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, model := range models {
		for _, scenario := range scenarios {
			for i := 0; i < 3; i++ { // 3 trials
				wg.Add(1)
				go func(m, prov, scenarioName, p string, exp string, maxTokens int, endpoint, key string) {
					defer wg.Done()
					start := time.Now()

					reqBody := map[string]interface{}{
						"model": m,
						"messages": []map[string]string{
							{"role": "system", "content": "You are a strict routing engine. You output routing decisions exactly as requested."},
							{"role": "user", "content": p},
						},
						"max_tokens":  maxTokens,
						"temperature": 0.1,
					}

					jsonData, _ := json.Marshal(reqBody)
					req, _ := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
					req.Header.Set("Authorization", "Bearer "+key)
					req.Header.Set("Content-Type", "application/json")

					client := &http.Client{Timeout: 15 * time.Second}
					resp, err := client.Do(req)
					latency := time.Since(start)

					res := TrialResult{
						Model:    m,
						Provider: prov,
						Scenario: scenarioName,
						Latency:  latency,
					}

					if err != nil {
						res.Error = err.Error()
					} else {
						defer resp.Body.Close()
						body, _ := io.ReadAll(resp.Body)

						if resp.StatusCode != 200 {
							res.Error = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body))
						} else {
							var respMap map[string]interface{}
							json.Unmarshal(body, &respMap)
							if usage, ok := respMap["usage"].(map[string]interface{}); ok {
								if total, ok := usage["total_tokens"].(float64); ok {
									res.Tokens = int(total)
								}
							}

							if choices, ok := respMap["choices"].([]interface{}); ok && len(choices) > 0 {
								if msg, ok := choices[0].(map[string]interface{})["message"].(map[string]interface{}); ok {
									if content, ok := msg["content"].(string); ok {
										res.RawContent = content

										decision, found := prompt.ParseDecision(content)
										res.Extracted = decision
										res.Success = found && (decision == exp)
									}
								}
							}
						}
					}

					mu.Lock()
					results = append(results, res)
					mu.Unlock()
				}(model.ID, model.Provider, scenario.Name, scenario.Prompt, scenario.ExpectedDecision, scenario.MaxTokens, model.Endpoint, model.Key)
			}
		}
	}

	wg.Wait()

	outDir := filepath.Join("/home/node/.openclaw/workspace/motor-autonomo/results", "phase441-relaxed_parsing_decision_fire_test")
	os.MkdirAll(outDir, 0755)
	outFile := filepath.Join(outDir, "manifest.json")
	outData, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile(outFile, outData, 0644)

	successes := 0
	for _, r := range results {
		if r.Success {
			successes++
		}
	}
	fmt.Printf("Completed %d trials. %d/%d (%.1f%%) Success. Results saved to %s\n", len(results), successes, len(results), float64(successes)/float64(len(results))*100, outFile)
}
