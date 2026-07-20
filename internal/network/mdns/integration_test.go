package mdns

import (
	"context"
	"net"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
)

func TestBeacon_Integration_PeerDiscovery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Test loopback integration using a normal UDP socket instead of multicast
	// to avoid environmental restrictions on multicast routing in CI/sandbox.
	addr, err := net.ResolveUDPAddr("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	connA, err := net.ListenUDP("udp4", addr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer connA.Close()

	connB, err := net.ListenUDP("udp4", addr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer connB.Close()

	regA := &mockRegistry{peers: make(map[string]domain.PeerRecord)}
	cfgA := MDNSConfig{
		NodeID:           "node-a",
		AllowedPKIHashes: []string{"node-b"},
	}
	beaconA := &Beacon{
		config:   cfgA,
		registry: regA,
		conn:     connA,
		running:  true,
	}

	regB := &mockRegistry{peers: make(map[string]domain.PeerRecord)}
	cfgB := MDNSConfig{
		NodeID:           "node-b",
		AllowedPKIHashes: []string{"node-a"},
	}
	beaconB := &Beacon{
		config:   cfgB,
		registry: regB,
		conn:     connB,
		running:  true,
	}

	go beaconA.listen(ctx)
	go beaconB.listen(ctx)

	// Manual send cross-peers
	_, err = connA.WriteToUDP([]byte("NODE:node-a"), connB.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatalf("write A->B: %v", err)
	}

	_, err = connB.WriteToUDP([]byte("NODE:node-b"), connA.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatalf("write B->A: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Verify A discovered B
	beaconA.mu.Lock()
	defer beaconA.mu.Unlock()
	if len(regA.peers) == 0 {
		t.Errorf("beacon A did not discover beacon B")
	} else if _, ok := regA.peers["node-b"]; !ok {
		t.Errorf("beacon A discovered wrong peer: %v", regA.peers)
	}

	// Verify B discovered A
	beaconB.mu.Lock()
	defer beaconB.mu.Unlock()
	if len(regB.peers) == 0 {
		t.Errorf("beacon B did not discover beacon A")
	} else if _, ok := regB.peers["node-a"]; !ok {
		t.Errorf("beacon B discovered wrong peer: %v", regB.peers)
	}
}
