package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const MaxSubagentReconcilePayloadBytes = 1024

var ErrInvalidSubagentReconcileRPC = errors.New("invalid subagent reconcile rpc")

type SubagentReconcileKind string

const (
	SubagentReconcileSpawn  SubagentReconcileKind = "SPAWN"
	SubagentReconcileStatus SubagentReconcileKind = "STATUS"
)

type SubagentReconcileState string

const (
	SubagentReconcileFound    SubagentReconcileState = "FOUND"
	SubagentReconcileNotFound SubagentReconcileState = "NOT_FOUND"
	SubagentReconcileConflict SubagentReconcileState = "CONFLICT"
)

type SubagentReconcileRequest struct {
	Kind       SubagentReconcileKind `json:"kind"`
	DeliveryID string                `json:"delivery_id"`
	SessionID  string                `json:"session_id"`
	Attempt    int                   `json:"attempt"`
	Digest     string                `json:"digest"`
}

type SubagentReconcileResponse struct {
	Kind              SubagentReconcileKind  `json:"kind"`
	DeliveryID        string                 `json:"delivery_id"`
	SessionID         string                 `json:"session_id"`
	Attempt           int                    `json:"attempt"`
	State             SubagentReconcileState `json:"state"`
	ReceiverSessionID string                 `json:"receiver_session_id,omitempty"`
}

func (r SubagentReconcileRequest) Validate() error {
	if (r.Kind != SubagentReconcileSpawn && r.Kind != SubagentReconcileStatus) || !ValidSubagentRPCField(r.DeliveryID) || !ValidSubagentRPCField(r.SessionID) || r.Attempt < 0 || len(r.Digest) != 64 {
		return ErrInvalidSubagentReconcileRPC
	}
	for _, c := range r.Digest {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return ErrInvalidSubagentReconcileRPC
		}
	}
	return nil
}

func (r SubagentReconcileResponse) Validate() error {
	if (r.Kind != SubagentReconcileSpawn && r.Kind != SubagentReconcileStatus) || !ValidSubagentRPCField(r.DeliveryID) || !ValidSubagentRPCField(r.SessionID) || r.Attempt < 0 || (r.State != SubagentReconcileFound && r.State != SubagentReconcileNotFound && r.State != SubagentReconcileConflict) {
		return ErrInvalidSubagentReconcileRPC
	}
	if r.Kind == SubagentReconcileSpawn && r.State == SubagentReconcileFound {
		if !ValidSubagentRPCField(r.ReceiverSessionID) {
			return ErrInvalidSubagentReconcileRPC
		}
	} else if r.ReceiverSessionID != "" {
		return ErrInvalidSubagentReconcileRPC
	}
	return nil
}

type SubagentTerminalStatus struct {
	DeliveryID string `json:"delivery_id"`
	SessionID  string `json:"session_id"`
	Attempt    int    `json:"attempt"`
	State      string `json:"state"`
	Result     string `json:"result,omitempty"`
	Failure    string `json:"failure,omitempty"`
}

func SubagentSpawnRequestDigest(request SubagentSpawnRequest) (string, error) {
	if ValidateSubagentSpawnRequest(request) != nil {
		return "", ErrInvalidSubagentReconcileRPC
	}
	return subagentReconcileDigest(request)
}

func SubagentTerminalStatusDigest(status SubagentTerminalStatus) (string, error) {
	if !ValidSubagentRPCField(status.DeliveryID) || !ValidSubagentRPCField(status.SessionID) || status.Attempt < 0 || len(status.Result) > MaxSubagentSpawnResultBytes || len(status.Failure) > MaxSubagentSpawnFailureBytes {
		return "", ErrInvalidSubagentReconcileRPC
	}
	if (status.State == "COMPLETE" && status.Failure != "") || (status.State == "FAILED" && (status.Result != "" || status.Failure == "")) || (status.State != "COMPLETE" && status.State != "FAILED") {
		return "", ErrInvalidSubagentReconcileRPC
	}
	return subagentReconcileDigest(status)
}

func subagentReconcileDigest(value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", ErrInvalidSubagentReconcileRPC
	}
	sum := sha256.Sum256(payload)
	return fmt.Sprintf("%x", sum[:]), nil
}

func EncodeSubagentReconcileRequest(request SubagentReconcileRequest) ([]byte, error) {
	if request.Validate() != nil {
		return nil, ErrInvalidSubagentReconcileRPC
	}
	return encodeSubagentReconcile(request)
}

func DecodeSubagentReconcileRequest(payload []byte) (SubagentReconcileRequest, error) {
	var request SubagentReconcileRequest
	if decodeSubagentReconcile(payload, &request) != nil || request.Validate() != nil {
		return SubagentReconcileRequest{}, ErrInvalidSubagentReconcileRPC
	}
	return request, nil
}

func EncodeSubagentReconcileResponse(response SubagentReconcileResponse) ([]byte, error) {
	if response.Validate() != nil {
		return nil, ErrInvalidSubagentReconcileRPC
	}
	return encodeSubagentReconcile(response)
}

func DecodeSubagentReconcileResponse(payload []byte) (SubagentReconcileResponse, error) {
	var response SubagentReconcileResponse
	if decodeSubagentReconcile(payload, &response) != nil || response.Validate() != nil {
		return SubagentReconcileResponse{}, ErrInvalidSubagentReconcileRPC
	}
	return response, nil
}

func encodeSubagentReconcile(value any) ([]byte, error) {
	payload, err := json.Marshal(value)
	if err != nil || len(payload) > MaxSubagentReconcilePayloadBytes {
		return nil, ErrInvalidSubagentReconcileRPC
	}
	return payload, nil
}

func decodeSubagentReconcile(payload []byte, value any) error {
	if len(payload) == 0 || len(payload) > MaxSubagentReconcilePayloadBytes {
		return ErrInvalidSubagentReconcileRPC
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return ErrInvalidSubagentReconcileRPC
	}
	return nil
}
