package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

const MaxSubagentSpawnPayloadBytes = 72 << 10
const maxSubagentSpawnTaskBytes = 64 << 10

var ErrInvalidSubagentSpawnRPC = errors.New("invalid subagent spawn rpc")

type SubagentSpawnRequest struct {
	RequestID   string `json:"request_id"`
	SessionID   string `json:"session_id"`
	Attempt     int    `json:"attempt"`
	Task        string `json:"task"`
	ContextMode string `json:"context_mode"`
}

type SubagentSpawnAcknowledgement struct {
	RequestID         string `json:"request_id"`
	SessionID         string `json:"session_id"`
	Attempt           int    `json:"attempt"`
	ReceiverSessionID string `json:"receiver_session_id"`
	Accepted          bool   `json:"accepted"`
	Code              string `json:"code,omitempty"`
	Retryable         bool   `json:"retryable,omitempty"`
}

func EncodeSubagentSpawnRequest(request SubagentSpawnRequest) ([]byte, error) {
	if err := ValidateSubagentSpawnRequest(request); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(request)
	if err != nil || len(payload) > MaxSubagentSpawnPayloadBytes {
		return nil, ErrInvalidSubagentSpawnRPC
	}
	return payload, nil
}

func DecodeSubagentSpawnRequest(payload []byte) (SubagentSpawnRequest, error) {
	if len(payload) == 0 || len(payload) > MaxSubagentSpawnPayloadBytes {
		return SubagentSpawnRequest{}, ErrInvalidSubagentSpawnRPC
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var request SubagentSpawnRequest
	if err := decoder.Decode(&request); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return SubagentSpawnRequest{}, ErrInvalidSubagentSpawnRPC
	}
	return request, ValidateSubagentSpawnRequest(request)
}

func DecodeSubagentSpawnAcknowledgement(payload []byte) (SubagentSpawnAcknowledgement, error) {
	if len(payload) == 0 || len(payload) > MaxSubagentSpawnPayloadBytes {
		return SubagentSpawnAcknowledgement{}, ErrInvalidSubagentSpawnRPC
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var ack SubagentSpawnAcknowledgement
	if err := decoder.Decode(&ack); err != nil || decoder.Decode(&struct{}{}) != io.EOF || !ValidSubagentRPCField(ack.RequestID) || !ValidSubagentRPCField(ack.SessionID) || ack.Attempt < 0 {
		return SubagentSpawnAcknowledgement{}, ErrInvalidSubagentSpawnRPC
	}
	if ack.Accepted {
		if !ValidSubagentRPCField(ack.ReceiverSessionID) || ack.Code != "" || ack.Retryable {
			return SubagentSpawnAcknowledgement{}, ErrInvalidSubagentSpawnRPC
		}
	} else if !ValidSubagentRPCField(ack.Code) || ack.ReceiverSessionID != "" {
		return SubagentSpawnAcknowledgement{}, ErrInvalidSubagentSpawnRPC
	}
	return ack, nil
}

func ValidateSubagentSpawnRequest(request SubagentSpawnRequest) error {
	if !ValidSubagentRPCField(request.RequestID) || !ValidSubagentRPCField(request.SessionID) || request.Attempt < 0 || request.Task == "" || len(request.Task) > maxSubagentSpawnTaskBytes || (request.ContextMode != "isolated" && request.ContextMode != "fork") {
		return ErrInvalidSubagentSpawnRPC
	}
	return nil
}

func ValidSubagentRPCField(value string) bool {
	return value != "" && len(value) <= 128 && strings.TrimSpace(value) == value
}
