// Command thinking_budget_threshold runs a bounded live campaign testing
// the hypothesis that thinking models (qwen3.6-27b) need significantly
// larger max_tokens to produce both thinking AND the structured answer.
//
// Phase 387 finding: qwen3.6-27b scored 0/15 semantic correctness at
// max_tokens=128 because thinking tokens consumed the entire budget
// (all finish_reason=length, no answer emitted).
//
// This campaign escalates max_tokens: 128, 256, 384, 512, 768, 1024, 2048
// to find the threshold where qwen3.6-27b starts producing the answer.
// Control: llama-3.3-70b-versatile (non-thinking, already passes at 128).
//
// Hypothesis: qwen3.6-27b needs at least 384-512 max_tokens to emit
// thinking + DATE/SOURCE answer. Below that, finish_reason=length
// always fires. The BudgetGuard floor should account for thinking overhead.
//
// Celling: 7 models × 1 tasks × 7 token levels × 3 reps = 147 calls max.
// Actually: 2 models × 7 levels × 3 reps = 42 calls.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

type trial struct {
	Model            string  `json:"model"`
	Provider         string  `json:"provider"`
	MaxTokens        int     `json:"max_tokens"`
	HTTPStatus       int     `json:"http_status"`
	LatencyMs        float64 `json:"latency_ms"`
	Content          string  `json:"content"`
	FinishReason     string  `json:"finish_reason"`
	Error            string  `json:"error,omitempty"`
	PromptTokens     int     `json:"prompt_tokens,omitempty"`
	CompletionTokens int     `json:"completion_tokens,omitempty"`
	FormatCorrect    bool    `json:"format_correct"`
	SemanticCorrect  bool    `json:"semantic_correct"`
	ThinkingTokens   int     `json:"thinking_tokens"`
	AnswerTokens     int     `json:"answer_tokens"`
}

var (
	groqKey = os.Getenv("GROQ_API_KEY")
	groqURL = "https://api.groq.com/openai/v1/chat/completions"

	// The prompt from Phase 387 baseline (CoT-poisoned)
	prompt = "Extract the publication date and source from the following text. Below are examples of correct extraction.\n\nExample 1:\nText: Paper published 2024-06-15 by team X-42.\nAnswer:\nSOURCE: X-42\nNOTE: Source first, date omitted for brevity.\n\nNow extract from this text:\n\nText: O artigo foi publicado em 2025-11-03 pela equipe S-17.\n\nReturn your answer in this format:\nDATE: <value>\nSOURCE: <value>"

	expectedDate   = "2025-11-03"
	expectedSource = "S-17"
)

func callGroq(ctx context.Context, model, apiKey string, maxTokens int) (int, string, string, float64, int, int, error) {
	body := map[string]interface{}{
		"model":       model,
		"temperature": 0.0,
		"max_tokens":  maxTokens,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequestWithContext(ctx, "POST", groqURL, bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", "", float64(time.Since(start).Milliseconds()), 0, 0, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	latency := float64(time.Since(start).Milliseconds())

	if resp.StatusCode != 200 {
		return resp.StatusCode, "", "", latency, 0, 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return resp.StatusCode, "", "", latency, 0, 0, fmt.Errorf("JSON decode: %v", err)
	}
	if len(result.Choices) == 0 {
		return resp.StatusCode, "", "", latency, 0, 0, fmt.Errorf("no choices")
	}

	return resp.StatusCode, strings.TrimSpace(result.Choices[0].Message.Content),
		result.Choices[0].FinishReason, latency,
		result.Usage.PromptTokens, result.Usage.CompletionTokens, nil
}

func checkResult(content string) (bool, bool, int, int) {
	lines := strings.Split(content, "\n")
	var dateVal, sourceVal string
	var dateLine, sourceLine int = -1, -1

	// Detect thinking block (qwen3.6 emits  or raw thinking before answer)
	thinkingTokens := 0
	answerStartLine := 0
	for idx, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(line), "DATE:") {
			dateVal = strings.TrimSpace(line[5:])
			if dateLine == -1 {
				dateLine = idx
			}
		}
		if strings.HasPrefix(strings.ToUpper(line), "SOURCE:") {
			sourceVal = strings.TrimSpace(line[7:])
			if sourceLine == -1 {
				sourceLine = idx
			}
		}
	}

	// If DATE line found, everything before it is "thinking"
	if dateLine >= 0 {
		answerStartLine = dateLine
		thinkingTokens = len(strings.Join(lines[:answerStartLine], " ")) / 4 // rough estimate
	} else {
		// No DATE line found — entire output is thinking
		thinkingTokens = len(content) / 4
	}

	answerTokens := 0
	if answerStartLine > 0 && answerStartLine < len(lines) {
		answerTokens = len(strings.Join(lines[answerStartLine:], " ")) / 4
	}

	fmtOK := (dateLine != -1 && sourceLine != -1 && dateLine < sourceLine)
	semOK := fmtOK && (dateVal == expectedDate && sourceVal == expectedSource)

	return fmtOK, semOK, thinkingTokens, answerTokens
}

func main() {
	if groqKey == "" {
		fmt.Println("GROQ_API_KEY is required")
		os.Exit(1)
	}

	ctx := context.Background()
	var trials []trial

	models := []string{
		"qwen/qwen3.6-27b",        // thinking model — the subject
		"llama-3.3-70b-versatile", // non-thinking control
	}

	tokenLevels := []int{128, 256, 384, 512, 768, 1024, 2048}
	reps := 3

	for _, model := range models {
		for _, maxTok := range tokenLevels {
			for r := 1; r <= reps; r++ {
				callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
				status, content, finish, latency, ptoks, ctoks, err := callGroq(
					callCtx, model, groqKey, maxTok)
				cancel()

				fmtOK, semOK, thinkTok, ansTok := checkResult(content)

				tr := trial{
					Model:            model,
					Provider:         "groq",
					MaxTokens:        maxTok,
					HTTPStatus:       status,
					LatencyMs:        latency,
					Content:          content,
					FinishReason:     finish,
					PromptTokens:     ptoks,
					CompletionTokens: ctoks,
					FormatCorrect:    fmtOK,
					SemanticCorrect:  semOK,
					ThinkingTokens:   thinkTok,
					AnswerTokens:     ansTok,
				}
				if err != nil {
					tr.Error = err.Error()
				}

				trials = append(trials, tr)

				fmt.Printf("[%s | max_tok=%d | rep %d] status=%d fmt=%v sem=%v finish=%s lat=%.0fms think_tok~%d ans_tok~%d\n",
					model, maxTok, r, status, fmtOK, semOK, finish, latency, thinkTok, ansTok)

				time.Sleep(350 * time.Millisecond)
			}
		}
	}

	// Ensure directory exists
	outDir := "/home/node/.openclaw/workspace/motor-autonomo/results/thinking-budget-threshold-phase388"
	_ = os.MkdirAll(outDir, 0755)

	// Save results
	data, _ := json.MarshalIndent(trials, "", "  ")
	_ = os.WriteFile(outDir+"/results.json", data, 0644)

	// Summary Markdown
	var sb strings.Builder
	sb.WriteString("# Phase 388 — Thinking Budget Threshold Campaign\n\n")
	sb.WriteString(fmt.Sprintf("**Date:** %s\n", time.Now().Format("2006-01-02 15:04 -0700")))
	sb.WriteString(fmt.Sprintf("**Total trials:** %d\n\n", len(trials)))
	sb.WriteString("**Hypothesis:** qwen3.6-27b needs at least 384-512 max_tokens to emit\nboth thinking tokens and the structured DATE:/SOURCE: answer.\nBelow that, finish_reason=length truncates before the answer.\n\n")
	sb.WriteString("**Models:** qwen/qwen3.6-27b (thinking), llama-3.3-70b-versatile (control)\n\n")
	sb.WriteString("| Model | Max Tok | N | Fmt OK | Sem OK | Avg Lat ms | Avg Comp Tok | Avg Think Tok | Avg Ans Tok |\n")
	sb.WriteString("|-------|---------|---|--------|--------|------------|--------------|---------------|-------------|\n")

	type aggKey struct {
		model     string
		maxTokens int
	}
	type aggVal struct {
		n, fmtOK, semOK int
		latencies       []float64
		compTokens      []int
		thinkTokens     []int
		ansTokens       []int
	}
	aggs := map[aggKey]*aggVal{}

	for _, tr := range trials {
		k := aggKey{tr.Model, tr.MaxTokens}
		v, ok := aggs[k]
		if !ok {
			v = &aggVal{}
			aggs[k] = v
		}
		v.n++
		if tr.FormatCorrect {
			v.fmtOK++
		}
		if tr.SemanticCorrect {
			v.semOK++
		}
		v.latencies = append(v.latencies, tr.LatencyMs)
		v.compTokens = append(v.compTokens, tr.CompletionTokens)
		v.thinkTokens = append(v.thinkTokens, tr.ThinkingTokens)
		v.ansTokens = append(v.ansTokens, tr.AnswerTokens)
	}

	// Sort keys for stable output
	type sortedKey struct {
		model     string
		maxTokens int
	}
	var keys []sortedKey
	for k := range aggs {
		keys = append(keys, sortedKey{k.model, k.maxTokens})
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].model != keys[j].model {
			return keys[i].model < keys[j].model
		}
		return keys[i].maxTokens < keys[j].maxTokens
	})

	for _, sk := range keys {
		k := aggKey{sk.model, sk.maxTokens}
		v := aggs[k]
		avgLat := 0.0
		avgComp := 0.0
		avgThink := 0.0
		avgAns := 0.0
		for _, l := range v.latencies {
			avgLat += l
		}
		for _, c := range v.compTokens {
			avgComp += float64(c)
		}
		for _, th := range v.thinkTokens {
			avgThink += float64(th)
		}
		for _, a := range v.ansTokens {
			avgAns += float64(a)
		}
		avgLat /= float64(v.n)
		avgComp /= float64(v.n)
		avgThink /= float64(v.n)
		avgAns /= float64(v.n)

		sb.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %d | %.0f | %.0f | %.0f | %.0f |\n",
			sk.model, sk.maxTokens, v.n, v.fmtOK, v.semOK, avgLat, avgComp, avgThink, avgAns))
	}

	_ = os.WriteFile(outDir+"/REPORT.md", []byte(sb.String()), 0644)
	fmt.Println("\nCampaign complete! Results written to " + outDir)
}
