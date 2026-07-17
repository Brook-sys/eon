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
	modelProvider, err := openModelProvider(opts.Model.BaseURL, opts.Model.Model, opts.Model.APIKeyEnv, opts.Model.MaxOutputField, opts.Model.ContextTokens, opts.Model.MaxResponseBytes, "openai-compatible", telemetry)
	if err != nil {
		return nil, fmt.Errorf("model provider: %w", err)
	}
	var fallbackProvider port.ModelProvider
	if fb := opts.Model.Fallback; fb != nil && fb.Enabled {
		// Context/field defaults are filled by Options.Validate; still defend here.
		field := fb.MaxOutputField
		if field == "" {
			field = opts.Model.MaxOutputField
		}
		ctxTokens := fb.ContextTokens
		if ctxTokens <= 0 {
			ctxTokens = opts.Model.ContextTokens
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
		PolicyVersion: opts.Model.PolicyVersion,
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
			ProviderContextTokens: opts.Model.ContextTokens,
		},
		PolicyVersion: opts.Model.PolicyVersion,
		LeaseTTL:      opts.Model.LeaseTTL,
	}
	// FR-RES-001: opt-in ResourceGate + PolicyEngine for model.complete.
	// Fail-closed MVP catalog; limits default; usage is durable via store.
	authorizer, err := kernel.NewMVPCapabilityAuthorizer(store, clock, opts.Model.PolicyVersion)
	if err != nil {
		return nil, fmt.Errorf("capability authorizer: %w", err)
	}
	exec.Authorizer = authorizer
	return exec, nil
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
