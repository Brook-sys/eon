package kernel

import (
	"motor-autonomo/internal/port"
)

// PeerTransport bridges the kernel and the P2P networking layer.
type PeerTransport struct {
	Registry port.PeerRegistry
	Caller   port.PeerCaller
	Handler  port.PeerRPCHandler
}

// NewPeerTransport creates a new bridge.
func NewPeerTransport(registry port.PeerRegistry, caller port.PeerCaller) *PeerTransport {
	handler, _ := caller.(port.PeerRPCHandler)
	return &PeerTransport{
		Registry: registry,
		Caller:   caller,
		Handler:  handler,
	}
}
