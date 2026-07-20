package network

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"

	"motor-autonomo/internal/domain"
)

var (
	ErrInvalidPeer  = errors.New("invalid peer record")
	ErrRegistryFull = errors.New("peer registry full")
)

// StaticRegistry is a bounded, process-local discovery baseline. It performs no
// gossip or network I/O; a later adapter may implement port.Network without
// changing the kernel-facing contract.
type StaticRegistry struct {
	mu     sync.RWMutex
	policy domain.PeerRegistryPolicy
	peers  map[string]domain.PeerRecord
}

func NewStaticRegistry(policy domain.PeerRegistryPolicy) (*StaticRegistry, error) {
	if policy.MaxPeers <= 0 || policy.EvictionTimeout <= 0 {
		return nil, ErrInvalidPeer
	}
	return &StaticRegistry{policy: policy, peers: make(map[string]domain.PeerRecord)}, nil
}

func (r *StaticRegistry) Register(ctx context.Context, peer domain.PeerRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validatePeer(peer); err != nil {
		return err
	}
	peer = clonePeer(peer)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.peers[peer.Identity.ID]; !exists && len(r.peers) >= r.policy.MaxPeers {
		return ErrRegistryFull
	}
	r.peers[peer.Identity.ID] = peer
	return nil
}

func (r *StaticRegistry) Lookup(ctx context.Context, nodeID string) (domain.PeerRecord, error) {
	if err := ctx.Err(); err != nil {
		return domain.PeerRecord{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	peer, ok := r.peers[nodeID]
	if !ok {
		return domain.PeerRecord{}, domain.ErrPeerNotFound
	}
	return clonePeer(peer), nil
}

func (r *StaticRegistry) List(ctx context.Context) ([]domain.PeerRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	peers := make([]domain.PeerRecord, 0, len(r.peers))
	for _, peer := range r.peers {
		peers = append(peers, clonePeer(peer))
	}
	sort.Slice(peers, func(i, j int) bool { return peers[i].Identity.ID < peers[j].Identity.ID })
	return peers, nil
}

func (r *StaticRegistry) Evict(ctx context.Context, nodeID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.peers[nodeID]; !ok {
		return domain.ErrPeerNotFound
	}
	delete(r.peers, nodeID)
	return nil
}

func validatePeer(peer domain.PeerRecord) error {
	if strings.TrimSpace(peer.Identity.ID) == "" || len(peer.Identity.ID) > 128 || len(peer.Identity.PublicKey) == 0 || len(peer.Identity.PublicKey) > 4096 {
		return ErrInvalidPeer
	}
	if strings.TrimSpace(peer.Address.Host) == "" || len(peer.Address.Host) > 253 || peer.Address.Port < 1 || peer.Address.Port > 65535 || peer.LastSeen.IsZero() || len(peer.Capabilities) > 64 {
		return ErrInvalidPeer
	}
	seen := make(map[string]struct{}, len(peer.Capabilities))
	for _, capability := range peer.Capabilities {
		if strings.TrimSpace(capability) == "" || len(capability) > 128 {
			return ErrInvalidPeer
		}
		if _, duplicate := seen[capability]; duplicate {
			return ErrInvalidPeer
		}
		seen[capability] = struct{}{}
	}
	return nil
}

func clonePeer(peer domain.PeerRecord) domain.PeerRecord {
	peer.Identity.PublicKey = append([]byte(nil), peer.Identity.PublicKey...)
	peer.Capabilities = append([]string(nil), peer.Capabilities...)
	return peer
}
