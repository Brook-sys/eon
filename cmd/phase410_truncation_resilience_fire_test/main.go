//go:build ignore

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

// Phase 410: Truncation resilience & tight output budget live fire campaign.
// Tests model completion under restricted max_tokens budget (16-32 tokens),
// causing partial completions or truncated key names, to validate the parser's
// truncated key prefix recovery and general resilience under budget starvation.

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
	FoundByTruncated    []string             `json:"found_by_truncated"`
	ParsedValues        map[string]string    `json:"parsed_values"`
	FormatCorrect       bool                 `json:"format_correct"`
	SemanticCorrect     bool                 `json:"semantic_correct"`
	RawContent          string               `json:"raw_content"`
}

type ModelSummary struct {
	TotalTrials       int     `json:"total_trials"`
	SuccessfulTrials  int     `json:"successful_trials"`
	SuccessRate       float64 `json:"success_rate"`
	P50LatencyMS      int64   `json:"p50_latency_ms"`
	P95LatencyMS      int64   `json:"p95_latency_ms"`
	AvgCompliance     float64 `json:"avg_compliance"`
	FormatCorrectRate float64 `json:"format_correct_rate"`
}

type CampaignSummary struct {
	Timestamp     string                  `json:"timestamp"`
	TotalTrials   int                     `json:"total_trials"`
	Successful    int                     `json:"successful"`
	SuccessRate   float64                 `json:"success_rate"`
	P50LatencyMS  int64                   `json:"p50_latency_ms"`
	P95LatencyMS  int64                   `json:"p95_latency_ms"`
	AvgCompliance float64                 `json:"avg_compliance"`
	ModelStats    map[string]ModelSummary `json:"model_stats"`
}

type TestCase struct {
	name         string
	expectedKeys []string
	instruction  string
	maxTokens    int
	effort       string
	checkSem     func(values map[string]string) bool
}

func main() {
	log.Println("=== Phase 410: Truncation Resilience & Budget Starvation Live Fire Campaign ===")

	groqKey := os.Getenv("GROQ_API_KEY")
	nimKey := os.Getenv("NVIDIA_NIM_API_KEY")
	if groqKey == "" && nimKey == "" {
		log.Fatal("ERROR: GROQ_API_KEY or NVIDIA_NIM_API_KEY missing from environment")
	}

	outDir := filepath.Join("results", "phase410-truncation-resilience")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		log.Fatalf("failed to create results dir: %v", err)
	}

	models := []struct {
		provider string
		model    string
		key      string
		endpoint string
	}{
		{"groq", "llama-3.1-8b-instant", groqKey, "https://api.groq.com/openai/v1"},
		{"groq", "llama-3.3-70b-versatile", groqKey, "https://api.groq.com/openai/v1"},
		{"groq", "qwen/qwen3.6-27b", groqKey, "https://api.groq.com/openai/v1"},
		{"groq", "openai/gpt-oss-120b", groqKey, "https://api.groq.com/openai/v1"},
		{"nim", "meta/llama-3.1-8b-instruct", nimKey, "https://integrate.api.nvidia.com/v1"},
	}

	testCases := []TestCase{
		{
			name:         "tight_budget_truncation",
			expectedKeys: []string{"DATE", "SOURCE", "STATUS"},
			instruction:  "Extract the fields as DATE: YYYY-MM-DD, SOURCE: text, STATUS: text. Input: Audit record from system Beta on 2026-08-09 has status PENDING.",
			maxTokens:    24,
			effort:       "",
			checkSem: func(v map[string]string) bool {
				// Require at least 2 keys extracted successfully despite tight budget
				validKeys := 0
				if strings.Contains(v["DATE"], "2026-08-09") {
					validKeys++
				}
				if strings.Contains(strings.ToLower(v["SOURCE"]), "beta") {
					validKeys++
				}
				if strings.Contains(strings.ToUpper(v["STATUS"]), "PENDING") {
					validKeys++
				}
				return validKeys >= 2
			},
		},
		{
			name:         "truncated_key_prefix",
			expectedKeys: []string{"DATE", "SOURCE", "STATUS"},
			instruction:  "Extract fields using compact KEY: VALUE format. Record: Date is 2026-08-09, Source is Audit Log Beta, Status is PENDING.",
			maxTokens:    32,
			effort:       "",
			checkSem: func(v map[string]string) bool {
				return strings.Contains(v["DATE"], "2026-08-09") &&
					(strings.Contains(strings.ToLower(v["SOURCE"]), "audit") || strings.Contains(strings.ToLower(v["SOURCE"]), "beta"))
			},
		},
		{
			name:         "multi_format_budget_pressure",
			expectedKeys: []string{"EVENT", "SEVERITY", "ACTION"},
			instruction:  "Extract incident details. Event: Database failure. Severity: CRITICAL. Action: Restarted service.",
			maxTokens:    30,
			effort:       "",
			checkSem: func(v map[string]string) bool {
				return (strings.Contains(strings.ToLower(v["EVENT"]), "database") || strings.Contains(strings.ToLower(v["EVENT"]), "fail")) &&
					strings.Contains(strings.ToUpper(v["SEVERITY"]), "CRITICAL")
			},
		},
	}

	var trials []Trial

	for _, m := range models {
		if m.key == "" {
			log.Printf("Skipping %s/%s (API key missing)", m.provider, m.model)
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

		for _, c := range testCases {
			log.Printf("Running trial: model=%s provider=%s case=%s (maxTokens=%d)", m.model, m.provider, c.name, c.maxTokens)

			currentMaxTokens := c.maxTokens
			spec := domain.OperationSpec{
				SchemaVersion:    1,
				ID:               "structured_extract@1",
				ContractVersion:  1,
				TemplateVersion:  1,
				InputSchema:      "facts",
				OutputSchema:     "structured",
				Budget:           domain.Budget{ModelCalls: 1, Tokens: 2000, Attempts: 2},
				MaxOutputTokens:  currentMaxTokens,
				SafetyMargin:     10,
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
				AllowedOutputs:         c.expectedKeys,
				AnswerFormat:           strings.Join(c.expectedKeys, "\n"),
				FormatExample:          fmt.Sprintf("%s: VALUE", c.expectedKeys[0]),
				FormatAnchoring:        prompt.FormatAnchoringStrict,
				ThinkingOverheadTokens: thinkingOverhead,
			}

			compResult, err := compiler.Compile(spec, input)
			if err != nil {
				log.Printf("Compiler error for %s: %v", m.model, err)
				continue
			}

			reqEffort := c.effort
			if (m.model == "openai/gpt-oss-20b" || m.model == "openai/gpt-oss-120b") && reqEffort == "" {
				reqEffort = "low"
			}
			if m.model == "qwen/qwen3.6-27b" && reqEffort == "" {
				compResult.Request.ReasoningEffort = "none"
			}
			if reqEffort != "" {
				compResult.Request.ReasoningEffort = reqEffort
			}

			start := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			resp, err := prov.Complete(ctx, compResult.Request)
			latencyMS := time.Since(start).Milliseconds()
			cancel()

			retriedWithScale := false

			if err != nil {
				if shouldRetry, newBudget := gatecampaign.ShouldRetryWithHigherBudget(err, currentMaxTokens, 2048); shouldRetry {
					log.Printf("Reasoning budget exhausted for %s; retrying with %d budget scale...", m.model, newBudget)
					currentMaxTokens = newBudget
					spec.MaxOutputTokens = currentMaxTokens
					compResult, err = compiler.Compile(spec, input)
					if err == nil {
						if reqEffort != "" {
							compResult.Request.ReasoningEffort = reqEffort
						}
						start = time.Now()
						ctx, cancel = context.WithTimeout(context.Background(), 45*time.Second)
						resp, err = prov.Complete(ctx, compResult.Request)
						latencyMS = time.Since(start).Milliseconds()
						cancel()
						retriedWithScale = true
					}
				}
			}

			trial := Trial{
				Model:            m.model,
				Provider:         m.provider,
				TaskCase:         c.name,
				RequestedEffort:  reqEffort,
				ThinkingOverhead: thinkingOverhead,
				RetriedWithScale: retriedWithScale,
				FinalMaxTokens:   currentMaxTokens,
				LatencyMS:        latencyMS,
			}

			if err != nil {
				log.Printf("Provider error for %s on %s: %v", m.model, c.name, err)
				trial.HTTPStatus = 500
				trial.RawContent = fmt.Sprintf("ERROR: %v", err)
				trials = append(trials, trial)
				continue
			}

			trial.HTTPStatus = 200
			trial.FinishReason = string(resp.FinishReason)
			trial.OutputTokens = resp.OutputTokens
			trial.RawContent = resp.Text

			parsed := prompt.ParseResponse(resp.Text, c.expectedKeys)
			trial.ParseStrategy = parsed.Strategy
			trial.ComplianceScore = parsed.FormatComplianceScore
			trial.NonEmptyLineCount = parsed.NonEmptyLineCount
			trial.UsedFallback = parsed.UsedFallback
			trial.FoundByTruncated = parsed.FoundByTruncated
			trial.ParsedValues = parsed.Values
			trial.FormatCorrect = (parsed.FormatComplianceScore > 0.6) // Graceful threshold under budget pressure
			trial.SemanticCorrect = c.checkSem(parsed.Values)

			log.Printf("Trial result [%s / %s]: strategy=%s compliance=%.2f semCorrect=%v finishReason=%s latency=%dms",
				m.model, c.name, parsed.Strategy, parsed.FormatComplianceScore, trial.SemanticCorrect, resp.FinishReason, latencyMS)

			trials = append(trials, trial)
		}
	}

	// Compute summaries
	summary := CampaignSummary{
		Timestamp:  time.Now().Format(time.RFC3339),
		ModelStats: make(map[string]ModelSummary),
	}

	var totalLatency []int64
	var totalCompliance []float64

	modelTrials := make(map[string][]Trial)
	for _, t := range trials {
		modelTrials[t.Model] = append(modelTrials[t.Model], t)
		if t.HTTPStatus == 200 {
			totalLatency = append(totalLatency, t.LatencyMS)
			totalCompliance = append(totalCompliance, t.ComplianceScore)
		}
	}

	summary.TotalTrials = len(trials)
	succCount := 0

	for model, mTrials := range modelTrials {
		ms := ModelSummary{
			TotalTrials: len(mTrials),
		}
		var mLat []int64
		var mComp []float64
		mSucc := 0
		mFmtSucc := 0

		for _, t := range mTrials {
			if t.HTTPStatus == 200 && t.SemanticCorrect && t.FormatCorrect {
				mSucc++
			}
			if t.HTTPStatus == 200 && t.FormatCorrect {
				mFmtSucc++
			}
			if t.HTTPStatus == 200 {
				mLat = append(mLat, t.LatencyMS)
				mComp = append(mComp, t.ComplianceScore)
			}
		}

		ms.SuccessfulTrials = mSucc
		succCount += mSucc
		if len(mTrials) > 0 {
			ms.SuccessRate = float64(mSucc) / float64(len(mTrials))
			ms.FormatCorrectRate = float64(mFmtSucc) / float64(len(mTrials))
		}

		sort.Slice(mLat, func(i, j int) bool { return mLat[i] < mLat[j] })
		if len(mLat) > 0 {
			ms.P50LatencyMS = mLat[len(mLat)/2]
			ms.P95LatencyMS = mLat[int(float64(len(mLat))*0.95)]
		}

		if len(mComp) > 0 {
			var sum float64
			for _, c := range mComp {
				sum += c
			}
			ms.AvgCompliance = sum / float64(len(mComp))
		}

		summary.ModelStats[model] = ms
	}

	summary.Successful = succCount
	if summary.TotalTrials > 0 {
		summary.SuccessRate = float64(succCount) / float64(summary.TotalTrials)
	}

	sort.Slice(totalLatency, func(i, j int) bool { return totalLatency[i] < totalLatency[j] })
	if len(totalLatency) > 0 {
		summary.P50LatencyMS = totalLatency[len(totalLatency)/2]
		summary.P95LatencyMS = totalLatency[int(float64(len(totalLatency))*0.95)]
	}

	if len(totalCompliance) > 0 {
		var sum float64
		for _, c := range totalCompliance {
			sum += c
		}
		summary.AvgCompliance = sum / float64(len(totalCompliance))
	}

	rawBytes, _ := json.MarshalIndent(trials, "", "  ")
	if err := os.WriteFile(filepath.Join(outDir, "trials.json"), rawBytes, 0644); err != nil {
		log.Printf("Failed to write trials.json: %v", err)
	}

	sumBytes, _ := json.MarshalIndent(summary, "", "  ")
	if err := os.WriteFile(filepath.Join(outDir, "summary.json"), sumBytes, 0644); err != nil {
		log.Printf("Failed to write summary.json: %v", err)
	}

	log.Printf("=== Phase 410 Campaign Complete: Total=%d Success=%d Rate=%.1f%% P50=%dms P95=%dms ===",
		summary.TotalTrials, summary.Successful, summary.SuccessRate*100, summary.P50LatencyMS, summary.P95LatencyMS)
}
