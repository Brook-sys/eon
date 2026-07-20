package peersync

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

func TestBasicConflictResolver_NovelEvent(t *testing.T) {
	resolver := NewBasicConflictResolver()
	store := memory.New()

	disp := resolveWithStore(t, resolver, store, domain.Event{ID: "evt-123"})
	if disp != DispositionApply {
		t.Errorf("expected DispositionApply for novel event, got %v", disp)
	}
}

func TestBasicConflictResolver_DuplicateEvent(t *testing.T) {
	resolver := NewBasicConflictResolver()
	store := memory.New()

	err := store.Update(context.Background(), func(tx port.Transaction) error {
		_, err := tx.AppendEvent(domain.Event{
			SchemaVersion: domain.SchemaVersionV1,
			ID:            "evt-123",
			Kind:          "test.kind",
			OccurredAt:    time.Unix(100, 0).UTC(),
		})
		return err
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	disp := resolveWithStore(t, resolver, store, domain.Event{ID: "evt-123"})
	if disp != DispositionDiscard {
		t.Errorf("expected DispositionDiscard for duplicate event, got %v", disp)
	}
}

func resolveWithStore(t *testing.T, resolver EventConflictResolver, store port.Store, event domain.Event) ConflictDisposition {
	t.Helper()
	var disposition ConflictDisposition
	if err := store.View(context.Background(), func(reader port.Reader) error {
		var err error
		disposition, err = resolver.ResolveConflict(context.Background(), reader, event)
		return err
	}); err != nil {
		t.Fatalf("resolve conflict: %v", err)
	}
	return disposition
}
