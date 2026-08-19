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
	groqKey := os.Getenv("GROQ_API_KEY")
	if groqKey == "" {
		fmt.Println("GROQ_API_KEY not set")
		os.Exit(1)
	}

	p, err := openai.New(openai.Config{
		BaseURL: "https://api.groq.com/openai/v1",
		APIKey:  groqKey,
		Model:   "qwen/qwen3.6-27b",
		Semaphore: &openai.SemaphoreConfig{
			MaxConcurrent:  3,
			AcquireTimeout: 10 * time.Second,
		},
		RateLimiter: &openai.RateLimiterConfig{
			RequestsPerMinute: 71,
			TokensPerMinute:   71000,
			InitialBurst:      71 / 6,
			AcquireTimeout:    15 * time.Second,
		},
	})
	if err != nil {
		fmt.Printf("New error: %v\n", err)
		os.Exit(1)
	}

	req := port.CompletionRequest{
		SystemPrompt:    "You are a test system.",
		Prompt:          "Reply with exactly: READY",
		MaxOutputTokens: 16,
		Temperature:     0,
		ReasoningEffort: "none",
		Timeout:         15 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	result, err := p.Complete(ctx, req)
	if err != nil {
		fmt.Printf("Complete error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("SUCCESS: %s\n", result.Text)
}
