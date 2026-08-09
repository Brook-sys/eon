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
	Model             string  `json:"model"`
	Scenario          string  `json:"scenario"`
	LatencyMs         int64   `json:"latency_ms"`
	InputTokens       int     `json:"input_tokens"`
	OutputTokens      int     `json:"output_tokens"`
	FinishReason      string  `json:"finish_reason"`
	RawContent        string  `json:"raw_content"`
	Strategy          string  `json:"strategy"`
	ComplianceScore   float64 `json:"compliance_score"`
	Success           bool    `json:"success"`
	Error             string  `json:"error,omitempty"`
	ExtractedGoal     string  `json:"extracted_goal"`
	ExtractedTasks    string  `json:"extracted_tasks"`
	ExtractedBlockers string  `json:"extracted_blockers"`
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
	phase := "416"
	name := "stacked_adversarial_parsing"

	fmt.Printf("Starting Phase %s Live Fire Campaign: %s\n", phase, name)

	models := []ModelConfig{
		{"llama-3.1-8b-instant", "https://api.groq.com/openai/v1", "GROQ_API_KEY"},
		{"llama-3.3-70b-versatile", "https://api.groq.com/openai/v1", "GROQ_API_KEY"},
		{"qwen-2.5-32b", "https://api.groq.com/openai/v1", "GROQ_API_KEY"}, // 400 on previous run, will check
		{"openai/gpt-oss-120b", "https://api.groq.com/openai/v1", "GROQ_API_KEY"},
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
			Name: "stacked_chaos",
			System: `You are an automated planner. Reply ONLY using this format:
GOAL => <main objective>
TASKS: [
  <task 1>,
  <task 2>
]
<BLOCKERS>: <blockers>

Do NOT output Markdown.`,
			User:     "Goal: Fix tests. Tasks: update parse, add test. Blockers: time.",
			MaxToken: 64, // Sufficient budget
			Temp:     0.7,
		},
		{
			Name: "stacked_chaos_starved",
			System: `You are an automated planner. Reply ONLY using this format:
GOAL => <main objective>
TASKS: [
  <task 1>,
  <task 2>
]
<BLOCKERS>: <blockers>

Do NOT output Markdown.`,
			User:     "Goal: Optimize latency. Tasks: connection pool, async writes. Blockers: schema.",
			MaxToken: 15, // Starve it mid-tasks
			Temp:     0.2,
		},
		{
			Name: "json_chaos_fallback",
			System: `You are an automated planner. Reply using this JSON schema:
{"GOAL": "string", "TASKS": ["task1"], "BLOCKERS": "string"}`,
			User:     "Goal: Release. Tasks: tag, push, deploy. Blockers: none.",
			MaxToken: 64,
			Temp:     0.5,
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

					parsed := prompt.ParseResponse(resp.Text, []string{"GOAL", "TASKS", "BLOCKERS"})
					res.Strategy = string(parsed.Strategy)
					res.ComplianceScore = parsed.FormatComplianceScore
					res.ExtractedGoal = parsed.Values["GOAL"]
					res.ExtractedTasks = parsed.Values["TASKS"]
					res.ExtractedBlockers = parsed.Values["BLOCKERS"]

					// Success criteria: extracted at least Goal and Tasks (blockers might truncate)
					if res.ExtractedGoal != "" && res.ExtractedTasks != "" {
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
