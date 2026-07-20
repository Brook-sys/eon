package port

import (
	"context"

	"motor-autonomo/internal/domain"
)

// PeerRegistry is the domain boundary for subagent network discovery.
type PeerRegistry interface {
	Register(context.Context, domain.PeerRecord) error
	Lookup(context.Context, string) (domain.PeerRecord, error)
	List(context.Context) ([]domain.PeerRecord, error)
	Evict(context.Context, string) error
}

// PeerTransport performs one authenticated RPC against an already resolved
// peer. Implementations own mTLS and framing; they receive no canonical store.
type PeerTransport interface {
	Invoke(context.Context, domain.PeerRecord, domain.PeerRPCRequest) (domain.PeerRPCResponse, error)
}

// PeerCaller resolves a peer and authorizes the requested advertised
// capability before delegating to a transport.
type PeerCaller interface {
	Call(context.Context, domain.PeerRPCRequest) (domain.PeerRPCResponse, error)
}

type PeerRPCHandler interface {
	Handle(context.Context, domain.PeerRPCRequest) (domain.PeerRPCResponse, error)
}

// Network is the narrow kernel-facing interface for authenticated peer
// discovery. Transport adapters must not receive canonical store authority.
type Network interface {
	PeerRegistry
	PeerCaller
}
