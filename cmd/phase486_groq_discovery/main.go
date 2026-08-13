//go:build ignore

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	key := os.Getenv("GROQ_API_KEY")
	if key == "" {
		log.Fatal("GROQ_API_KEY must be set")
	}

	url := "https://api.groq.com/openai/v1/models"

	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		log.Fatalf("failed to create request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+key)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("unexpected status code: %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Fatalf("failed to decode response: %v", err)
	}

	fmt.Println("Groq Models Available:")
	for _, m := range result.Data {
		fmt.Printf("- %s (owned by %s)\n", m.ID, m.OwnedBy)
	}
}
