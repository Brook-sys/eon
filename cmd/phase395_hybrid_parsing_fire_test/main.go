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
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/provider/openai"
)

type Trial struct {
	Model             string               `json:"model"`
	Provider          string               `json:"provider"`
	TaskCase          string               `json:"task_case"`
	MaxTokens         int                  `json:"max_tokens"`
	HTTPStatus        int                  `json:"http_status"`
	FinishReason      string               `json:"finish_reason"`
	OutputTokens      int                  `json:"output_tokens"`
	LatencyMS         int64                `json:"latency_ms"`
	ParseStrategy     prompt.ParseStrategy `json:"parse_strategy"`
	ComplianceScore   float64              `json:"compliance_score"`
	NonEmptyLineCount int                  `json:"non_empty_line_count"`
	UsedFallback      bool                 `json:"used_fallback"`
	ParsedValues      map[string]string    `json:"parsed_values"`
	FormatCorrect     bool                 `json:"format_correct"`
	SemanticCorrect   bool                 `json:"semantic_correct"`
	RawContent        string               `json:"raw_content"`
	Error             string               `json:"error,omitempty"`
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

	outDir := filepath.Join("results", "phase395-hybrid-parsing")
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
		}{"nim", "deepseek-ai/deepseek-v4-flash-0731", "https://integrate.api.nvidia.com/v1", nimKey})
	}

	// Test Cases
	cases := []struct {
		name         string
		instruction  string
		expectedKeys []string
		expectedVals map[string]string
	}{
		{
			name:         "standard_prefix",
			instruction:  "Output DATE on line 1 as 'DATE: YYYY-MM-DD' and SOURCE on line 2 as 'SOURCE: ID'.",
			expectedKeys: []string{"DATE", "SOURCE"},
			expectedVals: map[string]string{"DATE": "2025-11-03", "SOURCE": "S-17"},
		},
		{
			name:         "hybrid_prefix_bare",
			instruction:  "Output DATE on line 1 as 'DATE: 2025-11-03' and SOURCE on line 2 as bare 'S-17' without prefix.",
			expectedKeys: []string{"DATE", "SOURCE"},
			expectedVals: map[string]string{"DATE": "2025-11-03", "SOURCE": "S-17"},
		},
	}

	spec := domain.OperationSpec{
		SchemaVersion:    1,
		ID:               "extract_date_source@1",
		ContractVersion:  1,
		TemplateVersion:  1,
		InputSchema:      "facts",
		OutputSchema:     "structured",
		Budget:           domain.Budget{ModelCalls: 1, Tokens: 1000, Attempts: 1},
		MaxOutputTokens:  256,
		SafetyMargin:     10,
		Validators:       []string{"exact_keys"},
		RetryPolicy:      "none",
		FallbackPolicy:   "fail",
		MaximumAuthority: domain.AuthorityProposeOnly,
	}

	var trials []Trial
	strategyCounts := make(map[string]int)

	log.Printf("Starting Phase 395 Hybrid Parsing Live Campaign across %d models...", len(models))

	for _, m := range models {
		if m.key == "" {
			log.Printf("Skipping %s/%s (no API key)", m.provider, m.model)
			continue
		}

		prov, err := openai.New(openai.Config{
			BaseURL:        m.endpoint,
			APIKey:         m.key,
			Model:          m.model,
			MaxOutputField: openai.MaxOutputTokensLegacy,
			Timeout:        45 * time.Second,
		})
		if err != nil {
			log.Printf("Failed to create provider for %s/%s: %v", m.provider, m.model, err)
			continue
		}

		thinkingOverhead := domain.ResolveThinkingOverheadTokens(m.model)

		for _, tc := range cases {
			taskInput := prompt.Input{
				Task:           fmt.Sprintf("Document S-17 was released on 2025-11-03. %s", tc.instruction),
				AllowedOutputs: tc.expectedKeys,
				AnswerFormat:   "DATE: YYYY-MM-DD\nSOURCE: ID",
				Facts: []prompt.Fact{
					{ID: "f1", Text: "Document S-17 date is 2025-11-03.", Required: true},
				},
				ThinkingOverheadTokens: thinkingOverhead,
			}

			compiler := prompt.Compiler{
				Estimator:             prompt.ConservativeEstimator{},
				ProviderContextTokens: 4096,
			}

			compiled, err := compiler.Compile(spec, taskInput)
			if err != nil {
				log.Printf("Failed compile %s/%s/%s: %v", m.provider, m.model, tc.name, err)
				continue
			}

			req := compiled.Request
			req.MaxOutputTokens = 256
			if thinkingOverhead > 0 && req.ReasoningEffort == "" {
				req.ReasoningEffort = "none"
			}

			start := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			resp, err := prov.Complete(ctx, req)
			latency := time.Since(start).Milliseconds()
			cancel()

			trial := Trial{
				Model:        m.model,
				Provider:     m.provider,
				TaskCase:     tc.name,
				MaxTokens:    256,
				LatencyMS:    latency,
				ParsedValues: make(map[string]string),
			}

			if err != nil {
				trial.Error = err.Error()
				trial.HTTPStatus = 500
				log.Printf("FAIL [%s/%s/%s]: %v", m.provider, m.model, tc.name, err)
			} else {
				trial.HTTPStatus = 200
				trial.FinishReason = string(resp.FinishReason)
				trial.OutputTokens = resp.OutputTokens
				trial.RawContent = resp.Text

				parseRes := prompt.ParseResponse(resp.Text, tc.expectedKeys)
				trial.ParseStrategy = parseRes.Strategy
				trial.ComplianceScore = parseRes.FormatComplianceScore
				trial.NonEmptyLineCount = parseRes.NonEmptyLineCount
				trial.UsedFallback = parseRes.UsedFallback
				trial.ParsedValues = parseRes.Values

				strategyCounts[string(parseRes.Strategy)]++

				// Correctness checks
				trial.FormatCorrect = parseRes.FormatComplianceScore == 1.0
				semanticOK := true
				for k, wantVal := range tc.expectedVals {
					gotVal, ok := parseRes.Values[k]
					if !ok || !strings.Contains(strings.ToLower(gotVal), strings.ToLower(wantVal)) {
						semanticOK = false
						break
					}
				}
				trial.SemanticCorrect = semanticOK

				log.Printf("OK [%s/%s/%s]: strategy=%s compliance=%.2f semantic=%v latency=%dms",
					m.provider, m.model, tc.name, parseRes.Strategy, parseRes.FormatComplianceScore, trial.SemanticCorrect, latency)
			}

			trials = append(trials, trial)
			time.Sleep(200 * time.Millisecond) // avoid rapid bursts
		}
	}

	// Compute summary
	successCount := 0
	var latencies []int64
	var complianceSum float64

	for _, t := range trials {
		if t.FormatCorrect && t.SemanticCorrect {
			successCount++
		}
		if t.LatencyMS > 0 {
			latencies = append(latencies, t.LatencyMS)
		}
		complianceSum += t.ComplianceScore
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	var p50, p95 int64
	if len(latencies) > 0 {
		p50 = latencies[len(latencies)*50/100]
		p95 = latencies[len(latencies)*95/100]
	}

	var complianceAvg float64
	if len(trials) > 0 {
		complianceAvg = complianceSum / float64(len(trials))
	}

	summary := CampaignSummary{
		TotalTrials:    len(trials),
		SuccessCount:   successCount,
		SuccessRate:    float64(successCount) / float64(len(trials)),
		P50LatencyMS:   p50,
		P95LatencyMS:   p95,
		StrategyCounts: strategyCounts,
		ComplianceAvg:  complianceAvg,
	}

	// Save results
	trialsJSON, _ := json.MarshalIndent(trials, "", "  ")
	_ = os.WriteFile(filepath.Join(outDir, "trials.json"), trialsJSON, 0644)

	summaryJSON, _ := json.MarshalIndent(summary, "", "  ")
	_ = os.WriteFile(filepath.Join(outDir, "summary.json"), summaryJSON, 0644)

	report := fmt.Sprintf("# Phase 395 — Hybrid Parsing Live Campaign Report\n\n"+
		"**Date:** %s\n"+
		"**Total Trials:** %d\n"+
		"**Success Count:** %d (%.1f%%)\n"+
		"**P50 Latency:** %d ms\n"+
		"**P95 Latency:** %d ms\n"+
		"**Average Compliance Score:** %.2f\n\n"+
		"## Strategy Distribution\n",
		time.Now().Format("2006-01-02 15:04:05 -0700"),
		summary.TotalTrials,
		summary.SuccessCount,
		summary.SuccessRate*100,
		summary.P50LatencyMS,
		summary.P95LatencyMS,
		summary.ComplianceAvg,
	)

	for strat, count := range strategyCounts {
		report += fmt.Sprintf("- **%s**: %d\n", strat, count)
	}

	report += "\n## Trials Breakdown\n\n| Provider | Model | Case | Strategy | Compliance | Semantic | Latency |\n|---|---|---|---|---|---|---|\n"
	for _, t := range trials {
		report += fmt.Sprintf("| %s | %s | %s | %s | %.2f | %v | %dms |\n",
			t.Provider, t.Model, t.TaskCase, t.ParseStrategy, t.ComplianceScore, t.SemanticCorrect, t.LatencyMS)
	}

	_ = os.WriteFile(filepath.Join(outDir, "REPORT.md"), []byte(report), 0644)

	fmt.Println("\n=== Phase 395 Campaign Complete ===")
	fmt.Printf("Success: %d/%d (%.1f%%)\n", summary.SuccessCount, summary.TotalTrials, summary.SuccessRate*100)
	fmt.Printf("P50: %dms, P95: %dms\n", p50, p95)
	fmt.Printf("Report saved to %s\n", filepath.Join(outDir, "REPORT.md"))
}
