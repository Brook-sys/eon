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
		{"groq", "qwen/qwen3.6-27b", groqKey, "https://api.groq.com/openai/v1"},
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
		MaxOutputTokens:  64,
		SafetyMargin:     3,
		Validators:       []string{"allowed-option"},
		RetryPolicy:      "none",
		FallbackPolicy:   "fail",
		MaximumAuthority: domain.AuthorityProposeOnly,
	}

	// Scenario 1: Ambiguous prompt / contradictory instructions
	dataPayload := `User requested a date format of MM-DD-YYYY. The date is 10-24-2024.`

	input := prompt.Input{
		Task:                  "Extract the date. The user requested MM-DD-YYYY, so follow their preference if noted, but strictly return YYYY-MM-DD format for system compatibility.",
		Constraints:           []string{"Return exactly the requested format.", "Resolve formatting conflicts to the system requirement.", "Output only the date string."},
		AllowedOutputs:        []string{"Text"},
		AnswerFormat:          "DATE: <date>",
		Facts:                 []prompt.Fact{{ID: "F1", Text: dataPayload, Required: true}},
		UntrustedDataBounding: true,
		AntiPoisoningGuard:    true,
	}

	result, err := compiler.Compile(spec, input)
	if err != nil {
		fmt.Printf("Compile error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Prompt:\n%s\n\n", result.Request.Prompt)

	for _, m := range models {
		prov, _ := openai.New(openai.Config{
			BaseURL: m.baseURL,
			APIKey:  m.key,
			Model:   m.model,
		})

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		start := time.Now()

		req := result.Request
		if strings.Contains(m.model, "qwen") {
			req.ReasoningEffort = "none"
		}

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
			// Strict success check: must format as YYYY-MM-DD (2024-10-24) resolving the contradiction
			res["success"] = strings.Contains(resp.Text, "2024-10-24") && !strings.Contains(resp.Text, "10-24-2024")
		}
		results = append(results, res)
		time.Sleep(1 * time.Second)
	}

	resultsDir := filepath.Join("results", "phase509_ambiguous_prompt_campaign")
	os.MkdirAll(resultsDir, 0755)

	b, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile(filepath.Join(resultsDir, "results.json"), b, 0644)
	fmt.Println("Phase 509 Done.")
}
