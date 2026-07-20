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

func TestInboxCanonicalizer_Reconcile(t *testing.T) {
	store := memory.New()
	c := peersync.NewBoundedInboxCanonicalizer(store)

	// Test empty peerID
	_, err := c.Reconcile(context.Background(), "")
	if err == nil || err.Error() != "peer sync origin does not match authenticated caller" {
		t.Errorf("expected ErrInvalidPeerIdentity, got %v", err)
	}

	// Test valid skeleton call with empty inbox
	count, err := c.Reconcile(context.Background(), "peer1")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if count != 0 {
		t.Errorf("expected count 0, got %v", count)
	}

	// Test with events in inbox
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

	count, err = c.Reconcile(context.Background(), "peer1")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if count != 1 {
		t.Errorf("expected count 1, got %v", count)
	}
}
