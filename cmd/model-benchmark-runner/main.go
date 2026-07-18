package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"motor-autonomo/internal/evaluation"
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/provider/openai"
)

func main() {
	var fixturePath, outputDirectory, baseURL, model, contexts, maxField, apiKeyEnvironment, mode, campaignPath string
	var timeout time.Duration
	flag.StringVar(&fixturePath, "fixtures", "internal/evaluation/testdata/cognitive-v2.json", "fixture JSON path")
	flag.StringVar(&outputDirectory, "out", "results/model-benchmark", "artifact directory")
	flag.StringVar(&baseURL, "base-url", "", "OpenAI-compatible base URL (required for mode=live)")
	flag.StringVar(&model, "model", "", "provider model identifier (required for mode=live)")
	flag.StringVar(&contexts, "contexts", "2048,4096,8192", "comma-separated context limits")
	flag.StringVar(&maxField, "max-output-field", string(openai.MaxOutputTokensLegacy), "max_tokens or max_completion_tokens")
	flag.StringVar(&apiKeyEnvironment, "api-key-env", "OPENAI_API_KEY", "environment variable containing the API key")
	flag.StringVar(&mode, "mode", "live", "run mode: live | offline-oracle | offline-compile")
	flag.StringVar(&campaignPath, "campaign", "", "bounded multi-model campaign manifest JSON (live mode)")
	flag.DurationVar(&timeout, "timeout", 2*time.Minute, "timeout for the complete matrix")
	flag.Parse()
	if strings.TrimSpace(campaignPath) != "" {
		if err := runCampaign(campaignPath, outputDirectory); err != nil {
			log.Fatal(err)
		}
		return
	}

	mode = strings.TrimSpace(strings.ToLower(mode))
	switch mode {
	case "live", "offline-oracle", "offline-compile":
	default:
		log.Fatalf("unsupported -mode %q (want live|offline-oracle|offline-compile)", mode)
	}
	if mode == "live" && (baseURL == "" || model == "") {
		log.Fatal("-base-url and -model are required for mode=live (use -mode=offline-oracle without a provider)")
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

	matrix := evaluation.Matrix{ContextTokens: limits}
	spec := evaluation.DefaultOperationSpec()
	estimator := prompt.ConservativeEstimator{}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var report evaluation.Report
	switch mode {
	case "offline-compile":
		report, err = evaluation.CompileMatrix(fixtures, matrix, estimator, spec)
	case "offline-oracle":
		report, err = evaluation.RunOracle(ctx, fixtures, matrix, estimator, spec)
	default:
		provider, perr := openai.New(openai.Config{
			BaseURL:        baseURL,
			APIKey:         os.Getenv(apiKeyEnvironment),
			Model:          model,
			MaxOutputField: openai.MaxOutputField(maxField),
			Client:         &http.Client{Timeout: 90 * time.Second},
		})
		if perr != nil {
			log.Fatal(perr)
		}
		report, err = (evaluation.Runner{Provider: provider, Estimator: estimator, Spec: spec, ModelLabel: model}).Run(ctx, fixtures, matrix)
	}
	if err != nil {
		log.Fatal(err)
	}
	if err := evaluation.WriteArtifacts(outputDirectory, report); err != nil {
		log.Fatal(err)
	}
	interp := evaluation.InterpretReport(report)
	fmt.Printf("mode=%s verdict=%s runs=%d correct=%d syntax_valid=%d artifacts=%s\n", mode, interp.Verdict, report.Summary.Total, report.Summary.SemanticallyRight, report.Summary.SyntaxValid, outputDirectory)
	fmt.Printf("headline=%s\n", interp.Headline)
}

func runCampaign(manifestPath, outputDirectory string) error {
	file, err := os.Open(manifestPath)
	if err != nil {
		return err
	}
	manifest, err := evaluation.DecodeCampaignManifest(file, 1<<20)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	fixtureFile, err := os.Open(manifest.FixturePath)
	if err != nil {
		return err
	}
	fixtures, err := evaluation.DecodeFixtures(fixtureFile, 1<<20)
	fixtureCloseErr := fixtureFile.Close()
	if err != nil {
		return err
	}
	if fixtureCloseErr != nil {
		return fixtureCloseErr
	}
	planned := manifest.PlannedCalls(fixtures)
	if planned > manifest.MaxCalls {
		return fmt.Errorf("campaign planned calls %d exceed max_calls %d", planned, manifest.MaxCalls)
	}
	if err := evaluation.WriteCampaignManifest(filepath.Join(outputDirectory, "manifest.json"), manifest); err != nil {
		return err
	}
	matrix := evaluation.Matrix{ContextTokens: manifest.ContextTokens}
	spec := evaluation.DefaultOperationSpec()
	spec.MaxOutputTokens = manifest.MaxOutputTokens
	estimator := prompt.ConservativeEstimator{}
	campaign := evaluation.CampaignReport{SchemaVersion: evaluation.CampaignSchemaVersion, Name: manifest.Name, FixtureName: fixtures.Name, PlannedCalls: planned, MaxCalls: manifest.MaxCalls}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(manifest.TimeoutSeconds)*time.Second)
	defer cancel()
	for _, target := range manifest.Models {
		provider, err := openai.New(openai.Config{BaseURL: target.BaseURL, APIKey: os.Getenv(target.APIKeyEnvironment), Model: target.Model, MaxOutputField: openai.MaxOutputField(target.MaxOutputField), Client: &http.Client{Timeout: 90 * time.Second}})
		if err != nil {
			return fmt.Errorf("binding %s: %w", target.BindingID, err)
		}
		report, err := (evaluation.Runner{Provider: provider, Estimator: estimator, Spec: spec, ModelLabel: target.Model}).Run(ctx, fixtures, matrix)
		if err != nil {
			return fmt.Errorf("binding %s: %w", target.BindingID, err)
		}
		modelReport := evaluation.CampaignModelReport{Provider: target.Provider, BindingID: target.BindingID, Model: target.Model, Report: report, Qualification: evaluation.QualifyReport(report)}
		if strings.TrimSpace(target.BaselineReport) != "" {
			baseline, err := evaluation.ReadReport(target.BaselineReport)
			if err != nil {
				return fmt.Errorf("binding %s baseline: %w", target.BindingID, err)
			}
			modelReport.Regression, err = evaluation.CompareReports(baseline, report)
			if err != nil {
				return fmt.Errorf("binding %s comparison: %w", target.BindingID, err)
			}
		}
		campaign.Models = append(campaign.Models, modelReport)
		if err := evaluation.WriteArtifacts(filepath.Join(outputDirectory, target.BindingID), report); err != nil {
			return err
		}
	}
	if err := evaluation.WriteCampaignArtifacts(outputDirectory, campaign); err != nil {
		return err
	}
	fmt.Printf("mode=campaign name=%s models=%d planned_calls=%d artifacts=%s\n", manifest.Name, len(campaign.Models), planned, outputDirectory)
	return nil
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
