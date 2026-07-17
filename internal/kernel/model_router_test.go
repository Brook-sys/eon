package kernel

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

func TestSelectModelBindingReadsUsageAndRoutes(t *testing.T) {
	store := memory.New()
	ctx := context.Background()
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	config := domain.ModelsConfig{
		Version: "v1",
		Providers: []domain.ModelProviderConfig{
			{ID: "p", Kind: domain.ProviderKindOpenAICompatible, BaseURL: "http://example", APIKeyEnv: "KEY", Timeout: time.Second, MaxResponseBytes: 1024, GlobalLimit: domain.ResourceLimit{Resource: "model-provider:p"}},
		},
		Bindings: []domain.ModelBindingConfig{
			{ID: "b1", ProviderRef: "p", ModelID: "m1", Enabled: true, Priority: 10, ContextTokens: 2048, MaxOutputTokens: 100, MaxOutputDialect: domain.MaxOutputDialectLegacy, Limit: domain.ResourceLimit{Resource: "model-binding:b1"}},
			{ID: "b2", ProviderRef: "p", ModelID: "m2", Enabled: true, Priority: 20, ContextTokens: 2048, MaxOutputTokens: 100, MaxOutputDialect: domain.MaxOutputDialectLegacy, Limit: domain.ResourceLimit{Resource: "model-binding:b2"}},
		},
	}
	openUntil := now.Add(time.Minute)
	err := store.Update(ctx, func(w port.Transaction) error {
		return w.SaveResourceUsage(domain.ResourceUsage{Resource: "model-binding:b1", CircuitOpenUntil: &openUntil})
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	selected, decision, err := SelectModelBinding(ctx, store, config, 1024, now)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if selected.ID != "b2" {
		t.Fatalf("expected b2, got %q", selected.ID)
	}
	if decision.Rejected["b1"] != "circuit_open" {
		t.Fatalf("b1 should be rejected due to circuit open, got %+v", decision)
	}
}

func TestSelectModelBindingSkipsProviderCircuitOpen(t *testing.T) {
	store := memory.New()
	ctx := context.Background()
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	limit := func(resource domain.ResourceID) domain.ResourceLimit { return domain.ResourceLimit{Resource: resource} }
	config := domain.ModelsConfig{
		Version: "v1",
		Providers: []domain.ModelProviderConfig{
			{ID: "p1", Kind: domain.ProviderKindNVIDIANIM, BaseURL: "http://example", APIKeyEnv: "KEY1", Timeout: time.Second, MaxResponseBytes: 1024, GlobalLimit: limit(domain.ModelProviderResource("p1"))},
			{ID: "p2", Kind: domain.ProviderKindGroq, BaseURL: "http://example", APIKeyEnv: "KEY2", Timeout: time.Second, MaxResponseBytes: 1024, GlobalLimit: limit(domain.ModelProviderResource("p2"))},
		},
		Bindings: []domain.ModelBindingConfig{
			{ID: "b1", ProviderRef: "p1", ModelID: "m1", Enabled: true, Priority: 10, ContextTokens: 2048, MaxOutputTokens: 100, MaxOutputDialect: domain.MaxOutputDialectLegacy, Limit: limit(domain.ModelBindingResource("b1"))},
			{ID: "b2", ProviderRef: "p2", ModelID: "m2", Enabled: true, Priority: 20, ContextTokens: 2048, MaxOutputTokens: 100, MaxOutputDialect: domain.MaxOutputDialectLegacy, Limit: limit(domain.ModelBindingResource("b2"))},
		},
	}
	openUntil := now.Add(time.Minute)
	if err := store.Update(ctx, func(w port.Transaction) error {
		return w.SaveResourceUsage(domain.ResourceUsage{Resource: domain.ModelProviderResource("p1"), CircuitOpenUntil: &openUntil})
	}); err != nil {
		t.Fatal(err)
	}
	selected, decision, err := SelectModelBinding(ctx, store, config, 1024, now)
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != "b2" || decision.Rejected["b1"] != "provider_circuit_open" {
		t.Fatalf("provider-open route = %q, %+v", selected.ID, decision)
	}
	if decision.SelectedProviderID != "p2" {
		t.Fatalf("selected provider = %q", decision.SelectedProviderID)
	}
}
