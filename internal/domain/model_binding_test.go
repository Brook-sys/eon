package domain

import (
	"strings"
	"testing"
	"time"
)

func TestModelProviderAndBindingConfigValidateWithoutSecretValues(t *testing.T) {
	limit1 := ResourceLimit{Resource: "model-provider:nvidia", MaxConcurrent: 1, MaxPerMinute: 20, FailureThreshold: 2, CooldownBase: time.Second, CooldownMax: time.Minute}
	limit2 := ResourceLimit{Resource: "model-binding:nvidia-glm-5-2", MaxConcurrent: 1, MaxPerMinute: 10, FailureThreshold: 2, CooldownBase: time.Second, CooldownMax: time.Minute}
	provider := ModelProviderConfig{ID: "nvidia", Kind: ProviderKindNVIDIANIM, BaseURL: "https://integrate.api.nvidia.com/v1", APIKeyEnv: "NVIDIA_NIM_API_KEY", Timeout: 90 * time.Second, MaxResponseBytes: 1 << 20, GlobalLimit: limit1}
	if err := provider.Validate(); err != nil {
		t.Fatal(err)
	}
	binding := ModelBindingConfig{ID: "nvidia-glm-5-2", ProviderRef: provider.ID, ModelID: "z-ai/glm-5.2", Enabled: true, Priority: 10, ContextTokens: 131072, MaxOutputTokens: 8192, MaxOutputDialect: MaxOutputDialectCompletion, Limit: limit2}
	if err := binding.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := ModelProviderResource(provider.ID); got != "model-provider:nvidia" {
		t.Fatalf("resource = %q", got)
	}
	if got := ModelBindingResource(binding.ID); got != "model-binding:nvidia-glm-5-2" {
		t.Fatalf("resource = %q", got)
	}
}

func TestModelProviderConfigRejectsSecretValueAndUnsafeBindingID(t *testing.T) {
	limit1 := ResourceLimit{Resource: "model-provider:groq", MaxConcurrent: 1, CooldownBase: time.Second, CooldownMax: time.Minute}
	limit2 := ResourceLimit{Resource: "model-binding:escape", MaxConcurrent: 1, CooldownBase: time.Second, CooldownMax: time.Minute}
	provider := ModelProviderConfig{ID: "groq", Kind: ProviderKindGroq, BaseURL: "https://api.groq.com/openai/v1", APIKeyEnv: "GROQ_API_KEY=secret", Timeout: time.Second, MaxResponseBytes: 1024, GlobalLimit: limit1}
	if err := provider.Validate(); err == nil || !strings.Contains(err.Error(), "api_key_env") {
		t.Fatalf("unexpected error: %v", err)
	}
	binding := ModelBindingConfig{ID: "../escape", ProviderRef: "groq", ModelID: "openai/gpt-oss-120b", Enabled: true, ContextTokens: 100, MaxOutputTokens: 10, MaxOutputDialect: MaxOutputDialectCompletion, Limit: limit2}
	if err := binding.Validate(); err == nil {
		t.Fatal("expected unsafe id rejection")
	}
}

func TestModelsConfigValidate(t *testing.T) {
	provider := ModelProviderConfig{ID: "groq", Kind: ProviderKindGroq, BaseURL: "https://api.groq.com/openai/v1", APIKeyEnv: "GROQ_API_KEY", Timeout: time.Second, MaxResponseBytes: 1024, GlobalLimit: ResourceLimit{Resource: "model-provider:groq", MaxConcurrent: 1, CooldownBase: time.Second, CooldownMax: time.Minute}}
	binding := ModelBindingConfig{ID: "gemma2", ProviderRef: "groq", ModelID: "gemma2-9b-it", Enabled: true, Priority: 1, ContextTokens: 8192, MaxOutputTokens: 1024, MaxOutputDialect: MaxOutputDialectCompletion, Limit: ResourceLimit{Resource: "model-binding:gemma2", MaxConcurrent: 1, CooldownBase: time.Second, CooldownMax: time.Minute}}
	cfg := ModelsConfig{Version: "models.v1", Providers: []ModelProviderConfig{provider}, Bindings: []ModelBindingConfig{binding}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	cfg.Version = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected version rejection")
	}
	cfg.Version = "models.v1"
	cfg.Bindings = []ModelBindingConfig{binding, binding}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected duplicate binding rejection")
	}
	cfg.Bindings = []ModelBindingConfig{binding}
	cfg.Providers = []ModelProviderConfig{provider, provider}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected duplicate provider rejection")
	}
	cfg.Providers = nil
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing provider rejection")
	}
	cfg.Providers = []ModelProviderConfig{provider}
	cfg.Bindings[0].ProviderRef = "unknown"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected unknown provider rejection")
	}
}
