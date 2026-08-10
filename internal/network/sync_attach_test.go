package network_test

import (
	"context"
	"testing"
	"time"

	"motor-autonomo/internal/network"
	"motor-autonomo/internal/storage/memory"
)

func TestP2PManager_AttachSyncService(t *testing.T) {
	// Rejeita manager nulo
	t.Run("nil manager", func(t *testing.T) {
		var nilMgr *network.P2PManager
		store := memory.New()
		_, _, err := nilMgr.AttachSyncService(store)
		if err == nil {
			t.Fatalf("expected error on nil manager, got nil")
		}
	})

	opts := network.Options{
		Enabled:  true,
		BindAddr: "127.0.0.1:0",
		NodeID:   "test-node-1",
	}
	mgr, err := network.NewP2PManager(opts, nil)
	if err != nil {
		t.Fatalf("unexpected error creating P2PManager: %v", err)
	}

	// Rejeita store nulo
	_, _, err = mgr.AttachSyncService(nil)
	if err == nil {
		t.Fatalf("expected error on nil store, got nil")
	}

	store := memory.New()
	svc, ticker, err := mgr.AttachSyncService(store)
	if err != nil {
		t.Fatalf("failed to attach sync service: %v", err)
	}
	if svc == nil {
		t.Errorf("expected non-nil sync service")
	}
	if ticker == nil {
		t.Errorf("expected non-nil ticker")
	}

	// Executa um tick sem erros
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := ticker.Tick(ctx); err != nil {
		t.Errorf("unexpected error on ticker tick: %v", err)
	}
}
