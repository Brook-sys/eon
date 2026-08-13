//go:build ignore

// Command nim_fire_test runs a bounded live campaign testing the FormatExample
// prompt field against NVIDIA NIM meta/llama-3.1-8b-instruct, which showed
// different failure modes than Groq 8B in Phase 383 (polarity inversion on
// CONFLICT tasks, not just format dropping).
//
// Hypothesis: FormatExample improves format compliance on NIM 8B for the
// adv-language-degradation scenario. Additionally, test whether FormatExample
// helps or hurts the CONFLICT polarity inversion failure mode identified
// in Phase 383 cross-provider comparison.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type nimTrial struct {
	Model       string  `json:"model"`
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"max_tokens"`
	Prompt      string  `json:"prompt"`
	HasExample  bool    `json:"has_example"`
	Scenario    string  `json:"scenario"`
}

type nimResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type nimResult struct {
	Model           string  `json:"model"`
	Trial           int     `json:"trial"`
	HasExample      bool    `json:"has_example"`
	Scenario        string  `json:"scenario"`
	PromptTokens    int     `json:"prompt_tokens"`
	OutputTokens    int     `json:"output_tokens"`
	FinishReason    string  `json:"finish_reason"`
	LatencyMs       float64 `json:"latency_ms"`
	HTTPStatus      int     `json:"http_status"`
	ResponseText    string  `json:"response_text"`
	FormatCorrect   bool    `json:"format_correct"`
	SemanticCorrect bool    `json:"semantic_correct"`
	Error           string  `json:"error,omitempty"`
}

const (
	nimBaseURL = "https://integrate.api.nvidia.com/v1/chat/completions"
	nimTimeout = 60 * time.Second
	nimTrials  = 5
)

// Scenario 1: adv-language-degradation (PT-BR mixed context)
func buildNimLangDegradationPrompt(withExample bool) string {
	task := `Extraia a data de publicação e o identificador da fonte do contexto abaixo. Return ONLY the date and source in the EXACT format specified. No explanation, no markdown.

CONTEXTO: O relatório trimestral Q3-2025 foi submetido em 2025-11-03 pela analista S-17. O documento cobre o desempenho fiscal do período ending September 30. Revenue grew 12% year-over-year. A equipe de auditoria flagged three discrepancies in expense reporting. Source ID: S-17. Publication date: November 3, 2025. O relatório foi aprovado pelo conselho.

TASK: Extract DATE and SOURCE from the context above.
REQUIRED FORMAT:
DATE: <YYYY-MM-DD>
SOURCE: <identifier>`

	if withExample {
		task += "\n\nEXAMPLE\nDATE: 2025-11-03\nSOURCE: S-17"
	}
	return task
}

// Scenario 2: adv-conflicting-data (CONFLICT detection)
// In Phase 383, NIM 8B showed polarity inversion (3/5 CONFLICT: NO instead of
// CONFLICT: YES) on conflicting data. Tests if FormatExample helps the model
// correctly identify conflicts.
func buildNimConflictPrompt(withExample bool) string {
	task := `Analyze the following observations for conflicts. The latency measurements were taken in the same time window.

OBSERVATION A: The API response latency was measured at 120ms during peak hours (14:00-15:00 UTC).
OBSERVATION B: The API response latency was measured at 410ms during peak hours (14:00-15:00 UTC).

Both measurements used the same methodology and instrument calibration. The measurements were taken within the same 5-minute window.

TASK: Determine if there is a conflict between the observations.
REQUIRED FORMAT:
CONFLICT: <YES or NO>
REASON: <one sentence explanation>`

	if withExample {
		task += "\n\nEXAMPLE\nCONFLICT: YES\nREASON: Two different latency values (120ms vs 410ms) were measured in the same time window."
	}
	return task
}

// Scenario 3: adv-format-pressure (max_tokens=48, temp=0.7)
func buildNimFormatPressurePrompt(withExample bool) string {
	task := `Extract the publication date and source identifier from the following context. Return ONLY the date and source in the exact format specified. No explanation, no markdown.

CONTEXT: The quarterly report Q3-2025 was submitted on 2025-11-03 by analyst S-17. The document covers fiscal performance for the period ending September 30. Revenue grew 12% year-over-year. The audit team flagged three discrepancies in expense reporting. Source ID: S-17. Publication date: November 3, 2025.

TASK: Extract DATE and SOURCE from the context above.
REQUIRED FORMAT:
DATE: <YYYY-MM-DD>
SOURCE: <identifier>`

	if withExample {
		task += "\n\nEXAMPLE\nDATE: 2025-11-03\nSOURCE: S-17"
	}
	return task
}

func checkNimFormat(text string, scenario string) bool {
	if scenario == "adv-conflicting-data" {
		return strings.Contains(text, "CONFLICT:") && strings.Contains(text, "REASON:")
	}
	return strings.Contains(text, "DATE:") && strings.Contains(text, "SOURCE:")
}

func checkNimSemantic(text string, scenario string) bool {
	if scenario == "adv-conflicting-data" {
		return strings.Contains(strings.ToUpper(text), "CONFLICT: YES")
	}
	return strings.Contains(text, "2025-11-03") && strings.Contains(text, "S-17")
}

func runNimTrial(ctx context.Context, apiKey string, tr nimTrial, trialNum int) nimResult {
	reqBody := map[string]any{
		"model":       tr.Model,
		"temperature": tr.Temperature,
		"max_tokens":  tr.MaxTokens,
		"messages": []map[string]string{
			{"role": "user", "content": tr.Prompt},
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, "POST", nimBaseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nimResult{Model: tr.Model, Trial: trialNum, HasExample: tr.HasExample, Scenario: tr.Scenario, Error: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", "motor-autonomo/1.0")

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	latency := time.Since(start).Seconds() * 1000
	if err != nil {
		return nimResult{Model: tr.Model, Trial: trialNum, HasExample: tr.HasExample, Scenario: tr.Scenario, Error: err.Error(), LatencyMs: latency}
	}
	defer resp.Body.Close()

	r := nimResult{
		Model:      tr.Model,
		Trial:      trialNum,
		HasExample: tr.HasExample,
		Scenario:   tr.Scenario,
		HTTPStatus: resp.StatusCode,
		LatencyMs:  latency,
	}

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		r.Error = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body))
		return r
	}

	// NIM may emit SSE or JSON depending on stream flag; default is JSON
	var cr nimResp
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		// Try reading as text (some NIM deployments return non-JSON)
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		r.Error = fmt.Sprintf("decode error: %v, body start: %s", err, string(body[:min(len(body), 200)]))
		return r
	}

	if len(cr.Choices) > 0 {
		r.ResponseText = cr.Choices[0].Message.Content
		r.FinishReason = cr.Choices[0].FinishReason
	}
	r.PromptTokens = cr.Usage.PromptTokens
	r.OutputTokens = cr.Usage.CompletionTokens
	r.FormatCorrect = checkNimFormat(r.ResponseText, r.Scenario)
	r.SemanticCorrect = checkNimSemantic(r.ResponseText, r.Scenario)
	return r
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type nimScenario struct {
	name      string
	build     func(bool) string
	maxTokens int
	temp      float64
}

func main() {
	apiKey := os.Getenv("NVIDIA_NIM_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "NVIDIA_NIM_API_KEY not set")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// NIM model that showed different failure modes in Phase 383
	models := []string{
		"meta/llama-3.1-8b-instruct",
	}

	scenarios := []nimScenario{
		{"adv-language-degradation", buildNimLangDegradationPrompt, 128, 0.0},
		{"adv-conflicting-data", buildNimConflictPrompt, 128, 0.0},
		{"adv-format-pressure", buildNimFormatPressurePrompt, 48, 0.7},
	}

	var results []nimResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	trialIdx := 0
	for _, sc := range scenarios {
		promptWith := sc.build(true)
		promptWithout := sc.build(false)
		for _, m := range models {
			for hasExample := 0; hasExample < 2; hasExample++ {
				for trialNum := 0; trialNum < nimTrials; trialNum++ {
					trialIdx++
					wg.Add(1)
					go func(m string, sc nimScenario, hasEx bool, trialNum, idx int, promptW, promptWo string) {
						defer wg.Done()
						prompt := promptWo
						if hasEx {
							prompt = promptW
						}
						tr := nimTrial{
							Model:       m,
							Temperature: sc.temp,
							MaxTokens:   sc.maxTokens,
							Prompt:      prompt,
							HasExample:  hasEx,
							Scenario:    sc.name,
						}
						// Stagger to avoid bursts
						time.Sleep(time.Duration(idx%3) * 500 * time.Millisecond)
						r := runNimTrial(ctx, apiKey, tr, trialNum)
						mu.Lock()
						results = append(results, r)
						fmt.Printf("[%s] %s model=%s example=%v trial=%d format=%v semantic=%v latency=%.0fms tokens=%d finish=%s\n",
							time.Now().Format("15:04:05"), sc.name, m, hasEx, trialNum,
							r.FormatCorrect, r.SemanticCorrect, r.LatencyMs, r.OutputTokens, r.FinishReason)
						mu.Unlock()
					}(m, sc, hasExample == 1, trialNum, trialIdx, promptWith, promptWithout)
				}
			}
		}
	}
	wg.Wait()

	// Summary
	fmt.Println("\n=== NIM SUMMARY ===")
	type cell struct {
		formatHits, semanticHits, total int
		latencySum                      float64
		tokenSum                        int
		finishReasons                   map[string]int
	}
	cells := map[string]*cell{}
	for _, r := range results {
		key := fmt.Sprintf("%s|scenario=%s|example=%v", r.Model, r.Scenario, r.HasExample)
		c, ok := cells[key]
		if !ok {
			c = &cell{finishReasons: map[string]int{}}
			cells[key] = c
		}
		c.total++
		if r.FormatCorrect {
			c.formatHits++
		}
		if r.SemanticCorrect {
			c.semanticHits++
		}
		c.latencySum += r.LatencyMs
		c.tokenSum += r.OutputTokens
		if r.FinishReason != "" {
			c.finishReasons[r.FinishReason]++
		}
	}

	for key := range cells {
		c := cells[key]
		finishes := make([]string, 0, len(c.finishReasons))
		for fr, count := range c.finishReasons {
			finishes = append(finishes, fmt.Sprintf("%s×%d", fr, count))
		}
		fmt.Printf("%s: format=%d/%d (%.0f%%) semantic=%d/%d (%.0f%%) avg_latency=%.0fms avg_tokens=%d finish=[%s]\n",
			key, c.formatHits, c.total, float64(c.formatHits)/float64(c.total)*100,
			c.semanticHits, c.total, float64(c.semanticHits)/float64(c.total)*100,
			c.latencySum/float64(c.total), c.tokenSum/c.total, strings.Join(finishes, ", "))
	}

	// Write results JSON
	outDir := "results/format-example-campaign-2026-08-07"
	os.MkdirAll(outDir, 0755)
	out, _ := json.MarshalIndent(results, "", "  ")
	_ = os.WriteFile(outDir+"/nim-results.json", out, 0644)
	fmt.Println("\nResults saved to " + outDir + "/nim-results.json")
}
