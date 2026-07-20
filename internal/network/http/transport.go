package peerhttp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

const (
	RPCPath             = "/v1/peer/rpc"
	maxPeerPayloadBytes = 1 << 20
	// JSON encodes []byte as base64, so the wire frame must be larger than the
	// raw payload contract. Keep a separate bounded ceiling instead of silently
	// reducing the usable peer payload below 1 MiB.
	maxJSONFrameBytes = 2 << 20
)

var ErrInvalidFrame = errors.New("invalid peer rpc frame")

type frame struct {
	RequestID  string `json:"request_id"`
	PeerID     string `json:"peer_id"`
	Capability string `json:"capability,omitempty"`
	Payload    []byte `json:"payload"`
}

// Transport performs a single bounded JSON RPC over HTTPS. The supplied TLS
// config is expected to require and verify mutual TLS.
type Transport struct {
	client *http.Client
}

func NewTransport(config *tls.Config, timeout time.Duration) (*Transport, error) {
	if config == nil || config.MinVersion < tls.VersionTLS13 || config.ClientCAs == nil ||
		config.RootCAs == nil || config.ClientAuth != tls.RequireAndVerifyClientCert || len(config.Certificates) == 0 || timeout <= 0 {
		return nil, ErrInvalidFrame
	}
	return &Transport{client: &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{TLSClientConfig: config.Clone(), DisableCompression: true},
	}}, nil
}

func (t *Transport) Invoke(ctx context.Context, peer domain.PeerRecord, request domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
	if t == nil || t.client == nil || peer.Address.Host == "" || peer.Address.Port < 1 || peer.Address.Port > 65535 {
		return domain.PeerRPCResponse{}, ErrInvalidFrame
	}
	if len(request.Payload) > maxPeerPayloadBytes {
		return domain.PeerRPCResponse{}, ErrInvalidFrame
	}
	body, err := json.Marshal(frame{RequestID: request.RequestID, PeerID: request.PeerID, Capability: request.Capability, Payload: request.Payload})
	if err != nil || len(body) > maxJSONFrameBytes {
		return domain.PeerRPCResponse{}, ErrInvalidFrame
	}
	endpoint := url.URL{Scheme: "https", Host: peer.Address.Host + ":" + strconv.Itoa(peer.Address.Port), Path: RPCPath}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return domain.PeerRPCResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return domain.PeerRPCResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/json" {
		return domain.PeerRPCResponse{}, fmt.Errorf("peer rpc status: %d", resp.StatusCode)
	}
	var result frame
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxJSONFrameBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil || decoder.Decode(&struct{}{}) != io.EOF || result.Capability != "" || len(result.Payload) > maxPeerPayloadBytes {
		return domain.PeerRPCResponse{}, ErrInvalidFrame
	}
	return domain.PeerRPCResponse{RequestID: result.RequestID, PeerID: result.PeerID, Payload: result.Payload}, nil
}

var _ port.PeerTransport = (*Transport)(nil)
