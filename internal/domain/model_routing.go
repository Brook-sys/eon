package domain

import (
	"errors"
	"sort"
	"time"
)

// ModelRouteCandidate is a non-secret routing projection. Availability is
// derived from durable ResourceGate state; it never grants capability itself.
type ModelRouteCandidate struct {
	Binding       ModelBindingConfig `json:"binding"`
	BindingUsage  ResourceUsage      `json:"binding_usage"`
	ProviderUsage ResourceUsage      `json:"provider_usage"`
}

// ModelRouteDecision records deterministic selection and bounded rejection
// reasons suitable for audit events and inspect projections.
type ModelRouteDecision struct {
	SelectedProviderID string            `json:"selected_provider_id,omitempty"`
	SelectedBindingID  string            `json:"selected_binding_id,omitempty"`
	Considered         []string          `json:"considered"`
	Rejected           map[string]string `json:"rejected,omitempty"`
}

// SelectModelBinding orders candidates by configured priority and stable ID,
// then filters disabled, insufficient-context and circuit-open bindings. Rate
// reservations remain the ResourceGate's authority immediately before use.
func SelectModelBinding(candidates []ModelRouteCandidate, requiredTokens int, now time.Time) (ModelBindingConfig, ModelRouteDecision, error) {
	decision := ModelRouteDecision{Rejected: make(map[string]string)}
	if requiredTokens <= 0 {
		return ModelBindingConfig{}, decision, errors.New("required tokens must be positive")
	}
	if now.IsZero() {
		return ModelBindingConfig{}, decision, errors.New("routing requires now")
	}

	ordered := append([]ModelRouteCandidate(nil), candidates...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Binding.Priority == ordered[j].Binding.Priority {
			return ordered[i].Binding.ID < ordered[j].Binding.ID
		}
		return ordered[i].Binding.Priority < ordered[j].Binding.Priority
	})

	for _, candidate := range ordered {
		binding := candidate.Binding
		decision.Considered = append(decision.Considered, binding.ID)
		if err := binding.Validate(); err != nil {
			decision.Rejected[binding.ID] = "invalid_binding"
			continue
		}
		if !binding.Enabled {
			decision.Rejected[binding.ID] = "disabled"
			continue
		}
		if requiredTokens > binding.ContextTokens {
			decision.Rejected[binding.ID] = "context_insufficient"
			continue
		}
		if candidate.ProviderUsage.CircuitOpenUntil != nil && now.Before(candidate.ProviderUsage.CircuitOpenUntil.UTC()) {
			decision.Rejected[binding.ID] = "provider_circuit_open"
			continue
		}
		if candidate.BindingUsage.CircuitOpenUntil != nil && now.Before(candidate.BindingUsage.CircuitOpenUntil.UTC()) {
			decision.Rejected[binding.ID] = "circuit_open"
			continue
		}
		decision.SelectedBindingID = binding.ID
		decision.SelectedProviderID = binding.ProviderRef
		return binding, decision, nil
	}
	return ModelBindingConfig{}, decision, errors.New("no eligible model binding")
}
