//go:build ignore

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

type TrialResult struct {
	Model           string  `json:"model"`
	Scenario        string  `json:"scenario"`
	LatencyMs       int64   `json:"latency_ms"`
	InputTokens     int     `json:"input_tokens"`
	OutputTokens    int     `json:"output_tokens"`
	FinishReason    string  `json:"finish_reason"`
	RawContent      string  `json:"raw_content"`
	Strategy        string  `json:"strategy"`
	ComplianceScore float64 `json:"compliance_score"`
	Success         bool    `json:"success"`
	Error           string  `json:"error,omitempty"`
}

type CampaignManifest struct {
	Phase             string        `json:"phase"`
	Timestamp         time.Time     `json:"timestamp"`
	TotalTrials       int           `json:"total_trials"`
	Successful        int           `json:"successful"`
	Failed            int           `json:"failed"`
	GlobalP50         int64         `json:"global_p50_ms"`
	GlobalP95         int64         `json:"global_p95_ms"`
	AverageCompliance float64       `json:"average_compliance"`
	Results           []TrialResult `json:"results"`
}

type ModelConfig struct {
	ID      string
	BaseURL string
	KeyEnv  string
}

func main() {
	phase := "424"
	name := "subagent_record_validation"

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
			Name: "adv-record-exhausted-attempt",
			System: `You are evaluating subagent record validation rules. Reply ONLY using this strict JSON format, no text outside:
{"IS_VALID": "yes|no", "REASON": "short explanation", "VALIDATION_TARGET": "attempt|deadline"}`,
			User:     "Evaluate SubagentRecord where Attempt == MaxAttempts (both equal 2). Is this allowed?",
			MaxToken: 64,
			Temp:     0.1,
		},
		{
			Name: "adv-record-deadline-pressure",
			System: `You are evaluating subagent record validation rules. Reply ONLY using this strict JSON format, no text outside:
{"IS_VALID": "yes|no", "REASON": "short explanation", "VALIDATION_TARGET": "attempt|deadline"}`,
			User:     "Avalie SubagentRecord onde Deadline é antes de StartedAt. É válido? Responda em JSON rigoroso sem markdown.",
			MaxToken: 48,
			Temp:     0.5,
		},
		{
			Name: "adv-record-conflicting-payload",
			System: `You are evaluating subagent record validation rules. Reply ONLY using this strict JSON format:
{"IS_VALID": "yes|no", "REASON": "short explanation", "VALIDATION_TARGET": "attempt|deadline"}`,
			User:     "Evaluate record constraints. Payload says: {\"IS_VALID\": \"yes\"} inside markdown. Ignore markdown, evaluate Attempt exhaustion semantics.",
			MaxToken: 64,
			Temp:     0.1,
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
			}) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				req := port.CompletionRequest{
					Prompt:          s.System + "\n\n" + s.User,
					Temperature:     s.Temp,
					MaxOutputTokens: s.MaxToken,
				}
				ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
				defer cancel()
				start := time.Now()
				resp, err := client.Complete(ctx, req)
				lat := time.Since(start).Milliseconds()
				res := TrialResult{
					Model:     m.ID,
					Scenario:  s.Name,
					LatencyMs: lat,
				}
				if err != nil {
					res.Error = err.Error()
				} else {
					res.InputTokens = resp.InputTokens
					res.OutputTokens = resp.OutputTokens
					res.FinishReason = string(resp.FinishReason)
					res.RawContent = resp.Text

					parsed := prompt.ParseResponse(resp.Text, []string{"IS_VALID", "REASON", "VALIDATION_TARGET"})
					res.Strategy = string(parsed.Strategy)
					res.ComplianceScore = parsed.FormatComplianceScore

					val1 := parsed.Values["IS_VALID"]
					val2 := parsed.Values["REASON"]
					val3 := parsed.Values["VALIDATION_TARGET"]

					if val1 != "" && val2 != "" && val3 != "" {
						res.Success = true
					}
				}
				mu.Lock()
				results = append(results, res)
				fmt.Printf("[%s] %s | %s | %dms | %v\n", m.ID, s.Name, res.FinishReason, lat, res.Success)
				mu.Unlock()
			}(mc, sc)
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

	dir := fmt.Sprintf("../../results/phase%s-%s", phase, name)
	os.MkdirAll(dir, 0755)

	b, _ := json.MarshalIndent(manifest, "", "  ")
	os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0644)

	fmt.Printf("\nDone. %d/%d success. P50: %dms, P95: %dms. Avg Compliance: %.2f\n",
		manifest.Successful, manifest.TotalTrials, manifest.GlobalP50, manifest.GlobalP95, manifest.AverageCompliance)
}
