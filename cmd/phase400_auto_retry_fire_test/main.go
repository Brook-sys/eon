package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/gatecampaign"
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/provider/openai"
)

type Trial struct {
	Model               string               `json:"model"`
	Provider            string               `json:"provider"`
	TaskCase            string               `json:"task_case"`
	RequestedEffort     string               `json:"requested_effort"`
	ThinkingOverhead    int                  `json:"thinking_overhead"`
	ReasoningSuppressed bool                 `json:"reasoning_suppressed"`
	RetriedWithScale    bool                 `json:"retried_with_scale"`
	FinalMaxTokens      int                  `json:"final_max_tokens"`
	HTTPStatus          int                  `json:"http_status"`
	FinishReason        string               `json:"finish_reason"`
	OutputTokens        int                  `json:"output_tokens"`
	LatencyMS           int64                `json:"latency_ms"`
	ParseStrategy       prompt.ParseStrategy `json:"parse_strategy"`
	ComplianceScore     float64              `json:"compliance_score"`
	NonEmptyLineCount   int                  `json:"non_empty_line_count"`
	UsedFallback        bool                 `json:"used_fallback"`
	ParsedValues        map[string]string    `json:"parsed_values"`
	FormatCorrect       bool                 `json:"format_correct"`
	SemanticCorrect     bool                 `json:"semantic_correct"`
	RawContent          string               `json:"raw_content"`
	Error               string               `json:"error,omitempty"`
}

type CampaignSummary struct {
	TotalTrials    int            `json:"total_trials"`
	SuccessCount   int            `json:"success_count"`
	SuccessRate    float64        `json:"success_rate"`
	P50LatencyMS   int64          `json:"p50_latency_ms"`
	P95LatencyMS   int64          `json:"p95_latency_ms"`
	StrategyCounts map[string]int `json:"strategy_counts"`
	ComplianceAvg  float64        `json:"compliance_avg"`
}

func main() {
	groqKey := os.Getenv("GROQ_API_KEY")
	nimKey := os.Getenv("NVIDIA_NIM_API_KEY")

	if groqKey == "" && nimKey == "" {
		log.Fatal("Neither GROQ_API_KEY nor NVIDIA_NIM_API_KEY is set.")
	}

	outDir := filepath.Join("results", "phase400-auto-retry-parser")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		log.Fatalf("failed to create output dir: %v", err)
	}

	models := []struct {
		provider string
		model    string
		endpoint string
		key      string
	}{
		{"groq", "llama-3.1-8b-instant", "https://api.groq.com/openai/v1", groqKey},
		{"groq", "llama-3.3-70b-versatile", "https://api.groq.com/openai/v1", groqKey},
		{"groq", "qwen/qwen3.6-27b", "https://api.groq.com/openai/v1", groqKey},
		{"groq", "openai/gpt-oss-20b", "https://api.groq.com/openai/v1", groqKey},
	}

	if nimKey != "" {
		models = append(models, struct {
			provider string
			model    string
			endpoint string
			key      string
		}{"nim", "meta/llama-3.1-8b-instruct", "https://integrate.api.nvidia.com/v1", nimKey})
	}

	cases := []struct {
		name         string
		instruction  string
		expectedKeys []string
		expectedVals map[string]string
		effort       string
		maxTokens    int
	}{
		{
			name:         "bold_markdown_key_parsing",
			instruction:  "Extract deployment facts. Return formatted lines:\n**VERSION**: v3.1\n**STATUS**: DEPLOYED\n**ENVIRONMENT**: STAGING",
			expectedKeys: []string{"VERSION", "STATUS", "ENVIRONMENT"},
			expectedVals: map[string]string{"VERSION": "v3.1", "STATUS": "DEPLOYED", "ENVIRONMENT": "STAGING"},
			effort:       "",
			maxTokens:    256,
		},
		{
			name:         "bulleted_bold_key_parsing",
			instruction:  "Extract infrastructure metrics. Return lines:\n- **NODES**: 12\n* **MEMORY**: 64GB\n1. **CPU**: 98%",
			expectedKeys: []string{"NODES", "MEMORY", "CPU"},
			expectedVals: map[string]string{"NODES": "12", "MEMORY": "64GB", "CPU": "98%"},
			effort:       "",
			maxTokens:    256,
		},
		{
			name:         "thinking_overhead_auto_suppress",
			instruction:  "Assess security compliance. Line 1: 'STATUS: PASS', Line 2: 'SCORE: 98', Line 3: 'SEVERITY: LOW'.",
			expectedKeys: []string{"STATUS", "SCORE", "SEVERITY"},
			expectedVals: map[string]string{"STATUS": "PASS", "SCORE": "98", "SEVERITY": "LOW"},
			effort:       "",
			maxTokens:    512,
		},
	}

	var trials []Trial
	var latencies []int64
	strategyCounts := make(map[string]int)
	complianceSum := 0.0

	for _, m := range models {
		if m.key == "" {
			log.Printf("Skipping %s/%s because API key is missing", m.provider, m.model)
			continue
		}

		prov, err := openai.New(openai.Config{
			BaseURL: m.endpoint,
			APIKey:  m.key,
			Model:   m.model,
		})
		if err != nil {
			log.Printf("Failed to create provider %s/%s: %v", m.provider, m.model, err)
			continue
		}

		thinkingOverhead := domain.ResolveThinkingOverheadTokens(m.model)

		for _, c := range cases {
			currentMaxTokens := c.maxTokens
			retried := false

			spec := domain.OperationSpec{
				SchemaVersion:    1,
				ID:               "structured_extract@1",
				ContractVersion:  1,
				TemplateVersion:  1,
				InputSchema:      "facts",
				OutputSchema:     "structured",
				Budget:           domain.Budget{ModelCalls: 1, Tokens: 2000, Attempts: 2},
				MaxOutputTokens:  currentMaxTokens,
				SafetyMargin:     50,
				Validators:       []string{"schema"},
				RetryPolicy:      "none",
				FallbackPolicy:   "fail",
				MaximumAuthority: domain.AuthorityProposeOnly,
			}

			compiler := prompt.Compiler{
				Estimator:             prompt.ConservativeEstimator{},
				ProviderContextTokens: 4096,
			}

			input := prompt.Input{
				Task:                   c.instruction,
				AllowedOutputs:          c.expectedKeys,
				AnswerFormat:            strings.Join(c.expectedKeys, "\n"),
				FormatExample:           fmt.Sprintf("%s: VALUE", c.expectedKeys[0]),
				FormatAnchoring:        prompt.FormatAnchoringStrict,
				ThinkingOverheadTokens: thinkingOverhead,
			}

			compResult, err := compiler.Compile(spec, input)
			if err != nil {
				log.Printf("[%s/%s] Compile error on %s: %v", m.provider, m.model, c.name, err)
				continue
			}
			if c.effort != "" {
				compResult.Request.ReasoningEffort = c.effort
			}

			start := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			resp, err := prov.Complete(ctx, compResult.Request)
			elapsed := time.Since(start).Milliseconds()
			cancel()

			// Check if retry is needed for reasoning budget exhaustion
			if err != nil {
				if shouldRetry, newBudget := gatecampaign.ShouldRetryWithHigherBudget(err, currentMaxTokens, 2048); shouldRetry {
					log.Printf("[%s/%s] Task %s exhausted budget (%d tokens). Auto-scaling to %d tokens and retrying...",
						m.provider, m.model, c.name, currentMaxTokens, newBudget)
					retried = true
					currentMaxTokens = newBudget
					spec.MaxOutputTokens = newBudget

					compResultRetry, compileErr := compiler.Compile(spec, input)
					if compileErr == nil {
						if c.effort != "" {
							compResultRetry.Request.ReasoningEffort = c.effort
						}
						retryStart := time.Now()
						retryCtx, retryCancel := context.WithTimeout(context.Background(), 45*time.Second)
						resp, err = prov.Complete(retryCtx, compResultRetry.Request)
						elapsed += time.Since(retryStart).Milliseconds()
						retryCancel()
					}
				}
			}

			trial := Trial{
				Model:               m.model,
				Provider:            m.provider,
				TaskCase:            c.name,
				RequestedEffort:     c.effort,
				ThinkingOverhead:    thinkingOverhead,
				ReasoningSuppressed: compResult.ReasoningEffortSuppressed,
				RetriedWithScale:    retried,
				FinalMaxTokens:      currentMaxTokens,
				LatencyMS:           elapsed,
			}

			if err != nil {
				trial.Error = err.Error()
				if strings.Contains(err.Error(), "HTTP 400") || strings.Contains(err.Error(), "400") {
					trial.HTTPStatus = 400
				} else if strings.Contains(err.Error(), "HTTP 429") || strings.Contains(err.Error(), "429") {
					trial.HTTPStatus = 429
				} else if strings.Contains(err.Error(), "HTTP 500") || strings.Contains(err.Error(), "500") {
					trial.HTTPStatus = 500
				} else {
					trial.HTTPStatus = 502
				}
				log.Printf("[%s/%s] Task %s FAILED in %dms (retried=%v): %v", m.provider, m.model, c.name, elapsed, retried, err)
			} else {
				trial.HTTPStatus = 200
				trial.FinishReason = string(resp.FinishReason)
				trial.OutputTokens = resp.OutputTokens
				trial.RawContent = resp.Text

				parseRes := prompt.ParseResponse(resp.Text, c.expectedKeys)
				trial.ParseStrategy = parseRes.Strategy
				trial.ComplianceScore = parseRes.FormatComplianceScore
				trial.NonEmptyLineCount = parseRes.NonEmptyLineCount
				trial.UsedFallback = parseRes.UsedFallback
				trial.ParsedValues = parseRes.Values
				trial.FormatCorrect = parseRes.FormatComplianceScore == 1.0
				trial.SemanticCorrect = parseRes.AllMatch(c.expectedKeys, c.expectedVals)

				strategyCounts[string(parseRes.Strategy)]++
				complianceSum += parseRes.FormatComplianceScore
				latencies = append(latencies, elapsed)

				log.Printf("[%s/%s] Task %s OK in %dms (retried=%v max_tokens=%d) | strategy=%s compliance=%.2f semantic=%v",
					m.provider, m.model, c.name, elapsed, retried, currentMaxTokens, parseRes.Strategy, parseRes.FormatComplianceScore, trial.SemanticCorrect)
			}

			trials = append(trials, trial)
			time.Sleep(1 * time.Second)
		}
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50 := int64(0)
	p95 := int64(0)
	if len(latencies) > 0 {
		p50 = latencies[len(latencies)*50/100]
		p95 = latencies[len(latencies)*95/100]
	}

	successes := 0
	for _, t := range trials {
		if t.HTTPStatus == 200 && t.SemanticCorrect {
			successes++
		}
	}

	successRate := 0.0
	complianceAvg := 0.0
	if len(trials) > 0 {
		successRate = float64(successes) / float64(len(trials))
		complianceAvg = complianceSum / float64(len(trials))
	}

	summary := CampaignSummary{
		TotalTrials:    len(trials),
		SuccessCount:   successes,
		SuccessRate:    successRate,
		P50LatencyMS:   p50,
		P95LatencyMS:   p95,
		StrategyCounts: strategyCounts,
		ComplianceAvg:  complianceAvg,
	}

	trialsJSON, _ := json.MarshalIndent(trials, "", "  ")
	_ = os.WriteFile(filepath.Join(outDir, "trials.json"), trialsJSON, 0644)

	summaryJSON, _ := json.MarshalIndent(summary, "", "  ")
	_ = os.WriteFile(filepath.Join(outDir, "summary.json"), summaryJSON, 0644)

	reportMarkdown := fmt.Sprintf(`# Phase 400 — Auto-Retry & Budget Escalation Fire Campaign Report

**Date:** %s
**Total Trials:** %d
**Success Count:** %d (%.1f%%)
**P50 Latency:** %d ms
**P95 Latency:** %d ms
**Average Compliance Score:** %.2f

## Strategy Distribution
`, time.Now().UTC().Format(time.RFC3339), summary.TotalTrials, summary.SuccessCount, summary.SuccessRate*100, summary.P50LatencyMS, summary.P95LatencyMS, summary.ComplianceAvg)

	for k, v := range strategyCounts {
		reportMarkdown += fmt.Sprintf("- **%s**: %d\n", k, v)
	}

	reportMarkdown += "\n## Trials Breakdown\n\n| Provider | Model | Case | MaxTokens | Retried | Strategy | Compliance | Semantic | Latency |\n|---|---|---|---|---|---|---|---|---|\n"
	for _, t := range trials {
		reportMarkdown += fmt.Sprintf("| %s | %s | %s | %d | %v | %s | %.2f | %v | %dms |\n",
			t.Provider, t.Model, t.TaskCase, t.FinalMaxTokens, t.RetriedWithScale, t.ParseStrategy, t.ComplianceScore, t.SemanticCorrect, t.LatencyMS)
	}

	_ = os.WriteFile(filepath.Join(outDir, "REPORT.md"), []byte(reportMarkdown), 0644)
	fmt.Printf("\n=== Phase 400 Campaign Complete ===\nReport saved to %s/REPORT.md\n", outDir)
}
