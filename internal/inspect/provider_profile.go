package inspect

import (
	"context"
	"errors"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// ProviderProfileView is the operator-facing capability snapshot (FR-MODEL-005).
// It never includes secrets, base URLs with credentials, or free-form prompts.
type ProviderProfileView struct {
	SchemaVersion int                     `json:"schema_version"`
	Configured    bool                    `json:"configured"`
	Profile       *domain.ProviderProfile `json:"profile,omitempty"`
	// Live indicates whether the profile came from Probe (true) or DeclaredProfile (false).
	Live bool `json:"live,omitempty"`
	// Note is a non-secret explanation when no provider is wired.
	Note string `json:"note,omitempty"`
}

// SetModelProvider installs an optional model provider for capability inspect.
// Safe to call with nil to clear. Does not authorize model execution by itself.
func (p *Projector) SetModelProvider(provider port.ModelProvider) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.modelProvider = provider
}

// ModelProvider returns the process-local provider used for capability inspect.
func (p *Projector) ModelProvider() port.ModelProvider {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.modelProvider
}

// ProviderProfile returns the declared capability snapshot without network I/O.
func (p *Projector) ProviderProfile(_ context.Context) (ProviderProfileView, error) {
	if p == nil {
		return ProviderProfileView{}, errors.New("projector is nil")
	}
	provider := p.ModelProvider()
	if provider == nil {
		return ProviderProfileView{
			SchemaVersion: domain.SchemaVersionV1,
			Configured:    false,
			Note:          "no model provider configured; PROPOSE_ONLY path remains requires_model",
		}, nil
	}
	reporter, ok := provider.(port.ModelCapabilityReporter)
	if !ok {
		return ProviderProfileView{
			SchemaVersion: domain.SchemaVersionV1,
			Configured:    true,
			Note:          "provider does not implement ModelCapabilityReporter",
		}, nil
	}
	profile := reporter.DeclaredProfile()
	if err := profile.Validate(); err != nil {
		return ProviderProfileView{}, err
	}
	return ProviderProfileView{
		SchemaVersion: domain.SchemaVersionV1,
		Configured:    true,
		Profile:       &profile,
		Live:          false,
	}, nil
}

// ProviderProfileProbe runs a budgeted live probe when the provider supports it.
// Exhausted probe budgets return the cached/declared snapshot without looping.
func (p *Projector) ProviderProfileProbe(ctx context.Context) (ProviderProfileView, error) {
	if p == nil {
		return ProviderProfileView{}, errors.New("projector is nil")
	}
	provider := p.ModelProvider()
	if provider == nil {
		return ProviderProfileView{
			SchemaVersion: domain.SchemaVersionV1,
			Configured:    false,
			Note:          "no model provider configured; probe skipped",
		}, nil
	}
	reporter, ok := provider.(port.ModelCapabilityReporter)
	if !ok {
		return ProviderProfileView{
			SchemaVersion: domain.SchemaVersionV1,
			Configured:    true,
			Note:          "provider does not implement ModelCapabilityReporter",
		}, nil
	}
	profile, err := reporter.Probe(ctx)
	if err != nil {
		return ProviderProfileView{}, err
	}
	if err := profile.Validate(); err != nil {
		return ProviderProfileView{}, err
	}
	return ProviderProfileView{
		SchemaVersion: domain.SchemaVersionV1,
		Configured:    true,
		Profile:       &profile,
		Live:          true,
	}, nil
}
