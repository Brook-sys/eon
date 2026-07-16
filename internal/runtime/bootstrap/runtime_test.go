package bootstrap_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/mission"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/bootstrap"
	"motor-autonomo/internal/runtime/source"
)

func TestOpenAssemblesHTTPSurfaces(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	rt, err := bootstrap.Open(ctx, bootstrap.Options{
		ListenAddr:      "127.0.0.1:0",
		StoreBackend:    bootstrap.StorageMemory,
		EnableDashboard: true,
		RuntimeName:     "test-runtime",
		RuntimeVersion:  "test",
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })

	req := httptest.NewRequest(http.MethodGet, "/api/inspect/health", nil)
	rec := httptest.NewRecorder()
	rt.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("health status = %d body=%s", rec.Code, rec.Body.String())
	}
	var health map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &health); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if health["status"] != "ok" {
		t.Fatalf("health payload = %#v", health)
	}

	req = httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec = httptest.NewRecorder()
	rt.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d", rec.Code)
	}
}

func TestProcessCycleDrainsCommandAndStops(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	rt, err := bootstrap.Open(ctx, bootstrap.Options{
		StoreBackend: bootstrap.StorageMemory,
		IdleMin:      time.Millisecond,
		IdleMax:      2 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })

	result, err := rt.ProcessCycle(ctx)
	if err != nil {
		t.Fatalf("empty cycle: %v", err)
	}
	if result.Worked || result.CommandsProcessed != 0 {
		t.Fatalf("expected idle empty cycle, got %#v", result)
	}

	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	shutdown := domain.OperatorCommand{
		SchemaVersion:  domain.SchemaVersionV1,
		ID:             "cmd_shutdown_1",
		Kind:           domain.CommandGracefulShutdown,
		ActorType:      domain.ActorOperator,
		ActorID:        "operator_test",
		IdempotencyKey: "idem_shutdown_1",
		SubmittedAt:    now,
		Reason:         "test stop",
	}
	if err := shutdown.Validate(); err != nil {
		t.Fatalf("validate shutdown: %v", err)
	}
	if _, err := rt.Commands.SubmitCommand(shutdown); err != nil {
		t.Fatalf("submit shutdown: %v", err)
	}

	result, err = rt.ProcessCycle(ctx)
	if err != nil {
		t.Fatalf("shutdown cycle: %v", err)
	}
	if result.CommandsProcessed != 1 || !result.Worked || !result.Stopping {
		t.Fatalf("expected one command + stopping, got %#v", result)
	}

	result, err = rt.ProcessCycle(ctx)
	if err != nil {
		t.Fatalf("post-stop cycle: %v", err)
	}
	if !result.Stopping || result.CommandsProcessed != 0 {
		t.Fatalf("expected clean stopping cycle, got %#v", result)
	}
}

func TestProcessCycleSchedulerWithMission(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	rt, err := bootstrap.Open(ctx, bootstrap.Options{
		StoreBackend: bootstrap.StorageMemory,
		MissionID:    "mission_bootstrap",
		IdleMin:      time.Millisecond,
		IdleMax:      2 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })

	loader := mission.Loader{
		Store: rt.Store,
		Clock: rt.Clock,
		IDs:   rt.IDs,
	}
	specJSON := []byte(`{
  "schema_version": 1,
  "id": "mission_bootstrap",
  "revision": 1,
  "original_text": "bootstrap mission for control loop",
  "purpose": "exercise scheduler wiring",
  "domains": ["test"],
  "policies": ["policy.v1"],
  "budget": {"model_calls": 10, "tokens": 1024, "bytes": 4096, "attempts": 3, "duration": 60000000000},
  "status": "ACTIVE"
}`)
	if _, err := loader.Load(ctx, specJSON, "bootstrap:test"); err != nil {
		t.Fatalf("install mission: %v", err)
	}

	if err := kernel.EnsureCatalogSpecs(ctx, rt.Store, nil); err != nil {
		t.Fatalf("catalog: %v", err)
	}

	result, err := rt.ProcessCycle(ctx)
	if err != nil {
		t.Fatalf("scheduler cycle: %v", err)
	}
	if !result.SchedulerRan {
		t.Fatalf("expected scheduler to run, got %#v", result)
	}
	switch result.SchedulerKind {
	case kernel.DecisionDispatch, kernel.DecisionExpand, kernel.DecisionContinuityBlocked, kernel.DecisionDiagnose:
	default:
		t.Fatalf("unexpected scheduler kind %q", result.SchedulerKind)
	}

	err = rt.Store.View(ctx, func(r port.Reader) error {
		_, err := r.ActiveMissionRevision("mission_bootstrap")
		return err
	})
	if err != nil {
		t.Fatalf("active mission after cycle: %v", err)
	}
}

func TestOptionsValidateDefaults(t *testing.T) {
	t.Parallel()
	opts := bootstrap.Options{}
	if err := opts.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if opts.ListenAddr == "" || opts.StoreBackend != bootstrap.StorageMemory {
		t.Fatalf("unexpected defaults: %#v", opts)
	}
	if opts.DeliveryBatch != 8 || opts.DeliveryLease <= 0 {
		t.Fatalf("delivery defaults missing: %#v", opts)
	}
	if err := (&bootstrap.Options{StoreBackend: bootstrap.StorageSQLite}).Validate(); err == nil {
		t.Fatal("sqlite without path should fail")
	}
	if err := (&bootstrap.Options{Telegram: &bootstrap.TelegramOptions{Enabled: true, TokenEnv: "X"}}).Validate(); err == nil {
		t.Fatal("enabled telegram without allowlists should fail")
	}
}

func TestProcessCycleRunsTelegramOutboxWorker(t *testing.T) {
	// Cannot t.Parallel: mutates process env for Telegram token injection.
	ctx := context.Background()
	// Worker uses system clock; seed must already be due.
	now := time.Now().UTC().Add(-time.Minute)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":77}}`))
	}))
	t.Cleanup(server.Close)

	t.Setenv("MOTOR_TELEGRAM_TOKEN_TEST", "test-token")
	rt, err := bootstrap.Open(ctx, bootstrap.Options{
		StoreBackend:  bootstrap.StorageMemory,
		IdleMin:       time.Millisecond,
		IdleMax:       2 * time.Millisecond,
		DeliveryBatch: 4,
		DeliveryLease: time.Minute,
		DeliveryRetry: time.Minute,
		QuestionRoutes: []bootstrap.QuestionRouteConfig{{
			Channel: "telegram", DestinationRef: "operator_primary", MaxAttempts: 3,
		}},
		Telegram: &bootstrap.TelegramOptions{
			Enabled:       true,
			TokenEnv:      "MOTOR_TELEGRAM_TOKEN_TEST",
			BaseURL:       server.URL,
			Destinations:  map[string]int64{"operator_primary": 100},
			AllowedActors: map[int64]string{7: "operator_1"},
			AllowedChats:  map[int64]struct{}{100: {}},
			WorkerOwner:   "bootstrap-test",
		},
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })
	if rt.TelegramWorker == nil || rt.TelegramAdapter == nil {
		t.Fatal("expected telegram surfaces wired")
	}

	question := domain.OperatorQuestion{
		SchemaVersion: domain.SchemaVersionV1, ID: "ask_bootstrap_1", MissionID: "mission_bootstrap_delivery",
		MissionRevision: "rev_bootstrap_delivery", Revision: 1, Kind: domain.QuestionSingleChoice,
		Prompt: "Choose", Context: "bootstrap outbox wiring", Options: []domain.QuestionOption{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}},
		AllowSkip: true, FallbackPolicy: domain.QuestionContinueOtherWork, DedupSignature: "bootstrap-choose",
		Priority: 40, Status: domain.OperatorQuestionPending, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	delivery := domain.QuestionDelivery{
		SchemaVersion: 1, ID: "delivery_bootstrap_1", QuestionID: question.ID, QuestionRevision: 1,
		Channel: "telegram", DestinationRef: "operator_primary", Status: domain.QuestionDeliveryPending,
		MaxAttempts: 3, AvailableAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := rt.Store.Update(ctx, func(tx port.Transaction) error {
		mission := domain.MissionRevision{
			SchemaVersion: 1, ID: question.MissionRevision, MissionID: question.MissionID, Revision: 1,
			OriginalText: "bootstrap delivery", Purpose: "test", Status: domain.MissionActive,
			Provenance: "test", AcceptedAt: now,
		}
		if err := tx.AppendMissionRevision(mission); err != nil {
			return err
		}
		if err := tx.CreateOperatorQuestion(question); err != nil {
			return err
		}
		return tx.CreateQuestionDelivery(delivery)
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	result, err := rt.ProcessCycle(ctx)
	if err != nil {
		t.Fatalf("cycle: %v", err)
	}
	if result.DeliveriesProcessed != 1 || !result.Worked {
		t.Fatalf("expected one delivery, got %#v", result)
	}
	err = rt.Store.View(ctx, func(r port.Reader) error {
		got, err := r.QuestionDelivery(delivery.ID)
		if err != nil {
			return err
		}
		if got.Status != domain.QuestionDeliveryDelivered || got.TransportMessageID != "77" {
			t.Fatalf("delivery = %#v", got)
		}
		byTransport, err := r.QuestionDeliveryByTransport("telegram", "77")
		if err != nil {
			return err
		}
		if byTransport.ID != delivery.ID {
			t.Fatalf("transport lookup = %#v", byTransport)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestControlLoopIdleUsesClock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt, err := bootstrap.Open(ctx, bootstrap.Options{
		StoreBackend: bootstrap.StorageMemory,
		IdleMin:      20 * time.Millisecond,
		IdleMax:      40 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close(context.Background()) })

	clock := source.NewManualClock(time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC))
	rt.Clock = clock

	done := make(chan error, 1)
	go func() { done <- rt.RunControlLoop(ctx) }()

	for i := 0; i < 5; i++ {
		if err := clock.Advance(50 * time.Millisecond); err != nil {
			t.Fatalf("advance: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Fatalf("control loop exit: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("control loop did not exit")
	}
}
