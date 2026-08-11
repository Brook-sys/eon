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
	StorageDolt   StorageBackend = "dolt"
)

// Options is the process-level assembly contract. Zero values are filled by
// Validate with conservative local defaults suitable for single-mission MVP.
// SecretResolver provides write-only credential references to process assembly.
// Implementations must never expose enumeration or export operations here.
type SecretResolver interface {
	Resolve(name string) (string, error)
}

type Options struct {
	// ListenAddr is the HTTP bind address for inspect/control/dashboard.
	ListenAddr string
	// StoreBackend selects memory (ephemeral) or sqlite (durable checkpoint).
	StoreBackend StorageBackend
	// SQLitePath is required when StoreBackend is sqlite.
	SQLitePath string
	// DoltPath is required when StoreBackend is dolt.
	DoltPath string
	// MissionID is the single mission the control loop schedules against.
	// Empty disables scheduler steps (inbox processors and HTTP still run).
	MissionID domain.MissionID
	// RuntimeName/Version appear in inspect projections and OTel resource.
	RuntimeName    string
	RuntimeVersion string
	// IdleMin/IdleMax bound the sleep after an empty control cycle.
	IdleMin time.Duration
	IdleMax time.Duration
	// SubagentIngressRecoveryDelay bounds the wait before another ingress cycle
	// after the per-transaction retry budget is exhausted. It prevents a durable
	// PENDING receipt from turning contention into a tight process-level loop.
	SubagentIngressRecoveryDelay time.Duration
	// InsecureVaultAccess disables the loopback-only requirement for the credential vault API.
	InsecureVaultAccess bool
	// MaxInboxBatch caps commands/events drained per cycle (fairness).
	MaxInboxBatch int
	// MemoryCompactionBatch caps expired semantic memories removed per cycle.
	MemoryCompactionBatch int
	// ModelCompletionReceiptBatch caps durable model completion settlements at
	// the pre-dispatch recovery boundary of each control cycle.
	ModelCompletionReceiptBatch int
	// PeerBindAddr enables the P2P RPC listener when not empty.
	PeerBindAddr string
	// PeerNodeID is the stable local identity used in authenticated sync frames.
	PeerNodeID string
	// PeerSyncInterval bounds authority-free P2P event pull cadence.
	// Zero defaults to 30 seconds when the peer listener is enabled.
	PeerSyncInterval time.Duration
	// PeerCert/PeerKey/PeerCACert are required when PeerBindAddr is set.
	PeerCert   string
	PeerKey    string
	PeerCACert string
	// Observability is optional derived export; zero value keeps it disabled.
	Observability observability.Config
	// EnableDashboard mounts the experimental operator UI on the same server.
	AllowExec       bool
	EnableDashboard bool
	// ModelPresetCatalogPath optionally exposes an evidence-verified preset
	// catalog through the Control API/dashboard. Empty disables the catalog.
	ModelPresetCatalogPath string
	// ModelSecretResolver optionally resolves APIKeyEnv references when the
	// corresponding process environment variable is empty.
	ModelSecretResolver SecretResolver

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
	// Model, when non-nil and enabled, wires a PROPOSE_ONLY OpenAI-compatible
	// provider into DispatchExecutor. API keys come only from env (never flags).
	Model *ModelOptions
	// Web, when non-nil and enabled, wires web.search / web.fetch under FR-RES-001.
	// Empty SearchBaseURL / Fetch disabled leave the corresponding adapter nil.
	Web *WebOptions
	// File, when non-nil and enabled, wires file.discover / file.read under
	// authorized absolute roots (FR-RES-001/002). Requires at least one root.
	File     *FileOptions
	Network  *NetworkOptions
	Subagent *SubagentOptions
}

// WebOptions configures optional READ_ONLY web acquisition adapters.
// Secrets are not used for MVP SearXNG/httpfetch; deploy-level egress controls apply.
type WebOptions struct {
	Enabled bool
	// SearchBaseURL is the SearXNG base URL (without /search). Empty disables search.
	SearchBaseURL string
	// EnableFetch turns on the hostile-by-default HTTP fetcher.
	EnableFetch bool
	// FetchMaxBytes caps response bodies (0 = adapter default 1 MiB).
	FetchMaxBytes int64
	// FetchAllowPrivate allows RFC1918/link-local targets (tests only; default false).
	FetchAllowPrivate bool
	// SearchMaxResponseBytes caps SearXNG JSON bodies (0 = adapter default).
	SearchMaxResponseBytes int64
	// DefaultSearchLimit is used when operations omit limit: (0 = executor default 5).
	DefaultSearchLimit int
	// PolicyVersion stamps capability authorizer decisions (empty = policy@runtime).
	PolicyVersion string
	// LeaseTTL bounds RUNNING/VERIFYING leases for web ops.
	LeaseTTL time.Duration
	// IngestFetched, when true, materializes Source lineage after successful fetch.
	IngestFetched bool
}

// FileRootConfig is one operator-authorized absolute directory for file.* ops.
type FileRootConfig struct {
	Name string
	// Path must be absolute on the host filesystem.
	Path string
}

// FileOptions configures optional READ_ONLY file.discover / file.read.
type FileOptions struct {
	Enabled bool
	// Roots lists authorized absolute directories. Required when Enabled.
	Roots []FileRootConfig
	// MaxReadBytes caps file.read content (0 = executor default 1 MiB).
	MaxReadBytes int64
	// MaxDiscoverEntries caps directory listing size (0 = executor default 256).
	MaxDiscoverEntries int
	// PolicyVersion stamps capability authorizer decisions (empty = policy@runtime).
	PolicyVersion string
	// LeaseTTL bounds RUNNING/VERIFYING leases for file ops.
	LeaseTTL time.Duration
}

// ModelMaxOutputField is the Chat Completions dialect for bounding output.
type ModelMaxOutputField string

const (
	ModelMaxOutputTokensLegacy     ModelMaxOutputField = "max_tokens"
	ModelMaxOutputTokensCompletion ModelMaxOutputField = "max_completion_tokens"
)

// ModelOptions configures an optional OpenAI-compatible text→text provider for
// non-local PROPOSE_ONLY operations. Secrets stay in process env; durable
// config must never hold raw API keys.
type ModelOptions struct {
	Enabled       bool
	ProviderID    string
	BindingID     string
	ProviderLimit domain.ResourceLimit
	BindingLimit  domain.ResourceLimit
	// BaseURL is an absolute HTTP(S) root (without /v1/chat/completions).
	BaseURL string
	// Model is the provider model name.
	Model string
	// APIKeyEnv names the env var with the bearer token. Empty means no Authorization header
	// (typical for open local servers such as Ollama).
	APIKeyEnv string
	// MaxOutputField selects max_tokens vs max_completion_tokens dialect.
	MaxOutputField ModelMaxOutputField
	// ContextTokens is the provider context window for prompt budgeting.
	ContextTokens int
	// PolicyVersion is stamped on accepted changesets.
	PolicyVersion string
	// LeaseTTL bounds RUNNING/VERIFYING leases for model-backed ops.
	LeaseTTL time.Duration
	// MaxResponseBytes caps raw provider HTTP body size (0 = adapter default).
	MaxResponseBytes int64
	// Timeout bounds one provider HTTP call (0 = adapter default).
	Timeout time.Duration
	// Fallback is the optional FR-MODEL-004 step-7 alternate provider. When
	// Enabled with BaseURL+Model, bootstrap wires ModelExecutor.FallbackProvider.
	// Empty/nil leaves FallbackAvailable=false (policy never invents a provider).
	Fallback *ModelFallbackOptions
}

// ModelFallbackOptions configures one alternate OpenAI-compatible endpoint for
// recovery step 7. Same secret discipline as ModelOptions (env names only).
type ModelFallbackOptions struct {
	Enabled       bool
	ProviderID    string
	BindingID     string
	ProviderLimit domain.ResourceLimit
	BindingLimit  domain.ResourceLimit
	// BaseURL is an absolute HTTP(S) root for the alternate endpoint.
	BaseURL string
	// Model is the alternate provider model name.
	Model string
	// APIKeyEnv names the env var for the alternate bearer token (may differ).
	APIKeyEnv string
	// MaxOutputField selects dialect; empty inherits primary ModelOptions field.
	MaxOutputField ModelMaxOutputField
	// ContextTokens is the alternate context window; 0 inherits primary.
	ContextTokens int
	// MaxResponseBytes caps alternate HTTP body size (0 = adapter default).
	MaxResponseBytes int64
	// Timeout bounds one alternate provider HTTP call (0 = adapter default).
	Timeout time.Duration
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
	case StorageDolt:
		if strings.TrimSpace(o.DoltPath) == "" {
			return errors.New("dolt store requires -dolt-path")
		}
	default:
		return fmt.Errorf("unknown store backend %q (want memory, sqlite, or dolt)", o.StoreBackend)
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
	if o.SubagentIngressRecoveryDelay <= 0 {
		o.SubagentIngressRecoveryDelay = 100 * time.Millisecond
	}
	if o.IdleMin > o.IdleMax {
		return errors.New("idle min must not exceed idle max")
	}
	if o.MaxInboxBatch <= 0 {
		o.MaxInboxBatch = 8
	}
	if o.ModelCompletionReceiptBatch <= 0 {
		o.ModelCompletionReceiptBatch = 8
	}
	if o.ModelCompletionReceiptBatch > 256 {
		return errors.New("model completion receipt batch is capped at 256")
	}
	if o.MaxInboxBatch > 256 {
		return errors.New("max inbox batch is capped at 256")
	}
	if o.MemoryCompactionBatch <= 0 {
		o.MemoryCompactionBatch = 8
	}
	if o.MemoryCompactionBatch > 256 {
		return errors.New("memory compaction batch is capped at 256")
	}
	if o.PeerBindAddr != "" {
		if o.PeerCert == "" || o.PeerKey == "" || o.PeerCACert == "" {
			return errors.New("peer binding requires cert, key, and ca-cert options")
		}
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
	if o.Subagent != nil && o.Subagent.Enabled {
		if o.MissionID == "" {
			return errors.New("subagent orchestration requires mission id")
		}
		if o.Subagent.MaxConcurrent < 0 || o.Subagent.MaxConcurrent > 32 {
			return errors.New("subagent max concurrent must be between 0 and 32")
		}
		if o.Subagent.MaxAttempts < 0 || o.Subagent.MaxAttempts > 16 {
			return errors.New("subagent max attempts must be between 0 and 16")
		}
		if o.Subagent.Timeout < 0 {
			return errors.New("subagent timeout must not be negative")
		}
		if len(o.Subagent.TransportPeerID) > 128 || strings.TrimSpace(o.Subagent.TransportPeerID) != o.Subagent.TransportPeerID {
			return errors.New("subagent transport peer id is invalid")
		}
	}
	for i, route := range o.QuestionRoutes {
		if strings.TrimSpace(route.Channel) == "" || strings.TrimSpace(route.DestinationRef) == "" {
			return fmt.Errorf("question route %d requires channel and destination", i)
		}
		if route.MaxAttempts == 0 {
			o.QuestionRoutes[i].MaxAttempts = 3
		}
	}
	if o.Model != nil && o.Model.Enabled {
		if strings.TrimSpace(o.Model.BaseURL) == "" {
			return errors.New("model provider requires base URL")
		}
		if strings.TrimSpace(o.Model.Model) == "" {
			return errors.New("model provider requires model name")
		}
		field := ModelMaxOutputField(strings.TrimSpace(string(o.Model.MaxOutputField)))
		if field == "" {
			field = ModelMaxOutputTokensLegacy
		}
		switch field {
		case ModelMaxOutputTokensLegacy, ModelMaxOutputTokensCompletion:
			o.Model.MaxOutputField = field
		default:
			return fmt.Errorf("unknown model max-output field %q", o.Model.MaxOutputField)
		}
		if o.Model.ContextTokens <= 0 {
			o.Model.ContextTokens = 8000
		}
		if o.Model.ContextTokens > 1_000_000 {
			return errors.New("model context tokens is capped at 1000000")
		}
		if strings.TrimSpace(o.Model.PolicyVersion) == "" {
			o.Model.PolicyVersion = "policy@runtime"
		}
		if o.Model.LeaseTTL <= 0 {
			o.Model.LeaseTTL = 15 * time.Minute
		}
		if o.Model.MaxResponseBytes < 0 {
			return errors.New("model max response bytes must not be negative")
		}
		if fb := o.Model.Fallback; fb != nil && fb.Enabled {
			if strings.TrimSpace(fb.BaseURL) == "" {
				return errors.New("model fallback requires base URL")
			}
			if strings.TrimSpace(fb.Model) == "" {
				return errors.New("model fallback requires model name")
			}
			fbField := ModelMaxOutputField(strings.TrimSpace(string(fb.MaxOutputField)))
			if fbField == "" {
				fbField = o.Model.MaxOutputField
			}
			switch fbField {
			case ModelMaxOutputTokensLegacy, ModelMaxOutputTokensCompletion:
				fb.MaxOutputField = fbField
			default:
				return fmt.Errorf("unknown model fallback max-output field %q", fb.MaxOutputField)
			}
			if fb.ContextTokens < 0 {
				return errors.New("model fallback context tokens must not be negative")
			}
			if fb.ContextTokens == 0 {
				fb.ContextTokens = o.Model.ContextTokens
			}
			if fb.ContextTokens > 1_000_000 {
				return errors.New("model fallback context tokens is capped at 1000000")
			}
			if fb.MaxResponseBytes < 0 {
				return errors.New("model fallback max response bytes must not be negative")
			}
			// Write back defaults onto the nested pointer so buildModel sees them.
			o.Model.Fallback = fb
		}
	}
	if o.Web != nil && o.Web.Enabled {
		hasSearch := strings.TrimSpace(o.Web.SearchBaseURL) != ""
		if !hasSearch && !o.Web.EnableFetch {
			return errors.New("web enabled requires search base URL and/or fetch")
		}
		if o.Web.FetchMaxBytes < 0 {
			return errors.New("web fetch max bytes must not be negative")
		}
		if o.Web.SearchMaxResponseBytes < 0 {
			return errors.New("web search max response bytes must not be negative")
		}
		if o.Web.DefaultSearchLimit < 0 {
			return errors.New("web default search limit must not be negative")
		}
		if strings.TrimSpace(o.Web.PolicyVersion) == "" {
			o.Web.PolicyVersion = "policy@runtime"
		}
		if o.Web.LeaseTTL <= 0 {
			o.Web.LeaseTTL = 5 * time.Minute
		}
	}
	if o.File != nil && o.File.Enabled {
		if len(o.File.Roots) == 0 {
			return errors.New("file enabled requires at least one authorized root")
		}
		seen := make(map[string]struct{}, len(o.File.Roots))
		for i, root := range o.File.Roots {
			name := strings.TrimSpace(root.Name)
			path := strings.TrimSpace(root.Path)
			if name == "" {
				name = "default"
			}
			if path == "" || !strings.HasPrefix(path, "/") {
				return fmt.Errorf("file root %d requires an absolute path", i)
			}
			if strings.Contains(path, "\x00") || strings.Contains(name, "\x00") {
				return fmt.Errorf("file root %d contains NUL", i)
			}
			if _, ok := seen[name]; ok {
				return fmt.Errorf("duplicate file root name %q", name)
			}
			seen[name] = struct{}{}
			o.File.Roots[i].Name = name
			o.File.Roots[i].Path = path
		}
		if o.File.MaxReadBytes < 0 {
			return errors.New("file max read bytes must not be negative")
		}
		if o.File.MaxDiscoverEntries < 0 {
			return errors.New("file max discover entries must not be negative")
		}
		if strings.TrimSpace(o.File.PolicyVersion) == "" {
			o.File.PolicyVersion = "policy@runtime"
		}
		if o.File.LeaseTTL <= 0 {
			o.File.LeaseTTL = 5 * time.Minute
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
// Prefer domain.DefaultSchedulerCadenceConfig for shared defaults; durable
// SCHEDULER revisions replace process knobs via ActiveSchedulerCadence.
func DefaultSchedulerCadence() domain.SchedulerCadenceConfig {
	return domain.DefaultSchedulerCadenceConfig()
}

type NetworkOptions struct {
	Enabled       bool
	BindAddr      string
	NodeID        string
	MDNSEnabled   bool
	TLSCertFile   string
	TLSKeyFile    string
	TLSCACertFile string
}
