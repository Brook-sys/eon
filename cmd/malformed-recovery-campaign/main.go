//go:build ignore

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"motor-autonomo/internal/gatecampaign"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/provider/openai"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/sqlite"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	manifestPath := flag.String("manifest", "", "malformed recovery campaign manifest")
	outputDirectory := flag.String("out", "", "artifact directory")
	flag.Parse()
	if *manifestPath == "" || *outputDirectory == "" {
		return errors.New("-manifest and -out are required")
	}
	file, err := os.Open(*manifestPath)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var manifest gatecampaign.MalformedRecoveryCampaignManifest
	if err := decoder.Decode(&manifest); err != nil {
		return fmt.Errorf("decode malformed recovery manifest: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return errors.New("malformed recovery manifest contains trailing data")
	} else if err != io.EOF {
		return fmt.Errorf("decode trailing malformed recovery manifest data: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return err
	}

	providers := make(map[string]port.ModelProvider, len(manifest.Bindings))
	for _, binding := range manifest.Bindings {
		key := os.Getenv(binding.APIKeyEnvironment)
		if key == "" {
			return fmt.Errorf("environment variable %s is required", binding.APIKeyEnvironment)
		}
		provider, err := openai.New(openai.Config{BaseURL: binding.BaseURL, APIKey: key, Model: binding.Model, MaxOutputField: openai.MaxOutputField(binding.MaxOutputField), Timeout: time.Duration(manifest.TimeoutSeconds) * time.Second, MaxResponseBytes: 1 << 20}, openai.WithProfileName(binding.Provider), openai.WithContextTokens(binding.ContextTokens), openai.WithProbeBudget(0))
		if err != nil {
			return fmt.Errorf("build binding %s: %w", binding.BindingID, err)
		}
		providers[binding.BindingID] = provider
	}
	if err := os.MkdirAll(*outputDirectory, 0o755); err != nil {
		return err
	}
	if err := gatecampaign.WriteMalformedRecoveryCampaignManifest(filepath.Join(*outputDirectory, "manifest.json"), manifest); err != nil {
		return err
	}
	databasePath := filepath.Join(*outputDirectory, "malformed-recovery.sqlite")
	store, err := sqlite.Open(databasePath)
	if err != nil {
		return err
	}
	clock := source.SystemClock{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(manifest.TimeoutSeconds)*time.Second)
	defer cancel()
	report, runErr := (gatecampaign.MalformedRecoveryCampaignRunner{Store: store, Clock: clock, Providers: providers}).Run(ctx, manifest)
	closeErr := store.Close()
	if runErr != nil {
		return runErr
	}
	if closeErr != nil {
		return closeErr
	}
	reopened, err := sqlite.Open(databasePath)
	if err != nil {
		return fmt.Errorf("reopen malformed recovery store: %w", err)
	}
	if err := gatecampaign.VerifyMalformedRecoveryDurability(context.Background(), reopened, report); err != nil {
		reopened.Close()
		return err
	}
	if err := reopened.Close(); err != nil {
		return err
	}
	report.DurableReopen = true
	if err := gatecampaign.WriteMalformedRecoveryCampaignArtifacts(*outputDirectory, report); err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{"model_calls": report.ModelCalls, "external_calls": report.ExternalCalls, "recovery_stages": report.RecoveryStages, "commit": report.CommitID, "durable_reopen": report.DurableReopen})
	fmt.Println(string(body))
	return nil
}
