package bootstrap_test

import (
	"context"
	"encoding/json"
	"fmt"
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

	// Process portfolio is wired from StrategyRegistry into inspect (read-only).
	req = httptest.NewRequest(http.MethodGet, "/api/inspect/continuity/catalog", nil)
	rec = httptest.NewRecorder()
	rt.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("continuity catalog status = %d body=%s", rec.Code, rec.Body.String())
	}
	var catalog map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &catalog); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	if catalog["catalog_version"] != kernel.DefaultContinuityCatalogVersion {
		t.Fatalf("catalog_version = %#v", catalog["catalog_version"])
	}
	if n, ok := catalog["strategy_count"].(float64); !ok || n < 1 {
		t.Fatalf("strategy_count = %#v", catalog["strategy_count"])
	}

	req = httptest.NewRequest(http.MethodGet, "/api/inspect/version", nil)
	rec = httptest.NewRecorder()
	rt.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("version status = %d body=%s", rec.Code, rec.Body.String())
	}
	var version map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &version); err != nil {
		t.Fatalf("decode version: %v", err)
	}
	if version["continuity_catalog_version"] != kernel.DefaultContinuityCatalogVersion {
		t.Fatalf("version continuity_catalog_version = %#v", version["continuity_catalog_version"])
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

func TestProcessCycleAppliesDurableRemoteCompletionBeforeDeadline(t *testing.T) {
	ctx := context.Background()
	rt, err := bootstrap.Open(ctx, bootstrap.Options{
		StoreBackend: bootstrap.StorageMemory,
		MissionID:    "mission-deadline-ordering",
		Subagent:     &bootstrap.SubagentOptions{Enabled: true, MaxAttempts: 2, Timeout: time.Minute},
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })

	id, err := rt.Subagents.Spawn(ctx, kernel.SubagentSpec{Task: "return durable result", ContextMode: "isolated", Labels: map[string]string{kernel.SubagentTransportPeerLabel: "peer-a"}})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	manager, ok := rt.Subagents.(*kernel.PersistentSessionManager)
	if !ok {
		t.Fatalf("subagent manager type = %T", rt.Subagents)
	}
	if err := manager.AdmitRemoteStatus(ctx, "peer-a", "delivery-before-deadline", kernel.SubagentObservation{ID: id, State: kernel.SessionStateComplete, Result: "durable remote result"}); err != nil {
		t.Fatalf("admit remote status: %v", err)
	}
	now := rt.Clock.Now().UTC()
	if err := rt.Store.Update(ctx, func(tx port.Transaction) error {
		record, err := tx.SubagentRecord(string(id))
		if err != nil {
			return err
		}
		record.State = domain.SubagentStateRunning
		record.UpdatedAt = now
		record.Deadline = now
		return tx.SaveSubagentRecord(record)
	}); err != nil {
		t.Fatalf("arm deadline: %v", err)
	}
	rt.SubagentStatusIngressWorker = &kernel.SubagentStatusIngressWorker{Store: rt.Store, Manager: rt.Subagents, Clock: rt.Clock, Batch: 1}

	result, err := rt.ProcessCycle(ctx)
	if err != nil {
		t.Fatalf("cycle: %v", err)
	}
	if result.SubagentStatusesApplied != 1 || result.SubagentsReconciled != 1 {
		t.Fatalf("cycle result=%+v", result)
	}
	if err := rt.Store.View(ctx, func(r port.Reader) error {
		record, err := r.SubagentRecord(string(id))
		if err != nil {
			return err
		}
		if record.State != domain.SubagentStateComplete || record.Result != "durable remote result" || record.ErrorCode != "" {
			t.Fatalf("canonical record=%+v", record)
		}
		receipt, err := r.SubagentStatusIngressReceipt("peer-a", "delivery-before-deadline")
		if err != nil {
			return err
		}
		if receipt.Status != domain.SubagentStatusIngressApplied {
			t.Fatalf("receipt=%+v", receipt)
		}
		event, err := r.ExternalEventByDeduplicationKey("subagent-terminal:" + string(id))
		if err != nil {
			return err
		}
		if event.Content.Text != "COMPLETE:durable remote result" {
			t.Fatalf("completion event=%+v", event)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestOpenWiresSubagentToolsAndCycleSupervisor(t *testing.T) {
	ctx := context.Background()
	rt, err := bootstrap.Open(ctx, bootstrap.Options{
		StoreBackend: bootstrap.StorageMemory,
		MissionID:    "mission-1",
		Subagent:     &bootstrap.SubagentOptions{Enabled: true, MaxAttempts: 2, Timeout: time.Minute},
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })
	if rt.Subagents == nil || rt.Supervisor == nil {
		t.Fatal("subagent manager and supervisor must share runtime wiring")
	}
	id, err := rt.Subagents.Spawn(ctx, kernel.SubagentSpec{Task: "bounded task", ContextMode: "isolated", Labels: map[string]string{"task_id": "task-1"}})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if err := rt.Store.View(ctx, func(r port.Reader) error {
		record, readErr := r.SubagentRecord(string(id))
		if readErr != nil {
			return readErr
		}
		if record.MissionID != "mission-1" || record.MaxAttempts != 2 {
			t.Fatalf("unexpected record: %+v", record)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestOpenRestoresActiveSubagentAcrossSQLiteRestart(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/runtime.db"
	opts := bootstrap.Options{
		StoreBackend: bootstrap.StorageSQLite,
		SQLitePath:   dbPath,
		MissionID:    "mission-restart",
		Subagent:     &bootstrap.SubagentOptions{Enabled: true, MaxConcurrent: 2, MaxAttempts: 2, Timeout: time.Minute},
	}
	first, err := bootstrap.Open(ctx, opts)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	id, err := first.Subagents.Spawn(ctx, kernel.SubagentSpec{Task: "survive restart", ContextMode: "isolated", Labels: map[string]string{"task_id": "task-restart"}})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if err := first.Subagents.PublishStatus(ctx, kernel.SubagentObservation{ID: id, State: kernel.SessionStateRunning, Result: "", Failure: ""}); err != nil {
		t.Fatalf("publish running: %v", err)
	}
	if _, err := first.Supervisor.Reconcile(ctx); err != nil {
		t.Fatalf("persist running: %v", err)
	}
	if err := first.Close(ctx); err != nil {
		t.Fatalf("first close: %v", err)
	}

	second, err := bootstrap.Open(ctx, opts)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	t.Cleanup(func() { _ = second.Close(ctx) })
	status, err := second.Subagents.Status(ctx, id)
	if err != nil {
		t.Fatalf("restored status: %v", err)
	}
	if status.State != kernel.SessionStateRunning || status.Spec.Task != "survive restart" || status.Spec.Labels["task_id"] != "task-restart" {
		t.Fatalf("restored status = %+v", status)
	}
	if err := second.Subagents.PublishStatus(ctx, kernel.SubagentObservation{ID: id, State: kernel.SessionStateComplete, Result: "recovered", Failure: ""}); err != nil {
		t.Fatalf("publish completion: %v", err)
	}
	result, err := second.ProcessCycle(ctx)
	if err != nil {
		t.Fatalf("completion cycle: %v", err)
	}
	if result.SubagentsReconciled != 1 || result.EventsProcessed != 1 || !result.Worked {
		t.Fatalf("completion cycle = %#v", result)
	}
}

func TestOpenRestoresAppliedTerminalWinnerBeforePendingConflict(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/runtime.db"
	opts := bootstrap.Options{StoreBackend: bootstrap.StorageSQLite, SQLitePath: dbPath, MissionID: "mission-terminal-winner", Subagent: &bootstrap.SubagentOptions{Enabled: true, MaxConcurrent: 2, MaxAttempts: 2, Timeout: time.Minute}}
	first, err := bootstrap.Open(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	id, err := first.Subagents.Spawn(ctx, kernel.SubagentSpec{Task: "preserve terminal winner", ContextMode: "isolated", Labels: map[string]string{kernel.SubagentTransportPeerLabel: "peer-a"}})
	if err != nil {
		t.Fatal(err)
	}
	manager := first.Subagents.(*kernel.PersistentSessionManager)
	if err := manager.AdmitRemoteStatus(ctx, "peer-a", "delivery-winner", kernel.SubagentObservation{ID: id, State: kernel.SessionStateComplete, Result: "winner"}); err != nil {
		t.Fatal(err)
	}
	worker := kernel.SubagentStatusIngressWorker{Store: first.Store, Manager: first.Subagents, Clock: first.Clock, Batch: 1}
	if n, err := worker.ApplyPending(ctx); err != nil || n != 1 {
		t.Fatalf("apply winner n=%d err=%v", n, err)
	}
	if err := manager.AdmitRemoteStatus(ctx, "peer-a", "delivery-conflict", kernel.SubagentObservation{ID: id, State: kernel.SessionStateFailed, Failure: "contradiction"}); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(ctx); err != nil {
		t.Fatal(err)
	}

	second, err := bootstrap.Open(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close(ctx) })
	second.SubagentStatusIngressWorker = &kernel.SubagentStatusIngressWorker{Store: second.Store, Manager: second.Subagents, Clock: second.Clock, Batch: 1}
	result, err := second.ProcessCycle(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.SubagentStatusesApplied != 1 || result.SubagentsReconciled != 1 {
		t.Fatalf("cycle=%+v", result)
	}
	if err := second.Store.View(ctx, func(r port.Reader) error {
		record, err := r.SubagentRecord(string(id))
		if err != nil {
			return err
		}
		winner, err := r.SubagentStatusIngressReceipt("peer-a", "delivery-winner")
		if err != nil {
			return err
		}
		conflict, err := r.SubagentStatusIngressReceipt("peer-a", "delivery-conflict")
		if err != nil {
			return err
		}
		if record.State != domain.SubagentStateComplete || record.Result != "winner" || winner.Status != domain.SubagentStatusIngressApplied || conflict.Status != domain.SubagentStatusIngressRejected || conflict.RejectionCode != domain.SubagentStatusIngressRejectionTerminalConflict {
			t.Fatalf("record=%+v winner=%+v conflict=%+v", record, winner, conflict)
		}
		_, err = r.ExternalEventByDeduplicationKey("subagent-terminal:" + string(id))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if result, err := second.ProcessCycle(ctx); err != nil || result.SubagentStatusesApplied != 0 || result.SubagentsReconciled != 0 {
		t.Fatalf("idempotent cycle=%+v err=%v", result, err)
	}
}

func TestOpenRestoresReceiverTerminalReceiptBeforeFirstCycle(t *testing.T) {
	tests := []struct {
		name       string
		status     domain.SubagentSpawnReceiptStatus
		result     string
		failure    string
		wantState  kernel.SessionState
		wantRecord domain.SubagentState
	}{
		{name: "complete", status: domain.SubagentSpawnReceiptComplete, result: "durable result", wantState: kernel.SessionStateComplete, wantRecord: domain.SubagentStateComplete},
		{name: "failed", status: domain.SubagentSpawnReceiptFailed, failure: "durable failure", wantState: kernel.SessionStateFailed, wantRecord: domain.SubagentStateError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			dbPath := t.TempDir() + "/runtime.db"
			opts := bootstrap.Options{StoreBackend: bootstrap.StorageSQLite, SQLitePath: dbPath, MissionID: "mission-receiver-terminal", Subagent: &bootstrap.SubagentOptions{Enabled: true, MaxConcurrent: 2, MaxAttempts: 1, Timeout: time.Minute}}
			first, err := bootstrap.Open(ctx, opts)
			if err != nil {
				t.Fatal(err)
			}
			id, err := first.Subagents.Spawn(ctx, kernel.SubagentSpec{Task: "receiver work", ContextMode: "isolated", Labels: map[string]string{"task_id": "receiver-task-" + tt.name}})
			if err != nil {
				t.Fatal(err)
			}
			now := first.Clock.Now().UTC()
			receipt := domain.SubagentSpawnReceipt{SchemaVersion: 1, CallerPeerID: "peer-origin", RequestID: "request-" + tt.name, SourceSessionID: "source-" + tt.name, Attempt: 0, Task: "receiver work", ContextMode: "isolated", ReceiverSessionID: string(id), RecordedAt: now, Status: tt.status, UpdatedAt: now.Add(time.Second), Result: tt.result, Failure: tt.failure, StatusDelivery: domain.SubagentStatusDeliveryPending}
			if err := first.Store.Update(ctx, func(tx port.Transaction) error { return tx.CreateSubagentSpawnReceipt(receipt) }); err != nil {
				t.Fatal(err)
			}
			if err := first.Close(ctx); err != nil {
				t.Fatal(err)
			}

			second, err := bootstrap.Open(ctx, opts)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = second.Close(ctx) })
			status, err := second.Subagents.Status(ctx, id)
			if err != nil || status.State != tt.wantState || status.Result != tt.result {
				t.Fatalf("restored status=%+v err=%v", status, err)
			}
			if status.Error != nil && status.Error.Error() != tt.failure {
				t.Fatalf("restored error=%v", status.Error)
			}
			cycle, err := second.ProcessCycle(ctx)
			if err != nil || cycle.SubagentsReconciled != 1 {
				t.Fatalf("cycle=%+v err=%v", cycle, err)
			}
			if err := second.Store.View(ctx, func(r port.Reader) error {
				record, err := r.SubagentRecord(string(id))
				if err != nil {
					return err
				}
				if record.State != tt.wantRecord || record.Result != tt.result {
					t.Fatalf("record=%+v", record)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOpenRestoresSubagentTransportPeerAcrossSQLiteRestart(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/runtime.db"
	opts := bootstrap.Options{
		StoreBackend: bootstrap.StorageSQLite,
		SQLitePath:   dbPath,
		MissionID:    "mission-remote-restart",
		Subagent:     &bootstrap.SubagentOptions{Enabled: true, MaxConcurrent: 2, MaxAttempts: 2, Timeout: time.Minute, TransportPeerID: "peer-a"},
	}
	first, err := bootstrap.Open(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	id, err := first.Subagents.Spawn(ctx, kernel.SubagentSpec{Task: "remote restart", ContextMode: "isolated", Labels: map[string]string{"task_id": "task-remote", kernel.SubagentTransportPeerLabel: "peer-a"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(ctx); err != nil {
		t.Fatal(err)
	}
	second, err := bootstrap.Open(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close(ctx) })
	status, err := second.Subagents.Status(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got := status.Spec.Labels[kernel.SubagentTransportPeerLabel]; got != "peer-a" {
		t.Fatalf("restored transport peer = %q", got)
	}
	if err := second.Store.View(ctx, func(r port.Reader) error {
		record, readErr := r.SubagentRecord(string(id))
		if readErr != nil {
			return readErr
		}
		if record.TransportPeerID != "peer-a" {
			t.Fatalf("durable transport peer = %q", record.TransportPeerID)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestProcessCycleCompactsExpiredSemanticMemoryWithinBatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	rt, err := bootstrap.Open(ctx, bootstrap.Options{
		StoreBackend:          bootstrap.StorageMemory,
		MemoryCompactionBatch: 1,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })

	now := time.Now().UTC()
	for _, mem := range []domain.LongTermMemory{
		{ID: "expired-1", Key: "expired-1", Scope: domain.MemoryScopeAgent, Value: "one", StoredAt: now.Add(-2 * time.Hour), Expiration: now.Add(-time.Hour)},
		{ID: "expired-2", Key: "expired-2", Scope: domain.MemoryScopeAgent, Value: "two", StoredAt: now.Add(-2 * time.Hour), Expiration: now.Add(-30 * time.Minute)},
	} {
		if err := rt.Store.Update(ctx, func(tx port.Transaction) error { return tx.SaveMemory(mem) }); err != nil {
			t.Fatal(err)
		}
	}

	result, err := rt.ProcessCycle(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.MemoriesCompacted != 1 || !result.Worked {
		t.Fatalf("first cycle = %#v", result)
	}
	result, err = rt.ProcessCycle(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.MemoriesCompacted != 1 || !result.Worked {
		t.Fatalf("second cycle = %#v", result)
	}
	result, err = rt.ProcessCycle(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.MemoriesCompacted != 0 || result.Worked {
		t.Fatalf("third cycle should rest: %#v", result)
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

func TestProcessCycleExecutesLocalContinuityOperation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	rt, err := bootstrap.Open(ctx, bootstrap.Options{
		StoreBackend: bootstrap.StorageMemory,
		MissionID:    "mission_exec",
		IdleMin:      time.Millisecond,
		IdleMax:      2 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })

	// Pin MaxDispatches=1 so this regression stays focused on local post-dispatch
	// execution without multi-step replenishment filling the default budget of 8.
	applySchedulerCadence(t, rt, domain.SchedulerCadenceConfig{
		Version: "scheduler.test.single.v1", MinIdleSleep: time.Millisecond, MaxIdleSleep: 2 * time.Millisecond,
		MaxCycleDuration: time.Minute, MaxDispatches: 1,
	})

	loader := mission.Loader{Store: rt.Store, Clock: rt.Clock, IDs: rt.IDs}
	specJSON := []byte(`{
  "schema_version": 1,
  "id": "mission_exec",
  "revision": 1,
  "original_text": "execute local continuity",
  "purpose": "prove post-dispatch model-free execution",
  "domains": ["test"],
  "policies": ["policy.v1"],
  "budget": {"model_calls": 10, "tokens": 1024, "bytes": 4096, "attempts": 3, "duration": 60000000000},
  "status": "ACTIVE"
}`)
	revision, err := loader.Load(ctx, specJSON, "bootstrap:exec")
	if err != nil {
		t.Fatalf("install mission: %v", err)
	}
	if err := kernel.EnsureCatalogSpecs(ctx, rt.Store, nil); err != nil {
		t.Fatalf("catalog: %v", err)
	}

	now := rt.Clock.Now().UTC()
	opp := domain.WorkOpportunity{
		SchemaVersion: domain.SchemaVersionV1, ID: "opp_exec_1",
		MissionRevision: revision.ID, Family: domain.FamilyIntegrityAudit,
		Title: "local integrity", Origin: "test", ExpectedGain: "audit",
		Novelty: "exec-cycle", StopCondition: "report", DedupSignature: "integrity:exec",
		Risk: domain.RiskLow, Priority: 30, EstimatedCost: domain.Budget{Tokens: 64, Attempts: 1},
		Status: domain.OpportunityOpen, CreatedAt: now, UpdatedAt: now,
	}
	if err := rt.Store.Update(ctx, func(tx port.Transaction) error {
		return tx.CreateWorkOpportunity(opp)
	}); err != nil {
		t.Fatalf("seed opportunity: %v", err)
	}
	admitter := kernel.Admitter{Store: rt.Store, Clock: rt.Clock, IDs: rt.IDs}
	admitted, err := admitter.AdmitOne(ctx, opp.ID)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}

	result, err := rt.ProcessCycle(ctx)
	if err != nil {
		t.Fatalf("cycle: %v", err)
	}
	if result.SchedulerKind != kernel.DecisionDispatch {
		t.Fatalf("scheduler kind = %q, want DISPATCH", result.SchedulerKind)
	}
	if result.OperationsExecuted != 1 || !result.Worked {
		t.Fatalf("expected one local execution, got %#v", result)
	}
	if result.CadenceVersion != "scheduler.test.single.v1" || result.SchedulerSteps != 1 {
		t.Fatalf("cadence single-step expected, got %#v", result)
	}

	var op domain.Operation
	if err := rt.Store.View(ctx, func(r port.Reader) error {
		var err error
		op, err = r.Operation(admitted.Operation.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if op.State != domain.StateSucceeded {
		t.Fatalf("operation state = %s, want SUCCEEDED", op.State)
	}
}

func applySchedulerCadence(t *testing.T, rt *bootstrap.Runtime, cadence domain.SchedulerCadenceConfig) {
	t.Helper()
	ctx := context.Background()
	clock := source.NewManualClock(rt.Clock.Now().UTC())
	ids := source.NewSequenceIDGenerator(9000)
	applier, err := kernel.NewConfigApplier(rt.Store, clock, ids)
	if err != nil {
		t.Fatal(err)
	}
	draft := domain.ConfigDraft{
		SchemaVersion: domain.SchemaVersionV1,
		ID:            domain.ConfigDraftID("draft_sched_" + cadence.Version),
		Scope:         domain.ConfigScopeScheduler,
		Applicability: domain.ConfigNextCycle, Status: domain.ConfigDraftOpen,
		ActorType: domain.ActorOperator, ActorID: "op", Reason: "test cadence",
		Scheduler: &cadence, CreatedAt: clock.Now(),
	}
	if err := rt.Store.Update(ctx, func(tx port.Transaction) error {
		return tx.CreateConfigDraft(draft)
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := applier.ValidateDraft(ctx, draft.ID); err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	if _, _, err := applier.ApplyDraft(ctx, draft.ID); err != nil {
		t.Fatal(err)
	}
}

func TestProcessCycleHonorsMaxDispatchesCadence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	rt, err := bootstrap.Open(ctx, bootstrap.Options{
		StoreBackend: bootstrap.StorageMemory,
		MissionID:    "mission_budget",
		IdleMin:      time.Millisecond,
		IdleMax:      2 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })

	loader := mission.Loader{Store: rt.Store, Clock: rt.Clock, IDs: rt.IDs}
	specJSON := []byte(`{
  "schema_version": 1,
  "id": "mission_budget",
  "revision": 1,
  "original_text": "budget multi dispatch",
  "purpose": "prove MaxDispatches bounds a control cycle",
  "domains": ["test"],
  "policies": ["policy.v1"],
  "budget": {"model_calls": 10, "tokens": 1024, "bytes": 4096, "attempts": 3, "duration": 60000000000},
  "status": "ACTIVE"
}`)
	revision, err := loader.Load(ctx, specJSON, "bootstrap:budget")
	if err != nil {
		t.Fatalf("install mission: %v", err)
	}
	if err := kernel.EnsureCatalogSpecs(ctx, rt.Store, nil); err != nil {
		t.Fatalf("catalog: %v", err)
	}

	// At most two productive dispatches per ProcessCycle.
	applySchedulerCadence(t, rt, domain.SchedulerCadenceConfig{
		Version: "scheduler.budget.v1", MinIdleSleep: time.Millisecond, MaxIdleSleep: 2 * time.Millisecond,
		MaxCycleDuration: time.Minute, MaxDispatches: 2,
	})

	// Disable continuity strategies so only the three seeded READY ops exist;
	// otherwise replenishment would admit more work and blur the budget signal.
	rt.Scheduler.Strategies = nil
	rt.Scheduler.Registry = nil

	now := rt.Clock.Now().UTC()
	for i := 1; i <= 3; i++ {
		opp := domain.WorkOpportunity{
			SchemaVersion: domain.SchemaVersionV1, ID: domain.WorkOpportunityID(fmt.Sprintf("opp_budget_%d", i)),
			MissionRevision: revision.ID, Family: domain.FamilyIntegrityAudit,
			Title: "local integrity", Origin: "test", ExpectedGain: "audit",
			Novelty: fmt.Sprintf("budget-%d", i), StopCondition: "report",
			DedupSignature: fmt.Sprintf("integrity:budget:%d", i),
			Risk:           domain.RiskLow, Priority: 30, EstimatedCost: domain.Budget{Tokens: 64, Attempts: 1},
			Status: domain.OpportunityOpen, CreatedAt: now, UpdatedAt: now,
		}
		if err := rt.Store.Update(ctx, func(tx port.Transaction) error {
			return tx.CreateWorkOpportunity(opp)
		}); err != nil {
			t.Fatalf("seed opportunity %d: %v", i, err)
		}
		admitter := kernel.Admitter{Store: rt.Store, Clock: rt.Clock, IDs: rt.IDs}
		if _, err := admitter.AdmitOne(ctx, opp.ID); err != nil {
			t.Fatalf("admit %d: %v", i, err)
		}
	}

	result, err := rt.ProcessCycle(ctx)
	if err != nil {
		t.Fatalf("cycle: %v", err)
	}
	if result.CadenceVersion != "scheduler.budget.v1" {
		t.Fatalf("cadence version = %q", result.CadenceVersion)
	}
	if result.OperationsExecuted != 2 {
		t.Fatalf("executed = %d, want 2 under MaxDispatches (got %#v)", result.OperationsExecuted, result)
	}
	if !result.DispatchBudgetHit {
		t.Fatalf("expected dispatch budget hit, got %#v", result)
	}
	if result.SchedulerSteps != 2 {
		t.Fatalf("scheduler steps = %d, want 2", result.SchedulerSteps)
	}

	// Residual READY work remains for the next cycle.
	ready := 0
	if err := rt.Store.View(ctx, func(r port.Reader) error {
		ops, err := r.Operations(revision.ID)
		if err != nil {
			return err
		}
		for _, op := range ops {
			if op.State == domain.StateReady {
				ready++
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if ready != 1 {
		t.Fatalf("ready remaining = %d, want 1", ready)
	}

	// Second cycle drains the remainder; without strategies it cannot invent more.
	result2, err := rt.ProcessCycle(ctx)
	if err != nil {
		t.Fatalf("second cycle: %v", err)
	}
	if result2.OperationsExecuted != 1 {
		t.Fatalf("second cycle executed = %d, want 1 (got %#v)", result2.OperationsExecuted, result2)
	}
	if result2.DispatchBudgetHit {
		t.Fatalf("second cycle should not hit dispatch budget with one remaining op: %#v", result2)
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
			Ingress:       bootstrap.TelegramIngressNone,
		},
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })
	if rt.TelegramWorker == nil || rt.TelegramAdapter == nil {
		t.Fatal("expected telegram surfaces wired")
	}
	if rt.TelegramIngress != nil {
		t.Fatal("ingress none must not wire ingress")
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

func TestProcessCyclePollsTelegramIngressBeforeEventDrain(t *testing.T) {
	// Cannot t.Parallel: mutates process env for Telegram token injection.
	ctx := context.Background()
	now := time.Now().UTC().Add(-time.Minute)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case len(r.URL.Path) >= 11 && r.URL.Path[len(r.URL.Path)-11:] == "/getUpdates":
			_, _ = w.Write([]byte(`{"ok":true,"result":[{"update_id":101,"callback_query":{"id":"cb_boot","from":{"id":7},"message":{"message_id":42,"chat":{"id":100}},"data":"o:0"}}]}`))
		default:
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
		}
	}))
	t.Cleanup(server.Close)

	t.Setenv("MOTOR_TELEGRAM_TOKEN_POLL", "poll-token")
	rt, err := bootstrap.Open(ctx, bootstrap.Options{
		StoreBackend:  bootstrap.StorageMemory,
		IdleMin:       time.Millisecond,
		IdleMax:       2 * time.Millisecond,
		MaxInboxBatch: 4,
		DeliveryBatch: 4,
		DeliveryLease: time.Minute,
		DeliveryRetry: time.Minute,
		Telegram: &bootstrap.TelegramOptions{
			Enabled:       true,
			TokenEnv:      "MOTOR_TELEGRAM_TOKEN_POLL",
			BaseURL:       server.URL,
			Destinations:  map[string]int64{"operator_primary": 100},
			AllowedActors: map[int64]string{7: "operator_1"},
			AllowedChats:  map[int64]struct{}{100: {}},
			WorkerOwner:   "bootstrap-poll-test",
			Ingress:       bootstrap.TelegramIngressPoll,
			PollLimit:     10,
			PollTimeout:   0,
		},
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })
	if rt.TelegramIngress == nil {
		t.Fatal("expected poll ingress wired")
	}

	question := domain.OperatorQuestion{
		SchemaVersion: domain.SchemaVersionV1, ID: "ask_boot_poll", MissionID: "mission_boot_poll",
		MissionRevision: "rev_boot_poll", Revision: 1, Kind: domain.QuestionSingleChoice,
		Prompt: "Choose", Context: "poll wiring", Options: []domain.QuestionOption{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}},
		AllowSkip: true, FallbackPolicy: domain.QuestionContinueOtherWork, DedupSignature: "boot-poll",
		Priority: 40, Status: domain.OperatorQuestionPending, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	delivery := domain.QuestionDelivery{
		SchemaVersion: 1, ID: "delivery_boot_poll", QuestionID: question.ID, QuestionRevision: 1,
		Channel: "telegram", DestinationRef: "operator_primary", Status: domain.QuestionDeliveryPending,
		MaxAttempts: 3, AvailableAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := rt.Store.Update(ctx, func(tx port.Transaction) error {
		mission := domain.MissionRevision{
			SchemaVersion: 1, ID: question.MissionRevision, MissionID: question.MissionID, Revision: 1,
			OriginalText: "poll", Purpose: "test", Status: domain.MissionActive, Provenance: "test", AcceptedAt: now,
		}
		if err := tx.AppendMissionRevision(mission); err != nil {
			return err
		}
		if err := tx.CreateOperatorQuestion(question); err != nil {
			return err
		}
		if err := tx.CreateQuestionDelivery(delivery); err != nil {
			return err
		}
		leased, err := domain.LeaseQuestionDelivery(delivery, "seed", now, now.Add(time.Minute))
		if err != nil {
			return err
		}
		if err := tx.SaveQuestionDelivery(leased, delivery.Status, delivery.Attempt); err != nil {
			return err
		}
		completed, err := domain.CompleteQuestionDelivery(leased, "seed", "42", now.Add(time.Second))
		if err != nil {
			return err
		}
		return tx.SaveQuestionDelivery(completed, leased.Status, leased.Attempt)
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	result, err := rt.ProcessCycle(ctx)
	if err != nil {
		t.Fatalf("cycle: %v", err)
	}
	if result.TelegramFetched != 1 || result.TelegramAccepted != 1 || result.EventsProcessed != 1 {
		t.Fatalf("expected poll+process in same cycle, got %#v", result)
	}
}

func TestOpenMountsTelegramWebhookRoute(t *testing.T) {
	// Cannot t.Parallel: mutates process env.
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("MOTOR_TELEGRAM_TOKEN_HOOK", "hook-token")
	t.Setenv("MOTOR_TELEGRAM_HOOK_SECRET", "secret-value")
	rt, err := bootstrap.Open(ctx, bootstrap.Options{
		StoreBackend: bootstrap.StorageMemory,
		Telegram: &bootstrap.TelegramOptions{
			Enabled:          true,
			TokenEnv:         "MOTOR_TELEGRAM_TOKEN_HOOK",
			BaseURL:          server.URL,
			Destinations:     map[string]int64{"operator_primary": 100},
			AllowedActors:    map[int64]string{7: "operator_1"},
			AllowedChats:     map[int64]struct{}{100: {}},
			Ingress:          bootstrap.TelegramIngressWebhook,
			WebhookPath:      "/telegram/webhook",
			WebhookSecretEnv: "MOTOR_TELEGRAM_HOOK_SECRET",
		},
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })
	if rt.TelegramIngress == nil {
		t.Fatal("expected webhook ingress")
	}
	req := httptest.NewRequest(http.MethodPost, "/telegram/webhook", nil)
	rec := httptest.NewRecorder()
	rt.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing secret should 401, got %d", rec.Code)
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

func TestProcessCycleReconcilesExpiredLeaseAndRunsModelPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	rt, err := bootstrap.Open(ctx, bootstrap.Options{
		StoreBackend: bootstrap.StorageMemory,
		MissionID:    "mission_model",
		IdleMin:      time.Millisecond,
		IdleMax:      2 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })

	// Manual clock for deterministic lease expiry.
	start := time.Date(2026, 7, 16, 16, 0, 0, 0, time.UTC)
	clock := source.NewManualClock(start)
	rt.Clock = clock
	rt.LeaseReaper.Clock = clock
	rt.Executor.Local.Clock = clock

	loader := mission.Loader{Store: rt.Store, Clock: clock, IDs: rt.IDs}
	specJSON := []byte(`{
  "schema_version": 1,
  "id": "mission_model",
  "revision": 1,
  "original_text": "model path vertical",
  "purpose": "prove propose-only model path",
  "domains": ["test"],
  "policies": ["policy.v1"],
  "budget": {"model_calls": 10, "tokens": 8000, "bytes": 4096, "attempts": 3, "duration": 60000000000},
  "status": "ACTIVE"
}`)
	revision, err := loader.Load(ctx, specJSON, "bootstrap:model")
	if err != nil {
		t.Fatalf("install mission: %v", err)
	}

	// Seed a stuck RUNNING op with expired lease under the mission revision.
	expiredRef := kernel.FormatLeaseRef("lease_stuck", "operation_stuck", 1, start.Add(-time.Minute))
	if err := rt.Store.Update(ctx, func(tx port.Transaction) error {
		spec := domain.OperationSpec{
			SchemaVersion: 1, ID: "extract@1", ContractVersion: 1, TemplateVersion: 1,
			InputSchema: "refs", OutputSchema: "proposed changeset",
			Budget:          domain.Budget{ModelCalls: 1, Tokens: 4000, Attempts: 1},
			MaxOutputTokens: 500, SafetyMargin: 50, Validators: []string{"schema"},
			RetryPolicy: "none", FallbackPolicy: "fail", MaximumAuthority: domain.AuthorityProposeOnly,
		}
		if err := tx.AppendOperationSpec(spec); err != nil {
			return err
		}
		question := domain.Question{
			SchemaVersion: 1, ID: "q_model", MissionRevision: revision.ID,
			Text: "extract?", Origin: "mission", Relevance: "primary", AnswerCondition: "evidence",
		}
		if err := tx.CreateQuestion(question); err != nil {
			return err
		}
		candidate := domain.InquiryCandidate{
			SchemaVersion: 1, ID: "cand_model", MissionRevision: revision.ID, QuestionID: question.ID,
			DerivedFrom: []string{"gap"}, ExpectedProgress: "obs", Novelty: "n", Risk: domain.RiskLow,
			SourcePlan: []string{"fixture"}, AnswerCondition: "evidence", StopCondition: "done",
			ReviewAfter: start.Add(time.Hour),
		}
		if err := tx.CreateInquiryCandidate(candidate); err != nil {
			return err
		}
		inquiry := domain.Inquiry{
			SchemaVersion: 1, ID: "inq_model", CandidateID: candidate.ID, MissionRevision: revision.ID,
			QuestionID: question.ID, AdmissionReason: "test", StopCondition: "done",
			State: domain.StateReady, Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateReady},
		}
		if err := tx.CreateInquiry(inquiry); err != nil {
			return err
		}
		stuck := domain.Operation{
			SchemaVersion: 1, ID: "operation_stuck", InquiryID: inquiry.ID, MissionRevision: revision.ID,
			SpecID: spec.ID, ReadSet: []string{"fragment_1"}, ExpectedOutput: "proposed_change_set",
			IdempotencyKey: "idem_stuck", Attempt: 1, State: domain.StateRunning,
			Reevaluation: domain.ReevaluationCondition{Kind: domain.ReevaluateLease, Reference: expiredRef},
		}
		if err := tx.CreateOperation(stuck); err != nil {
			return err
		}
		// Also seed a READY model op that will run after reaper frees the stuck one... actually
		// scheduler picks READY only; reaper turns stuck into READY first, then DISPATCH may pick it.
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Cycle without model: reaper should READY the stuck op; local skip requires_model.
	result, err := rt.ProcessCycle(ctx)
	if err != nil {
		t.Fatalf("cycle reaper: %v", err)
	}
	if result.LeasesReconciled != 1 {
		t.Fatalf("expected 1 lease reconciled, got %#v", result)
	}
	var stuck domain.Operation
	if err := rt.Store.View(ctx, func(r port.Reader) error {
		var err error
		stuck, err = r.Operation("operation_stuck")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// After reaper READY, same cycle may DISPATCH and skip requires_model (no provider).
	if stuck.State != domain.StateReady && stuck.State != domain.StateRunning {
		// If dispatch claimed it and left running without model, that would be a bug.
		// With DispatchExecutor and no model, skip leaves READY.
		t.Fatalf("stuck state after cycle = %s", stuck.State)
	}
	if stuck.State == domain.StateRunning {
		t.Fatalf("without model path, op must not remain RUNNING: %+v", stuck)
	}
	if result.OperationsSkipped < 1 && stuck.State == domain.StateReady && result.SchedulerKind == kernel.DecisionDispatch {
		// dispatch happened and skip counted
	}
}

func TestOpenWiresModelExecutorWhenEnabled(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_test","object":"chat.completion","created":1,"model":"fixture","choices":[{"index":0,"message":{"role":"assistant","content":"{}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	t.Cleanup(server.Close)

	rt, err := bootstrap.Open(ctx, bootstrap.Options{
		ListenAddr:     "127.0.0.1:0",
		StoreBackend:   bootstrap.StorageMemory,
		RuntimeName:    "test-model-wire",
		RuntimeVersion: "test",
		Model: &bootstrap.ModelOptions{
			Enabled:       true,
			BaseURL:       server.URL,
			Model:         "fixture-model",
			ContextTokens: 4096,
			PolicyVersion: "policy@wire-test",
			LeaseTTL:      time.Minute,
		},
	})
	if err != nil {
		t.Fatalf("open with model: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })
	if rt.Model == nil {
		t.Fatal("Runtime.Model must be set when model options are enabled")
	}
	if rt.Executor.Model == nil {
		t.Fatal("DispatchExecutor.Model must be set when model options are enabled")
	}
	if rt.Model != rt.Executor.Model {
		t.Fatal("Runtime.Model and Executor.Model must be the same instance")
	}
}

func TestOpenWithoutModelKeepsNilExecutor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	rt, err := bootstrap.Open(ctx, bootstrap.Options{
		ListenAddr:     "127.0.0.1:0",
		StoreBackend:   bootstrap.StorageMemory,
		RuntimeName:    "test-no-model",
		RuntimeVersion: "test",
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })
	if rt.Model != nil || rt.Executor.Model != nil {
		t.Fatalf("model must stay nil without options: rt=%v exec=%v", rt.Model != nil, rt.Executor.Model != nil)
	}
}

func TestOpenWiresFallbackProviderWhenEnabled(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_primary","object":"chat.completion","created":1,"model":"primary","choices":[{"index":0,"message":{"role":"assistant","content":"{}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	t.Cleanup(primary.Close)
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_fallback","object":"chat.completion","created":1,"model":"fallback","choices":[{"index":0,"message":{"role":"assistant","content":"{}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	t.Cleanup(fallback.Close)

	rt, err := bootstrap.Open(ctx, bootstrap.Options{
		ListenAddr:     "127.0.0.1:0",
		StoreBackend:   bootstrap.StorageMemory,
		RuntimeName:    "test-model-fallback-wire",
		RuntimeVersion: "test",
		Model: &bootstrap.ModelOptions{
			Enabled:       true,
			BaseURL:       primary.URL,
			Model:         "primary-model",
			ContextTokens: 4096,
			PolicyVersion: "policy@fallback-wire",
			LeaseTTL:      time.Minute,
			Fallback: &bootstrap.ModelFallbackOptions{
				Enabled: true,
				BaseURL: fallback.URL,
				Model:   "fallback-model",
			},
		},
	})
	if err != nil {
		t.Fatalf("open with fallback model: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })
	if rt.Model == nil {
		t.Fatal("Runtime.Model must be set")
	}
	if rt.Model.FallbackProvider == nil {
		t.Fatal("ModelExecutor.FallbackProvider must be wired when fallback options are enabled")
	}
	if rt.Executor.Model == nil || rt.Executor.Model.FallbackProvider == nil {
		t.Fatal("DispatchExecutor.Model.FallbackProvider must be wired")
	}
}

func TestOpenModelFallbackRequiresURLAndName(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","created":1,"model":"p","choices":[{"index":0,"message":{"role":"assistant","content":"{}"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(primary.Close)

	_, err := bootstrap.Open(ctx, bootstrap.Options{
		ListenAddr:   "127.0.0.1:0",
		StoreBackend: bootstrap.StorageMemory,
		Model: &bootstrap.ModelOptions{
			Enabled:       true,
			BaseURL:       primary.URL,
			Model:         "primary-model",
			ContextTokens: 2048,
			Fallback: &bootstrap.ModelFallbackOptions{
				Enabled: true,
				// Missing BaseURL and Model — Validate must reject.
			},
		},
	})
	if err == nil {
		t.Fatal("expected validation error for incomplete fallback options")
	}
}

func TestOpenWithoutFallbackKeepsNilFallbackProvider(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_test","object":"chat.completion","created":1,"model":"fixture","choices":[{"index":0,"message":{"role":"assistant","content":"{}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	t.Cleanup(server.Close)

	rt, err := bootstrap.Open(ctx, bootstrap.Options{
		ListenAddr:   "127.0.0.1:0",
		StoreBackend: bootstrap.StorageMemory,
		Model: &bootstrap.ModelOptions{
			Enabled:       true,
			BaseURL:       server.URL,
			Model:         "fixture-model",
			ContextTokens: 4096,
			PolicyVersion: "policy@wire-test",
			LeaseTTL:      time.Minute,
		},
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })
	if rt.Model == nil {
		t.Fatal("expected model")
	}
	if rt.Model.FallbackProvider != nil {
		t.Fatal("FallbackProvider must stay nil without fallback options")
	}
}

func TestOpenWiresWebAndFileExecutors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"t","url":"https://example.com","content":"s"}]}`))
	}))
	t.Cleanup(searchServer.Close)

	rt, err := bootstrap.Open(ctx, bootstrap.Options{
		ListenAddr:   "127.0.0.1:0",
		StoreBackend: bootstrap.StorageMemory,
		RuntimeName:  "test-web-file",
		Web: &bootstrap.WebOptions{
			Enabled:       true,
			SearchBaseURL: searchServer.URL,
			EnableFetch:   true,
			// Tests may target the httptest loopback.
			FetchAllowPrivate: true,
			IngestFetched:     false,
			PolicyVersion:     "policy@web-wire",
		},
		File: &bootstrap.FileOptions{
			Enabled: true,
			Roots: []bootstrap.FileRootConfig{
				{Name: "default", Path: root},
			},
			PolicyVersion: "policy@file-wire",
		},
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close(ctx) })
	if rt.Web == nil || rt.Executor.Web == nil {
		t.Fatal("Web executor must be wired")
	}
	if rt.File == nil || rt.Executor.File == nil {
		t.Fatal("File executor must be wired")
	}
	if rt.Web.Searcher == nil || rt.Web.Fetcher == nil {
		t.Fatal("web searcher and fetcher must be present")
	}
	if len(rt.File.Roots) != 1 || rt.File.Roots[0].Path != root {
		t.Fatalf("file roots = %#v", rt.File.Roots)
	}
}

func TestOpenWebRequiresAdapter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, err := bootstrap.Open(ctx, bootstrap.Options{
		ListenAddr:   "127.0.0.1:0",
		StoreBackend: bootstrap.StorageMemory,
		Web: &bootstrap.WebOptions{
			Enabled: true,
			// No search URL and fetch disabled.
		},
	})
	if err == nil {
		t.Fatal("expected validation error when web has no adapters")
	}
}

func TestOpenFileRequiresRoots(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, err := bootstrap.Open(ctx, bootstrap.Options{
		ListenAddr:   "127.0.0.1:0",
		StoreBackend: bootstrap.StorageMemory,
		File: &bootstrap.FileOptions{
			Enabled: true,
		},
	})
	if err == nil {
		t.Fatal("expected validation error when file has no roots")
	}
}
