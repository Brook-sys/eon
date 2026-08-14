package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/provider/openai"
)

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
		{"groq", "llama-3.1-8b-instant", groqKey, "https://api.groq.com/openai/v1"},
		{"nim", "meta/llama-3.1-8b-instruct", nimKey, "https://integrate.api.nvidia.com/v1"},
	}

	results := make([]map[string]interface{}, 0)

	compiler := prompt.Compiler{
		Estimator:             prompt.ConservativeEstimator{},
		ProviderContextTokens: 4096,
	}

	spec := domain.OperationSpec{
		SchemaVersion:    1,
		ID:               "extract",
		ContractVersion:  1,
		TemplateVersion:  1,
		InputSchema:      "facts",
		OutputSchema:     "closed choice",
		Budget:           domain.Budget{ModelCalls: 1, Tokens: 1000, Attempts: 1},
		MaxOutputTokens:  128,
		SafetyMargin:     3,
		Validators:       []string{"allowed-option"},
		RetryPolicy:      "fail",
		FallbackPolicy:   "fail",
		MaximumAuthority: domain.AuthorityProposeOnly,
	}

	input := prompt.Input{
		Task:               "Extract the date, source, and confidence from the given facts.",
		Constraints:        []string{"Return exactly this format and nothing else. Output ONE key with its proper name and the other two as bare values, exactly like the example."},
		AllowedOutputs:     []string{"Text"},
		AnswerFormat:       "DATE: <date>\nSOURCE: <source>\nCONFIDENCE: <confidence>",
		FormatExample:      "DATE: 2024-05-10\nSyslog\n95%",
		AntiPoisoningGuard: false,
		Facts:              []prompt.Fact{{ID: "F1", Text: "The deployment happened on 2024-05-10. It was recorded in Syslog. We are 95% certain.", Required: true}},
		FormatAnchoring:    prompt.FormatAnchoringAuto,
	}

	result, err := compiler.Compile(spec, input)
	if err != nil {
		fmt.Printf("Compile error: %v\n", err)
		os.Exit(1)
	}

	for _, m := range models {
		prov, _ := openai.New(openai.Config{
			BaseURL: m.baseURL,
			APIKey:  m.key,
			Model:   m.model,
		})

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		start := time.Now()

		req := result.Request
		resp, err := prov.Complete(ctx, req)
		latency := time.Since(start)
		cancel()

		res := map[string]interface{}{
			"model":   m.model,
			"latency": latency.Milliseconds(),
		}

		if err != nil {
			res["error"] = err.Error()
		} else {
			res["text"] = resp.Text

			parseRes := prompt.ParseResponse(resp.Text, []string{"DATE", "SOURCE", "CONFIDENCE"})
			res["parsed_values"] = parseRes.Values
			res["parsed_strategy"] = parseRes.Strategy
			res["parsed_score"] = parseRes.FormatComplianceScore
			res["finish_reason"] = resp.FinishReason
		}

		results = append(results, res)
		time.Sleep(1 * time.Second)
	}

	resultsDir := filepath.Join("/home/node/.openclaw/workspace/motor-autonomo/results", "phase522_hybrid_missing_key_recovery")
	os.MkdirAll(resultsDir, 0755)

	b, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile(filepath.Join(resultsDir, "results.json"), b, 0644)
	fmt.Println("Phase 522 Done.")
}
