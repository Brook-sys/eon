package kernel

import (
	"context"
	"fmt"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// SelectModelBinding applies domain.SelectModelBinding after hydrating durable ResourceUsage
// for each candidate binding. This separates routing decisions from resource persistence.
func SelectModelBinding(ctx context.Context, store port.Store, config domain.ModelsConfig, requiredTokens int, now time.Time) (domain.ModelBindingConfig, domain.ModelRouteDecision, error) {
	if err := config.Validate(); err != nil {
		return domain.ModelBindingConfig{}, domain.ModelRouteDecision{}, fmt.Errorf("config: %w", err)
	}

	var candidates []domain.ModelRouteCandidate
	err := store.View(ctx, func(r port.Reader) error {
		for _, b := range config.Bindings {
			// Skip disabled early before I/O
			if !b.Enabled {
				candidates = append(candidates, domain.ModelRouteCandidate{Binding: b})
				continue
			}
			usage, err := r.ResourceUsage(domain.ModelBindingResource(b.ID))
			if err != nil {
				return err
			}
			candidates = append(candidates, domain.ModelRouteCandidate{
				Binding: b,
				Usage:   usage,
			})
		}
		return nil
	})
	if err != nil {
		return domain.ModelBindingConfig{}, domain.ModelRouteDecision{}, err
	}

	return domain.SelectModelBinding(candidates, requiredTokens, now)
}
