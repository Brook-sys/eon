package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/provider/openai"
)

type ModelConfig struct {
	ID      string
	BaseURL string
	KeyEnv  string
}

type TrialResult struct {
	Model           string  `json:"model"`
	Scenario        string  `json:"scenario"`
	Success         bool    `json:"success"`
	LatencyMs       int64   `json:"latency_ms"`
	InputTokens     int     `json:"input_tokens"`
	OutputTokens    int     `json:"output_tokens"`
	FinishReason    string  `json:"finish_reason"`
	RawContent      string  `json:"raw_content"`
	Error           string  `json:"error,omitempty"`
	Strategy        string  `json:"parsing_strategy"`
	ComplianceScore float64 `json:"compliance_score"`
}

type CampaignManifest struct {
	Phase             string        `json:"phase"`
	Timestamp         time.Time     `json:"timestamp"`
	TotalTrials       int           `json:"total_trials"`
	Successful        int           `json:"successful"`
	Failed            int           `json:"failed"`
	AverageCompliance float64       `json:"average_compliance_score"`
	GlobalP50         int64         `json:"global_p50_ms"`
	GlobalP95         int64         `json:"global_p95_ms"`
	Results           []TrialResult `json:"results"`
}

func main() {
	phase := "432"
	name := "runtime_vault_resolver_precedence"
	fmt.Printf("Starting Phase %s Live Fire Campaign: %s\n", phase, name)

	models := []ModelConfig{
		{"llama-3.1-8b-instant", "https://api.groq.com/openai/v1", "GROQ_API_KEY"},
		{"llama-3.3-70b-versatile", "https://api.groq.com/openai/v1", "GROQ_API_KEY"},
		{"meta/llama-3.1-8b-instruct", "https://integrate.api.nvidia.com/v1", "NVIDIA_NIM_API_KEY"},
	}

	scenarios := []struct {
		Name     string
		System   string
		User     string
		MaxToken int
		Temp     float64
	}{
		{
			Name:     "adv-resolver-env-precedence-structural",
			System:   "You evaluate runtime secret resolution. The system must prefer environment variables over vault records. If ENV_KEY=foo and vault=bar, the returned secret is foo. Does the resolver prefer ENV? Reply with PREFERS_ENV, FALLBACKS_TO_VAULT, REASON.",
			User:     "ENV: KEY=123, Vault: KEY=456",
			MaxToken: 64,
			Temp:     0.0,
		},
		{
			Name:     "adv-resolver-env-precedence-ptbr",
			System:   "Você avalia a resolução de segredos. O sistema prefere variáveis de ambiente sobre o cofre. Se a chave estiver no ambiente, ela ignora o cofre. O sistema prioriza ambiente? Responda usando as chaves PREFERS_ENV, FALLBACKS_TO_VAULT, REASON.",
			User:     "Ambiente tem precedência.",
			MaxToken: 48,
			Temp:     0.0,
		},
		{
			Name:     "adv-resolver-vault-only-structural",
			System:   "You evaluate runtime secret resolution. The system must prefer environment variables over vault records. If ENV is empty, it uses the vault. If ENV is empty, does it fail without checking the vault? Reply with FAILS_IF_ENV_EMPTY, REASON.",
			User:     "ENV is empty.",
			MaxToken: 64,
			Temp:     0.0,
		},
	}

	var results []TrialResult
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 3)

	for _, mc := range models {
		apiKey := os.Getenv(mc.KeyEnv)
		if apiKey == "" {
			fmt.Printf("Skipping %s (missing %s)\n", mc.ID, mc.KeyEnv)
			continue
		}

		client, err := openai.New(openai.Config{
			BaseURL: mc.BaseURL,
			APIKey:  apiKey,
			Model:   mc.ID,
		})
		if err != nil {
			fmt.Printf("Init err %s: %v\n", mc.ID, err)
			continue
		}

		for _, sc := range scenarios {
			wg.Add(1)
			go func(m ModelConfig, s struct {
				Name, System, User string
				MaxToken           int
				Temp               float64
			}) { defer wg.Done(); sem <- struct{}{}; defer func() { <-sem }(); req := port.CompletionRequest{
				Prompt:          s.System + "\n\n" + s.User,
				Temperature:     s.Temp,
				MaxOutputTokens: s.MaxToken,
			}; ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second); defer cancel(); start := time.Now(); resp, err := client.Complete(ctx, req); lat := time.Since(start).Milliseconds(); res := TrialResult{
				Model:     m.ID,
				Scenario:  s.Name,
				LatencyMs: lat,
			}; if err != nil {
				res.Error = err.Error()
			} else {
				res.InputTokens = resp.InputTokens
				res.OutputTokens = resp.OutputTokens
				res.FinishReason = string(resp.FinishReason)
				res.RawContent = resp.Text

				parsed := prompt.ParseResponse(resp.Text, []string{"PREFERS_ENV", "FALLBACKS_TO_VAULT", "FAILS_IF_ENV_EMPTY", "REASON"})
				res.Strategy = string(parsed.Strategy)
				res.ComplianceScore = parsed.FormatComplianceScore

				valEnv := parsed.Values["PREFERS_ENV"]
				valVault := parsed.Values["FALLBACKS_TO_VAULT"]
				valFails := parsed.Values["FAILS_IF_ENV_EMPTY"]

				if s.Name == "adv-resolver-vault-only-structural" {
					if valFails == "no" || valFails == "false" {
						res.Success = true
					}
				} else {
					if (valEnv == "yes" || valEnv == "true") && (valVault == "yes" || valVault == "true") {
						res.Success = true
					}
				}
			}; mu.Lock(); results = append(results, res); fmt.Printf("[%s] %s | %s | %dms | %v\n", m.ID, s.Name, res.FinishReason, lat, res.Success); mu.Unlock() }(mc, sc)
		}
	}

	wg.Wait()

	manifest := CampaignManifest{
		Phase:       phase,
		Timestamp:   time.Now().UTC(),
		TotalTrials: len(results),
		Results:     results,
	}

	var lats []int64
	var sumComp float64
	for _, r := range results {
		if r.Success {
			manifest.Successful++
		} else {
			manifest.Failed++
		}
		if r.LatencyMs > 0 {
			lats = append(lats, r.LatencyMs)
		}
		sumComp += r.ComplianceScore
	}

	if len(results) > 0 {
		manifest.AverageCompliance = sumComp / float64(len(results))
	}
	if len(lats) > 0 {
		manifest.GlobalP50 = lats[len(lats)/2]
		manifest.GlobalP95 = lats[int(float64(len(lats))*0.95)]
	}

	dir := fmt.Sprintf("results/phase%s-%s", phase, name)
	os.MkdirAll(dir, 0755)

	b, _ := json.MarshalIndent(manifest, "", "  ")
	os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0644)

	fmt.Printf("\nDone. %d/%d success. P50: %dms, P95: %dms. Avg Compliance: %.2f\n",
		manifest.Successful, manifest.TotalTrials, manifest.GlobalP50, manifest.GlobalP95, manifest.AverageCompliance)
}
