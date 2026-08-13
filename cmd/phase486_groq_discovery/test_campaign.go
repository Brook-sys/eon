package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
	"bytes"
)

type Msg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Req struct {
	Model       string `json:"model"`
	Messages    []Msg  `json:"messages"`
	MaxTokens   int    `json:"max_tokens,omitempty"`
	Temperature float64`json:"temperature"`
}

type Resp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func main() {
	key := os.Getenv("GROQ_API_KEY")
	url := "https://api.groq.com/openai/v1/chat/completions"
	
	models := []string{
		"openai/gpt-oss-safeguard-20b",
		"groq/compound",
		"groq/compound-mini",
		"allam-2-7b",
		"canopylabs/orpheus-v1-english",
	}

	prompt := "Respond with exactly the following string and nothing else, without quotes: SYSTEM_ONLINE. Do not add any reasoning."

	for _, m := range models {
		reqData := Req{
			Model: m,
			Messages: []Msg{{Role: "user", Content: prompt}},
			MaxTokens: 10,
			Temperature: 0,
		}
		
		body, _ := json.Marshal(reqData)
		req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")
		
		client := &http.Client{Timeout: 10 * time.Second}
		start := time.Now()
		resp, err := client.Do(req)
		
		if err != nil {
			fmt.Printf("Model %s: ERROR %v\n", m, err)
			continue
		}
		
		var res Resp
		if resp.StatusCode == 200 {
			json.NewDecoder(resp.Body).Decode(&res)
			fmt.Printf("Model %s: %d ms | OK | %s\n", m, time.Since(start).Milliseconds(), res.Choices[0].Message.Content)
		} else {
			var errObj map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&errObj)
			fmt.Printf("Model %s: %d ms | HTTP %d | %v\n", m, time.Since(start).Milliseconds(), resp.StatusCode, errObj)
		}
		resp.Body.Close()
	}
}
