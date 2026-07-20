package peersync

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
)

type peerCallerFunc func(context.Context, domain.PeerRPCRequest) (domain.PeerRPCResponse, error)

func (f peerCallerFunc) Call(ctx context.Context, request domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
	return f(ctx, request)
}

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

func TestPullOnceCommitsBeforeAckAndRecoversAfterCrash(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(100, 0).UTC()
	localStore := memory.New()
	remoteStore := memory.New()
	if err := remoteStore.Update(ctx, func(tx port.Transaction) error {
		for i := uint64(1); i <= 2; i++ {
			event := syncServiceEvent(fmt.Sprintf("remote-%d", i), 0)
			if _, err := tx.AppendEvent(event); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	local, _ := NewService(localStore, func() time.Time { return now })
	remote, _ := NewService(remoteStore, func() time.Time { return now })
	caller := peerCallerFunc(func(ctx context.Context, request domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
		message, err := Decode(request.Payload)
		if err != nil {
			return domain.PeerRPCResponse{}, err
		}
		response, err := remote.Handle(ctx, "local", "remote", message)
		if err != nil {
			return domain.PeerRPCResponse{}, err
		}
		payload, err := Encode(response)
		return domain.PeerRPCResponse{RequestID: request.RequestID, PeerID: request.PeerID, Payload: payload}, err
	})
	crash := errors.New("simulated crash")
	if _, err := local.PullOnce(ctx, caller, "remote", "local", "events", func(point PullCheckpoint) error {
		if point == PullAfterCommit {
			return crash
		}
		return nil
	}); !errors.Is(err, crash) {
		t.Fatalf("crash error = %v", err)
	}
	if err := localStore.View(ctx, func(reader port.Reader) error {
		cursor, err := reader.PeerSyncCursor("remote", "remote", "events", domain.PeerSyncInbound)
		if err != nil || cursor.NextSequence != 2 {
			t.Fatalf("durable cursor after crash = %+v, err=%v", cursor, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	result, err := local.PullOnce(ctx, caller, "remote", "local", "events", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Events != 0 || result.NextSequence != 2 {
		t.Fatalf("restart result = %+v", result)
	}
	if err := remoteStore.View(ctx, func(reader port.Reader) error {
		cursor, err := reader.PeerSyncCursor("local", "remote", "events", domain.PeerSyncOutboundAck)
		if err != nil || cursor.NextSequence != 2 {
			t.Fatalf("remote ack cursor = %+v, err=%v", cursor, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPullOnceDoesNotCommitResponseLostBeforeDurableBoundary(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(100, 0).UTC()
	local, _ := NewService(memory.New(), func() time.Time { return now })
	caller := peerCallerFunc(func(_ context.Context, request domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
		pull, _ := Decode(request.Payload)
		batch := domain.PeerSyncMessage{SchemaVersion: domain.SchemaVersionV1, StreamID: pull.StreamID, MessageID: pull.MessageID, Kind: domain.PeerSyncEventBatch, OriginID: "remote", AfterSequence: pull.AfterSequence, NextSequence: 1, Events: []domain.Event{syncServiceEvent("remote-1", 1)}}
		payload, _ := Encode(batch)
		return domain.PeerRPCResponse{RequestID: request.RequestID, PeerID: request.PeerID, Payload: payload}, nil
	})
	crash := errors.New("simulated crash")
	if _, err := local.PullOnce(ctx, caller, "remote", "local", "events", func(point PullCheckpoint) error { return crash }); !errors.Is(err, crash) {
		t.Fatalf("crash error = %v", err)
	}
	if _, exists, err := local.inboundPosition(ctx, "remote", "events"); err != nil || exists {
		t.Fatalf("cursor exists before commit: exists=%v err=%v", exists, err)
	}
}

func syncServiceEvent(id string, sequence uint64) domain.Event {
	return domain.Event{SchemaVersion: domain.SchemaVersionV1, ID: domain.EventID(id), Sequence: sequence, Kind: "remote.observed", OccurredAt: time.Unix(int64(sequence), 0).UTC()}
}
