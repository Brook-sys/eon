package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/modeltext"
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/provider/openai"
)

func main() {
	groqKey := os.Getenv("GROQ_API_KEY")
	if groqKey == "" {
		fmt.Println("GROQ_API_KEY is not set")
		os.Exit(1)
	}

	// Target Groq models including reasoning (qwen3.6-27b, compound-mini) and standard (llama-3.3-70b-versatile)
	models := []struct {
		name      string
		maxTokens int
		overhead  int
	}{
		{name: "qwen/qwen3.6-27b", maxTokens: 1024, overhead: 384},
		{name: "groq/compound-mini", maxTokens: 1024, overhead: 256},
		{name: "llama-3.3-70b-versatile", maxTokens: 256, overhead: 0},
	}

	fmt.Println("Starting Phase 389 Live Campaign — Thinking/Reasoning Tag Stripping & Response Parsing...")

	resultsDir := filepath.Join("results", "thinking-parser-phase389")
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		fmt.Printf("Failed to create results dir: %v\n", err)
		os.Exit(1)
	}

	reportFile, err := os.Create(filepath.Join(resultsDir, "REPORT.md"))
	if err != nil {
		fmt.Printf("Failed to create report file: %v\n", err)
		os.Exit(1)
	}
	defer reportFile.Close()

	fmt.Fprintf(reportFile, "# Phase 389 Live Campaign — Thinking & Structured Response Parsing\n\n")
	fmt.Fprintf(reportFile, "Date: %s\n\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(reportFile, "| Model | Tokens | Latency (ms) | Raw Text Preview | StripThinking Applied | DATE Match | SOURCE Match |\n")
	fmt.Fprintf(reportFile, "|---|---|---|---|---|---|---|\n")

	totalCalls := 0
	passedCalls := 0

	for _, m := range models {
		fmt.Printf("Testing model: %s (max_tokens=%d)...\n", m.name, m.maxTokens)

		provider, err := openai.New(openai.Config{
			BaseURL: "https://api.groq.com/openai/v1",
			APIKey:  groqKey,
			Model:   m.name,
		})
		if err != nil {
			fmt.Printf("Failed to create provider for %s: %v\n", m.name, err)
			continue
		}

		promptInput := prompt.Input{
			Task: "Extract the exact date and source from the facts provided below.",
			Facts: []prompt.Fact{
				{ID: "F1", Text: "The transaction was recorded on 2026-08-08.", Required: true},
				{ID: "F2", Text: "Source system identifier is SYS-ALPHA-99.", Required: true},
			},
			AllowedOutputs:         []string{"DATE: <YYYY-MM-DD>", "SOURCE: <ID>"},
			AnswerFormat:           "DATE: YYYY-MM-DD\nSOURCE: SYS-ALPHA-99",
			FormatExample:          "DATE: 2026-08-08\nSOURCE: SYS-ALPHA-99",
			ThinkingOverheadTokens: m.overhead,
		}

		compiler := prompt.Compiler{
			Estimator:             prompt.ConservativeEstimator{},
			ProviderContextTokens: 4096,
		}

		spec := domainOperationSpec(m.maxTokens)
		compiled, err := compiler.Compile(spec, promptInput)
		if err != nil {
			fmt.Printf("Compilation error for %s: %v\n", m.name, err)
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		start := time.Now()
		req := compiled.Request
		resp, err := provider.Complete(ctx, req)
		latency := time.Since(start)
		cancel()

		totalCalls++

		if err != nil {
			fmt.Printf("Completion error for %s: %v\n", m.name, err)
			fmt.Fprintf(reportFile, "| %s | %d | %d | ERROR: %v | - | - | - |\n", m.name, m.maxTokens, latency.Milliseconds(), err)
			continue
		}

		norm := modeltext.NormalizeStructuredResponse(resp.Text)
		parsed := prompt.ParseResponse(resp.Text, []string{"DATE", "SOURCE"})

		dateMatch := parsed.Values["DATE"] == "2026-08-08"
		sourceMatch := parsed.Values["SOURCE"] == "SYS-ALPHA-99"

		if dateMatch && sourceMatch {
			passedCalls++
		}

		preview := strings.ReplaceAll(resp.Text, "\n", " ")
		if len(preview) > 60 {
			preview = preview[:60] + "..."
		}

		appliedStr := strings.Join(norm.Applied, ",")
		if appliedStr == "" {
			appliedStr = "none"
		}

		fmt.Fprintf(reportFile, "| %s | %d | %d | `%s` | %s | %v | %v |\n",
			m.name, m.maxTokens, latency.Milliseconds(), preview, appliedStr, dateMatch, sourceMatch)

		fmt.Printf("Model %s: latency=%dms, norm_applied=%v, date=%s, source=%s\n",
			m.name, latency.Milliseconds(), norm.Applied, parsed.Values["DATE"], parsed.Values["SOURCE"])

		time.Sleep(1 * time.Second) // rate limit pacing
	}

	fmt.Fprintf(reportFile, "\n**Summary:** Passed %d/%d live calls (%.1f%%).\n", passedCalls, totalCalls, float64(passedCalls)/float64(totalCalls)*100)
	fmt.Printf("Campaign completed: %d/%d passed.\n", passedCalls, totalCalls)
}

func domainOperationSpec(maxTokens int) domain.OperationSpec {
	return domain.OperationSpec{
		SchemaVersion:    domain.SchemaVersionV1,
		ID:               "op-phase389",
		ContractVersion:  1,
		TemplateVersion:  1,
		InputSchema:      "schema-in-v1",
		OutputSchema:     "schema-out-v1",
		Budget:           domain.Budget{Tokens: 4096},
		MaxOutputTokens:  maxTokens,
		SafetyMargin:     128,
		Validators:       []string{"val1"},
		RetryPolicy:      "retry-standard",
		FallbackPolicy:   "fallback-standard",
		MaximumAuthority: domain.AuthorityProposeOnly,
	}
}
