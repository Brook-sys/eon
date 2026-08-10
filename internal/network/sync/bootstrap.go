package peersync

import (
	"fmt"
	"time"

	"motor-autonomo/internal/port"
)

// NewSyncService cria uma instância configurada do Service de sincronização P2P (event.sync.v1).
func NewSyncService(store port.Store) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("store cannot be nil")
	}
	svc, err := NewService(store, time.Now)
	if err != nil {
		return nil, fmt.Errorf("failed to create peersync service: %w", err)
	}
	return svc, nil
}
