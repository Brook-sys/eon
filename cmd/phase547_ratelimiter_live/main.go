// Phase 547 — Token Bucket Rate Limiter Live Validation
//
// Objective: Validate that the provider-level token bucket rate limiter prevents
// 429 cascade by enforcing RPM/TPM limits over time, compared to uncoordinated baseline.
//
// Hypothesis: With rate limiter (RPM configured per-model), all requests succeed
// without 429s. Without rate limiter, 429 cascade occurs under sustained load.
//
// Models: llama-3.3-70b-versatile (known ~30 RPM on Groq)
//         qwen/qwen3.6-27b (known ~60 RPM on Groq)
//
// Load: 50 requests per model per scenario
//       Rate limiter: RPM per-model, TPM=RPM*1000, AcquireTimeout=10s
//       Total: 200 live trials (2 models × 2 scenarios × 50)

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/provider/openai"
)

type TrialResult struct {
	Provider         string  `json:"provider"`
	Model            string  `json:"model"`
	Scenario         string  `json:"scenario"` // RATE_LIMITER_OFF, RATE_LIMITER_ON
	WorkerID         int     `json:"worker_id"`
	Trial            int     `json:"trial"`
	MaxTokens        int     `json:"max_tokens"`
	Temperature      float64 `json:"temperature"`
	Status           string  `json:"status"` // SUCCESS, FAIL_429, FAIL_5XX, FAIL_RATE_TIMEOUT, FAIL_OTHER
	LatencyMs        int64   `json:"latency_ms"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	HTTPStatus       int     `json:"http_status,omitempty"`
	Error            string  `json:"error,omitempty"`
}

func main() {
	groqKey := os.Getenv("GROQ_API_KEY")
	if groqKey == "" {
		fmt.Println("ERROR: GROQ_API_KEY not set")
		os.Exit(1)
	}

	// Known Groq RPM limits per model (from Phase 546 evidence)
	modelRPM := map[string]int{
		"llama-3.3-70b-versatile": 30,
		"qwen/qwen3.6-27b":        60,
	}

	models := []string{
		"llama-3.3-70b-versatile",
		"qwen/qwen3.6-27b",
	}

	facts := []prompt.Fact{
		{ID: "F1", Text: "The official product launch occurred on 2024-09-15.", Required: true},
		{ID: "F2", Text: "A beta preview was released on 2024-03-20.", Required: true},
	}

	spec := domain.OperationSpec{
		SchemaVersion:     domain.SchemaVersionV1,
		ID:                "phase547@1",
		ContractVersion:   1,
		TemplateVersion:   1,
		InputSchema:       "facts",
		OutputSchema:      "key-value",
		Budget:            domain.Budget{ModelCalls: 1, Tokens: 2048, Attempts: 1},
		MaxOutputTokens:   128,
		SafetyMargin:      16,
		Validators:        []string{"schema"},
		RetryPolicy:       "never",
		FallbackPolicy:    "abort",
		MaximumAuthority:  domain.AuthorityProposeOnly,
	}

	compiler := prompt.Compiler{
		Estimator:             prompt.ConservativeEstimator{},
		ProviderContextTokens: 4096,
	}

	pInput := prompt.Input{
		Task:                  "Extract the authoritative launch date from the provided facts. Output ONLY in this format: DATE: <YYYY-MM-DD> | CONFLICT: <YES|NO>",
		Facts:                 facts,
		AllowedOutputs:        []string{"DATE: <YYYY-MM-DD>", "CONFLICT: <YES|NO>"},
		AnswerFormat          : "DATE: <value>\nCONFLICT: <YES|NO>",
		UntrustedDataBounding: true,
	}

	compRes, err := compiler.Compile(spec, pInput)
	if err != nil {
		fmt.Printf("Compile error: %v\n", err)
		os.Exit(1)
	}

	compiledReq := compRes.Request
	compiledReq.MaxOutputTokens = 64
	compiledReq.Temperature = 0.0

	var allResults []TrialResult
	var mu sync.Mutex

	// Sequential workers (not concurrent) to measure pure rate limiting over time
	// We want to test sustained throughput, not burst concurrency
	sequentialWorkers := 50 // 50 sequential requests per scenario
	rateTimeout := 10 * time.Second

	for _, model := range models {
		req := compiledReq
		if strings.Contains(model, "qwen") {
			req.ReasoningEffort = "none"
		}

		rpm := modelRPM[model]
		tpm := rpm * 1000 // rough token estimate

		// Scenario 1: RATE_LIMITER_OFF (no rate limiter, uncoordinated)
		fmt.Printf("\n=== Model: %s | Scenario: RATE_LIMITER_OFF (50 sequential, no rate limiter) ===\n", model)
		runSequentialBatch(groqKey, model, req, "RATE_LIMITER_OFF", nil, sequentialWorkers, &allResults, &mu)

		// Extended cooldown for rate window reset
		fmt.Printf("  Cooldown 30s for rate window reset...\n")
		time.Sleep(30 * time.Second)

		// Scenario 2: RATE_LIMITER_ON (token bucket with RPM/TPM)
		fmt.Printf("\n=== Model: %s | Scenario: RATE_LIMITER_ON (50 sequential, RPM=%d, TPM=%d, timeout=%v) ===\n",
			model, rpm, tpm, rateTimeout)
		rateCfg := &openai.RateLimiterConfig{
			RequestsPerMinute: rpm,
			TokensPerMinute:   tpm,
			InitialBurst:      rpm / 6, // ~10 seconds headroom
			AcquireTimeout:    rateTimeout,
		}
		runSequentialBatch(groqKey, model, req, "RATE_LIMITER_ON", rateCfg, sequentialWorkers, &allResults, &mu)

		// Extended cooldown between models
		fmt.Printf("  Cooldown 30s between models...\n")
		time.Sleep(30 * time.Second)
	}

	// Save results
	resultsDir := filepath.Join("results", "phase547_ratelimiter_live")
	os.MkdirAll(resultsDir, 0755)

	summary := computeSummary(allResults)

	output := map[string]interface{}{
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
		"phase":        547,
		"description":  "Token bucket rate limiter live validation",
		"models":       models,
		"model_rpm":    modelRPM,
		"scenarios":    []string{"RATE_LIMITER_OFF", "RATE_LIMITER_ON"},
		"rate_limiter": map[string]interface{}{
			"acquire_timeout": rateTimeout.String(),
		},
		"load": map[string]int{
			"sequential_requests": sequentialWorkers,
			"total_per_cell":      sequentialWorkers,
		},
		"results": allResults,
		"summary": summary,
	}

	data, _ := json.MarshalIndent(output, "", "  ")
	outPath := filepath.Join(resultsDir, "results.json")
	os.WriteFile(outPath, data, 0644)

	fmt.Printf("\n=== Phase 547 Complete ===\n")
	fmt.Printf("Results saved to %s\n", outPath)
	fmt.Printf("Total trials: %d\n", len(allResults))

	// Print summary
	fmt.Printf("\n--- Summary ---\n")
	for _, s := range summary {
		fmt.Printf("  %s/%s | %s: %d/%d SUCCESS | %d 429s | %d 5xx | %d rate_timeout | %d other | P50=%dms P95=%dms\n",
			s.Provider, s.Model, s.Scenario,
			s.Success, s.Total,
			s.HTTP429, s.HTTP5xx, s.RateTimeout, s.Other,
			s.P50Latency, s.P95Latency)
	}
}

type SummaryEntry struct {
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	Scenario     string `json:"scenario"`
	Total        int    `json:"total"`
	Success      int    `json:"success"`
	HTTP429      int    `json:"http_429"`
	HTTP5xx      int    `json:"http_5xx"`
	RateTimeout  int    `json:"rate_limiter_timeout"`
	Other        int    `json:"other"`
	P50Latency   int64  `json:"p50_latency_ms"`
	P95Latency   int64  `json:"p95_latency_ms"`
	AvgLatency   int64  `json:"avg_latency_ms"`
}

func computeSummary(results []TrialResult) []SummaryEntry {
	type key struct{ provider, model, scenario string }
	groups := make(map[key][]TrialResult)
	for _, r := range results {
		k := key{r.Provider, r.Model, r.Scenario}
		groups[k] = append(groups[k], r)
	}
	var out []SummaryEntry
	for k, rs := range groups {
		var lats []int64
		s := SummaryEntry{Provider: k.provider, Model: k.model, Scenario: k.scenario, Total: len(rs)}
		for _, r := range rs {
			if r.Status == "SUCCESS" {
				s.Success++
			}
			if r.HTTPStatus == 429 {
				s.HTTP429++
			} else if r.HTTPStatus >= 500 {
				s.HTTP5xx++
			} else if r.Status == "FAIL_RATE_TIMEOUT" {
				s.RateTimeout++
			} else if r.Status != "SUCCESS" {
				s.Other++
			}
			lats = append(lats, r.LatencyMs)
		}
		sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
		if len(lats) > 0 {
			s.P50Latency = lats[len(lats)/2]
			s.P95Latency = lats[int(float64(len(lats))*0.95)]
			if s.P95Latency == 0 && len(lats) > 0 {
				s.P95Latency = lats[len(lats)-1]
			}
			var sum int64
			for _, l := range lats {
				sum += l
			}
			s.AvgLatency = sum / int64(len(lats))
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Model != out[j].Model {
			return out[i].Model < out[j].Model
		}
		return out[i].Scenario < out[j].Scenario
	})
	return out
}

func runSequentialBatch(apiKey, model string, compiledReq port.CompletionRequest, scenario string, rateCfg *openai.RateLimiterConfig, trials int, allResults *[]TrialResult, mu *sync.Mutex) {
	cfg := openai.Config{
		APIKey:       apiKey,
		BaseURL:      "https://api.groq.com/openai/v1",
		Model:        model,
		Timeout:      60 * time.Second,
		RateLimiter:  rateCfg,
	}

	req := compiledReq

	var total429 int32
	var totalSuccess int32
	var totalFail int32
	var totalRateTimeout int32

	client, err := openai.New(cfg)
	if err != nil {
		fmt.Printf("  Client setup error: %v\n", err)
		return
	}

	for trial := 1; trial <= trials; trial++ {
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		resp, err := client.Complete(ctx, req)
		latency := time.Since(start).Milliseconds()
		cancel()

		tr := TrialResult{
			Provider:    "groq",
			Model:       model,
			Scenario:    scenario,
			WorkerID:    1,
			Trial:       trial,
			MaxTokens:   64,
			Temperature: 0.0,
			LatencyMs:   latency,
		}

		if err != nil {
			errStr := err.Error()
			tr.Status = "FAIL_OTHER"
			tr.Error = truncate(errStr, 200)
			if strings.Contains(errStr, "429") {
				tr.Status = "FAIL_429"
				tr.HTTPStatus = 429
				atomic.AddInt32(&total429, 1)
			} else if strings.Contains(errStr, "500") || strings.Contains(errStr, "502") || strings.Contains(errStr, "503") || strings.Contains(errStr, "504") {
				tr.Status = "FAIL_5XX"
				tr.HTTPStatus = 500
			} else if strings.Contains(errStr, "rate limiter acquire timeout") || strings.Contains(errStr, "ErrRateLimitTimeout") {
				tr.Status = "FAIL_RATE_TIMEOUT"
				atomic.AddInt32(&totalRateTimeout, 1)
			}
			atomic.AddInt32(&totalFail, 1)
			fmt.Printf("  [T%d] FAIL: %s (%dms)\n", trial, tr.Status, latency)
		} else {
			tr.Status = "SUCCESS"
			tr.PromptTokens = resp.InputTokens
			tr.CompletionTokens = resp.OutputTokens
			tr.TotalTokens = resp.InputTokens + resp.OutputTokens
			atomic.AddInt32(&totalSuccess, 1)
			text := strings.TrimSpace(resp.Text)
			if len(text) > 60 {
				text = text[:60]
			}
			fmt.Printf("  [T%d] OK Lat=%dms Tok=%d/%d Text=%q\n", trial, latency, resp.InputTokens, resp.OutputTokens, text)
		}

		mu.Lock()
		*allResults = append(*allResults, tr)
		mu.Unlock()

		// Small delay between sequential requests to observe rate limiter behavior
		// (without this, we'd just hammer the API as fast as possible)
		if scenario == "RATE_LIMITER_ON" {
			// Let the rate limiter pace us - no extra sleep needed
		} else {
			// For OFF scenario, small delay to not instantly exhaust
			time.Sleep(100 * time.Millisecond)
		}
	}

	fmt.Printf("  Subtotal: %d success, %d 429, %d rate_timeout, %d other fail\n",
		atomic.LoadInt32(&totalSuccess), atomic.LoadInt32(&total429), atomic.LoadInt32(&totalRateTimeout), atomic.LoadInt32(&totalFail))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}