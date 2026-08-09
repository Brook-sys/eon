package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/provider/openai"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Println("Usage: run <base_url> <model>")
		os.Exit(1)
	}
	baseURL := os.Args[1]
	model := os.Args[2]
	apiKey := os.Getenv("GROQ_API_KEY")

	client, err := openai.New(openai.Config{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   model,
	})
	if err != nil {
		fmt.Printf("Failed to init provider: %v\n", err)
		os.Exit(1)
	}
	
	// Create an adversarial prompt meant to elicit a list output
	userPrompt := "Provide exactly a list of 10 tools named TOOLS: [tool1, tool2] without prose. List the 10 tools used to tighten a bolt."
	
	// Deliberately starve the model of tokens to force truncation mid-list
	req := port.CompletionRequest{
		Prompt:          userPrompt,
		Temperature:     0.1,
		MaxOutputTokens: 10, // Extreme budget starvation
	}

	start := time.Now()
	resp, err := client.Complete(context.Background(), req)
	elapsed := time.Since(start)

	if err != nil {
		fmt.Printf("Request failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Completed in %v. Reason: %v. Usage: %d input, %d output\n", elapsed, resp.FinishReason, resp.InputTokens, resp.OutputTokens)
	fmt.Printf("Raw Content:\n%s\n---\n", resp.Text)
	
	parsed := prompt.ParseResponse(resp.Text, []string{"TOOLS"})
	fmt.Printf("Parsed TOOLS: %q\n", parsed.Values["TOOLS"])
}
