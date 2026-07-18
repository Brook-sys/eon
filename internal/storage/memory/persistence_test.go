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

func TestCheckpointRejectsTrailingGobDocument(t *testing.T) {
	payload, err := New().MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	var trailing bytes.Buffer
	trailing.Write(payload)
	if err := gob.NewEncoder(&trailing).Encode("unexpected second document"); err != nil {
		t.Fatal(err)
	}
	_, err = NewFromBinary(trailing.Bytes())
	if !errors.Is(err, ErrCheckpointFraming) {
		t.Fatalf("restore error = %v, want framing failure", err)
	}
}

func TestValidateExternalCheckpointRequiresMatchingEnvelope(t *testing.T) {
	current, err := New().MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	legacy := persistedState{ActiveMissions: map[domain.MissionID]domain.MissionRevisionID{"mission_1": "revision_1"}}
	var v0 bytes.Buffer
	if err := gob.NewEncoder(&v0).Encode(legacy); err != nil {
		t.Fatal(err)
	}
	v1Envelope := checkpointEnvelopeV1{FormatVersion: checkpointFormatV1, State: legacy}
	var v1 bytes.Buffer
	if err := gob.NewEncoder(&v1).Encode(v1Envelope); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name    string
		version int
		payload []byte
		wantErr bool
	}{
		{name: "v2 table v2 envelope", version: 2, payload: current},
		{name: "v1 table v0 payload", version: 1, payload: v0.Bytes()},
		{name: "v1 table v1 envelope", version: 1, payload: v1.Bytes()},
		{name: "v2 table rejects v0 payload", version: 2, payload: v0.Bytes(), wantErr: true},
		{name: "v2 table rejects v1 envelope", version: 2, payload: v1.Bytes(), wantErr: true},
		{name: "v1 table rejects v2 envelope", version: 1, payload: current, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateExternalCheckpoint(tc.version, tc.payload)
			if tc.wantErr && !errors.Is(err, ErrCheckpointFormatMismatch) {
				t.Fatalf("validation error = %v, want format mismatch", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validation error = %v", err)
			}
		})
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
