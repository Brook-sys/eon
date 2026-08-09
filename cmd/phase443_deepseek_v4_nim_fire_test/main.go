package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"motor-autonomo/internal/provider/nim"
)

func main() {
	apiKey := os.Getenv("NVIDIA_NIM_API_KEY")
	if apiKey == "" {
		log.Fatal("NVIDIA_NIM_API_KEY not set")
	}

	p := nim.New(apiKey, nim.WithTimeout(60*time.Second))

	models := []string{
		"deepseek-ai/deepseek-r1",
	}

	ctx := context.Background()

	fmt.Println("Starting Phase 443 DeepSeek-R1 Base Evaluation on NIM")

	for _, m := range models {
		fmt.Printf("\n--- Model: %s ---\n", m)
		
		start := time.Now()
		req := nim.CompletionRequest{
			Model: m,
			Messages: []nim.Message{
				{Role: "user", Content: "Return strictly the text: STATUS: READY. Do not explain."},
			},
			MaxTokens:   32,
			Temperature: 0.1,
		}

		resp, err := p.Complete(ctx, req)
		lat := time.Since(start)

		if err != nil {
			fmt.Printf("❌ Failed: %v (Lat: %v)\n", err, lat)
			continue
		}

		if len(resp.Choices) > 0 {
			fmt.Printf("✅ Success (Lat: %v)\n", lat)
			fmt.Printf("InTokens: %d, OutTokens: %d, Total: %d\n", resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
			fmt.Printf("Content: %q\n", resp.Choices[0].Message.Content)
		} else {
			fmt.Printf("⚠️ Empty choices (Lat: %v)\n", lat)
		}
	}
}
