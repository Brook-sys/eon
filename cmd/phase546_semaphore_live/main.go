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

// Phase 546 — Semaphore Concurrency Control Live Validation
//
// Objective: Validate that the provider-level semaphore prevents 429 cascade
// by gating concurrent outbound requests, compared to the uncoordinated baseline.
//
// Hypothesis: With semaphore (MaxConcurrent=3), all requests succeed without 429s.
// Without semaphore (or MaxConcurrent >= concurrency), 429 cascade occurs.
//
// Models: llama-3.3-70b-versatile (known 429 cascade at c>=5 from Phase 543/545)
//         qwen/qwen3.6-27b (resilient at c=3, cascades at c=5)
//
// Load: 10 concurrent workers × 5 trials each = 50 requests per model per scenario
//       Semaphore: MaxConcurrent=3, AcquireTimeout=5s
//       Total: 200 live trials (2 models × 2 scenarios × 50)

type TrialResult struct {
	Provider         string  `json:"provider"`
	Model            string  `json:"model"`
	Scenario         string  `json:"scenario"` // SEMAPHORE_ON, SEMAPHORE_OFF
	WorkerID         int     `json:"worker_id"`
	Trial            int     `json:"trial"`
	MaxTokens        int     `json:"max_tokens"`
	Temperature      float64 `json:"temperature"`
	Status           string  `json:"status"` // SUCCESS, FAIL_429, FAIL_5XX, FAIL_SEM_TIMEOUT, FAIL_OTHER
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
	}

	facts := []prompt.Fact{
		{ID: "F1", Text: "The official product launch occurred on 2024-09-15.", Required: true},
		{ID: "F2", Text: "A beta preview was released on 2024-03-20.", Required: true},
	}

	spec := domain.OperationSpec{
		SchemaVersion:     domain.SchemaVersionV1,
		ID:                "phase546@1",
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

	concurrency := 10
	trialsPerWorker := 5
	semaphoreLimit := 3 // MaxConcurrent for semaphore test
	acquireTimeout := 5 * time.Second

	for _, model := range models {
		req := compiledReq
		if strings.Contains(model, "qwen") {
			req.ReasoningEffort = "none"
		}

		// Scenario 1: SEMAPHORE_OFF (no semaphore, uncoordinated)
		fmt.Printf("\n=== Model: %s | Scenario: SEMAPHORE_OFF (c=%d, trials=%d, no semaphore) ===\n", model, concurrency, trialsPerWorker)
		runBatch(groqKey, model, req, "SEMAPHORE_OFF", nil, nil, concurrency, trialsPerWorker, &allResults, &mu)

		// Cooldown
		fmt.Printf("  Cooldown 5s between scenarios...\n")
		time.Sleep(5 * time.Second)

		// Scenario 2: SEMAPHORE_ON (semaphore with MaxConcurrent=3)
		fmt.Printf("\n=== Model: %s | Scenario: SEMAPHORE_ON (c=%d, trials=%d, semaphore=%d, timeout=%v) ===\n",
			model, concurrency, trialsPerWorker, semaphoreLimit, acquireTimeout)
		semCfg := &openai.SemaphoreConfig{
			MaxConcurrent:   semaphoreLimit,
			AcquireTimeout:  acquireTimeout,
		}
		runBatch(groqKey, model, req, "SEMAPHORE_ON", semCfg, nil, concurrency, trialsPerWorker, &allResults, &mu)

		// Cooldown between models
		fmt.Printf("  Cooldown 8s between models...\n")
		time.Sleep(8 * time.Second)
	}

	// Save results
	resultsDir := filepath.Join("results", "phase546_semaphore_live")
	os.MkdirAll(resultsDir, 0755)

	summary := computeSummary(allResults)

	output := map[string]interface{}{
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
		"phase":        546,
		"description":  "Semaphore concurrency control live validation",
		"models":       models,
		"scenarios":    []string{"SEMAPHORE_OFF", "SEMAPHORE_ON"},
		"semaphore": map[string]interface{}{
			"max_concurrent":   semaphoreLimit,
			"acquire_timeout":  acquireTimeout.String(),
		},
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

	fmt.Printf("\n=== Phase 546 Complete ===\n")
	fmt.Printf("Results saved to %s\n", outPath)
	fmt.Printf("Total trials: %d\n", len(allResults))

	// Print summary
	fmt.Printf("\n--- Summary ---\n")
	for _, s := range summary {
		fmt.Printf("  %s/%s | %s: %d/%d SUCCESS | %d 429s | %d 5xx | %d sem_timeout | %d other | P50=%dms P95=%dms\n",
			s.Provider, s.Model, s.Scenario,
			s.Success, s.Total,
			s.HTTP429, s.HTTP5xx, s.SemTimeout, s.Other,
			s.P50Latency, s.P95Latency)
	}
}

type SummaryEntry struct {
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	Scenario    string `json:"scenario"`
	Total       int    `json:"total"`
	Success     int    `json:"success"`
	HTTP429     int    `json:"http_429"`
	HTTP5xx     int    `json:"http_5xx"`
	SemTimeout  int    `json:"semaphore_timeout"`
	Other       int    `json:"other"`
	P50Latency  int64  `json:"p50_latency_ms"`
	P95Latency  int64  `json:"p95_latency_ms"`
	AvgLatency  int64  `json:"avg_latency_ms"`
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
			} else if r.Status == "FAIL_SEM_TIMEOUT" {
				s.SemTimeout++
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

func runBatch(apiKey, model string, compiledReq port.CompletionRequest, scenario string, semCfg *openai.SemaphoreConfig, retryCfg *openai.RetryConfig, concurrency, trialsPerWorker int, allResults *[]TrialResult, mu *sync.Mutex) {
	cfg := openai.Config{
		APIKey:    apiKey,
		BaseURL:   "https://api.groq.com/openai/v1",
		Model:     model,
		Timeout:   60 * time.Second,
		Retry:     retryCfg,
		Semaphore: semCfg,
	}

	req := compiledReq

	var wg sync.WaitGroup
	var total429 int32
	var totalSuccess int32
	var totalFail int32
	var totalSemTimeout int32

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
					} else if strings.Contains(errStr, "semaphore acquire timeout") || strings.Contains(errStr, "ErrSemaphoreTimeout") {
						tr.Status = "FAIL_SEM_TIMEOUT"
						atomic.AddInt32(&totalSemTimeout, 1)
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

	fmt.Printf("  Subtotal: %d success, %d 429, %d sem_timeout, %d other fail\n",
		atomic.LoadInt32(&totalSuccess), atomic.LoadInt32(&total429), atomic.LoadInt32(&totalSemTimeout), atomic.LoadInt32(&totalFail))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}