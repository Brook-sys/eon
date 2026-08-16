package main

import (
	"context"
	"fmt"
	"os"
	"strings"
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

	// Test combined semaphore (MaxConcurrent=3) + rate limiter
	models := []struct {
		name      string
		rpm       int
		tpm       int
		reasoning string
	}{
		{"llama-3.3-70b-versatile", 30, 30000, ""},
		{"qwen/qwen3.6-27b", 40, 40000, "none"},
	}

	for _, m := range models {
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
				RequestsPerMinute: m.rpm,
				TokensPerMinute:   m.tpm,
				InitialBurst:      m.rpm / 6,
				AcquireTimeout:    15 * time.Second,
			},
		}

		fmt.Printf("\n=== Testing %s (RPM=%d, Semaphore=3) ===\n", m.name, m.rpm)
		client, err := openai.New(cfg)
		if err != nil {
			fmt.Printf("Client setup error: %v\n", err)
			continue
		}

		success := 0
		fail429 := 0
		for i := 1; i <= 10; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			start := time.Now()
			resp, err := client.Complete(ctx, req)
			latency := time.Since(start).Milliseconds()
			cancel()

			if err != nil {
				errStr := err.Error()
				if len(errStr) > 100 {
					errStr = errStr[:100]
				}
				if strings.Contains(errStr, "429") {
					fail429++
					fmt.Printf("  [%d] FAIL_429 (%dms): %s\n", i, latency, errStr)
				} else {
					fmt.Printf("  [%d] FAIL_OTHER (%dms): %s\n", i, latency, errStr)
				}
			} else {
				success++
				text := resp.Text
				if len(text) > 80 {
					text = text[:80]
				}
				fmt.Printf("  [%d] OK (%dms) Tok=%d/%d Text=%q\n", i, latency, resp.InputTokens, resp.OutputTokens, text)
			}
		}
		fmt.Printf("  Summary: %d success, %d 429\n", success, fail429)
	}
}
