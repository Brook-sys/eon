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
	nimKey := os.Getenv("NVIDIA_NIM_API_KEY")
	if nimKey == "" {
		fmt.Println("ERROR: NVIDIA_NIM_API_KEY not set")
		os.Exit(1)
	}

	facts := []prompt.Fact{
		{ID: "F1", Text: "The official product launch occurred on 2024-09-15.", Required: true},
		{ID: "F2", Text: "A beta preview was released on 2024-03-20.", Required: true},
	}

	spec := domain.OperationSpec{
		SchemaVersion:     domain.SchemaVersionV1,
		ID:                "nim_cross_provider_test@1",
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

	// NIM models to test - focusing on strong models per HEARTBEAT guidance
	models := []struct {
		name      string
		rpm       int
		tpm       int
		reasoning string
	}{
		{"meta/llama-3.3-70b-instruct", 30, 30000, ""},
		{"meta/llama-3.1-8b-instruct", 40, 40000, ""},
	}

	concurrency := 5

	for _, m := range models {
		pInput := prompt.Input{
			Task:                   "Extract the authoritative launch date from the provided facts. Output ONLY in this format: DATE: <YYYY-MM-DD> | CONFLICT: <YES|NO>",
			Facts:                  facts,
			AllowedOutputs:         []string{"DATE: <YYYY-MM-DD>", "CONFLICT: <YES|NO>"},
			AnswerFormat:           "DATE: <value>\nCONFLICT: <YES|NO>",
			UntrustedDataBounding:  true,
			FormatAntiForgeryGuard: true,
		}

		compRes, err := compiler.Compile(spec, pInput)
		if err != nil {
			fmt.Printf("Compile error: %v\n", err)
			os.Exit(1)
		}

		compRes.Request.MaxOutputTokens = 64
		compRes.Request.Temperature = 0.0
		if m.reasoning != "" {
			compRes.Request.ReasoningEffort = m.reasoning
		}

		cfg := openai.Config{
			APIKey:  nimKey,
			BaseURL: "https://integrate.api.nvidia.com/v1",
			Model:   m.name,
			Timeout: 60 * time.Second,
			Semaphore: &openai.SemaphoreConfig{
				MaxConcurrent:  3,
				AcquireTimeout: 10 * time.Second,
			},
			RateLimiter: &openai.RateLimiterConfig{
				RequestsPerMinute: m.rpm,
				TokensPerMinute:   m.tpm,
				InitialBurst:      m.rpm / 6,
				AcquireTimeout:    15 * time.Second,
			},
		}

		fmt.Printf("\n=== NIM CONCURRENT TEST: %s (RPM=%d, Semaphore=3, c=%d) ===\n", m.name, m.rpm, concurrency)
		client, err := openai.New(cfg)
		if err != nil {
			fmt.Printf("Client setup error: %v\n", err)
			continue
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
					if len(preview) > 120 {
						preview = preview[:120]
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
}
