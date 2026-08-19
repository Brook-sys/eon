package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/provider/openai"
)

func main() {
	apiKey := os.Getenv("GROQ_API_KEY")
	if apiKey == "" {
		fmt.Println("GROQ_API_KEY not set")
		os.Exit(1)
	}

	p, err := openai.New(openai.Config{
		BaseURL: "https://api.groq.com/openai/v1",
		APIKey:  apiKey,
		Model:   "qwen/qwen3.6-27b",
		Semaphore: &openai.SemaphoreConfig{
			MaxConcurrent:  3,
			AcquireTimeout: 10 * time.Second,
		},
		RateLimiter: &openai.RateLimiterConfig{
			RequestsPerMinute: 71,
			TokensPerMinute:   71000,
			InitialBurst:      12,
			AcquireTimeout:    15 * time.Second,
		},
	})
	if err != nil {
		fmt.Printf("New error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := p.Complete(ctx, port.CompletionRequest{
		SystemPrompt:    "Extract date. Format: DATE: <YYYY-MM-DD>",
		Prompt:          "Launch on 2024-11-15",
		MaxOutputTokens: 64,
		Temperature:     0.0,
		ReasoningEffort: "none",
		Timeout:         30 * time.Second,
	})
	if err != nil {
		fmt.Printf("Complete error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Result: %s\n", result.Text)
	fmt.Printf("Tokens: %d/%d\n", result.InputTokens, result.OutputTokens)
}