package kernel

import (
	"context"
	"errors"
	"fmt"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

const EventOperationModelRouted = "operation.model_routed"

// AppendModelRoutingEvent persists the non-secret IDs and bounded rejection
// reasons from a routing decision. Selection remains pure and legacy callers
// may keep using SelectModelBinding without event persistence.
func AppendModelRoutingEvent(ctx context.Context, store port.Store, now time.Time, operation domain.Operation, decision domain.ModelRouteDecision) error {
	if decision.SelectedBindingID == "" || decision.SelectedProviderID == "" {
		return errors.New("routing event requires selected provider and binding")
	}
	return store.Update(ctx, func(tx port.Transaction) error {
		_, err := tx.AppendEvent(domain.Event{
			SchemaVersion:   domain.SchemaVersionV1,
			ID:              domain.EventID(fmt.Sprintf("%s:model_routed:%d:%s", operation.ID, operation.Attempt, decision.SelectedBindingID)),
			Kind:            EventOperationModelRouted,
			OccurredAt:      now.UTC(),
			MissionRevision: operation.MissionRevision,
			InquiryID:       operation.InquiryID,
			OperationID:     operation.ID,
			PayloadRef:      fmt.Sprintf("provider_id=%s;binding_id=%s", decision.SelectedProviderID, decision.SelectedBindingID),
		})
		return err
	})
}

// SelectModelBinding applies domain.SelectModelBinding after hydrating durable ResourceUsage
// for each candidate binding. This separates routing decisions from resource persistence.
func SelectModelBinding(ctx context.Context, store port.Store, config domain.ModelsConfig, requiredTokens int, requiredCapability domain.RequiredCapability, profiles map[string]domain.ModelCapabilityProfile, now time.Time) (domain.ModelBindingConfig, domain.ModelRouteDecision, error) {
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
			bindingUsage, err := r.ResourceUsage(domain.ModelBindingResource(b.ID))
			if err != nil && !errors.Is(err, port.ErrNotFound) {
				return err
			}
			if errors.Is(err, port.ErrNotFound) {
				bindingUsage = domain.ResourceUsage{Resource: domain.ModelBindingResource(b.ID)}
			}
			providerUsage, err := r.ResourceUsage(domain.ModelProviderResource(b.ProviderRef))
			if err != nil && !errors.Is(err, port.ErrNotFound) {
				return err
			}
			if errors.Is(err, port.ErrNotFound) {
				providerUsage = domain.ResourceUsage{Resource: domain.ModelProviderResource(b.ProviderRef)}
			}
			candidates = append(candidates, domain.ModelRouteCandidate{
				Binding:       b,
				BindingUsage:  bindingUsage,
				ProviderUsage: providerUsage,
			})
		}
		return nil
	})
	if err != nil {
		return domain.ModelBindingConfig{}, domain.ModelRouteDecision{}, err
	}

	return domain.SelectSkilledModelBinding(candidates, profiles, requiredCapability, requiredTokens, now)
}
