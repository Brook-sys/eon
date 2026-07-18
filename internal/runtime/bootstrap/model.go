package bootstrap

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"motor-autonomo/internal/changeset"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/observability"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/provider/openai"
	"motor-autonomo/internal/runtime/source"
)

// BuildModelExecutor assembles the runtime's optional PROPOSE_ONLY executor.
// It is exported for bounded operator campaigns that must exercise the exact
// same catalog, changeset, and ResourceGate wiring as the runtime process.
func BuildModelExecutor(
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
	var modelProvider, fallbackProvider port.ModelProvider
	providersByBinding := map[string]port.ModelProvider{}
	if found {
		providerConfigs := make(map[string]domain.ModelProviderConfig, len(models.Providers))
		for _, provider := range models.Providers {
			providerConfigs[provider.ID] = provider
		}
		for _, binding := range models.Bindings {
			if !binding.Enabled {
				continue
			}
			provider := providerConfigs[binding.ProviderRef]
			field := ModelMaxOutputTokensLegacy
			if binding.MaxOutputDialect == domain.MaxOutputDialectCompletion {
				field = ModelMaxOutputTokensCompletion
			}
			instance, openErr := openModelProvider(provider.BaseURL, binding.ModelID, provider.APIKeyEnv, field, binding.ContextTokens, provider.MaxResponseBytes, provider.Timeout, string(provider.Kind)+":"+binding.ID, telemetry)
			if openErr != nil {
				return nil, fmt.Errorf("model binding %s provider: %w", binding.ID, openErr)
			}
			providersByBinding[binding.ID] = instance
		}
	} else {
		modelProvider, err = openModelProvider(modelOpts.BaseURL, modelOpts.Model, modelOpts.APIKeyEnv, modelOpts.MaxOutputField, modelOpts.ContextTokens, modelOpts.MaxResponseBytes, modelOpts.Timeout, "openai-compatible", telemetry)
		if err != nil {
			return nil, fmt.Errorf("model provider: %w", err)
		}
		if fb := modelOpts.Fallback; fb != nil && fb.Enabled {
			field := fb.MaxOutputField
			if field == "" {
				field = modelOpts.MaxOutputField
			}
			ctxTokens := fb.ContextTokens
			if ctxTokens <= 0 {
				ctxTokens = modelOpts.ContextTokens
			}
			fallbackProvider, err = openModelProvider(fb.BaseURL, fb.Model, fb.APIKeyEnv, field, ctxTokens, fb.MaxResponseBytes, fb.Timeout, "openai-compatible-fallback", telemetry)
			if err != nil {
				return nil, fmt.Errorf("model fallback provider: %w", err)
			}
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
		Providers:        providersByBinding,
		Changes:          processor,
		Compiler: prompt.Compiler{
			Estimator:             prompt.ConservativeEstimator{},
			ProviderContextTokens: modelOpts.ContextTokens,
		},
		PolicyVersion:     modelOpts.PolicyVersion,
		LeaseTTL:          modelOpts.LeaseTTL,
		PrimaryProviderID: modelOpts.ProviderID,
		PrimaryBindingID:  modelOpts.BindingID,
	}
	if found {
		exec.ModelsConfig = &models
	}
	if fb := modelOpts.Fallback; fb != nil {
		exec.FallbackProviderID = fb.ProviderID
		exec.FallbackBindingID = fb.BindingID
	}
	// FR-RES-001: opt-in ResourceGate + PolicyEngine for model.complete.
	// Fail-closed MVP catalog; limits default; usage is durable via store.
	authorizer, err := kernel.NewMVPCapabilityAuthorizer(store, clock, modelOpts.PolicyVersion)
	if err != nil {
		return nil, fmt.Errorf("capability authorizer: %w", err)
	}
	if found {
		for _, provider := range models.Providers {
			if provider.GlobalLimit.Resource != "" {
				authorizer.Limits[provider.GlobalLimit.Resource] = provider.GlobalLimit
			}
		}
		for _, binding := range models.Bindings {
			if binding.Limit.Resource != "" {
				authorizer.Limits[binding.Limit.Resource] = binding.Limit
			}
		}
	} else {
		if modelOpts.ProviderLimit.Resource != "" {
			authorizer.Limits[modelOpts.ProviderLimit.Resource] = modelOpts.ProviderLimit
		}
		if modelOpts.BindingLimit.Resource != "" {
			authorizer.Limits[modelOpts.BindingLimit.Resource] = modelOpts.BindingLimit
		}
		if fb := modelOpts.Fallback; fb != nil {
			if fb.ProviderLimit.Resource != "" {
				authorizer.Limits[fb.ProviderLimit.Resource] = fb.ProviderLimit
			}
			if fb.BindingLimit.Resource != "" {
				authorizer.Limits[fb.BindingLimit.Resource] = fb.BindingLimit
			}
		}
	}
	exec.Authorizer = authorizer
	return exec, nil
}

// buildModel keeps the package-local name used by older focused tests.
func buildModel(opts Options, store port.Store, clock source.Clock, ids source.IDGenerator, telemetry *observability.Runtime) (*kernel.ModelExecutor, error) {
	return BuildModelExecutor(opts, store, clock, ids, telemetry)
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
			ProviderID:       provider.ID,
			BindingID:        binding.ID,
			ProviderLimit:    provider.GlobalLimit,
			BindingLimit:     binding.Limit,
			BaseURL:          provider.BaseURL,
			Model:            binding.ModelID,
			APIKeyEnv:        provider.APIKeyEnv,
			MaxOutputField:   field,
			ContextTokens:    binding.ContextTokens,
			PolicyVersion:    config.Version,
			LeaseTTL:         15 * time.Minute,
			MaxResponseBytes: provider.MaxResponseBytes,
			Timeout:          provider.Timeout,
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
			ProviderID:       selected[1].ProviderID,
			BindingID:        selected[1].BindingID,
			ProviderLimit:    selected[1].ProviderLimit,
			BindingLimit:     selected[1].BindingLimit,
			BaseURL:          selected[1].BaseURL,
			Model:            selected[1].Model,
			APIKeyEnv:        selected[1].APIKeyEnv,
			MaxOutputField:   selected[1].MaxOutputField,
			ContextTokens:    selected[1].ContextTokens,
			MaxResponseBytes: selected[1].MaxResponseBytes,
			Timeout:          selected[1].Timeout,
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
	timeout time.Duration,
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
		Timeout:          timeout,
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
