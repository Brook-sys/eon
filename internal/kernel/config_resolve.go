package kernel

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

// ActiveSchedulerCadence loads the active SCHEDULER revision when present;
// otherwise returns DefaultSchedulerCadenceConfig. Callers re-validate before use.
func ActiveSchedulerCadence(ctx context.Context, store port.Store) (domain.SchedulerCadenceConfig, error) {
	if store == nil {
		return domain.SchedulerCadenceConfig{}, errors.New("active scheduler cadence requires store")
	}
	var cadence domain.SchedulerCadenceConfig
	err := store.View(ctx, func(r port.Reader) error {
		revision, err := r.ActiveConfigRevision(domain.ConfigScopeScheduler)
		if errors.Is(err, port.ErrNotFound) {
			cadence = domain.DefaultSchedulerCadenceConfig()
			return nil
		}
		if err != nil {
			return err
		}
		if revision.Scheduler == nil {
			return fmt.Errorf("active scheduler revision %s has no payload", revision.ID)
		}
		cadence = *revision.Scheduler
		return cadence.Validate()
	})
	return cadence, err
}

// ResolveSchedulerCadence prefers an explicit non-zero cadence (Version set),
// then the active durable SCHEDULER revision, then the built-in default.
func ResolveSchedulerCadence(ctx context.Context, store port.Store, explicit domain.SchedulerCadenceConfig) (domain.SchedulerCadenceConfig, error) {
	if strings.TrimSpace(explicit.Version) != "" {
		if err := explicit.Validate(); err != nil {
			return domain.SchedulerCadenceConfig{}, err
		}
		return explicit, nil
	}
	return ActiveSchedulerCadence(ctx, store)
}

// ActiveModelsConfig loads the active MODELS catalog. An absent revision is
// not an error: bootstrap may continue using its process-local model options.
func ActiveModelsConfig(ctx context.Context, store port.Store) (domain.ModelsConfig, bool, error) {
	if store == nil {
		return domain.ModelsConfig{}, false, errors.New("active models config requires store")
	}
	var config domain.ModelsConfig
	var found bool
	err := store.View(ctx, func(r port.Reader) error {
		revision, err := r.ActiveConfigRevision(domain.ConfigScopeModels)
		if errors.Is(err, port.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if revision.Models == nil {
			return fmt.Errorf("active models revision %s has no payload", revision.ID)
		}
		config = *revision.Models
		found = true
		return config.Validate()
	})
	return config, found, err
}

// ActiveReminderPolicy projects the active interruption policy's reminder
// section (or the built-in default, which disables reminders).
func ActiveReminderPolicy(ctx context.Context, store port.Store) (domain.ReminderPolicy, error) {
	gate, _, err := ActiveQuestionGatePolicy(ctx, store)
	if err != nil {
		return domain.ReminderPolicy{}, err
	}
	return gate.Reminder, gate.Reminder.Validate()
}

// ActiveChannelRoutes loads enabled CHANNELS routes as question outbox routes.
// When no CHANNELS revision exists, returns an empty slice (no channel delivery).
func ActiveChannelRoutes(ctx context.Context, store port.Store, defaultMaxAttempts uint32) ([]QuestionRoute, error) {
	if store == nil {
		return nil, errors.New("active channel routes require store")
	}
	if defaultMaxAttempts == 0 {
		defaultMaxAttempts = 3
	}
	var routes []QuestionRoute
	err := store.View(ctx, func(r port.Reader) error {
		revision, err := r.ActiveConfigRevision(domain.ConfigScopeChannels)
		if errors.Is(err, port.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if revision.Channels == nil {
			return fmt.Errorf("active channels revision %s has no payload", revision.ID)
		}
		for _, route := range revision.Channels.Routes {
			if !route.Enabled {
				continue
			}
			routes = append(routes, QuestionRoute{
				Channel:        route.Channel,
				DestinationRef: route.DestinationRef,
				MaxAttempts:    defaultMaxAttempts,
			})
		}
		return nil
	})
	return routes, err
}
