package domain

import (
	"errors"
	"sort"
	"time"
)

// RequiredCapability indicates the skill profile an operation requires.
type RequiredCapability struct {
	OperationGroup string // e.g., "EXTRACT", "CONFLICT"
	Format         string // e.g., "JSON", "DELIMITED"
}

// SelectSkilledModelBinding applies skill-based routing (ADR-0005) over base routing rules.
// It maps candidates against their known profiles, scoring them based on observed capability.
// Unprofiled candidates are treated as baseline but deprioritized compared to known strong models.
func SelectSkilledModelBinding(
	candidates []ModelRouteCandidate,
	profiles map[string]ModelCapabilityProfile, // Keyed by Binding.ID
	req RequiredCapability,
	requiredTokens int,
	now time.Time,
) (ModelBindingConfig, ModelRouteDecision, error) {

	decision := ModelRouteDecision{Rejected: make(map[string]string)}
	if requiredTokens <= 0 {
		return ModelBindingConfig{}, decision, errors.New("required tokens must be positive")
	}
	if now.IsZero() {
		return ModelBindingConfig{}, decision, errors.New("routing requires now")
	}

	// Filter viable candidates using core gate logic first
	var viable []ModelRouteCandidate
	for _, candidate := range candidates {
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
		viable = append(viable, candidate)
	}

	if len(viable) == 0 {
		return ModelBindingConfig{}, decision, errors.New("no eligible model binding")
	}

	// Score viable candidates
	type scoredCandidate struct {
		Candidate ModelRouteCandidate
		Score     int
	}

	var scored []scoredCandidate
	for _, v := range viable {
		score := 0
		profile, exists := profiles[v.Binding.ID]
		if exists {
			// Add observed skill score
			if req.OperationGroup != "" && profile.SkillScores != nil {
				score += profile.SkillScores[req.OperationGroup]
			}
			// Add syntax compliance weight
			if req.Format != "" && profile.SyntaxCompliance != nil {
				score += profile.SyntaxCompliance[req.Format]
			}
		} else {
			// Baseline score for unprofiled to allow discovery, but low enough
			// not to preempt proven models.
			score = 10
		}
		scored = append(scored, scoredCandidate{Candidate: v, Score: score})
	}

	// Sort by Score (descending), then Priority (ascending), then ID
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score == scored[j].Score {
			if scored[i].Candidate.Binding.Priority == scored[j].Candidate.Binding.Priority {
				return scored[i].Candidate.Binding.ID < scored[j].Candidate.Binding.ID
			}
			return scored[i].Candidate.Binding.Priority < scored[j].Candidate.Binding.Priority
		}
		return scored[i].Score > scored[j].Score // Highest score wins
	})

	winner := scored[0].Candidate.Binding
	decision.SelectedBindingID = winner.ID
	decision.SelectedProviderID = winner.ProviderRef

	return winner, decision, nil
}
