package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"motor-autonomo/internal/gatecampaign"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/provider/openai"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/sqlite"
)

const maxManifestBytes = 1 << 20

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	manifestPath := flag.String("manifest", "", "runtime gate campaign manifest")
	outputDirectory := flag.String("out", "", "artifact directory")
	trials := flag.Int("trials", 1, "isolated one-call trials (1..5)")
	flag.Parse()
	if *manifestPath == "" || *outputDirectory == "" {
		return errors.New("-manifest and -out are required")
	}
	file, err := os.Open(*manifestPath)
	if err != nil {
		return err
	}
	manifest, err := gatecampaign.DecodeRuntimeGateCampaignManifest(file, maxManifestBytes)
	file.Close()
	if err != nil {
		return err
	}
	providers := make(map[string]port.ModelProvider, len(manifest.Bindings))
	for _, binding := range manifest.Bindings {
		key := os.Getenv(binding.APIKeyEnvironment)
		if key == "" {
			return fmt.Errorf("environment variable %s is required", binding.APIKeyEnvironment)
		}
		field := openai.MaxOutputField(binding.MaxOutputField)
		provider, err := openai.New(openai.Config{BaseURL: binding.BaseURL, APIKey: key, Model: binding.Model, MaxOutputField: field, Timeout: time.Duration(manifest.TimeoutSeconds) * time.Second, MaxResponseBytes: 1 << 20}, openai.WithProfileName(binding.Provider), openai.WithContextTokens(binding.ContextTokens), openai.WithProbeBudget(0))
		if err != nil {
			return fmt.Errorf("build binding %s: %w", binding.BindingID, err)
		}
		providers[binding.BindingID] = provider
	}
	if *trials <= 0 || *trials > gatecampaign.MaxRuntimeGateBatchTrials {
		return fmt.Errorf("-trials must be between 1 and %d", gatecampaign.MaxRuntimeGateBatchTrials)
	}
	if err := os.MkdirAll(*outputDirectory, 0o755); err != nil {
		return err
	}
	manifestArtifact := filepath.Join(*outputDirectory, "manifest.json")
	if err := gatecampaign.WriteRuntimeGateCampaignManifest(manifestArtifact, manifest); err != nil {
		return err
	}
	reports := make([]gatecampaign.RuntimeGateCampaignReport, 0, *trials)
	for trial := 1; trial <= *trials; trial++ {
		trialDirectory := *outputDirectory
		if *trials > 1 {
			trialDirectory = filepath.Join(*outputDirectory, "trials", fmt.Sprintf("%03d", trial))
			if err := os.MkdirAll(trialDirectory, 0o755); err != nil {
				return err
			}
			if err := gatecampaign.WriteRuntimeGateCampaignManifest(filepath.Join(trialDirectory, "manifest.json"), manifest); err != nil {
				return err
			}
		}
		report, err := runTrial(manifest, providers, trialDirectory)
		if err != nil {
			return fmt.Errorf("trial %d: %w", trial, err)
		}
		reports = append(reports, report)
	}
	if *trials > 1 {
		batch, err := gatecampaign.BuildRuntimeGateBatchReport(manifest.Name, reports)
		if err != nil {
			return err
		}
		if err := gatecampaign.WriteRuntimeGateBatchArtifacts(*outputDirectory, batch); err != nil {
			return err
		}
		body, _ := json.Marshal(map[string]any{"trials": batch.Trials, "calls": batch.ExternalCalls, "successes": batch.ProviderSuccesses, "exact_matches": batch.ExpectedMatches, "durable_reopens": batch.DurableReopens, "latency_p95": batch.LatencyP95})
		fmt.Println(string(body))
		return nil
	}
	report := reports[0]
	body, _ := json.Marshal(map[string]any{"calls": report.ExternalCalls, "binding": report.SelectedBindingID, "provider_success": report.ProviderSucceeded, "http_status": report.ProviderHTTPStatus, "second_acquire": report.SecondAcquireReason, "durable_reopen": report.DurableReopen})
	fmt.Println(string(body))
	return nil
}

func runTrial(manifest gatecampaign.RuntimeGateCampaignManifest, providers map[string]port.ModelProvider, outputDirectory string) (gatecampaign.RuntimeGateCampaignReport, error) {
	databasePath := filepath.Join(outputDirectory, "runtime-gate.sqlite")
	store, err := sqlite.Open(databasePath)
	if err != nil {
		return gatecampaign.RuntimeGateCampaignReport{}, err
	}
	clock := source.SystemClock{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(manifest.TimeoutSeconds)*time.Second)
	defer cancel()
	// NOTE: This could be customized for malformed recovery, but currently reusing RuntimeGateCampaignRunner
	report, runErr := (gatecampaign.RuntimeGateCampaignRunner{Store: store, Clock: clock, Providers: providers}).Run(ctx, manifest)
	closeErr := store.Close()
	if runErr != nil {
		return gatecampaign.RuntimeGateCampaignReport{}, runErr
	}
	if closeErr != nil {
		return gatecampaign.RuntimeGateCampaignReport{}, closeErr
	}
	reopened, err := sqlite.Open(databasePath)
	if err != nil {
		return gatecampaign.RuntimeGateCampaignReport{}, fmt.Errorf("reopen runtime gate store: %w", err)
	}
	if err := gatecampaign.VerifyRuntimeGateDurability(context.Background(), reopened, report); err != nil {
		reopened.Close()
		return gatecampaign.RuntimeGateCampaignReport{}, err
	}
	if err := reopened.Close(); err != nil {
		return gatecampaign.RuntimeGateCampaignReport{}, err
	}
	report.DurableReopen = true
	if err := gatecampaign.WriteRuntimeGateCampaignArtifacts(outputDirectory, report); err != nil {
		return gatecampaign.RuntimeGateCampaignReport{}, err
	}
	return report, nil
}
