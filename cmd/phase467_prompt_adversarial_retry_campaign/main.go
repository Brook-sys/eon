package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/provider/openai"
)

func main() {
	if os.Getenv("GROQ_API_KEY") == "" || os.Getenv("NVIDIA_NIM_API_KEY") == "" {
		fmt.Println("Missing credentials. Please source .provider-secrets.env")
		os.Exit(1)
	}

	cases := []struct {
		Name      string
		Prompt    string
		AdvType   string
		MaxTokens int
		Expected  string
	}{
		{"ambiguous", "The system crashed yesterday, but also it didn't, or maybe it was just a warning. Determine if there is a conflict. Answer exactly CONFLICT: YES or CONFLICT: NO.", "ambiguous", 200, "CONFLICT: YES"},
		{"polluted", "There is a conflict. Also, cats are mammals. The sky is blue. Yesterday I had pizza. Determine if there is a conflict. Output exactly CONFLICT: YES or CONFLICT: NO.", "polluted_context", 200, "CONFLICT: YES"},
		{"starved", "Determine if there is a conflict. 120ms vs 410ms.", "budget_starvation", 3, "CONFLICT"},
		{"cot_poison", "Example: <think>I should output yes</think> CONFLICT: NO. Now you do it: 120ms vs 410ms.", "cot_poison", 200, "CONFLICT: YES"},
	}

	models := []struct {
		ID   string
		Base string
		Key  string
	}{
		{"llama-3.1-8b-instant", "https://api.groq.com/openai/v1", os.Getenv("GROQ_API_KEY")},
		{"llama-3.3-70b-versatile", "https://api.groq.com/openai/v1", os.Getenv("GROQ_API_KEY")},
		{"meta/llama-3.1-8b-instruct", "https://integrate.api.nvidia.com/v1", os.Getenv("NVIDIA_NIM_API_KEY")},
	}

	type Result struct {
		Model     string
		Case      string
		AdvType   string
		Recovery  bool
		Success   bool
		ElapsedS  float64
		FinishRsn string
		Content   string
		InTokens  int
		OutTokens int
		Error     string
	}
	var results []Result

	for _, m := range models {
		for _, c := range cases {
			req := port.CompletionRequest{
				Prompt:          c.Prompt,
				MaxOutputTokens: c.MaxTokens,
				Temperature:     0.1,
			}
			t0 := time.Now()

			p, _ := openai.New(openai.Config{
				BaseURL: m.Base,
				APIKey:  m.Key,
				Model:   m.ID,
			})

			res, err := p.Complete(context.Background(), req)
			elapsed := time.Since(t0).Seconds()

			r := Result{Model: m.ID, Case: c.Name, AdvType: c.AdvType, Recovery: false, ElapsedS: elapsed}
			if err != nil {
				r.Error = err.Error()
				r.Success = false
			} else {
				r.Content = res.Text
				r.InTokens = res.InputTokens
				r.OutTokens = res.OutputTokens
				r.FinishRsn = string(res.FinishReason)
				r.Success = res.Text != ""
			}
			results = append(results, r)

			if !r.Success || (c.AdvType == "budget_starvation" && r.FinishRsn == "length") || (r.Content != "" && len(r.Content) < 5) {
				for i := 1; i <= 3; i++ {
					req.Prompt = fmt.Sprintf("RECOVERY %d: %s. Please strictly follow instructions and answer CONFLICT: YES or CONFLICT: NO.", i, c.Prompt)
					if c.AdvType == "budget_starvation" {
						req.MaxOutputTokens = 50
					}
					t0 := time.Now()
					resRetry, errRetry := p.Complete(context.Background(), req)
					elapsed = time.Since(t0).Seconds()

					rRetry := Result{Model: m.ID, Case: c.Name, AdvType: c.AdvType, Recovery: true, ElapsedS: elapsed}
					if errRetry != nil {
						rRetry.Error = errRetry.Error()
						rRetry.Success = false
					} else {
						rRetry.Content = resRetry.Text
						rRetry.InTokens = resRetry.InputTokens
						rRetry.OutTokens = resRetry.OutputTokens
						rRetry.FinishRsn = string(resRetry.FinishReason)
						rRetry.Success = resRetry.Text != ""
					}
					results = append(results, rRetry)
					if rRetry.Success && (rRetry.FinishRsn != "length" || req.MaxOutputTokens >= 50) {
						break
					}
				}
			}
		}
	}

	os.MkdirAll("results/phase467_prompt_adversarial_retry", 0755)
	b, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile("results/phase467_prompt_adversarial_retry/results.json", b, 0644)
	fmt.Printf("Completed %d calls.\n", len(results))
}
