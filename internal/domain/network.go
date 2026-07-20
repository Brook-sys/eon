package domain

import (
	"errors"
	"time"
)

// NodeIdentity represents the cryptographic identity of a peer node.
type NodeIdentity struct {
	ID        string
	PublicKey []byte
}

// PeerAddress represents a routable endpoint for a peer.
type PeerAddress struct {
	Host string
	Port int
}

// PeerRecord represents a known subagent node in the network registry.
type PeerRecord struct {
	Identity     NodeIdentity
	Address      PeerAddress
	Capabilities []string
	LastSeen     time.Time
}

// PeerRegistryPolicy defines constraints for accepting and retaining peers.
type PeerRegistryPolicy struct {
	MaxPeers        int
	EvictionTimeout time.Duration
}

// PeerRPCRequest is a bounded, authority-free request addressed to a
// capability advertised by a peer. Payload is opaque to the network layer.
type PeerRPCRequest struct {
	RequestID  string
	PeerID     string
	CallerID   string
	Capability string
	Payload    []byte
}

// PeerRPCResponse preserves the remote result without granting it authority
// over canonical state. Callers must validate the payload before use.
type PeerRPCResponse struct {
	RequestID string
	PeerID    string
	Payload   []byte
}

var ErrPeerNotFound = errors.New("peer not found in registry")
