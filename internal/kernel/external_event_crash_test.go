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

// Crash/reopen of the external event processor proves a stimulus is either still
// RECEIVED (and applied once after reopen) or terminal and idempotently replayed.
func TestExternalEventProcessorCrashReplaySQLite(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "external.sqlite")
	now := time.Date(2026, 7, 16, 6, 30, 0, 0, time.UTC)

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	seedMissionSQLite(t, store, now)
	clock := source.NewManualClock(now)
	ids := source.NewSequenceIDGenerator(1)
	inbox, err := control.NewExternalEventInbox(store, control.DispositionFactoryFrom(clock))
	if err != nil {
		t.Fatal(err)
	}
	event := domain.ExternalEvent{
		SchemaVersion: domain.SchemaVersionV1, ID: "ext_wake_crash", DeduplicationKey: "dashboard:wake:crash",
		Source: "operator-dashboard", SourceActorID: "operator_1", Kind: domain.ExternalUserMessage,
		MissionID: "mission_1", TransportMessageID: "ui-crash-1",
		Content:    domain.ExternalContent{MediaType: "text/plain", Text: "wake after crash"},
		ReceivedAt: now,
	}
	received, err := inbox.SubmitExternalEvent(event)
	if err != nil || received.State != domain.ExternalEventReceived {
		t.Fatalf("submit = %#v err=%v", received, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	clock = source.NewManualClock(now.Add(time.Second))
	ids = source.NewSequenceIDGenerator(100)
	processor, err := kernel.NewExternalEventProcessor(store, clock, ids)
	if err != nil {
		t.Fatal(err)
	}
	applied, ok, err := processor.ProcessNext(context.Background())
	if err != nil || !ok {
		t.Fatalf("first process ok=%v err=%v", ok, err)
	}
	// No matching wait exists, so the durable outcome is IGNORED evidence.
	if applied.State != domain.ExternalEventIgnored {
		t.Fatalf("first process disposition = %#v", applied)
	}
	var eventsBefore int
	if err := store.View(context.Background(), func(r port.Reader) error {
		events, err := r.Events(0, 1000)
		if err != nil {
			return err
		}
		eventsBefore = len(events)
		disposition, err := r.ExternalEventDisposition(event.ID)
		if err != nil {
			return err
		}
		if disposition.State != domain.ExternalEventIgnored {
			t.Fatalf("stored disposition = %#v", disposition)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = sqlite.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	clock = source.NewManualClock(now.Add(2 * time.Second))
	ids = source.NewSequenceIDGenerator(200)
	processor, err = kernel.NewExternalEventProcessor(store, clock, ids)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := processor.Process(context.Background(), event.ID)
	if err != nil || replay.State != domain.ExternalEventIgnored || replay.ResultRef != applied.ResultRef {
		t.Fatalf("replay = %#v err=%v", replay, err)
	}
	if _, ok, err := processor.ProcessNext(context.Background()); err != nil || ok {
		t.Fatalf("unexpected pending external event after terminal apply ok=%v err=%v", ok, err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
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
