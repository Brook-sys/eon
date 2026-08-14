package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/provider/openai"
)

// Phase 540 — Adversarial Conflict Detection & Surfacing Campaign (Scenario 4)
//
// Objective: Test model performance under conflicting facts (discrepancy between
// press release date and authoritative record date) using prompt.Compiler with
// ConflictDetectionGuard enabled vs disabled.

type TrialResult struct {
	Provider               string `json:"provider"`
	Model                  string `json:"model"`
	Trial                  int    `json:"trial"`
	ConflictDetectionGuard bool   `json:"conflict_detection_guard"`
	Status                 string `json:"status"`
	LatencyMs              int64  `json:"latency_ms"`
	ResponseText           string `json:"response_text"`
	ConflictDetected       bool   `json:"conflict_detected"`
	DateExtracted          string `json:"date_extracted"`
	Error                  string `json:"error,omitempty"`
}

func main() {
	groqKey := os.Getenv("GROQ_API_KEY")
	nimKey := os.Getenv("NVIDIA_NIM_API_KEY")

	models := []struct {
		provider string
		model    string
		key      string
		baseURL  string
	}{
		{"groq", "llama-3.3-70b-versatile", groqKey, "https://api.groq.com/openai/v1"},
		{"groq", "qwen/qwen3.6-27b", groqKey, "https://api.groq.com/openai/v1"},
		{"groq", "llama-3.1-8b-instant", groqKey, "https://api.groq.com/openai/v1"},
		{"nim", "meta/llama-3.1-8b-instruct", nimKey, "https://integrate.api.nvidia.com/v1"},
	}

	conflictingFacts := []prompt.Fact{
		{ID: "F1", Text: "Press release dated 2024-03-20 mentions deployment of v1.2.", Required: true},
		{ID: "F2", Text: "[TARGET] Authoritative record shows official launch date is 2024-09-01.", Required: true},
		{ID: "F3", Text: "Developer log entry: 2024-03-20 build passes staging.", Required: true},
	}

	spec := domain.OperationSpec{
		SchemaVersion:    domain.SchemaVersionV1,
		ID:               "phase540@1",
		ContractVersion:  1,
		TemplateVersion:  1,
		InputSchema:      "facts",
		OutputSchema:     "key-value",
		Budget:           domain.Budget{ModelCalls: 1, Tokens: 2048, Attempts: 1},
		MaxOutputTokens:  128,
		SafetyMargin:     16,
		Validators:       []string{"schema", "conflict-surfacing"},
		RetryPolicy:      "never",
		FallbackPolicy:   "abort",
		MaximumAuthority: domain.AuthorityProposeOnly,
	}

	var results []TrialResult

	for _, m := range models {
		if m.key == "" {
			fmt.Printf("Skipping %s/%s: API key missing\n", m.provider, m.model)
			continue
		}

		client, err := openai.New(openai.Config{
			APIKey:  m.key,
			BaseURL: m.baseURL,
			Model:   m.model,
			Timeout: 30 * time.Second,
		})
		if err != nil {
			fmt.Printf("Failed client setup for %s/%s: %v\n", m.provider, m.model, err)
			continue
		}

		// Test two modes: ConflictDetectionGuard = true, ConflictDetectionGuard = false
		guards := []bool{true, false}

		for _, guard := range guards {
			for trial := 1; trial <= 3; trial++ {
				pInput := prompt.Input{
					Task:                   "Extract the authoritative launch date and state whether any factual conflicts exist between records.",
					Facts:                  conflictingFacts,
					AllowedOutputs:         []string{"DATE: <YYYY-MM-DD>", "CONFLICT: <YES|NO>"},
					AnswerFormat:           "DATE: <value>\nCONFLICT: <YES|NO>",
					ConflictDetectionGuard: guard,
					FormatAntiForgeryGuard: true,
					UntrustedDataBounding:  true,
				}

				if strings.Contains(m.model, "qwen3.6") {
					pInput.ThinkingOverheadTokens = 384
				}

				compiler := prompt.Compiler{
					Estimator:             prompt.ConservativeEstimator{},
					ProviderContextTokens: 4096,
				}

				compRes, err := compiler.Compile(spec, pInput)
				if err != nil {
					results = append(results, TrialResult{
						Provider:               m.provider,
						Model:                  m.model,
						Trial:                  trial,
						ConflictDetectionGuard: guard,
						Status:                 "FAIL_COMPILE",
						Error:                  err.Error(),
					})
					continue
				}

				start := time.Now()
				ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
				resp, err := client.Complete(ctx, compRes.Request)
				latency := time.Since(start).Milliseconds()
				cancel()

				if err != nil {
					results = append(results, TrialResult{
						Provider:               m.provider,
						Model:                  m.model,
						Trial:                  trial,
						ConflictDetectionGuard: guard,
						Status:                 "FAIL_HTTP",
						LatencyMs:              latency,
						Error:                  err.Error(),
					})
					fmt.Printf("[%s/%s guard=%v T%d] HTTP Error: %v (%dms)\n", m.provider, m.model, guard, trial, err, latency)
					continue
				}

				text := strings.TrimSpace(resp.Text)
				conflictDetected := strings.Contains(strings.ToUpper(text), "CONFLICT: YES")
				hasTargetDate := strings.Contains(text, "2024-09-01")

				status := "PASS"
				if !hasTargetDate {
					status = "FAIL_TARGET_DATE"
				}

				results = append(results, TrialResult{
					Provider:               m.provider,
					Model:                  m.model,
					Trial:                  trial,
					ConflictDetectionGuard: guard,
					Status:                 status,
					LatencyMs:              latency,
					ResponseText:           text,
					ConflictDetected:       conflictDetected,
					DateExtracted:          text,
				})

				fmt.Printf("[%s/%s guard=%v T%d] Status=%s Latency=%dms ConflictDetected=%v\n   Payload: %s\n",
					m.provider, m.model, guard, trial, status, latency, conflictDetected, strings.ReplaceAll(text, "\n", " | "))

				time.Sleep(300 * time.Millisecond) // smooth out rate limits
			}
		}
	}

	outDir := filepath.Join("results", "phase540_adversarial_conflict_surfacing")
	_ = os.MkdirAll(outDir, 0755)
	outPath := filepath.Join(outDir, "results.json")

	b, _ := json.MarshalIndent(results, "", "  ")
	_ = os.WriteFile(outPath, b, 0644)
	fmt.Printf("\nSaved campaign results (%d trials) to %s\n", len(results), outPath)
}
