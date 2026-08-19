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
		ID:                "rpm_calibration_test@1",
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

	// Models to calibrate - only models that exist on Groq
	models := []struct {
		name      string
		reasoning string
		startRPM  int
	}{
		{"qwen/qwen3.6-27b", "none", 60},
		{"openai/gpt-oss-20b", "", 30},
		{"groq/compound", "", 30},
	}

	concurrency := 10 // c=10 stress test
	attemptsPerRPM := 15

	fmt.Printf("=== RPM CALIBRATION CAMPAIGN ===\n")
	fmt.Printf("Concurrency: %d, Attempts per RPM: %d\n\n", concurrency, attemptsPerRPM)

	for _, m := range models {
		fmt.Printf("\n>>>> CALIBRATING: %s (start RPM=%d) <<<<\n", m.name, m.startRPM)

		// Binary search for max safe RPM
		low := 5
		high := m.startRPM * 2 // Upper bound
		bestSafeRPM := 0

		for low <= high {
			mid := (low + high) / 2
			fmt.Printf("\n  Testing RPM=%d (semaphore=3, c=%d, trials=%d)...\n", mid, concurrency, attemptsPerRPM)

			req := compiledReq
			if m.reasoning != "" {
				req.ReasoningEffort = m.reasoning
			}

			cfg := openai.Config{
				APIKey:  groqKey,
				BaseURL: "https://api.groq.com/openai/v1",
				Model:   m.name,
				Timeout: 60 * time.Second,
				Semaphore: &openai.SemaphoreConfig{
					MaxConcurrent:  3,
					AcquireTimeout: 10 * time.Second,
				},
				RateLimiter: &openai.RateLimiterConfig{
					RequestsPerMinute: mid,
					TokensPerMinute:   mid * 1000,
					InitialBurst:      mid / 6,
					AcquireTimeout:    15 * time.Second,
				},
			}

			client, err := openai.New(cfg)
			if err != nil {
				fmt.Printf("    Client setup error: %v\n", err)
				high = mid - 1
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
			)

			for i := 1; i <= attemptsPerRPM; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()

					ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
					defer cancel()

					start := time.Now()
					resp, err := client.Complete(ctx, req)
					latency := time.Since(start).Milliseconds()

					latMu.Lock()
					latencies = append(latencies, latency)
					latMu.Unlock()

					if err != nil {
						errStr := err.Error()
						if strings.Contains(errStr, "429") || strings.Contains(errStr, "rate limit") {
							fail429.Add(1)
						} else {
							failOther.Add(1)
						}
						return
					}

					text := strings.TrimSpace(resp.Text)
					validFormat := strings.HasPrefix(text, "DATE:") && strings.Contains(text, "CONFLICT:")

					if !validFormat {
						failFormat.Add(1)
						return
					}

					success.Add(1)
				}(i)
			}

			wg.Wait()

			// Calculate P50
			var p50 int64
			latMu.Lock()
			if len(latencies) > 0 {
				for i := 0; i < len(latencies); i++ {
					for j := i + 1; j < len(latencies); j++ {
						if latencies[i] > latencies[j] {
							latencies[i], latencies[j] = latencies[j], latencies[i]
						}
					}
				}
				p50 = latencies[len(latencies)/2]
			}
			latMu.Unlock()

			s := success.Load()
			f429 := fail429.Load()
			ffmt := failFormat.Load()
			foth := failOther.Load()

			fmt.Printf("    Results: success=%d 429=%d format_fail=%d other_fail=%d P50=%dms\n", s, f429, ffmt, foth, p50)

			// Success criteria: 90%+ success rate, zero format failures
			successRate := float64(s) / float64(attemptsPerRPM)
			if successRate >= 0.9 && ffmt == 0 && f429 == 0 {
				fmt.Printf("    ✓ RPM %d PASSED (%.0f%% success)\n", mid, successRate*100)
				bestSafeRPM = mid
				low = mid + 1
			} else {
				fmt.Printf("    ✗ RPM %d FAILED (%.0f%% success, 429s=%d)\n", mid, successRate*100, f429)
				high = mid - 1
			}

			// Cooldown between RPM tests to let rate window reset
			fmt.Printf("    Cooldown 5s...\n")
			time.Sleep(5 * time.Second)
		}

		fmt.Printf("\n  >>> BEST SAFE RPM FOR %s: %d <<<\n", m.name, bestSafeRPM)
	}

	fmt.Printf("\n=== CALIBRATION COMPLETE ===\n")
}