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

// Network is the narrow kernel-facing interface for authenticated peer
// discovery. Transport adapters must not receive canonical store authority.
type Network interface {
	PeerRegistry
}
