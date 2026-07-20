package network

import (
	"context"
	"errors"
	"strings"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
)

const maxPeerRPCPayload = 1 << 20

var (
	ErrInvalidRPCRequest     = errors.New("invalid peer rpc request")
	ErrCapabilityUnavailable = errors.New("peer capability unavailable")
	ErrInvalidRPCResponse    = errors.New("invalid peer rpc response")
)

// Router composes discovery with an authenticated transport. It verifies the
// peer capability locally and treats the remote response as untrusted bytes.
type Router struct {
	registry  port.PeerRegistry
	transport port.PeerTransport
}

func NewRouter(registry port.PeerRegistry, transport port.PeerTransport) (*Router, error) {
	if registry == nil || transport == nil {
		return nil, ErrInvalidRPCRequest
	}
	return &Router{registry: registry, transport: transport}, nil
}

func (r *Router) Register(ctx context.Context, peer domain.PeerRecord) error {
	return r.registry.Register(ctx, peer)
}

func (r *Router) Lookup(ctx context.Context, peerID string) (domain.PeerRecord, error) {
	return r.registry.Lookup(ctx, peerID)
}

func (r *Router) List(ctx context.Context) ([]domain.PeerRecord, error) {
	return r.registry.List(ctx)
}

func (r *Router) Evict(ctx context.Context, peerID string) error {
	return r.registry.Evict(ctx, peerID)
}

func (r *Router) Call(ctx context.Context, request domain.PeerRPCRequest) (domain.PeerRPCResponse, error) {
	if err := ctx.Err(); err != nil {
		return domain.PeerRPCResponse{}, err
	}
	if !validRPCField(request.RequestID) || !validRPCField(request.PeerID) ||
		!validRPCField(request.Capability) || len(request.Payload) > maxPeerRPCPayload {
		return domain.PeerRPCResponse{}, ErrInvalidRPCRequest
	}
	peer, err := r.registry.Lookup(ctx, request.PeerID)
	if err != nil {
		return domain.PeerRPCResponse{}, err
	}
	if !hasCapability(peer.Capabilities, request.Capability) {
		return domain.PeerRPCResponse{}, ErrCapabilityUnavailable
	}
	request.Payload = append([]byte(nil), request.Payload...)
	response, err := r.transport.Invoke(ctx, peer, request)
	if err != nil {
		return domain.PeerRPCResponse{}, err
	}
	if response.RequestID != request.RequestID || response.PeerID != request.PeerID || len(response.Payload) > maxPeerRPCPayload {
		return domain.PeerRPCResponse{}, ErrInvalidRPCResponse
	}
	response.Payload = append([]byte(nil), response.Payload...)
	return response, nil
}

func validRPCField(value string) bool {
	return strings.TrimSpace(value) == value && value != "" && len(value) <= 128
}

func hasCapability(capabilities []string, wanted string) bool {
	for _, capability := range capabilities {
		if capability == wanted {
			return true
		}
	}
	return false
}

var _ port.Network = (*Router)(nil)
