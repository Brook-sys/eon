package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/provider/openai"
)

type Trial struct {
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	Phase       string `json:"phase"`
	Prompt      string `json:"prompt"`
	MaxTokens   int    `json:"max_tokens"`
	StatusCode  int    `json:"status_code"`
	FinishRsn   string `json:"finish_reason"`
	Output      string `json:"output"`
	InputTokens int    `json:"input_tokens"`
	OutTokens   int    `json:"output_tokens"`
	LatencyMs   int64  `json:"latency_ms"`
	Success     bool   `json:"success"`
	Error       string `json:"error,omitempty"`
}

func main() {
	groqKey := os.Getenv("GROQ_API_KEY")
	nimKey := os.Getenv("NVIDIA_NIM_API_KEY")

	if groqKey == "" || nimKey == "" {
		fmt.Println("Missing provider API keys in environment!")
		os.Exit(1)
	}

	targets := []struct {
		Provider string
		BaseURL  string
		Key      string
		Model    string
	}{
		{"groq", "https://api.groq.com/openai/v1", groqKey, "allam-2-7b"},
		{"groq", "https://api.groq.com/openai/v1", groqKey, "llama-3.3-70b-versatile"},
		{"nim", "https://integrate.api.nvidia.com/v1", nimKey, "meta/llama-3.1-8b-instruct"},
	}

	promptOriginal := "Analyze system log: [2026-08-13 10:00:01] DB connection pool exhausted. [2026-08-13 10:00:02] Query succeeded. Determine if there is a blocking error. Output strictly STATUS: READY if non-blocking or STATUS: ERROR if blocking."

	var trials []Trial

	for _, target := range targets {
		fmt.Printf("=== Testing %s / %s ===\n", target.Provider, target.Model)
		p, err := openai.New(openai.Config{
			BaseURL: target.BaseURL,
			APIKey:  target.Key,
			Model:   target.Model,
		})
		if err != nil {
			fmt.Printf("Failed to init provider: %v\n", err)
			continue
		}

		// Stage 1: Starvation probe (max_tokens = 3)
		t0 := time.Now()
		req1 := port.CompletionRequest{
			Prompt:          promptOriginal,
			MaxOutputTokens: 3,
			Temperature:     0.0,
		}
		res1, err1 := p.Complete(context.Background(), req1)
		lat1 := time.Since(t0).Milliseconds()

		tr1 := Trial{
			Provider:  target.Provider,
			Model:     target.Model,
			Phase:     "starvation_initial",
			Prompt:    req1.Prompt,
			MaxTokens: 3,
			LatencyMs: lat1,
		}
		if err1 != nil {
			tr1.Error = err1.Error()
			tr1.StatusCode = 500
		} else {
			tr1.StatusCode = 200
			tr1.FinishRsn = string(res1.FinishReason)
			tr1.Output = res1.Text
			tr1.InputTokens = res1.InputTokens
			tr1.OutTokens = res1.OutputTokens
			tr1.Success = string(res1.FinishReason) == "length" || res1.Text != ""
		}
		trials = append(trials, tr1)
		fmt.Printf(" [Starve 3t] Lat: %dms | Finish: %s | Output: %q\n", lat1, tr1.FinishRsn, tr1.Output)

		time.Sleep(300 * time.Millisecond)

		// Stage 2: Starvation Recovery with RECOVERY N prefix (max_tokens = 40)
		recoveryPrompt := fmt.Sprintf("RECOVERY 1: %s\nPlease strictly follow instructions and ensure the response fits within the new budget.", promptOriginal)
		t0 = time.Now()
		req2 := port.CompletionRequest{
			Prompt:          recoveryPrompt,
			MaxOutputTokens: 40,
			Temperature:     0.0,
		}
		res2, err2 := p.Complete(context.Background(), req2)
		lat2 := time.Since(t0).Milliseconds()

		tr2 := Trial{
			Provider:  target.Provider,
			Model:     target.Model,
			Phase:     "starvation_recovery",
			Prompt:    req2.Prompt,
			MaxTokens: 40,
			LatencyMs: lat2,
		}
		if err2 != nil {
			tr2.Error = err2.Error()
			tr2.StatusCode = 500
		} else {
			tr2.StatusCode = 200
			tr2.FinishRsn = string(res2.FinishReason)
			tr2.Output = res2.Text
			tr2.InputTokens = res2.InputTokens
			tr2.OutTokens = res2.OutputTokens
			tr2.Success = string(res2.FinishReason) == "stop" && (res2.Text == "STATUS: READY" || res2.Text == "STATUS: READY." || res2.Text == "STATUS: ERROR")
		}
		trials = append(trials, tr2)
		fmt.Printf(" [Recovery 40t] Lat: %dms | Finish: %s | Match: %t | Output: %q\n", lat2, tr2.FinishRsn, tr2.Success, tr2.Output)

		time.Sleep(500 * time.Millisecond)
	}

	os.MkdirAll("results/phase489_starvation_recovery", 0755)
	data, _ := json.MarshalIndent(trials, "", "  ")
	os.WriteFile("results/phase489_starvation_recovery/results.json", data, 0644)
	fmt.Printf("\nSaved %d trials to results/phase489_starvation_recovery/results.json\n", len(trials))
}
