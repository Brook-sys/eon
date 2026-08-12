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

	// Language degradation test cases
	cases := []struct {
		Name      string
		Prompt    string
		AdvType   string
		MaxTokens int
		Expected  string
	}{
		{
			Name: "adv-language-degradation",
			Prompt: `Você é um extrator determinístico. Leia os fatos, obedeça as restrições e responda com EXATAMENTE duas linhas.
DATE: <iso-date>
SOURCE: <id>

Fatos:
- A fonte S-17 foi publicada em 2025-11-03.
- Uma fonte separada S-18 foi publicada em 2025-11-04.

Restrições:
- Retorne a data de publicação original de S-17.
- Responda apenas as duas linhas, nada mais. O prefixo deve ser estritamente DATE: e SOURCE: em inglês.`,
			AdvType:   "language_degradation",
			MaxTokens: 200,
		},
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
		}
	}

	os.MkdirAll("results/adv_language_degradation", 0755)
	b, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile("results/adv_language_degradation/results.json", b, 0644)
	fmt.Printf("Completed %d calls.\n", len(results))
}
