//go:build ignore

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/modeltext"
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/provider/openai"
)

type testRow struct {
	CaseName         string            `json:"case_name"`
	Model            string            `json:"model"`
	MaxTokens        int               `json:"max_tokens"`
	ThinkingOverhead int               `json:"thinking_overhead"`
	Suppressed       bool              `json:"suppressed"`
	WireEffort       string            `json:"wire_effort"`
	LatencyMs        int64             `json:"latency_ms"`
	InputTokens      int               `json:"input_tokens"`
	OutputTokens     int               `json:"output_tokens"`
	FinishReason     string            `json:"finish_reason"`
	ThoughtStripped  bool              `json:"thought_stripped"`
	ParsedValues     map[string]string `json:"parsed_values"`
	Error            string            `json:"error,omitempty"`
}

func main() {
	groqKey := os.Getenv("GROQ_API_KEY")
	if groqKey == "" {
		fmt.Println("GROQ_API_KEY is required")
		os.Exit(1)
	}

	model := "qwen/qwen3.6-27b"
	p, err := openai.New(openai.Config{
		BaseURL: "https://api.groq.com/openai/v1",
		APIKey:  groqKey,
		Model:   model,
	})
	if err != nil {
		fmt.Printf("Init error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fmt.Println("=== Phase 391 BudgetGuard Auto-Suppression Live Fire Test ===")

	estimator := prompt.ConservativeEstimator{}
	compiler := prompt.Compiler{
		Estimator:             estimator,
		ProviderContextTokens: 8192,
	}

	cases := []struct {
		name             string
		maxTokens        int
		thinkingOverhead int
	}{
		{
			name:             "auto-suppress-tight-budget",
			maxTokens:        64,
			thinkingOverhead: 384,
		},
		{
			name:             "unsuppressed-tight-budget-control",
			maxTokens:        64,
			thinkingOverhead: 0,
		},
		{
			name:             "auto-suppress-moderate-budget",
			maxTokens:        128,
			thinkingOverhead: 384,
		},
	}

	expectedKeys := []string{"STATUS", "CONFIRM"}
	var results []testRow

	for _, tc := range cases {
		spec := domain.OperationSpec{
			SchemaVersion:    1,
			ID:               "auto-suppress-test@1",
			ContractVersion:  1,
			TemplateVersion:  1,
			InputSchema:      "facts",
			OutputSchema:     "structured",
			Budget:           domain.Budget{ModelCalls: 1, Tokens: 8192, Attempts: 1},
			MaxOutputTokens:  tc.maxTokens,
			SafetyMargin:     10,
			Validators:       []string{"valid-format"},
			RetryPolicy:      "none",
			FallbackPolicy:   "fail",
			MaximumAuthority: domain.AuthorityProposeOnly,
		}

		input := prompt.Input{
			Task:                   "Verify system state for node eddb432c9129.",
			AllowedOutputs:         []string{"STATUS: OK or FAIL", "CONFIRM: YES or NO"},
			AnswerFormat:           "STATUS: OK\nCONFIRM: YES",
			ThinkingOverheadTokens: tc.thinkingOverhead,
		}

		compiled, err := compiler.Compile(spec, input)
		row := testRow{
			CaseName:         tc.name,
			Model:            model,
			MaxTokens:        tc.maxTokens,
			ThinkingOverhead: tc.thinkingOverhead,
		}

		if err != nil {
			row.Error = fmt.Sprintf("compile error: %v", err)
			fmt.Printf("[%s] Compile Error: %v\n", tc.name, err)
			results = append(results, row)
			continue
		}

		row.Suppressed = compiled.ReasoningEffortSuppressed
		row.WireEffort = compiled.Request.ReasoningEffort

		start := time.Now()
		resp, err := p.Complete(ctx, compiled.Request)
		latency := time.Since(start)
		row.LatencyMs = latency.Milliseconds()

		if err != nil {
			row.Error = err.Error()
			fmt.Printf("[%s] Provider Error: %v\n", tc.name, err)
			results = append(results, row)
			continue
		}

		row.InputTokens = resp.InputTokens
		row.OutputTokens = resp.OutputTokens
		row.FinishReason = string(resp.FinishReason)

		unthought := modeltext.StripThinkingTags(resp.Text)
		row.ThoughtStripped = unthought.Changed

		parsed := prompt.ParseResponse(resp.Text, expectedKeys)
		row.ParsedValues = parsed.Values

		fmt.Printf("\n--- [%s] ---\n", tc.name)
		fmt.Printf("Compiled: Suppressed=%v, WireEffort=%q\n", compiled.ReasoningEffortSuppressed, compiled.Request.ReasoningEffort)
		fmt.Printf("Provider: Latency=%v, OutTokens=%d, FinishReason=%s, ThoughtStripped=%v\n",
			latency, resp.OutputTokens, resp.FinishReason, unthought.Changed)
		for k, v := range row.ParsedValues {
			fmt.Printf("  %s: %q\n", k, v)
		}

		results = append(results, row)
	}

	outDir := "results/phase391-auto-suppression"
	_ = os.MkdirAll(outDir, 0755)
	jsonBytes, _ := json.MarshalIndent(results, "", "  ")
	_ = os.WriteFile(outDir+"/results.json", jsonBytes, 0644)
	fmt.Println("\n=== Campaign Complete ===")
}
