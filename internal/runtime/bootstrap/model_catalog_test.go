package bootstrap

import (
	"testing"
	"time"

	"motor-autonomo/internal/domain"
)

func TestModelOptionsFromCatalogSelectsPriorityAndFallback(t *testing.T) {
	config := domain.ModelsConfig{
		Version: "models@7",
		Providers: []domain.ModelProviderConfig{
			{ID: "nim", Kind: domain.ProviderKindNVIDIANIM, BaseURL: "https://integrate.api.nvidia.com/v1", APIKeyEnv: "NVIDIA_API_KEY", Timeout: 45 * time.Second, MaxResponseBytes: 2 << 20, GlobalLimit: domain.ResourceLimit{Resource: domain.ModelProviderResource("nim")}},
			{ID: "groq", Kind: domain.ProviderKindGroq, BaseURL: "https://api.groq.com/openai/v1", APIKeyEnv: "GROQ_API_KEY", Timeout: 30 * time.Second, MaxResponseBytes: 1 << 20, GlobalLimit: domain.ResourceLimit{Resource: domain.ModelProviderResource("groq")}},
		},
		Bindings: []domain.ModelBindingConfig{
			{ID: "fallback", ProviderRef: "nim", ModelID: "meta/llama", Enabled: true, Priority: 20, ContextTokens: 32768, MaxOutputTokens: 1024, MaxOutputDialect: domain.MaxOutputDialectCompletion, Limit: domain.ResourceLimit{Resource: domain.ModelBindingResource("fallback")}},
			{ID: "primary", ProviderRef: "groq", ModelID: "llama-fast", Enabled: true, Priority: 10, ContextTokens: 8192, MaxOutputTokens: 512, MaxOutputDialect: domain.MaxOutputDialectLegacy, Limit: domain.ResourceLimit{Resource: domain.ModelBindingResource("primary")}},
		},
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("fixture: %v", err)
	}

	got, err := modelOptionsFromCatalog(config, &ModelOptions{LeaseTTL: time.Minute})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got == nil || got.Model != "llama-fast" || got.APIKeyEnv != "GROQ_API_KEY" {
		t.Fatalf("primary = %+v", got)
	}
	if got.Timeout != 30*time.Second || got.PolicyVersion != "models@7" || got.LeaseTTL != time.Minute {
		t.Fatalf("primary metadata = %+v", got)
	}
	if got.Fallback == nil || got.Fallback.Model != "meta/llama" || got.Fallback.Timeout != 45*time.Second {
		t.Fatalf("fallback = %+v", got.Fallback)
	}
	if got.Fallback.MaxOutputField != ModelMaxOutputTokensCompletion {
		t.Fatalf("fallback dialect = %q", got.Fallback.MaxOutputField)
	}
}

func TestModelOptionsFromCatalogWithNoEnabledBindingsDisablesModel(t *testing.T) {
	got, err := modelOptionsFromCatalog(domain.ModelsConfig{Version: "models@empty"}, nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil options, got %+v", got)
	}
}
