package domain

import (
	"fmt"
	"time"
)

// ModelCapabilityProfile registers a persistent evaluation of a model's cognitive 
// and operational capabilities. This profile dictates the routing boundaries (ADR-0005).
type ModelCapabilityProfile struct {
	ModelID          string            `json:"model_id"`
	ProviderID       string            `json:"provider_id"`
	LastEvaluatedAt  time.Time         `json:"last_evaluated_at"`
	SyntaxCompliance map[string]int    `json:"syntax_compliance"`   // Format => percentage/boolean weight
	SkillScores      map[string]int    `json:"skill_scores"`        // Operation Group => percentage weight
	ObservedLimits   map[string]string `json:"observed_limits"`     // E.g., "max_output_tokens", "rate_limit_behavior"
	SchemaVersion    string            `json:"schema_version"`      // Must be "motor-autonomo.capability-profile.v1"
}

func (p *ModelCapabilityProfile) Validate() error {
	if p.SchemaVersion != "motor-autonomo.capability-profile.v1" {
		return fmt.Errorf("invalid schema version: %q", p.SchemaVersion)
	}
	if p.ModelID == "" || p.ProviderID == "" {
		return fmt.Errorf("model_id and provider_id are required")
	}
	if p.LastEvaluatedAt.IsZero() {
		return fmt.Errorf("last_evaluated_at is required")
	}
	return nil
}

// ModelCapabilityDelta represents the update intent after an empirical evaluation run.
type ModelCapabilityDelta struct {
	ModelID    string
	ProviderID string
	Format     string
	Skill      string
	Passed     bool
	EvaluatedAt time.Time
}
