package bootstrap

import (
	"fmt"
	"os"
	"strings"

	"motor-autonomo/internal/changeset"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/observability"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/provider/openai"
	"motor-autonomo/internal/runtime/source"
)

// buildModel assembles an optional PROPOSE_ONLY ModelExecutor. Returns nil when
// Model is disabled so non-local ops stay skipped as requires_model.
func buildModel(
	opts Options,
	store port.Store,
	clock source.Clock,
	ids source.IDGenerator,
	telemetry *observability.Runtime,
) (*kernel.ModelExecutor, error) {
	if opts.Model == nil || !opts.Model.Enabled {
		return nil, nil
	}
	apiKey := ""
	if env := strings.TrimSpace(opts.Model.APIKeyEnv); env != "" {
		apiKey = strings.TrimSpace(os.Getenv(env))
		// Empty key is allowed: some local OpenAI-compatible servers ignore auth.
		// We only refuse when the env name was set but we cannot read it at all —
		// getenv of missing var is empty, which is intentional for open endpoints.
	}
	maxField := openai.MaxOutputTokensLegacy
	switch opts.Model.MaxOutputField {
	case ModelMaxOutputTokensCompletion:
		maxField = openai.MaxOutputTokensCompletion
	case ModelMaxOutputTokensLegacy, "":
		maxField = openai.MaxOutputTokensLegacy
	default:
		return nil, fmt.Errorf("unsupported model max-output field %q", opts.Model.MaxOutputField)
	}
	provider, err := openai.New(openai.Config{
		BaseURL:          opts.Model.BaseURL,
		APIKey:           apiKey,
		Model:            opts.Model.Model,
		MaxOutputField:   maxField,
		MaxResponseBytes: opts.Model.MaxResponseBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("model provider: %w", err)
	}
	var modelProvider port.ModelProvider = provider
	if telemetry != nil {
		modelProvider = observability.InstrumentModel(provider, telemetry, opts.Model.Model)
	}
	processor, err := changeset.New(changeset.Config{
		Store:         store,
		Clock:         clock,
		IDs:           ids,
		PolicyVersion: opts.Model.PolicyVersion,
	})
	if err != nil {
		return nil, fmt.Errorf("changeset processor: %w", err)
	}
	exec := &kernel.ModelExecutor{
		Store:    store,
		Clock:    clock,
		IDs:      ids,
		Provider: modelProvider,
		Changes:  processor,
		Compiler: prompt.Compiler{
			Estimator:             prompt.ConservativeEstimator{},
			ProviderContextTokens: opts.Model.ContextTokens,
		},
		PolicyVersion: opts.Model.PolicyVersion,
		LeaseTTL:      opts.Model.LeaseTTL,
	}
	return exec, nil
}
