package memory

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"errors"
	"testing"

	"motor-autonomo/internal/domain"
)

func TestCheckpointEnvelopeRoundTrip(t *testing.T) {
	store := New()
	payload, err := store.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	var envelope checkpointEnvelope
	if err := gob.NewDecoder(bytes.NewReader(payload)).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.FormatVersion != CheckpointFormatVersion {
		t.Fatalf("format version = %d, want %d", envelope.FormatVersion, CheckpointFormatVersion)
	}
	if got := sha256.Sum256(envelope.Payload); got != envelope.PayloadDigest {
		t.Fatal("payload digest does not match encoded state")
	}
	if _, err := NewFromBinary(payload); err != nil {
		t.Fatalf("restore current checkpoint: %v", err)
	}
}

func TestCheckpointRejectsTamperedPayload(t *testing.T) {
	store := New()
	payload, err := store.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	var envelope checkpointEnvelope
	if err := gob.NewDecoder(bytes.NewReader(payload)).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	envelope.Payload[len(envelope.Payload)-1] ^= 0xff
	var tampered bytes.Buffer
	if err := gob.NewEncoder(&tampered).Encode(envelope); err != nil {
		t.Fatal(err)
	}
	_, err = NewFromBinary(tampered.Bytes())
	if !errors.Is(err, ErrCheckpointIntegrity) {
		t.Fatalf("restore error = %v, want integrity failure", err)
	}
}

func TestCheckpointRejectsFutureEnvelope(t *testing.T) {
	var payload bytes.Buffer
	if err := gob.NewEncoder(&payload).Encode(checkpointEnvelope{FormatVersion: CheckpointFormatVersion + 1}); err != nil {
		t.Fatal(err)
	}
	_, err := NewFromBinary(payload.Bytes())
	if !errors.Is(err, ErrUnsupportedCheckpointFormat) {
		t.Fatalf("restore error = %v, want unsupported format", err)
	}
}

func TestCheckpointAcceptsLegacyUnwrappedState(t *testing.T) {
	legacy := persistedState{ActiveMissions: map[domain.MissionID]domain.MissionRevisionID{"mission_1": "revision_1"}}
	var payload bytes.Buffer
	if err := gob.NewEncoder(&payload).Encode(legacy); err != nil {
		t.Fatal(err)
	}
	store, err := NewFromBinary(payload.Bytes())
	if err != nil {
		t.Fatalf("restore legacy checkpoint: %v", err)
	}
	if got := store.state.activeMissions["mission_1"]; got != "revision_1" {
		t.Fatalf("active revision = %q, want revision_1", got)
	}
}

func TestCheckpointAcceptsV1Envelope(t *testing.T) {
	legacy := checkpointEnvelopeV1{
		FormatVersion: checkpointFormatV1,
		State: persistedState{ActiveMissions: map[domain.MissionID]domain.MissionRevisionID{
			"mission_1": "revision_1",
		}},
	}
	var payload bytes.Buffer
	if err := gob.NewEncoder(&payload).Encode(legacy); err != nil {
		t.Fatal(err)
	}
	store, err := NewFromBinary(payload.Bytes())
	if err != nil {
		t.Fatalf("restore v1 checkpoint: %v", err)
	}
	if got := store.state.activeMissions["mission_1"]; got != "revision_1" {
		t.Fatalf("active revision = %q, want revision_1", got)
	}
}

func TestSupportsExternalCheckpointFormat(t *testing.T) {
	for _, version := range []int{1, CheckpointFormatVersion} {
		if !SupportsExternalCheckpointFormat(version) {
			t.Fatalf("version %d should be supported", version)
		}
	}
	for _, version := range []int{0, CheckpointFormatVersion + 1} {
		if SupportsExternalCheckpointFormat(version) {
			t.Fatalf("version %d should be rejected", version)
		}
	}
}
