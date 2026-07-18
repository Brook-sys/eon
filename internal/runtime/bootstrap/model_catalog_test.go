package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func TestLoadModelPresetCatalogVerifiesEvidenceDigest(t *testing.T) {
	dir := t.TempDir()
	evidence := []byte(`{"qualified":true}`)
	if err := os.MkdirAll(filepath.Join(dir, "results"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "results", "report.json"), evidence, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(evidence)
	preset := domain.ModelPreset{
		ID:         "qualified-model",
		Provider:   domain.ModelProviderConfig{ID: "groq", Kind: domain.ProviderKindGroq, BaseURL: "https://api.groq.com/openai/v1", APIKeyEnv: "GROQ_API_KEY", Timeout: time.Second, MaxResponseBytes: 1024, GlobalLimit: domain.ResourceLimit{Resource: domain.ModelProviderResource("groq"), MaxConcurrent: 1}},
		Binding:    domain.ModelBindingConfig{ID: "qualified-model", ProviderRef: "groq", ModelID: "model", Enabled: false, Priority: 10, ContextTokens: 2048, MaxOutputTokens: 128, MaxOutputDialect: domain.MaxOutputDialectLegacy, Limit: domain.ResourceLimit{Resource: domain.ModelBindingResource("qualified-model"), MaxConcurrent: 1}},
		ObservedAt: time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC), Qualification: "QUALIFIED", EvidenceReport: "results/report.json", EvidenceSHA256: hex.EncodeToString(digest[:]), RecommendedPriority: 10,
	}
	catalog := domain.ModelPresetCatalog{Schema: domain.ModelPresetCatalogSchema, Presets: []domain.ModelPreset{preset}}
	body, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "catalog.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadModelPresetCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Presets) != 1 || loaded.Presets[0].ID != preset.ID {
		t.Fatalf("loaded = %#v", loaded)
	}
	if err := os.WriteFile(filepath.Join(dir, "results", "report.json"), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadModelPresetCatalog(path); err == nil {
		t.Fatal("expected evidence digest mismatch")
	}
}

func TestBuildModelWiresAllBindingsAndLimits(t *testing.T) {
	config := domain.ModelsConfig{
		Version: "models@1",
		Providers: []domain.ModelProviderConfig{
			{ID: "p1", Kind: domain.ProviderKindOpenAICompatible, BaseURL: "http://p1", APIKeyEnv: "K1", Timeout: time.Second, MaxResponseBytes: 1024, GlobalLimit: domain.ResourceLimit{Resource: domain.ModelProviderResource("p1"), MaxConcurrent: 1}},
			{ID: "p2", Kind: domain.ProviderKindOpenAICompatible, BaseURL: "http://p2", APIKeyEnv: "K2", Timeout: time.Second, MaxResponseBytes: 1024, GlobalLimit: domain.ResourceLimit{Resource: domain.ModelProviderResource("p2"), MaxConcurrent: 2}},
		},
		Bindings: []domain.ModelBindingConfig{
			{ID: "b1", ProviderRef: "p1", ModelID: "m1", Enabled: true, Priority: 10, ContextTokens: 1000, MaxOutputTokens: 100, MaxOutputDialect: domain.MaxOutputDialectLegacy, Limit: domain.ResourceLimit{Resource: domain.ModelBindingResource("b1"), MaxConcurrent: 10}},
			{ID: "b2", ProviderRef: "p2", ModelID: "m2", Enabled: true, Priority: 20, ContextTokens: 2000, MaxOutputTokens: 200, MaxOutputDialect: domain.MaxOutputDialectLegacy, Limit: domain.ResourceLimit{Resource: domain.ModelBindingResource("b2"), MaxConcurrent: 20}},
			{ID: "b3", ProviderRef: "p1", ModelID: "m3", Enabled: true, Priority: 30, ContextTokens: 3000, MaxOutputTokens: 300, MaxOutputDialect: domain.MaxOutputDialectLegacy, Limit: domain.ResourceLimit{Resource: domain.ModelBindingResource("b3"), MaxConcurrent: 30}},
		},
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("fixture config invalid: %v", err)
	}
	store := memory.New()
	if err := store.SetActiveConfig(context.Background(), domain.ConfigScopeModels, config.Version, &config); err != nil {
		t.Fatalf("set active config: %v", err)
	}

	exec, err := buildModel(Options{
		Model: &ModelOptions{Enabled: true, PolicyVersion: "fallback-policy"},
	}, store, source.SystemClock{}, source.NewSequenceIDGenerator(1), nil)

	if err != nil {
		t.Fatalf("buildModel failed: %v", err)
	}
	if exec == nil {
		t.Fatalf("buildModel returned nil")
	}
	if exec.ModelsConfig == nil {
		t.Fatalf("ModelsConfig not wired")
	}
	if len(exec.Providers) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(exec.Providers))
	}
	for _, id := range []string{"b1", "b2", "b3"} {
		if exec.Providers[id] == nil {
			t.Errorf("missing provider for binding %s", id)
		}
	}
	if exec.Authorizer == nil {
		t.Fatalf("authorizer not wired")
	}
	for _, res := range []string{"model-provider:p1", "model-provider:p2", "model-binding:b1", "model-binding:b2", "model-binding:b3"} {
		if _, ok := exec.Authorizer.Limits[domain.ResourceID(res)]; !ok {
			t.Errorf("missing limit for %s", res)
		}
	}
}

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
