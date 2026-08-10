package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"motor-autonomo/internal/channel/telegram"
	"motor-autonomo/internal/control"
	"motor-autonomo/internal/dashboard"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/inspect"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/mission"
	"motor-autonomo/internal/network"
	"motor-autonomo/internal/network/http"
	"motor-autonomo/internal/observability"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/retry"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/secretvault"
	"motor-autonomo/internal/storage/dolt"
	"motor-autonomo/internal/storage/memory"
	"motor-autonomo/internal/storage/sqlite"
	"motor-autonomo/internal/tool"
)

// Runtime is the assembled process graph. Call Close after Run returns.
type Runtime struct {
	Opts Options

	Store  port.Store
	Clock  source.Clock
	IDs    source.IDGenerator
	Random source.RandomSource
	Closer io.Closer // optional store closer (sqlite)

	Commands *control.CommandInbox
	Events   *control.ExternalEventInbox
	// SemanticMemory owns audited current-view mutations and bounded expiry GC.
	SemanticMemory *control.SemanticMemory

	// Command/Event processors may be observability wrappers; they never gain authority.
	CommandProcessor observability.CommandProcessor
	EventProcessor   observability.ExternalEventProcessor
	ConfigApplier    *kernel.ConfigApplier
	Scheduler        kernel.Scheduler
	// Executor routes local continuity and optional file/web/model paths.
	Executor kernel.DispatchExecutor
	// LeaseReaper reconciles expired RUNNING/VERIFYING leases before dispatch.
	LeaseReaper kernel.LeaseReaper
	// Subagents and Supervisor share one manager so model tool calls, durable
	// lifecycle records, and cycle reconciliation observe the same sessions.
	Subagents                   kernel.SessionManager
	Supervisor                  *kernel.Supervisor
	SubagentDispatcher          *kernel.SubagentDispatcher
	SubagentEffectReconciler    *kernel.SubagentEffectReconciler
	RemoteSubagentWorker        *kernel.RemoteSubagentWorker
	SubagentStatusDispatcher    *kernel.SubagentStatusDispatcher
	SubagentStatusIngressWorker *kernel.SubagentStatusIngressWorker
	// subagentTools is retained separately so dynamic model-executor reloads
	// preserve the lifecycle tools rather than rebuilding an fs/exec-only catalog.
	subagentTools tool.Provider
	// Model is optional; nil keeps non-local ops skipped as requires_model.
	Model *kernel.ModelExecutor
	// Web is optional; nil keeps web-eligible ops skipped as requires_web.
	Web *kernel.WebExecutor
	// File is optional; nil keeps file-eligible ops skipped as requires_file.
	File      *kernel.FileExecutor
	Peer      *kernel.PeerTransport
	Registry  *kernel.StrategyRegistry
	Cooldowns *kernel.StrategyCooldownBook

	Inspect   *inspect.API
	Control   *control.API
	Dashboard *dashboard.V2Server
	Vault     *secretvault.Vault
	Handler   http.Handler

	// Optional non-authoritative Telegram surfaces. Nil when disabled.
	TelegramAdapter *telegram.Adapter
	TelegramWorker  *telegram.DeliveryWorker
	TelegramIngress *telegram.Ingress

	Telemetry      *observability.Runtime
	cycleTelemetry *observability.CycleInstruments

	logger     *log.Logger
	mu         sync.Mutex
	server     *http.Server
	peerServer *http.Server
}

// Open assembles dependencies. It does not start the HTTP server or loop.
func Open(ctx context.Context, opts Options) (*Runtime, error) {
	var p2pManager *network.P2PManager
	if opts.Network != nil && opts.Network.Enabled {
		pm, err := network.NewP2PManager(network.Options{
			Enabled:       opts.Network.Enabled,
			BindAddr:      opts.Network.BindAddr,
			MDNSEnabled:   opts.Network.MDNSEnabled,
			TLSCertFile:   opts.Network.TLSCertFile,
			TLSKeyFile:    opts.Network.TLSKeyFile,
			TLSCACertFile: opts.Network.TLSCACertFile,
		}, slog.Default())
		if err != nil {
			return nil, fmt.Errorf("create p2p manager: %w", err)
		}
		p2pManager = pm
	}
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	clock := source.SystemClock{}
	ids := source.CryptoIDGenerator{}
	random := source.CryptoRandomSource{}

	store, closer, err := openStore(opts)
	if err != nil {
		return nil, err
	}

	telemetry, err := observability.Setup(ctx, opts.Observability)
	if err != nil {
		if closer != nil {
			_ = closer.Close()
		}
		return nil, fmt.Errorf("observability setup: %w", err)
	}

	receipts := control.ReceiptFactoryFrom(clock, ids)
	dispositions := control.DispositionFactoryFrom(clock)

	commandInbox, err := control.NewCommandInbox(store, receipts)
	if err != nil {
		_ = telemetry.Shutdown(ctx)
		if closer != nil {
			_ = closer.Close()
		}
		return nil, err
	}
	eventInbox, err := control.NewExternalEventInbox(store, dispositions)
	if err != nil {
		_ = telemetry.Shutdown(ctx)
		if closer != nil {
			_ = closer.Close()
		}
		return nil, err
	}

	commandProcessor, err := kernel.NewCommandProcessor(store, clock, ids)
	if err != nil {
		_ = telemetry.Shutdown(ctx)
		if closer != nil {
			_ = closer.Close()
		}
		return nil, err
	}
	eventProcessor, err := kernel.NewExternalEventProcessor(store, clock, ids)
	if err != nil {
		_ = telemetry.Shutdown(ctx)
		if closer != nil {
			_ = closer.Close()
		}
		return nil, err
	}
	configApplier, err := kernel.NewConfigApplier(store, clock, ids)
	if err != nil {
		_ = telemetry.Shutdown(ctx)
		if closer != nil {
			_ = closer.Close()
		}
		return nil, err
	}

	registry := kernel.NewStrategyRegistry()
	if err := kernel.RegisterDefaultContinuityFamilies(registry, store, clock, ids, domain.DefaultHorizonPolicy()); err != nil {
		_ = telemetry.Shutdown(ctx)
		if closer != nil {
			_ = closer.Close()
		}
		return nil, fmt.Errorf("register continuity families: %w", err)
	}
	cooldowns := kernel.NewStrategyCooldownBook()
	scheduler := kernel.Scheduler{
		Store:       store,
		MemoryStore: store,
		Clock:       clock,
		Registry:    registry,
		IDs:         ids,
		Cooldowns:   cooldowns,
	}

	projector, err := inspect.NewProjector(store, inspect.RuntimeIdentity{
		Name:    opts.RuntimeName,
		Version: opts.RuntimeVersion,
	})
	if err != nil {
		_ = telemetry.Shutdown(ctx)
		if closer != nil {
			_ = closer.Close()
		}
		return nil, err
	}
	// Projector uses system clock for GeneratedAt; keep aligned with injectable clock.
	projector.Clock = clock.Now
	// Expose the process strategy portfolio on inspect (read-only, non-authoritative).
	projector.SetContinuityCatalog(continuityCatalogFromRegistry(registry))
	// Disposable OTel export posture for alerts/retention projections (never kernel input).
	projector.SetTelemetry(telemetry.Enabled(), telemetry.HasOTLP(), telemetry.Retention())

	inspectAPI, err := inspect.NewAPI(projector)
	if err != nil {
		_ = telemetry.Shutdown(ctx)
		if closer != nil {
			_ = closer.Close()
		}
		return nil, err
	}
	controlAPI, err := control.NewAPI(commandInbox, eventInbox, clock, ids)
	if err != nil {
		_ = telemetry.Shutdown(ctx)
		if closer != nil {
			_ = closer.Close()
		}
		return nil, err
	}
	controlAPI.ConfigValidate = configApplier
	controlAPI.ConfigApply = configApplier
	controlAPI.ConfigRollback = configApplier
	semanticMemory, err := control.NewSemanticMemory(store, clock, ids)
	if err != nil {
		_ = telemetry.Shutdown(ctx)
		if closer != nil {
			_ = closer.Close()
		}
		return nil, err
	}
	controlAPI.SemanticMemory = semanticMemory
	controlAPI.SemanticMemoryReader = store
	if opts.ModelPresetCatalogPath != "" {
		catalog, loadErr := loadModelPresetCatalog(opts.ModelPresetCatalogPath)
		if loadErr != nil {
			_ = telemetry.Shutdown(ctx)
			if closer != nil {
				_ = closer.Close()
			}
			return nil, fmt.Errorf("load model preset catalog: %w", loadErr)
		}
		controlAPI.ModelPresets = &catalog
	}
	missionAcceptor := mission.Acceptor{Store: store, Clock: clock, IDs: ids}
	controlAPI.MissionAccept = control.MissionAmendmentAcceptorFunc(func(ctx context.Context, amendment domain.UserAmendment, provenance string) (control.MissionAmendmentAcceptance, error) {
		result, err := missionAcceptor.Accept(ctx, amendment, provenance)
		if err != nil {
			return control.MissionAmendmentAcceptance{}, err
		}
		return control.MissionAmendmentAcceptance{
			Previous: result.Previous,
			Accepted: result.Accepted,
			Diff:     result.Diff,
			Impact:   result.Impact,
			Report:   result.Report,
		}, nil
	})

	telegramBits, err := buildTelegram(opts, store, clock, eventInbox, ids)
	if err != nil {
		_ = telemetry.Shutdown(ctx)
		if closer != nil {
			_ = closer.Close()
		}
		return nil, fmt.Errorf("telegram channel: %w", err)
	}

	var dash *dashboard.Server
	var dashV2 *dashboard.V2Server
	var vault *secretvault.Vault
	var handler http.Handler
	if opts.EnableDashboard {
		// Templ-based v2 dashboard serves /dash/* with the same inspect/control
		// APIs. Static assets are embedded; no external dir needed.
		var err error
		dashV2, err = dashboard.NewV2(inspectAPI.Handler(), controlAPI.Handler(), nil)
		if err != nil {
			_ = telemetry.Shutdown(ctx)
			if closer != nil {
				_ = closer.Close()
			}
			return nil, fmt.Errorf("dashboard v2: %w", err)
		}

		dash, err = dashboard.New(inspectAPI.Handler(), controlAPI.Handler())
		if err != nil {
			_ = telemetry.Shutdown(ctx)
			if closer != nil {
				_ = closer.Close()
			}
			return nil, err
		}
		dash.DefaultMissionID = string(opts.MissionID)

		// Vault setup (unchanged semantics).
		vaultPath := opts.SQLitePath + ".credentials.vault"
		if opts.SQLitePath == "" {
			vaultPath = filepath.Join(os.TempDir(), "eon-memory.credentials.vault")
		}
		var vaultErr error
		vault, vaultErr = secretvault.New(vaultPath)
		if vaultErr != nil {
			_ = telemetry.Shutdown(ctx)
			if closer != nil {
				_ = closer.Close()
			}
			return nil, fmt.Errorf("credential vault: %w", vaultErr)
		}
		dash.Vault = secretvault.HTTP{Vault: vault}.Handler()
		dashV2.Vault = dash.Vault

		// Compose the two dashboards. Legacy handles /, /api/*, /healthz;
		// the v2 Templ dashboard handles /dash/*.
		mux := http.NewServeMux()
		mux.Handle("/", dash.Handler())
		mux.Handle("/api/", dash.Handler())
		mux.Handle("/dash/", dashV2.Handler())
		handler = mux
	} else {
		mux := http.NewServeMux()
		mux.Handle("/api/inspect/", http.StripPrefix("/api/inspect", inspectAPI.Handler()))
		mux.Handle("/api/control/", http.StripPrefix("/api/control", controlAPI.Handler()))
		mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})
		handler = mux
	}
	handler = mountTelegramWebhook(handler, telegramBits.Ingress)

	localExec := kernel.LocalExecutor{
		Store: store,
		Clock: clock,
		IDs:   ids,
	}
	leaseReaper := kernel.LeaseReaper{
		Store: store,
		Clock: clock,
		IDs:   ids,
	}
	if vault != nil {
		opts.ModelSecretResolver = vault
	}
	modelExec, err := BuildModelExecutor(opts, store, clock, ids, telemetry)
	if err != nil {
		_ = telemetry.Shutdown(ctx)
		if closer != nil {
			_ = closer.Close()
		}
		return nil, fmt.Errorf("model provider: %w", err)
	}
	subagentTools, sessionManager, err := buildSubagent(opts.Subagent, store, clock, ids, opts.MissionID)
	if err != nil {
		_ = telemetry.Shutdown(ctx)
		if closer != nil {
			_ = closer.Close()
		}
		return nil, fmt.Errorf("subagent provider: %w", err)
	}
	var supervisor *kernel.Supervisor
	if sessionManager != nil {
		supervisor = &kernel.Supervisor{Store: store, Manager: sessionManager, Clock: clock, IDs: ids, LeaseTTL: opts.Subagent.LeaseTTL}
	}
	if modelExec != nil && subagentTools != nil {
		merged, mergeErr := tool.MergeProviders(modelExec.Tools, subagentTools)
		if mergeErr != nil {
			_ = telemetry.Shutdown(ctx)
			if closer != nil {
				_ = closer.Close()
			}
			return nil, fmt.Errorf("merge model and subagent tools: %w", mergeErr)
		}
		modelExec.Tools = merged
	}
	webExec, err := buildWeb(opts, store, clock, ids)
	if err != nil {
		_ = telemetry.Shutdown(ctx)
		if closer != nil {
			_ = closer.Close()
		}
		return nil, fmt.Errorf("web provider: %w", err)
	}
	fileExec, err := buildFile(opts, store, clock, ids)
	if err != nil {
		_ = telemetry.Shutdown(ctx)
		if closer != nil {
			_ = closer.Close()
		}
		return nil, fmt.Errorf("file provider: %w", err)
	}
	// Expose declared/probed provider capabilities on inspect (read-only; FR-MODEL-005).
	if modelExec != nil {
		projector.SetModelProvider(modelExec.Provider)
	}

	peerTransport, err := buildPeerTransport(opts, store, clock.Now, sessionManager)
	if err != nil {
		_ = telemetry.Shutdown(ctx)
		if closer != nil {
			_ = closer.Close()
		}
		return nil, fmt.Errorf("peer transport: %w", err)
	}
	if peerTransport != nil {
		controlAPI.PeerManager = &control.PeerManager{
			Registry: peerTransport.Registry,
		}
	}
	var subagentDispatcher *kernel.SubagentDispatcher
	var subagentEffectReconciler *kernel.SubagentEffectReconciler
	var remoteSubagentWorker *kernel.RemoteSubagentWorker
	var subagentStatusDispatcher *kernel.SubagentStatusDispatcher
	var subagentStatusIngressWorker *kernel.SubagentStatusIngressWorker
	if peerTransport != nil && sessionManager != nil {
		subagentDispatcher = &kernel.SubagentDispatcher{Store: store, Caller: peerTransport.Caller, Clock: clock, Owner: opts.RuntimeName, Batch: 4, Lease: 30 * time.Second, RetryDelay: 15 * time.Second, RPCTimeout: 10 * time.Second}
		subagentEffectReconciler = &kernel.SubagentEffectReconciler{Store: store, Caller: peerTransport.Caller, Clock: clock, Batch: 4, RPCTimeout: 10 * time.Second}
		subagentStatusDispatcher = &kernel.SubagentStatusDispatcher{Store: store, Caller: peerTransport.Caller, Clock: clock, Batch: 4, RPCTimeout: 10 * time.Second}
		subagentStatusIngressWorker = &kernel.SubagentStatusIngressWorker{
			Store: store, Manager: sessionManager, Clock: clock, Batch: 4, LeaseTTL: opts.Subagent.LeaseTTL,
			RetryPolicy:  kernel.DefaultSubagentStatusIngressRetryPolicy(),
			RetrySleeper: retry.SystemSleeper{}, RetryJitter: random,
		}
		if modelExec != nil && modelExec.Provider != nil {
			remoteSubagentWorker = &kernel.RemoteSubagentWorker{Store: store, Manager: sessionManager, Executor: kernel.ModelRemoteSubagentExecutor{Provider: modelExec.Provider, MaxOutputTokens: 512}, Clock: clock, Owner: opts.RuntimeName, Batch: 2, Lease: 2 * time.Minute, Timeout: 90 * time.Second}
		}
	}

	if p2pManager != nil {
		_ = p2pManager.Start(ctx)
	}

	return &Runtime{
		Opts:             opts,
		Store:            store,
		Clock:            clock,
		IDs:              ids,
		Random:           random,
		Closer:           closer,
		Commands:         commandInbox,
		Events:           eventInbox,
		SemanticMemory:   semanticMemory,
		CommandProcessor: observability.InstrumentCommand(commandProcessor, telemetry),
		EventProcessor:   observability.InstrumentExternalEvent(eventProcessor, telemetry),
		ConfigApplier:    configApplier,
		Scheduler:        scheduler,
		Executor: kernel.DispatchExecutor{
			Store: store,
			Local: localExec,
			File:  fileExec,
			Web:   webExec,
			Model: modelExec,
		},
		LeaseReaper:                 leaseReaper,
		Subagents:                   sessionManager,
		Supervisor:                  supervisor,
		SubagentDispatcher:          subagentDispatcher,
		SubagentEffectReconciler:    subagentEffectReconciler,
		RemoteSubagentWorker:        remoteSubagentWorker,
		SubagentStatusDispatcher:    subagentStatusDispatcher,
		SubagentStatusIngressWorker: subagentStatusIngressWorker,
		subagentTools:               subagentTools,
		Model:                       modelExec,
		Web:                         webExec,
		File:                        fileExec,
		Peer:                        peerTransport,
		Registry:                    registry,
		Cooldowns:                   cooldowns,
		Inspect:                     inspectAPI,
		Control:                     controlAPI,
		Dashboard:                   dashV2,
		Vault:                       vault,
		Handler:                     handler,
		TelegramAdapter:             telegramBits.Adapter,
		TelegramWorker:              telegramBits.Worker,
		TelegramIngress:             telegramBits.Ingress,
		Telemetry:                   telemetry,
		cycleTelemetry:              observability.NewCycleInstruments(telemetry),
		logger:                      log.Default(),
	}, nil
}

func loadModelPresetCatalog(path string) (domain.ModelPresetCatalog, error) {
	file, err := os.Open(path)
	if err != nil {
		return domain.ModelPresetCatalog{}, err
	}
	catalog, decodeErr := domain.DecodeModelPresetCatalog(file, 1<<20)
	closeErr := file.Close()
	if decodeErr != nil {
		return domain.ModelPresetCatalog{}, decodeErr
	}
	if closeErr != nil {
		return domain.ModelPresetCatalog{}, closeErr
	}
	base := filepath.Dir(path)
	for _, preset := range catalog.Presets {
		evidencePath := filepath.Join(base, filepath.FromSlash(preset.EvidenceReport))
		body, readErr := os.ReadFile(evidencePath)
		if readErr != nil {
			return domain.ModelPresetCatalog{}, fmt.Errorf("preset %s evidence: %w", preset.ID, readErr)
		}
		digest := sha256.Sum256(body)
		if hex.EncodeToString(digest[:]) != preset.EvidenceSHA256 {
			return domain.ModelPresetCatalog{}, fmt.Errorf("preset %s evidence digest mismatch", preset.ID)
		}
	}
	return catalog, nil
}

// AttachModel wires a PROPOSE_ONLY ModelExecutor into the dispatch path.
// Safe to call after Open for tests or process configuration with a free local
// OpenAI-compatible endpoint. nil clears the model path.
func (rt *Runtime) AttachModel(model *kernel.ModelExecutor) {
	if rt == nil {
		return
	}
	// Keep inspect capability surface aligned with the dispatch path.
	if rt.Inspect != nil && rt.Inspect.Projector != nil {
		if model == nil {
			rt.Inspect.Projector.SetModelProvider(nil)
		} else {
			rt.Inspect.Projector.SetModelProvider(model.Provider)
		}
	}
	rt.Model = model
	rt.Executor.Model = model
}

// AttachWeb wires a READ_ONLY WebExecutor into the dispatch path. nil clears it.
func (rt *Runtime) AttachWeb(web *kernel.WebExecutor) {
	if rt == nil {
		return
	}
	rt.Web = web
	rt.Executor.Web = web
}

// AttachFile wires a READ_ONLY FileExecutor into the dispatch path. nil clears it.
func (rt *Runtime) AttachFile(file *kernel.FileExecutor) {
	if rt == nil {
		return
	}
	rt.File = file
	rt.Executor.File = file
}

// mountTelegramWebhook layers a validated webhook route over the existing mux
// without altering dashboard/control paths. No-op when ingress is not webhook.
func mountTelegramWebhook(base http.Handler, ingress *telegram.Ingress) http.Handler {
	if base == nil || ingress == nil || ingress.Handler() == nil {
		return base
	}
	path := ingress.Config.WebhookPath
	if path == "" {
		path = "/telegram/webhook"
	}
	mux := http.NewServeMux()
	mux.Handle(path, ingress.Handler())
	mux.Handle("/", base)
	return mux
}

func openStore(opts Options) (port.Store, io.Closer, error) {
	switch opts.StoreBackend {
	case StorageMemory:
		return memory.New(), nil, nil
	case StorageSQLite:
		store, err := sqlite.Open(opts.SQLitePath)
		if err != nil {
			return nil, nil, fmt.Errorf("open sqlite store: %w", err)
		}
		return store, store, nil
	case StorageDolt:
		store, err := dolt.OpenServer(os.Getenv("DOLT_BIN"), opts.DoltPath)
		if err != nil {
			return nil, nil, fmt.Errorf("open dolt store: %w", err)
		}
		return store, store, nil
	default:
		return nil, nil, fmt.Errorf("unknown store backend %q", opts.StoreBackend)
	}
}

// Close flushes telemetry and closes durable storage. Safe to call multiple times.
func (rt *Runtime) Close(ctx context.Context) error {
	if rt == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var first error
	rt.mu.Lock()
	srv := rt.server
	peerSrv := rt.peerServer
	rt.server = nil
	rt.peerServer = nil
	rt.mu.Unlock()
	if srv != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := srv.Shutdown(shutdownCtx); err != nil && first == nil {
			first = err
		}
		cancel()
	}
	if peerSrv != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := peerSrv.Shutdown(shutdownCtx); err != nil && first == nil {
			first = err
		}
		cancel()
	}
	if rt.Telemetry != nil {
		if err := rt.Telemetry.Shutdown(ctx); err != nil && first == nil {
			first = err
		}
	}
	if rt.Closer != nil {
		if err := rt.Closer.Close(); err != nil && first == nil {
			first = err
		}
		rt.Closer = nil
	}
	return first
}

// CycleResult summarizes one control-loop iteration for tests and diagnostics.
type CycleResult struct {
	CommandsProcessed                 int
	EventsProcessed                   int
	MemoriesCompacted                 int
	RemindersScheduled                int
	DeliveriesProcessed               int
	TelegramFetched                   int
	TelegramAccepted                  int
	TelegramRejected                  int
	TelegramDuplicate                 int
	LeasesReconciled                  int
	ModelCompletionReceiptsReconciled int
	SubagentsReconciled               int
	SubagentDispatches                int
	SubagentEffectsReconciled         int
	RemoteSubagentsExecuted           int
	SubagentStatusesDispatched        int
	SubagentStatusesApplied           int
	SubagentIngressRetryAttempts      int
	SubagentIngressRetries            int
	SubagentIngressConflicts          int
	SubagentIngressExhaustions        int
	SubagentIngressRetrySleep         time.Duration
	SubagentIngressRecoveryDelay      time.Duration
	SchedulerRan                      bool
	SchedulerSteps                    int
	SchedulerKind                     kernel.DecisionKind
	OperationsExecuted                int
	OperationsSkipped                 int
	DispatchBudgetHit                 bool
	CycleBudgetHit                    bool
	CadenceVersion                    string
	Worked                            bool
	Stopping                          bool
}

// ProcessCycle drains inboxes and steps the scheduler under cadence budgets.
// MaxDispatches bounds productive DISPATCH decisions per cycle; MaxCycleDuration
// is a soft wall-clock budget that stops starting new scheduler steps.
// It never busy-polls: callers sleep when Worked is false.
func (rt *Runtime) ProcessCycle(ctx context.Context) (CycleResult, error) {
	if rt == nil {
		return CycleResult{}, errors.New("runtime is nil")
	}
	if ctx == nil {
		return CycleResult{}, errors.New("context is required")
	}

	var result CycleResult
	cycleStarted := rt.Clock.Now().UTC()
	_ = rt.reloadModelExecutorIfNeeded(ctx)
	ctx, span := rt.Telemetry.TraceControl(ctx, "runtime.control_cycle", "control_cycle", string(rt.Opts.MissionID), "")
	defer func() {
		outcome := "idle"
		if result.Worked {
			outcome = "worked"
		}
		if result.Stopping {
			outcome = "stopping"
		}
		observability.EndControl(span, outcome, "")
		if rt.cycleTelemetry != nil {
			rt.cycleTelemetry.Record(ctx, observability.CycleSnapshot{
				Outcome:                      outcome,
				CommandsProcessed:            result.CommandsProcessed,
				EventsProcessed:              result.EventsProcessed,
				OperationsExecuted:           result.OperationsExecuted,
				OperationsSkipped:            result.OperationsSkipped,
				LeasesReconciled:             result.LeasesReconciled,
				SubagentIngressRetryAttempts: result.SubagentIngressRetryAttempts,
				SubagentIngressRetries:       result.SubagentIngressRetries,
				SubagentIngressConflicts:     result.SubagentIngressConflicts,
				SubagentIngressExhaustions:   result.SubagentIngressExhaustions,
				SubagentIngressRetrySleep:    result.SubagentIngressRetrySleep,
				SubagentIngressRecoveryDelay: result.SubagentIngressRecoveryDelay,
				SchedulerRan:                 result.SchedulerRan,
				SchedulerKind:                string(result.SchedulerKind),
				Worked:                       result.Worked,
				Stopping:                     result.Stopping,
			})
		}
	}()

	// Operator commands first: pause/shutdown must gate subsequent dispatch.
	for i := 0; i < rt.Opts.MaxInboxBatch; i++ {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		_, ok, err := rt.CommandProcessor.ProcessNext(ctx)
		if err != nil {
			return result, fmt.Errorf("command processor: %w", err)
		}
		if !ok {
			break
		}
		result.CommandsProcessed++
		result.Worked = true
	}

	// Make durable authenticated status evidence process-visible before enforcing
	// local deadlines. Supervisor remains the sole canonical lifecycle writer.
	if rt.SubagentStatusIngressWorker != nil {
		applied, retryReport, err := rt.SubagentStatusIngressWorker.ApplyPendingWithRetryReport(ctx)
		if err != nil && !errors.Is(err, retry.ErrBudgetExhausted) {
			return result, fmt.Errorf("subagent status ingress worker: %w", err)
		}
		result.SubagentStatusesApplied = applied
		result.SubagentIngressRetryAttempts = retryReport.Attempts
		result.SubagentIngressRetries = retryReport.Retries
		result.SubagentIngressConflicts = retryReport.Classes["conflict"]
		result.SubagentIngressExhaustions = retryReport.Exhaustions
		result.SubagentIngressRetrySleep = retryReport.SleepTotal
		if errors.Is(err, retry.ErrBudgetExhausted) {
			result.SubagentIngressRecoveryDelay = rt.Opts.SubagentIngressRecoveryDelay
		}
		if applied > 0 {
			result.Worked = true
		}
	}

	// Reconcile terminal/deadline subagent observations before draining external
	// events so completion wakes can affect the same bounded control cycle.
	if rt.Supervisor != nil {
		reconciled, err := rt.Supervisor.Reconcile(ctx)
		if err != nil {
			return result, fmt.Errorf("subagent supervisor: %w", err)
		}
		result.SubagentsReconciled = reconciled
		if reconciled > 0 {
			result.Worked = true
		}
	}
	if rt.SubagentEffectReconciler != nil {
		reconciled, err := rt.SubagentEffectReconciler.Reconcile(ctx)
		if err != nil {
			return result, fmt.Errorf("subagent effect reconciler: %w", err)
		}
		if reconciled > 0 {
			result.SubagentEffectsReconciled = reconciled
			result.Worked = true
		}
	}
	if rt.SubagentDispatcher != nil {
		dispatched, err := rt.SubagentDispatcher.DispatchDue(ctx)
		if err != nil {
			return result, fmt.Errorf("subagent dispatcher: %w", err)
		}
		result.SubagentDispatches = dispatched
		if dispatched > 0 {
			result.Worked = true
		}
	}
	if rt.RemoteSubagentWorker != nil {
		executed, err := rt.RemoteSubagentWorker.ExecuteDue(ctx)
		if err != nil {
			return result, fmt.Errorf("remote subagent worker: %w", err)
		}
		result.RemoteSubagentsExecuted = executed
		if executed > 0 {
			result.Worked = true
		}
	}
	if rt.SubagentStatusDispatcher != nil {
		dispatched, err := rt.SubagentStatusDispatcher.DispatchTerminal(ctx)
		if err != nil {
			return result, fmt.Errorf("subagent status dispatcher: %w", err)
		}
		result.SubagentStatusesDispatched = dispatched
		if dispatched > 0 {
			result.Worked = true
		}
	}

	// Channel ingress before event drain so polled answers apply this cycle.
	if err := rt.processTelegramIngress(ctx, &result); err != nil {
		return result, err
	}

	for i := 0; i < rt.Opts.MaxInboxBatch; i++ {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		_, ok, err := rt.EventProcessor.ProcessNext(ctx)
		if err != nil {
			return result, fmt.Errorf("external event processor: %w", err)
		}
		if !ok {
			break
		}
		result.EventsProcessed++
		result.Worked = true
	}

	compacted, err := rt.SemanticMemory.CompactExpired(ctx, rt.Opts.MemoryCompactionBatch)
	if err != nil {
		return result, fmt.Errorf("semantic memory compaction: %w", err)
	}
	result.MemoriesCompacted = compacted
	if compacted > 0 {
		result.Worked = true
	}

	stopping, err := rt.processStopping(ctx)
	if err != nil {
		return result, err
	}
	result.Stopping = stopping
	if stopping {
		return result, nil
	}

	// Channel outbox + reminder planning run after inbox drain and before
	// continuity scheduling so operator answers and deliveries stay timely.
	if err := rt.processQuestionChannel(ctx, &result); err != nil {
		return result, err
	}

	// Pre-dispatch recovery boundary: settle durable completion receipts before
	// selecting any new work. This releases permits left reserved when a process
	// stopped after committing provider evidence but before settlement. Failure
	// is fail-closed: no scheduler decision is made in that cycle.
	if rt.Model != nil && rt.Model.Authorizer != nil {
		settled, err := rt.Model.Authorizer.ReconcileModelCompletionReceipts(ctx, rt.Opts.ModelCompletionReceiptBatch)
		if err != nil {
			return result, fmt.Errorf("model completion receipt reconciliation: %w", err)
		}
		result.ModelCompletionReceiptsReconciled = settled
		if settled > 0 {
			result.Worked = true
		}
	}

	if rt.Opts.MissionID == "" {
		return result, nil
	}

	var missionRevision domain.MissionRevisionID
	err = rt.Store.View(ctx, func(r port.Reader) error {
		active, err := r.ActiveMissionRevision(rt.Opts.MissionID)
		if err != nil {
			return err
		}
		missionRevision = active.ID
		return nil
	})
	if errors.Is(err, port.ErrNotFound) {
		// Mission not installed yet: idle until loader or operator acts.
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("resolve active mission: %w", err)
	}

	// Reconcile expired leases before selecting new work so stuck RUNNING units
	// re-enter READY without process-local timers.
	reconcile, recErr := rt.LeaseReaper.Reconcile(ctx, missionRevision)
	if recErr != nil {
		return result, fmt.Errorf("lease reaper: %w", recErr)
	}
	result.LeasesReconciled = reconcile.Reconciled
	if reconcile.Reconciled > 0 {
		result.Worked = true
	}

	cadence, cadErr := kernel.ActiveSchedulerCadence(ctx, rt.Store)
	if cadErr != nil {
		return result, fmt.Errorf("scheduler cadence: %w", cadErr)
	}
	if err := cadence.Validate(); err != nil {
		return result, fmt.Errorf("scheduler cadence: %w", err)
	}
	result.CadenceVersion = cadence.Version
	maxDispatches := cadence.MaxDispatches
	if maxDispatches <= 0 {
		maxDispatches = 1
	}

	// Bounded multi-step schedule/dispatch under FR-RES-001 cycle budgets.
	// Productive DISPATCH counts toward MaxDispatches; non-dispatch decisions
	// (EXPAND/DIAGNOSE/CONTINUITY_BLOCKED) end the loop after one attempt so we
	// do not spin on empty replenishment inside a single cycle.
	for result.OperationsExecuted+result.OperationsSkipped < maxDispatches {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if !domain.WithinCycleBudget(cycleStarted, rt.Clock.Now().UTC(), cadence.MaxCycleDuration) {
			result.CycleBudgetHit = true
			break
		}

		decision, stepErr := rt.Scheduler.Step(ctx, missionRevision)
		if stepErr != nil {
			return result, fmt.Errorf("scheduler step: %w", stepErr)
		}
		result.SchedulerRan = true
		result.SchedulerSteps++
		result.SchedulerKind = decision.Kind
		// Continuity blocked without admission is still a completed step, but does
		// not count as productive work for idle backoff purposes.
		if decision.Kind == kernel.DecisionDispatch || decision.Kind == kernel.DecisionExpand {
			result.Worked = true
		}

		if decision.Kind != kernel.DecisionDispatch || decision.Operation == "" {
			// No more ready work this cycle (or expand/diagnose already done).
			break
		}

		// After DISPATCH, route to local or optional model/file/web executors.
		execResult, execErr := rt.Executor.Execute(ctx, decision.Operation)
		if execErr != nil {
			return result, fmt.Errorf("dispatch executor: %w", execErr)
		}
		if execResult.Completed {
			result.OperationsExecuted++
			result.Worked = true
		} else if execResult.Skipped {
			result.OperationsSkipped++
			// Skips still consume a dispatch slot so a missing provider cannot
			// monopolize the cycle with infinite requires_* attempts.
		} else {
			// Unexpected non-complete non-skip: stop to avoid thrash.
			break
		}
	}
	if result.OperationsExecuted+result.OperationsSkipped >= maxDispatches && result.SchedulerRan {
		// Mark budget hit only when we actually reached the cap with dispatch activity.
		if result.OperationsExecuted+result.OperationsSkipped > 0 {
			result.DispatchBudgetHit = result.OperationsExecuted+result.OperationsSkipped >= maxDispatches
		}
	}
	return result, nil
}

func (rt *Runtime) processStopping(ctx context.Context) (bool, error) {
	var mode domain.ProcessMode
	err := rt.Store.View(ctx, func(r port.Reader) error {
		state, err := r.ControlState()
		if errors.Is(err, port.ErrNotFound) {
			mode = domain.ProcessRunning
			return nil
		}
		if err != nil {
			return err
		}
		mode = state.ProcessMode
		return nil
	})
	if err != nil {
		return false, err
	}
	return mode == domain.ProcessStopping || mode == domain.ProcessStopped, nil
}

// RunHTTP starts the HTTP server and blocks until ctx is cancelled or Listen fails.
func (rt *Runtime) RunHTTP(ctx context.Context) error {
	if rt == nil || rt.Handler == nil {
		return errors.New("runtime handler is not assembled")
	}
	srv := &http.Server{
		Addr:              rt.Opts.ListenAddr,
		Handler:           rt.Handler,
		ReadHeaderTimeout: 5 * time.Second,
		// Long-lived SSE timelines may exceed default timeouts; handlers own
		// their own deadlines for write paths that need them.
		ReadTimeout:  0,
		WriteTimeout: 0,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
	}
	var peerSrv *http.Server
	if rt.Peer != nil && rt.Opts.PeerBindAddr != "" {
		handler, err := peerhttp.NewServerHandler(rt.Peer.Handler)
		if err != nil {
			return fmt.Errorf("peer server handler: %w", err)
		}
		cfg := network.PeerConfig{
			NodeCert: rt.Opts.PeerCert,
			NodeKey:  rt.Opts.PeerKey,
			CACert:   rt.Opts.PeerCACert,
		}
		tlsConfig, err := network.LoadMTLSConfig(cfg)
		if err != nil {
			return fmt.Errorf("peer server tls: %w", err)
		}
		peerSrv = &http.Server{
			Addr:              rt.Opts.PeerBindAddr,
			Handler:           handler,
			TLSConfig:         tlsConfig,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			BaseContext: func(net.Listener) context.Context {
				return ctx
			},
		}
	}

	rt.mu.Lock()
	rt.server = srv
	rt.peerServer = peerSrv
	rt.mu.Unlock()

	errCh := make(chan error, 2)
	go func() {
		rt.logger.Printf("runtime listening on http://%s (store=%s dashboard=%v otel=%v)",
			rt.Opts.ListenAddr, rt.Opts.StoreBackend, rt.Opts.EnableDashboard, rt.Telemetry.Enabled())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	if peerSrv != nil {
		go func() {
			rt.logger.Printf("peer rpc listening on mTLS https://%s", rt.Opts.PeerBindAddr)
			if err := peerSrv.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
				return
			}
			errCh <- nil
		}()
	}

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		if peerSrv != nil {
			_ = peerSrv.Shutdown(shutdownCtx)
		}
		return nil // Graceful stop, ignore listener return channel
	case err := <-errCh:
		// If either listener fails eagerly, return immediately.
		return err
	}
}

// RunControlLoop repeatedly processes cycles until ctx ends or graceful stop.
// Empty cycles sleep with exponential backoff between cadence MinIdleSleep and
// MaxIdleSleep when a SCHEDULER revision is active; otherwise Options IdleMin/Max.
func (rt *Runtime) RunControlLoop(ctx context.Context) error {
	if rt == nil {
		return errors.New("runtime is nil")
	}
	if rt.Peer != nil && rt.Peer.Sync != nil {
		go func() {
			if err := rt.Peer.Sync.Run(ctx); err != nil && ctx.Err() == nil {
				rt.logger.Printf("peer sync loop exited: %v", err)
			}
		}()
	}
	idleMin, idleMax := rt.Opts.IdleMin, rt.Opts.IdleMax
	idle := idleMin
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Refresh durable cadence each iteration so HOT/NEXT_CYCLE applies without restart.
		if cad, err := kernel.ActiveSchedulerCadence(ctx, rt.Store); err == nil && cad.Validate() == nil {
			if cad.MinIdleSleep > 0 {
				idleMin = cad.MinIdleSleep
			}
			if cad.MaxIdleSleep > 0 {
				idleMax = cad.MaxIdleSleep
			}
			if idleMin > idleMax && idleMax > 0 {
				idleMin = idleMax
			}
		}
		result, err := rt.ProcessCycle(ctx)
		if err != nil {
			return err
		}
		if result.Stopping {
			rt.logger.Printf("runtime control loop exiting: process stopping")
			return nil
		}
		delay, nextIdle := nextControlCycleDelay(result, idle, idleMin, idleMax)
		idle = nextIdle
		if delay <= 0 {
			continue
		}
		// Bounded wait — never spin. Clock is injectable for tests via WaitUntil.
		deadline := rt.Clock.Now().UTC().Add(delay)
		if err := rt.Clock.WaitUntil(ctx, deadline); err != nil {
			return err
		}
	}
}

func nextControlCycleDelay(result CycleResult, idle, idleMin, idleMax time.Duration) (time.Duration, time.Duration) {
	if result.SubagentIngressRecoveryDelay > 0 {
		return result.SubagentIngressRecoveryDelay, idleMin
	}
	if result.Worked {
		return 0, idleMin
	}
	next := idle
	if next < idleMax {
		next *= 2
		if next > idleMax {
			next = idleMax
		}
	}
	return idle, next
}

// continuityCatalogFromRegistry projects the process StrategyRegistry into an
// inspect-safe catalogue. Nil/empty registries yield a zero catalogue (not set).
func continuityCatalogFromRegistry(reg *kernel.StrategyRegistry) inspect.ContinuityStrategyCatalog {
	if reg == nil || reg.Len() == 0 {
		return inspect.ContinuityStrategyCatalog{}
	}
	snap := reg.Snapshot()
	descriptors := make([]inspect.ContinuityStrategyDescriptor, 0, len(snap.Descriptors))
	for _, d := range snap.Descriptors {
		descriptors = append(descriptors, inspect.ContinuityStrategyDescriptor{
			Name:            d.Name,
			Family:          d.Family,
			Version:         d.Version,
			Priority:        d.Priority,
			RequiresModel:   d.RequiresModel,
			RequiresNetwork: d.RequiresNetwork,
			LocalOnly:       d.LocalOnly,
			Ref:             d.Ref(),
		})
	}
	return inspect.BuildContinuityStrategyCatalog(snap.CatalogVersion, descriptors)
}
