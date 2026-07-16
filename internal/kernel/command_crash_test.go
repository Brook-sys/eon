package kernel_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"motor-autonomo/internal/control"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/kernel"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/sqlite"
)

// Crash/reopen of the command processor proves inbox effects are atomic at the
// store boundary: either the command remains RECEIVED and can be applied once,
// or a terminal receipt is durable and Process is a pure replay.
func TestCommandProcessorCrashReplaySQLite(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "control.sqlite")
	now := time.Date(2026, 7, 16, 6, 0, 0, 0, time.UTC)

	// Phase 1: durable RECEIVED only (simulates crash before kernel apply).
	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	seedMissionSQLite(t, store, now)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	inbox, err := control.NewCommandInbox(store, control.ReceiptFactoryFrom(clock, ids))
	if err != nil {
		t.Fatal(err)
	}
	revision := uint64(1)
	pause := domain.OperatorCommand{
		SchemaVersion: domain.SchemaVersionV1, ID: "cmd_pause_crash", IdempotencyKey: "idem_pause_crash",
		ActorType: domain.ActorOperator, ActorID: "operator_1", Kind: domain.CommandPauseMission,
		Target: domain.CommandTarget{MissionID: "mission_1"}, ExpectedRevision: &revision,
		Reason: "crash harness", SubmittedAt: now,
	}
	received, err := inbox.SubmitCommand(pause)
	if err != nil || received.State != domain.CommandReceived {
		t.Fatalf("submit = %#v err=%v", received, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	// Phase 2: reopen and apply once.
	store, err = sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	clock = source.NewManualClock(now.Add(time.Second))
	ids = source.NewSequenceIDGenerator(100)
	processor, err := kernel.NewCommandProcessor(store, clock, ids)
	if err != nil {
		t.Fatal(err)
	}
	applied, ok, err := processor.ProcessNext(context.Background())
	if err != nil || !ok || applied.State != domain.CommandApplied {
		t.Fatalf("first apply = %#v ok=%v err=%v", applied, ok, err)
	}
	var controlBefore domain.ControlState
	var eventsBefore int
	if err := store.View(context.Background(), func(r port.Reader) error {
		state, err := r.ControlState()
		if err != nil {
			return err
		}
		controlBefore = state
		events, err := r.Events(0, 1000)
		if err != nil {
			return err
		}
		eventsBefore = len(events)
		if state.AllowsDispatch("mission_1") {
			t.Fatalf("dispatch still allowed after pause: %#v", state)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	// Phase 3: reopen after successful apply and replay Process; no second effect.
	store, err = sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	clock = source.NewManualClock(now.Add(2 * time.Second))
	ids = source.NewSequenceIDGenerator(200)
	processor, err = kernel.NewCommandProcessor(store, clock, ids)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := processor.Process(context.Background(), pause.ID)
	if err != nil || replay.State != domain.CommandApplied || replay.ResultRef != applied.ResultRef {
		t.Fatalf("replay = %#v err=%v", replay, err)
	}
	// ProcessNext must not invent another pending command.
	if _, ok, err := processor.ProcessNext(context.Background()); err != nil || ok {
		t.Fatalf("unexpected pending command after terminal apply ok=%v err=%v", ok, err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		state, err := r.ControlState()
		if err != nil {
			return err
		}
		if state.Revision != controlBefore.Revision {
			t.Fatalf("control revision changed on replay: before=%d after=%d", controlBefore.Revision, state.Revision)
		}
		events, err := r.Events(0, 1000)
		if err != nil {
			return err
		}
		if len(events) != eventsBefore {
			t.Fatalf("event count changed on replay: before=%d after=%d", eventsBefore, len(events))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func seedMissionSQLite(t *testing.T, store port.Store, now time.Time) {
	t.Helper()
	mission := domain.MissionRevision{
		SchemaVersion: domain.SchemaVersionV1, ID: "revision_1", MissionID: "mission_1", Revision: 1,
		OriginalText: "crash mission", Purpose: "crash", Status: domain.MissionActive,
		Provenance: "fixture", AcceptedAt: now,
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		if err := tx.AppendMissionRevision(mission); err != nil {
			return err
		}
		return tx.ActivateMissionRevision(mission.MissionID, mission.ID)
	}); err != nil {
		t.Fatal(err)
	}
}
