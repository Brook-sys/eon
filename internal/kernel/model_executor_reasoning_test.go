package kernel

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/prompt"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func TestModelExecutorResolveBindingReasoning(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	store := memory.New()

	modelsConfig := domain.ModelsConfig{
		Version: "models@reasoning-test",
		Providers: []domain.ModelProviderConfig{
			{ID: "groq", Kind: domain.ProviderKindGroq, BaseURL: "https://api.groq.com/openai/v1", APIKeyEnv: "GROQ_API_KEY", Timeout: 30 * time.Second, MaxResponseBytes: 1 << 20, GlobalLimit: domain.ResourceLimit{Resource: domain.ModelProviderResource("groq")}},
		},
		Bindings: []domain.ModelBindingConfig{
			{ID: "qwen36-effort", ProviderRef: "groq", ModelID: "qwen/qwen3.6-27b", Enabled: true, Priority: 10, ContextTokens: 32768, MaxOutputTokens: 1024, MaxOutputDialect: domain.MaxOutputDialectCompletion, ReasoningEffort: "medium", ReasoningFormat: "parsed", Limit: domain.ResourceLimit{Resource: domain.ModelBindingResource("qwen36-effort")}},
			{ID: "gptoss-hidden", ProviderRef: "groq", ModelID: "openai/gpt-oss-20b", Enabled: true, Priority: 20, ContextTokens: 131072, MaxOutputTokens: 2048, MaxOutputDialect: domain.MaxOutputDialectCompletion, ReasoningEffort: "low", ReasoningFormat: "hidden", Limit: domain.ResourceLimit{Resource: domain.ModelBindingResource("gptoss-hidden")}},
			{ID: "llama-inherit", ProviderRef: "groq", ModelID: "llama-3.1-8b-instant", Enabled: true, Priority: 30, ContextTokens: 8192, MaxOutputTokens: 512, MaxOutputDialect: domain.MaxOutputDialectLegacy, Limit: domain.ResourceLimit{Resource: domain.ModelBindingResource("llama-inherit")}},
		},
	}
	if err := modelsConfig.Validate(); err != nil {
		t.Fatalf("fixture config invalid: %v", err)
	}
	if err := store.SetActiveConfig(ctx, domain.ConfigScopeModels, modelsConfig.Version, &modelsConfig); err != nil {
		t.Fatalf("set active config: %v", err)
	}

	exec := ModelExecutor{
		Store:         store,
		Clock:         clock,
		IDs:           ids,
		Provider:      &fakeProvider{},
		PolicyVersion: "policy@test",
		LeaseTTL:      5 * time.Minute,
		ModelsConfig:  &modelsConfig,
		Compiler: prompt.Compiler{
			Estimator:             prompt.ConservativeEstimator{},
			ProviderContextTokens: 8000,
		},
	}

	effort, format := exec.resolveBindingReasoning("qwen36-effort")
	if effort != "medium" {
		t.Fatalf("qwen36-effort: expected effort=medium, got %q", effort)
	}
	if format != "parsed" {
		t.Fatalf("qwen36-effort: expected format=parsed, got %q", format)
	}

	effort, format = exec.resolveBindingReasoning("gptoss-hidden")
	if effort != "low" {
		t.Fatalf("gptoss-hidden: expected effort=low, got %q", effort)
	}
	if format != "hidden" {
		t.Fatalf("gptoss-hidden: expected format=hidden, got %q", format)
	}

	effort, format = exec.resolveBindingReasoning("llama-inherit")
	if effort != "" {
		t.Fatalf("llama-inherit: expected empty effort, got %q", effort)
	}
	if format != "" {
		t.Fatalf("llama-inherit: expected empty format, got %q", format)
	}

	effort, format = exec.resolveBindingReasoning("unknown")
	if effort != "" || format != "" {
		t.Fatalf("unknown binding: expected empty, got effort=%q format=%q", effort, format)
	}

	exec2 := ModelExecutor{Store: store, Clock: clock, IDs: ids, Provider: &fakeProvider{}, ModelsConfig: nil}
	effort, format = exec2.resolveBindingReasoning("any")
	if effort != "" || format != "" {
		t.Fatalf("nil ModelsConfig: expected empty, got effort=%q format=%q", effort, format)
	}
}

// fakeProvider is a minimal Provider implementation for unit tests.
type fakeProvider struct{}

func (f *fakeProvider) Kind() domain.ProviderKind {
	return domain.ProviderKindGroq
}

func (f *fakeProvider) Complete(ctx context.Context, request port.CompletionRequest) (port.CompletionResult, error) {
	return port.CompletionResult{Text: "OK", FinishReason: port.CompletionFinishStop, InputTokens: 10, OutputTokens: 5}, nil
}

func (f *fakeProvider) Models(ctx context.Context) ([]string, error) {
	return []string{"test"}, nil
}

func (f *fakeProvider) Close() error {
	return nil
}