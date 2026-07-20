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

var ErrPeerNotFound = errors.New("peer not found in registry")
