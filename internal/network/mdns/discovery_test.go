package mdns

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
)

type mockRegistry struct {
	peers map[string]domain.PeerRecord
}

func (m *mockRegistry) Register(ctx context.Context, peer domain.PeerRecord) error {
	m.peers[peer.Identity.ID] = peer
	return nil
}

func (m *mockRegistry) Lookup(ctx context.Context, id string) (domain.PeerRecord, error) {
	return domain.PeerRecord{}, nil
}

func (m *mockRegistry) List(ctx context.Context) ([]domain.PeerRecord, error) {
	return nil, nil
}

func (m *mockRegistry) Evict(ctx context.Context, id string) error {
	return nil
}

func TestBeacon_StartStop(t *testing.T) {
	registry := &mockRegistry{peers: make(map[string]domain.PeerRecord)}
	config := MDNSConfig{
		NodeID: "test-node",
	}

	beacon, err := NewBeacon(config, registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := beacon.Start(ctx); err != nil {
		t.Fatalf("failed to start: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if err := beacon.Stop(); err != nil {
		t.Fatalf("failed to stop: %v", err)
	}
}

func TestBeacon_ValidateAndRegister(t *testing.T) {
	registry := &mockRegistry{peers: make(map[string]domain.PeerRecord)}
	config := MDNSConfig{
		NodeID:           "test-node",
		AllowedPKIHashes: []string{"authorized-peer"},
	}

	beacon, _ := NewBeacon(config, registry)

	ctx := context.Background()

	// Should not register unauthorized peer
	beacon.validateAndRegister(ctx, "unauthorized-peer", "10.0.0.1:8080")
	if len(registry.peers) != 0 {
		t.Fatalf("expected 0 peers, got %d", len(registry.peers))
	}

	// Should register authorized peer
	beacon.validateAndRegister(ctx, "authorized-peer", "10.0.0.2:8080")
	if len(registry.peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(registry.peers))
	}

	if p := registry.peers["authorized-peer"]; p.Address.Host != "10.0.0.2" || p.Address.Port != 8080 {
		t.Fatalf("expected address 10.0.0.2:8080, got %s:%d", p.Address.Host, p.Address.Port)
	}
}
