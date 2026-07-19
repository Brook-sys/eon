package control

import (
	"context"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/runtime/source"
	"motor-autonomo/internal/storage/memory"
)

func TestSemanticMemoryPersistsViewAndAuditAtomically(t *testing.T) {
	clock := source.NewManualClock(time.Date(2026, 7, 19, 23, 0, 0, 0, time.UTC))
	store := memory.New()
	service, err := NewSemanticMemory(store, clock, source.NewSequenceIDGenerator(1))
	if err != nil {
		t.Fatal(err)
	}

	mem := domain.LongTermMemory{ID: "memory-1", Key: "mission", Scope: domain.MemoryScopeMission, Value: "private value", StoredAt: clock.Now()}
	if err := service.SaveMemory(mem); err != nil {
		t.Fatal(err)
	}

	stored, err := store.LongTermMemory(mem.Key)
	if err != nil || stored != mem {
		t.Fatalf("stored memory = %+v, err = %v", stored, err)
	}
	var events []domain.Event
	if err := store.View(context.Background(), func(r port.Reader) error {
		var err error
		events, err = r.Events(0, 10)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Kind != domain.EventMemoryStored {
		t.Fatalf("unexpected events: %+v", events)
	}
	if events[0].PayloadRef == "" || events[0].PayloadRef == mem.Value {
		t.Fatalf("audit payload must reference metadata without exposing value: %q", events[0].PayloadRef)
	}

	deleted, err := service.DeleteMemory(mem.ID, "operator_deleted")
	if err != nil || !deleted {
		t.Fatalf("delete = %v, err = %v", deleted, err)
	}
	if _, err := store.LongTermMemory(mem.Key); err == nil {
		t.Fatal("memory still present after deletion")
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		var err error
		events, err = r.Events(0, 10)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].Kind != domain.EventMemoryCompacted {
		t.Fatalf("unexpected events after deletion: %+v", events)
	}
}

func TestSemanticMemoryDuplicateEventRollsBackView(t *testing.T) {
	clock := source.NewManualClock(time.Date(2026, 7, 19, 23, 0, 0, 0, time.UTC))
	store := memory.New()
	ids := fixedIDGenerator("event-duplicate")
	service, _ := NewSemanticMemory(store, clock, ids)
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		_, err := tx.AppendEvent(domain.Event{SchemaVersion: domain.SchemaVersionV1, ID: "event-duplicate", Kind: "test", OccurredAt: clock.Now()})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	err := service.SaveMemory(domain.LongTermMemory{ID: "memory-1", Key: "mission", Scope: domain.MemoryScopeMission, Value: "value", StoredAt: clock.Now()})
	if err == nil {
		t.Fatal("expected duplicate event failure")
	}
	if _, lookupErr := store.LongTermMemory("mission"); lookupErr == nil {
		t.Fatal("memory mutation was not rolled back")
	}
}

func TestSemanticMemoryDeleteUnknownDoesNotAppendEvent(t *testing.T) {
	clock := source.NewManualClock(time.Now().UTC())
	store := memory.New()
	service, _ := NewSemanticMemory(store, clock, source.NewSequenceIDGenerator(1))
	deleted, err := service.DeleteMemory("missing", "operator_deleted")
	if err != nil || deleted {
		t.Fatalf("delete = %v, err = %v", deleted, err)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		events, err := r.Events(0, 10)
		if err != nil {
			return err
		}
		if len(events) != 0 {
			return errors.New("unexpected event for missing memory")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

type fixedIDGenerator string

func (g fixedIDGenerator) NewID(string) (string, error) { return string(g), nil }
