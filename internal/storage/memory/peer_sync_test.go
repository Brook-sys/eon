package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

func TestPeerSyncInboxReplayIgnoresReceivedAtAndPersists(t *testing.T) {
	store := New()
	message := domain.PeerSyncMessage{SchemaVersion: domain.SchemaVersionV1, StreamID: "stream", MessageID: "message", Kind: domain.PeerSyncHello, OriginID: "peer-a"}
	first := domain.PeerSyncInboxRecord{SchemaVersion: domain.SchemaVersionV1, PeerID: "peer-a", OriginID: "peer-a", StreamID: "stream", MessageID: "message", ReceivedAt: time.Unix(10, 0).UTC(), Message: message}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		_, created, err := tx.PutPeerSyncInboxRecord(first)
		if err != nil || !created {
			t.Fatalf("created=%v err=%v", created, err)
		}
		replay := first
		replay.ReceivedAt = time.Unix(20, 0).UTC()
		stored, created, err := tx.PutPeerSyncInboxRecord(replay)
		if err != nil || created || !stored.ReceivedAt.Equal(first.ReceivedAt) {
			t.Fatalf("replay stored=%+v created=%v err=%v", stored, created, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	checkpoint, err := store.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := NewFromBinary(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.View(context.Background(), func(reader port.Reader) error {
		stored, err := reader.PeerSyncInboxRecord("peer-a", "peer-a", "message")
		if err != nil || !stored.ReceivedAt.Equal(first.ReceivedAt) {
			t.Fatalf("stored=%+v err=%v", stored, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPeerSyncCursorScopesStreamsAndRejectsRegression(t *testing.T) {
	store := New()
	at := time.Unix(10, 0).UTC()
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		for _, stream := range []string{"stream-a", "stream-b"} {
			cursor, err := domain.InitialPeerSyncCursor("peer-a", "peer-a", stream, domain.PeerSyncInbound, 5, at)
			if err != nil {
				return err
			}
			if err := tx.SavePeerSyncCursor(cursor, 0); err != nil {
				return err
			}
		}
		cursor, err := tx.PeerSyncCursor("peer-a", "peer-a", "stream-a", domain.PeerSyncInbound)
		if err != nil {
			return err
		}
		cursor.NextSequence = 4
		cursor.Revision++
		return tx.SavePeerSyncCursor(cursor, 0)
	}); !errors.Is(err, port.ErrConflict) {
		t.Fatalf("regression error=%v", err)
	}
}
