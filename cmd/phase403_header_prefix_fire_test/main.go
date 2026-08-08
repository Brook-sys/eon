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
	effort       string
	checkSem     func(values map[string]string) bool
}

func main() {
	log.Println("=== Phase 403: Header Prefix Key Parsing & Multi-Provider Live Fire Campaign ===")

	groqKey := os.Getenv("GROQ_API_KEY")
	nimKey := os.Getenv("NVIDIA_NIM_API_KEY")
	if groqKey == "" && nimKey == "" {
		log.Fatal("ERROR: GROQ_API_KEY or NVIDIA_NIM_API_KEY missing from environment")
	}

	outDir := filepath.Join("results", "phase403-header-prefix-parser")
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
		{"groq", "openai/gpt-oss-20b", groqKey, "https://api.groq.com/openai/v1"},
		{"nim", "meta/llama-3.1-8b-instruct", nimKey, "https://integrate.api.nvidia.com/v1"},
	}

	testCases := []TestCase{
		{
			name:         "markdown_header_keys",
			expectedKeys: []string{"DATE", "SOURCE", "STATUS"},
			instruction:  "Extract report details (Date is 2026-08-08, Source is SecurityAudit, Status is CONFIRMED). Format output as markdown headers (# DATE: ..., ## SOURCE: ..., ### STATUS: ...).",
			checkSem: func(v map[string]string) bool {
				return strings.Contains(v["DATE"], "2026-08-08") &&
					strings.Contains(strings.ToLower(v["SOURCE"]), "securityaudit") &&
					strings.Contains(strings.ToUpper(v["STATUS"]), "CONFIRMED")
			},
		},
		{
			name:         "header_with_bullets",
			expectedKeys: []string{"ALERT_ID", "SEVERITY"},
			instruction:  "Process alert details (Alert ID is ALT-9921, Severity is CRITICAL). Format output as bulleted markdown headers (- # ALERT_ID: ..., - ## SEVERITY: ...).",
			checkSem: func(v map[string]string) bool {
				return strings.Contains(v["ALERT_ID"], "ALT-9921") &&
					strings.Contains(strings.ToUpper(v["SEVERITY"]), "CRITICAL")
			},
		},
		{
			name:         "bold_header_mix",
			expectedKeys: []string{"BUILD", "ERRORS", "TARGET"},
			instruction:  "Build status update (Build is PASS, Errors count is 0, Target platform is Linux-x64). Format output as bold markdown headers (# **BUILD**: ..., ## **ERRORS**: ..., ### **TARGET**: ...).",
			checkSem: func(v map[string]string) bool {
				return strings.Contains(strings.ToUpper(v["BUILD"]), "PASS") &&
					strings.Contains(v["ERRORS"], "0") &&
					strings.Contains(strings.ToLower(v["TARGET"]), "linux")
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
			log.Printf("Running trial: model=%s provider=%s case=%s", m.model, m.provider, c.name)

			currentMaxTokens := 256
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
				log.Printf("Compiler error for %s: %v", m.model, err)
				continue
			}

			reqEffort := c.effort
			if m.model == "openai/gpt-oss-20b" && reqEffort == "" {
				reqEffort = "low"
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
					retriedWithScale = true
					currentMaxTokens = newBudget
					spec.MaxOutputTokens = newBudget

					compResultRetry, compileErr := compiler.Compile(spec, input)
					if compileErr == nil {
						if reqEffort != "" {
							compResultRetry.Request.ReasoningEffort = reqEffort
						}
						retryStart := time.Now()
						retryCtx, retryCancel := context.WithTimeout(context.Background(), 45*time.Second)
						resp, err = prov.Complete(retryCtx, compResultRetry.Request)
						latencyMS += time.Since(retryStart).Milliseconds()
						retryCancel()
					}
				}
			}

			t := Trial{
				Model:               m.model,
				Provider:            m.provider,
				TaskCase:            c.name,
				RequestedEffort:     reqEffort,
				ThinkingOverhead:    thinkingOverhead,
				ReasoningSuppressed: compResult.ReasoningEffortSuppressed,
				RetriedWithScale:    retriedWithScale,
				FinalMaxTokens:      currentMaxTokens,
				LatencyMS:           latencyMS,
			}

			if err != nil {
				log.Printf("Completion failed for %s/%s: %v", m.model, c.name, err)
				t.HTTPStatus = 500
				trials = append(trials, t)
				continue
			}

			t.HTTPStatus = 200
			t.FinishReason = string(resp.FinishReason)
			t.OutputTokens = resp.OutputTokens
			t.RawContent = resp.Text

			parseRes := prompt.ParseResponse(resp.Text, c.expectedKeys)
			t.ParseStrategy = parseRes.Strategy
			t.ComplianceScore = parseRes.FormatComplianceScore
			t.NonEmptyLineCount = parseRes.NonEmptyLineCount
			t.UsedFallback = parseRes.UsedFallback
			t.ParsedValues = parseRes.Values

			t.FormatCorrect = (parseRes.FormatComplianceScore == 1.0)
			t.SemanticCorrect = t.FormatCorrect && c.checkSem(parseRes.Values)

			log.Printf("Result %s/%s: compliance=%.2f strategy=%s semantic=%v latency=%dms",
				m.model, c.name, t.ComplianceScore, t.ParseStrategy, t.SemanticCorrect, t.LatencyMS)

			trials = append(trials, t)
			time.Sleep(1 * time.Second)
		}
	}

	trialsPath := filepath.Join(outDir, "trials.json")
	tBytes, _ := json.MarshalIndent(trials, "", "  ")
	os.WriteFile(trialsPath, tBytes, 0644)

	summary := buildSummary(trials)
	summaryPath := filepath.Join(outDir, "summary.json")
	sBytes, _ := json.MarshalIndent(summary, "", "  ")
	os.WriteFile(summaryPath, sBytes, 0644)

	reportPath := filepath.Join(outDir, "REPORT.md")
	writeReport(reportPath, summary, trials)

	log.Printf("Phase 403 completed: %d/%d success (%.1f%%), P50 latency: %dms, P95 latency: %dms",
		summary.Successful, summary.TotalTrials, summary.SuccessRate*100, summary.P50LatencyMS, summary.P95LatencyMS)
}

func buildSummary(trials []Trial) CampaignSummary {
	s := CampaignSummary{
		Timestamp:  time.Now().Format(time.RFC3339),
		ModelStats: make(map[string]ModelSummary),
	}
	if len(trials) == 0 {
		return s
	}

	var latencies []int64
	var compliances []float64

	modelTrials := make(map[string][]Trial)
	for _, t := range trials {
		modelTrials[t.Model] = append(modelTrials[t.Model], t)
		if t.HTTPStatus == 200 {
			latencies = append(latencies, t.LatencyMS)
			compliances = append(compliances, t.ComplianceScore)
			if t.FormatCorrect && t.SemanticCorrect {
				s.Successful++
			}
		}
	}

	s.TotalTrials = len(trials)
	if s.TotalTrials > 0 {
		s.SuccessRate = float64(s.Successful) / float64(s.TotalTrials)
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	if len(latencies) > 0 {
		s.P50LatencyMS = latencies[len(latencies)/2]
		s.P95LatencyMS = latencies[int(float64(len(latencies))*0.95)]
	}

	if len(compliances) > 0 {
		var sum float64
		for _, c := range compliances {
			sum += c
		}
		s.AvgCompliance = sum / float64(len(compliances))
	}

	for mName, mTr := range modelTrials {
		var mLats []int64
		var mComp []float64
		var succ, fmtCorr int

		for _, t := range mTr {
			if t.HTTPStatus == 200 {
				mLats = append(mLats, t.LatencyMS)
				mComp = append(mComp, t.ComplianceScore)
				if t.FormatCorrect {
					fmtCorr++
				}
				if t.FormatCorrect && t.SemanticCorrect {
					succ++
				}
			}
		}

		sort.Slice(mLats, func(i, j int) bool { return mLats[i] < mLats[j] })
		ms := ModelSummary{
			TotalTrials:      len(mTr),
			SuccessfulTrials: succ,
		}
		if len(mTr) > 0 {
			ms.SuccessRate = float64(succ) / float64(len(mTr))
			ms.FormatCorrectRate = float64(fmtCorr) / float64(len(mTr))
		}
		if len(mLats) > 0 {
			ms.P50LatencyMS = mLats[len(mLats)/2]
			ms.P95LatencyMS = mLats[int(float64(len(mLats))*0.95)]
		}
		if len(mComp) > 0 {
			var sum float64
			for _, c := range mComp {
				sum += c
			}
			ms.AvgCompliance = sum / float64(len(mComp))
		}
		s.ModelStats[mName] = ms
	}

	return s
}

func writeReport(path string, s CampaignSummary, trials []Trial) {
	var sb strings.Builder
	sb.WriteString("# Phase 403 Live Fire Campaign Report: Header Prefix Key Parsing\n\n")
	sb.WriteString(fmt.Sprintf("**Date:** %s\n", s.Timestamp))
	sb.WriteString(fmt.Sprintf("**Total Trials:** %d\n", s.TotalTrials))
	sb.WriteString(fmt.Sprintf("**Overall Success Rate:** %.1f%% (%d/%d)\n", s.SuccessRate*100, s.Successful, s.TotalTrials))
	sb.WriteString(fmt.Sprintf("**Average Format Compliance:** %.2f\n", s.AvgCompliance))
	sb.WriteString(fmt.Sprintf("**P50 Latency:** %d ms\n", s.P50LatencyMS))
	sb.WriteString(fmt.Sprintf("**P95 Latency:** %d ms\n\n", s.P95LatencyMS))

	sb.WriteString("## Model Performance Summary\n\n")
	sb.WriteString("| Model | Success Rate | Avg Compliance | P50 Latency | P95 Latency |\n")
	sb.WriteString("| --- | --- | --- | --- | --- |\n")
	for m, ms := range s.ModelStats {
		sb.WriteString(fmt.Sprintf("| `%s` | %.1f%% (%d/%d) | %.2f | %d ms | %d ms |\n",
			m, ms.SuccessRate*100, ms.SuccessfulTrials, ms.TotalTrials, ms.AvgCompliance, ms.P50LatencyMS, ms.P95LatencyMS))
	}
	sb.WriteString("\n## Trial Details\n\n")
	sb.WriteString("| Model | Case | Status | Strategy | Compliance | Semantic | Latency |\n")
	sb.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, t := range trials {
		sb.WriteString(fmt.Sprintf("| `%s` | `%s` | %d | `%s` | %.2f | %v | %d ms |\n",
			t.Model, t.TaskCase, t.HTTPStatus, t.ParseStrategy, t.ComplianceScore, t.SemanticCorrect, t.LatencyMS))
	}

	os.WriteFile(path, []byte(sb.String()), 0644)
}
