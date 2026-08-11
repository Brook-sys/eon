package network

import (
	"fmt"
	"time"

	"motor-autonomo/internal/domain"
	peersync "motor-autonomo/internal/network/sync"
	"motor-autonomo/internal/port"
)

// AttachSyncService instancia e acopla o serviço de sincronização P2P (event.sync.v1) ao Router do P2PManager.
func (m *P2PManager) AttachSyncService(store port.Store) (*peersync.Service, *peersync.Ticker, error) {
	if m == nil {
		return nil, nil, fmt.Errorf("p2p manager cannot be nil")
	}
	if store == nil {
		return nil, nil, fmt.Errorf("store cannot be nil")
	}
	if m.Router == nil {
		return nil, nil, fmt.Errorf("router cannot be nil")
	}

	syncService, err := peersync.NewSyncService(store)
	if err != nil {
		return nil, nil, fmt.Errorf("create peersync service: %w", err)
	}

	nodeID := m.Router.localID
	if nodeID == "" {
		return nil, nil, fmt.Errorf("router localID is empty; NewP2PManager must set it before attaching sync")
	}

	if err := m.Router.AttachSync(nodeID, syncService); err != nil {
		return nil, nil, fmt.Errorf("attach sync to router: %w", err)
	}

	ticker, err := peersync.NewTicker(syncService, m.Router, nodeID, "events", 30*time.Second)
	if err != nil {
		return nil, nil, fmt.Errorf("create peersync ticker: %w", err)
	}

	canonicalizer, err := peersync.NewBoundedInboxCanonicalizer(store, peersync.NewBasicConflictResolver())
	if err != nil {
		return nil, nil, fmt.Errorf("create inbox canonicalizer: %w", err)
	}
	ticker.AttachCanonicalizer(canonicalizer)

	// Attach retention pressure reporter (read-only observer, MVP-safe).
	retentionReporter, err := peersync.NewRetentionReporter(domain.DefaultStoreRetentionPolicy(), store)
	if err != nil {
		return nil, nil, fmt.Errorf("create retention reporter: %w", err)
	}
	ticker.AttachRetentionReporter(retentionReporter)

	m.Ticker = ticker
	return syncService, ticker, nil
}
