package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"motor-autonomo/internal/channel/telegram"
	"motor-autonomo/internal/control"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
)

// resolveQuestionRoutes prefers durable CHANNELS revision routes; falls back to
// process-local seed routes from Options when no revision is installed.
func (rt *Runtime) resolveQuestionRoutes(ctx context.Context) ([]kernel.QuestionRoute, error) {
	if rt == nil {
		return nil, errors.New("runtime is nil")
	}
	routes, err := kernel.ActiveChannelRoutes(ctx, rt.Store, 3)
	if err != nil {
		return nil, err
	}
	if len(routes) > 0 {
		return routes, nil
	}
	out := make([]kernel.QuestionRoute, 0, len(rt.Opts.QuestionRoutes))
	for _, route := range rt.Opts.QuestionRoutes {
		out = append(out, kernel.QuestionRoute{
			Channel:        route.Channel,
			DestinationRef: route.DestinationRef,
			MaxAttempts:    route.MaxAttempts,
		})
	}
	return out, nil
}

func (rt *Runtime) processTelegramIngress(ctx context.Context, result *CycleResult) error {
	if rt == nil || result == nil {
		return errors.New("telegram ingress step requires runtime and result")
	}
	if rt.TelegramIngress == nil {
		return nil
	}
	// Poll before event drain so accepted USER_ANSWER events can be applied in
	// the same ProcessCycle. Webhook mode is a no-op here (HTTP-driven).
	ingest, err := rt.TelegramIngress.Poll(ctx)
	if err != nil {
		return fmt.Errorf("telegram ingress poll: %w", err)
	}
	if ingest.Fetched > 0 || ingest.Accepted > 0 || ingest.Rejected > 0 || ingest.Duplicate > 0 {
		result.TelegramFetched += ingest.Fetched
		result.TelegramAccepted += ingest.Accepted
		result.TelegramRejected += ingest.Rejected
		result.TelegramDuplicate += ingest.Duplicate
		// Accepted/duplicate imply durable inbox work; rejections still count as
		// productive channel activity so we do not idle-backoff aggressively.
		result.Worked = true
	}
	return nil
}

func (rt *Runtime) processQuestionChannel(ctx context.Context, result *CycleResult) error {
	if rt == nil || result == nil {
		return errors.New("question channel step requires runtime and result")
	}
	// Reminder planning is mission-scoped and independent of Telegram enablement.
	if rt.Opts.MissionID != "" {
		routes, err := rt.resolveQuestionRoutes(ctx)
		if err != nil {
			return fmt.Errorf("resolve question routes: %w", err)
		}
		if len(routes) > 0 {
			policy, err := kernel.ActiveReminderPolicy(ctx, rt.Store)
			if err != nil {
				return fmt.Errorf("active reminder policy: %w", err)
			}
			if policy.Enabled {
				processor, err := kernel.NewQuestionReminderProcessor(rt.Store, rt.Clock, rt.IDs, policy, routes)
				if err != nil {
					return err
				}
				scheduled, err := processor.ScheduleOpenForMission(ctx, rt.Opts.MissionID, rt.Opts.DeliveryBatch)
				if err != nil {
					return fmt.Errorf("schedule reminders: %w", err)
				}
				if scheduled > 0 {
					result.RemindersScheduled += scheduled
					result.Worked = true
				}
			}
		}
	}

	if rt.TelegramWorker != nil {
		processed, err := rt.TelegramWorker.ProcessDue(ctx, rt.Opts.DeliveryBatch)
		if err != nil {
			return fmt.Errorf("telegram delivery worker: %w", err)
		}
		if processed > 0 {
			result.DeliveriesProcessed += processed
			result.Worked = true
		}
	}
	return nil
}

// telegramSurfaces is the optional channel wiring returned by buildTelegram.
type telegramSurfaces struct {
	Worker  *telegram.DeliveryWorker
	Adapter *telegram.Adapter
	Ingress *telegram.Ingress
}

// buildTelegram assembles the non-authoritative adapter, outbox worker, and
// optional ingress. Returns zero value when Telegram is not enabled.
func buildTelegram(opts Options, store port.Store, clock source.Clock, events *control.ExternalEventInbox, ids source.IDGenerator) (telegramSurfaces, error) {
	var out telegramSurfaces
	if opts.Telegram == nil || !opts.Telegram.Enabled {
		return out, nil
	}
	token := strings.TrimSpace(os.Getenv(opts.Telegram.TokenEnv))
	if token == "" {
		return out, fmt.Errorf("telegram token env %q is empty", opts.Telegram.TokenEnv)
	}
	adapter, err := telegram.New(telegram.Config{
		Token:         token,
		BaseURL:       opts.Telegram.BaseURL,
		Destinations:  opts.Telegram.Destinations,
		AllowedActors: opts.Telegram.AllowedActors,
		AllowedChats:  opts.Telegram.AllowedChats,
	})
	if err != nil {
		return out, err
	}
	out.Adapter = adapter
	out.Worker = &telegram.DeliveryWorker{
		Store:         store,
		Adapter:       adapter,
		Clock:         clock,
		Owner:         opts.Telegram.WorkerOwner,
		LeaseDuration: opts.DeliveryLease,
		RetryDelay:    opts.DeliveryRetry,
	}
	ingressCfg := telegram.IngressConfig{
		Mode:        telegram.IngressMode(opts.Telegram.Ingress),
		PollLimit:   opts.Telegram.PollLimit,
		PollTimeout: opts.Telegram.PollTimeout,
		WebhookPath: opts.Telegram.WebhookPath,
		RejectUX:    opts.Telegram.RejectUX,
	}
	if opts.Telegram.Ingress == TelegramIngressWebhook && strings.TrimSpace(opts.Telegram.WebhookSecretEnv) != "" {
		ingressCfg.WebhookSecret = strings.TrimSpace(os.Getenv(opts.Telegram.WebhookSecretEnv))
	}
	ingress, err := telegram.NewIngress(adapter, store, events, ids, clock, ingressCfg)
	if err != nil {
		return out, err
	}
	if opts.Telegram.Ingress != TelegramIngressNone {
		out.Ingress = ingress
	}
	return out, nil
}
