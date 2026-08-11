package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
	"motor-autonomo/internal/storage/sqlite"
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

func TestModelOptionsFromCatalogBindingTimeoutOverridesProvider(t *testing.T) {
	config := domain.ModelsConfig{
		Version: "models@timeout",
		Providers: []domain.ModelProviderConfig{
			{ID: "nim", Kind: domain.ProviderKindNVIDIANIM, BaseURL: "https://integrate.api.nvidia.com/v1", APIKeyEnv: "NVIDIA_API_KEY", Timeout: 30 * time.Second, MaxResponseBytes: 2 << 20, GlobalLimit: domain.ResourceLimit{Resource: domain.ModelProviderResource("nim")}},
		},
		Bindings: []domain.ModelBindingConfig{
			// Binding with explicit timeout override (e.g., NIM cold-start needs longer).
			{ID: "cold-start", ProviderRef: "nim", ModelID: "meta/llama-3.3-70b-instruct", Enabled: true, Priority: 10, ContextTokens: 32768, MaxOutputTokens: 1024, MaxOutputDialect: domain.MaxOutputDialectCompletion, Timeout: 90 * time.Second, Limit: domain.ResourceLimit{Resource: domain.ModelBindingResource("cold-start")}},
			// Binding without timeout — should inherit provider's 30s.
			{ID: "fast", ProviderRef: "nim", ModelID: "meta/llama-3.1-8b-instruct", Enabled: true, Priority: 20, ContextTokens: 8192, MaxOutputTokens: 512, MaxOutputDialect: domain.MaxOutputDialectLegacy, Limit: domain.ResourceLimit{Resource: domain.ModelBindingResource("fast")}},
		},
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("fixture: %v", err)
	}

	got, err := modelOptionsFromCatalog(config, &ModelOptions{LeaseTTL: time.Minute})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil options")
	}
	// Primary (priority 10) has explicit 90s override.
	if got.Timeout != 90*time.Second {
		t.Fatalf("primary timeout = %v, want 90s (binding override)", got.Timeout)
	}
	// Fallback (priority 20) inherits provider's 30s.
	if got.Fallback == nil {
		t.Fatal("expected fallback")
	}
	if got.Fallback.Timeout != 30*time.Second {
		t.Fatalf("fallback timeout = %v, want 30s (provider inherit)", got.Fallback.Timeout)
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

func TestSQLiteReopenRestoresEnabledPresetAndRouter(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "runtime.db")
	preset := domain.ModelPreset{
		ID: "sqlite-smoke",
		Provider: domain.ModelProviderConfig{
			ID: "nim", Kind: domain.ProviderKindNVIDIANIM, BaseURL: "https://example.invalid/v1", APIKeyEnv: "NVIDIA_NIM_API_KEY",
			Timeout: 45 * time.Second, MaxResponseBytes: 2 << 20, GlobalLimit: domain.ResourceLimit{Resource: domain.ModelProviderResource("nim"), MaxConcurrent: 2},
		},
		Binding: domain.ModelBindingConfig{
			ID: "sqlite-smoke", ProviderRef: "nim", ModelID: "mistral-smoke", Enabled: false, Priority: 50,
			ContextTokens: 32768, MaxOutputTokens: 1024, MaxOutputDialect: domain.MaxOutputDialectCompletion,
			Limit: domain.ResourceLimit{Resource: domain.ModelBindingResource("sqlite-smoke"), MaxConcurrent: 4},
		},
		ObservedAt: time.Date(2026, 7, 18, 22, 40, 0, 0, time.UTC), Qualification: "QUALIFIED",
		EvidenceReport: "results/smoke.json", EvidenceSHA256: strings.Repeat("a", 64), RecommendedPriority: 10,
	}
	installed, err := preset.ModelsConfigDraft("models.installed.v1")
	if err != nil {
		t.Fatalf("materialize disabled preset: %v", err)
	}

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	now := time.Date(2026, 7, 18, 22, 40, 0, 0, time.UTC)
	clock := source.NewManualClock(now)
	applier, err := kernel.NewConfigApplier(store, clock, source.NewSequenceIDGenerator(1))
	if err != nil {
		t.Fatalf("new config applier: %v", err)
	}
	apply := func(id domain.ConfigDraftID, basedOn uint64, config domain.ModelsConfig) {
		t.Helper()
		draft := domain.ConfigDraft{
			SchemaVersion: domain.SchemaVersionV1, ID: id, Scope: domain.ConfigScopeModels, BasedOnRevision: basedOn,
			Applicability: domain.ConfigRestartRequired, Status: domain.ConfigDraftOpen,
			ActorType: domain.ActorOperator, ActorID: "operator_1", Reason: "review preset through normal authority path",
			Models: &config, CreatedAt: clock.Now(),
		}
		if err := store.Update(ctx, func(tx port.Transaction) error { return tx.CreateConfigDraft(draft) }); err != nil {
			t.Fatalf("create models draft: %v", err)
		}
		if _, _, err := applier.ValidateDraft(ctx, draft.ID); err != nil {
			t.Fatalf("validate models draft: %v", err)
		}
		clock.Advance(time.Second)
		if _, _, err := applier.ApplyDraft(ctx, draft.ID); err != nil {
			t.Fatalf("apply models draft: %v", err)
		}
		clock.Advance(time.Second)
	}
	apply("draft_models_install", 0, installed)
	preview, err := preset.PreviewEnablement(&installed, "models.enabled.v1")
	if err != nil {
		t.Fatalf("preview preset enablement: %v", err)
	}
	if preview.Blocked || preview.Candidate == nil {
		t.Fatalf("enablement preview blocked: %+v", preview)
	}
	apply("draft_models_enable", 1, *preview.Candidate)
	if err := store.Close(); err != nil {
		t.Fatalf("close sqlite before restart: %v", err)
	}

	rt, err := Open(ctx, Options{
		StoreBackend: StorageSQLite,
		SQLitePath:   dbPath,
		Model: &ModelOptions{
			Enabled: true, BaseURL: "http://unused.invalid/v1", Model: "unused",
			ContextTokens: 1024, MaxResponseBytes: 1024, Timeout: time.Second, LeaseTTL: time.Minute,
		},
	})
	if err != nil {
		t.Fatalf("reopen runtime: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close(context.Background()) })
	if rt.Model == nil || rt.Model.ModelsConfig == nil {
		t.Fatalf("reopened runtime did not restore models catalog: %#v", rt.Model)
	}
	if rt.Model.ModelsConfig.Version != "models.enabled.v1" {
		t.Fatalf("restored version = %q", rt.Model.ModelsConfig.Version)
	}
	if rt.Model.PrimaryProviderID != preset.Provider.ID || rt.Model.PrimaryBindingID != preset.Binding.ID {
		t.Fatalf("restored routing = provider %q binding %q", rt.Model.PrimaryProviderID, rt.Model.PrimaryBindingID)
	}
	provider := rt.Model.Providers[preset.Binding.ID]
	if provider == nil {
		t.Fatal("restored router missing preset provider")
	}
	reporter, ok := provider.(port.ModelCapabilityReporter)
	if !ok {
		t.Fatalf("restored provider does not report declared capabilities: %T", provider)
	}
	profile := reporter.DeclaredProfile()
	if profile.Name != string(preset.Provider.Kind)+":"+preset.Binding.ID || profile.Model != preset.Binding.ModelID || profile.MaxContextTokens != preset.Binding.ContextTokens || profile.MaxOutputDialect != preset.Binding.MaxOutputDialect {
		t.Fatalf("restored provider profile = %+v", profile)
	}
	for _, resource := range []domain.ResourceID{
		domain.ModelProviderResource(preset.Provider.ID), domain.ModelBindingResource(preset.Binding.ID),
	} {
		if _, ok := rt.Model.Authorizer.Limits[resource]; !ok {
			t.Errorf("restored authorizer missing limit for %s", resource)
		}
	}
	var reqCap domain.RequiredCapability
	profilesMap := make(map[string]domain.ModelCapabilityProfile)
	selected, decision, err := kernel.SelectModelBinding(ctx, rt.Store, *rt.Model.ModelsConfig, 128, reqCap, profilesMap, clock.Now())
	if err != nil {
		t.Fatalf("select restored binding: %v", err)
	}
	if selected.ID != preset.Binding.ID || decision.SelectedProviderID != preset.Provider.ID {
		t.Fatalf("restored routing decision = %+v / %+v", selected, decision)
	}
}

type fixedSecretResolver map[string]string

func (r fixedSecretResolver) Resolve(name string) (string, error) { return r[name], nil }

type failingSecretResolver struct{ err error }

func (r failingSecretResolver) Resolve(string) (string, error) { return "", r.err }

func TestOpenModelProviderUsesResolverOnlyWhenEnvironmentEmpty(t *testing.T) {
	const name = "MODEL_RESOLVER_TEST_KEY"
	t.Setenv(name, "")
	p, err := openModelProvider("http://localhost", "fixture", name, fixedSecretResolver{name: "vault-secret"}, ModelMaxOutputTokensLegacy, 1024, 1024, time.Second, "test", nil)
	if err != nil || p == nil {
		t.Fatalf("resolver provider: provider=%v err=%v", p, err)
	}

	t.Setenv(name, "env-secret")
	p, err = openModelProvider("http://localhost", "fixture", name, fixedSecretResolver{name: "vault-secret"}, ModelMaxOutputTokensLegacy, 1024, 1024, time.Second, "test", nil)
	if err != nil || p == nil {
		t.Fatalf("environment provider: provider=%v err=%v", p, err)
	}
}

func TestOpenModelProviderRedactsCredentialReferenceOnResolverFailure(t *testing.T) {
	const name = "SENSITIVE_CREDENTIAL_REFERENCE"
	t.Setenv(name, "")
	sentinel := errors.New("vault locked")
	_, err := openModelProvider("http://localhost", "fixture", name, failingSecretResolver{err: sentinel}, ModelMaxOutputTokensLegacy, 1024, 1024, time.Second, "test", nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("resolver error = %v, want wrapped sentinel", err)
	}
	if strings.Contains(err.Error(), name) {
		t.Fatalf("resolver error leaked credential reference: %v", err)
	}
}
