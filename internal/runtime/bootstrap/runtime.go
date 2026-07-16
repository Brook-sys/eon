package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"motor-autonomo/internal/channel/telegram"
	"motor-autonomo/internal/control"
	"motor-autonomo/internal/dashboard"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/inspect"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/observability"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
	"motor-autonomo/internal/storage/sqlite"
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

	CommandProcessor *kernel.CommandProcessor
	EventProcessor   *kernel.ExternalEventProcessor
	ConfigApplier    *kernel.ConfigApplier
	Scheduler        kernel.Scheduler
	// Executor routes local continuity and optional PROPOSE_ONLY model paths.
	Executor kernel.DispatchExecutor
	// LeaseReaper reconciles expired RUNNING/VERIFYING leases before dispatch.
	LeaseReaper kernel.LeaseReaper
	// Model is optional; nil keeps non-local ops skipped as requires_model.
	Model     *kernel.ModelExecutor
	Registry  *kernel.StrategyRegistry
	Cooldowns *kernel.StrategyCooldownBook

	Inspect   *inspect.API
	Control   *control.API
	Dashboard *dashboard.Server
	Handler   http.Handler

	// Optional non-authoritative Telegram surfaces. Nil when disabled.
	TelegramAdapter *telegram.Adapter
	TelegramWorker  *telegram.DeliveryWorker
	TelegramIngress *telegram.Ingress

	Telemetry *observability.Runtime

	logger *log.Logger
	mu     sync.Mutex
	server *http.Server
}

// Open assembles dependencies. It does not start the HTTP server or loop.
func Open(ctx context.Context, opts Options) (*Runtime, error) {
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
		Store:     store,
		Clock:     clock,
		Registry:  registry,
		IDs:       ids,
		Cooldowns: cooldowns,
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

	telegramBits, err := buildTelegram(opts, store, clock, eventInbox, ids)
	if err != nil {
		_ = telemetry.Shutdown(ctx)
		if closer != nil {
			_ = closer.Close()
		}
		return nil, fmt.Errorf("telegram channel: %w", err)
	}

	var dash *dashboard.Server
	var handler http.Handler
	if opts.EnableDashboard {
		dash, err = dashboard.New(inspectAPI.Handler(), controlAPI.Handler())
		if err != nil {
			_ = telemetry.Shutdown(ctx)
			if closer != nil {
				_ = closer.Close()
			}
			return nil, err
		}
		dash.DefaultMissionID = string(opts.MissionID)
		handler = dash.Handler()
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
	modelExec, err := buildModel(opts, store, clock, ids, telemetry)
	if err != nil {
		_ = telemetry.Shutdown(ctx)
		if closer != nil {
			_ = closer.Close()
		}
		return nil, fmt.Errorf("model provider: %w", err)
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
		CommandProcessor: commandProcessor,
		EventProcessor:   eventProcessor,
		ConfigApplier:    configApplier,
		Scheduler:        scheduler,
		Executor: kernel.DispatchExecutor{
			Store: store,
			Local: localExec,
			Model: modelExec,
		},
		LeaseReaper:     leaseReaper,
		Model:           modelExec,
		Registry:        registry,
		Cooldowns:       cooldowns,
		Inspect:         inspectAPI,
		Control:         controlAPI,
		Dashboard:       dash,
		Handler:         handler,
		TelegramAdapter: telegramBits.Adapter,
		TelegramWorker:  telegramBits.Worker,
		TelegramIngress: telegramBits.Ingress,
		Telemetry:       telemetry,
		logger:          log.Default(),
	}, nil
}

// AttachModel wires a PROPOSE_ONLY ModelExecutor into the dispatch path.
// Safe to call after Open for tests or process configuration with a free local
// OpenAI-compatible endpoint. nil clears the model path.
func (rt *Runtime) AttachModel(model *kernel.ModelExecutor) {
	if rt == nil {
		return
	}
	rt.Model = model
	rt.Executor.Model = model
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
	rt.server = nil
	rt.mu.Unlock()
	if srv != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := srv.Shutdown(shutdownCtx); err != nil && first == nil {
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
	CommandsProcessed   int
	EventsProcessed     int
	RemindersScheduled  int
	DeliveriesProcessed int
	TelegramFetched     int
	TelegramAccepted    int
	TelegramRejected    int
	TelegramDuplicate   int
	LeasesReconciled    int
	SchedulerRan        bool
	SchedulerKind       kernel.DecisionKind
	OperationsExecuted  int
	OperationsSkipped   int
	Worked              bool
	Stopping            bool
}

// ProcessCycle drains inboxes and optionally steps the scheduler once.
// It never busy-polls: callers sleep when Worked is false.
func (rt *Runtime) ProcessCycle(ctx context.Context) (CycleResult, error) {
	if rt == nil {
		return CycleResult{}, errors.New("runtime is nil")
	}
	if ctx == nil {
		return CycleResult{}, errors.New("context is required")
	}

	var result CycleResult
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

	decision, err := rt.Scheduler.Step(ctx, missionRevision)
	if err != nil {
		return result, fmt.Errorf("scheduler step: %w", err)
	}
	result.SchedulerRan = true
	result.SchedulerKind = decision.Kind
	// Continuity blocked without admission is still a completed step, but does
	// not count as productive work for idle backoff purposes.
	if decision.Kind == kernel.DecisionDispatch || decision.Kind == kernel.DecisionExpand {
		result.Worked = true
	}

	// After DISPATCH, route to local or optional model executor. Non-local
	// specs without a wired provider skip with requires_model.
	if decision.Kind == kernel.DecisionDispatch && decision.Operation != "" {
		execResult, execErr := rt.Executor.Execute(ctx, decision.Operation)
		if execErr != nil {
			return result, fmt.Errorf("dispatch executor: %w", execErr)
		}
		if execResult.Completed {
			result.OperationsExecuted++
			result.Worked = true
		} else if execResult.Skipped {
			result.OperationsSkipped++
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
	rt.mu.Lock()
	rt.server = srv
	rt.mu.Unlock()

	errCh := make(chan error, 1)
	go func() {
		rt.logger.Printf("runtime listening on http://%s (store=%s dashboard=%v otel=%v)",
			rt.Opts.ListenAddr, rt.Opts.StoreBackend, rt.Opts.EnableDashboard, rt.Telemetry.Enabled())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return <-errCh
	case err := <-errCh:
		return err
	}
}

// RunControlLoop repeatedly processes cycles until ctx ends or graceful stop.
// Empty cycles sleep with exponential backoff between IdleMin and IdleMax.
func (rt *Runtime) RunControlLoop(ctx context.Context) error {
	if rt == nil {
		return errors.New("runtime is nil")
	}
	idle := rt.Opts.IdleMin
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		result, err := rt.ProcessCycle(ctx)
		if err != nil {
			return err
		}
		if result.Stopping {
			rt.logger.Printf("runtime control loop exiting: process stopping")
			return nil
		}
		if result.Worked {
			idle = rt.Opts.IdleMin
			continue
		}
		// Bounded wait — never spin. Clock is injectable for tests via WaitUntil.
		deadline := rt.Clock.Now().UTC().Add(idle)
		if err := rt.Clock.WaitUntil(ctx, deadline); err != nil {
			return err
		}
		if idle < rt.Opts.IdleMax {
			idle *= 2
			if idle > rt.Opts.IdleMax {
				idle = rt.Opts.IdleMax
			}
		}
	}
}
