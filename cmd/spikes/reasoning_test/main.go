//go:build ignore

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/provider/openai"
)

func main() {
	apiKey := os.Getenv("GROQ_API_KEY")
	if apiKey == "" {
		fmt.Println("Error: GROQ_API_KEY environment variable is missing")
		os.Exit(1)
	}

	config := openai.Config{
		BaseURL:          "https://api.groq.com/openai/v1",
		APIKey:           apiKey,
		Model:            "qwen/qwen3.6-27b",
		MaxOutputField:   openai.MaxOutputTokensLegacy,
		MaxResponseBytes: 1 << 20,
		Client:           &http.Client{Timeout: 30 * time.Second},
	}

	provider, err := openai.New(config)
	if err != nil {
		fmt.Printf("Error initializing provider: %v\n", err)
		os.Exit(1)
	}

	req := port.CompletionRequest{
		Prompt:          "Explain the concept of entropy in 10 words or less.",
		MaxOutputTokens: 30,
		Temperature:     0.6,
		ReasoningEffort: "default",
		ReasoningFormat: "hidden",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	fmt.Println("Running CompletionRequest with reasoning_format=hidden and budget 30...")
	result, err := provider.Complete(ctx, req)
	if err != nil {
		fmt.Printf("Provider returned error: %v\n", err)
		if de, ok := err.(port.ProviderDiagnosticError); ok {
			fmt.Printf("DiagnosticReason: %s\n", de.DiagnosticReason())
			if de.DiagnosticReason() == "reasoning_budget_exhausted" {
				fmt.Println("SUCCESS: Correctly classified reasoning budget exhaustion!")
			}
		}
		os.Exit(1) // Still an error at the provider boundary as intended
	}

	resJSON, _ := json.MarshalIndent(result, "", "  ")
	fmt.Printf("Provider CompletionResult:\n%s\n", resJSON)
}
