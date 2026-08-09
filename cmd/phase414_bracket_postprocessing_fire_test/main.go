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
	key := os.Getenv("GROQ_API_KEY")
	if key == "" {
		fmt.Println("GROQ_API_KEY required")
		os.Exit(1)
	}
	
	model := "llama-3.1-8b-instant"
	
	client, err := openai.New(openai.Config{
		BaseURL: "https://api.groq.com/openai/v1",
		APIKey:  key,
		Model:   model,
	})
	if err != nil {
		fmt.Printf("Failed to init provider: %v\n", err)
		os.Exit(1)
	}
	
	systemPrompt := `You are a strict data classifier.
You must output EXACTLY the following keys in this format:
ID: <id>
TOOLS: [<tool1>, <tool2>, ...]
Do not add any other text.`

	userPrompt := `ID: 5543
TOOLS: Screwdriver, Hammer, Drill, Saw, Tape Measure, Wrench`

	fmt.Printf("Executing 1 trial on %s with aggressive truncation (max_tokens=15)...\n", model)

	req := port.CompletionRequest{
		Prompt:          systemPrompt + "\n\n" + userPrompt,
		Temperature:     0.5,
		MaxOutputTokens: 15,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	resp, err := client.Complete(ctx, req)
	if err != nil {
		fmt.Printf("Provider error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Raw model output:\n---\n%s\n---\n", resp.Text)

	parsed := prompt.ParseResponse(resp.Text, []string{"ID", "TOOLS"})
	fmt.Printf("Parsed ID: %q\n", parsed.Values["ID"])
	fmt.Printf("Parsed TOOLS: %q\n", parsed.Values["TOOLS"])
	
	if parsed.Values["TOOLS"] == "" || parsed.Values["TOOLS"][0] == '[' {
		fmt.Printf("FAIL: Failed to strip brackets on truncated value: %q\n", parsed.Values["TOOLS"])
		os.Exit(1)
	}
	fmt.Println("SUCCESS: Bracket post-processing correctly sanitized the truncated output.")
}
