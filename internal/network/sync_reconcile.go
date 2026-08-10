package network

import (
	"context"
	"fmt"
	"time"

	peersync "motor-autonomo/internal/network/sync"
)

// ReconcilePeerEventsNow executa uma rodada imediata e explícita de reconciliação/pull de eventos P2P
// contra um nó específico, processando a inbox canônica de forma síncrona.
func (m *P2PManager) ReconcilePeerEventsNow(ctx context.Context, peerID, streamID string) (peersync.PullResult, error) {
	if m == nil {
		return peersync.PullResult{}, fmt.Errorf("p2p manager cannot be nil")
	}
	if m.Ticker == nil {
		return peersync.PullResult{}, fmt.Errorf("p2p manager ticker not attached; call AttachSyncService first")
	}
	if m.Router == nil {
		return peersync.PullResult{}, fmt.Errorf("router cannot be nil")
	}
	if peerID == "" {
		return peersync.PullResult{}, fmt.Errorf("peerID cannot be empty")
	}

	nodeID := m.Router.localID
	if nodeID == "" {
		nodeID = "node-" + fmt.Sprintf("%d", time.Now().UnixNano())
	}
	if streamID == "" {
		streamID = "events"
	}

	// Executa PullOnce focado no peer especificador
	res, err := m.Ticker.ReconcilePeerNow(ctx, peerID, nodeID, streamID)
	if err != nil {
		return peersync.PullResult{}, fmt.Errorf("reconcile peer now (%s): %w", peerID, err)
	}

	return res, nil
}
