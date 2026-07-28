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

const pacingStateSchemaVersion = 1

const faultAfterPacingStateEnvironment = "MOTOR_AUTONOMO_FAULT_AFTER_PACING_STATE_TRIAL"

type pacingState struct {
	SchemaVersion      int       `json:"schema_version"`
	CampaignName       string    `json:"campaign_name"`
	PlannedTrials      int       `json:"planned_trials"`
	CompletedTrials    int       `json:"completed_trials"`
	NextTrialNotBefore time.Time `json:"next_trial_not_before,omitempty"`
}

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
	batchClock := source.SystemClock{}
	batchContext := context.Background()
	statePath := filepath.Join(*outputDirectory, "pacing-state.json")
	state, reports, err := loadPacingState(statePath, *outputDirectory, manifest.Name, *trials)
	if err != nil {
		return err
	}
	for trial := state.CompletedTrials + 1; trial <= *trials; trial++ {
		if trial > 1 {
			if err := waitUntilNextTrial(batchContext, batchClock, state.NextTrialNotBefore); err != nil {
				return fmt.Errorf("wait before trial %d: %w", trial, err)
			}
		}
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
		if err != nil && report.SchemaVersion == 0 {
			return fmt.Errorf("trial %d: %w", trial, err)
		}
		reports = append(reports, report)
		state.CompletedTrials = trial
		state.NextTrialNotBefore = report.CompletedAt.Add(time.Duration(manifest.InterTrialDelaySeconds) * time.Second)
		if err := writePacingState(statePath, state); err != nil {
			return fmt.Errorf("persist pacing after trial %d: %w", trial, err)
		}
		crashAfterPacingStatePublication(trial)
		if err != nil && *trials == 1 {
			return fmt.Errorf("trial %d: %w", trial, err)
		}
		if stop, _ := gatecampaign.RepeatedFailureEarlyStop(reports, manifest.EarlyStopRepeatedFailures); stop {
			break
		}
	}
	if *trials > 1 {
		batch, err := gatecampaign.BuildRuntimeGateBatchReport(manifest.Name, reports)
		if err != nil {
			return err
		}
		_, stopReason := gatecampaign.RepeatedFailureEarlyStop(reports, manifest.EarlyStopRepeatedFailures)
		batch = gatecampaign.AnnotateRuntimeGateBatchStop(batch, *trials, stopReason)
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

func loadPacingState(path, outputDirectory, campaign string, planned int) (pacingState, []gatecampaign.RuntimeGateCampaignReport, error) {
	state := pacingState{SchemaVersion: pacingStateSchemaVersion, CampaignName: campaign, PlannedTrials: planned}
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil, nil
	}
	if err != nil {
		return state, nil, err
	}
	if err := json.Unmarshal(body, &state); err != nil {
		return state, nil, fmt.Errorf("decode pacing state: %w", err)
	}
	if state.SchemaVersion != pacingStateSchemaVersion || state.CampaignName != campaign || state.PlannedTrials != planned || state.CompletedTrials < 0 || state.CompletedTrials > planned {
		return state, nil, errors.New("pacing state does not match requested campaign")
	}
	reports := make([]gatecampaign.RuntimeGateCampaignReport, 0, state.CompletedTrials)
	for trial := 1; trial <= state.CompletedTrials; trial++ {
		body, err := os.ReadFile(filepath.Join(outputDirectory, "trials", fmt.Sprintf("%03d", trial), "runtime-gate.json"))
		if err != nil {
			return state, nil, fmt.Errorf("load completed trial %d: %w", trial, err)
		}
		var report gatecampaign.RuntimeGateCampaignReport
		if err := json.Unmarshal(body, &report); err != nil || report.SchemaVersion == 0 || !report.DurableReopen {
			return state, nil, fmt.Errorf("completed trial %d report is invalid or not durable", trial)
		}
		reports = append(reports, report)
	}
	return state, reports, nil
}

func crashAfterPacingStatePublication(trial int) {
	if os.Getenv(faultAfterPacingStateEnvironment) == fmt.Sprint(trial) {
		os.Exit(86)
	}
}

func writePacingState(path string, state pacingState) error {
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(body, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func waitUntilNextTrial(ctx context.Context, clock source.Clock, deadline time.Time) error {
	if clock == nil {
		return errors.New("clock is required")
	}
	if deadline.IsZero() {
		return nil
	}
	return clock.WaitUntil(ctx, deadline)
}

func waitBeforeNextTrial(ctx context.Context, clock source.Clock, previous gatecampaign.RuntimeGateCampaignReport, delay time.Duration) error {
	if clock == nil {
		return errors.New("clock is required")
	}
	if delay <= 0 {
		return nil
	}
	return clock.WaitUntil(ctx, previous.CompletedAt.Add(delay))
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
	report, runErr := (gatecampaign.RuntimeGateCampaignRunner{Store: store, Clock: clock, Providers: providers}).Run(ctx, manifest)
	closeErr := store.Close()
	if closeErr != nil {
		return gatecampaign.RuntimeGateCampaignReport{}, closeErr
	}
	if runErr != nil && report.SchemaVersion == 0 {
		return report, runErr
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
	return report, runErr
}
