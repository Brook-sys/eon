package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/provider/openai"
)

func main() {
	groqKey := os.Getenv("GROQ_API_KEY")
	if groqKey == "" {
		fmt.Println("ERROR: GROQ_API_KEY not set")
		os.Exit(1)
	}

	facts := []prompt.Fact{
		{ID: "F1", Text: "The official product launch occurred on 2024-09-15.", Required: true},
		{ID: "F2", Text: "A beta preview was released on 2024-03-20.", Required: true},
	}

	spec := domain.OperationSpec{
		SchemaVersion:     domain.SchemaVersionV1,
		ID:                "combined_stack_test@1",
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
		Task:                   "Extract the authoritative launch date from the provided facts. Output ONLY in this format: DATE: <YYYY-MM-DD> | CONFLICT: <YES|NO>",
		Facts:                  facts,
		AllowedOutputs:         []string{"DATE: <YYYY-MM-DD>", "CONFLICT: <YES|NO>"},
		AnswerFormat:           "DATE: <value>\nCONFLICT: <YES|NO>",
		UntrustedDataBounding:  true,
		FormatAntiForgeryGuard: false,
	}

	compRes, err := compiler.Compile(spec, pInput)
	if err != nil {
		fmt.Printf("Compile error: %v\n", err)
		os.Exit(1)
	}

	// Check if ReasoningEffort is auto-set by compiler
	fmt.Printf("Compiler auto-set ReasoningEffort: %q\n", compRes.Request.ReasoningEffort)
	fmt.Printf("ReasoningEffortSuppressed: %v\n", compRes.ReasoningEffortSuppressed)

	// Explicitly set ReasoningEffort for Qwen
	compRes.Request.MaxOutputTokens = 64
	compRes.Request.Temperature = 0.0
	compRes.Request.ReasoningEffort = "none" // EXPLICIT SET

	concurrency := 10

	cfg := openai.Config{
		APIKey:  groqKey,
		BaseURL: "https://api.groq.com/openai/v1",
		Model:   "qwen/qwen3.6-27b",
		Timeout: 60 * time.Second,
		Semaphore: &openai.SemaphoreConfig{
			MaxConcurrent:  3,
			AcquireTimeout: 10 * time.Second,
		},
		RateLimiter: &openai.RateLimiterConfig{
			RequestsPerMinute: 40,
			TokensPerMinute:   40000,
			InitialBurst:      40 / 6,
			AcquireTimeout:    15 * time.Second,
		},
	}

	fmt.Printf("\n=== CONCURRENT TEST: qwen/qwen3.6-27b with EXPLICIT ReasoningEffort=none (RPM=40, Semaphore=3, c=%d) ===\n", concurrency)
	client, err := openai.New(cfg)
	if err != nil {
		fmt.Printf("Client setup error: %v\n", err)
		os.Exit(1)
	}

	var (
		wg          sync.WaitGroup
		success     atomic.Int32
		fail429     atomic.Int32
		failFormat  atomic.Int32
		failOther   atomic.Int32
		latencies   []int64
		latMu       sync.Mutex
		startTime   = time.Now()
	)

	for i := 1; i <= concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			start := time.Now()
			resp, err := client.Complete(ctx, compRes.Request)
			latency := time.Since(start).Milliseconds()

			latMu.Lock()
			latencies = append(latencies, latency)
			latMu.Unlock()

			if err != nil {
				errStr := err.Error()
				if len(errStr) > 120 {
					errStr = errStr[:120]
				}
				if strings.Contains(errStr, "429") || strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "RATE_LIMIT") {
					fail429.Add(1)
					fmt.Printf("  [%d] FAIL_429 (%dms): %s\n", idx, latency, errStr)
				} else {
					failOther.Add(1)
					fmt.Printf("  [%d] FAIL_OTHER (%dms): %s\n", idx, latency, errStr)
				}
				return
			}

			text := strings.TrimSpace(resp.Text)
			validFormat := strings.HasPrefix(text, "DATE:") && strings.Contains(text, "CONFLICT:")

			if !validFormat {
				failFormat.Add(1)
				preview := text
				if len(preview) > 80 {
					preview = preview[:80]
				}
				fmt.Printf("  [%d] FAIL_FORMAT (%dms) Tok=%d/%d Text=%q\n", idx, latency, resp.InputTokens, resp.OutputTokens, preview)
				return
			}

			success.Add(1)
			preview := text
			if len(preview) > 80 {
				preview = preview[:80]
			}
			fmt.Printf("  [%d] OK (%dms) Tok=%d/%d Text=%q\n", idx, latency, resp.InputTokens, resp.OutputTokens, preview)
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(startTime)

	latMu.Lock()
	if len(latencies) > 0 {
		for i := 0; i < len(latencies); i++ {
			for j := i + 1; j < len(latencies); j++ {
				if latencies[i] > latencies[j] {
					latencies[i], latencies[j] = latencies[j], latencies[i]
				}
			}
		}
		p50 := latencies[len(latencies)/2]
		p90 := latencies[len(latencies)*9/10]
		p99 := latencies[len(latencies)*99/100]
		fmt.Printf("  Latency: P50=%dms P90=%dms P99=%dms\n", p50, p90, p99)
	}
	latMu.Unlock()

	fmt.Printf("  Summary: %d success, %d 429, %d format_fail, %d other_fail | Wall time: %s\n",
		success.Load(), fail429.Load(), failFormat.Load(), failOther.Load(), elapsed.Round(time.Millisecond))
}