// Phase 392 — Full-chain integration fire test: BudgetGuard + reasoning effort
// auto-suppression + thinking tag stripping + format-tolerant parsing across
// multiple Groq models.
//
// Hypothesis: The chain of improvements from Phases 386-391 (BudgetGuard floor,
// ThinkingOverheadTokens, reasoning effort auto-suppression, StripThinkingTags,
// NormalizeStructuredResponse, ParseResponse with fallback) works end-to-end
// across diverse Groq models, recovering correct structured answers even under
// tight budgets and reasoning model output.
//
// Models tested (rotation from recent phases):
//   - qwen/qwen3.6-27b (reasoning model, needs auto-suppression + tag stripping)
//   - llama-3.3-70b-versatile (baseline control, should pass cleanly)
//   - llama-3.1-8b-instant (small model, tests format-fallback parsing)
//   - openai/gpt-oss-20b (reasoning-capable, tests overhead handling)
//
// Each model is tested under 3 budget conditions:
//   - tight (64 tokens, should trigger auto-suppression for reasoning models)
//   - moderate (256 tokens, should succeed without suppression)
//   - comfortable (512 tokens, control case)
//
// 4 models × 3 budgets × 3 reps = 36 live calls.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/modeltext"
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/provider/openai"
)

type trialResult struct {
	CaseName           string            `json:"case_name"`
	Model              string            `json:"model"`
	MaxTokens          int               `json:"max_tokens"`
	ThinkingOverhead   int               `json:"thinking_overhead"`
	SuppressionApplied bool              `json:"suppression_applied"`
	WireEffort         string            `json:"wire_effort"`
	LatencyMs          int64             `json:"latency_ms"`
	InputTokens        int               `json:"input_tokens"`
	OutputTokens       int               `json:"output_tokens"`
	FinishReason       string            `json:"finish_reason"`
	ThoughtStripped    bool              `json:"thought_stripped"`
	ParsedValues       map[string]string `json:"parsed_values"`
	ParseUsedFallback  bool              `json:"parse_used_fallback"`
	FormatCorrect      bool              `json:"format_correct"`
	SemanticCorrect    bool              `json:"semantic_correct"`
	Error              string            `json:"error,omitempty"`
}

type summary struct {
	TotalTrials int                      `json:"total_trials"`
	TotalErrors int                      `json:"total_errors"`
	Total429    int                      `json:"total_429"`
	TotalOK     int                      `json:"total_ok"`
	ByModel     map[string]*modelSummary `json:"by_model"`
	LatencyP50  int64                    `json:"latency_p50_ms"`
	LatencyP95  int64                    `json:"latency_p95_ms"`
	LatencyMax  int64                    `json:"latency_max_ms"`
}

type modelSummary struct {
	Trials         int `json:"trials"`
	FormatOK       int `json:"format_ok"`
	SemanticOK     int `json:"semantic_ok"`
	Errors         int `json:"errors"`
	AutoSuppressed int `json:"auto_suppressed"`
}

func main() {
	groqKey := os.Getenv("GROQ_API_KEY")
	if groqKey == "" {
		fmt.Println("GROQ_API_KEY is required")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	outDir := "results/phase392-integration-fire"
	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Printf("mkdir error: %v\n", err)
		os.Exit(1)
	}

	models := []struct {
		id               string
		thinkingOverhead int
	}{
		{"qwen/qwen3.6-27b", 640},
		{"llama-3.3-70b-versatile", 0},
		{"llama-3.1-8b-instant", 0},
		{"openai/gpt-oss-20b", 128},
	}

	budgets := []struct {
		name      string
		maxTokens int
	}{
		{"tight-64", 64},
		{"moderate-256", 256},
		{"comfortable-512", 512},
	}

	reps := 3
	expectedKeys := []string{"DATE", "SOURCE"}
	// Expected answer for the test case
	expectedDate := "2025-11-03"
	expectedSource := "S-17"

	estimator := prompt.ConservativeEstimator{}
	compiler := prompt.Compiler{
		Estimator:             estimator,
		ProviderContextTokens: 8192,
	}

	// Build the operation spec input once
	input := prompt.Input{
		Task: "EXTRACT",
		Facts: []prompt.Fact{
			{ID: "F1", Text: "On 2025-11-03, sensor S-17 reported an anomalous temperature reading of 42.7°C in sector D.", Required: true, Priority: 1},
			{ID: "F2", Text: "The maintenance log shows that sector D was inspected on 2025-11-01 and no issues were found.", Required: false, Priority: 2},
		},
		AllowedOutputs: []string{"DATE: YYYY-MM-DD", "SOURCE: S-XX"},
		AnswerFormat:   "DATE: YYYY-MM-DD\nSOURCE: S-XX",
		FormatExample:  "DATE: 2025-01-15\nSOURCE: S-42",
	}

	var results []trialResult
	var latencies []int64

	fmt.Println("=== Phase 392 Full-Chain Integration Fire Test ===")
	fmt.Printf("Models: %d, Budgets: %d, Reps: %d, Total calls: %d\n",
		len(models), len(budgets), reps, len(models)*len(budgets)*reps)

	for _, m := range models {
		p, err := openai.New(openai.Config{
			BaseURL: "https://api.groq.com/openai/v1",
			APIKey:  groqKey,
			Model:   m.id,
		})
		if err != nil {
			fmt.Printf("Init error for %s: %v\n", m.id, err)
			continue
		}

		for _, b := range budgets {
			for r := 0; r < reps; r++ {
				caseName := fmt.Sprintf("%s/%s/rep%d", m.id, b.name, r)

				// Compile with budget
				compileInput := input
				compileInput.ThinkingOverheadTokens = m.thinkingOverhead
				spec := domain.OperationSpec{
					SchemaVersion:    1,
					ID:               "integration-fire@1",
					ContractVersion:  1,
					TemplateVersion:  1,
					InputSchema:      "facts",
					OutputSchema:     "structured",
					Budget:           domain.Budget{ModelCalls: 1, Tokens: 8192, Attempts: 1},
					MaxOutputTokens:  b.maxTokens,
					SafetyMargin:     10,
					Validators:       []string{"valid-format"},
					RetryPolicy:      "none",
					FallbackPolicy:   "fail",
					MaximumAuthority: domain.AuthorityProposeOnly,
				}
				compiled, err := compiler.Compile(spec, compileInput)

				result := trialResult{
					CaseName:         caseName,
					Model:            m.id,
					MaxTokens:        b.maxTokens,
					ThinkingOverhead: m.thinkingOverhead,
				}

				if err != nil {
					result.Error = fmt.Sprintf("compile error: %v", err)
					fmt.Printf("  %s → COMPILE ERROR: %v\n", caseName, err)
					results = append(results, result)
					continue
				}

				result.SuppressionApplied = compiled.ReasoningEffortSuppressed
				result.WireEffort = compiled.Request.ReasoningEffort

				// Execute
				start := time.Now()
				completion, err := p.Complete(ctx, compiled.Request)
				latency := time.Since(start).Milliseconds()
				latencies = append(latencies, latency)
				result.LatencyMs = latency

				if err != nil {
					result.Error = err.Error()
					fmt.Printf("  %s → ERROR: %v (%dms)\n", caseName, err, latency)
					results = append(results, result)
					continue
				}

				result.InputTokens = completion.InputTokens
				result.OutputTokens = completion.OutputTokens
				result.FinishReason = string(completion.FinishReason)

				// Normalize: strip thinking tags
				norm := modeltext.NormalizeStructuredResponse(completion.Text)
				result.ThoughtStripped = norm.Changed

				// Parse structured response
				parsed := prompt.ParseResponse(norm.Text, expectedKeys)
				result.ParsedValues = parsed.Values
				result.ParseUsedFallback = parsed.UsedFallback

				// Check format correctness: both keys found by prefix (not fallback)
				result.FormatCorrect = len(parsed.FoundKeys) == 2 && !parsed.UsedFallback

				// Check semantic correctness
				result.SemanticCorrect = parsed.Values["DATE"] == expectedDate &&
					parsed.Values["SOURCE"] == expectedSource

				status := "FAIL"
				if result.SemanticCorrect {
					status = "OK"
				}
				fmt.Printf("  %s → %s (%dms, %d out tokens, finish=%s, suppressed=%v, stripped=%v, fallback=%v)\n",
					caseName, status, latency, completion.OutputTokens,
					completion.FinishReason, result.SuppressionApplied,
					result.ThoughtStripped, result.ParseUsedFallback)

				// Small delay to avoid hammering
				time.Sleep(250 * time.Millisecond)

				results = append(results, result)
			}
		}
	}

	// Compute summary
	s := computeSummary(results, latencies)

	// Write results
	resultsJSON, _ := json.MarshalIndent(results, "", "  ")
	summaryJSON, _ := json.MarshalIndent(s, "", "  ")
	os.WriteFile(filepath.Join(outDir, "results.json"), resultsJSON, 0644)
	os.WriteFile(filepath.Join(outDir, "summary.json"), summaryJSON, 0644)

	// Print summary
	fmt.Println("\n=== SUMMARY ===")
	fmt.Printf("Total trials: %d, OK: %d, Errors: %d, 429s: %d\n",
		s.TotalTrials, s.TotalOK, s.TotalErrors, s.Total429)
	fmt.Printf("Latency P50: %dms, P95: %dms, Max: %dms\n",
		s.LatencyP50, s.LatencyP95, s.LatencyMax)
	for model, ms := range s.ByModel {
		fmt.Printf("  %s: trials=%d format_ok=%d semantic_ok=%d errors=%d auto_suppressed=%d\n",
			model, ms.Trials, ms.FormatOK, ms.SemanticOK, ms.Errors, ms.AutoSuppressed)
	}
	fmt.Printf("\nArtifacts: %s/\n", outDir)
}

func computeSummary(results []trialResult, latencies []int64) *summary {
	s := &summary{
		TotalTrials: len(results),
		ByModel:     make(map[string]*modelSummary),
	}

	for _, r := range results {
		if r.Error != "" {
			s.TotalErrors++
			continue
		}
		s.TotalOK++
		if r.FinishReason == "429" || r.Error != "" && contains429(r.Error) {
			s.Total429++
		}

		ms, ok := s.ByModel[r.Model]
		if !ok {
			ms = &modelSummary{}
			s.ByModel[r.Model] = ms
		}
		ms.Trials++
		if r.FormatCorrect {
			ms.FormatOK++
		}
		if r.SemanticCorrect {
			ms.SemanticOK++
		}
		if r.SuppressionApplied {
			ms.AutoSuppressed++
		}
	}

	// Sort latencies for percentiles
	sortedLat := make([]int64, len(latencies))
	copy(sortedLat, latencies)
	sortInt64s(sortedLat)
	if len(sortedLat) > 0 {
		s.LatencyP50 = sortedLat[len(sortedLat)/2]
		s.LatencyP95 = sortedLat[(len(sortedLat)*95)/100]
		s.LatencyMax = sortedLat[len(sortedLat)-1]
	}

	return s
}

func contains429(s string) bool {
	return strings.Contains(s, "429")
}

func sortInt64s(a []int64) {
	for i := 1; i < len(a); i++ {
		key := a[i]
		j := i - 1
		for j >= 0 && a[j] > key {
			a[j+1] = a[j]
			j--
		}
		a[j+1] = key
	}
}
