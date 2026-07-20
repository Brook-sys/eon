package kernel

import (
	"motor-autonomo/internal/port"
)

// PeerTransport bridges the kernel and the P2P networking layer.
type PeerTransport struct {
	Registry port.PeerRegistry
	Caller   port.PeerCaller
}

// NewPeerTransport creates a new bridge.
func NewPeerTransport(registry port.PeerRegistry, caller port.PeerCaller) *PeerTransport {
	return &PeerTransport{
		Registry: registry,
		Caller:   caller,
	}
}
