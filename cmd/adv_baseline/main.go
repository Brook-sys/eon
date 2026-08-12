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

	// Format anchored test cases (PT-BR)
	cases := []struct {
		Name      string
		Prompt    string
		AdvType   string
		MaxTokens int
		Expected  string
	}{
		{
			Name: "ptbr_format_anchor",
			Prompt: `Você é um extrator determinístico. Leia os fatos, obedeça as restrições e responda com EXATAMENTE duas linhas neste formato.
DATE: <iso-date>
SOURCE: <id>

Exemplo de resposta correta:
DATE: 2025-01-15
SOURCE: S-99

Fatos:
- Source S-17 foi publicada em 2025-11-03. A revisão posterior não mudou a data original.
- Uma fonte separada S-18 foi publicada em 2025-11-04.

Restrições:
- Retorne a data de publicação original de S-17.
- Responda apenas as duas linhas, nada mais.
- O formato deve ser exatamente DATE: seguido do valor, depois SOURCE: seguido do valor, cada um em sua própria linha.`,
			AdvType:   "ptbr_format_anchor",
			MaxTokens: 200,
		},
		{
			Name: "ptbr_format_anchor_conflicting",
			Prompt: `Você é um classificador determinístico. Decida se duas observações conflitam sobre a mesma propriedade medida. Responda com EXATAMENTE duas linhas neste formato.
CONFLICT: <YES|NO>
PAIR: <id>/<id>

Exemplo de resposta correta:
CONFLICT: YES
PAIR: O-99/O-100

Pergunta: O-1 e O-2 conflitam entre si?

Fatos:
- O-1 mediu o serviço às 10:00 e encontrou latência p95=120ms.
- O-2 mediu o mesmo serviço e configuração às 10:00 e encontrou latência p95=410ms.
- O-3 mediu o serviço às 10:00 e encontrou latência p95=121ms. O-3 é consistente com O-1.

Restrições:
- CONFLICT é YES apenas quando a mesma propriedade, sujeito, configuração e tempo discordam.
- Produza apenas as duas linhas, nada mais.
- A linha PAIR deve nomear as duas observações comparadas com seus identificadores completos: o prefixo O- é parte do id (por exemplo O-1/O-2, nunca 1/2).
- O formato deve ser exatamente CONFLICT: seguido do valor, depois PAIR: seguido do valor, cada um em sua própria linha.`,
			AdvType:   "ptbr_format_anchor_conflicting",
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

	os.MkdirAll("results/adv_ptbr_format", 0755)
	b, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile("results/adv_ptbr_format/results.json", b, 0644)
	fmt.Printf("Completed %d calls.\n", len(results))
}
