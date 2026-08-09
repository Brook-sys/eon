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
	Extracted  string        `json:"extracted_status,omitempty"`
	Reasoning  string        `json:"reasoning,omitempty"`
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
		Name          string
		Prompt        string
		ExpectedState string
		MaxTokens     int
	}{
		{
			Name: "English Success Transition",
			Prompt: `The executor initiated a state transition from RUNNING to VERIFIED. No errors were encountered and the verification token was validated. 
Emit the status using exactly this format:
STATUS: [SUCCESS or FAILURE]
REASON: [Brief explanation]`,
			ExpectedState: "SUCCESS",
			MaxTokens:     48,
		},
		{
			Name: "Portuguese Failure Transition",
			Prompt: `O executor tentou uma transição de estado de DISPATCHED para DELIVERED, mas o endpoint remoto retornou timeout (408).
Emita o status usando exatamente este formato:
STATUS: [SUCCESS ou FAILURE]
REASON: [Explicação breve]`,
			ExpectedState: "FAILURE",
			MaxTokens:     48,
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
			for i := 0; i < 3; i++ { // 3 trials per model/scenario
				wg.Add(1)
				go func(m, prov string, scenarioName, p string, exp string, maxTokens int, endpoint, key string) {
					defer wg.Done()
					start := time.Now()

					reqBody := map[string]interface{}{
						"model": m,
						"messages": []map[string]string{
							{"role": "system", "content": "You are a state transition monitor. You classify state changes exactly as requested, under strict token limits."},
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

							if choices, ok := respMap["choices"].([]interface{}); ok && len(choices) > 0 {
								if msg, ok := choices[0].(map[string]interface{})["message"].(map[string]interface{}); ok {
									if content, ok := msg["content"].(string); ok {
										res.RawContent = content

										// USE THE NEW GO INTEGRATION
										status, reason := prompt.ParseStatus(content)
										res.Extracted = status
										res.Reasoning = reason
										res.Success = (status == exp)
									}
								}
							}
						}
					}

					mu.Lock()
					results = append(results, res)
					mu.Unlock()
				}(model.ID, model.Provider, scenario.Name, scenario.Prompt, scenario.ExpectedState, scenario.MaxTokens, model.Endpoint, model.Key)
			}
		}
	}

	wg.Wait()

	outDir := filepath.Join("/home/node/.openclaw/workspace/motor-autonomo/results", "phase439-relaxed_parsing_integration")
	os.MkdirAll(outDir, 0755)
	outFile := filepath.Join(outDir, "manifest.json")
	outData, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile(outFile, outData, 0644)
	fmt.Printf("Completed %d trials. Results saved to %s\n", len(results), outFile)
}
