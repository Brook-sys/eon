//go:build ignore

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"motor-autonomo/internal/modeltext"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/provider/openai"
)

type testCase struct {
	name            string
	model           string
	reasoningEffort string
	prompt          string
	expectedKeys    []string
}

type resultRow struct {
	Model           string            `json:"model"`
	ReasoningEffort string            `json:"reasoning_effort"`
	ProbeConfirmed  bool              `json:"probe_confirmed"`
	LatencyMs       int64             `json:"latency_ms"`
	InputTokens     int               `json:"input_tokens"`
	OutputTokens    int               `json:"output_tokens"`
	StrippedThought bool              `json:"stripped_thought"`
	ParsedValues    map[string]string `json:"parsed_values"`
	UsedFallback    bool              `json:"used_fallback"`
	Error           string            `json:"error,omitempty"`
}

func main() {
	groqKey := os.Getenv("GROQ_API_KEY")
	if groqKey == "" {
		fmt.Println("GROQ_API_KEY is not set")
		os.Exit(1)
	}
	nimKey := os.Getenv("NVIDIA_NIM_API_KEY")

	cases := []testCase{
		{
			name:            "weekday-baseline",
			model:           "llama-3.3-70b-versatile",
			reasoningEffort: "",
			prompt:          "Task: Determine if 2026-08-08 is a Saturday. Output format strictly:\nDATE: 2026-08-08\nWEEKEND: YES or NO",
			expectedKeys:    []string{"DATE", "WEEKEND"},
		},
		{
			name:            "weekday-effort-none",
			model:           "qwen/qwen3.6-27b",
			reasoningEffort: "none",
			prompt:          "Task: Determine if 2026-08-08 is a Saturday. Output format strictly:\nDATE: 2026-08-08\nWEEKEND: YES or NO",
			expectedKeys:    []string{"DATE", "WEEKEND"},
		},
		{
			name:            "weekday-baseline-8b",
			model:           "llama-3.1-8b-instant",
			reasoningEffort: "",
			prompt:          "Task: Determine if 2026-08-08 is a Saturday. Output format strictly:\nDATE: 2026-08-08\nWEEKEND: YES or NO",
			expectedKeys:    []string{"DATE", "WEEKEND"},
		},
		{
			name:            "conflict-detection-70b",
			model:           "llama-3.3-70b-versatile",
			reasoningEffort: "",
			prompt:          "Two sensors report latency for the same window. Sensor A says 120ms. Sensor B says 410ms. Is there a conflict?\nOutput format strictly:\nCONFLICT: YES or NO\nREASON: one sentence",
			expectedKeys:    []string{"CONFLICT", "REASON"},
		},
		{
			name:            "conflict-detection-8b",
			model:           "llama-3.1-8b-instant",
			reasoningEffort: "",
			prompt:          "Two sensors report latency for the same window. Sensor A says 120ms. Sensor B says 410ms. Is there a conflict?\nOutput format strictly:\nCONFLICT: YES or NO\nREASON: one sentence",
			expectedKeys:    []string{"CONFLICT", "REASON"},
		},
		{
			name:            "conflict-detection-qwen",
			model:           "qwen/qwen3.6-27b",
			reasoningEffort: "none",
			prompt:          "Two sensors report latency for the same window. Sensor A says 120ms. Sensor B says 410ms. Is there a conflict?\nOutput format strictly:\nCONFLICT: YES or NO\nREASON: one sentence",
			expectedKeys:    []string{"CONFLICT", "REASON"},
		},
		{
			name:            "thinking-tags-resilience",
			model:           "qwen/qwen3.6-27b",
			reasoningEffort: "",
			prompt:          "Analyze the following and respond in strict format.\nQuestion: Was 2026 a leap year?\nThink step by step internally, but output ONLY:\nDATE: 2026\nLEAP: YES or NO",
			expectedKeys:    []string{"DATE", "LEAP"},
		},
	}

	// Add NIM control if key available
	if nimKey != "" {
		cases = append(cases, testCase{
			name:            "nim-control-8b",
			model:           "meta/llama-3.1-8b-instruct",
			reasoningEffort: "",
			prompt:          "Task: Determine if 2026-08-08 is a Saturday. Output format strictly:\nDATE: 2026-08-08\nWEEKEND: YES or NO",
			expectedKeys:    []string{"DATE", "WEEKEND"},
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	fmt.Println("=== Phase 390 Reasoning Effort & Structured Parsing Live Fire Test ===")
	fmt.Printf("Models under test: %d cases\n", len(cases))

	results := make([]resultRow, 0, len(cases))

	for _, tc := range cases {
		baseURL := "https://api.groq.com/openai/v1"
		apiKey := groqKey
		if tc.model == "meta/llama-3.1-8b-instruct" {
			baseURL = "https://integrate.api.nvidia.com/v1"
			apiKey = nimKey
		}

		p, err := openai.New(openai.Config{
			BaseURL: baseURL,
			APIKey:  apiKey,
			Model:   tc.model,
		})
		if err != nil {
			fmt.Printf("Model %s provider init error: %v\n", tc.model, err)
			results = append(results, resultRow{Model: tc.model, Error: fmt.Sprintf("init: %v", err)})
			continue
		}

		prof, probeErr := p.Probe(ctx)
		probeOK := probeErr == nil && prof.TextToTextConfirmed

		start := time.Now()
		req := port.CompletionRequest{
			Prompt:          tc.prompt,
			Temperature:     0.0,
			MaxOutputTokens: 256,
			ReasoningEffort: tc.reasoningEffort,
		}

		resp, err := p.Complete(ctx, req)
		latency := time.Since(start)

		row := resultRow{
			Model:           tc.model,
			ReasoningEffort: tc.reasoningEffort,
			ProbeConfirmed:  probeOK,
			LatencyMs:       latency.Milliseconds(),
		}

		if err != nil {
			row.Error = err.Error()
			fmt.Printf("[%s] %s: ERROR (%v)\n", tc.name, tc.model, err)
			results = append(results, row)
			continue
		}

		row.InputTokens = resp.InputTokens
		row.OutputTokens = resp.OutputTokens

		unthought := modeltext.StripThinkingTags(resp.Text)
		row.StrippedThought = unthought.Changed

		parsed := prompt.ParseResponse(resp.Text, tc.expectedKeys)
		row.ParsedValues = parsed.Values
		row.UsedFallback = parsed.UsedFallback

		fmt.Printf("\n--- [%s] Model: %s (Probe: %v) ---\n", tc.name, tc.model, probeOK)
		fmt.Printf("Latency: %v | InTokens: %d | OutTokens: %d\n", latency, resp.InputTokens, resp.OutputTokens)
		fmt.Printf("Raw text len: %d | Thought Stripped: %v | UsedFallback: %v\n", len(resp.Text), unthought.Changed, parsed.UsedFallback)
		for k, v := range row.ParsedValues {
			fmt.Printf("  %s: %q\n", k, v)
		}

		results = append(results, row)
	}

	// Write JSON results
	outDir := "results/phase390-reasoning-effort"
	_ = os.MkdirAll(outDir, 0755)
	jsonData, _ := json.MarshalIndent(results, "", "  ")
	_ = os.WriteFile(outDir+"/results.json", jsonData, 0644)

	fmt.Println("\n=== Campaign Complete ===")
	fmt.Printf("Results written to %s/results.json\n", outDir)
}
