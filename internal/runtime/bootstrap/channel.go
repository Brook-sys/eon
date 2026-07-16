package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"motor-autonomo/internal/channel/telegram"
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

	if rt.TelegramWorker == nil {
		return nil
	}
	processed, err := rt.TelegramWorker.ProcessDue(ctx, rt.Opts.DeliveryBatch)
	if err != nil {
		return fmt.Errorf("telegram delivery worker: %w", err)
	}
	if processed > 0 {
		result.DeliveriesProcessed += processed
		result.Worked = true
	}
	return nil
}

// buildTelegram assembles the non-authoritative adapter and outbox worker.
// Returns (nil, nil, nil) when Telegram is not enabled.
func buildTelegram(opts Options, store port.Store, clock source.Clock) (*telegram.DeliveryWorker, *telegram.Adapter, error) {
	if opts.Telegram == nil || !opts.Telegram.Enabled {
		return nil, nil, nil
	}
	token := strings.TrimSpace(os.Getenv(opts.Telegram.TokenEnv))
	if token == "" {
		return nil, nil, fmt.Errorf("telegram token env %q is empty", opts.Telegram.TokenEnv)
	}
	adapter, err := telegram.New(telegram.Config{
		Token:         token,
		BaseURL:       opts.Telegram.BaseURL,
		Destinations:  opts.Telegram.Destinations,
		AllowedActors: opts.Telegram.AllowedActors,
		AllowedChats:  opts.Telegram.AllowedChats,
	})
	if err != nil {
		return nil, nil, err
	}
	worker := &telegram.DeliveryWorker{
		Store:         store,
		Adapter:       adapter,
		Clock:         clock,
		Owner:         opts.Telegram.WorkerOwner,
		LeaseDuration: opts.DeliveryLease,
		RetryDelay:    opts.DeliveryRetry,
	}
	return worker, adapter, nil
}
