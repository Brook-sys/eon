package domain

import (
	"testing"
	"time"
)

func TestSelectSkilledModelBinding(t *testing.T) {
	now := time.Date(2026, 7, 19, 14, 0, 0, 0, time.UTC)

	limit := ResourceLimit{Resource: "model-binding:test", MaxPerMinute: 10, MaxTokensPerMinute: 1000}

	candidates := []ModelRouteCandidate{
		{
			Binding: ModelBindingConfig{ID: "weak-fast", ProviderRef: "groq", ModelID: "llama", Enabled: true, ContextTokens: 8000, MaxOutputTokens: 1024, Priority: 1, MaxOutputDialect: MaxOutputDialectLegacy, Limit: limit},
		},
		{
			Binding: ModelBindingConfig{ID: "strong-slow", ProviderRef: "nvidia", ModelID: "mistral", Enabled: true, ContextTokens: 32000, MaxOutputTokens: 4096, Priority: 2, MaxOutputDialect: MaxOutputDialectCompletion, Limit: limit},
		},
	}

	profiles := map[string]ModelCapabilityProfile{
		"weak-fast": {
			SkillScores:      map[string]int{"EXTRACT": 90, "CONFLICT": 10},
			SyntaxCompliance: map[string]int{"JSON": 30},
		},
		"strong-slow": {
			SkillScores:      map[string]int{"EXTRACT": 85, "CONFLICT": 95},
			SyntaxCompliance: map[string]int{"JSON": 100},
		},
	}

	tests := []struct {
		name       string
		req        RequiredCapability
		expectedID string
	}{
		{
			name:       "Simple EXTRACT prefers fast model because it scored higher (90 vs 85)",
			req:        RequiredCapability{OperationGroup: "EXTRACT"},
			expectedID: "weak-fast",
		},
		{
			name:       "Complex CONFLICT routes to strong model (95 vs 10)",
			req:        RequiredCapability{OperationGroup: "CONFLICT"},
			expectedID: "strong-slow",
		},
		{
			name:       "Strict JSON requirement shifts weight to strong model",
			req:        RequiredCapability{OperationGroup: "EXTRACT", Format: "JSON"},
			expectedID: "strong-slow",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binding, _, err := SelectSkilledModelBinding(candidates, profiles, tt.req, 2048, now)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if binding.ID != tt.expectedID {
				t.Errorf("expected %q, got %q", tt.expectedID, binding.ID)
			}
		})
	}
}

func TestSelectSkilledModelBinding_CircuitBreaker(t *testing.T) {
	now := time.Date(2026, 7, 19, 14, 0, 0, 0, time.UTC)
	future := now.Add(1 * time.Minute)

	limit := ResourceLimit{Resource: "model-binding:test", MaxPerMinute: 10, MaxTokensPerMinute: 1000}

	candidates := []ModelRouteCandidate{
		{
			Binding:      ModelBindingConfig{ID: "strong", ProviderRef: "groq", ModelID: "llama", Enabled: true, ContextTokens: 8000, MaxOutputTokens: 1024, MaxOutputDialect: MaxOutputDialectLegacy, Limit: limit},
			BindingUsage: ResourceUsage{CircuitOpenUntil: &future}, // OVERLOADED/429!
		},
		{
			Binding: ModelBindingConfig{ID: "weak-fallback", ProviderRef: "nvidia", ModelID: "mistral", Enabled: true, ContextTokens: 8000, MaxOutputTokens: 1024, MaxOutputDialect: MaxOutputDialectCompletion, Limit: limit},
		},
	}

	profiles := map[string]ModelCapabilityProfile{
		"strong":        {SkillScores: map[string]int{"CONFLICT": 100}},
		"weak-fallback": {SkillScores: map[string]int{"CONFLICT": 40}},
	}

	binding, _, err := SelectSkilledModelBinding(candidates, profiles, RequiredCapability{OperationGroup: "CONFLICT"}, 2048, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if binding.ID != "weak-fallback" {
		t.Errorf("expected weak-fallback (failover), got %q", binding.ID)
	}
}
