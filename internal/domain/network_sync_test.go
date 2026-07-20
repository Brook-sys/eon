package domain

import (
	"testing"
	"time"
)

func syncEvent(id string, sequence uint64) Event {
	return Event{SchemaVersion: SchemaVersionV1, ID: EventID(id), Sequence: sequence, Kind: "remote.observed", OccurredAt: time.Unix(int64(sequence), 0).UTC()}
}

func TestPeerSyncEventBatchValidation(t *testing.T) {
	message := PeerSyncMessage{SchemaVersion: SchemaVersionV1, StreamID: "stream-1", MessageID: "message-1", Kind: PeerSyncEventBatch, OriginID: "node-a", AfterSequence: 4, NextSequence: 6, Events: []Event{syncEvent("event-5", 5), syncEvent("event-6", 6)}}
	if err := message.Validate(); err != nil {
		t.Fatalf("valid batch rejected: %v", err)
	}
	message.Events[1].Sequence = 5
	if err := message.Validate(); err == nil {
		t.Fatal("non-monotonic batch accepted")
	}
}

func TestPeerSyncMessageRejectsCrossKindFieldsAndOversize(t *testing.T) {
	pull := PeerSyncMessage{SchemaVersion: SchemaVersionV1, StreamID: "stream", MessageID: "message", Kind: PeerSyncPull, OriginID: "node-a", Events: []Event{syncEvent("event", 1)}}
	if err := pull.Validate(); err == nil {
		t.Fatal("PULL with events accepted")
	}
	batch := PeerSyncMessage{SchemaVersion: SchemaVersionV1, StreamID: "stream", MessageID: "message", Kind: PeerSyncEventBatch, OriginID: "node-a", Events: make([]Event, MaxPeerSyncEvents+1)}
	if err := batch.Validate(); err == nil {
		t.Fatal("oversize event batch accepted")
	}
}

func TestResolvePeerAgendaReplica(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	current := PeerAgendaReplica{SchemaVersion: SchemaVersionV1, OriginID: "node-a", EntityID: "operation-1", Version: 2, UpdatedAt: now, State: "WAITING", PayloadHash: "aaa"}
	newer := current
	newer.Version = 3
	newer.State = "DONE"
	newer.PayloadHash = "bbb"
	winner, conflict, err := ResolvePeerAgendaReplica(current, newer)
	if err != nil || conflict || winner.Version != 3 {
		t.Fatalf("newer version not selected: winner=%+v conflict=%v err=%v", winner, conflict, err)
	}
	divergent := current
	divergent.UpdatedAt = now.Add(time.Second)
	divergent.PayloadHash = "ccc"
	winner, conflict, err = ResolvePeerAgendaReplica(current, divergent)
	if err != nil || !conflict || winner.PayloadHash != "ccc" {
		t.Fatalf("equal-version conflict not converged: winner=%+v conflict=%v err=%v", winner, conflict, err)
	}
	other := current
	other.OriginID = "node-b"
	if _, _, err := ResolvePeerAgendaReplica(current, other); err == nil {
		t.Fatal("cross-origin overwrite accepted")
	}
}
