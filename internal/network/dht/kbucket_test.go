package dht_test

import (
	"context"
	"motor-autonomo/internal/network/dht"
	"testing"
)

func TestLocalRoutingTable_AddAndList(t *testing.T) {
	rt := dht.NewLocalRoutingTable("local-node", 5)

	err := rt.Add(context.Background(), dht.PeerEndpoint{
		ID:      "peer-1",
		Address: "127.0.0.1",
		Port:    8081,
	})

	if err != nil {
		t.Fatalf("failed to add peer: %v", err)
	}

	peers := rt.List()
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(peers))
	}
	if peers[0].ID != "peer-1" {
		t.Errorf("expected peer ID peer-1, got %s", peers[0].ID)
	}

	// Ensure self is rejected
	err = rt.Add(context.Background(), dht.PeerEndpoint{ID: "local-node"})
	if err == nil || err.Error() != "cannot route to self" {
		t.Errorf("expected self rejection, got %v", err)
	}
}

func TestLocalRoutingTable_BucketFull(t *testing.T) {
	// K = 2
	rt := dht.NewLocalRoutingTable("local-node", 2)

	rt.Add(context.Background(), dht.PeerEndpoint{ID: "peer-1"})
	rt.Add(context.Background(), dht.PeerEndpoint{ID: "peer-2"})

	err := rt.Add(context.Background(), dht.PeerEndpoint{ID: "peer-3"})
	if err == nil || err.Error() != "bucket is full" {
		t.Errorf("expected bucket full error, got %v", err)
	}

	// Update existing should work even if full
	err = rt.Add(context.Background(), dht.PeerEndpoint{ID: "peer-1", Address: "10.0.0.1"})
	if err != nil {
		t.Errorf("expected update to succeed, got %v", err)
	}

	// Check update
	peers := rt.List()
	for _, p := range peers {
		if p.ID == "peer-1" && p.Address != "10.0.0.1" {
			t.Errorf("expected address updated to 10.0.0.1, got %s", p.Address)
		}
	}
}
