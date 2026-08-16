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

// Phase 545 — Live 429-induced Retry Recovery Stress Test
//
// Objective: Deliberately trigger Groq 429 rate limits with high concurrency
// and verify the bounded retry with exponential backoff recovers requests
// that would otherwise fail. Compare retry-enabled vs retry-disabled under
// the same load.
//
// Hypothesis: With retry enabled (MaxAttempts=3, BaseDelay=200ms, MaxDelay=10s),
// a significant fraction of 429'd requests will recover after backoff.
// Without retry, all 429'd requests are terminal failures.
//
// Models: llama-3.3-70b-versatile (known 429 cascade at c>=5)
//         qwen/qwen3.6-27b (resilient at c=3, cascades at c=5)
//         openai/gpt-oss-20b (broader coverage)
//
// Load: 10 concurrent workers × 5 trials each = 50 requests per model
//       per retry config. Total: 300 live trials.

type TrialResult struct {
	Provider         string  `json:"provider"`
	Model            string  `json:"model"`
	Scenario         string  `json:"scenario"` // RETRY_ON, RETRY_OFF
	WorkerID         int     `json:"worker_id"`
	Trial            int     `json:"trial"`
	MaxTokens        int     `json:"max_tokens"`
	Temperature      float64 `json:"temperature"`
	Status           string  `json:"status"` // SUCCESS, FAIL_429, FAIL_5XX, FAIL_OTHER
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

	models := []string{
		"llama-3.3-70b-versatile",
		"qwen/qwen3.6-27b",
		"openai/gpt-oss-20b",
	}

	facts := []prompt.Fact{
		{ID: "F1", Text: "The official product launch occurred on 2024-09-15.", Required: true},
		{ID: "F2", Text: "A beta preview was released on 2024-03-20.", Required: true},
	}

	spec := domain.OperationSpec{
		SchemaVersion:     domain.SchemaVersionV1,
		ID:                "phase545@1",
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
		AnswerFormat:          "DATE: <value>\nCONFLICT: <YES|NO>",
		UntrustedDataBounding: true,
	}

	// For qwen models, add thinking overhead suppression
	compRes, err := compiler.Compile(spec, pInput)
	if err != nil {
		fmt.Printf("Compile error: %v\n", err)
		os.Exit(1)
	}

	// Prepare the compiled request once; each trial will reuse it
	compiledReq := compRes.Request
	compiledReq.MaxOutputTokens = 64
	compiledReq.Temperature = 0.0

	var allResults []TrialResult
	var mu sync.Mutex

	concurrency := 10
	trialsPerWorker := 5

	for _, model := range models {
		// Adjust reasoning effort for qwen
		req := compiledReq
		if strings.Contains(model, "qwen") {
			req.ReasoningEffort = "none"
		}

		fmt.Printf("\n=== Model: %s | Scenario: RETRY_OFF (c=%d, trials=%d) ===\n", model, concurrency, trialsPerWorker)
		runBatch(groqKey, model, req, "RETRY_OFF", nil, concurrency, trialsPerWorker, &allResults, &mu)

		// Brief cooldown between scenarios to let rate limit windows reset
		fmt.Printf("  Cooldown 3s between scenarios...\n")
		time.Sleep(3 * time.Second)

		// Scenario 2: Retry ON (MaxAttempts=3, BaseDelay=200ms, MaxDelay=10s, Jitter=500ms)
		fmt.Printf("\n=== Model: %s | Scenario: RETRY_ON (c=%d, trials=%d, max=3, base=200ms, max=10s, jitter=500ms) ===\n", model, concurrency, trialsPerWorker)
		retryCfg := &openai.RetryConfig{
			MaxAttempts: 3,
			BaseDelay:   200 * time.Millisecond,
			MaxDelay:    10 * time.Second,
			MaxJitter:   500 * time.Millisecond,
		}
		runBatch(groqKey, model, req, "RETRY_ON", retryCfg, concurrency, trialsPerWorker, &allResults, &mu)

		// Cooldown between models
		fmt.Printf("  Cooldown 5s between models...\n")
		time.Sleep(5 * time.Second)
	}

	// Save results
	resultsDir := filepath.Join("results", "phase545_retry_stress_429_live")
	os.MkdirAll(resultsDir, 0755)

	summary := computeSummary(allResults)

	output := map[string]interface{}{
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"phase":       545,
		"description": "Live 429-induced retry recovery stress test",
		"models":      models,
		"scenarios":   []string{"RETRY_OFF", "RETRY_ON"},
		"load": map[string]int{
			"concurrency":       concurrency,
			"trials_per_worker": trialsPerWorker,
			"total_per_cell":    concurrency * trialsPerWorker,
		},
		"results": allResults,
		"summary": summary,
	}

	data, _ := json.MarshalIndent(output, "", "  ")
	outPath := filepath.Join(resultsDir, "results.json")
	os.WriteFile(outPath, data, 0644)

	fmt.Printf("\n=== Phase 545 Complete ===\n")
	fmt.Printf("Results saved to %s\n", outPath)
	fmt.Printf("Total trials: %d\n", len(allResults))

	// Print summary
	fmt.Printf("\n--- Summary ---\n")
	for _, s := range summary {
		fmt.Printf("  %s/%s | %s: %d/%d SUCCESS | %d 429s | %d 5xx | %d other | P50=%dms P95=%dms\n",
			s.Provider, s.Model, s.Scenario,
			s.Success, s.Total,
			s.HTTP429, s.HTTP5xx, s.Other,
			s.P50Latency, s.P95Latency)
	}
}

type SummaryEntry struct {
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Scenario   string `json:"scenario"`
	Total      int    `json:"total"`
	Success    int    `json:"success"`
	HTTP429    int    `json:"http_429"`
	HTTP5xx    int    `json:"http_5xx"`
	Other      int    `json:"other"`
	P50Latency int64  `json:"p50_latency_ms"`
	P95Latency int64  `json:"p95_latency_ms"`
	AvgLatency int64  `json:"avg_latency_ms"`
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

func runBatch(apiKey, model string, compiledReq port.CompletionRequest, scenario string, retryCfg *openai.RetryConfig, concurrency, trialsPerWorker int, allResults *[]TrialResult, mu *sync.Mutex) {
	cfg := openai.Config{
		APIKey:  apiKey,
		BaseURL: "https://api.groq.com/openai/v1",
		Model:   model,
		Timeout: 60 * time.Second,
		Retry:   retryCfg,
	}

	req := compiledReq

	var wg sync.WaitGroup
	var total429 int32
	var totalSuccess int32
	var totalFail int32

	for wid := 1; wid <= concurrency; wid++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			client, err := openai.New(cfg)
			if err != nil {
				fmt.Printf("  [W%d] Client setup error: %v\n", workerID, err)
				return
			}

			for trial := 1; trial <= trialsPerWorker; trial++ {
				start := time.Now()
				ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
				resp, err := client.Complete(ctx, req)
				latency := time.Since(start).Milliseconds()
				cancel()

				tr := TrialResult{
					Provider:    "groq",
					Model:       model,
					Scenario:    scenario,
					WorkerID:    workerID,
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
					}
					atomic.AddInt32(&totalFail, 1)
					fmt.Printf("  [W%d T%d] FAIL: %s (%dms)\n", workerID, trial, tr.Status, latency)
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
					fmt.Printf("  [W%d T%d] OK Lat=%dms Tok=%d/%d Text=%q\n", workerID, trial, latency, resp.InputTokens, resp.OutputTokens, text)
				}

				mu.Lock()
				*allResults = append(*allResults, tr)
				mu.Unlock()

				// No delay between trials — maximum pressure to trigger 429s
			}
		}(wid)
	}
	wg.Wait()

	fmt.Printf("  Subtotal: %d success, %d 429, %d other fail\n",
		atomic.LoadInt32(&totalSuccess), atomic.LoadInt32(&total429), atomic.LoadInt32(&totalFail))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
