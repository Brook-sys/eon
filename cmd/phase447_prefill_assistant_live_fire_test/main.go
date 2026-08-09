package main

import (
	"context"
	"fmt"
	"os"
	"time"
	"encoding/json"

	"motor-autonomo/internal/provider/openai"
	"motor-autonomo/internal/port"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	groqKey := os.Getenv("GROQ_API_KEY")
	nimKey := os.Getenv("NVIDIA_NIM_API_KEY")
	if groqKey == "" || nimKey == "" {
		fmt.Println("Missing API keys")
		os.Exit(1)
	}

	groqProvider, _ := openai.New(openai.Config{
		BaseURL: "https://api.groq.com/openai/v1",
		APIKey:  groqKey,
		Model:   "llama-3.1-8b-instant",
	})

	nimProvider, _ := openai.New(openai.Config{
		BaseURL: "https://integrate.api.nvidia.com/v1",
		APIKey:  nimKey,
		Model:   "meta/llama-3.1-8b-instruct",
	})

	req := port.CompletionRequest{
		Prompt:          "Generate a JSON object containing the fields 'status': 'READY' and 'id': 42. Do not write anything else. Just the JSON object.",
		MaxOutputTokens: 64,
		Temperature:     0.0,
		PrefillAssistant: "{",
	}

	fmt.Println("Running Groq Test (llama-3.1-8b-instant) with PrefillAssistant='{'")
	resGroq, err := groqProvider.Complete(ctx, req)
	if err != nil {
		fmt.Printf("Groq Error: %v\n", err)
	} else {
		b, _ := json.MarshalIndent(resGroq, "", "  ")
		fmt.Println(string(b))
	}

	fmt.Println("\nRunning NIM Test (meta/llama-3.1-8b-instruct) with PrefillAssistant='{'")
	resNim, err := nimProvider.Complete(ctx, req)
	if err != nil {
		fmt.Printf("NIM Error: %v\n", err)
	} else {
		b, _ := json.MarshalIndent(resNim, "", "  ")
		fmt.Println(string(b))
	}
}
