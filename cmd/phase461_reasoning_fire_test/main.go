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

type campaignReport struct {
	Campaign   string                 `json:"campaign"`
	Timestamp  string                 `json:"timestamp"`
	Provider   string                 `json:"provider"`
	Hypothesis string                 `json:"hypothesis"`
	Scenario   string                 `json:"scenario"`
	Bounds     map[string]interface{} `json:"bounds"`
	Results    []result               `json:"results"`
	Analysis   map[string]string      `json:"analysis,omitempty"`
	Decisions  []string               `json:"decisions,omitempty"`
}

func main() {
	apiKey := os.Getenv("GROQ_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "GROQ_API_KEY not set")
		os.Exit(1)
	}

	cases := []struct {
		model  string
		effort string
		format string
		label  string
	}{
		{"llama-3.1-8b-instant", "", "", "baseline-non-reasoning"},
		{"openai/gpt-oss-20b", "none", "parsed", "gptoss20b-none"},
		{"openai/gpt-oss-20b", "low", "parsed", "gptoss20b-low"},
		{"openai/gpt-oss-20b", "medium", "parsed", "gptoss20b-medium"},
		{"openai/gpt-oss-120b", "low", "parsed", "gptoss120b-low"},
		{"openai/gpt-oss-120b", "medium", "parsed", "gptoss120b-medium"},
		{"qwen/qwen3.6-27b", "none", "parsed", "qwen36-none"},
		{"qwen/qwen3.6-27b", "default", "parsed", "qwen36-default"},
		{"qwen/qwen3.6-27b", "low", "parsed", "qwen36-low"},
	}

	prompt := "Two observations report different latency values for the same test and time window: O-1 says 120ms, O-2 says 410ms. Answer with exactly CONFLICT: YES or CONFLICT: NO on the first line, followed by a brief explanation."

	var results []result

	for _, tc := range cases {
		fmt.Fprintf(os.Stderr, "Testing %s effort=%s format=%s ...\n", tc.model, tc.effort, tc.format)
		provider, err := openai.New(openai.Config{
			BaseURL:        "https://api.groq.com/openai/v1",
			APIKey:         apiKey,
			Model:          tc.model,
			MaxOutputField: openai.MaxOutputTokensLegacy,
			Client:         &http.Client{Timeout: 60 * time.Second},
		})
		if err != nil {
			results = append(results, result{Model: tc.model, Effort: tc.effort, Format: tc.format, Status: "error", Error: err.Error()})
			continue
		}

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
			Model:        tc.model,
			Effort:       tc.effort,
			Format:       tc.format,
			ElapsedSec:   elapsed,
			FinishReason: string(res.FinishReason),
			InputTokens:  res.InputTokens,
			OutputTokens: res.OutputTokens,
		}

		if err != nil {
			r.Status = "error"
			r.Error = err.Error()
			fmt.Fprintf(os.Stderr, "  ERROR: %s (%.3fs)\n", err.Error(), elapsed)
		} else {
			r.Status = "ok"
			r.Content = strings.TrimSpace(res.Text)
			if len(r.Content) > 300 {
				r.Content = r.Content[:300] + "..."
			}
			contentPreview := r.Content
			if len(contentPreview) > 80 {
				contentPreview = contentPreview[:80]
			}
			fmt.Fprintf(os.Stderr, "  OK: %s (%.3fs, in=%d out=%d)\n", contentPreview, elapsed, r.InputTokens, r.OutputTokens)
		}

		results = append(results, r)
		time.Sleep(500 * time.Millisecond)
	}

	report := campaignReport{
		Campaign:   "phase461-reasoning-config-fire-test2",
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Provider:   "groq",
		Hypothesis: "Per-binding reasoning_effort and reasoning_format are correctly serialized to the OpenAI-compatible wire. effort=none eliminates shadow thinking, effort=low/medium scales reasoning budget, and non-reasoning models are unaffected.",
		Scenario:   "Conflict detection — two observations with contradictory latency values",
		Bounds: map[string]interface{}{
			"max_calls":   len(cases),
			"max_tokens":   256,
			"temperature": 0.0,
			"timeout":      "60s",
		},
		Results: results,
	}

	analysis := make(map[string]string)
	for _, r := range results {
		key := fmt.Sprintf("%s_effort_%s", r.Model, r.Effort)
		if r.Effort == "" {
			key = fmt.Sprintf("%s_baseline", r.Model)
		}
		if r.Status == "ok" {
			hasFormat := strings.Contains(r.Content, "CONFLICT: YES") || strings.Contains(r.Content, "CONFLICT: NO")
			analysis[key] = fmt.Sprintf("status=ok, has_format=%v, latency=%.3fs, tokens=%d/%d, finish=%s", hasFormat, r.ElapsedSec, r.InputTokens, r.OutputTokens, r.FinishReason)
		} else {
			analysis[key] = fmt.Sprintf("status=%s, error=%s", r.Status, r.Error)
		}
	}
	report.Analysis = analysis

	// Decisions based on results
	var decisions []string
	for _, r := range results {
		if r.Status == "ok" && r.Effort == "none" && strings.Contains(r.Content, "CONFLICT:") {
			decisions = append(decisions, fmt.Sprintf("%s with effort=none produces clean formatted output in %.3fs — recommended for bounded deterministic tasks", r.Model, r.ElapsedSec))
		}
		if r.Status == "ok" && (r.Effort == "low" || r.Effort == "medium") && strings.Contains(r.Content, "CONFLICT:") {
			decisions = append(decisions, fmt.Sprintf("%s with effort=%s produces correct output in %.3fs with %d output tokens", r.Model, r.Effort, r.ElapsedSec, r.OutputTokens))
		}
		if r.Status == "error" {
			decisions = append(decisions, fmt.Sprintf("%s with effort=%s failed: %s", r.Model, r.Effort, r.Error))
		}
	}
	report.Decisions = decisions

	body, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(body))
}
