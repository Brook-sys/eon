package network

import (
	"fmt"
	"time"

	peersync "motor-autonomo/internal/network/sync"
	"motor-autonomo/internal/port"
)

// AttachSyncService instancia e acopla o serviço de sincronização P2P (event.sync.v1) ao Router do P2PManager.
func (m *P2PManager) AttachSyncService(store port.Store) (*peersync.Service, *peersync.Ticker, error) {
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
		nodeID = "node-" + fmt.Sprintf("%d", time.Now().UnixNano())
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

	return syncService, ticker, nil
}
