package peersync

import (
	"context"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

func TestServiceStoresBatchAdvancesCursorAndDeduplicates(t *testing.T) {
	store := memory.New()
	now := time.Unix(100, 0).UTC()
	service, err := NewService(store, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	message := domain.PeerSyncMessage{
		SchemaVersion: domain.SchemaVersionV1, StreamID: "stream-1", MessageID: "message-1",
		Kind: domain.PeerSyncEventBatch, OriginID: "node-a", AfterSequence: 0, NextSequence: 2,
		Events: []domain.Event{syncServiceEvent("event-1", 1), syncServiceEvent("event-2", 2)},
	}
	response, err := service.Handle(context.Background(), "node-a", "node-local", message)
	if err != nil {
		t.Fatal(err)
	}
	if response.Kind != domain.PeerSyncAck || response.OriginID != "node-local" || response.NextSequence != 2 {
		t.Fatalf("unexpected ack: %+v", response)
	}
	if err := store.View(context.Background(), func(reader port.Reader) error {
		record, err := reader.PeerSyncInboxRecord("node-a", "node-a", "message-1")
		if err != nil || len(record.Message.Events) != 2 {
			t.Fatalf("inbox = %+v, err = %v", record, err)
		}
		cursor, err := reader.PeerSyncCursor("node-a", "node-a", "stream-1", domain.PeerSyncInbound)
		if err != nil || cursor.NextSequence != 2 || cursor.Revision != 0 {
			t.Fatalf("cursor = %+v, err = %v", cursor, err)
		}
		local, err := reader.Events(0, 10)
		if err != nil || len(local) != 0 {
			t.Fatalf("remote events entered canonical log: %+v, err=%v", local, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Handle(context.Background(), "node-a", "node-local", message); err != nil {
		t.Fatalf("replay failed: %v", err)
	}
}

func TestServiceRejectsIdentityMismatchAndCursorGap(t *testing.T) {
	service, err := NewService(memory.New(), func() time.Time { return time.Unix(100, 0).UTC() })
	if err != nil {
		t.Fatal(err)
	}
	message := domain.PeerSyncMessage{SchemaVersion: domain.SchemaVersionV1, StreamID: "s", MessageID: "m", Kind: domain.PeerSyncEventBatch, OriginID: "node-a", AfterSequence: 1, NextSequence: 1}
	if _, err := service.Handle(context.Background(), "node-b", "local", message); !errors.Is(err, ErrInvalidPeerIdentity) {
		t.Fatalf("identity error = %v", err)
	}
	if _, err := service.Handle(context.Background(), "node-a", "local", message); !errors.Is(err, ErrCursorGap) {
		t.Fatalf("gap error = %v", err)
	}
}

func TestServiceAckAdvancesPeerScopedOutboundCursor(t *testing.T) {
	store := memory.New()
	now := time.Unix(100, 0).UTC()
	service, _ := NewService(store, func() time.Time { return now })
	ack := domain.PeerSyncMessage{SchemaVersion: domain.SchemaVersionV1, StreamID: "stream", MessageID: "ack-1", Kind: domain.PeerSyncAck, OriginID: "peer-a", NextSequence: 7}
	if _, err := service.Handle(context.Background(), "peer-a", "node-local", ack); err != nil {
		t.Fatal(err)
	}
	if err := store.View(context.Background(), func(reader port.Reader) error {
		cursor, err := reader.PeerSyncCursor("peer-a", "node-local", "stream", domain.PeerSyncOutboundAck)
		if err != nil || cursor.NextSequence != 7 {
			t.Fatalf("cursor=%+v err=%v", cursor, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Handle(context.Background(), "peer-a", "node-local", ack); err != nil {
		t.Fatalf("ack replay failed: %v", err)
	}
}

func TestServicePullReadsBoundedLocalEvents(t *testing.T) {
	store := memory.New()
	now := time.Unix(100, 0).UTC()
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		for i := 0; i < domain.MaxPeerSyncEvents+2; i++ {
			event := domain.Event{SchemaVersion: domain.SchemaVersionV1, ID: domain.EventID("event-" + string(rune('a'+i))), Kind: "local", OccurredAt: now}
			if _, err := tx.AppendEvent(event); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	service, _ := NewService(store, func() time.Time { return now })
	response, err := service.Handle(context.Background(), "node-a", "node-local", domain.PeerSyncMessage{SchemaVersion: domain.SchemaVersionV1, StreamID: "s", MessageID: "m", Kind: domain.PeerSyncPull, OriginID: "node-a"})
	if err != nil {
		t.Fatal(err)
	}
	if response.Kind != domain.PeerSyncEventBatch || len(response.Events) != domain.MaxPeerSyncEvents || response.NextSequence != domain.MaxPeerSyncEvents {
		t.Fatalf("unexpected batch: kind=%s events=%d next=%d", response.Kind, len(response.Events), response.NextSequence)
	}
}

func syncServiceEvent(id string, sequence uint64) domain.Event {
	return domain.Event{SchemaVersion: domain.SchemaVersionV1, ID: domain.EventID(id), Sequence: sequence, Kind: "remote.observed", OccurredAt: time.Unix(int64(sequence), 0).UTC()}
}
