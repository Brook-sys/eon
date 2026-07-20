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

func TestReconcile_WithResolver(t *testing.T) {
	store := memory.New()
	resolver := &dummyResolver{disposition: peersync.DispositionDiscard}
	c := peersync.NewBoundedInboxCanonicalizer(store, resolver)

	// Setup an inbox record with one event
	err := store.Update(context.Background(), func(tx port.Transaction) error {
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
