//go:build ignore

// Phase 457 — Dashboard Retention Panel Live Fire Campaign
//
// Hypothesis: All available Groq text-completion models and NVIDIA NIM models
// can produce valid structured retention-pressure assessment JSON under strict
// format constraints, budget starvation (low max_tokens), and mixed-language prompts.
//
// Bounds: max 30 external calls total, 45s deadline per call, max 128 tokens output.
// Models: llama-3.3-70b-versatile, llama-3.1-8b-instant, qwen/qwen3.6-27b,
//         openai/gpt-oss-20b, openai/gpt-oss-120b, allam-2-7b (Groq)
//         + deepseek-ai/deepseek-v4-flash-0731, meta/llama-3.1-8b-instruct (NIM)
//
// Adversarial scenarios tested:
//   1. Strict JSON extraction under budget starvation (max_tokens=32)
//   2. Mixed PT-BR/EN instruction confusion
//   3. Contradictory data in system prompt

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	Model           string    `json:"model"`
	Messages        []Message `json:"messages"`
	MaxTokens       int       `json:"max_tokens"`
	Temperature     float64   `json:"temperature"`
	ReasoningEffort string    `json:"reasoning_effort,omitempty"`
}

type Choice struct {
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
type Response struct {
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type TrialResult struct {
	Provider         string  `json:"provider"`
	Model            string  `json:"model"`
	Scenario         string  `json:"scenario"`
	LatencyMs        float64 `json:"latency_ms"`
	Status           string  `json:"status"` // ok, format_error, provider_error, timeout
	FinishReason     string  `json:"finish_reason"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	Output           string  `json:"output"`
	JSONValid        bool    `json:"json_valid"`
	Error            string  `json:"error,omitempty"`
}

func callModel(ctx context.Context, baseURL, apiKey, model string, msgs []Message, maxTokens int, reasoning string) (*Response, time.Duration, error) {
	req := Request{
		Model:       model,
		Messages:    msgs,
		MaxTokens:   maxTokens,
		Temperature: 0.0,
	}
	if reasoning != "" {
		req.ReasoningEffort = reasoning
	}
	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("User-Agent", "motor-autonomo/phase457-live-fire")

	start := time.Now()
	resp, err := http.DefaultClient.Do(httpReq)
	elapsed := time.Since(start)
	if err != nil {
		return nil, elapsed, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 32768))
	if err != nil {
		return nil, elapsed, fmt.Errorf("read: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, elapsed, fmt.Errorf("status %d: %s", resp.StatusCode, string(data))
	}

	var result Response
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, elapsed, fmt.Errorf("decode: %w (%s)", err, string(data))
	}
	return &result, elapsed, nil
}

func isValidJSON(s string) bool {
	s = strings.TrimSpace(s)
	var v interface{}
	return json.Unmarshal([]byte(s), &v) == nil
}

func main() {
	groqKey := os.Getenv("GROQ_API_KEY")
	nimKey := os.Getenv("NVIDIA_NIM_API_KEY")
	if groqKey == "" || nimKey == "" {
		fmt.Fprintln(os.Stderr, "FATAL: missing GROQ_API_KEY or NVIDIA_NIM_API_KEY")
		os.Exit(1)
	}

	type ModelSpec struct {
		Provider  string
		BaseURL   string
		APIKey    string
		Model     string
		Reasoning string // for gpt-oss models
	}

	models := []ModelSpec{
		{"groq", "https://api.groq.com/openai/v1", groqKey, "llama-3.3-70b-versatile", ""},
		{"groq", "https://api.groq.com/openai/v1", groqKey, "llama-3.1-8b-instant", ""},
		{"groq", "https://api.groq.com/openai/v1", groqKey, "qwen/qwen3.6-27b", "none"},
		{"groq", "https://api.groq.com/openai/v1", groqKey, "openai/gpt-oss-20b", "low"},
		{"groq", "https://api.groq.com/openai/v1", groqKey, "openai/gpt-oss-120b", "low"},
		{"groq", "https://api.groq.com/openai/v1", groqKey, "allam-2-7b", ""},
		{"nim", "https://integrate.api.nvidia.com/v1", nimKey, "deepseek-ai/deepseek-v4-flash-0731", ""},
		{"nim", "https://integrate.api.nvidia.com/v1", nimKey, "meta/llama-3.1-8b-instruct", ""},
	}

	type Scenario struct {
		Name      string
		System    string
		User      string
		MaxTokens int
	}

	scenarios := []Scenario{
		{
			Name:      "strict_json_retention_status",
			System:    "You are a retention pressure assessment engine. Output ONLY valid JSON, no explanation.",
			User:      `Assess the following event log state and respond with a JSON object: {"event_head": 0, "pressure": "normal", "prune_authorized": false, "finding": "empty log"}. Event log: empty, 0 events, 0 artifacts.`,
			MaxTokens: 64,
		},
		{
			Name:      "budget_starvation_json",
			System:    "Output ONLY valid JSON with exactly these keys: event_head, pressure, status. No markdown, no explanation.",
			User:      `Event head sequence: 42, pressure: warn, 3 stale artifacts. Respond JSON only.`,
			MaxTokens: 32,
		},
		{
			Name:      "mixed_language_instruction",
			System:    "Você é um avaliador de pressão de retenção. Respond ONLY in valid JSON format with keys: event_head, pressure, prune_ok.",
			User:      `Analise: event_head_sequence=100, pressão alta, 12 artefatos obsoletos de 50 total. Prune não autorizado. Reply strictly as JSON.`,
			MaxTokens: 64,
		},
	}

	var results []TrialResult
	totalCalls := 0
	const maxCalls = 30

	for _, model := range models {
		for _, scenario := range scenarios {
			if totalCalls >= maxCalls {
				fmt.Printf("BUDGET CAP: stopping at %d calls\n", totalCalls)
				goto done
			}

			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			msgs := []Message{
				{Role: "system", Content: scenario.System},
				{Role: "user", Content: scenario.User},
			}

			fmt.Printf("[%s/%s] scenario=%s max_tokens=%d ... ", model.Provider, model.Model, scenario.Name, scenario.MaxTokens)
			resp, elapsed, err := callModel(ctx, model.BaseURL, model.APIKey, model.Model, msgs, scenario.MaxTokens, model.Reasoning)
			cancel()

			result := TrialResult{
				Provider:  model.Provider,
				Model:     model.Model,
				Scenario:  scenario.Name,
				LatencyMs: float64(elapsed.Milliseconds()),
			}

			if err != nil {
				result.Status = "provider_error"
				result.Error = err.Error()
				fmt.Printf("ERROR (%.1fms): %s\n", result.LatencyMs, err)
			} else if len(resp.Choices) == 0 {
				result.Status = "provider_error"
				result.Error = "empty choices"
				fmt.Printf("ERROR: empty choices\n")
			} else {
				output := strings.TrimSpace(resp.Choices[0].Message.Content)
				result.Output = output
				result.FinishReason = resp.Choices[0].FinishReason
				result.PromptTokens = resp.Usage.PromptTokens
				result.CompletionTokens = resp.Usage.CompletionTokens
				result.JSONValid = isValidJSON(output)

				if result.JSONValid {
					result.Status = "ok"
					fmt.Printf("OK (%.1fms, %d+%d tok, %s, json=true)\n",
						result.LatencyMs, result.PromptTokens, result.CompletionTokens, result.FinishReason)
				} else {
					result.Status = "format_error"
					truncated := output
					if len(truncated) > 80 {
						truncated = truncated[:80] + "..."
					}
					fmt.Printf("FORMAT_ERR (%.1fms, %d+%d tok, %s): %s\n",
						result.LatencyMs, result.PromptTokens, result.CompletionTokens, result.FinishReason, truncated)
				}
			}

			results = append(results, result)
			totalCalls++

			// Small delay between calls to be respectful
			time.Sleep(200 * time.Millisecond)
		}
	}

done:
	// Summary
	fmt.Println("\n=== CAMPAIGN SUMMARY ===")
	okCount, fmtErr, provErr := 0, 0, 0
	for _, r := range results {
		switch r.Status {
		case "ok":
			okCount++
		case "format_error":
			fmtErr++
		default:
			provErr++
		}
	}
	fmt.Printf("Total calls: %d | OK: %d | Format errors: %d | Provider errors: %d\n", len(results), okCount, fmtErr, provErr)

	// Write detailed results
	os.MkdirAll("results/phase457-dashboard_retention_live_fire", 0755)
	out, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile("results/phase457-dashboard_retention_live_fire/summary.json", out, 0644)
	fmt.Println("Results written to results/phase457-dashboard_retention_live_fire/summary.json")
}
