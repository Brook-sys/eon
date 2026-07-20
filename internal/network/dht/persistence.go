package dht

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// PeerPersistenceContract defines the interface for durably storing the DHT peer list.
type PeerPersistenceContract interface {
	Save(ctx context.Context, peers []PeerEndpoint) error
	Load(ctx context.Context) ([]PeerEndpoint, error)
}

// FilePeerStore is a simple JSON file-based implementation of PeerPersistenceContract.
type FilePeerStore struct {
	mu   sync.Mutex
	path string
}

func NewFilePeerStore(path string) (*FilePeerStore, error) {
	if path == "" {
		return nil, fmt.Errorf("persistence path cannot be empty")
	}
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return nil, fmt.Errorf("failed to create peer store directory: %w", err)
	}
	return &FilePeerStore{path: path}, nil
}

func (s *FilePeerStore) Save(ctx context.Context, peers []PeerEndpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	data, err := json.Marshal(peers)
	if err != nil {
		return fmt.Errorf("failed to encode peers: %w", err)
	}
	
	// Write to temp file first for atomic replacement
	tempPath := s.path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write temp peer file: %w", err)
	}
	
	if err := os.Rename(tempPath, s.path); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to commit peer file: %w", err)
	}
	
	return nil
}

func (s *FilePeerStore) Load(ctx context.Context) ([]PeerEndpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []PeerEndpoint{}, nil
		}
		return nil, fmt.Errorf("failed to read peer file: %w", err)
	}
	
	var peers []PeerEndpoint
	if err := json.Unmarshal(data, &peers); err != nil {
		return nil, fmt.Errorf("failed to parse peer file: %w", err)
	}
	
	return peers, nil
}
