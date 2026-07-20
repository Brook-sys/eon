package subagentstatus

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"motor-autonomo/internal/kernel"
)

const (
	Capability       = "subagent.status.v1"
	TransportPeerKey = kernel.SubagentTransportPeerLabel
	maxPayloadBytes  = 72 << 10
	maxResultBytes   = 64 << 10
	maxFailureBytes  = 4 << 10
)

var ErrInvalidObservation = errors.New("invalid subagent status observation")

// Observation is an authenticated transport report. It is evidence only: the
// SessionManager validates the lifecycle transition and the Supervisor remains
// the sole component that writes canonical records and wake events.
type Observation struct {
	DeliveryID string              `json:"delivery_id"`
	SessionID  string              `json:"session_id"`
	Attempt    int                 `json:"attempt"`
	State      kernel.SessionState `json:"state"`
	Result     string              `json:"result,omitempty"`
	Failure    string              `json:"failure,omitempty"`
}

type Acknowledgement struct {
	SessionID string              `json:"session_id"`
	State     kernel.SessionState `json:"state"`
}

type Service struct {
	manager *kernel.PersistentSessionManager
}

type Handler interface {
	Handle(context.Context, string, []byte) ([]byte, error)
}

func NewService(manager *kernel.PersistentSessionManager) (*Service, error) {
	if manager == nil {
		return nil, errors.New("subagent status manager is required")
	}
	return &Service{manager: manager}, nil
}

func (s *Service) Handle(ctx context.Context, callerID string, payload []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validField(callerID) || len(payload) == 0 || len(payload) > maxPayloadBytes {
		return nil, ErrInvalidObservation
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var observation Observation
	if err := decoder.Decode(&observation); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return nil, ErrInvalidObservation
	}
	if !validField(observation.DeliveryID) || !validField(observation.SessionID) || observation.Attempt < 0 || len(observation.Result) > maxResultBytes || len(observation.Failure) > maxFailureBytes {
		return nil, ErrInvalidObservation
	}

	// The expected reporter is checked against the durable transport binding;
	// certificate identity, not payload or model output, establishes callerID.
	if err := s.manager.AdmitRemoteStatus(ctx, callerID, observation.DeliveryID, kernel.SubagentObservation{ID: kernel.SessionID(observation.SessionID), Attempt: observation.Attempt, State: observation.State, Result: observation.Result, Failure: observation.Failure}); err != nil {
		return nil, err
	}
	return json.Marshal(Acknowledgement{SessionID: observation.SessionID, State: observation.State})
}

func Encode(observation Observation) ([]byte, error) {
	if !validField(observation.DeliveryID) || !validField(observation.SessionID) || observation.Attempt < 0 || len(observation.Result) > maxResultBytes || len(observation.Failure) > maxFailureBytes {
		return nil, ErrInvalidObservation
	}
	payload, err := json.Marshal(observation)
	if err != nil || len(payload) > maxPayloadBytes {
		return nil, ErrInvalidObservation
	}
	return payload, nil
}

func DecodeAcknowledgement(payload []byte) (Acknowledgement, error) {
	if len(payload) == 0 || len(payload) > maxPayloadBytes {
		return Acknowledgement{}, ErrInvalidObservation
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var acknowledgement Acknowledgement
	if err := decoder.Decode(&acknowledgement); err != nil || decoder.Decode(&struct{}{}) != io.EOF || !validField(acknowledgement.SessionID) {
		return Acknowledgement{}, ErrInvalidObservation
	}
	return acknowledgement, nil
}

func validField(value string) bool {
	return value != "" && len(value) <= 128 && strings.TrimSpace(value) == value
}
