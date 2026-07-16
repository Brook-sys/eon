package kernel

import (
	"context"
	"errors"
	"fmt"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

// QuestionGatePolicyFromInterruption projects a versioned interruption config
// into the pure gate evaluation surface. PolicyVersion is taken from the
// durable config Version field so audits can correlate decisions with revisions.
func QuestionGatePolicyFromInterruption(p domain.InterruptionRuntimePolicy) (QuestionGatePolicy, string, error) {
	if err := p.Validate(); err != nil {
		return QuestionGatePolicy{}, "", err
	}
	policy := QuestionGatePolicy{
		MinPriority:                   p.MinPriority,
		MaxPending:                    p.MaxPending,
		MaxDeliveredPerWindow:         p.MaxDeliveredPerWindow,
		MaxAdmittedPerWindow:          p.MaxAdmittedPerWindow,
		Window:                        p.Window,
		Cooldown:                      p.Cooldown,
		TopicCooldown:                 p.TopicCooldown,
		QuietStartHour:                p.QuietStartHour,
		QuietEndHour:                  p.QuietEndHour,
		UrgentPriority:                p.UrgentPriority,
		MinAlternativesTried:          p.MinAlternativesTried,
		SuppressSafeReversibleDefault: p.SuppressSafeReversibleDefault,
		Digest:                        p.Digest,
		Reminder:                      p.Reminder,
	}
	if err := policy.Validate(); err != nil {
		return QuestionGatePolicy{}, "", err
	}
	return policy, p.Version, nil
}

// ActiveQuestionGatePolicy loads the active INTERRUPTION revision when present;
// otherwise returns the conservative built-in default. The second return is the
// policy version string used in gate decision audit records.
func ActiveQuestionGatePolicy(ctx context.Context, store port.Store) (QuestionGatePolicy, string, error) {
	if store == nil {
		return QuestionGatePolicy{}, "", errors.New("active question gate policy requires store")
	}
	var policy QuestionGatePolicy
	var version string
	err := store.View(ctx, func(r port.Reader) error {
		revision, err := r.ActiveConfigRevision(domain.ConfigScopeInterruption)
		if errors.Is(err, port.ErrNotFound) {
			fallback := domain.DefaultInterruptionRuntimePolicy()
			var mapErr error
			policy, version, mapErr = QuestionGatePolicyFromInterruption(fallback)
			return mapErr
		}
		if err != nil {
			return err
		}
		if revision.Interruption == nil {
			return fmt.Errorf("active interruption revision %s has no payload", revision.ID)
		}
		var mapErr error
		policy, version, mapErr = QuestionGatePolicyFromInterruption(*revision.Interruption)
		if mapErr != nil {
			return mapErr
		}
		// Prefer revision content hash suffix for audit when version collides.
		if version == "" {
			version = string(revision.ID)
		}
		return nil
	})
	return policy, version, err
}

// ActiveHorizonPolicy loads the active HORIZON revision when present; otherwise
// returns DefaultHorizonPolicy. Callers still re-validate before use.
func ActiveHorizonPolicy(ctx context.Context, store port.Store) (domain.HorizonPolicy, error) {
	if store == nil {
		return domain.HorizonPolicy{}, errors.New("active horizon policy requires store")
	}
	var policy domain.HorizonPolicy
	err := store.View(ctx, func(r port.Reader) error {
		revision, err := r.ActiveConfigRevision(domain.ConfigScopeHorizon)
		if errors.Is(err, port.ErrNotFound) {
			policy = domain.DefaultHorizonPolicy()
			return nil
		}
		if err != nil {
			return err
		}
		if revision.Horizon == nil {
			return fmt.Errorf("active horizon revision %s has no payload", revision.ID)
		}
		policy = *revision.Horizon
		return policy.Validate()
	})
	return policy, err
}

// ResolveHorizonPolicy prefers an explicit non-zero policy, then the active
// durable HORIZON revision, then the built-in default.
func ResolveHorizonPolicy(ctx context.Context, store port.Store, explicit domain.HorizonPolicy) (domain.HorizonPolicy, error) {
	if explicit.Version != "" || explicit.SchemaVersion != 0 {
		if err := explicit.Validate(); err != nil {
			return domain.HorizonPolicy{}, err
		}
		return explicit, nil
	}
	return ActiveHorizonPolicy(ctx, store)
}
