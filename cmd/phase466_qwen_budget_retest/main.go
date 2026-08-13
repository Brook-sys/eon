//go:build ignore

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/provider/openai"
)

type TrialResult struct {
	Model        string  `json:"model"`
	Format       string  `json:"format"`
	MaxTokens    int     `json:"max_tokens"`
	Status       string  `json:"status"`
	Content      string  `json:"content,omitempty"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	FinishReason string  `json:"finish_reason"`
	ElapsedSec   float64 `json:"elapsed_s"`
	Error        string  `json:"error,omitempty"`
}

type Summary struct {
	Timestamp   string        `json:"timestamp"`
	TotalTrials int           `json:"total_trials"`
	Successes   int           `json:"successes"`
	Failures    int           `json:"failures"`
	Results     []TrialResult `json:"results"`
}

func main() {
	apiKey := os.Getenv("GROQ_API_KEY")
	if apiKey == "" {
		fmt.Fprintf(os.Stderr, "GROQ_API_KEY missing\n")
		os.Exit(1)
	}

	prompt := "Two observations report different latency values for the same test window: O-1 says 120ms, O-2 says 410ms. Answer with exactly CONFLICT: YES or CONFLICT: NO on the first line."

	type ConfigSpec struct {
		Format    string
		MaxTokens int
	}

	specs := []ConfigSpec{
		{"parsed", 256},
		{"parsed", 1024},
		{"parsed", 2048},
		{"hidden", 256},
		{"hidden", 1024},
		{"hidden", 2048},
	}

	var results []TrialResult
	successes := 0
	failures := 0

	for _, spec := range specs {
		fmt.Fprintf(os.Stderr, "Executing model=qwen/qwen3.6-27b format=%q max_tokens=%d ...\n", spec.Format, spec.MaxTokens)
		provider, err := openai.New(openai.Config{
			BaseURL:        "https://api.groq.com/openai/v1",
			APIKey:         apiKey,
			Model:          "qwen/qwen3.6-27b",
			MaxOutputField: openai.MaxOutputTokensLegacy,
			Client:         &http.Client{Timeout: 60 * time.Second},
		})
		if err != nil {
			failures++
			results = append(results, TrialResult{
				Model:     "qwen/qwen3.6-27b",
				Format:    spec.Format,
				MaxTokens: spec.MaxTokens,
				Status:    "error",
				Error:     err.Error(),
			})
			continue
		}

		req := port.CompletionRequest{
			Prompt:          prompt,
			MaxOutputTokens: spec.MaxTokens,
			Temperature:     0.0,
			ReasoningEffort: "low",
			ReasoningFormat: spec.Format,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		start := time.Now()
		res, completeErr := provider.Complete(ctx, req)
		elapsed := time.Since(start).Seconds()
		cancel()

		tr := TrialResult{
			Model:      "qwen/qwen3.6-27b",
			Format:     spec.Format,
			MaxTokens:  spec.MaxTokens,
			ElapsedSec: elapsed,
		}

		if completeErr != nil {
			tr.Status = "error"
			tr.Error = completeErr.Error()
			failures++
			fmt.Fprintf(os.Stderr, "  FAIL: %s (%.3fs)\n", completeErr.Error(), elapsed)
		} else {
			tr.Status = "ok"
			tr.Content = strings.TrimSpace(res.Text)
			tr.InputTokens = res.InputTokens
			tr.OutputTokens = res.OutputTokens
			tr.FinishReason = string(res.FinishReason)
			successes++
			fmt.Fprintf(os.Stderr, "  OK: %.3fs in=%d out=%d\n", elapsed, tr.InputTokens, tr.OutputTokens)
		}
		results = append(results, tr)
		time.Sleep(300 * time.Millisecond)
	}

	summary := Summary{
		Timestamp:   time.Now().Format(time.RFC3339),
		TotalTrials: len(results),
		Successes:   successes,
		Failures:    failures,
		Results:     results,
	}

	outDir := "results/phase466_qwen_budget_retest"
	_ = os.MkdirAll(outDir, 0755)
	summaryPath := filepath.Join(outDir, "summary.json")

	data, _ := json.MarshalIndent(summary, "", "  ")
	if err := os.WriteFile(summaryPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write summary: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Campaign finished: %d total, %d success, %d failure. Written to %s\n", len(results), successes, failures, summaryPath)
}
