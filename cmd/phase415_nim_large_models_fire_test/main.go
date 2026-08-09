package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/provider/openai"
)

func main() {
	nimKey := os.Getenv("NVIDIA_NIM_API_KEY")
	groqKey := os.Getenv("GROQ_API_KEY")
	
	if nimKey == "" || groqKey == "" {
		log.Fatal("NVIDIA_NIM_API_KEY and GROQ_API_KEY must be set for this cross-provider test")
	}

	models := []struct{
		BaseURL string
		APIKey  string
		ModelID string
	}{
		{"https://integrate.api.nvidia.com/v1", nimKey, "meta/llama-3.1-8b-instruct"},
		{"https://integrate.api.nvidia.com/v1", nimKey, "meta/llama-3.1-70b-instruct"}, // Correct endpoint
		{"https://integrate.api.nvidia.com/v1", nimKey, "nvidia/nemotron-4-340b-instruct"}, // Trying again
		{"https://api.groq.com/openai/v1", groqKey, "llama-3.3-70b-versatile"},
	}

	req := port.CompletionRequest{
		Prompt: `Extract the core entities from the text.
Text: The Falcon Heavy rocket launched from Cape Canaveral at 15:30 EST carrying the GSAT-24 satellite.

Respond with the exact keys in this format:
VEHICLE: <value>
LOCATION: <value>
PAYLOAD: <value>`,
        MaxOutputTokens: 500,
        Temperature: 0.1,
	}
	
	fmt.Println("=== Phase 415: NIM Large Models Cross-Provider Fire Test ===")
	
	ctx := context.Background()
	keys := []string{"VEHICLE", "LOCATION", "PAYLOAD"}

	for _, m := range models {
		fmt.Printf("\nTesting model: %s\n", m.ModelID)
		
		adapter, err := openai.New(openai.Config{
		    BaseURL: m.BaseURL,
		    APIKey: m.APIKey,
		    Model: m.ModelID,
		})
		
		if err != nil {
		    fmt.Println(err)
		    continue
		}

		start := time.Now()
		resp, err := adapter.Complete(ctx, req)
		lat := time.Since(start)

		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
			continue
		}

		fmt.Printf("Latency: %v | Usage: In %d / Out %d | Reason: %s\n", lat, resp.InputTokens, resp.OutputTokens, string(resp.FinishReason))
		
		parsed := prompt.ParseResponse(resp.Text, keys)
		fmt.Printf("Format compliance: %.2f\n", parsed.FormatComplianceScore)
		fmt.Printf("Parsed values: %v\n", parsed.Values)
	}
}
