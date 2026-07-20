package dht_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"motor-autonomo/internal/network/dht"
)

func TestFilePeerStore_SaveLoadRoundTrip(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "dht-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	
	storePath := filepath.Join(tempDir, "peers.json")
	store, err := dht.NewFilePeerStore(storePath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	
	// Initially empty (file doesn't exist)
	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatalf("failed to load empty store: %v", err)
	}
	if len(loaded) != 0 {
		t.Errorf("expected 0 peers, got %d", len(loaded))
	}
	
	// Save peers
	peers := []dht.PeerEndpoint{
		{ID: "node-1", Address: "10.0.0.1", Port: 8080},
		{ID: "node-2", Address: "10.0.0.2", Port: 9090},
	}
	
	if err := store.Save(context.Background(), peers); err != nil {
		t.Fatalf("failed to save peers: %v", err)
	}
	
	// Load back and verify
	loaded, err = store.Load(context.Background())
	if err != nil {
		t.Fatalf("failed to load peers: %v", err)
	}
	
	if len(loaded) != 2 {
		t.Fatalf("expected 2 peers, got %d", len(loaded))
	}
	
	if loaded[0].ID != "node-1" || loaded[1].ID != "node-2" {
		t.Errorf("peers did not match expected IDs")
	}
}

func TestFilePeerStore_InvalidPath(t *testing.T) {
	_, err := dht.NewFilePeerStore("")
	if err == nil || err.Error() != "persistence path cannot be empty" {
		t.Errorf("expected empty path error, got %v", err)
	}
}
