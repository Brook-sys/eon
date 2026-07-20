package peersync_test

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	peersync "motor-autonomo/internal/network/sync"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

func TestReconcile_AppliesAuthorizedEventToCanonicalLog(t *testing.T) {
	store := memory.New()
	resolver := &dummyResolver{disposition: peersync.DispositionApply}
	c, err := peersync.NewBoundedInboxCanonicalizer(store, resolver)
	if err != nil {
		t.Fatalf("init canonicalizer: %v", err)
	}

	putInboxRecord(t, store, "apply-message", "apply-event")
	count, err := c.Reconcile(context.Background(), "peer1")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if count != 1 {
		t.Fatalf("reconciled = %d, want 1", count)
	}
	if err := store.View(context.Background(), func(r port.Reader) error {
		events, err := r.Events(0, 10)
		if err != nil {
			return err
		}
		if len(events) != 1 || events[0].ID != "apply-event" || events[0].Sequence != 1 {
			t.Fatalf("canonical events = %+v", events)
		}
		pending, err := r.PendingPeerSyncInboxRecords("peer1", 128)
		if err != nil {
			return err
		}
		if len(pending) != 0 {
			t.Fatalf("pending records = %d, want 0", len(pending))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestReconcile_WithResolver(t *testing.T) {
	store := memory.New()
	resolver := &dummyResolver{disposition: peersync.DispositionDiscard}
	c, err := peersync.NewBoundedInboxCanonicalizer(store, resolver)
	if err != nil {
		t.Fatalf("init canonicalizer: %v", err)
	}

	// Setup an inbox record with one event
	err = store.Update(context.Background(), func(tx port.Transaction) error {
		msg := domain.PeerSyncMessage{
			SchemaVersion: domain.SchemaVersionV1,
			StreamID:      "stream1",
			MessageID:     "msg1",
			Kind:          domain.PeerSyncEventBatch,
			OriginID:      "peer1",
			AfterSequence: 0,
			NextSequence:  1,
			Events: []domain.Event{
				{
					SchemaVersion: domain.SchemaVersionV1,
					ID:            "event1",
					Kind:          "test.event",
					OccurredAt:    time.Now(),
					Sequence:      1,
				},
			},
		}
		record := domain.PeerSyncInboxRecord{
			SchemaVersion: domain.SchemaVersionV1,
			PeerID:        "peer1",
			OriginID:      "peer1",
			MessageID:     "msg1",
			StreamID:      "stream1",
			ReceivedAt:    time.Now(),
			Message:       msg,
		}
		_, _, err := tx.PutPeerSyncInboxRecord(record)
		return err
	})
	if err != nil {
		t.Fatalf("failed to setup inbox record: %v", err)
	}

	// Should reconcile 0 because disposition is Discard
	count, err := c.Reconcile(context.Background(), "peer1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count 0, got %v", count)
	}

	// Make sure it was actually deleted from inbox
	err = store.View(context.Background(), func(r port.Reader) error {
		pending, err := r.PendingPeerSyncInboxRecords("peer1", 128)
		if err != nil {
			return err
		}
		if len(pending) != 0 {
			t.Errorf("expected inbox empty, got %d", len(pending))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error reading inbox: %v", err)
	}
}

func putInboxRecord(t *testing.T, store port.Store, messageID string, eventID domain.EventID) {
	t.Helper()
	err := store.Update(context.Background(), func(tx port.Transaction) error {
		msg := domain.PeerSyncMessage{
			SchemaVersion: domain.SchemaVersionV1,
			StreamID:      "stream1",
			MessageID:     messageID,
			Kind:          domain.PeerSyncEventBatch,
			OriginID:      "peer1",
			AfterSequence: 0,
			NextSequence:  1,
			Events: []domain.Event{{
				SchemaVersion: domain.SchemaVersionV1,
				ID:            eventID,
				Kind:          "test.event",
				OccurredAt:    time.Unix(100, 0).UTC(),
				Sequence:      1,
			}},
		}
		_, _, err := tx.PutPeerSyncInboxRecord(domain.PeerSyncInboxRecord{
			SchemaVersion: domain.SchemaVersionV1,
			PeerID:        "peer1",
			OriginID:      "peer1",
			MessageID:     messageID,
			StreamID:      "stream1",
			ReceivedAt:    time.Unix(101, 0).UTC(),
			Message:       msg,
		})
		return err
	})
	if err != nil {
		t.Fatalf("setup inbox record: %v", err)
	}
}
