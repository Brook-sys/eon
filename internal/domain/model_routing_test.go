package domain

import (
	"reflect"
	"testing"
	"time"
)

func routeBinding(id string, priority, contextTokens int) ModelBindingConfig {
	return ModelBindingConfig{
		ID:               id,
		ProviderRef:      "provider",
		ModelID:          "model-" + id,
		Enabled:          true,
		Priority:         priority,
		ContextTokens:    contextTokens,
		MaxOutputTokens:  64,
		MaxOutputDialect: MaxOutputDialectLegacy,
		Limit:            ResourceLimit{Resource: ModelBindingResource(id)},
	}
}

func TestSelectModelBindingOrdersAndExplainsRejections(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	openUntil := now.Add(time.Minute)
	disabled := routeBinding("disabled", 1, 4096)
	disabled.Enabled = false
	selected, decision, err := SelectModelBinding([]ModelRouteCandidate{
		{Binding: routeBinding("healthy", 40, 4096)},
		{Binding: routeBinding("small", 20, 512)},
		{Binding: disabled},
		{Binding: routeBinding("open", 30, 4096), Usage: ResourceUsage{Resource: ModelBindingResource("open"), CircuitOpenUntil: &openUntil}},
	}, 1024, now)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if selected.ID != "healthy" || decision.SelectedBindingID != "healthy" {
		t.Fatalf("selected=%q decision=%+v", selected.ID, decision)
	}
	wantOrder := []string{"disabled", "small", "open", "healthy"}
	if !reflect.DeepEqual(decision.Considered, wantOrder) {
		t.Fatalf("considered=%v want=%v", decision.Considered, wantOrder)
	}
	if decision.Rejected["disabled"] != "disabled" || decision.Rejected["small"] != "context_insufficient" || decision.Rejected["open"] != "circuit_open" {
		t.Fatalf("rejected=%v", decision.Rejected)
	}
}

func TestSelectModelBindingUsesStableIDTieBreakAndDoesNotMutateInput(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	candidates := []ModelRouteCandidate{{Binding: routeBinding("zeta", 10, 2048)}, {Binding: routeBinding("alpha", 10, 2048)}}
	selected, _, err := SelectModelBinding(candidates, 1000, now)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if selected.ID != "alpha" {
		t.Fatalf("selected=%q", selected.ID)
	}
	if candidates[0].Binding.ID != "zeta" {
		t.Fatal("input candidates were mutated")
	}
}
