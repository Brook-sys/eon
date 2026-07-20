package dht

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// PeerEndpoint represents a node discovered on the mesh.
type PeerEndpoint struct {
	ID        string
	Address   string
	Port      int
	LastSeen  time.Time
}

// KBucket represents a bucket of K peers with a similar distance from the local node ID.
type KBucket struct {
	mu    sync.RWMutex
	Peers map[string]PeerEndpoint
	k     int
}

func NewKBucket(k int) *KBucket {
	if k <= 0 {
		k = 20 // Default Kademlia bucket size
	}
	return &KBucket{
		Peers: make(map[string]PeerEndpoint),
		k:     k,
	}
}

func (b *KBucket) AddPeer(peer PeerEndpoint) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	if peer.ID == "" {
		return fmt.Errorf("peer ID cannot be empty")
	}
	
	if _, exists := b.Peers[peer.ID]; exists {
		// Update existing
		p := b.Peers[peer.ID]
		p.LastSeen = time.Now()
		p.Address = peer.Address
		p.Port = peer.Port
		b.Peers[peer.ID] = p
		return nil
	}
	
	if len(b.Peers) >= b.k {
		// In a real DHT we would ping the oldest before evicting,
		// but for this phase we reject to maintain the k-bound.
		return fmt.Errorf("bucket is full")
	}
	
	peer.LastSeen = time.Now()
	b.Peers[peer.ID] = peer
	return nil
}

func (b *KBucket) RemovePeer(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.Peers, id)
}

func (b *KBucket) GetPeers() []PeerEndpoint {
	b.mu.RLock()
	defer b.mu.RUnlock()
	
	out := make([]PeerEndpoint, 0, len(b.Peers))
	for _, p := range b.Peers {
		out = append(out, p)
	}
	return out
}

type LocalRoutingTable struct {
	mu       sync.RWMutex
	localID  string
	// For MVP, we use a single global bucket instead of strict XOR-distance bucketing.
	bucket   *KBucket
}

func NewLocalRoutingTable(localID string, k int) *LocalRoutingTable {
	return &LocalRoutingTable{
		localID: localID,
		bucket:  NewKBucket(k),
	}
}

func (t *LocalRoutingTable) Add(ctx context.Context, peer PeerEndpoint) error {
	if peer.ID == t.localID {
		return fmt.Errorf("cannot route to self")
	}
	return t.bucket.AddPeer(peer)
}

func (t *LocalRoutingTable) Remove(id string) {
	t.bucket.RemovePeer(id)
}

func (t *LocalRoutingTable) List() []PeerEndpoint {
	return t.bucket.GetPeers()
}
