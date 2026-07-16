// Package bootstrap assembles a process-local runtime from validated packages.
//
// It is the only place that wires storage, kernel processors, control/inspect
// HTTP surfaces, optional observability, and the control loop. Domain code and
// adapters stay free of process-level concerns.
package bootstrap

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/observability"
)

// StorageBackend selects the durable store adapter used by the process.
type StorageBackend string

const (
	StorageMemory StorageBackend = "memory"
	StorageSQLite StorageBackend = "sqlite"
)

// Options is the process-level assembly contract. Zero values are filled by
// Validate with conservative local defaults suitable for single-mission MVP.
type Options struct {
	// ListenAddr is the HTTP bind address for inspect/control/dashboard.
	ListenAddr string
	// StoreBackend selects memory (ephemeral) or sqlite (durable checkpoint).
	StoreBackend StorageBackend
	// SQLitePath is required when StoreBackend is sqlite.
	SQLitePath string
	// MissionID is the single mission the control loop schedules against.
	// Empty disables scheduler steps (inbox processors and HTTP still run).
	MissionID domain.MissionID
	// RuntimeName/Version appear in inspect projections and OTel resource.
	RuntimeName    string
	RuntimeVersion string
	// IdleMin/IdleMax bound the sleep after an empty control cycle.
	IdleMin time.Duration
	IdleMax time.Duration
	// MaxInboxBatch caps commands/events drained per cycle (fairness).
	MaxInboxBatch int
	// Observability is optional derived export; zero value keeps it disabled.
	Observability observability.Config
	// EnableDashboard mounts the experimental operator UI on the same server.
	EnableDashboard bool

	// QuestionRoutes seeds the active outbox/reminder routes when no durable
	// CHANNELS revision is installed. Empty means channel delivery is idle until
	// config is applied (dashboard answers still work via control API).
	QuestionRoutes []QuestionRouteConfig
	// DeliveryBatch caps outbox delivery attempts and reminder scans per cycle.
	DeliveryBatch int
	// DeliveryLease/DeliveryRetry configure the optional Telegram outbox worker.
	DeliveryLease time.Duration
	DeliveryRetry time.Duration
	// Telegram, when non-nil and enabled, wires the non-authoritative channel
	// adapter + outbox worker into the control loop. Token must come from env.
	Telegram *TelegramOptions
}

// TelegramIngressMode is the process-local inbound update collection mode.
type TelegramIngressMode string

const (
	TelegramIngressNone    TelegramIngressMode = "none"
	TelegramIngressPoll    TelegramIngressMode = "poll"
	TelegramIngressWebhook TelegramIngressMode = "webhook"
)

// QuestionRouteConfig is a process-local seed for kernel.QuestionRoute.
type QuestionRouteConfig struct {
	Channel        string
	DestinationRef string
	MaxAttempts    uint32
}

// TelegramOptions configures the optional Bot API adapter. Secrets stay in the
// process env; durable CHANNELS config only holds SecretRef names.
type TelegramOptions struct {
	Enabled       bool
	TokenEnv      string // environment variable holding the bot token
	BaseURL       string // optional override (tests / self-hosted gateways)
	Destinations  map[string]int64
	AllowedActors map[int64]string
	AllowedChats  map[int64]struct{}
	WorkerOwner   string

	// Ingress selects how updates enter the process. Empty/none = delivery only.
	Ingress TelegramIngressMode
	// PollLimit/PollTimeout configure getUpdates when Ingress=poll.
	// PollTimeout is long-poll seconds (0..50). Prefer 0 inside ProcessCycle so
	// the control loop never blocks on Telegram; use a dedicated poller if long-poll is needed.
	PollLimit   int
	PollTimeout int
	// WebhookPath is mounted on the process HTTP server when Ingress=webhook.
	WebhookPath string
	// WebhookSecretEnv names the env var with X-Telegram-Bot-Api-Secret-Token.
	// Empty disables secret validation (local tests only).
	WebhookSecretEnv string
	// RejectUX enables answerCallbackQuery / short notices on rejected updates.
	RejectUX bool
}

// Validate fills defaults and rejects unsafe combinations.
func (o *Options) Validate() error {
	if o == nil {
		return errors.New("bootstrap options are required")
	}
	if strings.TrimSpace(o.ListenAddr) == "" {
		o.ListenAddr = "127.0.0.1:8080"
	}
	if o.StoreBackend == "" {
		o.StoreBackend = StorageMemory
	}
	switch o.StoreBackend {
	case StorageMemory:
	case StorageSQLite:
		if strings.TrimSpace(o.SQLitePath) == "" {
			return errors.New("sqlite store requires -sqlite-path")
		}
	default:
		return fmt.Errorf("unknown store backend %q (want memory or sqlite)", o.StoreBackend)
	}
	if strings.TrimSpace(o.RuntimeName) == "" {
		o.RuntimeName = "motor-autonomo"
	}
	if strings.TrimSpace(o.RuntimeVersion) == "" {
		o.RuntimeVersion = "dev"
	}
	if o.IdleMin <= 0 {
		o.IdleMin = 50 * time.Millisecond
	}
	if o.IdleMax <= 0 {
		o.IdleMax = time.Second
	}
	if o.IdleMin > o.IdleMax {
		return errors.New("idle min must not exceed idle max")
	}
	if o.MaxInboxBatch <= 0 {
		o.MaxInboxBatch = 8
	}
	if o.MaxInboxBatch > 256 {
		return errors.New("max inbox batch is capped at 256")
	}
	if err := o.Observability.Validate(); err != nil {
		return err
	}
	if o.Observability.ServiceName == "" {
		o.Observability.ServiceName = o.RuntimeName
	}
	if o.Observability.ServiceVersion == "" {
		o.Observability.ServiceVersion = o.RuntimeVersion
	}
	if o.DeliveryBatch <= 0 {
		o.DeliveryBatch = 8
	}
	if o.DeliveryBatch > 64 {
		return errors.New("delivery batch is capped at 64")
	}
	if o.DeliveryLease <= 0 {
		o.DeliveryLease = 30 * time.Second
	}
	if o.DeliveryRetry <= 0 {
		o.DeliveryRetry = 15 * time.Second
	}
	for i, route := range o.QuestionRoutes {
		if strings.TrimSpace(route.Channel) == "" || strings.TrimSpace(route.DestinationRef) == "" {
			return fmt.Errorf("question route %d requires channel and destination", i)
		}
		if route.MaxAttempts == 0 {
			o.QuestionRoutes[i].MaxAttempts = 3
		}
	}
	if o.Telegram != nil && o.Telegram.Enabled {
		if strings.TrimSpace(o.Telegram.TokenEnv) == "" {
			return errors.New("telegram requires token env name")
		}
		if len(o.Telegram.Destinations) == 0 || len(o.Telegram.AllowedActors) == 0 || len(o.Telegram.AllowedChats) == 0 {
			return errors.New("telegram requires destinations, actor allowlist, and chat allowlist")
		}
		if strings.TrimSpace(o.Telegram.WorkerOwner) == "" {
			o.Telegram.WorkerOwner = "telegram-delivery"
		}
		mode := TelegramIngressMode(strings.TrimSpace(string(o.Telegram.Ingress)))
		if mode == "" {
			mode = TelegramIngressNone
		}
		switch mode {
		case TelegramIngressNone, TelegramIngressPoll, TelegramIngressWebhook:
			o.Telegram.Ingress = mode
		default:
			return fmt.Errorf("unknown telegram ingress mode %q", o.Telegram.Ingress)
		}
		if o.Telegram.PollLimit <= 0 {
			o.Telegram.PollLimit = 20
		}
		if o.Telegram.PollLimit > 100 {
			return errors.New("telegram poll limit is capped at 100")
		}
		if o.Telegram.PollTimeout < 0 || o.Telegram.PollTimeout > 50 {
			return errors.New("telegram poll timeout must be in [0,50]")
		}
		if mode == TelegramIngressWebhook {
			path := strings.TrimSpace(o.Telegram.WebhookPath)
			if path == "" {
				path = "/telegram/webhook"
			}
			if !strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
				return errors.New("telegram webhook path must be an absolute path without ..")
			}
			o.Telegram.WebhookPath = path
		}
	}
	return nil
}

// DefaultSchedulerCadence returns conservative process-local cadence knobs.
// Durable SCHEDULER revisions may later replace these via config projection.
func DefaultSchedulerCadence() domain.SchedulerCadenceConfig {
	return domain.SchedulerCadenceConfig{
		Version:          "scheduler.bootstrap.v1",
		MinIdleSleep:     50 * time.Millisecond,
		MaxIdleSleep:     time.Second,
		MaxCycleDuration: 30 * time.Second,
		MaxDispatches:    8,
	}
}
