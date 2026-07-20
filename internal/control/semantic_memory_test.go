package control

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	if err := service.SaveMemory(context.Background(), mem); err != nil {
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

	deleted, err := service.DeleteMemory(context.Background(), mem.ID, "operator_deleted")
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

	err := service.SaveMemory(context.Background(), domain.LongTermMemory{ID: "memory-1", Key: "mission", Scope: domain.MemoryScopeMission, Value: "value", StoredAt: clock.Now()})
	if err == nil {
		t.Fatal("expected duplicate event failure")
	}
	if _, lookupErr := store.LongTermMemory("mission"); lookupErr == nil {
		t.Fatal("memory mutation was not rolled back")
	}
}

func TestSemanticMemoryDeleteUnknownDoesNotAppendEvent(t *testing.T) {
	clock := source.NewManualClock(time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC))
	store := memory.New()
	service, _ := NewSemanticMemory(store, clock, source.NewSequenceIDGenerator(1))
	deleted, err := service.DeleteMemory(context.Background(), "missing", "operator_deleted")
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

func TestSemanticMemoryRejectsIdentityCollisionAndRollsBackDeleteEventFailure(t *testing.T) {
	clock := source.NewManualClock(time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC))
	store := memory.New()
	service, _ := NewSemanticMemory(store, clock, source.NewSequenceIDGenerator(1))
	first := domain.LongTermMemory{ID: "memory-1", Key: "mission", Scope: domain.MemoryScopeMission, Value: "one"}
	if err := service.SaveMemory(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveMemory(context.Background(), domain.LongTermMemory{ID: "memory-2", Key: first.Key, Scope: first.Scope, Value: "two"}); err == nil {
		t.Fatal("expected key/ID collision rejection")
	}
	stored, err := store.LongTermMemory(first.Key)
	if err != nil || stored.ID != first.ID || stored.Value != first.Value {
		t.Fatalf("collision changed current view: %+v, err=%v", stored, err)
	}

	duplicateEventService, _ := NewSemanticMemory(store, clock, fixedIDGenerator("event_0000000000000001"))
	deleted, err := duplicateEventService.DeleteMemory(context.Background(), first.ID, "operator_deleted")
	if err == nil || deleted {
		t.Fatalf("delete with duplicate event = %v, err=%v", deleted, err)
	}
	if _, lookupErr := store.LongTermMemory(first.Key); lookupErr != nil {
		t.Fatal("delete mutation was not rolled back")
	}
}

func TestSemanticMemoryHonorsCanceledContext(t *testing.T) {
	clock := source.NewManualClock(time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC))
	store := memory.New()
	service, _ := NewSemanticMemory(store, clock, source.NewSequenceIDGenerator(1))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.SaveMemory(ctx, domain.LongTermMemory{ID: "memory-1", Key: "key", Scope: domain.MemoryScopeAgent, Value: "value"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("save error = %v, want context.Canceled", err)
	}
	if _, err := store.LongTermMemory("key"); !errors.Is(err, port.ErrNotFound) {
		t.Fatalf("canceled save changed memory view: %v", err)
	}
}

func TestSemanticMemoryCompactsExpiredInBoundedDeterministicBatches(t *testing.T) {
	clock := source.NewManualClock(time.Date(2026, 7, 20, 1, 0, 0, 0, time.UTC))
	store := memory.New()
	service, _ := NewSemanticMemory(store, clock, source.NewSequenceIDGenerator(1))
	for _, mem := range []domain.LongTermMemory{
		{ID: "memory-first", Key: "first", Scope: domain.MemoryScopeAgent, Value: "v", Expiration: clock.Now().Add(time.Hour)},
		{ID: "memory-later", Key: "later", Scope: domain.MemoryScopeAgent, Value: "v", Expiration: clock.Now().Add(2 * time.Hour)},
		{ID: "memory-equal", Key: "equal", Scope: domain.MemoryScopeAgent, Value: "v", Expiration: clock.Now().Add(3 * time.Hour)},
		{ID: "memory-active", Key: "active", Scope: domain.MemoryScopeAgent, Value: "v", Expiration: clock.Now().Add(4 * time.Hour)},
	} {
		if err := service.SaveMemory(context.Background(), mem); err != nil {
			t.Fatal(err)
		}
	}
	if err := clock.Advance(3 * time.Hour); err != nil {
		t.Fatal(err)
	}

	compacted, err := service.CompactExpired(context.Background(), 2)
	if err != nil || compacted != 2 {
		t.Fatalf("first compaction = %d, err=%v", compacted, err)
	}
	if _, err := store.LongTermMemory("first"); !errors.Is(err, port.ErrNotFound) {
		t.Fatalf("oldest memory remains: %v", err)
	}
	if _, err := store.LongTermMemory("later"); !errors.Is(err, port.ErrNotFound) {
		t.Fatalf("second-oldest memory remains: %v", err)
	}
	if _, err := store.LongTermMemory("equal"); err != nil {
		t.Fatalf("batch exceeded limit: %v", err)
	}

	compacted, err = service.CompactExpired(context.Background(), 2)
	if err != nil || compacted != 1 {
		t.Fatalf("second compaction = %d, err=%v", compacted, err)
	}
	if _, err := store.LongTermMemory("equal"); !errors.Is(err, port.ErrNotFound) {
		t.Fatalf("deadline-equal memory remains: %v", err)
	}
	if _, err := store.LongTermMemory("active"); err != nil {
		t.Fatalf("active memory removed: %v", err)
	}

	if err := store.View(context.Background(), func(r port.Reader) error {
		events, err := r.Events(0, 20)
		if err != nil {
			return err
		}
		var compactedEvents int
		for _, event := range events {
			if event.Kind == domain.EventMemoryCompacted {
				compactedEvents++
				if !strings.Contains(event.PayloadRef, "reason=expired") {
					return fmt.Errorf("unexpected compaction payload: %s", event.PayloadRef)
				}
			}
		}
		if compactedEvents != 3 {
			return fmt.Errorf("compaction events = %d", compactedEvents)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSemanticMemoryCompactionRollsBackWholeBatchOnAuditFailure(t *testing.T) {
	clock := source.NewManualClock(time.Date(2026, 7, 20, 1, 0, 0, 0, time.UTC))
	store := memory.New()
	seed, _ := NewSemanticMemory(store, clock, source.NewSequenceIDGenerator(1))
	if err := seed.SaveMemory(context.Background(), domain.LongTermMemory{
		ID: "memory-expired", Key: "expired", Scope: domain.MemoryScopeAgent, Value: "v", Expiration: clock.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := clock.Advance(time.Hour); err != nil {
		t.Fatal(err)
	}
	service, _ := NewSemanticMemory(store, clock, fixedIDGenerator("event_0000000000000001"))
	compacted, err := service.CompactExpired(context.Background(), 1)
	if err == nil || compacted != 0 {
		t.Fatalf("compaction = %d, err=%v", compacted, err)
	}
	if _, err := store.LongTermMemory("expired"); err != nil {
		t.Fatalf("failed audit removed memory: %v", err)
	}
}

type fixedIDGenerator string

func (g fixedIDGenerator) NewID(string) (string, error) { return string(g), nil }
