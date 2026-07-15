package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"motor-autonomo/internal/evaluation"
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/provider/openai"
)

func main() {
	var fixturePath, outputDirectory, baseURL, model, contexts, maxField, apiKeyEnvironment string
	var timeout time.Duration
	flag.StringVar(&fixturePath, "fixtures", "internal/evaluation/testdata/cognitive-v1.json", "fixture JSON path")
	flag.StringVar(&outputDirectory, "out", "results/model-benchmark", "artifact directory")
	flag.StringVar(&baseURL, "base-url", "", "OpenAI-compatible base URL")
	flag.StringVar(&model, "model", "", "provider model identifier")
	flag.StringVar(&contexts, "contexts", "2048,4096,8192", "comma-separated context limits")
	flag.StringVar(&maxField, "max-output-field", string(openai.MaxOutputTokensLegacy), "max_tokens or max_completion_tokens")
	flag.StringVar(&apiKeyEnvironment, "api-key-env", "OPENAI_API_KEY", "environment variable containing the API key")
	flag.DurationVar(&timeout, "timeout", 2*time.Minute, "timeout for the complete matrix")
	flag.Parse()
	if baseURL == "" || model == "" {
		log.Fatal("-base-url and -model are required")
	}
	limits, err := parseContexts(contexts)
	if err != nil {
		log.Fatal(err)
	}
	file, err := os.Open(fixturePath)
	if err != nil {
		log.Fatal(err)
	}
	fixtures, err := evaluation.DecodeFixtures(file, 1<<20)
	closeErr := file.Close()
	if err != nil {
		log.Fatal(err)
	}
	if closeErr != nil {
		log.Fatal(closeErr)
	}
	provider, err := openai.New(openai.Config{BaseURL: baseURL, APIKey: os.Getenv(apiKeyEnvironment), Model: model, MaxOutputField: openai.MaxOutputField(maxField), Client: &http.Client{Timeout: 90 * time.Second}})
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	report, err := (evaluation.Runner{Provider: provider, Estimator: prompt.ConservativeEstimator{}, Spec: evaluation.DefaultOperationSpec()}).Run(ctx, fixtures, evaluation.Matrix{ContextTokens: limits})
	if err != nil {
		log.Fatal(err)
	}
	if err := evaluation.WriteArtifacts(outputDirectory, report); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("runs=%d correct=%d syntax_valid=%d artifacts=%s\n", report.Summary.Total, report.Summary.SemanticallyRight, report.Summary.SyntaxValid, outputDirectory)
}

func parseContexts(value string) ([]int, error) {
	parts := strings.Split(value, ",")
	limits := make([]int, 0, len(parts))
	seen := map[int]bool{}
	for _, part := range parts {
		limit, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || limit <= 0 || seen[limit] {
			return nil, fmt.Errorf("invalid context matrix %q", value)
		}
		seen[limit] = true
		limits = append(limits, limit)
	}
	if len(limits) == 0 {
		return nil, fmt.Errorf("context matrix is empty")
	}
	return limits, nil
}
