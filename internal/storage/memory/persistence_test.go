package memory

import (
	"bytes"
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
	if _, err := NewFromBinary(payload); err != nil {
		t.Fatalf("restore current checkpoint: %v", err)
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
