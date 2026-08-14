package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/provider/openai"
)

type TrialResult struct {
	Model            string  `json:"model"`
	LatencyMs        int64   `json:"latency"`
	Error            string  `json:"error,omitempty"`
	FormatCompliance float64 `json:"format_compliance"`
	ParsedStrategy   string  `json:"parsed_strategy"`
	Text             string  `json:"text,omitempty"`
}

func main() {
	if os.Getenv("NVIDIA_NIM_API_KEY") == "" {
		log.Fatal("NVIDIA_NIM_API_KEY is required")
	}

	outDir := filepath.Join("results", "phase527_nim_fire_test")
	os.MkdirAll(outDir, 0755)

	models := []string{
		"deepseek-ai/deepseek-v4-flash-0731",
		"nvidia/llama-3.3-nemotron-super-49b-v1.5",
	}

	var results []TrialResult

	exec := kernel.ModelExecutor{
		Compiler: prompt.Compiler{
			Estimator:             prompt.ConservativeEstimator{},
			ProviderContextTokens: 8192,
		},
	}

	for _, model := range models {
		provider, err := openai.New(openai.Config{
			BaseURL: "https://integrate.api.nvidia.com/v1",
			APIKey:  os.Getenv("NVIDIA_NIM_API_KEY"),
			Model:   model,
		})
		if err != nil {
			log.Fatal(err)
		}

		var p port.ModelProvider = provider

		for i := 0; i < 3; i++ {
			log.Printf("Running %s (Trial %d)", model, i+1)

			start := time.Now()

			spec := domain.OperationSpec{
				SchemaVersion:    domain.SchemaVersionV1,
				ID:               "test-spec",
				ContractVersion:  1,
				TemplateVersion:  2,
				InputSchema:      "in",
				OutputSchema:     "out",
				Budget:           domain.Budget{Tokens: 8192, ModelCalls: 1, Bytes: 1000, Attempts: 1, Duration: time.Minute},
				MaxOutputTokens:  128,
				SafetyMargin:     256,
				Validators:       []string{"a"},
				RetryPolicy:      "a",
				FallbackPolicy:   "a",
				MaximumAuthority: domain.AuthorityProposeOnly,
			}

			input := prompt.Input{
				Task:           "Extract the STATUS. STATUS must be OK.",
				AnswerFormat:   "STATUS: <status>",
				AllowedOutputs: []string{"STATUS: <value>"},
				Facts: []prompt.Fact{
					{ID: "F1", Text: "System diagnostic: the system status is OK.", Required: true},
				},
				UntrustedDataBounding: true,
				AntiPoisoningGuard:    true,
				FormatAnchoring:       prompt.FormatAnchoringStrict,
				ThinkingOverheadTokens: domain.ResolveThinkingOverheadTokens(model),
			}

			compiled, err := exec.Compiler.Compile(spec, input)
			if err != nil {
				results = append(results, TrialResult{Model: model, Error: err.Error()})
				continue
			}

			compiled.Request.ReasoningEffort = ""
			compiled.Request.PrefillAssistant = ""

			res, err := p.Complete(context.Background(), compiled.Request)
			latency := time.Since(start).Milliseconds()

			if err != nil {
				results = append(results, TrialResult{Model: model, LatencyMs: latency, Error: err.Error()})
				continue
			}

			parsed := prompt.ParseResponse(res.Text, []string{"STATUS:"})

			results = append(results, TrialResult{
				Model:            model,
				LatencyMs:        latency,
				FormatCompliance: parsed.FormatComplianceScore,
				ParsedStrategy:   string(parsed.Strategy),
				Text:             res.Text,
			})

			time.Sleep(1 * time.Second)
		}
	}

	b, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile(filepath.Join(outDir, "results.json"), b, 0644)
	log.Println("Phase 527 Done.")
}
