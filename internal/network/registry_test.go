package network

import (
	"context"
	"errors"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
)

func testPeer(id string) domain.PeerRecord {
	return domain.PeerRecord{
		Identity:     domain.NodeIdentity{ID: id, PublicKey: []byte("public-key-" + id)},
		Address:      domain.PeerAddress{Host: id + ".internal", Port: 8443},
		Capabilities: []string{"subagent.spawn"},
		LastSeen:     time.Unix(1_700_000_000, 0).UTC(),
	}
}

func TestStaticRegistryLifecycleAndIsolation(t *testing.T) {
	registry, err := NewStaticRegistry(domain.PeerRegistryPolicy{MaxPeers: 2, EvictionTimeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	first := testPeer("node-b")
	if err := registry.Register(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(context.Background(), testPeer("node-a")); err != nil {
		t.Fatal(err)
	}

	first.Identity.PublicKey[0] = 'X'
	got, err := registry.Lookup(context.Background(), "node-b")
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Identity.PublicKey) != "public-key-node-b" {
		t.Fatalf("stored key mutated: %q", got.Identity.PublicKey)
	}
	got.Capabilities[0] = "mutated"

	listed, err := registry.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if listed[0].Identity.ID != "node-a" || listed[1].Identity.ID != "node-b" {
		t.Fatalf("list is not deterministic: %#v", listed)
	}
	if listed[1].Capabilities[0] != "subagent.spawn" {
		t.Fatal("lookup leaked mutable slice")
	}

	if err := registry.Register(context.Background(), testPeer("node-c")); !errors.Is(err, ErrRegistryFull) {
		t.Fatalf("full registry error = %v", err)
	}
	if err := registry.Evict(context.Background(), "node-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Lookup(context.Background(), "node-a"); !errors.Is(err, domain.ErrPeerNotFound) {
		t.Fatalf("lookup error = %v", err)
	}
}

func TestStaticRegistryRejectsInvalidAndCancelledRequests(t *testing.T) {
	if _, err := NewStaticRegistry(domain.PeerRegistryPolicy{}); !errors.Is(err, ErrInvalidPeer) {
		t.Fatalf("policy error = %v", err)
	}
	registry, err := NewStaticRegistry(domain.PeerRegistryPolicy{MaxPeers: 1, EvictionTimeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	invalid := testPeer("")
	if err := registry.Register(context.Background(), invalid); !errors.Is(err, ErrInvalidPeer) {
		t.Fatalf("invalid peer error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := registry.Register(ctx, testPeer("node-a")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error = %v", err)
	}
}
