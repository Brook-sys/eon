package bootstrap

import (
	"context"
	"fmt"
	"os"
	"sort"
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
	modelOpts := opts.Model
	models, found, err := kernel.ActiveModelsConfig(context.Background(), store)
	if err != nil {
		return nil, fmt.Errorf("load active models config: %w", err)
	}
	if found {
		resolved, resolveErr := modelOptionsFromCatalog(models, opts.Model)
		if resolveErr != nil {
			return nil, resolveErr
		}
		modelOpts = resolved
	}
	if modelOpts == nil || !modelOpts.Enabled {
		return nil, nil
	}
	modelProvider, err := openModelProvider(modelOpts.BaseURL, modelOpts.Model, modelOpts.APIKeyEnv, modelOpts.MaxOutputField, modelOpts.ContextTokens, modelOpts.MaxResponseBytes, "openai-compatible", telemetry)
	if err != nil {
		return nil, fmt.Errorf("model provider: %w", err)
	}
	var fallbackProvider port.ModelProvider
	if fb := modelOpts.Fallback; fb != nil && fb.Enabled {
		// Context/field defaults are filled by Options.Validate; still defend here.
		field := fb.MaxOutputField
		if field == "" {
			field = modelOpts.MaxOutputField
		}
		ctxTokens := fb.ContextTokens
		if ctxTokens <= 0 {
			ctxTokens = modelOpts.ContextTokens
		}
		fallbackProvider, err = openModelProvider(fb.BaseURL, fb.Model, fb.APIKeyEnv, field, ctxTokens, fb.MaxResponseBytes, "openai-compatible-fallback", telemetry)
		if err != nil {
			return nil, fmt.Errorf("model fallback provider: %w", err)
		}
	}
	processor, err := changeset.New(changeset.Config{
		Store:         store,
		Clock:         clock,
		IDs:           ids,
		PolicyVersion: modelOpts.PolicyVersion,
	})
	if err != nil {
		return nil, fmt.Errorf("changeset processor: %w", err)
	}
	exec := &kernel.ModelExecutor{
		Store:            store,
		Clock:            clock,
		IDs:              ids,
		Provider:         modelProvider,
		FallbackProvider: fallbackProvider,
		Changes:          processor,
		Compiler: prompt.Compiler{
			Estimator:             prompt.ConservativeEstimator{},
			ProviderContextTokens: modelOpts.ContextTokens,
		},
		PolicyVersion: modelOpts.PolicyVersion,
		LeaseTTL:      modelOpts.LeaseTTL,
	}
	// FR-RES-001: opt-in ResourceGate + PolicyEngine for model.complete.
	// Fail-closed MVP catalog; limits default; usage is durable via store.
	authorizer, err := kernel.NewMVPCapabilityAuthorizer(store, clock, modelOpts.PolicyVersion)
	if err != nil {
		return nil, fmt.Errorf("capability authorizer: %w", err)
	}
	exec.Authorizer = authorizer
	return exec, nil
}

func modelOptionsFromCatalog(config domain.ModelsConfig, fallback *ModelOptions) (*ModelOptions, error) {
	providers := make(map[string]domain.ModelProviderConfig, len(config.Providers))
	for _, provider := range config.Providers {
		providers[provider.ID] = provider
	}
	bindings := append([]domain.ModelBindingConfig(nil), config.Bindings...)
	sort.SliceStable(bindings, func(i, j int) bool {
		if bindings[i].Priority == bindings[j].Priority {
			return bindings[i].ID < bindings[j].ID
		}
		return bindings[i].Priority < bindings[j].Priority
	})
	selected := make([]*ModelOptions, 0, 2)
	for _, binding := range bindings {
		if !binding.Enabled {
			continue
		}
		provider := providers[binding.ProviderRef]
		field := ModelMaxOutputTokensLegacy
		if binding.MaxOutputDialect == domain.MaxOutputDialectCompletion {
			field = ModelMaxOutputTokensCompletion
		}
		selected = append(selected, &ModelOptions{
			Enabled:          true,
			BaseURL:          provider.BaseURL,
			Model:            binding.ModelID,
			APIKeyEnv:        provider.APIKeyEnv,
			MaxOutputField:   field,
			ContextTokens:    binding.ContextTokens,
			PolicyVersion:    config.Version,
			LeaseTTL:         15 * time.Minute,
			MaxResponseBytes: provider.MaxResponseBytes,
		})
		if len(selected) == 2 {
			break
		}
	}
	if len(selected) == 0 {
		return nil, nil
	}
	if fallback != nil && fallback.LeaseTTL > 0 {
		selected[0].LeaseTTL = fallback.LeaseTTL
	}
	if len(selected) == 2 {
		selected[0].Fallback = &ModelFallbackOptions{
			Enabled:          true,
			BaseURL:          selected[1].BaseURL,
			Model:            selected[1].Model,
			APIKeyEnv:        selected[1].APIKeyEnv,
			MaxOutputField:   selected[1].MaxOutputField,
			ContextTokens:    selected[1].ContextTokens,
			MaxResponseBytes: selected[1].MaxResponseBytes,
		}
	}
	return selected[0], nil
}

// openModelProvider builds one OpenAI-compatible provider (+ optional OTel).
// apiKeyEnv is the env var name only — secrets never land in flags or durable config.
func openModelProvider(
	baseURL, modelName, apiKeyEnv string,
	maxField ModelMaxOutputField,
	contextTokens int,
	maxResponseBytes int64,
	profileName string,
	telemetry *observability.Runtime,
) (port.ModelProvider, error) {
	apiKey := ""
	if env := strings.TrimSpace(apiKeyEnv); env != "" {
		apiKey = strings.TrimSpace(os.Getenv(env))
		// Empty key is allowed: some local OpenAI-compatible servers ignore auth.
	}
	field := openai.MaxOutputTokensLegacy
	switch maxField {
	case ModelMaxOutputTokensCompletion:
		field = openai.MaxOutputTokensCompletion
	case ModelMaxOutputTokensLegacy, "":
		field = openai.MaxOutputTokensLegacy
	default:
		return nil, fmt.Errorf("unsupported model max-output field %q", maxField)
	}
	if strings.TrimSpace(profileName) == "" {
		profileName = "openai-compatible"
	}
	provider, err := openai.New(openai.Config{
		BaseURL:          baseURL,
		APIKey:           apiKey,
		Model:            modelName,
		MaxOutputField:   field,
		MaxResponseBytes: maxResponseBytes,
	}, openai.WithContextTokens(contextTokens), openai.WithProfileName(profileName))
	if err != nil {
		return nil, err
	}
	var modelProvider port.ModelProvider = provider
	if telemetry != nil {
		// Instrument under the model name so traces distinguish primary vs fallback.
		label := modelName
		if profileName != "" && profileName != "openai-compatible" {
			label = profileName + ":" + modelName
		}
		modelProvider = observability.InstrumentModel(provider, telemetry, label)
	}
	return modelProvider, nil
}
