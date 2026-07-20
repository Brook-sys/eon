package peersync

import (
	"bytes"
	"errors"
	"testing"

	"motor-autonomo/internal/domain"
)

func TestFrameRoundTrip(t *testing.T) {
	message := domain.PeerSyncMessage{SchemaVersion: domain.SchemaVersionV1, StreamID: "stream-1", MessageID: "message-1", Kind: domain.PeerSyncPull, OriginID: "node-a", AfterSequence: 42}
	payload, err := Encode(message)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := Decode(payload)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.StreamID != message.StreamID || decoded.AfterSequence != 42 {
		t.Fatalf("round-trip mismatch: %+v", decoded)
	}
}

func TestDecodeRejectsUnknownTrailingAndOversize(t *testing.T) {
	for _, payload := range [][]byte{
		[]byte(`{"schema_version":1,"stream_id":"s","message_id":"m","kind":"PULL","origin_id":"n","unknown":true}`),
		[]byte(`{"schema_version":1,"stream_id":"s","message_id":"m","kind":"PULL","origin_id":"n"} {}`),
		bytes.Repeat([]byte("x"), MaxFrameBytes+1),
	} {
		if _, err := Decode(payload); !errors.Is(err, ErrInvalidFrame) {
			t.Fatalf("invalid payload accepted: %v", err)
		}
	}
}
