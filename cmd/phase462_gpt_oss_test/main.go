package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/provider/openai"
)

type result struct {
	Model        string  `json:"model"`
	Effort       string  `json:"effort"`
	Format       string  `json:"format"`
	Status       string  `json:"status"`
	Content      string  `json:"content,omitempty"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	FinishReason string  `json:"finish_reason"`
	ElapsedSec   float64 `json:"elapsed_s"`
	Error        string  `json:"error,omitempty"`
}

func testModel(model string, apiKey string) []result {
	cases := []struct {
		effort string
		format string
	}{
		{"", ""},
		{"none", ""},
		{"low", ""},
		{"", "hidden"},
		{"low", "hidden"},
		{"low", "parsed"},
		{"none", "parsed"},
	}

	prompt := "Two observations report different latency values for the same test and time window: O-1 says 120ms, O-2 says 410ms. Answer with exactly CONFLICT: YES or CONFLICT: NO on the first line."

	var results []result
	for _, tc := range cases {
		fmt.Fprintf(os.Stderr, "Testing %s effort=%q format=%q ...\n", model, tc.effort, tc.format)
		provider, _ := openai.New(openai.Config{
			BaseURL:        "https://api.groq.com/openai/v1",
			APIKey:         apiKey,
			Model:          model,
			MaxOutputField: openai.MaxOutputTokensLegacy,
			Client:         &http.Client{Timeout: 60 * time.Second},
		})

		req := port.CompletionRequest{
			Prompt:          prompt,
			MaxOutputTokens: 256,
			Temperature:     0.0,
			ReasoningEffort: tc.effort,
			ReasoningFormat: tc.format,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		start := time.Now()
		res, err := provider.Complete(ctx, req)
		elapsed := time.Since(start).Seconds()
		cancel()

		r := result{
			Model:      model,
			Effort:     tc.effort,
			Format:     tc.format,
			ElapsedSec: elapsed,
		}

		if err != nil {
			r.Status = "error"
			r.Error = err.Error()
			fmt.Fprintf(os.Stderr, "  ERROR: %s (%.3fs)\n", err.Error(), elapsed)
		} else {
			r.Status = "ok"
			r.Content = strings.TrimSpace(res.Text)
			r.InputTokens = res.InputTokens
			r.OutputTokens = res.OutputTokens
			r.FinishReason = string(res.FinishReason)
			fmt.Fprintf(os.Stderr, "  OK: %.3fs, in=%d out=%d\n", elapsed, r.InputTokens, r.OutputTokens)
		}
		results = append(results, r)
		time.Sleep(500 * time.Millisecond)
	}
	return results
}

func main() {
	apiKey := os.Getenv("GROQ_API_KEY")

	models := []string{
		"openai/gpt-oss-20b",
		"openai/gpt-oss-120b",
	}

	var allResults []result
	for _, model := range models {
		results := testModel(model, apiKey)
		allResults = append(allResults, results...)
	}

	body, _ := json.MarshalIndent(allResults, "", "  ")
	fmt.Println(string(body))
}
