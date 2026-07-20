package peersync

import (
	"bytes"
	"encoding/json"
	"errors"

	"motor-autonomo/internal/domain"
)

const MaxFrameBytes = 1 << 20

var ErrInvalidFrame = errors.New("invalid peer sync frame")

// Encode validates and bounds one multiplexed sync frame before transport.
func Encode(message domain.PeerSyncMessage) ([]byte, error) {
	if err := message.Validate(); err != nil {
		return nil, ErrInvalidFrame
	}
	payload, err := json.Marshal(message)
	if err != nil || len(payload) > MaxFrameBytes {
		return nil, ErrInvalidFrame
	}
	return payload, nil
}

// Decode rejects unknown fields/trailing JSON and never applies received data.
func Decode(payload []byte) (domain.PeerSyncMessage, error) {
	if len(payload) == 0 || len(payload) > MaxFrameBytes {
		return domain.PeerSyncMessage{}, ErrInvalidFrame
	}
	var message domain.PeerSyncMessage
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&message); err != nil || decoder.Decode(&struct{}{}) == nil {
		return domain.PeerSyncMessage{}, ErrInvalidFrame
	}
	if err := message.Validate(); err != nil {
		return domain.PeerSyncMessage{}, ErrInvalidFrame
	}
	return message, nil
}
