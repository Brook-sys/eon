//go:build ignore

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/provider/openai"
)

type Target struct {
	Name     string
	Model    string
	BaseURL  string
	APIKey   string
	Thinking int
}

type Scenario struct {
	Name          string
	MaxTokens     int
	Task          string
	AnswerFormat  string
	FormatExample string
	ExpectedKey   string
	ExpectedVal   string
}

type TrialResult struct {
	Target                    string `json:"target"`
	Model                     string `json:"model"`
	Scenario                  string `json:"scenario"`
	Rep                       int    `json:"rep"`
	MaxOutputTokens           int    `json:"max_output_tokens"`
	CalculatedMinOutput       int    `json:"calculated_min_output"`
	ThinkingOverhead          int    `json:"thinking_overhead"`
	ReasoningEffortSuppressed bool   `json:"reasoning_effort_suppressed"`
	ReasoningEffortSent       string `json:"reasoning_effort_sent"`
	LatencyMs                 int64  `json:"latency_ms"`
	InputTokens               int    `json:"input_tokens"`
	OutputTokens              int    `json:"output_tokens"`
	FinishReason              string `json:"finish_reason"`
	RawContent                string `json:"raw_content"`
	FormatOK                  bool   `json:"format_ok"`
	SemanticOK                bool   `json:"semantic_ok"`
	Error                     string `json:"error,omitempty"`
}

func main() {
	groqKey := os.Getenv("GROQ_API_KEY")
	nimKey := os.Getenv("NVIDIA_NIM_API_KEY")
	if groqKey == "" {
		log.Fatal("GROQ_API_KEY is required")
	}

	targets := []Target{
		{
			Name:     "qwen3.6-27b",
			Model:    "qwen/qwen3.6-27b",
			BaseURL:  "https://api.groq.com/openai/v1",
			APIKey:   groqKey,
			Thinking: 640,
		},
		{
			Name:     "llama3.3-70b",
			Model:    "llama-3.3-70b-versatile",
			BaseURL:  "https://api.groq.com/openai/v1",
			APIKey:   groqKey,
			Thinking: 0,
		},
		{
			Name:     "gpt-oss-20b",
			Model:    "openai/gpt-oss-20b",
			BaseURL:  "https://api.groq.com/openai/v1",
			APIKey:   groqKey,
			Thinking: 128,
		},
		{
			Name:     "llama3.1-8b",
			Model:    "llama-3.1-8b-instant",
			BaseURL:  "https://api.groq.com/openai/v1",
			APIKey:   groqKey,
			Thinking: 0,
		},
	}

	if nimKey != "" {
		targets = append(targets, Target{
			Name:     "nim-deepseek-v4",
			Model:    "deepseek-ai/deepseek-v4-flash-0731",
			BaseURL:  "https://integrate.api.nvidia.com/v1",
			APIKey:   nimKey,
			Thinking: 0,
		})
	}

	scenarios := []Scenario{
		{
			Name:          "tight-64-simple",
			MaxTokens:     64,
			Task:          "Extract event date and source",
			AnswerFormat:  "DATE: <YYYY-MM-DD>\nSOURCE: <name>",
			FormatExample: "DATE: 2026-08-08\nSOURCE: Groq",
			ExpectedKey:   "DATE",
			ExpectedVal:   "2026-08-08",
		},
		{
			Name:          "moderate-256-simple",
			MaxTokens:     256,
			Task:          "Extract event date and source",
			AnswerFormat:  "DATE: <YYYY-MM-DD>\nSOURCE: <name>",
			FormatExample: "DATE: 2026-08-08\nSOURCE: Groq",
			ExpectedKey:   "DATE",
			ExpectedVal:   "2026-08-08",
		},
		{
			Name:          "comfortable-512-simple",
			MaxTokens:     512,
			Task:          "Extract event date and source",
			AnswerFormat:  "DATE: <YYYY-MM-DD>\nSOURCE: <name>",
			FormatExample: "DATE: 2026-08-08\nSOURCE: Groq",
			ExpectedKey:   "DATE",
			ExpectedVal:   "2026-08-08",
		},
	}

	ctx := context.Background()
	var results []TrialResult

	fmt.Printf("=== Phase 393 Adaptive Reasoning Effort Campaign ===\n")
	fmt.Printf("Targets: %d, Scenarios: %d, Reps: 3\n", len(targets), len(scenarios))

	for _, target := range targets {
		provider, err := openai.New(openai.Config{
			BaseURL: target.BaseURL,
			Model:   target.Model,
			APIKey:  target.APIKey,
		})
		if err != nil {
			log.Fatalf("Failed to create provider for %s: %v", target.Name, err)
		}

		compiler := prompt.Compiler{
			Estimator:             prompt.ConservativeEstimator{},
			ProviderContextTokens: 8192,
		}

		for _, scenario := range scenarios {
			for rep := 0; rep < 3; rep++ {
				spec := domain.OperationSpec{
					SchemaVersion:    domain.SchemaVersionV1,
					ID:               domain.OperationSpecID(fmt.Sprintf("phase393-%s-%s-r%d", target.Name, scenario.Name, rep)),
					ContractVersion:  1,
					TemplateVersion:  1,
					InputSchema:      "task.v1",
					OutputSchema:     "text.v1",
					Budget:           domain.Budget{Tokens: 2048, ModelCalls: 1},
					MaxOutputTokens:  scenario.MaxTokens,
					SafetyMargin:     128,
					Validators:       []string{"strict_format"},
					RetryPolicy:      "none",
					FallbackPolicy:   "none",
					MaximumAuthority: domain.AuthorityProposeOnly,
				}

				compileInput := prompt.Input{
					Task:           scenario.Task,
					AnswerFormat:   scenario.AnswerFormat,
					AllowedOutputs: []string{scenario.AnswerFormat},
					FormatExample:  scenario.FormatExample,
					Facts: []prompt.Fact{
						{ID: "f1", Text: "The conference was announced on 2026-08-08 by Groq.", Required: true},
					},
					ThinkingOverheadTokens: target.Thinking,
				}

				compiled, compileErr := compiler.Compile(spec, compileInput)
				if compileErr != nil {
					res := TrialResult{
						Target:   target.Name,
						Model:    target.Model,
						Scenario: scenario.Name,
						Rep:      rep,
						Error:    compileErr.Error(),
					}
					results = append(results, res)
					fmt.Printf("  %s / %s / r%d → COMPILE ERROR: %v\n", target.Name, scenario.Name, rep, compileErr)
					continue
				}

				req := compiled.Request
				if target.Thinking > 0 && !compiled.ReasoningEffortSuppressed {
					req.ReasoningEffort = "medium"
				}

				start := time.Now()
				compRes, compErr := provider.Complete(ctx, req)
				latency := time.Since(start).Milliseconds()

				if compErr != nil {
					if (target.Name == "gpt-oss-20b" || target.Name == "llama3.1-8b") && req.ReasoningEffort != "" {
						// Retry without reasoning effort if rejected by HTTP 400
						req.ReasoningEffort = ""
						compRes, compErr = provider.Complete(ctx, req)
					}
					if compErr != nil {
						res := TrialResult{
							Target:                    target.Name,
							Model:                     target.Model,
							Scenario:                  scenario.Name,
							Rep:                       rep,
							MaxOutputTokens:           scenario.MaxTokens,
							CalculatedMinOutput:       compiled.MinOutputTokens,
							ThinkingOverhead:          target.Thinking,
							ReasoningEffortSuppressed: compiled.ReasoningEffortSuppressed,
							ReasoningEffortSent:       req.ReasoningEffort,
							LatencyMs:                 latency,
							Error:                     compErr.Error(),
						}
						results = append(results, res)
						fmt.Printf("  %s / %s / r%d → CALL ERROR (%dms): %v\n", target.Name, scenario.Name, rep, latency, compErr)
						time.Sleep(300 * time.Millisecond)
						continue
					}
				}

				parsed := prompt.ParseResponse(compRes.Text, []string{"DATE", "SOURCE"})
				formatOK := len(parsed.Values) > 0
				semanticOK := false
				if formatOK {
					if val, ok := parsed.Values[scenario.ExpectedKey]; ok && val == scenario.ExpectedVal {
						semanticOK = true
					}
				}

				res := TrialResult{
					Target:                    target.Name,
					Model:                     target.Model,
					Scenario:                  scenario.Name,
					Rep:                       rep,
					MaxOutputTokens:           scenario.MaxTokens,
					CalculatedMinOutput:       compiled.MinOutputTokens,
					ThinkingOverhead:          target.Thinking,
					ReasoningEffortSuppressed: compiled.ReasoningEffortSuppressed,
					ReasoningEffortSent:       req.ReasoningEffort,
					LatencyMs:                 latency,
					InputTokens:               compRes.InputTokens,
					OutputTokens:              compRes.OutputTokens,
					FinishReason:              string(compRes.FinishReason),
					RawContent:                compRes.Text,
					FormatOK:                  formatOK,
					SemanticOK:                semanticOK,
				}
				results = append(results, res)

				status := "OK"
				if !semanticOK {
					status = "FAIL"
				}
				fmt.Printf("  %s / %s / r%d → %s (%dms, out=%d, finish=%s, effort=%q, suppressed=%v)\n",
					target.Name, scenario.Name, rep, status, latency, compRes.OutputTokens, compRes.FinishReason, req.ReasoningEffort, compiled.ReasoningEffortSuppressed)

				time.Sleep(300 * time.Millisecond)
			}
		}
	}

	outDir := filepath.Join("results", "phase393-adaptive-budget")
	_ = os.MkdirAll(outDir, 0755)
	data, _ := json.MarshalIndent(results, "", "  ")
	_ = os.WriteFile(filepath.Join(outDir, "results.json"), data, 0644)
	fmt.Printf("\nSaved %d trial records to %s/results.json\n", len(results), outDir)
}
