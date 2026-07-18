package inspect

import (
	"context"
	"errors"
	"sort"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// ModelBindingPostureView correlates operator-controlled model configuration
// with durable ResourceGate usage and binding-local context pressure. Missing
// usage or pressure remains explicit and is never synthesized as observed zero.
type ModelBindingPostureView struct {
	BindingID       string                    `json:"binding_id"`
	ProviderID      string                    `json:"provider_id"`
	ProviderKind    domain.ProviderKind       `json:"provider_kind"`
	ModelID         string                    `json:"model_id"`
	Enabled         bool                      `json:"enabled"`
	Priority        int                       `json:"priority"`
	ContextTokens   int                       `json:"context_tokens"`
	MaxOutputTokens int                       `json:"max_output_tokens"`
	ProviderLimit   domain.ResourceLimit      `json:"provider_limit"`
	BindingLimit    domain.ResourceLimit      `json:"binding_limit"`
	ProviderUsage   *ResourceUsageView        `json:"provider_usage,omitempty"`
	BindingUsage    *ResourceUsageView        `json:"binding_usage,omitempty"`
	ContextPressure *ModelContextPressureView `json:"context_pressure,omitempty"`
}

// ModelBindingsProjection is a coherent read-only posture for the active MODELS
// revision. Bindings retain configured routing order (priority then ID).
type ModelBindingsProjection struct {
	SchemaVersion    int                       `json:"schema_version"`
	ObservedAt       time.Time                 `json:"observed_at"`
	ConfigRevision   domain.ConfigRevisionID   `json:"config_revision_id,omitempty"`
	ConfigGeneration uint64                    `json:"config_generation,omitempty"`
	Count            int                       `json:"count"`
	Bindings         []ModelBindingPostureView `json:"bindings"`
	Note             string                    `json:"note,omitempty"`
}

// ListModelBindingPostures correlates the active declared catalog with durable
// evidence. No active MODELS revision is a valid empty state, not an implicit
// catalog or unlimited capacity.
func (p *Projector) ListModelBindingPostures(ctx context.Context) (ModelBindingsProjection, error) {
	if p == nil {
		return ModelBindingsProjection{}, errors.New("projector is nil")
	}
	now := p.Clock().UTC()
	out := ModelBindingsProjection{
		SchemaVersion: domain.SchemaVersionV1,
		ObservedAt:    now,
		Bindings:      []ModelBindingPostureView{},
	}
	err := p.Store.View(ctx, func(r port.Reader) error {
		rev, err := r.ActiveConfigRevision(domain.ConfigScopeModels)
		if errors.Is(err, port.ErrNotFound) {
			out.Note = "no active MODELS revision; no binding posture can be correlated"
			return nil
		}
		if err != nil {
			return err
		}
		if rev.Models == nil {
			return errors.New("active MODELS revision has no models payload")
		}
		providers := make(map[string]domain.ModelProviderConfig, len(rev.Models.Providers))
		for _, provider := range rev.Models.Providers {
			providers[provider.ID] = provider
		}
		bindings := append([]domain.ModelBindingConfig(nil), rev.Models.Bindings...)
		sort.SliceStable(bindings, func(i, j int) bool {
			if bindings[i].Priority == bindings[j].Priority {
				return bindings[i].ID < bindings[j].ID
			}
			return bindings[i].Priority < bindings[j].Priority
		})
		out.ConfigRevision = rev.ID
		out.ConfigGeneration = rev.Revision
		out.Bindings = make([]ModelBindingPostureView, 0, len(bindings))
		for _, binding := range bindings {
			provider, ok := providers[binding.ProviderRef]
			if !ok {
				return errors.New("active MODELS revision has binding with unknown provider")
			}
			view := ModelBindingPostureView{
				BindingID: binding.ID, ProviderID: provider.ID, ProviderKind: provider.Kind,
				ModelID: binding.ModelID, Enabled: binding.Enabled, Priority: binding.Priority,
				ContextTokens: binding.ContextTokens, MaxOutputTokens: binding.MaxOutputTokens,
				ProviderLimit: provider.GlobalLimit, BindingLimit: binding.Limit,
			}
			if usage, usageErr := r.ResourceUsage(domain.ModelProviderResource(provider.ID)); usageErr == nil {
				projected := projectResourceUsage(usage, now)
				view.ProviderUsage = &projected
			} else if !errors.Is(usageErr, port.ErrNotFound) {
				return usageErr
			}
			if usage, usageErr := r.ResourceUsage(domain.ModelBindingResource(binding.ID)); usageErr == nil {
				projected := projectResourceUsage(usage, now)
				view.BindingUsage = &projected
			} else if !errors.Is(usageErr, port.ErrNotFound) {
				return usageErr
			}
			if pressure, pressureErr := r.ModelContextPressure(binding.ID); pressureErr == nil {
				projected := projectModelContextPressure(pressure)
				view.ContextPressure = &projected
			} else if !errors.Is(pressureErr, port.ErrNotFound) {
				return pressureErr
			}
			out.Bindings = append(out.Bindings, view)
		}
		out.Count = len(out.Bindings)
		if out.Count == 0 {
			out.Note = "active MODELS revision contains no bindings"
		}
		return nil
	})
	if err != nil {
		return ModelBindingsProjection{}, err
	}
	return out, nil
}
